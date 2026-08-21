package xml

import (
	encxml "encoding/xml"
	"io"
	"strings"
	"github.com/omnist-dev/omnist-go/osd"
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


// --- schema-aware pretyping tests (issue #81, omnist-spec#44) ---

func TestReadXMLWithSchemaWorkedExample(t *testing.T) {
	// Replicates vendor/omnist-spec/docs/formats/xml.md worked example.
	schema, err := osd.Read(`
record Address  { "street": string, "city": string }
record LineItem { "sku": string, "qty": integer, "price": number }

record Order {
    "id":           string,
    "status":       string,
    "total":        number,
    "address":      Address,
    "items" [1,]:   LineItem,
    "coupon" [0,1]: string,
}

record Root { "order": Order }
root Root
`)
	if err != nil {
		t.Fatalf("osd.Read failed: %v", err)
	}

	src := `<order>
  <id>A1</id>
  <status>shipped</status>
  <total>29.97</total>
  <address><street>1 Main</street><city>London</city></address>
  <items><sku>W</sku><qty>3</qty><price>9.99</price></items>
  <items><sku>G</sku><qty>1</qty><price>9.99</price></items>
</order>`

	doc, err := ReadWithSchema(src, &schema, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	diags := omnist.Validate(doc, schema)
	if len(diags) != 0 {
		t.Fatalf("validation failed: diags=%v err=%v", diags, err)
	}

	// Verify pre-typed leaves
	orderNode, ok := doc.Node.Edges[0].Target.Node()
	if !ok {
		t.Fatalf("expected order node")
	}

	// total should be KindNumber 29.97
	for _, e := range orderNode.Edges {
		if e.Label == "total" {
			v, _ := e.Target.Value()
			if v.Scalar.Kind != omnist.KindNumber || v.Scalar.Num != 29.97 {
				t.Errorf("total: got %+v, want 29.97 number", v.Scalar)
			}
		}
		if e.Label == "items" {
			itemNode, _ := e.Target.Node()
			for _, ie := range itemNode.Edges {
				if ie.Label == "qty" {
					v, _ := ie.Target.Value()
					if v.Scalar.Kind != omnist.KindInteger {
						t.Errorf("qty: got kind %v, want integer", v.Scalar.Kind)
					}
				}
				if ie.Label == "price" {
					v, _ := ie.Target.Value()
					if v.Scalar.Kind != omnist.KindNumber || v.Scalar.Num != 9.99 {
						t.Errorf("price: got %+v, want 9.99 number", v.Scalar)
					}
				}
			}
		}
	}
}

func TestReadXMLWithSchemaAllScalarKinds(t *testing.T) {
	schema := &omnist.Schema{
		Root: "R",
		Env: map[string]*omnist.Record{
			"R": {
				Name: "R",
				Fields: []omnist.Field{
					{Label: "b1", Type: omnist.ScalarType(omnist.KindBoolean, false)},
					{Label: "b2", Type: omnist.ScalarType(omnist.KindBoolean, false)},
					{Label: "i", Type: omnist.ScalarType(omnist.KindInteger, false)},
					{Label: "n", Type: omnist.ScalarType(omnist.KindNumber, false)},
					{Label: "d", Type: omnist.ScalarType(omnist.KindDate, false)},
					{Label: "t", Type: omnist.ScalarType(omnist.KindTime, false)},
					{Label: "dt", Type: omnist.ScalarType(omnist.KindDateTime, false)},
					{Label: "s", Type: omnist.ScalarType(omnist.KindString, false)},
				},
			},
		},
		EnvOrder: []string{"R"},
	}

	src := `<R>
  <b1>true</b1>
  <b2>false</b2>
  <i>12345</i>
  <n>3.1415</n>
  <d>2024-01-15</d>
  <t>12:30:00</t>
  <dt>2024-01-15T12:30:00</dt>
  <s>hello</s>
</R>`

	doc, err := ReadWithSchema(src, schema, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	node, _ := doc.Node.Edges[0].Target.Node()
	for _, e := range node.Edges {
		v, _ := e.Target.Value()
		switch e.Label {
		case "b1":
			if v.Scalar.Kind != omnist.KindBoolean || !v.Scalar.Bool {
				t.Errorf("b1: got %+v", v.Scalar)
			}
		case "b2":
			if v.Scalar.Kind != omnist.KindBoolean || v.Scalar.Bool {
				t.Errorf("b2: got %+v", v.Scalar)
			}
		case "i":
			if v.Scalar.Kind != omnist.KindInteger || v.Scalar.Int.Int64() != 12345 {
				t.Errorf("i: got %+v", v.Scalar)
			}
		case "n":
			if v.Scalar.Kind != omnist.KindNumber || v.Scalar.Num != 3.1415 {
				t.Errorf("n: got %+v", v.Scalar)
			}
		case "d":
			if v.Scalar.Kind != omnist.KindDate {
				t.Errorf("d: got %+v", v.Scalar)
			}
		case "t":
			if v.Scalar.Kind != omnist.KindTime {
				t.Errorf("t: got %+v", v.Scalar)
			}
		case "dt":
			if v.Scalar.Kind != omnist.KindDateTime {
				t.Errorf("dt: got %+v", v.Scalar)
			}
		case "s":
			if v.Scalar.Kind != omnist.KindString || v.Scalar.Str != "hello" {
				t.Errorf("s: got %+v", v.Scalar)
			}
		}
	}
}

func TestReadXMLWithSchemaInvalidLiteralsRemainStrings(t *testing.T) {
	schema := &omnist.Schema{
		Root: "R",
		Env: map[string]*omnist.Record{
			"R": {
				Name: "R",
				Fields: []omnist.Field{
					{Label: "b", Type: omnist.ScalarType(omnist.KindBoolean, false)},
					{Label: "i", Type: omnist.ScalarType(omnist.KindInteger, false)},
					{Label: "n", Type: omnist.ScalarType(omnist.KindNumber, false)},
					{Label: "d", Type: omnist.ScalarType(omnist.KindDate, false)},
					{Label: "t", Type: omnist.ScalarType(omnist.KindTime, false)},
					{Label: "dt", Type: omnist.ScalarType(omnist.KindDateTime, false)},
					{Label: "unknown", Type: omnist.Type{Kind: omnist.TypeKind(99)}},
				},
			},
		},
		EnvOrder: []string{"R"},
	}

	src := `<R>
  <b>not-bool</b>
  <i>not-int</i>
  <n>not-num</n>
  <d>not-date</d>
  <t>not-time</t>
  <dt>not-datetime</dt>
  <unknown>val</unknown>
</R>`

	doc, err := ReadWithSchema(src, schema, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	node, _ := doc.Node.Edges[0].Target.Node()
	for _, e := range node.Edges {
		v, _ := e.Target.Value()
		if v.Scalar.Kind != omnist.KindString {
			t.Errorf("field %s: expected KindString fallback for invalid literal, got %v", e.Label, v.Scalar.Kind)
		}
	}
}

func TestReadXMLWithSchemaRootElementIsLeaf(t *testing.T) {
	schema := &omnist.Schema{
		Root: "Root",
		Env: map[string]*omnist.Record{
			"Root": {
				Name: "Root",
				Fields: []omnist.Field{
					{Label: "count", Type: omnist.ScalarType(omnist.KindInteger, false)},
				},
			},
		},
		EnvOrder: []string{"Root"},
	}

	src := `<count>42</count>`
	doc, err := ReadWithSchema(src, schema, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := doc.Node.Edges[0].Target.Value()
	if !ok || v.Scalar.Kind != omnist.KindInteger || v.Scalar.Int.Int64() != 42 {
		t.Fatalf("got %+v, want integer 42", doc.Node.Edges[0].Target)
	}
}


func TestReadXMLWithSchemaRootFallback(t *testing.T) {
	schema := &omnist.Schema{
		Root: "Other",
		Env: map[string]*omnist.Record{
			"Other": {
				Name: "Other",
				Fields: []omnist.Field{
					{Label: "foo", Type: omnist.ScalarType(omnist.KindString, false)},
				},
			},
			"Target": {
				Name: "Target",
				Fields: []omnist.Field{
					{Label: "num", Type: omnist.ScalarType(omnist.KindInteger, false)},
				},
			},
		},
		EnvOrder: []string{"Other", "Target"},
	}

	src := `<Target><num>100</num></Target>`
	doc, err := ReadWithSchema(src, schema, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	targetNode, _ := doc.Node.Edges[0].Target.Node()
	v, _ := targetNode.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindInteger || v.Scalar.Int.Int64() != 100 {
		t.Errorf("got %+v, want 100 integer", v.Scalar)
	}

	// Unresolved root record fallback
	schemaNoRoot := &omnist.Schema{
		Root: "MissingRoot",
		Env: map[string]*omnist.Record{
			"Target": {
				Name: "Target",
				Fields: []omnist.Field{
					{Label: "num", Type: omnist.ScalarType(omnist.KindInteger, false)},
				},
			},
		},
		EnvOrder: []string{"Target"},
	}
	doc2, err := ReadWithSchema(src, schemaNoRoot, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	targetNode2, _ := doc2.Node.Edges[0].Target.Node()
	v2, _ := targetNode2.Edges[0].Target.Value()
	if v2.Scalar.Kind != omnist.KindInteger || v2.Scalar.Int.Int64() != 100 {
		t.Errorf("got %+v, want 100 integer", v2.Scalar)
	}
}

func TestPretypeScalarDefault(t *testing.T) {
	_, ok := pretypeScalar("foo", omnist.ScalarKind(999))
	if ok {
		t.Errorf("expected false for unknown scalar kind")
	}
}

func TestReadXMLUnexpectedEOFInsideBody(t *testing.T) {
	_, err := Read("<root><child>", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for unterminated tag")
	}
}

func TestWrapDecodeErrEOF(t *testing.T) {
	r := &xmlReader{
		dec: encxml.NewDecoder(strings.NewReader("")),
	}
	err := r.wrapDecodeErr(io.EOF)
	if err == nil || !strings.Contains(err.Error(), "unexpected end of input") {
		t.Errorf("got %v, want unexpected end of input", err)
	}
}
