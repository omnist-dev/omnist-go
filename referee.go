package omnist

import "math"

// This file is the conformance harness's "referee": structural comparison
// primitives built from this repository's own types and equality, never
// another port's. See vendor/omnist-spec/docs/porting-a-conformance-runner.md
// ("What to build, in order", step 1) and issue #31.
//
// DocumentsEqual promotes the docEqual/nodeEqual/targetEqual/valueEqual/
// scalarEqual family that writer_testutil_test.go built as test-only
// helpers (for oml_writer_test.go/osd_writer_test.go's round-trip
// properties) to real, non-test-only referee code. SchemasEqual's "exact"
// mode promotes recordEqual/schemaEqual the same way, with one correction:
// the test-only recordEqual compared Fields by slice index, which is
// wrong for a referee (spec §3.1: a record's fields are an unordered set
// at the model layer, so field *declaration order* must not affect
// equality — see the referee self-test's case 01). SchemasEqual's
// "isomorphic" mode is new: no prior code in this repository attempted a
// schema-graph isomorphism check.

// DocumentsEqual reports whether a and b are the same Document: the same
// shape (node vs. bare value), the same edges in the same order (edge
// order is data, per spec §2.3 D-1/D-3 — this is why DocumentsEqual is
// order-sensitive, unlike SchemasEqual's field-set comparison), and the
// same scalar values including kind. Two NaN-valued KindNumber scalars
// compare equal here (plain Scalar.Equal, and Go's == underneath it, does
// not: NaN != NaN), matching the reserved "nan" OML spelling's intended
// round-trip semantics.
func DocumentsEqual(a, b Document) bool {
	if a.IsNode != b.IsNode {
		return false
	}
	if a.IsNode {
		return nodesEqual(a.Node, b.Node)
	}
	return valuesEqual(a.Value, b.Value)
}

func valuesEqual(a, b Value) bool {
	if a.IsNull != b.IsNull {
		return false
	}
	if a.IsNull {
		return true
	}
	return scalarsEqual(a.Scalar, b.Scalar)
}

// scalarsEqual defers to Scalar.Equal for every kind except KindNumber,
// where two NaNs compare equal (see DocumentsEqual's doc comment).
func scalarsEqual(a, b Scalar) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind == KindNumber && math.IsNaN(a.Num) && math.IsNaN(b.Num) {
		return true
	}
	return a.Equal(b)
}

func targetsEqual(a, b Target) bool {
	an, aok := a.Node()
	bn, bok := b.Node()
	if aok != bok {
		return false
	}
	if aok {
		return nodesEqual(an, bn)
	}
	av, _ := a.Value()
	bv, _ := b.Value()
	return valuesEqual(av, bv)
}

func nodesEqual(a, b *Node) bool {
	if len(a.Edges) != len(b.Edges) {
		return false
	}
	for i := range a.Edges {
		if a.Edges[i].Label != b.Edges[i].Label {
			return false
		}
		if !targetsEqual(a.Edges[i].Target, b.Edges[i].Target) {
			return false
		}
	}
	return true
}

// SchemaEqualityMode selects one of the two schema-comparison modes the
// porting guide calls for: ModeExact requires every record name and field
// to match; ModeIsomorphic requires the same structure up to consistent
// record renaming.
type SchemaEqualityMode string

const (
	// ModeExact is used for normalize/prune/extract, whose output naming
	// is spec-determined (§6's operations fix record names
	// deterministically, so a naming difference is a real divergence).
	ModeExact SchemaEqualityMode = "exact"
	// ModeIsomorphic is used only for infer, since §6.10's infer_type
	// never normalizes its output — two schemas that differ only in which
	// arbitrary names infer picked for its records are still the same
	// answer.
	ModeIsomorphic SchemaEqualityMode = "isomorphic"
)

// SchemasEqual reports whether a and b are the same Schema under mode.
//
// ModeExact: the root name must match, and every record in a's env must
// have a same-named counterpart in b's env (and vice versa, via the
// length check) whose fields are equal as a *set* keyed by label — field
// declaration order does not participate (spec §3.1; referee self-test
// case 01). A field is equal when its Type and Cardinality are equal;
// Type equality for TypeRefKind compares RefName literally in this mode
// (no renaming is permitted), which is what makes case 04 (same shape,
// different record names) correctly compare not-equal even though case
// 03's identical shape compares equal under ModeIsomorphic.
//
// ModeIsomorphic: same structure up to a consistent renaming of records,
// checked by walking both schemas' reference graphs from their respective
// roots in lock-step and building a bijection between record names as new
// ones are encountered (see schemaIsomorphic's doc comment for the
// algorithm and its scope).
func SchemasEqual(a, b Schema, mode SchemaEqualityMode) bool {
	switch mode {
	case ModeIsomorphic:
		return schemaIsomorphic(a, b)
	default:
		return schemaExactEqual(a, b)
	}
}

func schemaExactEqual(a, b Schema) bool {
	if a.Root != b.Root {
		return false
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for name, rec := range a.Env {
		other, ok := b.Env[name]
		if !ok || !recordFieldSetEqualExact(rec, other) {
			return false
		}
	}
	return true
}

// recordFieldSetEqualExact compares two records' fields as a set keyed by
// label (declaration order is not significant — spec §3.1), requiring
// literal Type equality (no record renaming permitted).
func recordFieldSetEqualExact(a, b *Record) bool {
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	byLabel := make(map[string]Field, len(b.Fields))
	for _, f := range b.Fields {
		byLabel[f.Label] = f
	}
	for _, fa := range a.Fields {
		fb, ok := byLabel[fa.Label]
		if !ok || fa.Cardinality != fb.Cardinality || !typesEqualExact(fa.Type, fb.Type) {
			return false
		}
	}
	return true
}

func typesEqualExact(a, b Type) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case TypeScalarKind:
		return a.ScalarKind == b.ScalarKind && a.Nullable == b.Nullable
	case TypeRefKind:
		return a.RefName == b.RefName
	default: // TypeAnyKind
		return true
	}
}

// schemaIsomorphic decides whether a and b are the same schema up to a
// consistent renaming of records, per the porting guide's step 1 and
// spec ch.6's use of isomorphism for infer.
//
// This walks both schemas' record-reference graphs together, starting
// from their respective roots, maintaining a bijection between record
// names discovered so far (aToB/bToA). Field labels are matched exactly
// (labels are never renamed — only record *names* can differ), so there
// is no field-pairing ambiguity to resolve by search: for a given pair of
// records already known to correspond, each field in a has at most one
// candidate counterpart in b (the field sharing its label), so the walk
// is a straightforward mutual recursion rather than a general graph-
// isomorphism search with backtracking. A first-encountered mapping is
// permanent: if the walk later needs aName to correspond to a different
// bName than it already committed to (or vice versa), that is treated as
// a mismatch. This is sufficient for schemas as they occur from OSD
// parsing and this repository's own operations (records reached from a
// root via a tree of Ref fields, cycles handled by the visited check
// below) — it is not a claim of solving general graph automorphism
// search, which this narrow domain does not need.
func schemaIsomorphic(a, b Schema) bool {
	aToB := map[string]string{}
	bToA := map[string]string{}
	return recordsIsomorphic(a, a.Root, b, b.Root, aToB, bToA)
}

func recordsIsomorphic(a Schema, aName string, b Schema, bName string, aToB, bToA map[string]string) bool {
	if mapped, ok := aToB[aName]; ok {
		return mapped == bName && bToA[bName] == aName
	}
	if _, taken := bToA[bName]; taken {
		// bName is already claimed by some other aName - not a
		// consistent bijection.
		return false
	}
	recA, okA := a.Env[aName]
	recB, okB := b.Env[bName]
	if !okA || !okB {
		return false
	}
	// Commit the mapping before recursing so cycles (mutual/self
	// references) terminate instead of looping forever.
	aToB[aName] = bName
	bToA[bName] = aName

	if len(recA.Fields) != len(recB.Fields) {
		return false
	}
	byLabel := make(map[string]Field, len(recB.Fields))
	for _, f := range recB.Fields {
		byLabel[f.Label] = f
	}
	for _, fa := range recA.Fields {
		fb, ok := byLabel[fa.Label]
		if !ok || fa.Cardinality != fb.Cardinality {
			return false
		}
		if fa.Type.Kind != fb.Type.Kind {
			return false
		}
		switch fa.Type.Kind {
		case TypeScalarKind:
			if fa.Type.ScalarKind != fb.Type.ScalarKind || fa.Type.Nullable != fb.Type.Nullable {
				return false
			}
		case TypeRefKind:
			if !recordsIsomorphic(a, fa.Type.RefName, b, fb.Type.RefName, aToB, bToA) {
				return false
			}
		default: // TypeAnyKind
		}
	}
	return true
}
