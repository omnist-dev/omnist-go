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
	// CodeParseLeadingZero is raised when a NUMBER/INTEGER literal's
	// integer part has a leading zero (e.g. "01", "00.5"). Per spec
	// section 4.2.3 (added 2026-08-29): int-part = "0" / (a nonzero
	// digit followed by any digits) -- a bare "0" alone, or "-0", is
	// never a leading zero and remains valid.
	CodeParseLeadingZero Code = "parse.leading-zero"
	// CodeParseInvalidDate is raised when a DATE token (or DATETIME's
	// date portion) is a valid ISO-8601 shape but not a valid calendar
	// date -- month out of 01-12, or day invalid for month/year
	// (including leap years). Per spec section 4.2.4 (added
	// 2026-08-29).
	CodeParseInvalidDate Code = "parse.invalid-date"
	// CodeParseInvalidTime is raised when a TIME token (or DATETIME's
	// time portion, or a tz-offset) is a valid ISO-8601 shape but not a
	// valid clock value -- hour out of 00-23, minute or second out of
	// 00-59 (no leap-second spelling), or -- for a tz-offset -- the
	// same hour/minute ranges TIME itself uses. Per spec section 4.2.4
	// (added 2026-08-29): tz-offset shares TIME's exact range check,
	// not a separately implemented one.
	CodeParseInvalidTime Code = "parse.invalid-time"
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
	// CodeSchemaQuotedType is the reverse of CodeSchemaUnquotedLabel, per
	// spec §5.2's quoting rule (added upstream via omnist-spec#35): a
	// quoted string in type position is a data string and can never
	// legally appear there, since type position only ever accepts a bare
	// schema name (a scalar keyword, `any`, or a reference).
	CodeSchemaQuotedType Code = "schema.quoted-type"
	// CodeSchemaDuplicateRoot is raised when a schema contains more than
	// one `root` declaration. Per spec §5.8 (updated 2026-08-23, closing
	// chapter 9 divergence-ledger D-2), this is normatively an error, not
	// an implementation-defined choice.
	CodeSchemaDuplicateRoot Code = "schema.duplicate-root"
	// CodeSchemaEmptyLabel is raised when a field label is the empty
	// string. Per spec section 5.4 (added 2026-08-29): a label is an
	// identifier, not a value -- an empty label names nothing a caller
	// could ever reference. Path is the enclosing record, the same
	// convention CodeSchemaUnquotedLabel uses when the label itself is
	// the problem.
	CodeSchemaEmptyLabel Code = "schema.empty-label"
	// CodeSchemaBracketInLabel is raised when a field label contains a
	// literal '[' or ']' character. Per spec section 5.4 (added
	// 2026-08-29): section 3.6.1's validate() pseudocode appends "[i]"
	// to a repeated label's second and later occurrences when building a
	// diagnostic path, so a label containing a literal bracket can
	// collide with that convention (e.g. a repeatable field "a" and a
	// separately declared field literally named "a[1]" can both path as
	// $.a[1]). Rejecting the character vocabulary in labels is the
	// narrowest fix. Path is the enclosing record, same convention as
	// CodeSchemaEmptyLabel/CodeSchemaUnquotedLabel.
	CodeSchemaBracketInLabel Code = "schema.bracket-in-label"
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
	// CodeAlgebraInferMixedShape is raised when a label is a node in some
	// samples and a scalar in others (allow_any=false). The spec's §8.3.6
	// taxonomy table does not list a dedicated code for this failure --
	// only infer-no-samples, infer-scalar-root, and infer-conflicting-
	// scalars are enumerated there, even though §6.10's infer_type
	// pseudocode has two distinct hard-failure branches (mixed shape, and
	// conflicting scalar kinds). This is the plainly-correct reading of
	// that gap: mint a fourth algebra.infer-* code following the same
	// naming convention, rather than overloading infer-conflicting-scalars
	// (whose taxonomy description is specifically "disagree on a scalar
	// kind") for a shape mismatch that isn't a scalar-kind disagreement at
	// all.
	CodeAlgebraInferMixedShape Code = "algebra.infer-mixed-shape"
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
// CONTRIBUTING.md §2.4. Its Path field MUST be a text-position
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
