package yaml

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

// FuzzRead exercises yaml.Read against arbitrary input text. Per issue
// #57, the only properties asserted are the ones that must hold for ANY
// input: Read must never panic and must never hang (bounded by go test's
// fuzz timeout), and on success the resulting Document must be
// structurally sane. On error nothing is asserted beyond "it's a real
// error" -- Read's documented contract is that failures come back as
// *omnist.ParseError, so we additionally confirm the concrete type
// without caring which one.
func FuzzRead(f *testing.F) {
	f.Add("a: 1\n")
	f.Add("")
	f.Add("m:\n  - 1\n  - 2\n") // formats-yaml/basic/sequence-becomes-repeated-edges
	f.Add("d: 2024-01-01\n")    // formats-yaml/temporals/native-date-resolves-with-no-schema
	f.Add("n: 12:00:00\n")      // formats-yaml/sharp-edges/bare-time-resolves-to-sexagesimal-integer-not-a-time
	f.Add("on:\n  push: true\n") // formats-yaml/sharp-edges/norway-problem-bare-on-key-resolves-to-boolean-and-is-rejected
	f.Add("v: \"no\"")          // repo-test-literal
	f.Add("a: 1\nb: 2\n")       // repo-test-literal
	f.Add("m:\n  - A\n  - B\n") // repo-test-literal
	f.Add("status: no\n")       // repo-test-literal
	f.Add("placed: 2024-01-01\n") // repo-test-literal
	f.Add("a: &x foo\nb: *x\n")   // repo-test-literal
	f.Add("a: &x {k: v}\nb: *x\n") // repo-test-literal
	f.Add("t: 12:00:00\n")         // repo-test-literal
	f.Add("t: -1:02\n")            // repo-test-literal
	f.Add("on: true\n")            // repo-test-literal
	f.Add("\"on\": true\n")        // repo-test-literal
	f.Add("v: ")                   // repo-test-literal
	f.Add("~: value\n")            // repo-test-literal
	f.Add("? {a: 1}\n: value\n")   // repo-test-literal
	f.Add("v: 0x_\n")              // repo-test-literal
	f.Add("v: 1e400\n")            // repo-test-literal
	f.Add("v: hello world\n")      // repo-test-literal
	f.Add("- 1\n- 2\n")            // repo-test-literal
	f.Add("a: []\n")               // repo-test-literal
	f.Add("a:\n  - 1\n  - - 2\n")  // repo-test-literal
	f.Add("42\n")                  // repo-test-literal
	f.Add("a:\n  b:\n    c:\n      d: 1\n") // repo-test-literal
	f.Add("a:\n  - b: 1\n  - c:\n      d: 1\n") // repo-test-literal
	f.Add("a: {}\nb: {}\nc: {}\n") // repo-test-literal
	f.Add("123456\n")              // repo-test-literal
	f.Add("num: 12345\n")          // repo-test-literal

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
