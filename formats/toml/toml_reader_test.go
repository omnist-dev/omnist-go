package toml

import (
	"math"
	"math/big"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- model mapping table (docs/formats/toml.md) ---

func TestTOMLModelMappingTableSimpleKeys(t *testing.T) {
	d, err := Read("a = 1\nb = 2\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().
		AddValue("a", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))).
		AddValue("b", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(2)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLArrayOfTablesRepeatedLabel(t *testing.T) {
	src := "[[x]]\nname = \"a\"\n[[x]]\nname = \"b\"\n"
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	x1 := omnist.NewNode().AddValue("name", omnist.ScalarValue(omnist.NewStringScalar("a")))
	x2 := omnist.NewNode().AddValue("name", omnist.ScalarValue(omnist.NewStringScalar("b")))
	want := omnist.NodeDocument(omnist.NewNode().AddNode("x", x1).AddNode("x", x2))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
	// Confirm this is genuinely 2 distinct edges sharing a label, not a
	// list collapsed onto one edge.
	if len(d.Node.Edges) != 2 || d.Node.Edges[0].Label != "x" || d.Node.Edges[1].Label != "x" {
		t.Fatalf("expected 2 edges labeled x, got %+v", d.Node.Edges)
	}
}

func TestTOMLSingleTableIsOneEdge(t *testing.T) {
	src := "[x]\nname = \"c\"\n"
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Node.Edges) != 1 || d.Node.Edges[0].Label != "x" {
		t.Fatalf("expected exactly one edge labeled x, got %+v", d.Node.Edges)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddNode("x", omnist.NewNode().AddValue("name", omnist.ScalarValue(omnist.NewStringScalar("c")))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- worked example (docs/formats/toml.md) ---

func TestTOMLWorkedExample(t *testing.T) {
	src := `[order]
id = "A1"
status = "shipped"
total = 29.97

[order.address]
street = "1 Main"
city = "London"

[[order.items]]
sku = "W"
qty = 3
price = 9.99

[[order.items]]
sku = "G"
qty = 1
price = 9.99
`
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}

	address := omnist.NewNode().AddValue("street", omnist.ScalarValue(omnist.NewStringScalar("1 Main"))).
		AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("London")))
	item1 := omnist.NewNode().AddValue("sku", omnist.ScalarValue(omnist.NewStringScalar("W"))).
		AddValue("qty", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(3)))).
		AddValue("price", omnist.ScalarValue(omnist.NewNumberScalar(9.99)))
	item2 := omnist.NewNode().AddValue("sku", omnist.ScalarValue(omnist.NewStringScalar("G"))).
		AddValue("qty", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))).
		AddValue("price", omnist.ScalarValue(omnist.NewNumberScalar(9.99)))
	order := omnist.NewNode().
		AddValue("id", omnist.ScalarValue(omnist.NewStringScalar("A1"))).
		AddValue("status", omnist.ScalarValue(omnist.NewStringScalar("shipped"))).
		AddValue("total", omnist.ScalarValue(omnist.NewNumberScalar(29.97))).
		AddNode("address", address).
		AddNode("items", item1).
		AddNode("items", item2)
	want := omnist.NodeDocument(omnist.NewNode().AddNode("order", order))

	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- native temporal literals ---

func TestTOMLNativeDate(t *testing.T) {
	d, err := Read("d = 2024-01-01\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("d", omnist.ScalarValue(omnist.NewDateScalar(omnist.DateValue{Year: 2024, Month: 1, Day: 1}))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLNativeTime(t *testing.T) {
	d, err := Read("t = 12:30:45\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("t", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 12, Minute: 30, Second: 45}))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLNativeTimeWithFraction(t *testing.T) {
	d, err := Read("t = 12:30:45.5\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	got := d.Node.Edges[0].Target
	v, _ := got.Value()
	if v.Scalar.Kind != omnist.KindTime || v.Scalar.Time.Nanosecond != 500000000 {
		t.Errorf("got %+v, want a time with 500000000ns", v)
	}
}

func TestTOMLNativeLocalDateTime(t *testing.T) {
	d, err := Read("dt = 2024-01-01T12:00:00\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("dt", omnist.ScalarValue(omnist.NewDateTimeScalar(omnist.DateTimeValue{
		Date: omnist.DateValue{Year: 2024, Month: 1, Day: 1},
		Time: omnist.TimeValue{Hour: 12},
	}))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLNativeOffsetDateTimeZ(t *testing.T) {
	d, err := Read("dt = 2024-01-01T12:00:00Z\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindDateTime {
		t.Fatalf("kind = %v, want omnist.KindDateTime", v.Scalar.Kind)
	}
	if !v.Scalar.DateTime.Time.HasOffset || v.Scalar.DateTime.Time.OffsetSeconds != 0 {
		t.Errorf("got %+v, want HasOffset=true OffsetSeconds=0", v.Scalar.DateTime.Time)
	}
}

func TestTOMLNativeOffsetDateTimeLowercaseZ(t *testing.T) {
	d, err := Read("dt = 2024-01-01 12:00:00z\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if !v.Scalar.DateTime.Time.HasOffset {
		t.Errorf("got %+v, want HasOffset=true", v.Scalar.DateTime.Time)
	}
}

func TestTOMLNativeOffsetDateTimeWithOffset(t *testing.T) {
	d, err := Read("dt = 2024-01-01T12:00:00.123+05:30\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	tv := v.Scalar.DateTime.Time
	if !tv.HasOffset || tv.OffsetSeconds != 5*3600+30*60 {
		t.Errorf("got %+v, want offset +05:30", tv)
	}
	if tv.Nanosecond != 123000000 {
		t.Errorf("got nanosecond %d, want 123000000", tv.Nanosecond)
	}
}

func TestTOMLNativeDateTimeSpaceSeparator(t *testing.T) {
	d, err := Read("dt = 2024-01-01 12:00:00\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindDateTime || v.Scalar.DateTime.Time.HasOffset {
		t.Errorf("got %+v, want a local datetime with no offset", v)
	}
}

// --- integer/float distinction ---

func TestTOMLIntegerFloatDistinctionByShape(t *testing.T) {
	d, err := Read("i = 2\nf = 2.0\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().
		AddValue("i", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(2)))).
		AddValue("f", omnist.ScalarValue(omnist.NewNumberScalar(2.0))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLIntegerRadixForms(t *testing.T) {
	d, err := Read("hex = 0xFF\noct = 0o17\nbin = 0b101\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().
		AddValue("hex", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(255)))).
		AddValue("oct", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(15)))).
		AddValue("bin", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(5)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLIntegerArbitraryPrecision(t *testing.T) {
	big53 := "99999999999999999999999999999999999999999999999999"
	d, err := Read("n = "+big53+"\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindInteger || v.Scalar.Int.String() != big53 {
		t.Errorf("got %+v, want integer %s", v, big53)
	}
}

func TestTOMLIntegerUnderscoreSeparators(t *testing.T) {
	d, err := Read("n = 1_000_000\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("n", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1000000)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLIntegerNegative(t *testing.T) {
	d, err := Read("n = -42\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("n", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(-42)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- floats, including special values ---

func TestTOMLFloatSpecialValues(t *testing.T) {
	d, err := Read("a = inf\nb = +inf\nc = -inf\nd = nan\ne = -nan\nf = +nan\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	get := func(label string) float64 {
		for _, e := range d.Node.Edges {
			if e.Label == label {
				v, _ := e.Target.Value()
				return v.Scalar.Num
			}
		}
		t.Fatalf("no edge %s", label)
		return 0
	}
	if !math.IsInf(get("a"), 1) || !math.IsInf(get("b"), 1) || !math.IsInf(get("c"), -1) {
		t.Errorf("inf spellings did not resolve to Inf/-Inf")
	}
	if !math.IsNaN(get("d")) || !math.IsNaN(get("e")) || !math.IsNaN(get("f")) {
		t.Errorf("nan spellings (including signed) did not resolve to NaN")
	}
}

func TestTOMLFloatUnderscoreAndExponent(t *testing.T) {
	d, err := Read("n = 1_234.5e1_0\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	want := 1234.5e10
	if v.Scalar.Kind != omnist.KindNumber || v.Scalar.Num != want {
		t.Errorf("got %+v, want %v", v, want)
	}
}

// --- inline tables and arrays ---

func TestTOMLInlineTable(t *testing.T) {
	d, err := Read(`p = {x = 1, y = 2}`+"\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddNode("p", omnist.NewNode().
		AddValue("x", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))).
		AddValue("y", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(2))))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLInlineArrayRepeatedLabel(t *testing.T) {
	d, err := Read("nums = [1, 2, 3]\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().
		AddValue("nums", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))).
		AddValue("nums", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(2)))).
		AddValue("nums", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(3)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLInlineArrayOfInlineTables(t *testing.T) {
	d, err := Read(`items = [{n = "a"}, {n = "b"}]`+"\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().
		AddNode("items", omnist.NewNode().AddValue("n", omnist.ScalarValue(omnist.NewStringScalar("a")))).
		AddNode("items", omnist.NewNode().AddValue("n", omnist.ScalarValue(omnist.NewStringScalar("b")))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLEmptyInlineArrayIsRejected(t *testing.T) {
	_, err := Read("a = []\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for an empty array")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseEmptyArray {
		t.Errorf("error = %#v, want omnist.CodeParseEmptyArray", err)
	}
}

func TestTOMLNestedArrayIsRejected(t *testing.T) {
	_, err := Read("a = [[1, 2], [3, 4]]\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a nested bare array")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want omnist.CodeDocumentUnlabeledElement", err)
	}
}

// --- dotted keys ---

func TestTOMLDottedKeyCreatesImplicitTable(t *testing.T) {
	d, err := Read("host.name = \"x\"\nhost.port = 1\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Node.Edges) != 1 || d.Node.Edges[0].Label != "host" {
		t.Fatalf("expected a single host edge, got %+v", d.Node.Edges)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddNode("host", omnist.NewNode().
		AddValue("name", omnist.ScalarValue(omnist.NewStringScalar("x"))).
		AddValue("port", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1))))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLDottedKeyDeepNesting(t *testing.T) {
	d, err := Read("a.b.c = 5\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddNode("a", omnist.NewNode().AddNode("b", omnist.NewNode().
		AddValue("c", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(5)))))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLQuotedKey(t *testing.T) {
	d, err := Read(`"q key" = 9` + "\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("q key", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(9)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// TestTOMLDottedKeyOverALabelAlreadyUsedForAScalar exercises
// navigateOrCreate's "existing edge with a matching label but a scalar
// (non-node) target" path: this reader does not independently enforce
// TOML's own "cannot redefine an already-defined key" validity rule (see
// navigateOrCreate's doc comment) — a dotted key that collides with an
// earlier plain key of the same label falls through to allocating a
// fresh node rather than reusing the scalar edge (which it structurally
// cannot reuse: a scalar has no edges to extend). This is exactly the
// narrow, documented gap, not a crash or silent data loss.
func TestTOMLDottedKeyOverALabelAlreadyUsedForAScalar(t *testing.T) {
	d, err := Read("a = 1\na.b = 2\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Node.Edges) != 2 {
		t.Fatalf("expected 2 edges labeled a, got %+v", d.Node.Edges)
	}
	v, ok := d.Node.Edges[0].Target.Value()
	if !ok || v.Scalar.Kind != omnist.KindInteger {
		t.Errorf("first a edge = %+v, want the original scalar", d.Node.Edges[0])
	}
	child, ok := d.Node.Edges[1].Target.Node()
	if !ok || len(child.Edges) != 1 || child.Edges[0].Label != "b" {
		t.Errorf("second a edge = %+v, want a fresh node holding b", d.Node.Edges[1])
	}
}

func TestTOMLNestedErrorInsideInlineTableInsideArray(t *testing.T) {
	_, err := Read("items = [{a = []}]\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for the empty array nested inside the inline table")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseEmptyArray {
		t.Errorf("error = %#v, want omnist.CodeParseEmptyArray", err)
	}
}

// --- ArrayTable path re-entry (official TOML fruits example) ---

func TestTOMLArrayTableDottedPathReentry(t *testing.T) {
	src := `[[fruits]]
name = "apple"

[fruits.physical]
color = "red"

[[fruits.varieties]]
name = "red delicious"

[[fruits]]
name = "banana"

[[fruits.varieties]]
name = "plantain"
`
	d, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Node.Edges) != 2 {
		t.Fatalf("expected 2 top-level fruits edges, got %d", len(d.Node.Edges))
	}
	apple, _ := d.Node.Edges[0].Target.Node()
	banana, _ := d.Node.Edges[1].Target.Node()

	// apple: name, physical, varieties(1)
	if len(apple.Edges) != 3 {
		t.Fatalf("apple edges = %+v, want 3", apple.Edges)
	}
	if apple.Edges[1].Label != "physical" {
		t.Errorf("apple.Edges[1] = %+v, want physical", apple.Edges[1])
	}
	if apple.Edges[2].Label != "varieties" {
		t.Errorf("apple.Edges[2] = %+v, want varieties", apple.Edges[2])
	}

	// banana: name, varieties(1) — the [fruits.physical] and first
	// [[fruits.varieties]] from the apple instance must not leak in.
	if len(banana.Edges) != 2 {
		t.Fatalf("banana edges = %+v, want 2", banana.Edges)
	}
	if banana.Edges[1].Label != "varieties" {
		t.Errorf("banana.Edges[1] = %+v, want varieties", banana.Edges[1])
	}
}

// --- empty document ---

func TestTOMLEmptyDocument(t *testing.T) {
	d, err := Read("", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsNode || len(d.Node.Edges) != 0 {
		t.Errorf("got %+v, want an empty node document", d)
	}
}

// --- syntax errors ---

func TestTOMLSyntaxErrorReported(t *testing.T) {
	_, err := Read("a = -0xFF\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected a parse error for a signed radix literal")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseUnexpectedToken {
		t.Errorf("error = %#v, want omnist.CodeParseUnexpectedToken", err)
	}
	if !strings.Contains(pe.Message, "radix") {
		t.Errorf("message = %q, want it to mention the radix-prefix rule", pe.Message)
	}
}

// --- limits ---

func TestTOMLLimitDepth(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 2
	_, err := Read("a.b.c.d = 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitDepth", err)
	}
}

func TestTOMLLimitDepthViaTableHeader(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 1
	_, err := Read("[a.b]\nc = 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitDepth", err)
	}
}

func TestTOMLLimitNodes(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxNodes = 2
	_, err := Read("a = {}\nb = {}\nc = {}\n", limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitNodes {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitNodes", err)
	}
}

func TestTOMLLimitDepthViaArrayTableParentPath(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 1
	_, err := Read("[[a.b.c]]\nx = 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitDepth", err)
	}
}

func TestTOMLLimitNodesViaArrayTable(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxNodes = 1
	_, err := Read("[[a]]\nx = 1\n[[a]]\ny = 2\n", limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitNodes {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitNodes", err)
	}
}

func TestTOMLLimitIntDigitsInsideArray(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read("n = [12345]\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

func TestTOMLLimitIntDigits(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read("n = 12345\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

// --- string escaping ---

func TestTOMLBasicStringEscapes(t *testing.T) {
	d, err := Read(`s = "a\nb\tc\"d"` + "\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	want := "a\nb\tc\"d"
	if v.Scalar.Str != want {
		t.Errorf("got %q, want %q", v.Scalar.Str, want)
	}
}

func TestTOMLMultilineString(t *testing.T) {
	d, err := Read("s = \"\"\"hi\nthere\"\"\"\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Str != "hi\nthere" {
		t.Errorf("got %q, want %q", v.Scalar.Str, "hi\nthere")
	}
}

// --- booleans ---

func TestTOMLBooleans(t *testing.T) {
	d, err := Read("t = true\nf = false\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().
		AddValue("t", omnist.ScalarValue(omnist.NewBooleanScalar(true))).
		AddValue("f", omnist.ScalarValue(omnist.NewBooleanScalar(false))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- malformed temporal literals (issue #57: found by fuzzing) ---
//
// go-toml/v2's unstable parser can tag a value node Kind=LocalDate/
// LocalTime/LocalDateTime/DateTime on text that does not actually have
// the full digit layout omnist.ParseISODate/omnist.ParseISOTime require
// (their documented precondition) -- e.g. "00:" reaches Kind=LocalTime
// even though it is not a complete time literal. These confirm the
// reader now reports a *omnist.ParseError instead of panicking, for
// every one of readScalar's and parseTOMLDateTime's new validation
// branches.

func TestTOMLMalformedLocalDateDoesNotPanic(t *testing.T) {
	// go-toml tags this Kind=LocalDate with Data "2024-01-0" (one digit
	// short of a complete day), confirmed empirically against the
	// library's own unstable.Parser.
	_, err := Read("d = 2024-01-0\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected a parse error for a malformed date literal, got success")
	}
	if _, ok := err.(*omnist.ParseError); !ok {
		t.Errorf("error = %#v (%T), want *omnist.ParseError", err, err)
	}
}

func TestTOMLMalformedLocalTimeDoesNotPanic(t *testing.T) {
	// The exact input the fuzzer found (issue #57): go-toml tags this
	// Kind=LocalTime with Data "00:", too short for ParseISOTime's
	// precondition.
	_, err := Read("d = 00:\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected a parse error for a malformed time literal, got success")
	}
	if _, ok := err.(*omnist.ParseError); !ok {
		t.Errorf("error = %#v (%T), want *omnist.ParseError", err, err)
	}
}

func TestTOMLMalformedLocalDateTimeDoesNotPanic(t *testing.T) {
	// The exact input the fuzzer found for the LocalDateTime path (issue
	// #57): go-toml tags this Kind=LocalDateTime with Data
	// "2024-01-01T", an empty time portion.
	_, err := Read("d = 2024-01-01T\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected a parse error for a malformed datetime literal, got success")
	}
	if _, ok := err.(*omnist.ParseError); !ok {
		t.Errorf("error = %#v (%T), want *omnist.ParseError", err, err)
	}
}

func TestTOMLMalformedDateTimeWithBadOffsetSuffixDoesNotPanic(t *testing.T) {
	// go-toml tags this Kind=DateTime with Data "2024-01-01Tz": the
	// offset marker is present but the time body before it is empty.
	// Exercises parseTOMLDateTime's Z/z-suffix branch with an invalid
	// body, distinct from the plain-suffix branch above.
	_, err := Read("d = 2024-01-01Tz\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected a parse error, got success")
	}
	if _, ok := err.(*omnist.ParseError); !ok {
		t.Errorf("error = %#v (%T), want *omnist.ParseError", err, err)
	}
}

// TestParseTOMLDateTimeDirect white-box tests parseTOMLDateTime's
// validation branches directly, including two (the too-short-length
// guard and the invalid-separator guard) that -- as far as empirical
// probing of go-toml/v2's unstable.Parser found -- it never actually
// hands a LocalDateTime/DateTime node malformed enough to reach: TOML's
// own tokenizer rejects a bad separator and never tags anything under 11
// bytes with either kind. They stay as defense-in-depth against a
// different or future go-toml version behaving more like the "00:"/
// "2024-01-01T" cases above, and are exercised here directly since they
// are otherwise unreachable through Read.
func TestParseTOMLDateTimeDirect(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"too short", "2024-01-01", false},
		{"bad separator", "2024-01-01X00:00:00", false},
		{"bad date part", "202X-01-01T00:00:00", false},
		{"bad time part", "2024-01-01T00:00:0", false},
		{"bad Z body", "2024-01-01Tz", false},
		{"valid space-separated", "2024-01-01 00:00:00", true},
		{"valid T-separated with Z", "2024-01-01T00:00:00Z", true},
		{"valid t-separated", "2024-01-01t00:00:00", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := parseTOMLDateTime(c.in)
			if ok != c.ok {
				t.Errorf("parseTOMLDateTime(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
		})
	}
}
