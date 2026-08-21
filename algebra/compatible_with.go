package algebra

import "github.com/omnist-dev/omnist-go"

// This file implements §6.6's compatible_with (and its scalar_sub and
// record_sub helpers) and §6.7's equivalent — port-order step 6. It builds
// on algebra.go's le (§6.2) and SatisfiableSet (§6.4), both from issue #9,
// reused here rather than reimplemented.

// scalarSub implements §6.3's `scalar_sub(a, b)` exactly: there is exactly
// one subtyping relation between scalar kinds (`integer <: number`), and
// nothing else. Nullable-narrowing (a nullable, b not) is checked first and
// blocks regardless of kind; nullable-widening (a not nullable, b nullable)
// is fine and falls through to the kind check.
func scalarSub(a, b omnist.Type) bool {
	if a.Nullable && !b.Nullable {
		return false
	}
	if a.ScalarKind == b.ScalarKind {
		return true
	}
	return a.ScalarKind == omnist.KindInteger && b.ScalarKind == omnist.KindNumber
}

// subKey is the coinductive memo key for sub: the identity of the two
// *resolved* definitions, one from each side. §6.6 is explicit and
// load-bearing about this: "The memo key MUST be the identity of the
// resolved definitions, not the reference names. Two names bound to the
// same record definition are the same node in the graph, and keying on
// names would defeat cycle detection where aliasing is present."
//
// A *Record pointer already carries identity within one schema's Env: two
// field types that resolve to the same underlying record (aliasing — two
// different Ref names bound to the same *Record) produce the same pointer,
// so keying on the pointer (rather than the Ref's name string) makes them
// the same memo entry, which is exactly what correct cycle detection
// requires. Since sub compares records drawn from two different schemas
// (A and B), a pointer from A.Env and a pointer from B.Env must never be
// treated as the same node even in the structurally-impossible case they
// happened to be equal — using both pointers together as a two-field
// struct key (rather than e.g. hashing them into a single combined value)
// keeps the two sides distinguishable unconditionally, with no reliance on
// the two allocators never colliding.
//
// Scalar and Any resolved types have no *Record identity to key on. Only
// records recurse (scalar_sub and the Any clauses are non-recursive), so
// sub only ever needs to memoize the Record/Record case; the key is
// therefore typed directly in terms of *Record rather than a more general
// "resolved type" identity.
type subKey struct {
	da *omnist.Record
	db *omnist.Record
}

// CompatibleWith implements §6.6's `compatible_with(A, B)`: true when every
// Document A accepts is also accepted by B. A's satisfiable set is computed
// once up front (reusing SatisfiableSet from issue #9) and threaded through
// so an A-side record known unsatisfiable is treated as vacuously
// compatible with anything, without needing callers to prune first.
func CompatibleWith(a, b omnist.Schema) bool {
	satA := SatisfiableSet(a)
	memo := make(map[subKey]bool)
	return sub(a, omnist.RefType(a.Root), b, omnist.RefType(b.Root), satA, memo)
}

// sub implements §6.6's `sub(SA, ta, SB, tb, sat_a, memo)`.
func sub(sa omnist.Schema, ta omnist.Type, sb omnist.Schema, tb omnist.Type, satA map[string]bool, memo map[subKey]bool) bool {
	if ta.Kind == omnist.TypeRefKind && !satA[ta.RefName] {
		return true // vacuous: A emits nothing here
	}
	// omnist.ResolveType and the omnist.Resolved/omnist.ResolvedKind types
	// are validate.go's (issue #7), implementing the same §6.2 S.resolve(t)
	// notation this spec section uses; reused here rather than duplicated.
	da := omnist.ResolveType(sa, ta)
	db := omnist.ResolveType(sb, tb)

	// Only Record/Record pairs recurse and need memoization (see subKey's
	// doc comment); Scalar and Any resolved types have no *Record to key
	// on and are decided directly below without consulting memo.
	if da.Kind == omnist.ResolvedRecord && db.Kind == omnist.ResolvedRecord {
		key := subKey{da: da.Record, db: db.Record}
		if v, ok := memo[key]; ok {
			return v
		}
		memo[key] = true // coinductive assumption
		result := recordSub(sa, da.Record, sb, db.Record, satA, memo)
		memo[key] = result
		return result
	}

	switch {
	case db.Kind == omnist.ResolvedAny:
		return true // any absorbs all
	case da.Kind == omnist.ResolvedAny:
		return false // only any holds any
	case da.Kind == omnist.ResolvedScalar && db.Kind == omnist.ResolvedScalar:
		return scalarSub(
			omnist.ScalarType(da.ScalarKind, da.Nullable),
			omnist.ScalarType(db.ScalarKind, db.Nullable),
		)
	default:
		return false // value versus object
	}
}

// fieldByLabel implements §6.2's `R.field(label)`: the field with that
// label, or none.
func fieldByLabel(r *omnist.Record, label string) (omnist.Field, bool) {
	f := r.Index().Field(label)
	if f == nil {
		return omnist.Field{}, false
	}
	return *f, true
}

// recordSub implements ?6.6's `record_sub(SA, a, SB, b, sat_a, memo)`.
func recordSub(sa omnist.Schema, a *omnist.Record, sb omnist.Schema, b *omnist.Record, satA map[string]bool, memo map[subKey]bool) bool {
	bIdx := b.Index()
	aIdx := a.Index()

	// 1. Every label A may emit must be allowed by B.
	for _, fa := range a.Fields {
		if !fa.Cardinality.Unbounded && fa.Cardinality.Max == 0 {
			continue // A never emits it
		}
		if fa.Cardinality.Min == 0 && fa.Type.Kind == omnist.TypeRefKind && !satA[fa.Type.RefName] {
			continue // A never emits it either
		}
		fb := bIdx.Field(fa.Label)
		if fb == nil {
			return false // B is closed
		}
		if fb.Cardinality.Min > fa.Cardinality.Min ||
			!le(fa.Cardinality.Max, fa.Cardinality.Unbounded, fb.Cardinality.Max, fb.Cardinality.Unbounded) {
			return false
		}
		if !sub(sa, fa.Type, sb, fb.Type, satA, memo) {
			return false
		}
	}

	// 2. Every label B requires must be guaranteed by A.
	for _, fb := range b.Fields {
		if fb.Cardinality.Min >= 1 {
			fa := aIdx.Field(fb.Label)
			if fa == nil || fa.Cardinality.Min < fb.Cardinality.Min {
				return false
			}
		}
	}
	return true
}

// Equivalent implements ?6.7's `equivalent(A, B)`: exactly
// CompatibleWith(A, B) && CompatibleWith(B, A). The spec explicitly forbids
// any structural-equality shortcut here ? structural equality is strictly
// stronger than equivalence, so substituting one would reject equivalent
// schemas that merely differ in record naming, declaration order, or
// unreachable records.
func Equivalent(a, b omnist.Schema) bool {
	return CompatibleWith(a, b) && CompatibleWith(b, a)
}
