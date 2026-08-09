package omnist

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

// --- model mapping table (docs/formats/toml.md) ---

func TestTOMLModelMappingTableSimpleKeys(t *testing.T) {
	d, err := ReadTOML("a = 1\nb = 2\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().
		AddValue("a", ScalarValue(NewIntegerScalar(big.NewInt(1)))).
		AddValue("b", ScalarValue(NewIntegerScalar(big.NewInt(2)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLArrayOfTablesRepeatedLabel(t *testing.T) {
	src := "[[x]]\nname = \"a\"\n[[x]]\nname = \"b\"\n"
	d, err := ReadTOML(src, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	x1 := NewNode().AddValue("name", ScalarValue(NewStringScalar("a")))
	x2 := NewNode().AddValue("name", ScalarValue(NewStringScalar("b")))
	want := NodeDocument(NewNode().AddNode("x", x1).AddNode("x", x2))
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
	d, err := ReadTOML(src, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Node.Edges) != 1 || d.Node.Edges[0].Label != "x" {
		t.Fatalf("expected exactly one edge labeled x, got %+v", d.Node.Edges)
	}
	want := NodeDocument(NewNode().AddNode("x", NewNode().AddValue("name", ScalarValue(NewStringScalar("c")))))
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
	d, err := ReadTOML(src, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}

	address := NewNode().AddValue("street", ScalarValue(NewStringScalar("1 Main"))).
		AddValue("city", ScalarValue(NewStringScalar("London")))
	item1 := NewNode().AddValue("sku", ScalarValue(NewStringScalar("W"))).
		AddValue("qty", ScalarValue(NewIntegerScalar(big.NewInt(3)))).
		AddValue("price", ScalarValue(NewNumberScalar(9.99)))
	item2 := NewNode().AddValue("sku", ScalarValue(NewStringScalar("G"))).
		AddValue("qty", ScalarValue(NewIntegerScalar(big.NewInt(1)))).
		AddValue("price", ScalarValue(NewNumberScalar(9.99)))
	order := NewNode().
		AddValue("id", ScalarValue(NewStringScalar("A1"))).
		AddValue("status", ScalarValue(NewStringScalar("shipped"))).
		AddValue("total", ScalarValue(NewNumberScalar(29.97))).
		AddNode("address", address).
		AddNode("items", item1).
		AddNode("items", item2)
	want := NodeDocument(NewNode().AddNode("order", order))

	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- native temporal literals ---

func TestTOMLNativeDate(t *testing.T) {
	d, err := ReadTOML("d = 2024-01-01\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddValue("d", ScalarValue(NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 1}))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLNativeTime(t *testing.T) {
	d, err := ReadTOML("t = 12:30:45\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddValue("t", ScalarValue(NewTimeScalar(TimeValue{Hour: 12, Minute: 30, Second: 45}))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLNativeTimeWithFraction(t *testing.T) {
	d, err := ReadTOML("t = 12:30:45.5\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	got := d.Node.Edges[0].Target
	v, _ := got.Value()
	if v.Scalar.Kind != KindTime || v.Scalar.Time.Nanosecond != 500000000 {
		t.Errorf("got %+v, want a time with 500000000ns", v)
	}
}

func TestTOMLNativeLocalDateTime(t *testing.T) {
	d, err := ReadTOML("dt = 2024-01-01T12:00:00\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddValue("dt", ScalarValue(NewDateTimeScalar(DateTimeValue{
		Date: DateValue{Year: 2024, Month: 1, Day: 1},
		Time: TimeValue{Hour: 12},
	}))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLNativeOffsetDateTimeZ(t *testing.T) {
	d, err := ReadTOML("dt = 2024-01-01T12:00:00Z\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindDateTime {
		t.Fatalf("kind = %v, want KindDateTime", v.Scalar.Kind)
	}
	if !v.Scalar.DateTime.Time.HasOffset || v.Scalar.DateTime.Time.OffsetSeconds != 0 {
		t.Errorf("got %+v, want HasOffset=true OffsetSeconds=0", v.Scalar.DateTime.Time)
	}
}

func TestTOMLNativeOffsetDateTimeLowercaseZ(t *testing.T) {
	d, err := ReadTOML("dt = 2024-01-01 12:00:00z\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if !v.Scalar.DateTime.Time.HasOffset {
		t.Errorf("got %+v, want HasOffset=true", v.Scalar.DateTime.Time)
	}
}

func TestTOMLNativeOffsetDateTimeWithOffset(t *testing.T) {
	d, err := ReadTOML("dt = 2024-01-01T12:00:00.123+05:30\n", DefaultLimits())
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
	d, err := ReadTOML("dt = 2024-01-01 12:00:00\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindDateTime || v.Scalar.DateTime.Time.HasOffset {
		t.Errorf("got %+v, want a local datetime with no offset", v)
	}
}

// --- integer/float distinction ---

func TestTOMLIntegerFloatDistinctionByShape(t *testing.T) {
	d, err := ReadTOML("i = 2\nf = 2.0\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().
		AddValue("i", ScalarValue(NewIntegerScalar(big.NewInt(2)))).
		AddValue("f", ScalarValue(NewNumberScalar(2.0))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLIntegerRadixForms(t *testing.T) {
	d, err := ReadTOML("hex = 0xFF\noct = 0o17\nbin = 0b101\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().
		AddValue("hex", ScalarValue(NewIntegerScalar(big.NewInt(255)))).
		AddValue("oct", ScalarValue(NewIntegerScalar(big.NewInt(15)))).
		AddValue("bin", ScalarValue(NewIntegerScalar(big.NewInt(5)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLIntegerArbitraryPrecision(t *testing.T) {
	big53 := "99999999999999999999999999999999999999999999999999"
	d, err := ReadTOML("n = "+big53+"\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindInteger || v.Scalar.Int.String() != big53 {
		t.Errorf("got %+v, want integer %s", v, big53)
	}
}

func TestTOMLIntegerUnderscoreSeparators(t *testing.T) {
	d, err := ReadTOML("n = 1_000_000\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddValue("n", ScalarValue(NewIntegerScalar(big.NewInt(1000000)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLIntegerNegative(t *testing.T) {
	d, err := ReadTOML("n = -42\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddValue("n", ScalarValue(NewIntegerScalar(big.NewInt(-42)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- floats, including special values ---

func TestTOMLFloatSpecialValues(t *testing.T) {
	d, err := ReadTOML("a = inf\nb = +inf\nc = -inf\nd = nan\ne = -nan\nf = +nan\n", DefaultLimits())
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
	d, err := ReadTOML("n = 1_234.5e1_0\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	want := 1234.5e10
	if v.Scalar.Kind != KindNumber || v.Scalar.Num != want {
		t.Errorf("got %+v, want %v", v, want)
	}
}

// --- inline tables and arrays ---

func TestTOMLInlineTable(t *testing.T) {
	d, err := ReadTOML(`p = {x = 1, y = 2}`+"\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddNode("p", NewNode().
		AddValue("x", ScalarValue(NewIntegerScalar(big.NewInt(1)))).
		AddValue("y", ScalarValue(NewIntegerScalar(big.NewInt(2))))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLInlineArrayRepeatedLabel(t *testing.T) {
	d, err := ReadTOML("nums = [1, 2, 3]\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().
		AddValue("nums", ScalarValue(NewIntegerScalar(big.NewInt(1)))).
		AddValue("nums", ScalarValue(NewIntegerScalar(big.NewInt(2)))).
		AddValue("nums", ScalarValue(NewIntegerScalar(big.NewInt(3)))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLInlineArrayOfInlineTables(t *testing.T) {
	d, err := ReadTOML(`items = [{n = "a"}, {n = "b"}]`+"\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().
		AddNode("items", NewNode().AddValue("n", ScalarValue(NewStringScalar("a")))).
		AddNode("items", NewNode().AddValue("n", ScalarValue(NewStringScalar("b")))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLEmptyInlineArrayIsRejected(t *testing.T) {
	_, err := ReadTOML("a = []\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for an empty array")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeParseEmptyArray {
		t.Errorf("error = %#v, want CodeParseEmptyArray", err)
	}
}

func TestTOMLNestedArrayIsRejected(t *testing.T) {
	_, err := ReadTOML("a = [[1, 2], [3, 4]]\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a nested bare array")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want CodeDocumentUnlabeledElement", err)
	}
}

// --- dotted keys ---

func TestTOMLDottedKeyCreatesImplicitTable(t *testing.T) {
	d, err := ReadTOML("host.name = \"x\"\nhost.port = 1\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Node.Edges) != 1 || d.Node.Edges[0].Label != "host" {
		t.Fatalf("expected a single host edge, got %+v", d.Node.Edges)
	}
	want := NodeDocument(NewNode().AddNode("host", NewNode().
		AddValue("name", ScalarValue(NewStringScalar("x"))).
		AddValue("port", ScalarValue(NewIntegerScalar(big.NewInt(1))))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLDottedKeyDeepNesting(t *testing.T) {
	d, err := ReadTOML("a.b.c = 5\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddNode("a", NewNode().AddNode("b", NewNode().
		AddValue("c", ScalarValue(NewIntegerScalar(big.NewInt(5)))))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

func TestTOMLQuotedKey(t *testing.T) {
	d, err := ReadTOML(`"q key" = 9` + "\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddValue("q key", ScalarValue(NewIntegerScalar(big.NewInt(9)))))
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
	d, err := ReadTOML("a = 1\na.b = 2\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Node.Edges) != 2 {
		t.Fatalf("expected 2 edges labeled a, got %+v", d.Node.Edges)
	}
	v, ok := d.Node.Edges[0].Target.Value()
	if !ok || v.Scalar.Kind != KindInteger {
		t.Errorf("first a edge = %+v, want the original scalar", d.Node.Edges[0])
	}
	child, ok := d.Node.Edges[1].Target.Node()
	if !ok || len(child.Edges) != 1 || child.Edges[0].Label != "b" {
		t.Errorf("second a edge = %+v, want a fresh node holding b", d.Node.Edges[1])
	}
}

func TestTOMLNestedErrorInsideInlineTableInsideArray(t *testing.T) {
	_, err := ReadTOML("items = [{a = []}]\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for the empty array nested inside the inline table")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeParseEmptyArray {
		t.Errorf("error = %#v, want CodeParseEmptyArray", err)
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
	d, err := ReadTOML(src, DefaultLimits())
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
	d, err := ReadTOML("", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsNode || len(d.Node.Edges) != 0 {
		t.Errorf("got %+v, want an empty node document", d)
	}
}

// --- syntax errors ---

func TestTOMLSyntaxErrorReported(t *testing.T) {
	_, err := ReadTOML("a = -0xFF\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected a parse error for a signed radix literal")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeParseUnexpectedToken {
		t.Errorf("error = %#v, want CodeParseUnexpectedToken", err)
	}
	if !strings.Contains(pe.Message, "radix") {
		t.Errorf("message = %q, want it to mention the radix-prefix rule", pe.Message)
	}
}

// --- limits ---

func TestTOMLLimitDepth(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDepth = 2
	_, err := ReadTOML("a.b.c.d = 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want CodeDocumentLimitDepth", err)
	}
}

func TestTOMLLimitDepthViaTableHeader(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDepth = 1
	_, err := ReadTOML("[a.b]\nc = 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want CodeDocumentLimitDepth", err)
	}
}

func TestTOMLLimitNodes(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxNodes = 2
	_, err := ReadTOML("a = {}\nb = {}\nc = {}\n", limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitNodes {
		t.Errorf("error = %#v, want CodeDocumentLimitNodes", err)
	}
}

func TestTOMLLimitDepthViaArrayTableParentPath(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDepth = 1
	_, err := ReadTOML("[[a.b.c]]\nx = 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want CodeDocumentLimitDepth", err)
	}
}

func TestTOMLLimitNodesViaArrayTable(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxNodes = 1
	_, err := ReadTOML("[[a]]\nx = 1\n[[a]]\ny = 2\n", limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitNodes {
		t.Errorf("error = %#v, want CodeDocumentLimitNodes", err)
	}
}

func TestTOMLLimitIntDigitsInsideArray(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := ReadTOML("n = [12345]\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
	}
}

func TestTOMLLimitIntDigits(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := ReadTOML("n = 12345\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
	}
}

// --- string escaping ---

func TestTOMLBasicStringEscapes(t *testing.T) {
	d, err := ReadTOML(`s = "a\nb\tc\"d"` + "\n", DefaultLimits())
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
	d, err := ReadTOML("s = \"\"\"hi\nthere\"\"\"\n", DefaultLimits())
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
	d, err := ReadTOML("t = true\nf = false\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().
		AddValue("t", ScalarValue(NewBooleanScalar(true))).
		AddValue("f", ScalarValue(NewBooleanScalar(false))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}
