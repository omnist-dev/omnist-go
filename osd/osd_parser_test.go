package osd

import (
	"fmt"
	"strings"
	"strconv"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

func mustParseOSD(t *testing.T, src string) omnist.Schema {
	t.Helper()
	s, err := Read(src)
	if err != nil {
		t.Fatalf("Read(%q) unexpected error: %v", src, err)
	}
	return s
}

func mustFailOSD(t *testing.T, src string) error {
	t.Helper()
	_, err := Read(src)
	if err == nil {
		t.Fatalf("Read(%q) expected error, got none", src)
	}
	return err
}

func wantParseErrCode(t *testing.T, err error, code omnist.Code) {
	t.Helper()
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("error is not *ParseError: %T %v", err, err)
	}
	if pe.Code != code {
		t.Errorf("code = %q, want %q (err: %v)", pe.Code, code, pe)
	}
}

func wantDiagCode(t *testing.T, err error, code omnist.Code) {
	t.Helper()
	d, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("error is not Diagnostic: %T %v", err, err)
	}
	if d.Code != code {
		t.Errorf("code = %q, want %q (err: %v)", d.Code, code, d)
	}
}

func wantDiag(t *testing.T, err error, code omnist.Code, path string) {
	t.Helper()
	d, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("error is not Diagnostic: %T %v", err, err)
	}
	if d.Code != code {
		t.Errorf("code = %q, want %q (err: %v)", d.Code, code, d)
	}
	if d.Path != path {
		t.Errorf("path = %q, want %q (err: %v)", d.Path, path, d)
	}
}

// --- §5.10 worked examples, verbatim ---

func TestWorkedStringUnescaping(t *testing.T) {
	s := mustParseOSD(t, `record R { "a\nb": string } root R`)
	rec := s.Env["R"]
	if len(rec.Fields) != 1 || rec.Fields[0].Label != "anb" {
		t.Fatalf("got fields %+v", rec.Fields)
	}
}

func TestWorkedCardinality15(t *testing.T) {
	s := mustParseOSD(t, `record R { "a" [1,5]: string } root R`)
	c := s.Env["R"].Fields[0].Cardinality
	if c.Min != 1 || c.Max != 5 || c.Unbounded {
		t.Fatalf("got %+v", c)
	}
}

func TestWorkedCardinality5Unbounded(t *testing.T) {
	s := mustParseOSD(t, `record R { "a" [5,]: string } root R`)
	c := s.Env["R"].Fields[0].Cardinality
	if c.Min != 5 || !c.Unbounded {
		t.Fatalf("got %+v", c)
	}
}

func TestWorkedCardinalityZeroTo5(t *testing.T) {
	s := mustParseOSD(t, `record R { "a" [,5]: string } root R`)
	c := s.Env["R"].Fields[0].Cardinality
	if c.Min != 0 || c.Max != 5 || c.Unbounded {
		t.Fatalf("got %+v", c)
	}
}

func TestWorkedCardinalityAny(t *testing.T) {
	s := mustParseOSD(t, `record R { "a" [,]: string } root R`)
	c := s.Env["R"].Fields[0].Cardinality
	if c.Min != 0 || !c.Unbounded {
		t.Fatalf("got %+v", c)
	}
}

func TestWorkedEmptyCardinality(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" []: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaEmptyCardinality)
}

func TestWorkedNegativeCardinality(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [-1]: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaInvalidCardinality)
}

func TestWorkedInvertedCardinality(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [1,0]: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaInvalidCardinality)
}

func TestWorkedNonIntegerCardinality(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [1.5]: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaNonIntegerCardinality)
}

func TestWorkedNullableScalar(t *testing.T) {
	s := mustParseOSD(t, `record R { "a": string? } root R`)
	typ := s.Env["R"].Fields[0].Type
	if typ.Kind != omnist.TypeScalarKind || typ.ScalarKind != omnist.KindString || !typ.Nullable {
		t.Fatalf("got %+v", typ)
	}
}

func TestWorkedNullableRef(t *testing.T) {
	err := mustFailOSD(t, `record R { "a": Other? } record Other { } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaNullableRef)
}

func TestWorkedReservedScalarName(t *testing.T) {
	err := mustFailOSD(t, `record string { "a": string } root string`)
	wantDiagCode(t, err, omnist.CodeSchemaReservedName)
}

func TestWorkedReservedAnyName(t *testing.T) {
	err := mustFailOSD(t, `record any { "a": string } root any`)
	wantDiagCode(t, err, omnist.CodeSchemaReservedName)
}

func TestWorkedDuplicateRecord(t *testing.T) {
	err := mustFailOSD(t, `record R{"a":string} record R{"a":string} root R`)
	wantDiagCode(t, err, omnist.CodeSchemaDuplicateRecord)
}

func TestWorkedNoRoot(t *testing.T) {
	err := mustFailOSD(t, `record R{"a":string}`)
	wantDiag(t, err, omnist.CodeSchemaNoRoot, "$")
}

func TestWorkedUnquotedLabel(t *testing.T) {
	err := mustFailOSD(t, `record R{a:string} root R`)
	wantDiagCode(t, err, omnist.CodeSchemaUnquotedLabel)
}

func TestWorkedTrailingComma(t *testing.T) {
	s := mustParseOSD(t, `record R { "a": string, } root R`)
	if len(s.Env["R"].Fields) != 1 {
		t.Fatalf("got %+v", s.Env["R"].Fields)
	}
}

func TestWorkedAnyField(t *testing.T) {
	s := mustParseOSD(t, `record R { "data": any } root R`)
	if s.Env["R"].Fields[0].Type.Kind != omnist.TypeAnyKind {
		t.Fatalf("got %+v", s.Env["R"].Fields[0].Type)
	}
}

func TestWorkedAnyWithCardinality(t *testing.T) {
	s := mustParseOSD(t, `record R { "data" [0,]: any } root R`)
	f := s.Env["R"].Fields[0]
	if f.Type.Kind != omnist.TypeAnyKind || f.Cardinality.Min != 0 || !f.Cardinality.Unbounded {
		t.Fatalf("got %+v", f)
	}
}

func TestWorkedNullableAny(t *testing.T) {
	err := mustFailOSD(t, `record R { "data": any? } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaNullableAny)
}

func TestWorkedCapitalizedAnyUnknown(t *testing.T) {
	err := mustFailOSD(t, `record R { "data": Any } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaUnknownType)
}

// --- §5.2 quoting rule ---

func TestQuotedStringInTypePositionIsError(t *testing.T) {
	// Per osd-grammar/labels/quoted-string-in-type-position-is-rejected
	// (spec §5.2's quoting rule, omnist-spec#35): a quoted string in type
	// position is schema.quoted-type, the symmetric counterpart of
	// schema.unquoted-label, pathed to the enclosing record ("R").
	err := mustFailOSD(t, `record R { "a": "string" } root R`)
	wantDiag(t, err, omnist.CodeSchemaQuotedType, "R")
}

// --- §5.3.1 string unescaping ---

func TestStringUnescapingBackslashBackslash(t *testing.T) {
	s := mustParseOSD(t, `record R { "a\\b": string } root R`)
	if s.Env["R"].Fields[0].Label != `a\b` {
		t.Fatalf("got %q", s.Env["R"].Fields[0].Label)
	}
}

func TestStringUnescapingBackslashQuote(t *testing.T) {
	s := mustParseOSD(t, `record R { "a\"b": string } root R`)
	if s.Env["R"].Fields[0].Label != `a"b` {
		t.Fatalf("got %q", s.Env["R"].Fields[0].Label)
	}
}

func TestStringUnescapingArbitraryLetterNotUpgraded(t *testing.T) {
	// \t must become the literal letter t, not a tab -- this is the "MUST
	// NOT be upgraded to OML-style escaping" rule from spec §5.3.1.
	s := mustParseOSD(t, `record R { "a\tb": string } root R`)
	if s.Env["R"].Fields[0].Label != "atb" {
		t.Fatalf("got %q", s.Env["R"].Fields[0].Label)
	}
}

// --- §5.6 types: all seven scalars, with and without '?' ---

func TestAllScalarKinds(t *testing.T) {
	cases := []struct {
		name string
		kind omnist.ScalarKind
	}{
		{"string", omnist.KindString},
		{"integer", omnist.KindInteger},
		{"number", omnist.KindNumber},
		{"boolean", omnist.KindBoolean},
		{"date", omnist.KindDate},
		{"time", omnist.KindTime},
		{"datetime", omnist.KindDateTime},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := mustParseOSD(t, `record R { "a": `+c.name+` } root R`)
			typ := s.Env["R"].Fields[0].Type
			if typ.Kind != omnist.TypeScalarKind || typ.ScalarKind != c.kind || typ.Nullable {
				t.Fatalf("got %+v", typ)
			}
			s2 := mustParseOSD(t, `record R { "a": `+c.name+`? } root R`)
			typ2 := s2.Env["R"].Fields[0].Type
			if typ2.Kind != omnist.TypeScalarKind || typ2.ScalarKind != c.kind || !typ2.Nullable {
				t.Fatalf("got %+v", typ2)
			}
		})
	}
}

// --- references: forward, mutual recursion, dangling ---

func TestForwardReference(t *testing.T) {
	s := mustParseOSD(t, `record R { "b": B } record B { "x": string } root R`)
	f := s.Env["R"].Fields[0]
	if f.Type.Kind != omnist.TypeRefKind || f.Type.RefName != "B" {
		t.Fatalf("got %+v", f.Type)
	}
}

func TestMutualRecursion(t *testing.T) {
	s := mustParseOSD(t, `record A { "b" [0,1]: B } record B { "a" [0,1]: A } root A`)
	if s.Env["A"].Fields[0].Type.RefName != "B" {
		t.Fatal("A.b should reference B")
	}
	if s.Env["B"].Fields[0].Type.RefName != "A" {
		t.Fatal("B.a should reference A")
	}
}

func TestDanglingFieldReference(t *testing.T) {
	err := mustFailOSD(t, `record R { "b": Ghost } root R`)
	wantDiag(t, err, omnist.CodeSchemaUnknownType, "R.b")
}

func TestDanglingRoot(t *testing.T) {
	err := mustFailOSD(t, `record R { "a": string } root Ghost`)
	wantDiag(t, err, omnist.CodeSchemaUnknownType, "$")
}

// --- S-1..S-7 well-formedness, each with its own test ---

// S-1: exactly one root, and root MUST resolve to a Ref.
func TestS1RootMissingIsError(t *testing.T) {
	err := mustFailOSD(t, `record R { "a": string }`)
	wantDiag(t, err, omnist.CodeSchemaNoRoot, "$")
}

func TestS1DuplicateRootFirstWins(t *testing.T) {
	// D-2 (chapter 9, open divergence item): this implementation's chosen
	// resolution is "first root wins" -- see the comment in
	// parseSchema's `case "root":` branch for the full reasoning.
	s := mustParseOSD(t, `record A{"x":string} record B{"x":string} root A root B`)
	if s.Root != "A" {
		t.Fatalf("Root = %q, want %q (first root should win)", s.Root, "A")
	}
}

// S-2: cardinality bounds.
func TestS2NegativeMinIsError(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [-1,5]: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaInvalidCardinality)
}

func TestS2MaxLessThanMinIsError(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [5,1]: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaInvalidCardinality)
}

// S-3: reserved record names -- every scalar keyword, plus "any".
func TestS3AllReservedRecordNames(t *testing.T) {
	for _, name := range []string{"string", "integer", "number", "boolean", "date", "time", "datetime", "any"} {
		t.Run(name, func(t *testing.T) {
			err := mustFailOSD(t, `record `+name+` { } root `+name)
			wantDiagCode(t, err, omnist.CodeSchemaReservedName)
		})
	}
}

// S-4: unique record names.
func TestS4DuplicateRecordName(t *testing.T) {
	err := mustFailOSD(t, `record R{} record R{} root R`)
	wantDiag(t, err, omnist.CodeSchemaDuplicateRecord, "R")
}

// S-5: unique field labels per record.
func TestS5DuplicateFieldLabel(t *testing.T) {
	// Per osd-grammar/records/duplicate-field-label-in-one-record-is-an-error
	// the path is the record itself ("R"), a omnist.Schema record-level path, not
	// "R.a" — this is a record-level diagnostic (two fields sharing one
	// label), not a field-level one.
	err := mustFailOSD(t, `record R { "a": string, "a": integer } root R`)
	wantDiag(t, err, omnist.CodeSchemaDuplicateField, "R")
}

// S-6: dangling reference (already covered above for a field and for
// root; this covers a record that exists but is never reachable, which is
// legal -- S-6 only requires resolution, not reachability).
func TestS6UnreachableRecordIsLegal(t *testing.T) {
	s := mustParseOSD(t, `record R{"a":string} record Unused{"x":string} root R`)
	if _, ok := s.Env["Unused"]; !ok {
		t.Fatal("Unused record should still be present in env")
	}
}

// S-7: nullable only on a Scalar -- nullable Ref and nullable any both
// error (each already covered above by name; this groups them under S-7).
func TestS7NullableOnlyOnScalar(t *testing.T) {
	err1 := mustFailOSD(t, `record R { "a": Other? } record Other{} root R`)
	wantDiagCode(t, err1, omnist.CodeSchemaNullableRef)

	err2 := mustFailOSD(t, `record R { "a": any? } root R`)
	wantDiagCode(t, err2, omnist.CodeSchemaNullableAny)
}

// --- misc: empty record body, comments, default cardinality ---

func TestEmptyRecordBody(t *testing.T) {
	s := mustParseOSD(t, `record R { } root R`)
	if len(s.Env["R"].Fields) != 0 {
		t.Fatalf("got %+v", s.Env["R"].Fields)
	}
}

func TestCommentsAnywhere(t *testing.T) {
	src := "# top comment\nrecord R { # inside body\n \"a\": string, # after field\n} root R # trailing"
	s := mustParseOSD(t, src)
	if len(s.Env["R"].Fields) != 1 {
		t.Fatalf("got %+v", s.Env["R"].Fields)
	}
}

func TestDefaultCardinalityIsOneOne(t *testing.T) {
	s := mustParseOSD(t, `record R { "a": string } root R`)
	c := s.Env["R"].Fields[0].Cardinality
	if c.Min != 1 || c.Max != 1 || c.Unbounded {
		t.Fatalf("got %+v", c)
	}
}

func TestDeclarationsAnyOrderRootFirst(t *testing.T) {
	s := mustParseOSD(t, `root R record R { "a": string }`)
	if s.Root != "R" || len(s.Env["R"].Fields) != 1 {
		t.Fatalf("got %+v", s)
	}
}

func TestFullShapeExample(t *testing.T) {
	src := `
# Service topology
record Database {
    "type":            string,
    "server":          string,
    "port":             integer,
}
record Service {
    "host":            string,
    "port":            integer,
    "databases" [1,]:  Database,
    "tags" [0,]:       string,
    "owner" [0,1]:     string?,
    "payload":         any,
}
root Service
`
	s := mustParseOSD(t, src)
	if s.Root != "Service" {
		t.Fatalf("Root = %q", s.Root)
	}
	svc := s.Env["Service"]
	if len(svc.Fields) != 6 {
		t.Fatalf("got %d fields", len(svc.Fields))
	}
}

// --- syntax errors ---

func TestUnterminatedString(t *testing.T) {
	err := mustFailOSD(t, `record R { "a`)
	wantParseErrCode(t, err, omnist.CodeParseUnterminatedString)
}

func TestUnterminatedStringAfterBackslash(t *testing.T) {
	err := mustFailOSD(t, "record R { \"a\\")
	wantParseErrCode(t, err, omnist.CodeParseUnterminatedString)
}

func TestControlCharacterInString(t *testing.T) {
	err := mustFailOSD(t, "record R { \"a\x01b\": string } root R")
	wantParseErrCode(t, err, omnist.CodeParseControlCharacter)
}

func TestUnexpectedCharacter(t *testing.T) {
	err := mustFailOSD(t, `record R { "a": string } root R %`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestMissingBraceAfterRecordName(t *testing.T) {
	err := mustFailOSD(t, `record R "a": string } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestMissingRecordNameEOF(t *testing.T) {
	err := mustFailOSD(t, `record`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestUnterminatedRecordBody(t *testing.T) {
	err := mustFailOSD(t, `record R { "a": string`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestMissingCommaOrBraceAfterField(t *testing.T) {
	err := mustFailOSD(t, `record R { "a": string "b": integer } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestMissingColonAfterLabel(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" string } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestMissingRootName(t *testing.T) {
	err := mustFailOSD(t, `record R{"a":string} root`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestTopLevelUnexpectedToken(t *testing.T) {
	err := mustFailOSD(t, `"a": string`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestTopLevelUnexpectedKeyword(t *testing.T) {
	err := mustFailOSD(t, `bogus R { } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestCardinalityMissingCloseBracket(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [1,2: string } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestCardinalityMissingCloseBracketAfterMinComma(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [1,2 : string } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestCardinalityUnexpectedTokenAfterOpenBracket(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [x]: string } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestCardinalityCommaFirstMissingCloseBracket(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [,5: string } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestCardinalityCommaFirstBadBound(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [,x]: string } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestTypeMissingAfterColon(t *testing.T) {
	err := mustFailOSD(t, `record R { "a": [1] } root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestDiagnosticErrorMethod(t *testing.T) {
	err := mustFailOSD(t, `record R{"a":string}`)
	if err.Error() == "" {
		t.Fatal("omnist.Diagnostic.Error() returned empty string")
	}
}

func TestParseErrorErrorMethod(t *testing.T) {
	err := mustFailOSD(t, `record`)
	if err.Error() == "" {
		t.Fatal("omnist.ParseError.Error() returned empty string")
	}
}

// --- lexer errors surfacing mid-parse, at every point where the parser
// asks the lexer for the token following a just-consumed one. Each of
// these sources places an unlexable '%' immediately after a different
// sync point, exercising the parser's `if err := p.advance(); err != nil`
// branches (a lexer failure can occur at any of them, not just at the
// very first token).

func TestLexerErrorAtEveryAdvancePoint(t *testing.T) {
	sources := []string{
		`%`,                     // very first token
		`record%`,               // after 'record' keyword
		`record R%`,             // after record name
		`record R{%`,            // after '{'
		`record R{"a":string,%`, // after ',' in record body
		`record R{}%`,           // after '}'
		`root%`,                 // after 'root' keyword
		`root R%`,               // after root name
		`record R{"a"%`,         // after string label
		`record R{"a":%`,        // after ':'
		`record R{"a" [%`,       // after '[' in cardinality
		`record R{"a" []%`,      // after ']' in empty cardinality
		`record R{"a" [,%`,      // after ',' in comma-first cardinality
		`record R{"a" [,]%`,     // after ']' in [,] cardinality
		`record R{"a" [1]%`,     // after ']' in [n] cardinality
		`record R{"a" [1,%`,     // after ',' following int bound
		`record R{"a" [1,]%`,    // after ']' in [m,] cardinality
		`record R{"a" [1%`,      // after int bound itself
		`record R{"a" [1.5%`,    // after non-integer bound itself
		`record R{"a":string%`,  // after scalar type name
		`record R{"a":string?%`, // after '?'
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			err := mustFailOSD(t, src)
			if _, ok := err.(*omnist.ParseError); !ok {
				t.Fatalf("Read(%q) error is not *ParseError: %T %v", src, err, err)
			}
		})
	}
}

func TestEOFRightAfterCommaInRecordBody(t *testing.T) {
	err := mustFailOSD(t, `record R{"a":string,`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestEOFRightAfterOpenBrace(t *testing.T) {
	err := mustFailOSD(t, `record R{`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestNumericTokenInLabelPosition(t *testing.T) {
	err := mustFailOSD(t, `record R{5:string} root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestCardinalitySingleValuePositive(t *testing.T) {
	s := mustParseOSD(t, `record R { "a" [3]: string } root R`)
	c := s.Env["R"].Fields[0].Cardinality
	if c.Min != 3 || c.Max != 3 || c.Unbounded {
		t.Fatalf("got %+v", c)
	}
}

func TestCardinalityCommaFirstNegativeMax(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [,-1]: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaInvalidCardinality)
}

func TestCardinalityMinCommaOnlyNegativeMin(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [-1,]: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaInvalidCardinality)
}

func TestCardinalitySecondBoundNonInteger(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [1,2.5]: string } root R`)
	wantDiagCode(t, err, omnist.CodeSchemaNonIntegerCardinality)
}

func TestCardinalityUnexpectedTokenAfterFirstBound(t *testing.T) {
	err := mustFailOSD(t, `record R { "a" [1:string} root R`)
	wantParseErrCode(t, err, omnist.CodeParseUnexpectedToken)
}

func TestCardinalityBoundOverflowsInt64(t *testing.T) {
	// A pathologically large cardinality bound: the tokenizer clamps it to
	// 0 rather than failing outright, and field construction then reports
	// it as an invalid (though not "negative" in this particular case,
	// since it clamps to 0, which is >= 0) cardinality only if paired with
	// a smaller max -- here it is used as min with an explicit max of 1,
	// so min(0) <= max(1) and the field parses successfully.
	s := mustParseOSD(t, `record R { "a" [99999999999999999999,1]: string } root R`)
	c := s.Env["R"].Fields[0].Cardinality
	if c.Min != 0 || c.Max != 1 {
		t.Fatalf("got %+v", c)
	}
}


func BenchmarkReadOSDManyFields(b *testing.B) {
	sizes := []int{100, 1000, 5000, 10000}
	for _, size := range sizes {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			var sb strings.Builder
			sb.WriteString("record ManyFields {\n")
			for i := 0; i < size; i++ {
				fmt.Fprintf(&sb, "  field_%d: string,\n", i)
			}
			sb.WriteString("}\nroot ManyFields\n")
			src := sb.String()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Read(src)
			}
		})
	}
}

func TestOSDPeekRuneAtEOF(t *testing.T) {
	l := newOSDLexer("")
	if r := l.peekRune(); r != 0 {
		t.Errorf("expected 0 at EOF, got %v", r)
	}
}
