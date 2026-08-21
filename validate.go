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

// ResolvedKind identifies which of Type's three alternatives S.resolve(t)
// produced (spec §3.3's `Type = Scalar | Ref | Any`, via §6.2's
// S.resolve notation).
type ResolvedKind int

const (
	ResolvedScalar ResolvedKind = iota
	ResolvedRecord
	ResolvedAny
)

// Resolved is S.resolve(t): a Type resolved through the schema's env to
// exactly one of a scalar declaration, a Record, or `any`. A Ref whose name
// is absent from Env resolves to a nil Record; that indicates a
// not-well-formed schema (spec §3.3 S-6 requires every reference to
// resolve), which is a schema.* well-formedness concern from issue #5, not
// something validate re-checks — conformRecord treats a nil Record as
// "no fields, closed", so any node there reports unexpected-field for every
// edge rather than panicking.
type Resolved struct {
	Kind       ResolvedKind
	ScalarKind ScalarKind
	Nullable   bool
	Record     *Record
}

// ResolveType implements S.resolve(t) from the §3.6.1 pseudocode.
func ResolveType(s Schema, t Type) Resolved {
	switch t.Kind {
	case TypeAnyKind:
		return Resolved{Kind: ResolvedAny}
	case TypeScalarKind:
		return Resolved{Kind: ResolvedScalar, ScalarKind: t.ScalarKind, Nullable: t.Nullable}
	case TypeRefKind:
		return Resolved{Kind: ResolvedRecord, Record: s.Env[t.RefName]}
	default:
		// Type is a closed struct (schema.go) constructed only via
		// ScalarType/RefType/AnyType, so every legal value hits one of the
		// three cases above. This default only guards against a future
		// TypeKind constant added without updating ResolveType.
		return Resolved{Kind: ResolvedAny}
	}
}

// conform is `conform(node, S, t, path, result)` from §3.6.1: it resolves t
// and dispatches to the scalar or record check, or stops descent for `any`
// (spec §3.7: "Descent stops. The subtree is accepted unchecked.").
func conform(target Target, s Schema, t Type, path Path, checker *LimitChecker, result *[]Diagnostic) {
	r := ResolveType(s, t)
	switch r.Kind {
	case ResolvedAny:
		return
	case ResolvedScalar:
		conformScalar(target, r, path, result)
	case ResolvedRecord:
		conformRecord(target, s, r.Record, path, checker, result)
	}
}

// conformScalar is `conform_scalar(node, s, path, result)` from §3.6.1.
func conformScalar(target Target, r Resolved, path Path, result *[]Diagnostic) {
	if target.IsNode() {
		addDiagnostic(result, path, CodeValidateShapeMismatch, "expected a scalar value, got an object")
		return
	}
	v, _ := target.Value()
	if v.IsNull {
		if !r.Nullable {
			addDiagnostic(result, path, CodeValidateNullNotAllowed, "null not allowed here")
		}
		return // null is never checked against kind, matching kind or not.
	}
	if !matchesKind(v.Scalar.Kind, r.ScalarKind) {
		addDiagnostic(result, path, CodeValidateTypeMismatch, "value does not match declared kind")
	}
}

// matchesKind is `matches_kind(value, declared_kind)` from §3.6.1. A
// value's own kind satisfies the declared kind either by exact match, or
// via the one sanctioned scalar subtype relation (§6.3): an integer value
// satisfies a number-typed field directly. This is not materialization --
// no conversion happens, the value's own kind is simply a subtype of the
// declared one. No other subtype relation exists; this relation is
// one-directional (a number value does NOT satisfy an integer-typed field).
func matchesKind(valueKind, declaredKind ScalarKind) bool {
	if valueKind == declaredKind {
		return true
	}
	if valueKind == KindInteger && declaredKind == KindNumber {
		return true
	}
	return false
}

// conformRecord is `conform_record(node, S, rec, path, result)` from
// §3.6.1. rec may be nil (see Resolved's doc comment); a nil record has no
// fields, so every edge is unexpected and no field is ever satisfied.
//
// This is a thin wrapper around walkRecordShape, which holds the actual
// shape/cardinality-checking logic shared with materialize.go's
// materializeRecord — see walkRecordShape's doc comment for why the
// sharing is structured this way, per issue #35's lockstep invariant
// ("whatever materialize accepts at a leaf, validate must also accept,
// and vice versa").
func conformRecord(target Target, s Schema, rec *Record, path Path, checker *LimitChecker, result *[]Diagnostic) {
	walkRecordShape(target, rec, path, checker, result, func(e Edge, f *Field, childPath Path) Target {
		conform(e.Target, s, f.Type, childPath, checker, result)
		return e.Target
	})
}

// walkRecordShape is the shape/cardinality-checking core of §3.6.1's
// `conform_record` (validate) and §7.2.1's `materialize_record`
// (materialize): both operations check exactly the same things at a
// record boundary (is the target a node, is the depth limit respected, is
// every edge's label declared, does every declared field's edge count
// fall within its cardinality) and differ only in what they do with a
// *matched* field's target — validate recurses into conform (and
// discards the result), materialize recurses into materialize_type and
// keeps the converted Target for its output document. onMatched is that
// one point of difference; everything else here is identical between the
// two callers by construction, which is how the two stay in lockstep
// rather than via two independently-written implementations that could
// drift apart.
//
// Returns the record's output edges: a matched edge holds whatever
// onMatched returned, an unmatched (unexpected-field) edge is carried
// through unchanged, per §7.2.1's explicit note that materialize appends
// an unexpected field's (label, child) to its output unmodified rather
// than dropping it. validate's caller (conformRecord) ignores this
// return value; only materialize.go's materializeRecord uses it.
func walkRecordShape(target Target, rec *Record, path Path, checker *LimitChecker, result *[]Diagnostic, onMatched func(e Edge, f *Field, childPath Path) Target) []Edge {
	if !target.IsNode() {
		addDiagnostic(result, path, CodeValidateShapeMismatch, "expected an object, got a value")
		return nil
	}
	node, _ := target.Node()

	// §3.6's explicit note: validate MUST run within the §2.4 depth limit,
	// and exceeding it is a resource-cap error, not a validation finding.
	// Every EnterNode is paired with exactly one LeaveNode (even on the
	// limit-exceeded path) so the checker's running depth stays correct for
	// sibling subtrees visited later in the same walk. materialize needs
	// the identical depth accounting (issue #35: "materialize needs the
	// same [LimitChecker wiring]"), which this sharing gives it for free.
	if diag := checker.EnterNode(path.String()); diag != nil {
		*result = append(*result, *diag)
		checker.LeaveNode()
		return nil
	}
	defer checker.LeaveNode()

	// Closedness and targets (spec §3.6 points 2 and 3): walk edges in
	// order for path-index bookkeeping only (§3.6.1's comment: "in edge
	// order; order not otherwise used"). counts accumulates independently
	// of that order for the cardinality check below, per D-3 order
	// independence.
	counts := make(map[string]int, len(node.Edges))
	for _, e := range node.Edges {
		counts[e.Label]++
	}
	runningIndex := make(map[string]int, len(counts))
	fieldsIdx := rec.Index()

	outEdges := make([]Edge, 0, len(node.Edges))
	for _, e := range node.Edges {
		occurrence := runningIndex[e.Label]
		repeated := counts[e.Label] > 1
		runningIndex[e.Label]++
		childPath := path.Child(e.Label, occurrence, repeated)

		f := fieldsIdx.Field(e.Label)
		if f == nil {
			addDiagnostic(result, childPath, CodeValidateUnexpectedField, "field not declared on this record")
			// No further descent: an undeclared field has no type to check
			// against (§3.6.1's first "easy to miss" detail). Kept
			// unchanged in the output, per materialize_record's pseudocode.
			outEdges = append(outEdges, e)
			continue
		}
		outEdges = append(outEdges, Edge{Label: e.Label, Target: onMatched(e, f, childPath)})
	}

	// Cardinality (spec §3.6 point 1). The error's path is the record's
	// own path, not any edge's, even when the count is nonzero-but-wrong
	// (§3.6.1's second "easy to miss" detail) — never childPath.
	if rec == nil {
		return outEdges
	}
	for _, f := range rec.Fields {
		c := counts[f.Label]
		if c < int(f.Cardinality.Min) || (!f.Cardinality.Unbounded && c > int(f.Cardinality.Max)) {
			addDiagnostic(result, path, CodeValidateCardinality, cardinalityMessage(f, c))
		}
	}
	return outEdges
}

// findField looks up rec.field(label) from the pseudocode. rec may be nil
// (see Resolved's doc comment on conformRecord).
func findField(rec *Record, label string) *Field {
	return rec.Index().Field(label)
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
