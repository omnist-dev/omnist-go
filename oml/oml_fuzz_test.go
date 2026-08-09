package oml

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
// (the latter panics on nil), so this should never fire — it's a cheap
// belt-and-suspenders structural check, not a claim about document
// semantics.
func checkNodeSane(t *testing.T, n *omnist.Node, depth int) {
	t.Helper()
	if n == nil {
		t.Fatal("checkNodeSane: nil node")
	}
	if depth > 10000 {
		// Guard the checker itself against unbounded recursion; a Document
		// built under fuzzLimits() can never actually be this deep.
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

// FuzzRead exercises oml.Read against arbitrary input text. Per issue #57,
// the only properties asserted are the ones that must hold for ANY input:
// Read must never panic and must never hang (bounded by go test's fuzz
// timeout), and on success the resulting Document must be structurally
// sane. On error nothing is asserted beyond "it's a real error" -- Read's
// documented contract is that failures come back as *omnist.ParseError,
// so we additionally confirm the concrete type without caring which one.
func FuzzRead(f *testing.F) {
	f.Add("a: 1\n")
	f.Add("")
	f.Add("nan: 1\n") // oml-grammar/reserved/nan-bare-is-a-number-token-not-a-label
	f.Add("\"nan\": 1\n") // oml-grammar/reserved/quoted-nan-is-an-ordinary-label
	f.Add("null: 1\n") // oml-grammar/reserved/null-bare-label-at-top-level-fails-on-trailing-colon
	f.Add("a: { null: 1 }\n") // oml-grammar/reserved/null-bare-label-inside-a-node-is-the-reserved-word-error
	f.Add("b: [1, 2, 3]\n") // oml-grammar/arrays/repeated-label-sugar-expands-in-place
	f.Add("b: []\n") // oml-grammar/arrays/empty-array-is-an-error-not-a-zero-edge-expansion
	f.Add("b: [[1, 2], [3, 4]]\n") // oml-grammar/arrays/nested-array-is-rejected
	f.Add("b: [1\n2]\n") // oml-grammar/arrays/newline-inside-array-is-an-error
	f.Add("a: 'C:\\no\\escapes'\n") // oml-grammar/strings/raw-string-performs-no-escape-processing
	f.Add("a: \"\"\"\nhello\nworld\"\"\"\n") // oml-grammar/strings/multiline-string-strips-leading-newline
	f.Add("a: \"\"\"\nsays \"\"hi\"\" there\"\"\"\n") // oml-grammar/strings/multiline-string-closes-at-first-run-of-three-quotes
	f.Add("a: 2024-01-01T10:30\n") // oml-grammar/temporals/date-then-time-lookahead-yields-one-datetime-token
	f.Add("a: 2024-01-01T99\n") // oml-grammar/temporals/date-then-non-time-suffix-is-date-plus-trailing-content
	f.Add("\"hello\"\n") // oml-grammar/shape/single-top-level-scalar-is-the-whole-document
	f.Add("a: hello\n") // oml-grammar/errors/bare-word-in-value-position-is-an-error
	f.Add("a: \"hi\nthere\"\n") // oml-grammar/errors/literal-control-character-in-string-is-an-error
	f.Add("a: \"\\q\"\n") // oml-grammar/errors/unrecognized-escape-is-an-error
	f.Add("a: \"\\ud800\"\n") // oml-grammar/errors/unpaired-high-surrogate-is-an-error
	f.Add("a: \"hello") // oml-grammar/errors/unterminated-string-is-an-error
	f.Add("2024-01-01T10:30") // repo-test-literal
	f.Add("a: 'C:\\no\\escapes'") // repo-test-literal
	f.Add("\"nan\": 1") // repo-test-literal
	f.Add("b: [1, 2, 3]") // repo-test-literal
	f.Add("a: {}") // repo-test-literal
	f.Add("\"hello\"") // repo-test-literal
	f.Add("2024-01-01") // repo-test-literal
	f.Add("a: \"\\\"\\\\\\/\\b\\f\\n\\r\\t\"") // repo-test-literal
	f.Add("a: \"\\u00e9\"") // repo-test-literal
	f.Add("a: \"\\ud83d\\ude00\"") // repo-test-literal
	f.Add("a: 'a\\b'") // repo-test-literal
	f.Add("a: \"\"\"hi\"\"\"") // repo-test-literal
	f.Add("a: \"\"\"x\"y\"\"\"") // repo-test-literal
	f.Add("a: [1, 2,]") // repo-test-literal
	f.Add("a: [{x: 1}, {x: 2}]") // repo-test-literal
	f.Add("42") // repo-test-literal
	f.Add("null") // repo-test-literal
	f.Add("true") // repo-test-literal
	f.Add("false") // repo-test-literal
	f.Add("name: \"Ann\"; address: { city: \"Zurich\"; postcode: \"8001\" }; tag: \"x\"; tag: \"y\"") // repo-test-literal

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
