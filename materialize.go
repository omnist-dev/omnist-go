package omnist

import (
	"math"
	"math/big"
	"regexp"
)

// This file implements materialize(document, schema) per spec §7.1 (the
// two-stage read model's stage 2) and its §7.2/§7.2.1 pseudocode: walk an
// untyped Document together with a Schema, upgrading leaves to their
// declared types and checking record shape in the same pass.
//
// §7.2.1 describes materialize as "structurally this is §3.6.1's validate
// with every leaf replaced by its upgrade result instead of discarded",
// and the spec states a hard invariant: the two MUST stay in lockstep —
// whatever materialize accepts at a leaf, validate must also accept
// there, and vice versa. This file follows that structure directly:
// materialize/materializeType/materializeRecord mirror
// Validate/conform/conformRecord one-for-one, and materializeRecord's
// shape/cardinality checking is not reimplemented here at all — it calls
// validate.go's walkRecordShape, the exact function conformRecord itself
// calls (see walkRecordShape's doc comment in validate.go). The only
// genuinely new logic in this file is materializeScalar/tryUpgrade, since
// validate has no equivalent of "convert a leaf", only "check its kind".

// Materialize walks doc against s, upgrading leaves to their declared
// scalar kinds and checking record shape, collecting every problem found
// (not failing fast, matching Validate). On success (no diagnostics) the
// returned Document is the materialized result. On failure the returned
// Document is a best-effort partial materialization — not part of this
// function's contract, since a caller with a non-empty diagnostics slice
// has no ok Document per §7.1 ("fails-with-diagnostics on any problem")
// and must not use it. err is always nil today; it is part of the
// signature for symmetry with the two-stage read model's stage-1 codec
// readers (json_reader.go et al.), which do report structural errors
// distinct from diagnostics, and to leave room for a future internal
// failure mode without a breaking signature change.
func Materialize(doc Document, s Schema) (Document, []Diagnostic, error) {
	result := []Diagnostic{}
	checker := NewLimitChecker(DefaultLimits())
	out := materialize(documentTarget(doc), s, RefType(s.Root), RootPath(), checker, &result)
	return targetToDocument(out), result, nil
}

// targetToDocument adapts a materialized Target back to a Document, the
// inverse of validate.go's documentTarget.
func targetToDocument(t Target) Document {
	if t.IsNode() {
		n, _ := t.Node()
		return NodeDocument(n)
	}
	v, _ := t.Value()
	return ValueDocument(v)
}

// materialize is `materialize(node, S, t, path, result)` from §7.2.1: it
// resolves t and dispatches to the scalar or record upgrade, or stops
// descent for `any` and returns the subtree untouched (spec §7.2's
// any-boundary rule, consistent with validate's own any handling: "the
// subtree passes through untouched", not just unchecked but *unconverted*
// too — this is the one point where materialize's any behavior needs its
// own sentence beyond "same as validate", since validate has nothing to
// convert in the first place).
func materialize(target Target, s Schema, t Type, path Path, checker *LimitChecker, result *[]Diagnostic) Target {
	r := resolveType(s, t)
	switch r.kind {
	case resolvedScalar:
		return materializeScalar(target, r, path, result)
	case resolvedRecord:
		return materializeRecord(target, s, r.record, path, checker, result)
	default:
		// resolvedAny (spec §7.2's any-boundary rule: return target
		// completely unconverted), and — since materialize, unlike
		// conform in validate.go, must return a Target on every path —
		// the same fallback for resolvedKind's closed set. resolveType
		// only ever produces resolvedAny/resolvedScalar/resolvedRecord,
		// so this default is exercised exclusively by the any case in
		// practice, matching conform's three-case switch one-for-one.
		return target
	}
}

// materializeRecord is `materialize_record(node, S, rec, path, result)`
// from §7.2.1. It shares validate.go's walkRecordShape for every
// shape/cardinality check (node-ness, depth limit, unexpected fields,
// cardinality) and supplies only the one piece specific to materialize:
// what to do with a matched field's target, namely recurse via
// materialize instead of conform.
func materializeRecord(target Target, s Schema, rec *Record, path Path, checker *LimitChecker, result *[]Diagnostic) Target {
	edges := walkRecordShape(target, rec, path, checker, result, func(e Edge, f *Field, childPath Path) Target {
		return materialize(e.Target, s, f.Type, childPath, checker, result)
	})
	if !target.IsNode() {
		// walkRecordShape already reported shape-mismatch; nothing to
		// rebuild, hand the original (non-node) target back unchanged.
		return target
	}
	return NodeTarget(&Node{Edges: edges})
}

// materializeScalar is `materialize_scalar(node, s, path, result)` from
// §7.2.1: the leaf-level counterpart of validate.go's conformScalar, but
// converting instead of merely checking.
func materializeScalar(target Target, r resolved, path Path, result *[]Diagnostic) Target {
	if target.IsNode() {
		addDiagnostic(result, path, CodeValidateShapeMismatch, "expected a scalar value, got an object")
		return target
	}
	v, _ := target.Value()
	if v.IsNull {
		if !r.nullable {
			addDiagnostic(result, path, CodeValidateNullNotAllowed, "null not allowed here")
		}
		return target // null is never upgraded, matching kind or not.
	}
	upgraded, ok := tryUpgrade(v.Scalar, r.scalarKind)
	if !ok {
		addDiagnostic(result, path, CodeMaterializeInexactConversion, "value cannot be upgraded to the declared kind without loss")
		return target
	}
	return ValueTarget(ScalarValue(upgraded))
}

// tryUpgrade is `try_upgrade(scalar, target_kind)` from §7.2.1: it
// upgrades scalar to targetKind only when doing so is value-exact (spec
// §7.2's materialization rule), per the pseudocode's explicit per-kind
// rules:
//
//   - Same kind already: always accepted, unchanged (this also covers the
//     case where source and target are both e.g. `date`, `time`, or
//     `datetime` — those three never convert into one another, spec
//     §7.2's "the two shapes are disjoint by construction").
//   - boolean: accepted only when the source is already boolean — never
//     treated as a subtype of integer or number in either direction, even
//     though some host languages consider bool a subtype of int (this
//     repo's ScalarKind enum has no such subtyping to begin with, but the
//     rule is enforced explicitly below rather than left to happen to be
//     true, since a future edit to the numeric branches must not silently
//     start accepting KindBoolean).
//   - integer target: accepted if the source is a float that is exactly
//     integral (no fractional part); undefined (rejected) otherwise —
//     e.g. a string numeral never upgrades, since that would invent a
//     numeric interpretation the value doesn't already carry.
//   - number target: accepted if the source is an integer, always
//     producing a genuinely float-typed (KindNumber) result even when the
//     source integer "looks whole" — §7.2's easy-to-miss detail that the
//     table's "Yes" does not itself spell out the target representation.
//   - date/time/datetime targets: accepted only if the source is a string
//     matching that exact kind's spelling per matchesTemporalKind (reused
//     from oml_lexer.go's own lexical regexes — see that function's doc
//     comment for why reuse rather than a new parser is correct here).
//   - anything else: undefined (rejected).
func tryUpgrade(s Scalar, targetKind ScalarKind) (Scalar, bool) {
	if s.Kind == targetKind {
		return s, true
	}
	switch targetKind {
	case KindInteger:
		if s.Kind != KindNumber {
			return Scalar{}, false
		}
		return integerFromExactFloat(s.Num)
	case KindNumber:
		if s.Kind != KindInteger {
			return Scalar{}, false
		}
		f, _ := new(big.Float).SetInt(s.Int).Float64()
		return NewNumberScalar(f), true
	case KindDate:
		if s.Kind != KindString || !matchesTemporalKind(s.Str, temporalDate) {
			return Scalar{}, false
		}
		return NewDateScalar(parseDateValue(s.Str)), true
	case KindTime:
		if s.Kind != KindString || !matchesTemporalKind(s.Str, temporalTime) {
			return Scalar{}, false
		}
		return NewTimeScalar(parseTimeValue(s.Str)), true
	case KindDateTime:
		if s.Kind != KindString || !matchesTemporalKind(s.Str, temporalDateTime) {
			return Scalar{}, false
		}
		return NewDateTimeScalar(parseDateTimeValue(s.Str)), true
	default:
		// KindString, KindBoolean: no cross-kind upgrade path exists for
		// either (boolean per the rule above; string is never produced by
		// upgrading a non-string source — materialize only ever narrows
		// strings into date/time/datetime/integer, never the reverse).
		return Scalar{}, false
	}
}

// integerFromExactFloat implements the integer-target half of §7.2.1's
// numeric try_upgrade rule: accept a float source only when it is
// finite and has no fractional part, converting it to the arbitrary-
// precision integer representation §2.4 requires (document.go's
// NewIntegerScalar/*big.Int) exactly, not via a fixed-width int64 that
// could silently truncate a very large whole-number float.
func integerFromExactFloat(f float64) (Scalar, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Scalar{}, false
	}
	bf := big.NewFloat(f)
	if !bf.IsInt() {
		return Scalar{}, false
	}
	bi, _ := bf.Int(nil)
	return NewIntegerScalar(bi), true
}

// temporalKind selects which of oml_lexer.go's three fully-anchored
// temporal regexes matchesTemporalKind checks against.
type temporalKind int

const (
	temporalDate temporalKind = iota
	temporalTime
	temporalDateTime
)

// matchesTemporalKind reports whether s is *exactly* (start to end) the
// spelling reDate/reTime/reDateTime (oml_lexer.go) accept for kind — the
// "exact spelling matches_kind() accepts for that specific kind, not
// merely parseable by a looser library function" rule from §7.2's
// try_upgrade notes. oml_lexer.go's own use of these regexes is via
// FindString on a remaining-input tail (correct for a lexer scanning
// forward through a longer document), which only pins the *start* of a
// match, not the end — reusing them here needs a full-string check, so
// this wraps FindString with an explicit "the match is the whole string"
// comparison rather than introducing new regexes with different anchoring
// (which is exactly the kind of drift the issue warns against: "reuse the
// EXACT same format-matching logic rather than writing a new, possibly
// looser or stricter parser").
//
// This is deliberately the OML lexer's spelling (§4's grammar), not
// toml_reader.go's parseTOMLDateTime (which documents itself as *wider*
// than oml_lexer.go's format: TOML allows a lowercase or space date/time
// separator and a bare 'Z'/'z' offset). materialize's source strings
// arrive from JSON/YAML/arbitrary Document construction, not specifically
// from a TOML read, and §7.2's rule is a single canonical spelling per
// kind — the OML lexer's spelling is that spelling, since OML is the
// format whose native literal syntax the temporal scalar kinds were
// designed around (§2.2.1/§4).
func matchesTemporalKind(s string, kind temporalKind) bool {
	var re *regexp.Regexp
	switch kind {
	case temporalDate:
		re = reDate
	case temporalTime:
		re = reTime
	case temporalDateTime:
		re = reDateTime
	}
	return re.FindString(s) == s
}
