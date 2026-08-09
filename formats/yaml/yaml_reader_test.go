package yaml

import (
	"math"
	"math/big"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- model mapping table (docs/formats/yaml.md) ---

func TestYAMLModelMappingTable(t *testing.T) {
	d, err := Read("a: 1\nb: 2\n", omnist.DefaultLimits())
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

func TestYAMLRepeatedLabelFromSequence(t *testing.T) {
	d, err := Read("m:\n  - A\n  - B\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B"))))
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

// TestYAMLWorkedExampleStatusNo confirms the worked example's first
// YAML-only note: an unquoted "no" resolves to the boolean false, not the
// string "no" JSON's equivalent scalar would produce — a genuine,
// spec-mandated stage-1 divergence between the two formats for the "same"
// source value.
func TestYAMLWorkedExampleStatusNo(t *testing.T) {
	d, err := Read("status: no\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("status", omnist.ScalarValue(omnist.NewBooleanScalar(false))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// TestYAMLWorkedExamplePlacedDate confirms the worked example's second
// YAML-only note: a bare ISO date resolves to a date-kind omnist.Scalar at stage
// 1, where JSON would hand stage 2 a string to upgrade.
func TestYAMLWorkedExamplePlacedDate(t *testing.T) {
	d, err := Read("placed: 2024-01-01\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("placed", omnist.ScalarValue(omnist.NewDateScalar(omnist.DateValue{Year: 2024, Month: 1, Day: 1}))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- aliases (docs/formats/yaml.md: "Aliases resolve at parse time") ---

func TestYAMLAliasExpandsToIndependentCopies(t *testing.T) {
	d, err := Read("a: &x foo\nb: *x\n", omnist.DefaultLimits())
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
	// Mutating one omnist.Scalar's string field cannot be observed on the
	// other: Go strings are immutable and omnist.NewStringScalar copies by
	// value, so there is no shared backing to mutate through in the
	// first place — this assertion documents that guarantee rather than
	// exercising a mutation (there is no mutator to call), matching the
	// issue's instruction to "write the test to be robust against this
	// [identity sharing] even if types are currently immutable." A
	// pointer-identity check on the two Scalars' Str fields would be
	// meaningless (Go string headers may or may not share a backing
	// array for equal literals; that is not what "shared identity" in
	// the omnist.Document model means). What IS observable and load-bearing is
	// that the two omnist.Node pointers under a and b's aliased mapping value
	// (had the anchor been a mapping, not a scalar) are never the same
	// pointer — TestYAMLAliasedMappingExpandsToIndependentNodes below
	// covers that directly.
	if av.Scalar.Str != bv.Scalar.Str {
		t.Fatalf("expected equal values, got %q vs %q", av.Scalar.Str, bv.Scalar.Str)
	}
}

// TestYAMLAliasedMappingExpandsToIndependentNodes is the structural half
// of the aliasing guarantee: when the anchored value is itself a mapping,
// each alias occurrence must build its own *omnist.Node, not share one — the
// omnist.Document model (issue #1) has no notion of node identity, so a reader
// that reused a single *omnist.Node pointer across both edges would make an
// identity-sensitive bug (e.g. a future caller mutating one edge's node
// in place) silently corrupt the other, unobservable today only because
// nothing currently mutates a *omnist.Node after ReadYAML returns.
func TestYAMLAliasedMappingExpandsToIndependentNodes(t *testing.T) {
	d, err := Read("a: &x {k: v}\nb: *x\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	na, _ := d.Node.Edges[0].Target.Node()
	nb, _ := d.Node.Edges[1].Target.Node()
	if na == nb {
		t.Fatal("aliased mapping edges share the same *omnist.Node pointer; must be independent copies")
	}
	if !nodeEqual(na, nb) {
		t.Fatalf("aliased mapping edges have different content: %+v vs %+v", na, nb)
	}
}

// --- sexagesimal sharp edge ---

func TestYAMLSexagesimalTimeResolvesToInteger(t *testing.T) {
	d, err := Read("t: 12:00:00\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindInteger {
		t.Fatalf("kind = %v, want integer", v.Scalar.Kind)
	}
	want := big.NewInt(12*3600 + 0*60 + 0)
	if v.Scalar.Int.Cmp(want) != 0 {
		t.Errorf("value = %s, want %s", v.Scalar.Int, want)
	}
}

func TestYAMLSexagesimalIntegerNegative(t *testing.T) {
	d, err := Read("t: -1:02\n", omnist.DefaultLimits())
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
	_, err := Read("on: true\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a boolean-resolved mapping key")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error type = %T, want *omnist.ParseError", err)
	}
	if pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Errorf("code = %v, want %v", pe.Code, omnist.CodeDocumentUnlabeledElement)
	}
}

func TestYAMLNorwayProblemQuotedOnKeySucceeds(t *testing.T) {
	d, err := Read("\"on\": true\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("quoted \"on\" key should read cleanly, got error: %v", err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("on", omnist.ScalarValue(omnist.NewBooleanScalar(true))))
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
		d, err := Read("v: "+c.text+"\n", omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", c.text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != omnist.KindBoolean || v.Scalar.Bool != c.want {
			t.Errorf("%q: got %+v, want boolean %v", c.text, v.Scalar, c.want)
		}
	}
}

func TestYAMLQuotedNorwayWordStaysString(t *testing.T) {
	d, err := Read(`v: "no"`+"\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindString || v.Scalar.Str != "no" {
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
		d, err := Read(src, omnist.DefaultLimits())
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
	_, err := Read("~: value\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a null-resolved mapping key")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want omnist.CodeDocumentUnlabeledElement", err)
	}
}

func TestYAMLComplexMappingKeyRejected(t *testing.T) {
	_, err := Read("? {a: 1}\n: value\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a non-scalar mapping key")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want omnist.CodeDocumentUnlabeledElement", err)
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
		d, err := Read("v: "+c.text+"\n", omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != omnist.KindInteger || v.Scalar.Int.Int64() != c.want {
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
	d, err := Read("v: 0x_\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindString || v.Scalar.Str != "0x_" {
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
		d, err := Read("v: "+c.text+"\n", omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != omnist.KindNumber || v.Scalar.Num != c.want {
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
		d, err := Read("v: "+c.text+"\n", omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", c.text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != omnist.KindNumber || !c.check(v.Scalar.Num) {
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
		d, err := Read("v: "+c+"\n", omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", c, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != omnist.KindDateTime {
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
	d, err := Read("v: 1e400\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindString || v.Scalar.Str != "1e400" {
		t.Errorf("got %+v, want string \"1e400\"", v.Scalar)
	}
}

func TestYAMLPlainStringFallback(t *testing.T) {
	d, err := Read("v: hello world\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindString || v.Scalar.Str != "hello world" {
		t.Errorf("got %+v", v.Scalar)
	}
}

// --- structure rules (sequences, empty sequence, nested sequence) ---

func TestYAMLTopLevelSequenceRejected(t *testing.T) {
	_, err := Read("- 1\n- 2\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a top-level sequence")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want omnist.CodeDocumentUnlabeledElement", err)
	}
}

func TestYAMLEmptySequenceRejected(t *testing.T) {
	_, err := Read("a: []\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for an empty sequence")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseEmptyArray {
		t.Errorf("error = %#v, want omnist.CodeParseEmptyArray", err)
	}
}

func TestYAMLNestedSequenceRejected(t *testing.T) {
	_, err := Read("a:\n  - 1\n  - - 2\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for a sequence element that is itself a sequence")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Errorf("error = %#v, want omnist.CodeDocumentUnlabeledElement", err)
	}
}

func TestYAMLTopLevelBareScalar(t *testing.T) {
	d, err := Read("42\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.ValueDocument(omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(42))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// --- limits ---

func TestYAMLLimitDepth(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 2
	_, err := Read("a:\n  b:\n    c:\n      d: 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitDepth", err)
	}
}

func TestYAMLLimitDepthInsideSequence(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 1
	_, err := Read("a:\n  - b: 1\n  - c:\n      d: 1\n", limits)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitDepth", err)
	}
}

func TestYAMLLimitNodes(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxNodes = 2
	_, err := Read("a: {}\nb: {}\nc: {}\n", limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitNodes {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitNodes", err)
	}
}

func TestYAMLLimitIntDigits(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 5
	_, err := Read("123456\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

func TestYAMLLimitIntDigitsWithinMapping(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read("num: 12345\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

// TestYAMLLimitIntDigitsOnMappingKey exercises readLabel's error-
// propagation branch (readLabel's own call to readScalar can fail on the
// digit limit before it ever gets to ask whether the resolved kind is a
// string) — a key that is itself a too-long bare integer hits the digit
// limit first, distinctly from (and before) the separate
// not-a-string-label rejection a resolved-to-boolean key would hit.
func TestYAMLLimitIntDigitsOnMappingKey(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read("12345: v\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

// TestYAMLLimitIntDigitsInsideSequence exercises readSequenceElements'
// error-propagation branch for a scalar element that exceeds the digit
// limit.
func TestYAMLLimitIntDigitsInsideSequence(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read("a:\n  - 1\n  - 12345\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

func TestYAMLLimitIntDigitsFromSexagesimal(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 1
	// 12:00:00 -> 43200, five digits, exceeds a MaxIntDigits of 1.
	_, err := Read("t: 12:00:00\n", limits)
	if err == nil {
		t.Fatal("expected int-digits limit error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("error = %#v, want omnist.CodeDocumentLimitIntDigits", err)
	}
}

// --- malformed input / error paths ---

func TestYAMLEmptyInputIsAnError(t *testing.T) {
	_, err := Read("", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error for empty input")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseUnexpectedToken {
		t.Errorf("error = %#v, want omnist.CodeParseUnexpectedToken", err)
	}
}

func TestYAMLMalformedSyntax(t *testing.T) {
	_, err := Read("a: [1,2\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected a syntax error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseUnexpectedToken {
		t.Errorf("error = %#v, want omnist.CodeParseUnexpectedToken", err)
	}
}

func TestYAMLTrailingDocumentIsAnError(t *testing.T) {
	_, err := Read("a: 1\n---\nb: 2\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected a trailing-content error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseTrailingContent {
		t.Errorf("error = %#v, want omnist.CodeParseTrailingContent", err)
	}
}

func TestYAMLMalformedSecondDocument(t *testing.T) {
	_, err := Read("a: 1\n---\nb: [1,2\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected a syntax error decoding the second document")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseUnexpectedToken {
		t.Errorf("error = %#v, want omnist.CodeParseUnexpectedToken", err)
	}
}

// --- round-trip and cross-format equality ---

func TestYAMLRoundTripProperty(t *testing.T) {
	cases := []omnist.Document{
		omnist.NodeDocument(omnist.NewNode()),
		omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("hello")))),
		omnist.NodeDocument(omnist.NewNode().
			AddValue("s", omnist.ScalarValue(omnist.NewStringScalar("on"))).
			AddValue("i", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(-42)))).
			AddValue("f", omnist.ScalarValue(omnist.NewNumberScalar(3.5))).
			AddValue("b", omnist.ScalarValue(omnist.NewBooleanScalar(true))).
			AddValue("nul", omnist.NullValue()).
			AddValue("d", omnist.ScalarValue(omnist.NewDateScalar(omnist.DateValue{Year: 2024, Month: 1, Day: 1}))).
			AddValue("dt", omnist.ScalarValue(omnist.NewDateTimeScalar(omnist.DateTimeValue{
				Date: omnist.DateValue{Year: 2024, Month: 1, Day: 1},
				Time: omnist.TimeValue{Hour: 12, Minute: 30},
			}))).
			AddValue("t", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 12, Minute: 0, Second: 30}))).
			AddNode("child", omnist.NewNode().AddValue("x", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1))))).
			AddValue("rep", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))).
			AddValue("rep", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(2))))),
		omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("bare"))),
		omnist.ValueDocument(omnist.NullValue()),
	}
	for i, d := range cases {
		out, err := Write(d)
		if err != nil {
			t.Fatalf("case %d: WriteYAML: %v", i, err)
		}
		back, err := Read(out, omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("case %d: Read(%q): %v", i, out, err)
		}
		// omnist.KindTime is documented (yaml_writer.go's WriteYAML doc
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

// --- issue #33: document.unlabeled-element carries a omnist.Document path (spec
// §8.4), never a text-position path ---

func TestNorwayProblemKeyUsesDollarPath(t *testing.T) {
	_, err := Read("on:\n  push: true\n", omnist.DefaultLimits())
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

func TestYAMLTopLevelSequenceUsesDollarPath(t *testing.T) {
	_, err := Read("- 1\n- 2\n", omnist.DefaultLimits())
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is not *omnist.ParseError: %T %v", err, err)
	}
	if pe.Path != "$" {
		t.Errorf("path = %q, want %q", pe.Path, "$")
	}
}

func TestYAMLNodeDepthLimitUsesDollarPath(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 1
	_, err := Read("a:\n  b:\n    c: 1\n", limits)
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is not *omnist.ParseError: %T %v", err, err)
	}
	if pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("code = %q, want %q", pe.Code, omnist.CodeDocumentLimitDepth)
	}
	if pe.Path != "$" {
		t.Errorf("path = %q, want %q", pe.Path, "$")
	}
}
