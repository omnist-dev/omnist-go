package json

import (
	"math"
	"math/big"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- model-mapping table (docs/formats/json.md) ---

func TestJSONModelMappingTable(t *testing.T) {
	cases := []struct {
		name string
		json string
		want omnist.Document
	}{
		{
			`{"a":1,"b":2} -> [(a,1),(b,2)]`,
			`{"a":1,"b":2}`,
			omnist.NodeDocument(omnist.NewNode().
				AddValue("a", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))).
				AddValue("b", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(2))))),
		},
		{
			`{"m":["A","B"]} -> [(m,A),(m,B)]`,
			`{"m":["A","B"]}`,
			omnist.NodeDocument(omnist.NewNode().
				AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
				AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B")))),
		},
		{
			`{"m":["A"]} -> [(m,A)] (one edge)`,
			`{"m":["A"]}`,
			omnist.NodeDocument(omnist.NewNode().AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A")))),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Read(tc.json, omnist.DefaultLimits())
			if err != nil {
				t.Fatalf("Read(%s) failed: %v", tc.json, err)
			}
			if !docEqual(tc.want, got) {
				t.Errorf("Read(%s) = %#v, want %#v", tc.json, got, tc.want)
			}
		})
	}
}

// The count-1 asymmetry called out explicitly in §7.3: {"m":["A"]} reads to
// one edge, and that one-edge omnist.Document, written back, produces {"m":"A"}
// NOT {"m":["A"]}.
func TestJSONCountOneAsymmetry(t *testing.T) {
	doc, err := Read(`{"m":["A"]}`, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	got, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if want := `{"m": "A"}`; got != want {
		t.Errorf("Write(Read(%q)) = %q, want %q", `{"m":["A"]}`, got, want)
	}
}

// --- worked example from docs/formats/json.md ---

func TestJSONWorkedExample(t *testing.T) {
	text := `{"order": {"id": "A1", "status": "shipped", "total": 29.97,
  "address": {"street": "1 Main", "city": "London"},
  "items": [{"sku": "W", "qty": 3, "price": 9.99},
            {"sku": "G", "qty": 1, "price": 9.99}]}}`

	item := func(sku string, qty int64, price float64) *omnist.Node {
		return omnist.NewNode().
			AddValue("sku", omnist.ScalarValue(omnist.NewStringScalar(sku))).
			AddValue("qty", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(qty)))).
			AddValue("price", omnist.ScalarValue(omnist.NewNumberScalar(price)))
	}
	want := omnist.NodeDocument(omnist.NewNode().AddNode("order", omnist.NewNode().
		AddValue("id", omnist.ScalarValue(omnist.NewStringScalar("A1"))).
		AddValue("status", omnist.ScalarValue(omnist.NewStringScalar("shipped"))).
		AddValue("total", omnist.ScalarValue(omnist.NewNumberScalar(29.97))).
		AddNode("address", omnist.NewNode().
			AddValue("street", omnist.ScalarValue(omnist.NewStringScalar("1 Main"))).
			AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("London")))).
		AddNode("items", item("W", 3, 9.99)).
		AddNode("items", item("G", 1, 9.99))))

	got, err := Read(text, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if !docEqual(want, got) {
		t.Fatalf("worked example mismatch:\ngot:  %#v\nwant: %#v", got, want)
	}
}

// --- integer vs. number preservation ---

func TestJSONIntegerVsNumberKind(t *testing.T) {
	cases := []struct {
		json     string
		wantKind omnist.ScalarKind
	}{
		{`1`, omnist.KindInteger},
		{`-1`, omnist.KindInteger},
		{`0`, omnist.KindInteger},
		{`1.0`, omnist.KindNumber},
		{`1e0`, omnist.KindNumber},
		{`1E0`, omnist.KindNumber},
		{`-1.5`, omnist.KindNumber},
		{`100000000000000000000000000000000000000`, omnist.KindInteger}, // beyond float precision
	}
	for _, tc := range cases {
		t.Run(tc.json, func(t *testing.T) {
			doc, err := Read(tc.json, omnist.DefaultLimits())
			if err != nil {
				t.Fatalf("Read(%s) failed: %v", tc.json, err)
			}
			if doc.IsNode {
				t.Fatalf("Read(%s) produced a node document, want a bare value", tc.json)
			}
			if doc.Value.IsNull {
				t.Fatalf("Read(%s) produced null, want a scalar", tc.json)
			}
			if got := doc.Value.Scalar.Kind; got != tc.wantKind {
				t.Errorf("Read(%s).Value.Scalar.Kind = %v, want %v", tc.json, got, tc.wantKind)
			}
		})
	}
}

func TestJSONHugeIntegerPreservesExactDigits(t *testing.T) {
	digits := strings.Repeat("9", 60)
	doc, err := Read(digits, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	want := new(big.Int)
	want.SetString(digits, 10)
	if doc.Value.Scalar.Int.Cmp(want) != 0 {
		t.Errorf("integer value mismatch: got %s, want %s", doc.Value.Scalar.Int.String(), digits)
	}
}

// --- no temporal auto-detection ---

func TestJSONNoTemporalAutoDetection(t *testing.T) {
	cases := []string{
		`"2024-01-01"`,
		`"12:30:00"`,
		`"2024-01-01T12:30:00"`,
	}
	for _, json := range cases {
		t.Run(json, func(t *testing.T) {
			doc, err := Read(json, omnist.DefaultLimits())
			if err != nil {
				t.Fatalf("Read(%s) failed: %v", json, err)
			}
			if doc.Value.Scalar.Kind != omnist.KindString {
				t.Errorf("Read(%s).Value.Scalar.Kind = %v, want omnist.KindString (stage 1 never consults a schema)", json, doc.Value.Scalar.Kind)
			}
		})
	}
}

// --- bare nested arrays rejected ---

func TestJSONBareNestedArrayRejected(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"nested inside labeled array", `{"m":[[1,2],[3,4]]}`},
		{"top-level bare array", `[[1,2],[3,4]]`},
		{"top-level single-element bare array", `[1]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Read(tc.json, omnist.DefaultLimits())
			if err == nil {
				t.Fatalf("Read(%s) succeeded, want a rejection error", tc.json)
			}
			pe, ok := err.(*omnist.ParseError)
			if !ok {
				t.Fatalf("Read(%s) error is %T, want *omnist.ParseError", tc.json, err)
			}
			if pe.Code != omnist.CodeDocumentUnlabeledElement {
				t.Errorf("Read(%s) error code = %s, want %s", tc.json, pe.Code, omnist.CodeDocumentUnlabeledElement)
			}
		})
	}
}

func TestJSONEmptyArrayRejected(t *testing.T) {
	_, err := Read(`{"m":[]}`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("Read(`{\"m\":[]}`) succeeded, want a rejection error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is %T, want *omnist.ParseError", err)
	}
	if pe.Code != omnist.CodeParseEmptyArray {
		t.Errorf("error code = %s, want %s", pe.Code, omnist.CodeParseEmptyArray)
	}
}

// --- round-trip property ---

func TestJSONRoundTripProperty(t *testing.T) {
	bigDigits := strings.Repeat("9", 50)
	bigInt := new(big.Int)
	bigInt.SetString(bigDigits, 10)

	cases := []struct {
		name string
		doc  omnist.Document
	}{
		{"empty node", omnist.NodeDocument(omnist.NewNode())},
		{"bare string scalar", omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("hi")))},
		{"bare null", omnist.ValueDocument(omnist.NullValue())},
		{"bare integer", omnist.ValueDocument(omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(-42))))},
		{"bare boolean true", omnist.ValueDocument(omnist.ScalarValue(omnist.NewBooleanScalar(true)))},
		{"bare number", omnist.ValueDocument(omnist.ScalarValue(omnist.NewNumberScalar(3.5)))},
		{
			"mixed scalar kinds (JSON has no native temporal, so those are excluded here)",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("str", omnist.ScalarValue(omnist.NewStringScalar("hello"))).
				AddValue("int", omnist.ScalarValue(omnist.NewIntegerScalar(bigInt))).
				AddValue("num", omnist.ScalarValue(omnist.NewNumberScalar(3.5))).
				AddValue("boolT", omnist.ScalarValue(omnist.NewBooleanScalar(true))).
				AddValue("boolF", omnist.ScalarValue(omnist.NewBooleanScalar(false))).
				AddValue("null", omnist.NullValue()),
			),
		},
		{
			"nested node two levels",
			omnist.NodeDocument(omnist.NewNode().AddNode("address", omnist.NewNode().
				AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("Zurich"))).
				AddNode("geo", omnist.NewNode().
					AddValue("lat", omnist.ScalarValue(omnist.NewNumberScalar(47.37))).
					AddValue("lon", omnist.ScalarValue(omnist.NewNumberScalar(8.55)))))),
		},
		{
			"repeated labels (adjacent, since JSON grouping loses interleaving)",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("x"))).
				AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("y"))).
				AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("z")))),
		},
		{
			"empty child node",
			omnist.NodeDocument(omnist.NewNode().AddNode("empty", omnist.NewNode())),
		},
		{
			"string escaping: quote, backslash, control, unicode",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("s", omnist.ScalarValue(omnist.NewStringScalar("a\"b\\c\nd\re\tf\x01g héllo 世界")))),
		},
		{
			"number kinds: integer-valued float, exponent, big",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("whole", omnist.ScalarValue(omnist.NewNumberScalar(5))).
				AddValue("frac", omnist.ScalarValue(omnist.NewNumberScalar(3.25))).
				AddValue("big", omnist.ScalarValue(omnist.NewNumberScalar(1e30)))),
		},
		{
			"labels needing JSON string escaping",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("has space", omnist.ScalarValue(omnist.NewStringScalar("v1"))).
				AddValue("has\"quote", omnist.ScalarValue(omnist.NewStringScalar("v2")))),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := Write(tc.doc)
			if err != nil {
				t.Fatalf("WriteJSON failed: %v", err)
			}
			got, err := Read(text, omnist.DefaultLimits())
			if err != nil {
				t.Fatalf("Read(Write(doc)) failed: %v\ntext: %s", err, text)
			}
			if !docEqual(tc.doc, got) {
				t.Fatalf("round-trip mismatch\ntext: %s\ngot:  %#v\nwant: %#v", text, got, tc.doc)
			}
		})
	}
}

// NaN/Infinity survive a write(lenient)->null->read round trip only in the
// sense that reading the null back gives omnist.NullValue, not the original NaN
// (that data is genuinely lost by design) — covered separately below, not
// as part of the round-trip property (which is explicitly scoped to things
// JSON CAN represent, per the issue's test list).
func TestJSONNaNInfinityWriteThenReadBecomesNull(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().
		AddValue("n", omnist.ScalarValue(omnist.NewNumberScalar(math.NaN()))).
		AddValue("ok", omnist.ScalarValue(omnist.NewStringScalar("still here"))))
	text, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	got, err := Read(text, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	want := omnist.NodeDocument(omnist.NewNode().
		AddValue("n", omnist.NullValue()).
		AddValue("ok", omnist.ScalarValue(omnist.NewStringScalar("still here"))))
	if !docEqual(want, got) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// TestJSONCrossFormatStructuralEqualityWithOML moved to
// json_reader_public_test.go (package omnist_test, issue #43): it is the
// only test in this file that needs oml.Read, and an internal (package
// omnist) test file cannot import oml without an import-cycle error in the
// test build (oml imports omnist for omnist.Document/omnist.Node/etc.) — the same
// constraint issue #41 already solved for OSD. Every other test here stays
// put, since most also use the unexported offsetToLineCol/docEqual, which
// isn't reachable from an external test package either.

// --- limits (shared omnist.LimitChecker, same as OML) ---

func TestJSONLimitDepth(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 2
	// {"a":{"b":{"c":{"d":1}}}} nests three levels of objects beneath the
	// (uncounted) root ("a", "b", "c" each open a nested object), so
	// depth 3 exceeds a MaxDepth of 2.
	_, err := Read(`{"a":{"b":{"c":{"d":1}}}}`, limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitDepth", err)
	}
}

func TestJSONLimitDepthInsideArray(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 1
	_, err := Read(`{"a":[{"b":1},{"c":{"d":1}}]}`, limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitDepth", err)
	}
}

func TestJSONLimitNodes(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxNodes = 2
	_, err := Read(`{"a":{},"b":{},"c":{}}`, limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitNodes {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitNodes", err)
	}
}

func TestJSONLimitIntDigits(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 5
	_, err := Read(`123456`, limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

func TestJSONLimitIntDigitsWithinObject(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read(`{"n":12345}`, limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

// --- malformed input / error paths ---

func TestJSONMalformedInputs(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"empty input", ``},
		{"truncated object", `{"a":1`},
		{"truncated bare array", `[1,2`},
		{"stray closing brace", `}`},
		{"stray closing bracket", `]`},
		{"trailing comma in object", `{"a":1,}`},
		{"non-string key", `{1:2}`},
		{"trailing content after object", `{"a":1} garbage`},
		{"trailing content after scalar", `1 2`},
		{"unterminated string", `"abc`},
		{"huge exponent overflows float64", `1e400`},
		{"nan token is not valid JSON", `NaN`},
		{"infinity token is not valid JSON", `Infinity`},
		{"truncated labeled array right after '['", `{"a":[`},
		{"truncated labeled array after an element", `{"a":[1,2`},
		{"malformed element mid-array", `{"a":[1,"ab}`},
		{"huge exponent inside array element", `{"a":[1e400]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Read(tc.json, omnist.DefaultLimits())
			if err == nil {
				t.Fatalf("Read(%q) succeeded, want an error", tc.json)
			}
			if _, ok := err.(*omnist.ParseError); !ok {
				t.Errorf("Read(%q) error is %T, want *omnist.ParseError", tc.json, err)
			}
		})
	}
}

func TestJSONMultilinePositionReporting(t *testing.T) {
	text := "{\n  \"a\": 1,\n  \"b\": ]\n}"
	_, err := Read(text, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is %T, want *omnist.ParseError", err)
	}
	if pe.Line != 3 {
		t.Errorf("Line = %d, want 3", pe.Line)
	}
	if pe.Error() == "" {
		t.Error("omnist.ParseError.Error() returned empty string")
	}
}

// --- offsetToLineCol helper ---

// --- issue #33: document.* diagnostics carry omnist.Document paths (spec §8.4),
// never text-position paths ---

func TestNestedArrayIsRejectedWithDocumentPath(t *testing.T) {
	_, err := Read(`{"m":[[1,2],[3,4]]}`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is not *omnist.ParseError: %T %v", err, err)
	}
	if pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Errorf("code = %q, want %q", pe.Code, omnist.CodeDocumentUnlabeledElement)
	}
	if pe.Path != "$.m[0]" {
		t.Errorf("path = %q, want %q", pe.Path, "$.m[0]")
	}
}

func TestTopLevelArrayIsRejectedWithDollarPath(t *testing.T) {
	_, err := Read(`[1,2]`, omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is not *omnist.ParseError: %T %v", err, err)
	}
	if pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Errorf("code = %q, want %q", pe.Code, omnist.CodeDocumentUnlabeledElement)
	}
	if pe.Path != "$" {
		t.Errorf("path = %q, want %q", pe.Path, "$")
	}
}

func TestJSONNodeCountLimitUsesDollarPath(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxNodes = 1
	_, err := Read(`{"a":{"b":{"c":1}}}`, limits)
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is not *omnist.ParseError: %T %v", err, err)
	}
	if pe.Code != omnist.CodeDocumentLimitNodes {
		t.Errorf("code = %q, want %q", pe.Code, omnist.CodeDocumentLimitNodes)
	}
	if pe.Path != "$" {
		t.Errorf("path = %q, want %q", pe.Path, "$")
	}
}

func TestJSONIntDigitsLimitUsesDocumentPath(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read(`{"n":1000}`, limits)
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is not *omnist.ParseError: %T %v", err, err)
	}
	if pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("code = %q, want %q", pe.Code, omnist.CodeDocumentLimitIntDigits)
	}
	if pe.Path != "$.n" {
		t.Errorf("path = %q, want %q", pe.Path, "$.n")
	}
}

func TestOffsetToLineCol(t *testing.T) {
	text := "ab\ncd\nef"
	cases := []struct {
		offset   int64
		wantLine int
		wantCol  int
	}{
		{0, 1, 1},
		{2, 1, 3},
		{3, 2, 1},
		{6, 3, 1},
	}
	for _, tc := range cases {
		line, col := offsetToLineCol(text, tc.offset)
		if line != tc.wantLine || col != tc.wantCol {
			t.Errorf("offsetToLineCol(%q, %d) = (%d,%d), want (%d,%d)", text, tc.offset, line, col, tc.wantLine, tc.wantCol)
		}
	}
}
