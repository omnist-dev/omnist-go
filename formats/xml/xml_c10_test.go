package xml

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// C-10 (spec §7.3, formats/xml.md): the XML writer MUST fail with
// write.unsupported-value, unconditionally, at the Document path of a null
// leaf (indexed per E-10), and MUST NOT write an empty element in its place:
// the XML read rule makes a childless element a string leaf, so `<a/>` is the
// empty string, a different Document. The five vectors formats-xml/nulls/*
// pin the same cases through the conformance runner; this pins them (and the
// writer's refusal to emit anything) for this package.
func TestWriteNullLeafFailsAtItsIndexedPath(t *testing.T) {
	str := func(s string) omnist.Value { return omnist.ScalarValue(omnist.NewStringScalar(s)) }
	tests := []struct {
		name string
		doc  omnist.Document
		path string
	}{
		{"top level", omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.NullValue())), "$.a"},
		{"nested", omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().
			AddNode("x", omnist.NewNode().AddValue("n", omnist.NullValue())))), "$.root.x.n"},
		{"second of three repeated", omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().
			AddValue("item", str("a")).AddValue("item", omnist.NullValue()).AddValue("item", str("c")))), "$.root.item[1]"},
		{"first of two repeated", omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().
			AddValue("item", omnist.NullValue()).AddValue("item", str("b")))), "$.root.item[0]"},
		{"a null beside a string is still a failure", omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().
			AddValue("s", str("")).AddValue("n", omnist.NullValue()))), "$.root.n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, diags, err := Write(tc.doc)
			d, ok := err.(omnist.Diagnostic)
			if !ok || d.Code != omnist.CodeWriteUnsupportedValue || d.Path != tc.path {
				t.Fatalf("got err %v, want write.unsupported-value at %s", err, tc.path)
			}
			if out != "" || len(diags) != 0 {
				t.Errorf("a failed write returned text %q and diagnostics %v; a null must never be written as an empty element or reported as an adjustment", out, diags)
			}
		})
	}
}

// The neighbours: an empty string leaf is not a null leaf and still writes, and
// the read side reads a childless element back as that empty string, which is
// why a written null would have been indistinguishable from it.
func TestEmptyStringLeafWritesAndReadsBackAsTheEmptyString(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().
		AddValue("s", omnist.ScalarValue(omnist.NewStringScalar("")))))
	out, _, err := Write(d)
	if err != nil {
		t.Fatalf("an empty string leaf must be writable: %v", err)
	}
	back, _, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("Read(%q): %v", out, err)
	}
	if !docEqual(back, d) {
		t.Errorf("Read(Write(d)) = %#v, want the empty string leaf", back)
	}
	for _, src := range []string{"<root><s/></root>", "<root><s></s></root>"} {
		got, _, err := Read(src, omnist.DefaultLimits())
		if err != nil || !docEqual(got, d) {
			t.Errorf("Read(%q) = %#v, %v; want the empty string leaf", src, got, err)
		}
	}
}
