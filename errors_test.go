package omnist

import "testing"

func TestSeverityString(t *testing.T) {
	cases := []struct {
		s    Severity
		want string
	}{
		{SeverityError, "error"},
		{SeverityWarning, "warning"},
		{SeverityInfo, "info"},
		{Severity(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("Severity(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestDiagnosticError(t *testing.T) {
	d := Diagnostic{
		Path:     "$.port",
		Code:     CodeValidateTypeMismatch,
		Message:  "expected integer",
		Severity: SeverityError,
	}
	want := "$.port: validate.type-mismatch: expected integer"
	if got := d.Error(); got != want {
		t.Errorf("Diagnostic.Error() = %q, want %q", got, want)
	}
	// Diagnostic must satisfy the error interface.
	var _ error = d
}

func TestParseErrorError(t *testing.T) {
	e := ParseError{
		Line:    14,
		Col:     8,
		Path:    "14:8",
		Code:    CodeParseUnexpectedToken,
		Message: "unexpected token",
	}
	want := "14:8: parse.unexpected-token: unexpected token"
	if got := e.Error(); got != want {
		t.Errorf("ParseError.Error() = %q, want %q", got, want)
	}
	var _ error = e
}

// TestAllTaxonomyCodesPresent is a light sanity check that every family's
// constants are distinct non-empty strings, catching copy-paste
// duplication across the long const blocks in errors.go.
func TestAllTaxonomyCodesPresent(t *testing.T) {
	codes := []Code{
		CodeParseUnexpectedToken, CodeParseTrailingContent, CodeParseUnterminatedString,
		CodeParseInvalidEscape, CodeParseUnpairedSurrogate, CodeParseControlCharacter,
		CodeParseReservedWordLabel, CodeParseBareWord, CodeParseEmptyArray,
		CodeParseNestedArray, CodeParseSeparatorInArray,
		CodeDocumentLimitDepth, CodeDocumentLimitNodes, CodeDocumentLimitIntDigits,
		CodeDocumentUnlabeledElement,
		CodeSchemaNoRoot, CodeSchemaUnknownType, CodeSchemaDuplicateRecord,
		CodeSchemaDuplicateField, CodeSchemaReservedName, CodeSchemaInvalidCardinality,
		CodeSchemaNonIntegerCardinality, CodeSchemaEmptyCardinality,
		CodeSchemaUnquotedLabel, CodeSchemaNullableRef, CodeSchemaNullableAny,
		CodeValidateShapeMismatch, CodeValidateTypeMismatch, CodeValidateNullNotAllowed,
		CodeValidateUnexpectedField, CodeValidateCardinality,
		CodeMaterializeInexactConversion,
		CodeAlgebraExtractInvalidatesRoot, CodeAlgebraInferNoSamples,
		CodeAlgebraInferScalarRoot, CodeAlgebraInferConflictingScalars,
		CodeLintUnsatisfiableRecord, CodeLintUnreachableRecord,
		CodeLintDuplicateRecord, CodeLintAnyField,
		CodeFormatTemporalStringified, CodeFormatFloatSpecial,
		CodeFormatNullUnrepresentable, CodeFormatAttributeDropped,
		CodeFormatNamespaceDropped, CodeFormatInterleavingLost,
		CodeFormatMultipleRoots,
		CodeWriteUnsupportedValue,
	}
	seen := make(map[Code]bool, len(codes))
	for _, c := range codes {
		if c == "" {
			t.Errorf("empty code constant found")
		}
		if seen[c] {
			t.Errorf("duplicate code constant: %q", c)
		}
		seen[c] = true
	}
	if len(codes) != 48 {
		t.Errorf("expected 48 taxonomy codes, got %d", len(codes))
	}
}
