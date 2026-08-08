package omnist

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

// --- model-mapping table (docs/formats/json.md) ---

func TestJSONModelMappingTable(t *testing.T) {
	cases := []struct {
		name string
		json string
		want Document
	}{
		{
			`{"a":1,"b":2} -> [(a,1),(b,2)]`,
			`{"a":1,"b":2}`,
			NodeDocument(NewNode().
				AddValue("a", ScalarValue(NewIntegerScalar(big.NewInt(1)))).
				AddValue("b", ScalarValue(NewIntegerScalar(big.NewInt(2))))),
		},
		{
			`{"m":["A","B"]} -> [(m,A),(m,B)]`,
			`{"m":["A","B"]}`,
			NodeDocument(NewNode().
				AddValue("m", ScalarValue(NewStringScalar("A"))).
				AddValue("m", ScalarValue(NewStringScalar("B")))),
		},
		{
			`{"m":["A"]} -> [(m,A)] (one edge)`,
			`{"m":["A"]}`,
			NodeDocument(NewNode().AddValue("m", ScalarValue(NewStringScalar("A")))),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadJSON(tc.json, DefaultLimits())
			if err != nil {
				t.Fatalf("ReadJSON(%s) failed: %v", tc.json, err)
			}
			if !docEqual(tc.want, got) {
				t.Errorf("ReadJSON(%s) = %#v, want %#v", tc.json, got, tc.want)
			}
		})
	}
}

// The count-1 asymmetry called out explicitly in §7.3: {"m":["A"]} reads to
// one edge, and that one-edge Document, written back, produces {"m":"A"}
// NOT {"m":["A"]}.
func TestJSONCountOneAsymmetry(t *testing.T) {
	doc, err := ReadJSON(`{"m":["A"]}`, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	got, err := WriteJSON(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if want := `{"m":"A"}`; got != want {
		t.Errorf("WriteJSON(ReadJSON(%q)) = %q, want %q", `{"m":["A"]}`, got, want)
	}
}

// --- worked example from docs/formats/json.md ---

func TestJSONWorkedExample(t *testing.T) {
	text := `{"order": {"id": "A1", "status": "shipped", "total": 29.97,
  "address": {"street": "1 Main", "city": "London"},
  "items": [{"sku": "W", "qty": 3, "price": 9.99},
            {"sku": "G", "qty": 1, "price": 9.99}]}}`

	item := func(sku string, qty int64, price float64) *Node {
		return NewNode().
			AddValue("sku", ScalarValue(NewStringScalar(sku))).
			AddValue("qty", ScalarValue(NewIntegerScalar(big.NewInt(qty)))).
			AddValue("price", ScalarValue(NewNumberScalar(price)))
	}
	want := NodeDocument(NewNode().AddNode("order", NewNode().
		AddValue("id", ScalarValue(NewStringScalar("A1"))).
		AddValue("status", ScalarValue(NewStringScalar("shipped"))).
		AddValue("total", ScalarValue(NewNumberScalar(29.97))).
		AddNode("address", NewNode().
			AddValue("street", ScalarValue(NewStringScalar("1 Main"))).
			AddValue("city", ScalarValue(NewStringScalar("London")))).
		AddNode("items", item("W", 3, 9.99)).
		AddNode("items", item("G", 1, 9.99))))

	got, err := ReadJSON(text, DefaultLimits())
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
		wantKind ScalarKind
	}{
		{`1`, KindInteger},
		{`-1`, KindInteger},
		{`0`, KindInteger},
		{`1.0`, KindNumber},
		{`1e0`, KindNumber},
		{`1E0`, KindNumber},
		{`-1.5`, KindNumber},
		{`100000000000000000000000000000000000000`, KindInteger}, // beyond float precision
	}
	for _, tc := range cases {
		t.Run(tc.json, func(t *testing.T) {
			doc, err := ReadJSON(tc.json, DefaultLimits())
			if err != nil {
				t.Fatalf("ReadJSON(%s) failed: %v", tc.json, err)
			}
			if doc.IsNode {
				t.Fatalf("ReadJSON(%s) produced a node document, want a bare value", tc.json)
			}
			if doc.Value.IsNull {
				t.Fatalf("ReadJSON(%s) produced null, want a scalar", tc.json)
			}
			if got := doc.Value.Scalar.Kind; got != tc.wantKind {
				t.Errorf("ReadJSON(%s).Value.Scalar.Kind = %v, want %v", tc.json, got, tc.wantKind)
			}
		})
	}
}

func TestJSONHugeIntegerPreservesExactDigits(t *testing.T) {
	digits := strings.Repeat("9", 60)
	doc, err := ReadJSON(digits, DefaultLimits())
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
			doc, err := ReadJSON(json, DefaultLimits())
			if err != nil {
				t.Fatalf("ReadJSON(%s) failed: %v", json, err)
			}
			if doc.Value.Scalar.Kind != KindString {
				t.Errorf("ReadJSON(%s).Value.Scalar.Kind = %v, want KindString (stage 1 never consults a schema)", json, doc.Value.Scalar.Kind)
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
			_, err := ReadJSON(tc.json, DefaultLimits())
			if err == nil {
				t.Fatalf("ReadJSON(%s) succeeded, want a rejection error", tc.json)
			}
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("ReadJSON(%s) error is %T, want *ParseError", tc.json, err)
			}
			if pe.Code != CodeDocumentUnlabeledElement {
				t.Errorf("ReadJSON(%s) error code = %s, want %s", tc.json, pe.Code, CodeDocumentUnlabeledElement)
			}
		})
	}
}

func TestJSONEmptyArrayRejected(t *testing.T) {
	_, err := ReadJSON(`{"m":[]}`, DefaultLimits())
	if err == nil {
		t.Fatal("ReadJSON(`{\"m\":[]}`) succeeded, want a rejection error")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("error is %T, want *ParseError", err)
	}
	if pe.Code != CodeParseEmptyArray {
		t.Errorf("error code = %s, want %s", pe.Code, CodeParseEmptyArray)
	}
}

// --- round-trip property ---

func TestJSONRoundTripProperty(t *testing.T) {
	bigDigits := strings.Repeat("9", 50)
	bigInt := new(big.Int)
	bigInt.SetString(bigDigits, 10)

	cases := []struct {
		name string
		doc  Document
	}{
		{"empty node", NodeDocument(NewNode())},
		{"bare string scalar", ValueDocument(ScalarValue(NewStringScalar("hi")))},
		{"bare null", ValueDocument(NullValue())},
		{"bare integer", ValueDocument(ScalarValue(NewIntegerScalar(big.NewInt(-42))))},
		{"bare boolean true", ValueDocument(ScalarValue(NewBooleanScalar(true)))},
		{"bare number", ValueDocument(ScalarValue(NewNumberScalar(3.5)))},
		{
			"mixed scalar kinds (JSON has no native temporal, so those are excluded here)",
			NodeDocument(NewNode().
				AddValue("str", ScalarValue(NewStringScalar("hello"))).
				AddValue("int", ScalarValue(NewIntegerScalar(bigInt))).
				AddValue("num", ScalarValue(NewNumberScalar(3.5))).
				AddValue("boolT", ScalarValue(NewBooleanScalar(true))).
				AddValue("boolF", ScalarValue(NewBooleanScalar(false))).
				AddValue("null", NullValue()),
			),
		},
		{
			"nested node two levels",
			NodeDocument(NewNode().AddNode("address", NewNode().
				AddValue("city", ScalarValue(NewStringScalar("Zurich"))).
				AddNode("geo", NewNode().
					AddValue("lat", ScalarValue(NewNumberScalar(47.37))).
					AddValue("lon", ScalarValue(NewNumberScalar(8.55)))))),
		},
		{
			"repeated labels (adjacent, since JSON grouping loses interleaving)",
			NodeDocument(NewNode().
				AddValue("tag", ScalarValue(NewStringScalar("x"))).
				AddValue("tag", ScalarValue(NewStringScalar("y"))).
				AddValue("tag", ScalarValue(NewStringScalar("z")))),
		},
		{
			"empty child node",
			NodeDocument(NewNode().AddNode("empty", NewNode())),
		},
		{
			"string escaping: quote, backslash, control, unicode",
			NodeDocument(NewNode().
				AddValue("s", ScalarValue(NewStringScalar("a\"b\\c\nd\re\tf\x01g héllo 世界")))),
		},
		{
			"number kinds: integer-valued float, exponent, big",
			NodeDocument(NewNode().
				AddValue("whole", ScalarValue(NewNumberScalar(5))).
				AddValue("frac", ScalarValue(NewNumberScalar(3.25))).
				AddValue("big", ScalarValue(NewNumberScalar(1e30)))),
		},
		{
			"labels needing JSON string escaping",
			NodeDocument(NewNode().
				AddValue("has space", ScalarValue(NewStringScalar("v1"))).
				AddValue("has\"quote", ScalarValue(NewStringScalar("v2")))),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := WriteJSON(tc.doc)
			if err != nil {
				t.Fatalf("WriteJSON failed: %v", err)
			}
			got, err := ReadJSON(text, DefaultLimits())
			if err != nil {
				t.Fatalf("ReadJSON(WriteJSON(doc)) failed: %v\ntext: %s", err, text)
			}
			if !docEqual(tc.doc, got) {
				t.Fatalf("round-trip mismatch\ntext: %s\ngot:  %#v\nwant: %#v", text, got, tc.doc)
			}
		})
	}
}

// NaN/Infinity survive a write(lenient)->null->read round trip only in the
// sense that reading the null back gives NullValue, not the original NaN
// (that data is genuinely lost by design) — covered separately below, not
// as part of the round-trip property (which is explicitly scoped to things
// JSON CAN represent, per the issue's test list).
func TestJSONNaNInfinityWriteThenReadBecomesNull(t *testing.T) {
	doc := NodeDocument(NewNode().
		AddValue("n", ScalarValue(NewNumberScalar(math.NaN()))).
		AddValue("ok", ScalarValue(NewStringScalar("still here"))))
	text, err := WriteJSON(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	got, err := ReadJSON(text, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	want := NodeDocument(NewNode().
		AddValue("n", NullValue()).
		AddValue("ok", ScalarValue(NewStringScalar("still here"))))
	if !docEqual(want, got) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// --- cross-format structural equality (JSON vs OML) ---

func TestJSONCrossFormatStructuralEqualityWithOML(t *testing.T) {
	omlText := `id: "A1"; total: 29.97; count: 3; ok: true; nothing: null; address: { street: "1 Main"; city: "London" }; items: "W"; items: "G"`
	jsonText := `{"id":"A1","total":29.97,"count":3,"ok":true,"nothing":null,"address":{"street":"1 Main","city":"London"},"items":["W","G"]}`

	omlDoc, err := ReadOML(omlText, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadOML failed: %v", err)
	}
	jsonDoc, err := ReadJSON(jsonText, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if !docEqual(omlDoc, jsonDoc) {
		t.Fatalf("format-independence violated:\nOML:  %#v\nJSON: %#v", omlDoc, jsonDoc)
	}
}

// --- limits (shared LimitChecker, same as OML) ---

func TestJSONLimitDepth(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDepth = 2
	// {"a":{"b":{"c":{"d":1}}}} nests three levels of objects beneath the
	// (uncounted) root ("a", "b", "c" each open a nested object), so
	// depth 3 exceeds a MaxDepth of 2.
	_, err := ReadJSON(`{"a":{"b":{"c":{"d":1}}}}`, limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want CodeDocumentLimitDepth", err)
	}
}

func TestJSONLimitDepthInsideArray(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDepth = 1
	_, err := ReadJSON(`{"a":[{"b":1},{"c":{"d":1}}]}`, limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want CodeDocumentLimitDepth", err)
	}
}

func TestJSONLimitNodes(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxNodes = 2
	_, err := ReadJSON(`{"a":{},"b":{},"c":{}}`, limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitNodes {
		t.Errorf("error = %#v, want CodeDocumentLimitNodes", err)
	}
}

func TestJSONLimitIntDigits(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 5
	_, err := ReadJSON(`123456`, limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
	}
}

func TestJSONLimitIntDigitsWithinObject(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := ReadJSON(`{"n":12345}`, limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
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
			_, err := ReadJSON(tc.json, DefaultLimits())
			if err == nil {
				t.Fatalf("ReadJSON(%q) succeeded, want an error", tc.json)
			}
			if _, ok := err.(*ParseError); !ok {
				t.Errorf("ReadJSON(%q) error is %T, want *ParseError", tc.json, err)
			}
		})
	}
}

func TestJSONMultilinePositionReporting(t *testing.T) {
	text := "{\n  \"a\": 1,\n  \"b\": ]\n}"
	_, err := ReadJSON(text, DefaultLimits())
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("error is %T, want *ParseError", err)
	}
	if pe.Line != 3 {
		t.Errorf("Line = %d, want 3", pe.Line)
	}
	if pe.Error() == "" {
		t.Error("ParseError.Error() returned empty string")
	}
}

// --- offsetToLineCol helper ---

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
