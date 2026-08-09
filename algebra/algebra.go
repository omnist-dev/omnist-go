package algebra

import "github.com/omnist-dev/omnist-go"

// This file begins the schema algebra (spec ch.6): operations over Schema
// alone (schema.go, issue #5's types), never over Document. This issue
// (#9, port-order step 5) implements §6.2's le helper, §6.4's
// satisfiable_set/is_empty, and §6.5's prune. Later issues
// (compatible_with, normalize, extract, lint) land in this same file or
// siblings named after the operation they add — algebra.go is kept for the
// operations that don't warrant their own file.

// le implements §6.2's `le(x, y)`: the unbounded-aware `<=` used wherever a
// maximum is compared throughout this chapter. x and y are each a value
// paired with an "is this unbounded" flag, matching how Cardinality.Max is
// only meaningful when Cardinality.Unbounded is false (schema.go).
// Unbounded is treated as +infinity per the spec's pseudocode:
//
//	function le(x, y):
//	    if y is unbounded: return true
//	    if x is unbounded: return false
//	    return x <= y
func le(x uint64, xUnbounded bool, y uint64, yUnbounded bool) bool {
	if yUnbounded {
		return true
	}
	if xUnbounded {
		return false
	}
	return x <= y
}

// SatisfiableSet implements §6.4's `satisfiable_set(S)`: the least fixpoint
// of "a record is satisfiable if every mandatory field's type is
// satisfiable" (scalars and `any` always count as satisfiable; optional
// fields never block). env is finite and the set only grows each pass, so
// this terminates (§6.12).
//
// Iteration over S.Env is done via S.EnvOrder (declaration order), per the
// determinism requirement at the end of §6.4 and the issue #9 follow-up
// comment directing every deterministic-iteration need in this issue to
// EnvOrder rather than Go's (randomized) native map iteration.
func SatisfiableSet(s omnist.Schema) map[string]bool {
	sat := make(map[string]bool, len(s.EnvOrder))
	for {
		changed := false
		for _, name := range s.EnvOrder {
			if sat[name] {
				continue
			}
			if recordSatisfiable(s.Env[name], sat) {
				sat[name] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return sat
}

// recordSatisfiable implements §6.4's `record_satisfiable(rec, sat)`.
func recordSatisfiable(rec *omnist.Record, sat map[string]bool) bool {
	for _, f := range rec.Fields {
		if f.Cardinality.Min < 1 {
			continue // optional never blocks
		}
		if f.Type.Kind == omnist.TypeScalarKind || f.Type.Kind == omnist.TypeAnyKind {
			continue
		}
		if !sat[f.Type.RefName] {
			return false
		}
	}
	return true
}

// IsEmpty implements §6.4's `is_empty(S)`: true when the root record is not
// satisfiable, i.e. no finite Document can ever match S.
func IsEmpty(s omnist.Schema) bool {
	return !SatisfiableSet(s)[s.Root]
}

// Prune implements §6.5's `prune(S)`: a schema equivalent to S (accepts
// exactly the same Documents) with unreachable records, never-emittable
// fields (max == 0), and optional fields pointing at unsatisfiable records
// all removed, plus any record left unreachable as a result.
//
// The root-unsatisfiable case is normative and special-cased here exactly
// as §6.5 specifies: if the root itself is unsatisfiable, its fields are
// NOT pruned (they're exactly what makes it unsatisfiable; stripping them
// would change the accepted language), and reachability from the root
// follows every one of its fields rather than only the surviving ones.
func Prune(s omnist.Schema) omnist.Schema {
	sat := SatisfiableSet(s)
	rootOK := sat[s.Root]
	keep := reachableFromRoot(s, sat, rootOK)

	// Determinism requirement (§6.5, called out as having bitten a real
	// implementation): the output order MUST come from iterating S.EnvOrder
	// (declaration order) and filtering by keep, never from iterating keep
	// itself, which as a map has no defined order.
	newEnv := make(map[string]*omnist.Record, len(keep))
	newOrder := make([]string, 0, len(keep))
	for _, name := range s.EnvOrder {
		if !keep[name] {
			continue
		}
		if !rootOK && name == s.Root {
			newEnv[name] = s.Env[name] // keep the bad root intact, unpruned
		} else {
			newEnv[name] = pruneRecord(s.Env[name], sat)
		}
		newOrder = append(newOrder, name)
	}
	return omnist.Schema{Root: s.Root, Env: newEnv, EnvOrder: newOrder}
}

// pruneRecord implements §6.5's `prune_record(rec, sat)`.
func pruneRecord(rec *omnist.Record, sat map[string]bool) *omnist.Record {
	kept := make([]omnist.Field, 0, len(rec.Fields))
	for _, f := range rec.Fields {
		if !f.Cardinality.Unbounded && f.Cardinality.Max == 0 {
			continue
		}
		if f.Cardinality.Min == 0 && f.Type.Kind == omnist.TypeRefKind && !sat[f.Type.RefName] {
			continue
		}
		kept = append(kept, f)
	}
	return &omnist.Record{Name: rec.Name, Fields: kept}
}

// reachableFromRoot implements §6.5's `reachable(S, sat, root_ok)`: a walk
// from the root following references through the fields that would survive
// prune_record — except at an unsatisfiable root, where every field is
// followed regardless (the root-unsatisfiable special case).
func reachableFromRoot(s omnist.Schema, sat map[string]bool, rootOK bool) map[string]bool {
	visited := make(map[string]bool)
	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		rec := s.Env[name]
		if rec == nil {
			// A reference that doesn't resolve in env indicates a
			// not-well-formed schema (spec §3.3 S-6), a schema.*
			// well-formedness concern from issue #5, not something prune
			// re-checks (mirrors resolveType's handling in validate.go).
			return
		}

		var fields []omnist.Field
		if name == s.Root && !rootOK {
			fields = rec.Fields // unfiltered: the root-unsatisfiable exception
		} else {
			fields = pruneRecord(rec, sat).Fields
		}
		for _, f := range fields {
			if f.Type.Kind == omnist.TypeRefKind {
				visit(f.Type.RefName)
			}
		}
	}
	visit(s.Root)
	return visited
}
