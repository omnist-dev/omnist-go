package json

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

// FuzzRead exercises json.Read against arbitrary input text. Per issue
// #57, the only properties asserted are the ones that must hold for ANY
// input: Read must never panic and must never hang (bounded by go test's
// fuzz timeout), and on success the resulting Document must be
// structurally sane. On error nothing is asserted beyond "it's a real
// error" -- Read's documented contract is that failures come back as
// *omnist.ParseError, so we additionally confirm the concrete type
// without caring which one.
func FuzzRead(f *testing.F) {
	f.Add(`{"a":1}`)
	f.Add("")
	f.Add(`{"m":[1,2]}`) // formats-json/basic/array-value-becomes-repeated-edges
	f.Add(`{"m":[1]}`)   // formats-json/basic/single-element-array-is-indistinguishable-from-a-bare-value
	f.Add(`{"m":[]}`)    // formats-json/basic/empty-array-value-is-zero-edges-not-an-error
	f.Add(`{"m":[[1,2],[3,4]]}`) // formats-json/basic/nested-array-is-rejected
	f.Add(`{"d":"2024-01-01"}`)  // formats-json/basic/date-looking-string-stays-a-string
	f.Add(`{"m":["A"]}`)         // repo-test-literal
	f.Add(`{}`)                  // repo-test-literal
	f.Add(`{"outer":{"m":[]}}`)  // repo-test-literal
	f.Add(`{"a":{"b":{"c":{"d":1}}}}`)       // repo-test-literal
	f.Add(`{"a":[{"b":1},{"c":{"d":1}}]}`)   // repo-test-literal
	f.Add(`{"a":{},"b":{},"c":{}}`)          // repo-test-literal
	f.Add(`123456`)                          // repo-test-literal
	f.Add(`{"n":12345}`)                     // repo-test-literal
	f.Add(`[1,2]`)                           // repo-test-literal
	f.Add(`{"a":{"b":{"c":1}}}`)             // repo-test-literal
	f.Add(`{"n":1000}`)                      // repo-test-literal

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
