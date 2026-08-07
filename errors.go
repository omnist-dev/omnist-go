package omnist

import "fmt"

// Code is a diagnostic code from the spec §8.3 taxonomy: a lowercase,
// dot-separated path whose first segment is the family. Codes are stable
// identifiers; once published, a code's meaning MUST NOT change.
type Code string

// parse.* — text to Document, stage 1 (spec §8.3.1).
const (
	CodeParseUnexpectedToken    Code = "parse.unexpected-token"
	CodeParseTrailingContent    Code = "parse.trailing-content"
	CodeParseUnterminatedString Code = "parse.unterminated-string"
	CodeParseInvalidEscape      Code = "parse.invalid-escape"
	CodeParseUnpairedSurrogate  Code = "parse.unpaired-surrogate"
	CodeParseControlCharacter   Code = "parse.control-character"
	CodeParseReservedWordLabel  Code = "parse.reserved-word-label"
	CodeParseBareWord           Code = "parse.bare-word"
	CodeParseEmptyArray         Code = "parse.empty-array"
	CodeParseNestedArray        Code = "parse.nested-array"
	CodeParseSeparatorInArray   Code = "parse.separator-in-array"
)

// document.* — building and limits (spec §8.3.2).
const (
	CodeDocumentLimitDepth       Code = "document.limit.depth"
	CodeDocumentLimitNodes       Code = "document.limit.nodes"
	CodeDocumentLimitIntDigits   Code = "document.limit.int-digits"
	CodeDocumentUnlabeledElement Code = "document.unlabeled-element"
)

// schema.* — schema well-formedness (spec §8.3.3).
const (
	CodeSchemaNoRoot                Code = "schema.no-root"
	CodeSchemaUnknownType           Code = "schema.unknown-type"
	CodeSchemaDuplicateRecord       Code = "schema.duplicate-record"
	CodeSchemaDuplicateField        Code = "schema.duplicate-field"
	CodeSchemaReservedName          Code = "schema.reserved-name"
	CodeSchemaInvalidCardinality    Code = "schema.invalid-cardinality"
	CodeSchemaNonIntegerCardinality Code = "schema.non-integer-cardinality"
	CodeSchemaEmptyCardinality      Code = "schema.empty-cardinality"
	CodeSchemaUnquotedLabel         Code = "schema.unquoted-label"
	CodeSchemaNullableRef           Code = "schema.nullable-ref"
	CodeSchemaNullableAny           Code = "schema.nullable-any"
)

// validate.* — document against schema (spec §8.3.4).
const (
	CodeValidateShapeMismatch   Code = "validate.shape-mismatch"
	CodeValidateTypeMismatch    Code = "validate.type-mismatch"
	CodeValidateNullNotAllowed  Code = "validate.null-not-allowed"
	CodeValidateUnexpectedField Code = "validate.unexpected-field"
	CodeValidateCardinality     Code = "validate.cardinality"
)

// materialize.* — schema-directed deserialization (spec §8.3.5).
const (
	CodeMaterializeInexactConversion Code = "materialize.inexact-conversion"
)

// algebra.* — operations over schemas (spec §8.3.6).
const (
	CodeAlgebraExtractInvalidatesRoot  Code = "algebra.extract-invalidates-root"
	CodeAlgebraInferNoSamples          Code = "algebra.infer-no-samples"
	CodeAlgebraInferScalarRoot         Code = "algebra.infer-scalar-root"
	CodeAlgebraInferConflictingScalars Code = "algebra.infer-conflicting-scalars"
)

// lint.* — schema diagnostics (spec §8.3.7).
const (
	CodeLintUnsatisfiableRecord Code = "lint.unsatisfiable-record"
	CodeLintUnreachableRecord   Code = "lint.unreachable-record"
	CodeLintDuplicateRecord     Code = "lint.duplicate-record"
	CodeLintAnyField            Code = "lint.any-field"
)

// format.* — codec adjustments (spec §8.3.8).
const (
	CodeFormatTemporalStringified Code = "format.temporal-stringified"
	CodeFormatFloatSpecial        Code = "format.float-special"
	CodeFormatNullUnrepresentable Code = "format.null-unrepresentable"
	CodeFormatAttributeDropped    Code = "format.attribute-dropped"
	CodeFormatNamespaceDropped    Code = "format.namespace-dropped"
	CodeFormatInterleavingLost    Code = "format.interleaving-lost"
	CodeFormatMultipleRoots       Code = "format.multiple-roots"
)

// write.* (spec §8.3.9).
const (
	CodeWriteUnsupportedValue Code = "write.unsupported-value"
)

// Severity is a diagnostic's severity level, per spec §8.2's table.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityInfo
)

// String returns the taxonomy-style lowercase name of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	default:
		return "unknown"
	}
}

// Diagnostic is a single reported problem, carrying at least the four
// fields spec §8.2 requires: code, path, message, severity.
type Diagnostic struct {
	Path     string
	Code     Code
	Message  string
	Severity Severity
}

// Error implements the error interface so a Diagnostic can be used
// wherever a plain error is expected.
func (d Diagnostic) Error() string {
	return fmt.Sprintf("%s: %s: %s", d.Path, d.Code, d.Message)
}

// ParseError is the structured error a stage-1 (text to Document) reader
// reports, per the design decision recorded in
// docs/workflow-playbook.md §2.4. Its Path field MUST be a text-position
// path per spec §8.4 (e.g. "14:8"), since a parse.* diagnostic fires
// before any Document exists to descend a Document-shaped path into.
type ParseError struct {
	Line    int
	Col     int
	Path    string
	Code    Code
	Message string
}

// Error implements the error interface.
func (e ParseError) Error() string {
	return fmt.Sprintf("%d:%d: %s: %s", e.Line, e.Col, e.Code, e.Message)
}
