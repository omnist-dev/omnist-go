package xml

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- interleaving preservation: the highest-priority test in this issue ---

func TestReadXMLPreservesInterleaving(t *testing.T) {
	d, err := Read(`<root><m/><x/><m/></root>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	root := d.Node.Edges[0]
	node, ok := root.Target.Node()
	if !ok {
		t.Fatalf("expected root to be a node")
	}
	if len(node.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d: %+v", len(node.Edges), node.Edges)
	}
	gotLabels := []string{node.Edges[0].Label, node.Edges[1].Label, node.Edges[2].Label}
	wantLabels := []string{"m", "x", "m"}
	for i := range wantLabels {
		if gotLabels[i] != wantLabels[i] {
			t.Fatalf("edge %d: got label %q, want %q (full order: %v)", i, gotLabels[i], wantLabels[i], gotLabels)
		}
	}
}

func TestReadXMLInterleavingNotRegrouped(t *testing.T) {
	// A grouping reader (like WriteJSON's write side) would produce
	// [(m,[A,B]),(x,X)] — 2 edges. This must stay 3 edges in source order.
	d, err := Read(`<root><m>A</m><x>X</x><m>B</m></root>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	node, _ := d.Node.Edges[0].Target.Node()
	want := omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("x", omnist.ScalarValue(omnist.NewStringScalar("X"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B")))
	if !nodeEqual(node, want) {
		t.Errorf("got %+v, want %+v", node, want)
	}
}

// --- worked example (docs/formats/xml.md), stage-1 (untyped) output ---

func TestReadXMLWorkedExampleStage1Untyped(t *testing.T) {
	src := `<order>
  <id>A1</id>
  <status>shipped</status>
  <total>29.97</total>
  <address><street>1 Main</street><city>London</city></address>
  <items><sku>W</sku><qty>3</qty><price>9.99</price></items>
  <items><sku>G</sku><qty>1</qty><price>9.99</price></items>
</order>`
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	address := omnist.NewNode().
		AddValue("street", omnist.ScalarValue(omnist.NewStringScalar("1 Main"))).
		AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("London")))
	items1 := omnist.NewNode().
		AddValue("sku", omnist.ScalarValue(omnist.NewStringScalar("W"))).
		AddValue("qty", omnist.ScalarValue(omnist.NewStringScalar("3"))).
		AddValue("price", omnist.ScalarValue(omnist.NewStringScalar("9.99")))
	items2 := omnist.NewNode().
		AddValue("sku", omnist.ScalarValue(omnist.NewStringScalar("G"))).
		AddValue("qty", omnist.ScalarValue(omnist.NewStringScalar("1"))).
		AddValue("price", omnist.ScalarValue(omnist.NewStringScalar("9.99")))
	order := omnist.NewNode().
		AddValue("id", omnist.ScalarValue(omnist.NewStringScalar("A1"))).
		AddValue("status", omnist.ScalarValue(omnist.NewStringScalar("shipped"))).
		AddValue("total", omnist.ScalarValue(omnist.NewStringScalar("29.97"))). // stage 1: string, not number
		AddNode("address", address).
		AddNode("items", items1).
		AddNode("items", items2)
	want := omnist.NodeDocument(omnist.NewNode().AddNode("order", order))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
	// qty specifically must be the string "3"/"1", never an integer kind,
	// per "Stage 1 output differs here" (docs/formats/xml.md).
	qty := items1.Edges[1].Target
	v, _ := qty.Value()
	if v.Scalar.Kind != omnist.KindString || v.Scalar.Str != "3" {
		t.Fatalf("expected qty to be string \"3\" at stage 1, got %+v", v.Scalar)
	}
}

// --- repeated elements -> repeated labels, no wrapper ---

func TestReadXMLRepeatedElementsNoWrapper(t *testing.T) {
	d, err := Read(`<root><items>a</items><items>b</items><items>c</items></root>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	node, _ := d.Node.Edges[0].Target.Node()
	if len(node.Edges) != 3 {
		t.Fatalf("expected 3 edges labeled items, got %d", len(node.Edges))
	}
	for i, want := range []string{"a", "b", "c"} {
		if node.Edges[i].Label != "items" {
			t.Errorf("edge %d: got label %q, want items", i, node.Edges[i].Label)
		}
		v, _ := node.Edges[i].Target.Value()
		if v.Scalar.Str != want {
			t.Errorf("edge %d: got %q, want %q", i, v.Scalar.Str, want)
		}
	}
}

// --- attribute dropping: silent, no diagnostic ---

func TestReadXMLDropsAttributesSilently(t *testing.T) {
	d, err := Read(`<a x="1"><b>hi</b></a>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("expected a clean nil error, got %v", err)
	}
	b := omnist.NewNode().AddValue("b", omnist.ScalarValue(omnist.NewStringScalar("hi")))
	want := omnist.NodeDocument(omnist.NewNode().AddNode("a", b))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v (attribute must leave no trace)", d, want)
	}
}

func TestReadXMLDropsMultipleAttributesSilently(t *testing.T) {
	d, err := Read(`<a x="1" y="2" z="3"/>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("expected a clean nil error, got %v", err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewStringScalar(""))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- namespace-prefix dropping ---

func TestReadXMLDropsNamespacePrefix(t *testing.T) {
	d, err := Read(`<root xmlns:ns="http://example.com/ns"><ns:b>hi</ns:b></root>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	node, _ := d.Node.Edges[0].Target.Node()
	if len(node.Edges) != 1 || node.Edges[0].Label != "b" {
		t.Fatalf("expected a single edge labeled b (prefix dropped), got %+v", node.Edges)
	}
}

func TestReadXMLDropsUndeclaredNamespacePrefix(t *testing.T) {
	// A prefix with no matching xmlns declaration must still resolve to
	// the local name only.
	d, err := Read(`<ns:b>hi</ns:b>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if d.Node.Edges[0].Label != "b" {
		t.Fatalf("got label %q, want b", d.Node.Edges[0].Label)
	}
}

// --- text is always a string, zero auto-typing ---

func TestReadXMLTextAlwaysString(t *testing.T) {
	src := `<root><d>2024-01-15</d><i>42</i><b>true</b></root>`
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	node, _ := d.Node.Edges[0].Target.Node()
	for _, e := range node.Edges {
		v, ok := e.Target.Value()
		if !ok {
			t.Fatalf("edge %q: expected a value target", e.Label)
		}
		if v.Scalar.Kind != omnist.KindString {
			t.Errorf("edge %q: got kind %v, want omnist.KindString (zero auto-typing)", e.Label, v.Scalar.Kind)
		}
	}
}

// --- self-closing / empty leaf ---

func TestReadXMLSelfClosingElementIsEmptyString(t *testing.T) {
	d, err := Read(`<a/>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, ok := d.Node.Edges[0].Target.Value()
	if !ok || v.Scalar.Kind != omnist.KindString || v.Scalar.Str != "" {
		t.Fatalf("got %+v, want empty string leaf", d.Node.Edges[0].Target)
	}
}

// --- prolog / comments / whitespace are ignored ---

func TestReadXMLSkipsPrologAndComments(t *testing.T) {
	src := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- a comment -->\n<a>hi</a>\n"
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("hi"))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- multiple top-level elements are rejected on read (not well-formed) ---

func TestReadXMLRejectsMultipleTopLevelElements(t *testing.T) {
	_, err := Read(`<a/><b/>`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for multiple top-level elements")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("expected *omnist.ParseError, got %T: %v", err, err)
	}
	if pe.Code != omnist.CodeParseTrailingContent {
		t.Errorf("got code %v, want %v", pe.Code, omnist.CodeParseTrailingContent)
	}
}

func TestReadXMLRejectsTextBeforeRoot(t *testing.T) {
	_, err := Read(`stray text<a/>`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for text before the root element")
	}
}

func TestReadXMLRejectsStrayEndElementBeforeRoot(t *testing.T) {
	_, err := Read(`</a>`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a stray closing tag before any root element")
	}
}

func TestReadXMLSkipsCommentAndProcInstInsideElementBody(t *testing.T) {
	// A comment/processing instruction interleaved with actual child
	// elements must be ignored without disturbing sibling order.
	src := "<a><b/><!-- c --><?pi d?><e/></a>"
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	node, _ := d.Node.Edges[0].Target.Node()
	want := omnist.NewNode().
		AddValue("b", omnist.ScalarValue(omnist.NewStringScalar(""))).
		AddValue("e", omnist.ScalarValue(omnist.NewStringScalar("")))
	if !nodeEqual(node, want) {
		t.Errorf("got %+v, want %+v", node, want)
	}
}

func TestReadXMLSkipsCommentAndProcInstAfterRoot(t *testing.T) {
	src := "<a/>\n<!-- trailing comment -->\n<?pi data?>\n"
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewStringScalar(""))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestReadXMLRejectsMalformedContentAfterRoot(t *testing.T) {
	// Unterminated element after the root closes: a genuine decoder error,
	// not just a trailing-content shape violation, must surface from
	// checkTrailing too.
	_, err := Read(`<a/><b`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for malformed content after the root element")
	}
	if _, ok := err.(*omnist.ParseError); !ok {
		t.Fatalf("expected *omnist.ParseError, got %T: %v", err, err)
	}
}

func TestReadXMLRejectsTextAfterRoot(t *testing.T) {
	_, err := Read(`<a/>stray`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for text after the root element")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseTrailingContent {
		t.Fatalf("got %v (%T), want omnist.CodeParseTrailingContent", err, err)
	}
}

func TestReadXMLRejectsEmptyInput(t *testing.T) {
	_, err := Read(``, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for empty input")
	}
}

// --- limits ---

func TestReadXMLEnforcesMaxDepth(t *testing.T) {
	src := "<a><b><c><d>x</d></c></b></a>"
	limits := omnist.Limits{MaxDepth: 2, MaxNodes: 1_000_000, MaxIntDigits: 4300}
	_, err := Read(src, limits)
	if err == nil {
		t.Fatal("expected a depth-limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Fatalf("got %v (%T), want omnist.CodeDocumentLimitDepth", err, err)
	}
}

func TestReadXMLEnforcesMaxNodes(t *testing.T) {
	src := "<a><b/><c/><d/></a>"
	limits := omnist.Limits{MaxDepth: 200, MaxNodes: 2, MaxIntDigits: 4300}
	_, err := Read(src, limits)
	if err == nil {
		t.Fatal("expected a node-count-limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitNodes {
		t.Fatalf("got %v (%T), want omnist.CodeDocumentLimitNodes", err, err)
	}
}

// --- malformed XML surfaces a parse error, not a panic ---

func TestReadXMLMalformedInputReturnsParseError(t *testing.T) {
	_, err := Read(`<a><b></a>`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for mismatched tags")
	}
	if _, ok := err.(*omnist.ParseError); !ok {
		t.Fatalf("expected *omnist.ParseError, got %T: %v", err, err)
	}
}

func TestReadXMLUnterminatedElementReturnsParseError(t *testing.T) {
	_, err := Read(`<a><b>`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for unterminated element")
	}
	if _, ok := err.(*omnist.ParseError); !ok {
		t.Fatalf("expected *omnist.ParseError, got %T: %v", err, err)
	}
}

// --- mixed content: narrow/cosmetic, elements win over stray text ---

func TestReadXMLMixedContentDiscardsStrayText(t *testing.T) {
	d, err := Read(`<a>hello<b>x</b>world</a>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	node, ok := d.Node.Edges[0].Target.Node()
	if !ok {
		t.Fatalf("expected a as a node")
	}
	want := omnist.NewNode().AddValue("b", omnist.ScalarValue(omnist.NewStringScalar("x")))
	if !nodeEqual(node, want) {
		t.Errorf("got %+v, want %+v", node, want)
	}
}
