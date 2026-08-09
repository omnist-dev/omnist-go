package algebra

import (
	"fmt"

	"github.com/omnist-dev/omnist-go"
)

// This file implements §6.9's extract(S, keep) (port-order step 8): given a
// set of permissible labels, the minimal subschema recognizing only
// Documents built from those labels. It depends on Prune (issue #9,
// algebra.go), Normalize (issue #13, normalize.go), and the
// Schema/Record/Field/Type/Cardinality/EnvOrder types (issue #5).

// firstBad records the first offender (label, record name) encountered
// during Extract's step 1 declaration-order pass, per §6.9's determinism
// requirement: "first_bad records the first offender encountered during
// step 1's single pass over S.env in declaration order — not the first one
// propagation later discovers."
type firstBad struct {
	label  string
	record string
}

// Extract implements §6.9's `extract(S, keep)`. keep is the set of
// permissible labels; a label present as a key with a true value is kept
// (matching the issue's suggested `map[string]bool` representation).
//
// On success, the returned Schema has already been run through Prune then
// Normalize (step 5's final line), landing in the same canonical form
// Normalize produces everywhere else — callers MUST NOT re-normalize.
//
// On failure (the root ends up invalidated, step 4), the returned error is
// a Diagnostic with code algebra.extract-invalidates-root naming the first
// offending label and the record it invalidated.
func Extract(s omnist.Schema, keep map[string]bool) (omnist.Schema, error) {
	trimmed, invalidated, bad := extractFilterFields(s, keep)
	extractPropagate(s, trimmed, invalidated)

	if invalidated[s.Root] {
		return omnist.Schema{}, omnist.Diagnostic{
			Path:     bad.record,
			Code:     omnist.CodeAlgebraExtractInvalidatesRoot,
			Message:  fmt.Sprintf("removing label %q deletes a mandatory field of %s", bad.label, bad.record),
			Severity: omnist.SeverityError,
		}
	}

	newEnv, newOrder := extractBuildEnv(s, trimmed, invalidated)
	return Normalize(Prune(omnist.Schema{Root: s.Root, Env: newEnv, EnvOrder: newOrder})), nil
}

// extractFilterFields implements §6.9 steps 1+2: for every record (in
// EnvOrder, for determinism), filter fields to those in keep. A
// filtered-out mandatory field (min >= 1) invalidates that record and, if
// this is the first offender seen in this single pass, records it as
// first_bad. Per the spec's normative "deleting a mandatory field is an
// error, not a silent relaxation" rule, a deleted mandatory field is never
// downgraded to optional — the record is invalidated outright instead.
func extractFilterFields(s omnist.Schema, keep map[string]bool) (trimmed map[string]*omnist.Record, invalidated map[string]bool, bad firstBad) {
	trimmed = make(map[string]*omnist.Record, len(s.EnvOrder))
	invalidated = make(map[string]bool)
	for _, name := range s.EnvOrder {
		rec := s.Env[name]
		kept := make([]omnist.Field, 0, len(rec.Fields))
		for _, f := range rec.Fields {
			if keep[f.Label] {
				kept = append(kept, f)
				continue
			}
			if f.Cardinality.Min >= 1 {
				if bad.record == "" && bad.label == "" {
					bad = firstBad{label: f.Label, record: name}
				}
				invalidated[name] = true
			}
		}
		trimmed[name] = &omnist.Record{Name: rec.Name, Fields: kept}
	}
	return trimmed, invalidated, bad
}

// extractPropagate implements §6.9 step 3: a record with a mandatory field
// typed to an invalidated record is itself invalidated. Least fixpoint,
// same repeat-until-no-change-over-EnvOrder shape as SatisfiableSet
// (algebra.go) — the fixpoint condition differs (invalidation instead of
// satisfiability), so the function isn't reused directly.
func extractPropagate(s omnist.Schema, trimmed map[string]*omnist.Record, invalidated map[string]bool) {
	for {
		changed := false
		for _, name := range s.EnvOrder {
			if invalidated[name] {
				continue
			}
			for _, f := range trimmed[name].Fields {
				if f.Cardinality.Min >= 1 && f.Type.Kind == omnist.TypeRefKind && invalidated[f.Type.RefName] {
					invalidated[name] = true
					changed = true
					break
				}
			}
		}
		if !changed {
			break
		}
	}
}

// extractBuildEnv implements §6.9 step 5's new_env construction: drop
// invalidated records, and for surviving records drop any field —
// mandatory or not — that still points at an invalidated record, so no
// surviving record is left with a dangling reference. The result is
// returned unpruned/unnormalized; Extract runs it through Prune then
// Normalize itself.
func extractBuildEnv(s omnist.Schema, trimmed map[string]*omnist.Record, invalidated map[string]bool) (map[string]*omnist.Record, []string) {
	newEnv := make(map[string]*omnist.Record, len(s.EnvOrder))
	newOrder := make([]string, 0, len(s.EnvOrder))
	for _, name := range s.EnvOrder {
		if invalidated[name] {
			continue
		}
		rec := trimmed[name]
		fields := make([]omnist.Field, 0, len(rec.Fields))
		for _, f := range rec.Fields {
			if f.Type.Kind == omnist.TypeRefKind && invalidated[f.Type.RefName] {
				continue
			}
			fields = append(fields, f)
		}
		newEnv[name] = &omnist.Record{Name: name, Fields: fields}
		newOrder = append(newOrder, name)
	}
	return newEnv, newOrder
}
