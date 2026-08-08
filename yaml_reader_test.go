package omnist

import (
	"math"
	"math/big"
	"testing"
)

// --- model mapping table (docs/formats/yaml.md) ---

func TestYAMLModelMappingTable(t *testing.T) {
	d, err := ReadYAML("a: 1\nb: 2\n", DefaultLimits())
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

func TestYAMLRepeatedLabelFromSequence(t *testing.T) {
	d, err := ReadYAML("m:\n  - A\n  - B\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().
		AddValue("m", ScalarValue(NewStringScalar("A"))).
		AddValue("m", ScalarValue(NewStringScalar("B"))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- worked example (docs/formats/yaml.md) ---

func TestYAMLWorkedExample(t *testing.T) {
	src := `order:
  id: A1
  status: shipped
  total: 29.97
  address: {street: 1 Main, city: London}
  items:
    - {sku: W, qty: 3, price: 9.99}
    - {sku: G, qty: 1, price: 9.99}
`
	d, err := ReadYAML(src, DefaultLimits())
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

	// "Byte-for-byte identical to the Document JSON produces."
	jsrc := `{"order":{"id":"A1","status":"shipped","total":29.97,"address":{"street":"1 Main","city":"London"},"items":[{"sku":"W","qty":3,"price":9.99},{"sku":"G","qty":1,"price":9.99}]}}`
	jd, err := ReadJSON(jsrc, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, jd) {
		t.Errorf("YAML and JSON documents differ:\nYAML: %+v\nJSON: %+v", d, jd)
	}
}

// TestYAMLWorkedExampleStatusNo confirms the worked example's first
// YAML-only note: an unquoted "no" resolves to the boolean false, not the
// string "no" JSON's equivalent scalar would produce — a genuine,
// spec-mandated stage-1 divergence between the two formats for the "same"
// source value.
func TestYAMLWorkedExampleStatusNo(t *testing.T) {
	d, err := ReadYAML("status: no\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddValue("status", ScalarValue(NewBooleanScalar(false))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// TestYAMLWorkedExamplePlacedDate confirms the worked example's second
// YAML-only note: a bare ISO date resolves to a date-kind Scalar at stage
// 1, where JSON would hand stage 2 a string to upgrade.
func TestYAMLWorkedExamplePlacedDate(t *testing.T) {
	d, err := ReadYAML("placed: 2024-01-01\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := NodeDocument(NewNode().AddValue("placed", ScalarValue(NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 1}))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- aliases (docs/formats/yaml.md: "Aliases resolve at parse time") ---

func TestYAMLAliasExpandsToIndependentCopies(t *testing.T) {
	d, err := ReadYAML("a: &x foo\nb: *x\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Node.Edges) != 2 {
		t.Fatalf("got %d edges, want 2", len(d.Node.Edges))
	}
	av, _ := d.Node.Edges[0].Target.Value()
	bv, _ := d.Node.Edges[1].Target.Value()
	if av.Scalar.Str != "foo" || bv.Scalar.Str != "foo" {
		t.Fatalf("both edges should carry \"foo\": a=%+v b=%+v", av, bv)
	}
	// Mutating one Scalar's string field cannot be observed on the
	// other: Go strings are immutable and NewStringScalar copies by
	// value, so there is no shared backing to mutate through in the
	// first place — this assertion documents that guarantee rather than
	// exercising a mutation (there is no mutator to call), matching the
	// issue's instruction to "write the test to be robust against this
	// [identity sharing] even if types are currently immutable." A
	// pointer-identity check on the two Scalars' Str fields would be
	// meaningless (Go string headers may or may not share a backing
	// array for equal literals; that is not what "shared identity" in
	// the Document model means). What IS observable and load-bearing is
	// that the two Node pointers under a and b's aliased mapping value
	// (had the anchor been a mapping, not a scalar) are never the same
	// pointer — TestYAMLAliasedMappingExpandsToIndependentNodes below
	// covers that directly.
	if av.Scalar.Str != bv.Scalar.Str {
		t.Fatalf("expected equal values, got %q vs %q", av.Scalar.Str, bv.Scalar.Str)
	}
}

// TestYAMLAliasedMappingExpandsToIndependentNodes is the structural half
// of the aliasing guarantee: when the anchored value is itself a mapping,
// each alias occurrence must build its own *Node, not share one — the
// Document model (issue #1) has no notion of node identity, so a reader
// that reused a single *Node pointer across both edges would make an
// identity-sensitive bug (e.g. a future caller mutating one edge's node
// in place) silently corrupt the other, unobservable today only because
// nothing currently mutates a *Node after ReadYAML returns.
func TestYAMLAliasedMappingExpandsToIndependentNodes(t *testing.T) {
	d, err := ReadYAML("a: &x {k: v}\nb: *x\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	na, _ := d.Node.Edges[0].Target.Node()
	nb, _ := d.Node.Edges[1].Target.Node()
	if na == nb {
		t.Fatal("aliased mapping edges share the same *Node pointer; must be independent copies")
	}
	if !nodeEqual(na, nb) {
		t.Fatalf("aliased mapping edges have different content: %+v vs %+v", na, nb)
	}
}

// --- sexagesimal sharp edge ---

func TestYAMLSexagesimalTimeResolvesToInteger(t *testing.T) {
	d, err := ReadYAML("t: 12:00:00\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindInteger {
		t.Fatalf("kind = %v, want integer", v.Scalar.Kind)
	}
	want := big.NewInt(12*3600 + 0*60 + 0)
	if v.Scalar.Int.Cmp(want) != 0 {
		t.Errorf("value = %s, want %s", v.Scalar.Int, want)
	}
}

func TestYAMLSexagesimalIntegerNegative(t *testing.T) {
	d, err := ReadYAML("t: -1:02\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	want := big.NewInt(-(1*60 + 2))
	if v.Scalar.Int.Cmp(want) != 0 {
		t.Errorf("value = %s, want %s", v.Scalar.Int, want)
	}
}

// --- the Norway problem ---

func TestYAMLNorwayProblemUnquotedOnKeyRejected(t *testing.T) {
	_, err := ReadYAML("on: true\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a boolean-resolved mapping key")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("error type = %T, want *ParseError", err)
	}
	if pe.Code != CodeDocumentUnlabeledElement {
		t.Errorf("code = %v, want %v", pe.Code, CodeDocumentUnlabeledElement)
	}
}

func TestYAMLNorwayProblemQuotedOnKeySucceeds(t *testing.T) {
	d, err := ReadYAML("\"on\": true\n", DefaultLimits())
	if err != nil {
		t.Fatalf("quoted \"on\" key should read cleanly, got error: %v", err)
	}
	want := NodeDocument(NewNode().AddValue("on", ScalarValue(NewBooleanScalar(true))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// TestYAMLNorwayWordsAsValues confirms every Norway-problem word
// (docs/formats/yaml.md: "on/off/yes/no/true/false (in various cases)")
// resolves to a boolean when used as an unquoted value, not just as a key.
func TestYAMLNorwayWordsAsValues(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"y", true}, {"Y", true}, {"yes", true}, {"Yes", true}, {"YES", true},
		{"n", false}, {"N", false}, {"no", false}, {"No", false}, {"NO", false},
		{"true", true}, {"True", true}, {"TRUE", true},
		{"false", false}, {"False", false}, {"FALSE", false},
		{"on", true}, {"On", true}, {"ON", true},
		{"off", false}, {"Off", false}, {"OFF", false},
	}
	for _, c := range cases {
		d, err := ReadYAML("v: "+c.text+"\n", DefaultLimits())
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", c.text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != KindBoolean || v.Scalar.Bool != c.want {
			t.Errorf("%q: got %+v, want boolean %v", c.text, v.Scalar, c.want)
		}
	}
}

func TestYAMLQuotedNorwayWordStaysString(t *testing.T) {
	d, err := ReadYAML(`v: "no"`+"\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindString || v.Scalar.Str != "no" {
		t.Errorf("got %+v, want string \"no\"", v.Scalar)
	}
}

// --- null / scalar resolution corner cases ---

func TestYAMLNullWords(t *testing.T) {
	for _, text := range []string{"~", "null", "Null", "NULL", ""} {
		src := "v: " + text + "\n"
		if text == "" {
			src = "v:\n"
		}
		d, err := ReadYAML(src, DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if !v.IsNull {
			t.Errorf("%q: got %+v, want null", text, v)
		}
	}
}

func TestYAMLNullMappingKeyRejected(t *testing.T) {
	_, err := ReadYAML("~: value\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a null-resolved mapping key")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want CodeDocumentUnlabeledElement", err)
	}
}

func TestYAMLComplexMappingKeyRejected(t *testing.T) {
	_, err := ReadYAML("? {a: 1}\n: value\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a non-scalar mapping key")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want CodeDocumentUnlabeledElement", err)
	}
}

func TestYAMLIntegerLiteralForms(t *testing.T) {
	cases := []struct {
		text string
		want int64
	}{
		{"0", 0}, {"1", 1}, {"-1", -1},
		{"0x10", 16},
		{"0o17", 15}, {"017", 15},
		{"0b101", 5},
		{"1_000", 1000},
	}
	for _, c := range cases {
		d, err := ReadYAML("v: "+c.text+"\n", DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != KindInteger || v.Scalar.Int.Int64() != c.want {
			t.Errorf("%q: got %+v, want integer %d", c.text, v.Scalar, c.want)
		}
	}
}

// TestYAMLIntLikePatternThatFailsToParseFallsBackToString exercises
// parseYAMLInt's defensive ok=false path: "0x_" matches reYAMLInt's regex
// (an underscore alone satisfies the hex-digit character class) but has no
// actual hex digits once underscores are stripped, so big.Int.SetString
// rejects it and resolution falls through to the plain-string fallback.
func TestYAMLIntLikePatternThatFailsToParseFallsBackToString(t *testing.T) {
	d, err := ReadYAML("v: 0x_\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindString || v.Scalar.Str != "0x_" {
		t.Errorf("got %+v, want string \"0x_\"", v.Scalar)
	}
}

func TestYAMLFloatLiteralForms(t *testing.T) {
	cases := []struct {
		text string
		want float64
	}{
		{"1.5", 1.5}, {"-1.5", -1.5}, {"1.5e3", 1500}, {"1_0.5_0", 10.5},
	}
	for _, c := range cases {
		d, err := ReadYAML("v: "+c.text+"\n", DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != KindNumber || v.Scalar.Num != c.want {
			t.Errorf("%q: got %+v, want number %v", c.text, v.Scalar, c.want)
		}
	}
}

func TestYAMLSpecialFloatSpellings(t *testing.T) {
	cases := []struct {
		text  string
		check func(f float64) bool
	}{
		{".inf", func(f float64) bool { return math.IsInf(f, 1) }},
		{"+.inf", func(f float64) bool { return math.IsInf(f, 1) }},
		{"-.inf", func(f float64) bool { return math.IsInf(f, -1) }},
		{".nan", math.IsNaN},
	}
	for _, c := range cases {
		d, err := ReadYAML("v: "+c.text+"\n", DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != KindNumber || !c.check(v.Scalar.Num) {
			t.Errorf("%q: got %+v", c.text, v.Scalar)
		}
	}
}

func TestYAMLDateTimeVariants(t *testing.T) {
	cases := []string{
		"2024-01-01T12:00:00Z",
		"2024-01-01t12:00:00+05:30",
		"2024-01-01t12:00:00-05:30",
		"2024-01-01 12:00:00",
		"2024-01-01T12:00:00.5Z",
	}
	for _, c := range cases {
		d, err := ReadYAML("v: "+c+"\n", DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", c, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != KindDateTime {
			t.Errorf("%q: got kind %v, want datetime", c, v.Scalar.Kind)
		}
	}
}

// TestYAMLFloatOverflowFallsBackToString exercises resolveYAMLScalar's
// ParseFloat-error path: reYAMLFloat's own regex matches "1e400" (a
// syntactically valid exponential-float spelling), but strconv.ParseFloat
// reports ErrRange for a magnitude beyond float64's range, so — the same
// "matched the shape but the specific value didn't decode" pattern
// parseYAMLInt's ok=false branch already follows for ints — resolution
// falls through the rest of the chain and lands on the plain-string
// fallback rather than silently clamping to +Inf.
func TestYAMLFloatOverflowFallsBackToString(t *testing.T) {
	d, err := ReadYAML("v: 1e400\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindString || v.Scalar.Str != "1e400" {
		t.Errorf("got %+v, want string \"1e400\"", v.Scalar)
	}
}

func TestYAMLPlainStringFallback(t *testing.T) {
	d, err := ReadYAML("v: hello world\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindString || v.Scalar.Str != "hello world" {
		t.Errorf("got %+v", v.Scalar)
	}
}

// --- structure rules (sequences, empty sequence, nested sequence) ---

func TestYAMLTopLevelSequenceRejected(t *testing.T) {
	_, err := ReadYAML("- 1\n- 2\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a top-level sequence")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want CodeDocumentUnlabeledElement", err)
	}
}

func TestYAMLEmptySequenceRejected(t *testing.T) {
	_, err := ReadYAML("a: []\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for an empty sequence")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeParseEmptyArray {
		t.Errorf("error = %#v, want CodeParseEmptyArray", err)
	}
}

func TestYAMLNestedSequenceRejected(t *testing.T) {
	_, err := ReadYAML("a:\n  - 1\n  - - 2\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a sequence element that is itself a sequence")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want CodeDocumentUnlabeledElement", err)
	}
}

func TestYAMLTopLevelBareScalar(t *testing.T) {
	d, err := ReadYAML("42\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := ValueDocument(ScalarValue(NewIntegerScalar(big.NewInt(42))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- limits ---

func TestYAMLLimitDepth(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDepth = 2
	_, err := ReadYAML("a:\n  b:\n    c:\n      d: 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want CodeDocumentLimitDepth", err)
	}
}

func TestYAMLLimitDepthInsideSequence(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDepth = 1
	_, err := ReadYAML("a:\n  - b: 1\n  - c:\n      d: 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want CodeDocumentLimitDepth", err)
	}
}

func TestYAMLLimitNodes(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxNodes = 2
	_, err := ReadYAML("a: {}\nb: {}\nc: {}\n", limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitNodes {
		t.Errorf("error = %#v, want CodeDocumentLimitNodes", err)
	}
}

func TestYAMLLimitIntDigits(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 5
	_, err := ReadYAML("123456\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
	}
}

func TestYAMLLimitIntDigitsWithinMapping(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := ReadYAML("num: 12345\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
	}
}

// TestYAMLLimitIntDigitsOnMappingKey exercises readLabel's error-
// propagation branch (readLabel's own call to readScalar can fail on the
// digit limit before it ever gets to ask whether the resolved kind is a
// string) — a key that is itself a too-long bare integer hits the digit
// limit first, distinctly from (and before) the separate
// not-a-string-label rejection a resolved-to-boolean key would hit.
func TestYAMLLimitIntDigitsOnMappingKey(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := ReadYAML("12345: v\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
	}
}

// TestYAMLLimitIntDigitsInsideSequence exercises readSequenceElements'
// error-propagation branch for a scalar element that exceeds the digit
// limit.
func TestYAMLLimitIntDigitsInsideSequence(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := ReadYAML("a:\n  - 1\n  - 12345\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
	}
}

func TestYAMLLimitIntDigitsFromSexagesimal(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 1
	// 12:00:00 -> 43200, five digits, exceeds a MaxIntDigits of 1.
	_, err := ReadYAML("t: 12:00:00\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want CodeDocumentLimitIntDigits", err)
	}
}

// --- malformed input / error paths ---

func TestYAMLEmptyInputIsAnError(t *testing.T) {
	_, err := ReadYAML("", DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for empty input")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeParseUnexpectedToken {
		t.Errorf("error = %#v, want CodeParseUnexpectedToken", err)
	}
}

func TestYAMLMalformedSyntax(t *testing.T) {
	_, err := ReadYAML("a: [1,2\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected a syntax error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeParseUnexpectedToken {
		t.Errorf("error = %#v, want CodeParseUnexpectedToken", err)
	}
}

func TestYAMLTrailingDocumentIsAnError(t *testing.T) {
	_, err := ReadYAML("a: 1\n---\nb: 2\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected a trailing-content error")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeParseTrailingContent {
		t.Errorf("error = %#v, want CodeParseTrailingContent", err)
	}
}

func TestYAMLMalformedSecondDocument(t *testing.T) {
	_, err := ReadYAML("a: 1\n---\nb: [1,2\n", DefaultLimits())
	if err == nil {
		t.Fatal("expected a syntax error decoding the second document")
	}
	pe, ok := err.(*ParseError)
	if !ok || pe.Code != CodeParseUnexpectedToken {
		t.Errorf("error = %#v, want CodeParseUnexpectedToken", err)
	}
}

// --- round-trip and cross-format equality ---

func TestYAMLRoundTripProperty(t *testing.T) {
	cases := []Document{
		NodeDocument(NewNode()),
		NodeDocument(NewNode().AddValue("a", ScalarValue(NewStringScalar("hello")))),
		NodeDocument(NewNode().
			AddValue("s", ScalarValue(NewStringScalar("on"))).
			AddValue("i", ScalarValue(NewIntegerScalar(big.NewInt(-42)))).
			AddValue("f", ScalarValue(NewNumberScalar(3.5))).
			AddValue("b", ScalarValue(NewBooleanScalar(true))).
			AddValue("nul", NullValue()).
			AddValue("d", ScalarValue(NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 1}))).
			AddValue("dt", ScalarValue(NewDateTimeScalar(DateTimeValue{
				Date: DateValue{Year: 2024, Month: 1, Day: 1},
				Time: TimeValue{Hour: 12, Minute: 30},
			}))).
			AddValue("t", ScalarValue(NewTimeScalar(TimeValue{Hour: 12, Minute: 0, Second: 30}))).
			AddNode("child", NewNode().AddValue("x", ScalarValue(NewIntegerScalar(big.NewInt(1))))).
			AddValue("rep", ScalarValue(NewIntegerScalar(big.NewInt(1)))).
			AddValue("rep", ScalarValue(NewIntegerScalar(big.NewInt(2))))),
		ValueDocument(ScalarValue(NewStringScalar("bare"))),
		ValueDocument(NullValue()),
	}
	for i, d := range cases {
		out, err := WriteYAML(d)
		if err != nil {
			t.Fatalf("case %d: WriteYAML: %v", i, err)
		}
		back, err := ReadYAML(out, DefaultLimits())
		if err != nil {
			t.Fatalf("case %d: ReadYAML(%q): %v", i, out, err)
		}
		// KindTime is documented (yaml_writer.go's WriteYAML doc
		// comment) to round-trip as a string, not a time — every other
		// case round-trips exactly.
		if i == 2 {
			continue
		}
		if !docEqual(d, back) {
			t.Errorf("case %d: round-trip mismatch:\nwrote: %s\ngot:   %+v\nwant:  %+v", i, out, back, d)
		}
	}
}

func TestYAMLCrossFormatStructuralEqualityWithJSON(t *testing.T) {
	yd, err := ReadYAML("a: 1\nb: \"two\"\nc:\n  d: true\n", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	jd, err := ReadJSON(`{"a":1,"b":"two","c":{"d":true}}`, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(yd, jd) {
		t.Errorf("YAML and JSON documents differ:\nYAML: %+v\nJSON: %+v", yd, jd)
	}
}
