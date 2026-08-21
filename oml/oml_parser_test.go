package oml

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

func mustParse(t *testing.T, src string) omnist.Document {
	t.Helper()
	doc, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("Read(%q) unexpected error: %v", src, err)
	}
	return doc
}

func mustFail(t *testing.T, src string) *omnist.ParseError {
	t.Helper()
	_, err := Read(src, omnist.DefaultLimits())
	if err == nil {
		t.Fatalf("Read(%q) expected error, got none", src)
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("Read(%q) error is not *omnist.ParseError: %T %v", src, err, err)
	}
	return pe
}

func wantCode(t *testing.T, pe *omnist.ParseError, code omnist.Code) {
	t.Helper()
	if pe.Code != code {
		t.Errorf("code = %q, want %q (err: %v)", pe.Code, code, pe)
	}
}

// --- §4.8 worked examples, verbatim ---

func TestWorkedDateTime(t *testing.T) {
	doc := mustParse(t, `2024-01-01T10:30`)
	if doc.IsNode || doc.Value.IsNull || doc.Value.Scalar.Kind != omnist.KindDateTime {
		t.Fatalf("got %+v", doc)
	}
	want := omnist.DateTimeValue{Date: omnist.DateValue{Year: 2024, Month: 1, Day: 1}, Time: omnist.TimeValue{Hour: 10, Minute: 30}}
	if doc.Value.Scalar.DateTime != want {
		t.Errorf("got %+v want %+v", doc.Value.Scalar.DateTime, want)
	}
}

func TestWorkedDateThenTrailingIdent(t *testing.T) {
	pe := mustFail(t, `2024-01-01T99`)
	wantCode(t, pe, omnist.CodeParseTrailingContent)
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
	wantCode(t, pe, omnist.CodeParseUnterminatedString)
}

func TestWorkedNanLabelError(t *testing.T) {
	// nan is a NUMBER token and never reaches label position: the parser
	// sees NUMBER, COLON. Per
	// oml-grammar/reserved/nan-bare-is-a-number-token-not-a-label, this is
	// parse.unexpected-token, not parse.trailing-content — unlike the
	// analogous null/true/false case (TestWorkedNullAtTopLevel below), a
	// leftover ':' after a NUMBER was never a candidate for label
	// position at all, so it is simply out of place rather than a
	// continuation of an almost-valid construct.
	pe := mustFail(t, `nan: 1`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
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
	wantCode(t, pe, omnist.CodeParseTrailingContent)
}

func TestWorkedNullInsideNode(t *testing.T) {
	pe := mustFail(t, `a: { null: 1 }`)
	wantCode(t, pe, omnist.CodeParseReservedWordLabel)
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
	wantCode(t, pe, omnist.CodeParseEmptyArray)
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
	if doc.Value.Scalar.Kind != omnist.KindDate {
		t.Fatalf("got kind %v", doc.Value.Scalar.Kind)
	}
}

func TestPriorityNanBecomesNumberNotIdent(t *testing.T) {
	// Confirms nan never reaches label position even mid-document.
	pe := mustFail(t, `a: { nan: 1 }`)
	// nan tokenizes as NUMBER; inside braces label-position expects
	// STRING/IDENT, so NUMBER where a label is expected is unexpected-token.
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
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
	wantCode(t, pe, omnist.CodeParseUnpairedSurrogate)
}

func TestDQuoteHighSurrogateFollowedByNonLowSurrogate(t *testing.T) {
	pe := mustFail(t, `a: "\ud83d\u0041"`)
	wantCode(t, pe, omnist.CodeParseUnpairedSurrogate)
}

func TestDQuoteLoneLowSurrogate(t *testing.T) {
	pe := mustFail(t, `a: "\udc00"`)
	wantCode(t, pe, omnist.CodeParseUnpairedSurrogate)
}

func TestDQuoteInvalidEscape(t *testing.T) {
	pe := mustFail(t, `a: "\x"`)
	wantCode(t, pe, omnist.CodeParseInvalidEscape)
}

func TestDQuoteInvalidHexEscape(t *testing.T) {
	pe := mustFail(t, `a: "\uZZZZ"`)
	wantCode(t, pe, omnist.CodeParseInvalidEscape)
}

func TestDQuoteTruncatedHexEscape(t *testing.T) {
	pe := mustFail(t, `a: "\u12`)
	wantCode(t, pe, omnist.CodeParseUnterminatedString)
}

func TestDQuoteControlCharacter(t *testing.T) {
	pe := mustFail(t, "a: \"x\ty\"")
	wantCode(t, pe, omnist.CodeParseControlCharacter)
}

func TestDQuoteUnterminated(t *testing.T) {
	pe := mustFail(t, `a: "abc`)
	wantCode(t, pe, omnist.CodeParseUnterminatedString)
}

func TestDQuoteUnterminatedAfterBackslash(t *testing.T) {
	pe := mustFail(t, `a: "abc\`)
	wantCode(t, pe, omnist.CodeParseUnterminatedString)
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
	wantCode(t, pe, omnist.CodeParseUnterminatedString)
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
	wantCode(t, pe, omnist.CodeParseControlCharacter)
}

func TestMultilineUnterminated(t *testing.T) {
	pe := mustFail(t, `a: """abc`)
	wantCode(t, pe, omnist.CodeParseUnterminatedString)
}

func TestMultilineFiveClosingQuotesLeavesTwoLiteralQuotes(t *testing.T) {
	// first 3 close it; the remaining 2 quotes form a new (empty,
	// well-formed) dquote-string token, immediately following with no
	// separator before it and nothing (EOF) after it — so, per the same
	// §4.6.1-lookahead reasoning oml-grammar/temporals/
	// date-then-non-time-suffix-is-date-plus-trailing-content pins for an
	// analogous leftover IDENT, this reads as trailing content rather
	// than a missing separator between two edges: the leftover token
	// never goes on to look like a genuine second edge (it isn't followed
	// by ':'), so there's no "edge" it could be missing a separator from.
	pe := mustFail(t, "a: \"\"\"x\"\"\"\"\"")
	wantCode(t, pe, omnist.CodeParseTrailingContent)
}

// --- arrays ---

func TestArrayNestedError(t *testing.T) {
	pe := mustFail(t, `a: [[1]]`)
	wantCode(t, pe, omnist.CodeParseNestedArray)
}

func TestArrayTrailingComma(t *testing.T) {
	doc := mustParse(t, `a: [1, 2,]`)
	if len(doc.Node.Edges) != 2 {
		t.Fatalf("got %d edges", len(doc.Node.Edges))
	}
}

func TestArrayNewlineSeparatorError(t *testing.T) {
	pe := mustFail(t, "a: [1,\n2]")
	wantCode(t, pe, omnist.CodeParseSeparatorInArray)
}

func TestArraySemicolonSeparatorError(t *testing.T) {
	pe := mustFail(t, "a: [1;2]")
	wantCode(t, pe, omnist.CodeParseSeparatorInArray)
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
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestArrayCommentFollowedByNewlineStillErrors(t *testing.T) {
	// The comment itself is insignificant, but the newline after it is
	// still a forbidden array separator - comments don't disable that rule.
	pe := mustFail(t, "a: [1, # comment\n2]")
	wantCode(t, pe, omnist.CodeParseSeparatorInArray)
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
	wantCode(t, pe, omnist.CodeParseTrailingContent)
}

func TestBareIdentifierError(t *testing.T) {
	pe := mustFail(t, `hello`)
	wantCode(t, pe, omnist.CodeParseBareWord)
}

func TestSingleLineCompactForm(t *testing.T) {
	doc := mustParse(t, `name: "Ann"; address: { city: "Zurich"; postcode: "8001" }; tag: "x"; tag: "y"`)
	if len(doc.Node.Edges) != 4 {
		t.Fatalf("got %d edges", len(doc.Node.Edges))
	}
}

func TestMissingSeparatorBetweenEdgesError(t *testing.T) {
	pe := mustFail(t, `a: 1 b: 2`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
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
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestUnexpectedTokenAtLabelPosition(t *testing.T) {
	pe := mustFail(t, `a: { 1: 2 }`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestBareIdentifierFollowedByAnotherTokenIsBareWord(t *testing.T) {
	// "a" is not followed by ':', so the top-level lookahead takes the
	// scalar branch; "a" alone is not null/true/false, so it fails as a
	// bare word before "1" is ever reached.
	pe := mustFail(t, `a 1`)
	wantCode(t, pe, omnist.CodeParseBareWord)
}

func TestValueIsRBraceError(t *testing.T) {
	pe := mustFail(t, `a: }`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

// --- limits ---

func TestIntDigitLimitBoundaryOK(t *testing.T) {
	digits := strings.Repeat("9", 4300)
	doc := mustParse(t, digits)
	if doc.Value.Scalar.Kind != omnist.KindInteger {
		t.Fatalf("got %+v", doc.Value)
	}
}

func TestIntDigitLimitExceeded(t *testing.T) {
	digits := strings.Repeat("9", 4301)
	pe := mustFail(t, digits)
	wantCode(t, pe, omnist.CodeDocumentLimitIntDigits)
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
	// Depth is counted from the Document root (spec §2.4): the implicit
	// top-level node is level 1, so 199 explicit '{' levels plus the root
	// is exactly the default 200-level limit, per §4.7's boundary table.
	src := buildNesting(199)
	if _, err := Read(src, omnist.DefaultLimits()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNestingDepthExceeded(t *testing.T) {
	// 200 explicit '{' levels plus the root is 201 -- one past the limit.
	src := buildNesting(200)
	pe := mustFail(t, src)
	wantCode(t, pe, omnist.CodeDocumentLimitDepth)
}

// TestRootNodeCountsTowardDepthAndNodeLimits pins the off-by-one fix
// directly (beyond the boundary tests above, which only exercise it
// indirectly through the default 200-level limit): the implicit
// top-level node itself must count as one level of depth and one node,
// matching spec §2.4's "counted from the Document root". Regression
// vectors: document-model/limits/{depth,node-count}-one-past-declared-
// limit-fails.
func TestRootNodeCountsTowardDepthAndNodeLimits(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 3
	if _, err := Read("a: { b: { c: 1 } }", limits); err != nil {
		t.Fatalf("depth exactly at limit: unexpected error: %v", err)
	}
	pe := mustFailWithLimits(t, "a: { b: { c: { d: 1 } } }", limits)
	wantCode(t, pe, omnist.CodeDocumentLimitDepth)

	nodeLimits := omnist.DefaultLimits()
	nodeLimits.MaxNodes = 1
	pe = mustFailWithLimits(t, "a: { b: 1 }", nodeLimits)
	wantCode(t, pe, omnist.CodeDocumentLimitNodes)
}

// TestRootNodeItselfExceedsDepthLimit exercises parseDocument's EnterNode
// error branch directly: with MaxDepth=0, even the implicit top-level
// node (before any explicit '{') already exceeds the limit.
func TestRootNodeItselfExceedsDepthLimit(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 0
	pe := mustFailWithLimits(t, "a: 1", limits)
	wantCode(t, pe, omnist.CodeDocumentLimitDepth)
}

func mustFailWithLimits(t *testing.T, src string, limits omnist.Limits) *omnist.ParseError {
	t.Helper()
	_, err := Read(src, limits)
	if err == nil {
		t.Fatalf("Read(%q) expected error, got none", src)
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("Read(%q) error is not *omnist.ParseError: %T %v", src, err, err)
	}
	return pe
}

func TestCustomLimits(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 2
	_, err := Read("123", limits)
	if err == nil {
		t.Fatal("expected error with custom low limit")
	}
	pe := err.(*omnist.ParseError)
	wantCode(t, pe, omnist.CodeDocumentLimitIntDigits)
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
	if doc.Value.Scalar.Kind != omnist.KindNumber || doc.Value.Scalar.Num != 3.14 {
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
	if dt.Date != (omnist.DateValue{Year: 2024, Month: 6, Day: 15}) {
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
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

// --- coverage: surrogate-pair second escape has invalid hex ---

func TestDQuoteHighSurrogateThenInvalidHex(t *testing.T) {
	pe := mustFail(t, `a: "\ud83d\uZZZZ"`)
	wantCode(t, pe, omnist.CodeParseInvalidEscape)
}

// --- coverage: looksLikeEdgeStart's swallowed lookahead-lex-error path ---

func TestLookaheadTokenMalformedFallsBackToScalarBareWord(t *testing.T) {
	// "a" starts a label/scalar; the lookahead token right after it ("$")
	// is unlexable, so looksLikeEdgeStart must not propagate that as the
	// reported error - it falls back to the scalar branch, which then
	// fails as a bare word without ever needing to retokenize "$".
	pe := mustFail(t, `a$`)
	wantCode(t, pe, omnist.CodeParseBareWord)
}

// --- coverage: p.advance() surfacing a lex error for the token that
// immediately follows an already-consumed token, at every call site that
// checks it. ---

func TestAdvanceErrorAfterStringLabel(t *testing.T) {
	pe := mustFail(t, "x: 1; \"a\"$: 2")
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterIdentLabel(t *testing.T) {
	pe := mustFail(t, "x: 1; a$: 2")
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterOpenBrace(t *testing.T) {
	pe := mustFail(t, `a: {$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterCloseBrace(t *testing.T) {
	pe := mustFail(t, `a: {}$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterStringValue(t *testing.T) {
	pe := mustFail(t, `a: "x"$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterIntegerValue(t *testing.T) {
	pe := mustFail(t, `a: 1$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterNumberValue(t *testing.T) {
	pe := mustFail(t, `a: 1.5$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterDateValue(t *testing.T) {
	pe := mustFail(t, `a: 2024-01-01$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterTimeValue(t *testing.T) {
	pe := mustFail(t, `a: 10:30$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterDateTimeValue(t *testing.T) {
	pe := mustFail(t, `a: 2024-01-01T10:30$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterNullValue(t *testing.T) {
	pe := mustFail(t, `a: null$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterTrueValue(t *testing.T) {
	pe := mustFail(t, `a: true$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterFalseValue(t *testing.T) {
	pe := mustFail(t, `a: false$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterOpenBracket(t *testing.T) {
	pe := mustFail(t, `a: [$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterComma(t *testing.T) {
	pe := mustFail(t, `a: [1,$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterTrailingCommaCloseBracket(t *testing.T) {
	pe := mustFail(t, `a: [1,]$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

func TestAdvanceErrorAfterCloseBracket(t *testing.T) {
	pe := mustFail(t, `a: [1]$`)
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
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
	wantCode(t, pe, omnist.CodeParseUnexpectedToken)
}

// --- coverage: separator immediately after '[', and a malformed element ---

func TestArraySeparatorRightAfterOpenBracket(t *testing.T) {
	pe := mustFail(t, "a: [\n1]")
	wantCode(t, pe, omnist.CodeParseSeparatorInArray)
}

func TestArrayElementBareWordError(t *testing.T) {
	pe := mustFail(t, `a: [x]`)
	wantCode(t, pe, omnist.CodeParseBareWord)
}

// --- issue #33: string-literal error positions anchor to the opening
// quote, not the byte that triggered the error ---

func TestControlCharacterInStringAnchorsToOpeningQuote(t *testing.T) {
	// a: "hi\nthere" -- the literal newline inside the string is the
	// control character; the diagnostic anchors to the opening '"' (col
	// 4), per oml-grammar/errors/literal-control-character-in-string-is-an-error.
	pe := mustFail(t, "a: \"hi\nthere\"\n")
	wantCode(t, pe, omnist.CodeParseControlCharacter)
	if pe.Line != 1 || pe.Col != 4 {
		t.Errorf("got %d:%d, want 1:4", pe.Line, pe.Col)
	}
}

func TestUnrecognizedEscapeAnchorsToOpeningQuote(t *testing.T) {
	pe := mustFail(t, `a: "\q"`)
	wantCode(t, pe, omnist.CodeParseInvalidEscape)
	if pe.Line != 1 || pe.Col != 4 {
		t.Errorf("got %d:%d, want 1:4", pe.Line, pe.Col)
	}
}

func TestUnpairedHighSurrogateAnchorsToOpeningQuote(t *testing.T) {
	pe := mustFail(t, `a: "\ud800"`)
	wantCode(t, pe, omnist.CodeParseUnpairedSurrogate)
	if pe.Line != 1 || pe.Col != 4 {
		t.Errorf("got %d:%d, want 1:4", pe.Line, pe.Col)
	}
}

// --- issue #33: a newline inside an array reports the position just past
// the newline, not the newline character's own column ---

func TestNewlineInsideArrayReportsPositionAfterNewline(t *testing.T) {
	pe := mustFail(t, "b: [1\n2]\n")
	wantCode(t, pe, omnist.CodeParseSeparatorInArray)
	if pe.Line != 2 || pe.Col != 1 {
		t.Errorf("got %d:%d, want 2:1", pe.Line, pe.Col)
	}
}

// --- issue #33: date-then-non-time-suffix is DATE + trailing content,
// not a missing-separator error ---

func TestDateThenNonTimeSuffixIsTrailingContent(t *testing.T) {
	pe := mustFail(t, "a: 2024-01-01T99\n")
	wantCode(t, pe, omnist.CodeParseTrailingContent)
	if pe.Line != 1 || pe.Col != 14 {
		t.Errorf("got %d:%d, want 1:14", pe.Line, pe.Col)
	}
}

// --- issue #33: document.limit.depth/nodes diagnostics carry the "$"
// omnist.Document-path fallback (spec §8.4), never a text-position path ---

func TestBracedNodeDepthLimitUsesDollarPath(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxDepth = 1
	_, err := Read(`a: { b: { c: 1 } }`, limits)
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

// --- issue #33: document.limit.int-digits carries the omnist.Document path of
// the offending edge, e.g. "$.n", not a text-position path ---

func TestIntDigitsLimitUsesDocumentPath(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read("n: 1000\n", limits)
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

// --- issue #33: an in-bounds int-digits value nested inside a brace still
// gets a nested omnist.Document path ---

func TestIntDigitsLimitUsesNestedDocumentPath(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 3
	_, err := Read("a: { n: 1000 }", limits)
	if err == nil {
		t.Fatal("expected an error")
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is not *omnist.ParseError: %T %v", err, err)
	}
	if pe.Path != "$.a.n" {
		t.Errorf("path = %q, want %q", pe.Path, "$.a.n")
	}
}


func BenchmarkReadOMLFlat(b *testing.B) {
	sizes := []int{100, 1000, 5000, 10000}
	for _, size := range sizes {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			var sb strings.Builder
			for i := 0; i < size; i++ {
				fmt.Fprintf(&sb, "item_%d: %d\n", i, i)
			}
			src := sb.String()
			limits := omnist.DefaultLimits()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Read(src, limits)
			}
		})
	}
}

func TestPeekRuneAtEOF(t *testing.T) {
	l := newLexer("a", omnist.NewLimitChecker(omnist.DefaultLimits()))
	if r := l.peekRuneAt(10); r != 0 {
		t.Errorf("expected 0, got %v", r)
	}
}
