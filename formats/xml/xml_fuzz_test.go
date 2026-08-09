package xml

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

// FuzzRead exercises xml.Read against arbitrary input text. Per issue #57,
// the only properties asserted are the ones that must hold for ANY input:
// Read must never panic and must never hang (bounded by go test's fuzz
// timeout), and on success the resulting Document must be structurally
// sane. On error nothing is asserted beyond "it's a real error" -- Read's
// documented contract is that failures come back as *omnist.ParseError,
// so we additionally confirm the concrete type without caring which one.
func FuzzRead(f *testing.F) {
	f.Add("<a>1</a>")
	f.Add("")
	f.Add("<r><m>1</m><x>2</x><m>3</m></r>")           // formats-xml/basic/interleaved-elements-preserve-order
	f.Add(`<a x="1"><b>hi</b></a>`)                     // formats-xml/basic/attributes-are-dropped-on-read
	f.Add("<root><m/><x/><m/></root>")                  // repo-test-literal
	f.Add("<root><m>A</m><x>X</x><m>B</m></root>")      // repo-test-literal
	f.Add("<root><items>a</items><items>b</items><items>c</items></root>") // repo-test-literal
	f.Add(`<a x="1" y="2" z="3"/>`)                     // repo-test-literal
	f.Add(`<root xmlns:ns="http://example.com/ns"><ns:b>hi</ns:b></root>`) // repo-test-literal
	f.Add("<ns:b>hi</ns:b>")                            // repo-test-literal
	f.Add("<a/>")                                       // repo-test-literal
	f.Add("<a/><b/>")                                   // repo-test-literal
	f.Add("stray text<a/>")                             // repo-test-literal
	f.Add("</a>")                                       // repo-test-literal
	f.Add("<a/><b")                                     // repo-test-literal
	f.Add("<a/>stray")                                  // repo-test-literal
	f.Add("<a><b></a>")                                 // repo-test-literal
	f.Add("<a><b>")                                     // repo-test-literal
	f.Add("<a>hello<b>x</b>world</a>")                  // repo-test-literal

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
