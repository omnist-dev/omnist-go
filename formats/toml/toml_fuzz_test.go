package toml

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// fuzzLimits is deliberately small (unlike omnist.DefaultLimits()) so the
// fuzzer's mutations spend their budget exploring the limit-boundary logic
// itself (depth/node-count/int-digit checks) rather than just building
// large-but-legal documents. See issue #57.
func fuzzLimits() omnist.Limits {
	return omnist.Limits{MaxDepth: 8, MaxNodes: 64, MaxIntDigits: 12}
}

// checkNodeSane walks a Node and fails t if any edge's Target is
// internally inconsistent: a node-target with a nil *omnist.Node. Target
// is a closed struct only constructible via omnist.ValueTarget/NodeTarget
// (the latter panics on nil), so this should never fire -- it's a cheap
// belt-and-suspenders structural check, not a claim about document
// semantics.
func checkNodeSane(t *testing.T, n *omnist.Node, depth int) {
	t.Helper()
	if n == nil {
		t.Fatal("checkNodeSane: nil node")
	}
	if depth > 10000 {
		t.Fatal("checkNodeSane: implausible recursion depth, aborting walk")
	}
	for _, e := range n.Edges {
		if child, ok := e.Target.Node(); ok {
			if child == nil {
				t.Fatalf("edge %q: IsNode target with nil *Node", e.Label)
			}
			checkNodeSane(t, child, depth+1)
		}
	}
}

// FuzzRead exercises toml.Read against arbitrary input text. Per issue
// #57, the only properties asserted are the ones that must hold for ANY
// input: Read must never panic and must never hang (bounded by go test's
// fuzz timeout), and on success the resulting Document must be
// structurally sane. On error nothing is asserted beyond "it's a real
// error" -- Read's documented contract is that failures come back as
// *omnist.ParseError, so we additionally confirm the concrete type
// without caring which one.
func FuzzRead(f *testing.F) {
	f.Add("a = 1\n")
	f.Add("")
	f.Add("[[x]]\nname = \"a\"\n\n[[x]]\nname = \"b\"\n") // formats-toml/basic/array-of-tables-becomes-repeated-edges
	f.Add("d = 2024-01-01\nt = 12:00:00\ndt = 2024-01-01T12:30:00\n") // formats-toml/temporals/native-date-time-and-datetime-all-resolve-with-no-schema
	f.Add("p = {x = 1, y = 2}")               // repo-test-literal
	f.Add("items = [{n = \"a\"}, {n = \"b\"}]") // repo-test-literal
	f.Add("\"q key\" = 9")                     // repo-test-literal
	f.Add("s = \"a\\nb\\tc\\\"d\"")             // repo-test-literal
	f.Add("a = 1\nb = 2\n")                    // repo-test-literal
	f.Add("d = 2024-01-01\n")                  // repo-test-literal
	f.Add("t = 12:30:45\n")                    // repo-test-literal
	f.Add("t = 12:30:45.5\n")                  // repo-test-literal
	f.Add("dt = 2024-01-01T12:00:00\n")        // repo-test-literal
	f.Add("dt = 2024-01-01T12:00:00Z\n")       // repo-test-literal
	f.Add("dt = 2024-01-01 12:00:00z\n")       // repo-test-literal
	f.Add("dt = 2024-01-01T12:00:00.123+05:30\n") // repo-test-literal
	f.Add("dt = 2024-01-01 12:00:00\n")        // repo-test-literal
	f.Add("i = 2\nf = 2.0\n")                  // repo-test-literal
	f.Add("hex = 0xFF\noct = 0o17\nbin = 0b101\n") // repo-test-literal
	f.Add("n = ")                              // repo-test-literal
	f.Add("n = 1_000_000\n")                   // repo-test-literal
	f.Add("n = -42\n")                         // repo-test-literal
	f.Add("a = inf\nb = +inf\nc = -inf\nd = nan\ne = -nan\nf = +nan\n") // repo-test-literal
	f.Add("n = 1_234.5e1_0\n")                 // repo-test-literal
	f.Add("nums = [1, 2, 3]\n")                // repo-test-literal
	f.Add("a = []\n")                          // repo-test-literal
	f.Add("a = [[1, 2], [3, 4]]\n")            // repo-test-literal
	f.Add("host.name = \"x\"\nhost.port = 1\n") // repo-test-literal
	f.Add("a.b.c = 5\n")                       // repo-test-literal
	f.Add("a = 1\na.b = 2\n")                  // repo-test-literal
	f.Add("items = [{a = []}]\n")              // repo-test-literal

	f.Fuzz(func(t *testing.T, text string) {
		doc, err := Read(text, fuzzLimits())
		if err != nil {
			if _, ok := err.(*omnist.ParseError); !ok {
				t.Fatalf("Read returned a non-*omnist.ParseError error: %T: %v", err, err)
			}
			return
		}
		if doc.IsNode {
			checkNodeSane(t, doc.Node, 0)
		}
	})
}
