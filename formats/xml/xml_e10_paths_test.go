package xml

import (
	"reflect"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// E-10 (spec §8.4): the index [i] is present on EVERY edge of a label that
// occurs more than once in a node, the first included, and absent when the
// label occurs once. The XML reader's format.attribute-dropped and
// format.namespace-dropped diagnostics are at the element's own Document
// path, so they carry the index too (omnist-go issue #130).
func TestReadXMLDroppedPathsAreIndexed(t *testing.T) {
	type want struct {
		path string
		code omnist.Code
	}
	attr := omnist.CodeFormatAttributeDropped
	ns := omnist.CodeFormatNamespaceDropped
	tests := []struct {
		name string
		src  string
		want []want
	}{
		{"single occurrence has no index", `<root><item a="1"/><x/></root>`, []want{{"$.root.item", attr}}},
		{"repeated label indexes every occurrence", `<root><item a="1"/><item a="2"/></root>`,
			[]want{{"$.root.item[0]", attr}, {"$.root.item[1]", attr}}},
		{"only the second repeated element carries the attribute", `<root><item/><item a="2"/></root>`,
			[]want{{"$.root.item[1]", attr}}},
		{"first of an interleaved repeat is indexed once the repeat appears", `<root><item a="1"/><x/><item/></root>`,
			[]want{{"$.root.item[0]", attr}}},
		{"namespace prefix on a repeated label", `<root><ns:item/><ns:item/></root>`,
			[]want{{"$.root.item[0]", ns}, {"$.root.item[1]", ns}}},
		{"attribute then namespace on one element, in order", `<root><n:item a="1"/><n:item/></root>`,
			[]want{{"$.root.item[0]", attr}, {"$.root.item[0]", ns}, {"$.root.item[1]", ns}}},
		{"index is per node, applied at every level", `<root><g><t a="1"/><t/></g><g><t a="1"/></g></root>`,
			[]want{{"$.root.g[0].t[0]", attr}, {"$.root.g[1].t", attr}}},
		{"a repeated parent indexes its descendants", `<root><g><t a="1"/></g><g/></root>`,
			[]want{{"$.root.g[0].t", attr}}},
		{"root attribute is never indexed", `<root a="1"><x/></root>`, []want{{"$.root", attr}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, diags, err := Read(tc.src, omnist.DefaultLimits())
			if err != nil {
				t.Fatalf("Read(%q): %v", tc.src, err)
			}
			var got []want
			for _, d := range diags {
				got = append(got, want{d.Path, d.Code})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Read(%q) diagnostics = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

// ReadWithSchema takes the same path-building code; pin it with a schema so a
// future split of the two cannot regress it.
func TestReadWithSchemaDroppedPathsAreIndexed(t *testing.T) {
	_, diags, err := ReadWithSchema(`<root><item a="1"/><item a="2"/></root>`, nil, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 2 || diags[0].Path != "$.root.item[0]" || diags[1].Path != "$.root.item[1]" {
		t.Errorf("diagnostics = %+v", diags)
	}
}

// C-10 and E-10 (spec §7.3, §8.4): the writer's null-leaf failure is at the
// leaf's Document path, indexed when the label repeats (DIV-24).
func TestWriteXMLFailurePathsAreIndexed(t *testing.T) {
	str := func(s string) omnist.Value { return omnist.ScalarValue(omnist.NewStringScalar(s)) }
	tests := []struct {
		name string
		root func() *omnist.Node
		want string
	}{
		{"null in the second of three", func() *omnist.Node {
			return omnist.NewNode().AddValue("item", str("a")).AddValue("item", omnist.NullValue()).AddValue("item", str("c"))
		}, "$.root.item[1]"},
		{"null in the first of two", func() *omnist.Node {
			return omnist.NewNode().AddValue("item", omnist.NullValue()).AddValue("item", str("b"))
		}, "$.root.item[0]"},
		{"single null has no index", func() *omnist.Node {
			return omnist.NewNode().AddValue("item", omnist.NullValue()).AddValue("x", str("b"))
		}, "$.root.item"},
		{"null below a repeated parent", func() *omnist.Node {
			return omnist.NewNode().
				AddNode("g", omnist.NewNode().AddValue("a", str("1"))).
				AddNode("g", omnist.NewNode().AddValue("n", omnist.NullValue()))
		}, "$.root.g[1].n"},
		{"empty internal node in a repeated label", func() *omnist.Node {
			return omnist.NewNode().AddValue("item", str("a")).AddNode("item", omnist.NewNode())
		}, "$.root.item[1]"},
		{"control character in a repeated label", func() *omnist.Node {
			return omnist.NewNode().AddValue("item", str("a")).AddValue("item", str("\x01"))
		}, "$.root.item[1]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := omnist.NodeDocument(omnist.NewNode().AddNode("root", tc.root()))
			_, _, err := Write(d)
			diag, ok := err.(omnist.Diagnostic)
			if !ok {
				t.Fatalf("Write error = %v (%T), want a Diagnostic", err, err)
			}
			if diag.Code != omnist.CodeWriteUnsupportedValue || diag.Path != tc.want {
				t.Errorf("got %s at %q, want %s at %q", diag.Code, diag.Path, omnist.CodeWriteUnsupportedValue, tc.want)
			}
		})
	}
}
