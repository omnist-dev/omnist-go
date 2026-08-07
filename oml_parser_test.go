package omnist

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) Document {
	t.Helper()
	doc, err := ReadOML(src, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadOML(%q) unexpected error: %v", src, err)
	}
	return doc
}

func mustFail(t *testing.T, src string) *ParseError {
	t.Helper()
	_, err := ReadOML(src, DefaultLimits())
	if err == nil {
		t.Fatalf("ReadOML(%q) expected error, got none", src)
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("ReadOML(%q) error is not *ParseError: %T %v", src, err, err)
	}
	return pe
}

func wantCode(t *testing.T, pe *ParseError, code Code) {
	t.Helper()
	if pe.Code != code {
		t.Errorf("code = %q, want %q (err: %v)", pe.Code, code, pe)
	}
}

// --- §4.8 worked examples, verbatim ---

func TestWorkedDateTime(t *testing.T) {
	doc := mustParse(t, `2024-01-01T10:30`)
	if doc.IsNode || doc.Value.IsNull || doc.Value.Scalar.Kind != KindDateTime {
		t.Fatalf("got %+v", doc)
	}
	want := DateTimeValue{Date: DateValue{2024, 1, 1}, Time: TimeValue{Hour: 10, Minute: 30}}
	if doc.Value.Scalar.DateTime != want {
		t.Errorf("got %+v want %+v", doc.Value.Scalar.DateTime, want)
	}
}

func TestWorkedDateThenTrailingIdent(t *testing.T) {
	pe := mustFail(t, `2024-01-01T99`)
	wantCode(t, pe, CodeParseTrailingContent)
}

func TestWorkedRawString(t *testing.T) {
	doc := mustParse(t, `a: 'C:\no\escapes'`)
	edge := doc.Node.Edges[0]
	v, _ := edge.Target.Value()
	if v.Scalar.Str != `C:\no\escapes` {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestWorkedMultilineBasic(t *testing.T) {
	doc := mustParse(t, "a: \"\"\"\nhello\nworld\"\"\"")
	edge := doc.Node.Edges[0]
	v, _ := edge.Target.Value()
	if v.Scalar.Str != "hello\nworld" {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestWorkedMultilineTwoQuoteRuns(t *testing.T) {
	doc := mustParse(t, "a: \"\"\"\nsays \"\"hi\"\" there\"\"\"")
	edge := doc.Node.Edges[0]
	v, _ := edge.Target.Value()
	if v.Scalar.Str != `says ""hi"" there` {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestWorkedMultilineFourClosingQuotes(t *testing.T) {
	pe := mustFail(t, "a: \"\"\"\nx\"\"\"\"")
	wantCode(t, pe, CodeParseUnterminatedString)
}

func TestWorkedNanLabelError(t *testing.T) {
	// nan is a NUMBER token and never reaches label position: the parser
	// sees NUMBER, COLON — trailing content after the scalar-branch nan.
	pe := mustFail(t, `nan: 1`)
	wantCode(t, pe, CodeParseTrailingContent)
}

func TestWorkedQuotedNan(t *testing.T) {
	doc := mustParse(t, `"nan": 1`)
	edge := doc.Node.Edges[0]
	if edge.Label != "nan" {
		t.Errorf("label = %q", edge.Label)
	}
}

func TestWorkedNullAtTopLevel(t *testing.T) {
	pe := mustFail(t, `null: 1`)
	wantCode(t, pe, CodeParseTrailingContent)
}

func TestWorkedNullInsideNode(t *testing.T) {
	pe := mustFail(t, `a: { null: 1 }`)
	wantCode(t, pe, CodeParseReservedWordLabel)
}

func TestWorkedRepeatedTags(t *testing.T) {
	doc := mustParse(t, "tag: \"x\"\ntag: \"y\"")
	if len(doc.Node.Edges) != 2 {
		t.Fatalf("got %d edges", len(doc.Node.Edges))
	}
	v0, _ := doc.Node.Edges[0].Target.Value()
	v1, _ := doc.Node.Edges[1].Target.Value()
	if v0.Scalar.Str != "x" || v1.Scalar.Str != "y" {
		t.Errorf("got %q %q", v0.Scalar.Str, v1.Scalar.Str)
	}
}

func TestWorkedArraySugar(t *testing.T) {
	doc := mustParse(t, `b: [1, 2, 3]`)
	if len(doc.Node.Edges) != 3 {
		t.Fatalf("got %d edges", len(doc.Node.Edges))
	}
	for i, want := range []int64{1, 2, 3} {
		v, _ := doc.Node.Edges[i].Target.Value()
		if v.Scalar.Int.Cmp(big.NewInt(want)) != 0 {
			t.Errorf("edge %d = %v want %d", i, v.Scalar.Int, want)
		}
		if doc.Node.Edges[i].Label != "b" {
			t.Errorf("edge %d label = %q", i, doc.Node.Edges[i].Label)
		}
	}
}

func TestWorkedEmptyArray(t *testing.T) {
	pe := mustFail(t, `a: []`)
	wantCode(t, pe, CodeParseEmptyArray)
}

func TestWorkedEmptyBraces(t *testing.T) {
	doc := mustParse(t, `a: {}`)
	if len(doc.Node.Edges) != 1 {
		t.Fatalf("got %d edges", len(doc.Node.Edges))
	}
	n, ok := doc.Node.Edges[0].Target.Node()
	if !ok || len(n.Edges) != 0 {
		t.Errorf("got %+v", doc.Node.Edges[0].Target)
	}
}

func TestWorkedBareScalar(t *testing.T) {
	doc := mustParse(t, `"hello"`)
	if doc.IsNode {
		t.Fatalf("expected bare-value document, got node")
	}
	if doc.Value.Scalar.Str != "hello" {
		t.Errorf("got %q", doc.Value.Scalar.Str)
	}
}

// --- tokenizer priority-order edge cases ---

func TestPriorityDateVsDateTimeTrailingIdent(t *testing.T) {
	doc := mustParse(t, `2024-01-01`)
	if doc.Value.Scalar.Kind != KindDate {
		t.Fatalf("got kind %v", doc.Value.Scalar.Kind)
	}
}

func TestPriorityNanBecomesNumberNotIdent(t *testing.T) {
	// Confirms nan never reaches label position even mid-document.
	pe := mustFail(t, `a: { nan: 1 }`)
	// nan tokenizes as NUMBER; inside braces label-position expects
	// STRING/IDENT, so NUMBER where a label is expected is unexpected-token.
	wantCode(t, pe, CodeParseUnexpectedToken)
}

// --- string forms ---

func TestDQuoteEscapes(t *testing.T) {
	doc := mustParse(t, `a: "\"\\\/\b\f\n\r\t"`)
	v, _ := doc.Node.Edges[0].Target.Value()
	want := "\"\\/\b\f\n\r\t"
	if v.Scalar.Str != want {
		t.Errorf("got %q want %q", v.Scalar.Str, want)
	}
}

func TestDQuoteUnicodeEscape(t *testing.T) {
	doc := mustParse(t, `a: "\u00e9"`)
	v, _ := doc.Node.Edges[0].Target.Value()
	if v.Scalar.Str != "é" {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestDQuoteSurrogatePair(t *testing.T) {
	// U+1F600 GRINNING FACE, as a surrogate pair.
	doc := mustParse(t, `a: "\ud83d\ude00"`)
	v, _ := doc.Node.Edges[0].Target.Value()
	if v.Scalar.Str != "😀" {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestDQuoteUnpairedHighSurrogate(t *testing.T) {
	pe := mustFail(t, `a: "\ud83d"`)
	wantCode(t, pe, CodeParseUnpairedSurrogate)
}

func TestDQuoteHighSurrogateFollowedByNonLowSurrogate(t *testing.T) {
	pe := mustFail(t, `a: "\ud83d\u0041"`)
	wantCode(t, pe, CodeParseUnpairedSurrogate)
}

func TestDQuoteLoneLowSurrogate(t *testing.T) {
	pe := mustFail(t, `a: "\udc00"`)
	wantCode(t, pe, CodeParseUnpairedSurrogate)
}

func TestDQuoteInvalidEscape(t *testing.T) {
	pe := mustFail(t, `a: "\x"`)
	wantCode(t, pe, CodeParseInvalidEscape)
}

func TestDQuoteInvalidHexEscape(t *testing.T) {
	pe := mustFail(t, `a: "\uZZZZ"`)
	wantCode(t, pe, CodeParseInvalidEscape)
}

func TestDQuoteTruncatedHexEscape(t *testing.T) {
	pe := mustFail(t, `a: "\u12`)
	wantCode(t, pe, CodeParseUnterminatedString)
}

func TestDQuoteControlCharacter(t *testing.T) {
	pe := mustFail(t, "a: \"x\ty\"")
	wantCode(t, pe, CodeParseControlCharacter)
}

func TestDQuoteUnterminated(t *testing.T) {
	pe := mustFail(t, `a: "abc`)
	wantCode(t, pe, CodeParseUnterminatedString)
}

func TestDQuoteUnterminatedAfterBackslash(t *testing.T) {
	pe := mustFail(t, `a: "abc\`)
	wantCode(t, pe, CodeParseUnterminatedString)
}

func TestRawStringLiteralBackslash(t *testing.T) {
	doc := mustParse(t, `a: 'a\b'`)
	v, _ := doc.Node.Edges[0].Target.Value()
	if v.Scalar.Str != `a\b` {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestRawStringUnterminated(t *testing.T) {
	pe := mustFail(t, `a: 'abc`)
	wantCode(t, pe, CodeParseUnterminatedString)
}

func TestMultilineLeadingCRLFStripped(t *testing.T) {
	doc := mustParse(t, "a: \"\"\"\r\nhi\"\"\"")
	v, _ := doc.Node.Edges[0].Target.Value()
	if v.Scalar.Str != "hi" {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestMultilineNoLeadingNewline(t *testing.T) {
	doc := mustParse(t, `a: """hi"""`)
	v, _ := doc.Node.Edges[0].Target.Value()
	if v.Scalar.Str != "hi" {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestMultilineSingleQuoteLiteral(t *testing.T) {
	doc := mustParse(t, `a: """x"y"""`)
	v, _ := doc.Node.Edges[0].Target.Value()
	if v.Scalar.Str != `x"y` {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestMultilineTabAllowed(t *testing.T) {
	doc := mustParse(t, "a: \"\"\"x\ty\"\"\"")
	v, _ := doc.Node.Edges[0].Target.Value()
	if v.Scalar.Str != "x\ty" {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

func TestMultilineControlCharacter(t *testing.T) {
	pe := mustFail(t, "a: \"\"\"x\x01y\"\"\"")
	wantCode(t, pe, CodeParseControlCharacter)
}

func TestMultilineUnterminated(t *testing.T) {
	pe := mustFail(t, `a: """abc`)
	wantCode(t, pe, CodeParseUnterminatedString)
}

func TestMultilineFiveClosingQuotesLeavesTwoLiteralQuotes(t *testing.T) {
	// first 3 close it; the remaining 2 quotes form a new (empty,
	// well-formed) dquote-string token, which then has no separator
	// before it -> "missing separator between edges", not unterminated.
	pe := mustFail(t, "a: \"\"\"x\"\"\"\"\"")
	wantCode(t, pe, CodeParseUnexpectedToken)
}

// --- arrays ---

func TestArrayNestedError(t *testing.T) {
	pe := mustFail(t, `a: [[1]]`)
	wantCode(t, pe, CodeParseNestedArray)
}

func TestArrayTrailingComma(t *testing.T) {
	doc := mustParse(t, `a: [1, 2,]`)
	if len(doc.Node.Edges) != 2 {
		t.Fatalf("got %d edges", len(doc.Node.Edges))
	}
}

func TestArrayNewlineSeparatorError(t *testing.T) {
	pe := mustFail(t, "a: [1,\n2]")
	wantCode(t, pe, CodeParseSeparatorInArray)
}

func TestArraySemicolonSeparatorError(t *testing.T) {
	pe := mustFail(t, "a: [1;2]")
	wantCode(t, pe, CodeParseSeparatorInArray)
}

func TestArrayOfNodes(t *testing.T) {
	doc := mustParse(t, `a: [{x: 1}, {x: 2}]`)
	if len(doc.Node.Edges) != 2 {
		t.Fatalf("got %d edges", len(doc.Node.Edges))
	}
	n0, ok := doc.Node.Edges[0].Target.Node()
	if !ok || len(n0.Edges) != 1 {
		t.Fatalf("got %+v", doc.Node.Edges[0].Target)
	}
}

func TestArrayMissingCommaError(t *testing.T) {
	pe := mustFail(t, `a: [1 2]`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestArrayCommentFollowedByNewlineStillErrors(t *testing.T) {
	// The comment itself is insignificant, but the newline after it is
	// still a forbidden array separator - comments don't disable that rule.
	pe := mustFail(t, "a: [1, # comment\n2]")
	wantCode(t, pe, CodeParseSeparatorInArray)
}

func TestArrayCommentSameLineAllowed(t *testing.T) {
	doc := mustParse(t, "a: [1, 2] # trailing comment")
	if len(doc.Node.Edges) != 2 {
		t.Fatalf("got %+v", doc.Node.Edges)
	}
}

// --- document shapes / top-level disambiguation ---

func TestEmptyDocument(t *testing.T) {
	doc := mustParse(t, ``)
	if !doc.IsNode || len(doc.Node.Edges) != 0 {
		t.Fatalf("got %+v", doc)
	}
}

func TestEmptyDocumentWhitespaceOnly(t *testing.T) {
	doc := mustParse(t, "  \n  ")
	if !doc.IsNode || len(doc.Node.Edges) != 0 {
		t.Fatalf("got %+v", doc)
	}
}

func TestBareIntegerDocument(t *testing.T) {
	doc := mustParse(t, `42`)
	if doc.Value.Scalar.Int.Cmp(big.NewInt(42)) != 0 {
		t.Errorf("got %v", doc.Value.Scalar.Int)
	}
}

func TestBareNullDocument(t *testing.T) {
	doc := mustParse(t, `null`)
	if !doc.Value.IsNull {
		t.Errorf("got %+v", doc.Value)
	}
}

func TestBareBooleanDocuments(t *testing.T) {
	docT := mustParse(t, `true`)
	if !docT.Value.Scalar.Bool {
		t.Errorf("got %+v", docT.Value)
	}
	docF := mustParse(t, `false`)
	if docF.Value.Scalar.Bool {
		t.Errorf("got %+v", docF.Value)
	}
}

func TestTwoBareScalarsError(t *testing.T) {
	pe := mustFail(t, `1 2`)
	wantCode(t, pe, CodeParseTrailingContent)
}

func TestBareIdentifierError(t *testing.T) {
	pe := mustFail(t, `hello`)
	wantCode(t, pe, CodeParseBareWord)
}

func TestSingleLineCompactForm(t *testing.T) {
	doc := mustParse(t, `name: "Ann"; address: { city: "Zurich"; postcode: "8001" }; tag: "x"; tag: "y"`)
	if len(doc.Node.Edges) != 4 {
		t.Fatalf("got %d edges", len(doc.Node.Edges))
	}
}

func TestMissingSeparatorBetweenEdgesError(t *testing.T) {
	pe := mustFail(t, `a: 1 b: 2`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestNestedNode(t *testing.T) {
	doc := mustParse(t, "name: \"Ann\"\naddress: {\n  city: \"Zurich\"\n  postcode: \"8001\"\n}\ntag: \"x\"\ntag: \"y\"")
	if len(doc.Node.Edges) != 4 {
		t.Fatalf("got %d edges: %+v", len(doc.Node.Edges), doc.Node.Edges)
	}
	addr, ok := doc.Node.Edges[1].Target.Node()
	if !ok || len(addr.Edges) != 2 {
		t.Fatalf("got %+v", doc.Node.Edges[1].Target)
	}
}

func TestCommentHandling(t *testing.T) {
	doc := mustParse(t, "# a comment\nname: \"Ann\" # trailing\n")
	if len(doc.Node.Edges) != 1 {
		t.Fatalf("got %+v", doc.Node.Edges)
	}
}

func TestQuotedLabel(t *testing.T) {
	doc := mustParse(t, `"weird label!": 1`)
	if doc.Node.Edges[0].Label != "weird label!" {
		t.Errorf("got %q", doc.Node.Edges[0].Label)
	}
}

func TestUnclosedBraceError(t *testing.T) {
	pe := mustFail(t, `a: { b: 1`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestUnexpectedTokenAtLabelPosition(t *testing.T) {
	pe := mustFail(t, `a: { 1: 2 }`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestBareIdentifierFollowedByAnotherTokenIsBareWord(t *testing.T) {
	// "a" is not followed by ':', so the top-level lookahead takes the
	// scalar branch; "a" alone is not null/true/false, so it fails as a
	// bare word before "1" is ever reached.
	pe := mustFail(t, `a 1`)
	wantCode(t, pe, CodeParseBareWord)
}

func TestValueIsRBraceError(t *testing.T) {
	pe := mustFail(t, `a: }`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

// --- limits ---

func TestIntDigitLimitBoundaryOK(t *testing.T) {
	digits := strings.Repeat("9", 4300)
	doc := mustParse(t, digits)
	if doc.Value.Scalar.Kind != KindInteger {
		t.Fatalf("got %+v", doc.Value)
	}
}

func TestIntDigitLimitExceeded(t *testing.T) {
	digits := strings.Repeat("9", 4301)
	pe := mustFail(t, digits)
	wantCode(t, pe, CodeDocumentLimitIntDigits)
}

func buildNesting(n int) string {
	var b strings.Builder
	b.WriteString("a: ")
	for i := 0; i < n; i++ {
		b.WriteString("{ a: ")
	}
	b.WriteString("1")
	for i := 0; i < n; i++ {
		b.WriteString(" }")
	}
	return b.String()
}

func TestNestingDepthBoundaryOK(t *testing.T) {
	// 200 '{' levels, per §4.7's boundary table.
	src := buildNesting(200)
	if _, err := ReadOML(src, DefaultLimits()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNestingDepthExceeded(t *testing.T) {
	src := buildNesting(201)
	pe := mustFail(t, src)
	wantCode(t, pe, CodeDocumentLimitDepth)
}

func TestCustomLimits(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxIntDigits = 2
	_, err := ReadOML("123", limits)
	if err == nil {
		t.Fatal("expected error with custom low limit")
	}
	pe := err.(*ParseError)
	wantCode(t, pe, CodeDocumentLimitIntDigits)
}

// --- error paths (line:col) ---

func TestErrorPathLineCol(t *testing.T) {
	pe := mustFail(t, "a: 1\nb: {\n  c 2\n}")
	if pe.Line != 3 {
		t.Errorf("line = %d, want 3", pe.Line)
	}
	if pe.Path != "3:5" {
		t.Errorf("path = %q, want 3:5", pe.Path)
	}
}

func TestErrorPathFirstChar(t *testing.T) {
	pe := mustFail(t, `1 2`)
	if pe.Line != 1 || pe.Col != 3 {
		t.Errorf("got %d:%d", pe.Line, pe.Col)
	}
}

// --- numbers / negative / exponent / reserved floats ---

func TestNegativeInteger(t *testing.T) {
	doc := mustParse(t, `-42`)
	if doc.Value.Scalar.Int.Cmp(big.NewInt(-42)) != 0 {
		t.Errorf("got %v", doc.Value.Scalar.Int)
	}
}

func TestDecimalNumber(t *testing.T) {
	doc := mustParse(t, `3.14`)
	if doc.Value.Scalar.Kind != KindNumber || doc.Value.Scalar.Num != 3.14 {
		t.Errorf("got %+v", doc.Value.Scalar)
	}
}

func TestExponentNumber(t *testing.T) {
	doc := mustParse(t, `1e10`)
	if doc.Value.Scalar.Num != 1e10 {
		t.Errorf("got %v", doc.Value.Scalar.Num)
	}
}

func TestNegativeDecimalWithExponent(t *testing.T) {
	doc := mustParse(t, `-1.5e-3`)
	if doc.Value.Scalar.Num != -1.5e-3 {
		t.Errorf("got %v", doc.Value.Scalar.Num)
	}
}

func TestReservedInfNumber(t *testing.T) {
	doc := mustParse(t, `a: inf`)
	v, _ := doc.Node.Edges[0].Target.Value()
	if !math.IsInf(v.Scalar.Num, 1) {
		t.Errorf("got %v", v.Scalar.Num)
	}
}

func TestReservedNegInfNumber(t *testing.T) {
	doc := mustParse(t, `a: -inf`)
	v, _ := doc.Node.Edges[0].Target.Value()
	if !math.IsInf(v.Scalar.Num, -1) {
		t.Errorf("got %v", v.Scalar.Num)
	}
}

func TestReservedNanNumber(t *testing.T) {
	doc := mustParse(t, `a: nan`)
	v, _ := doc.Node.Edges[0].Target.Value()
	if !math.IsNaN(v.Scalar.Num) {
		t.Errorf("got %v", v.Scalar.Num)
	}
}

// --- date / time / datetime with offsets and fractions ---

func TestTimeWithSecondsAndOffset(t *testing.T) {
	doc := mustParse(t, `10:30:15+02:00`)
	tv := doc.Value.Scalar.Time
	if tv.Hour != 10 || tv.Minute != 30 || tv.Second != 15 || !tv.HasOffset || tv.OffsetSeconds != 7200 {
		t.Errorf("got %+v", tv)
	}
}

func TestTimeWithFraction(t *testing.T) {
	doc := mustParse(t, `10:30:15.5`)
	tv := doc.Value.Scalar.Time
	if tv.Nanosecond != 500000000 {
		t.Errorf("got %d", tv.Nanosecond)
	}
}

func TestTimeWithNegativeOffset(t *testing.T) {
	doc := mustParse(t, `10:30-05:30`)
	tv := doc.Value.Scalar.Time
	if !tv.HasOffset || tv.OffsetSeconds != -(5*3600+30*60) {
		t.Errorf("got %+v", tv)
	}
}

func TestDateTimeWithFractionAndOffset(t *testing.T) {
	doc := mustParse(t, `2024-06-15T10:30:15.123456+00:00`)
	dt := doc.Value.Scalar.DateTime
	if dt.Date != (DateValue{2024, 6, 15}) {
		t.Errorf("date = %+v", dt.Date)
	}
	if dt.Time.Second != 15 || dt.Time.Nanosecond != 123456000 || !dt.Time.HasOffset {
		t.Errorf("time = %+v", dt.Time)
	}
}

// --- reserved-word scalar values are fine (only label position is special) ---

func TestReservedWordsAsValuesFine(t *testing.T) {
	doc := mustParse(t, `a: { true_val: true; false_val: false; null_val: null }`)
	n := doc.Node.Edges[0]
	node, _ := n.Target.Node()
	if len(node.Edges) != 3 {
		t.Fatalf("got %+v", node.Edges)
	}
}

func TestIdentLabelWithHyphenUnderscore(t *testing.T) {
	doc := mustParse(t, `my-label_1: 1`)
	if doc.Node.Edges[0].Label != "my-label_1" {
		t.Errorf("got %q", doc.Node.Edges[0].Label)
	}
}

// --- coverage: separator edge cases (lone CR, CR at EOF) ---

func TestLoneCRAsSeparator(t *testing.T) {
	doc := mustParse(t, "a: 1\rb: 2")
	if len(doc.Node.Edges) != 2 {
		t.Fatalf("got %+v", doc.Node.Edges)
	}
}

func TestLoneCRAtEOF(t *testing.T) {
	doc := mustParse(t, "a: 1\r")
	if len(doc.Node.Edges) != 1 {
		t.Fatalf("got %+v", doc.Node.Edges)
	}
}

// --- coverage: genuinely unrecognized character ---

func TestUnrecognizedCharacter(t *testing.T) {
	pe := mustFail(t, `$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

// --- coverage: surrogate-pair second escape has invalid hex ---

func TestDQuoteHighSurrogateThenInvalidHex(t *testing.T) {
	pe := mustFail(t, `a: "\ud83d\uZZZZ"`)
	wantCode(t, pe, CodeParseInvalidEscape)
}

// --- coverage: looksLikeEdgeStart's swallowed lookahead-lex-error path ---

func TestLookaheadTokenMalformedFallsBackToScalarBareWord(t *testing.T) {
	// "a" starts a label/scalar; the lookahead token right after it ("$")
	// is unlexable, so looksLikeEdgeStart must not propagate that as the
	// reported error - it falls back to the scalar branch, which then
	// fails as a bare word without ever needing to retokenize "$".
	pe := mustFail(t, `a$`)
	wantCode(t, pe, CodeParseBareWord)
}

// --- coverage: p.advance() surfacing a lex error for the token that
// immediately follows an already-consumed token, at every call site that
// checks it. ---

func TestAdvanceErrorAfterStringLabel(t *testing.T) {
	pe := mustFail(t, "x: 1; \"a\"$: 2")
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterIdentLabel(t *testing.T) {
	pe := mustFail(t, "x: 1; a$: 2")
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterOpenBrace(t *testing.T) {
	pe := mustFail(t, `a: {$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterCloseBrace(t *testing.T) {
	pe := mustFail(t, `a: {}$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterStringValue(t *testing.T) {
	pe := mustFail(t, `a: "x"$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterIntegerValue(t *testing.T) {
	pe := mustFail(t, `a: 1$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterNumberValue(t *testing.T) {
	pe := mustFail(t, `a: 1.5$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterDateValue(t *testing.T) {
	pe := mustFail(t, `a: 2024-01-01$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterTimeValue(t *testing.T) {
	pe := mustFail(t, `a: 10:30$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterDateTimeValue(t *testing.T) {
	pe := mustFail(t, `a: 2024-01-01T10:30$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterNullValue(t *testing.T) {
	pe := mustFail(t, `a: null$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterTrueValue(t *testing.T) {
	pe := mustFail(t, `a: true$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterFalseValue(t *testing.T) {
	pe := mustFail(t, `a: false$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterOpenBracket(t *testing.T) {
	pe := mustFail(t, `a: [$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterComma(t *testing.T) {
	pe := mustFail(t, `a: [1,$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterTrailingCommaCloseBracket(t *testing.T) {
	pe := mustFail(t, `a: [1,]$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterCloseBracket(t *testing.T) {
	pe := mustFail(t, `a: [1]$`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

// --- coverage: CRLF as a single separator ---

func TestCRLFSeparator(t *testing.T) {
	doc := mustParse(t, "a: 1\r\nb: 2")
	if len(doc.Node.Edges) != 2 {
		t.Fatalf("got %+v", doc.Node.Edges)
	}
}

// --- coverage: bare array at top level (not in edge-value position) ---

func TestBareArrayAtTopLevelIsUnexpectedToken(t *testing.T) {
	pe := mustFail(t, `[1, 2]`)
	wantCode(t, pe, CodeParseUnexpectedToken)
}

// --- coverage: separator immediately after '[', and a malformed element ---

func TestArraySeparatorRightAfterOpenBracket(t *testing.T) {
	pe := mustFail(t, "a: [\n1]")
	wantCode(t, pe, CodeParseSeparatorInArray)
}

func TestArrayElementBareWordError(t *testing.T) {
	pe := mustFail(t, `a: [x]`)
	wantCode(t, pe, CodeParseBareWord)
}
