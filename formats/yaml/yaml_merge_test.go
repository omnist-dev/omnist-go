package yaml

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// edgesOf renders one node's edges as "label=scalar" strings in order, so a
// merge test can assert order and values without building a Document by hand.
func edgesOf(t *testing.T, text string, path ...string) []string {
	t.Helper()
	d, err := Read(text, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("Read(%q): %v", text, err)
	}
	n := d.Node
	for _, label := range path {
		var next *omnist.Node
		for _, e := range n.Edges {
			if e.Label == label {
				next, _ = e.Target.Node()
			}
		}
		if next == nil {
			t.Fatalf("no node %q under %v", label, path)
		}
		n = next
	}
	var out []string
	for _, e := range n.Edges {
		if v, ok := e.Target.Value(); ok && !v.IsNull {
			s := v.Scalar
			if s.Kind == omnist.KindString {
				out = append(out, e.Label+"="+s.Str)
			} else {
				out = append(out, e.Label+"="+s.Int.String())
			}
		} else {
			out = append(out, e.Label+"={}")
		}
	}
	return out
}

func eq(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// docs/formats/yaml.md, "Merged entries come first, in source order".
func TestMergeKeyFlattensIntoTheReferringMapping(t *testing.T) {
	// `<<` never survives as a label, and the merged entries come first.
	eq(t, edgesOf(t, "d: &d {a: 1}\ne: {<<: *d, b: 2}\n", "e"), "a=1", "b=2")
	// Where the `<<` entry sits among the mapping's own entries is irrelevant.
	eq(t, edgesOf(t, "d: &d {a: 1}\ne: {b: 2, <<: *d}\n", "e"), "a=1", "b=2")
}

func TestMergeKeySequenceIsInSourceOrder(t *testing.T) {
	text := "base: &base\n  region: eu\nlimits: &limits\n  retries: 3\nsvc:\n  <<: [*base, *limits]\n  name: api\n"
	eq(t, edgesOf(t, text, "svc"), "region=eu", "retries=3", "name=api")
}

func TestMergeKeyCollisionRules(t *testing.T) {
	// The local value wins, in the merged key's position, before or after `<<`.
	eq(t, edgesOf(t, "d: &d {a: 1, b: 2}\ne: {<<: *d, a: 99}\n", "e"), "a=99", "b=2")
	eq(t, edgesOf(t, "d: &d {a: 1, b: 2}\ne: {a: 99, <<: *d}\n", "e"), "a=99", "b=2")
	// Otherwise the earliest alias wins.
	eq(t, edgesOf(t, "p: &p {a: 1}\nq: &q {a: 2}\nr: {<<: [*p, *q]}\n", "r"), "a=1")
	// One edge per key, at its first position: a=1 b=5 (from p) c=99 (local).
	text := "p: &p {a: 1, b: 5}\nq: &q {b: 2, c: 3}\nr: {<<: [*p, *q], c: 99}\n"
	eq(t, edgesOf(t, text, "r"), "a=1", "b=5", "c=99")
	// A repeated alias contributes once.
	eq(t, edgesOf(t, "p: &p {a: 1, b: 2}\nr: {<<: [*p, *p]}\n", "r"), "a=1", "b=2")
}

func TestMergeKeyNests(t *testing.T) {
	text := "g: &g {a: 1}\nm: &m {<<: *g, b: 2}\nn: {<<: *m, c: 3}\n"
	eq(t, edgesOf(t, text, "n"), "a=1", "b=2", "c=3")
}

func TestMergeKeyAcceptsInlineMappings(t *testing.T) {
	eq(t, edgesOf(t, "e: {<<: {a: 1}, b: 2}\n", "e"), "a=1", "b=2")
	eq(t, edgesOf(t, "e: {<<: [{a: 1}, {c: 3}], b: 2}\n", "e"), "a=1", "c=3", "b=2")
}

// A merged source's repeated-label list (a key whose value is a sequence)
// stays together: the earliest source supplying the label supplies every one
// of its edges, and a local write replaces them all.
func TestMergeKeyKeepsRepeatedLabelGroupsTogether(t *testing.T) {
	eq(t, edgesOf(t, "p: &p {t: [1, 2]}\nq: &q {t: [9]}\nr: {<<: [*p, *q]}\n", "r"), "t=1", "t=2")
	eq(t, edgesOf(t, "p: &p {t: [1, 2]}\nr: {<<: *p, t: 7}\n", "r"), "t=7")
}

// A quoted "<<" is an ordinary string key, not the merge key.
func TestQuotedMergeKeyIsAnOrdinaryLabel(t *testing.T) {
	eq(t, edgesOf(t, "e: {\"<<\": 1, a: 2}\n", "e"), "<<=1", "a=2")
}

func TestMergeKeyValueMustBeMappings(t *testing.T) {
	for _, text := range []string{
		"e: {<<: 1}\n",
		"e: {<<: [1, 2]}\n",
		"e: {<<: []}\n",
		"e: {<<: [[a]]}\n",
		"d: &d 5\ne: {<<: *d}\n",
	} {
		_, err := Read(text, omnist.DefaultLimits())
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeParseCodecSyntax {
			t.Errorf("%q: got %#v, want parse.codec-syntax", text, err)
		}
	}
}

// A merge that refers back to itself is bounded by the depth limit rather than
// recursing without end.
func TestSelfReferentialMergeIsBoundedByTheDepthLimit(t *testing.T) {
	_, err := Read("a: &a {<<: *a, y: 1}\n", omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("got %#v, want document.limit.depth", err)
	}
}

// A syntax error names the line yaml.v3 reports, with column 1.
func TestSyntaxErrorReportsTheLibrarysLine(t *testing.T) {
	_, err := Read("a: 1\nb: c: d\n", omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseCodecSyntax || pe.Line != 2 || pe.Path != "2:1" {
		t.Errorf("got %#v, want parse.codec-syntax at 2:1", err)
	}
	// An error text naming no line falls back to 1:1.
	pe2, _ := wrapYAMLDecodeErr(errNoLine{}).(*omnist.ParseError)
	if pe2 == nil || pe2.Path != "1:1" || pe2.Code != omnist.CodeParseCodecSyntax {
		t.Errorf("got %#v, want 1:1 parse.codec-syntax", pe2)
	}
}

type errNoLine struct{}

func (errNoLine) Error() string { return "yaml: something went wrong" }
