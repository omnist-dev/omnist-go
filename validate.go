package omnist

import "fmt"

// This file implements validate(document, schema) per spec §3.6 and its
// §3.6.1 pseudocode: check a Document (document.go, spec ch.2) against a
// Schema (schema.go, spec ch.3) and report every conformance failure found,
// not just the first.

// Validate checks doc against s and returns every diagnostic found. An
// empty (non-nil) slice means doc conforms to s (spec §3.6: "an empty list
// means valid").
//
// validate MUST run within the depth limit of §2.4 (spec §3.6's own text);
// exceeding it produces a document.limit.depth diagnostic (via the
// existing LimitChecker from issue #1's limits.go) rather than a
// validate.* finding, and descent stops at that point.
func Validate(doc Document, s Schema) []Diagnostic {
	result := []Diagnostic{}
	checker := NewLimitChecker(DefaultLimits())
	conform(documentTarget(doc), s, RefType(s.Root), RootPath(), checker, &result)
	return result
}

// documentTarget adapts a Document (spec §2.2: node | value) to a Target
// (spec D-4: value | node) so conform's single recursive walk can treat the
// document root the same way it treats every edge target beneath it.
func documentTarget(doc Document) Target {
	if doc.IsNode {
		return NodeTarget(doc.Node)
	}
	return ValueTarget(doc.Value)
}

// resolvedKind identifies which of Type's three alternatives S.resolve(t)
// produced (spec §3.3's `Type = Scalar | Ref | Any`, via §6.2's
// S.resolve notation).
type resolvedKind int

const (
	resolvedScalar resolvedKind = iota
	resolvedRecord
	resolvedAny
)

// resolved is S.resolve(t): a Type resolved through the schema's env to
// exactly one of a scalar declaration, a Record, or `any`. A Ref whose name
// is absent from Env resolves to a nil Record; that indicates a
// not-well-formed schema (spec §3.3 S-6 requires every reference to
// resolve), which is a schema.* well-formedness concern from issue #5, not
// something validate re-checks — conformRecord treats a nil Record as
// "no fields, closed", so any node there reports unexpected-field for every
// edge rather than panicking.
type resolved struct {
	kind       resolvedKind
	scalarKind ScalarKind
	nullable   bool
	record     *Record
}

// resolveType implements S.resolve(t) from the §3.6.1 pseudocode.
func resolveType(s Schema, t Type) resolved {
	switch t.Kind {
	case TypeAnyKind:
		return resolved{kind: resolvedAny}
	case TypeScalarKind:
		return resolved{kind: resolvedScalar, scalarKind: t.ScalarKind, nullable: t.Nullable}
	case TypeRefKind:
		return resolved{kind: resolvedRecord, record: s.Env[t.RefName]}
	default:
		// Type is a closed struct (schema.go) constructed only via
		// ScalarType/RefType/AnyType, so every legal value hits one of the
		// three cases above. This default only guards against a future
		// TypeKind constant added without updating resolveType.
		return resolved{kind: resolvedAny}
	}
}

// conform is `conform(node, S, t, path, result)` from §3.6.1: it resolves t
// and dispatches to the scalar or record check, or stops descent for `any`
// (spec §3.7: "Descent stops. The subtree is accepted unchecked.").
func conform(target Target, s Schema, t Type, path Path, checker *LimitChecker, result *[]Diagnostic) {
	r := resolveType(s, t)
	switch r.kind {
	case resolvedAny:
		return
	case resolvedScalar:
		conformScalar(target, r, path, result)
	case resolvedRecord:
		conformRecord(target, s, r.record, path, checker, result)
	}
}

// conformScalar is `conform_scalar(node, s, path, result)` from §3.6.1.
func conformScalar(target Target, r resolved, path Path, result *[]Diagnostic) {
	if target.IsNode() {
		addDiagnostic(result, path, CodeValidateShapeMismatch, "expected a scalar value, got an object")
		return
	}
	v, _ := target.Value()
	if v.IsNull {
		if !r.nullable {
			addDiagnostic(result, path, CodeValidateNullNotAllowed, "null not allowed here")
		}
		return // null is never checked against kind, matching kind or not.
	}
	if v.Scalar.Kind != r.scalarKind {
		addDiagnostic(result, path, CodeValidateTypeMismatch, "value does not match declared kind")
	}
}

// conformRecord is `conform_record(node, S, rec, path, result)` from
// §3.6.1. rec may be nil (see resolved's doc comment); a nil record has no
// fields, so every edge is unexpected and no field is ever satisfied.
func conformRecord(target Target, s Schema, rec *Record, path Path, checker *LimitChecker, result *[]Diagnostic) {
	if !target.IsNode() {
		addDiagnostic(result, path, CodeValidateShapeMismatch, "expected an object, got a value")
		return
	}
	node, _ := target.Node()

	// §3.6's explicit note: validate MUST run within the §2.4 depth limit,
	// and exceeding it is a resource-cap error, not a validation finding.
	// Every EnterNode is paired with exactly one LeaveNode (even on the
	// limit-exceeded path) so the checker's running depth stays correct for
	// sibling subtrees visited later in the same walk.
	if diag := checker.EnterNode(path.String()); diag != nil {
		*result = append(*result, *diag)
		checker.LeaveNode()
		return
	}
	defer checker.LeaveNode()

	// Closedness and targets (spec §3.6 points 2 and 3): walk edges in
	// order for path-index bookkeeping only (§3.6.1's comment: "in edge
	// order; order not otherwise used"). counts accumulates independently
	// of that order for the cardinality check below, per D-3 order
	// independence.
	counts := make(map[string]int, len(node.Edges))
	for i, e := range node.Edges {
		counts[e.Label]++
		occurrence, repeated := PathIndexInNode(node, i)
		childPath := path.Child(e.Label, occurrence, repeated)

		f := findField(rec, e.Label)
		if f == nil {
			addDiagnostic(result, childPath, CodeValidateUnexpectedField, "field not declared on this record")
			// No further descent: an undeclared field has no type to check
			// against (§3.6.1's first "easy to miss" detail).
			continue
		}
		conform(e.Target, s, f.Type, childPath, checker, result)
	}

	// Cardinality (spec §3.6 point 1). The error's path is the record's
	// own path, not any edge's, even when the count is nonzero-but-wrong
	// (§3.6.1's second "easy to miss" detail) — never childPath.
	if rec == nil {
		return
	}
	for _, f := range rec.Fields {
		c := counts[f.Label]
		if c < int(f.Cardinality.Min) || (!f.Cardinality.Unbounded && c > int(f.Cardinality.Max)) {
			addDiagnostic(result, path, CodeValidateCardinality, cardinalityMessage(f, c))
		}
	}
}

// findField looks up rec.field(label) from the pseudocode. rec may be nil
// (see resolved's doc comment on conformRecord).
func findField(rec *Record, label string) *Field {
	if rec == nil {
		return nil
	}
	for i := range rec.Fields {
		if rec.Fields[i].Label == label {
			return &rec.Fields[i]
		}
	}
	return nil
}

// cardinalityMessage renders the §3.6.1 message template: "field X occurs
// N time(s), expected [min,max]", with "max" rendered as "" (unbounded)
// when the field's cardinality has no upper bound, matching the OSD
// unbounded-max spelling used elsewhere in this repository (schema.go,
// spec §3.4's `[n,]` form).
func cardinalityMessage(f Field, count int) string {
	max := "unbounded"
	if !f.Cardinality.Unbounded {
		max = fmt.Sprintf("%d", f.Cardinality.Max)
	}
	return fmt.Sprintf("field %s occurs %d time(s), expected [%d,%s]", f.Label, count, f.Cardinality.Min, max)
}

// addDiagnostic appends an error-severity diagnostic at path to result.
// Every validate.* code is an error per spec §8.2's severity table (only
// lint.* and format.* codes use warning/info).
func addDiagnostic(result *[]Diagnostic, path Path, code Code, message string) {
	*result = append(*result, Diagnostic{
		Path:     path.String(),
		Code:     code,
		Message:  message,
		Severity: SeverityError,
	})
}
