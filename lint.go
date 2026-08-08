package omnist

import (
	"fmt"
	"sort"
	"strings"
)

// This file implements §6.11's lint(S) (port-order step 9): schema
// diagnostics for structures that parse fine but can never do anything.
// lint reports and MUST NOT mutate its input Schema. It depends on
// SatisfiableSet (issue #9, algebra.go) and EquivalenceClasses (issue #13,
// normalize.go), plus the Schema/Record/Field/Type/EnvOrder types (issue
// #5).
//
// lint's own reachability walk (reachablePlain, below) is deliberately a
// SEPARATE, simpler function from algebra.go's reachableFromRoot used by
// Prune: §6.11's reachable(S) follows every Ref-typed field unconditionally
// regardless of cardinality or satisfiability, while Prune's walk is
// language-aware (it only follows the fields that would survive
// prune_record, with a root-unsatisfiable special case). A record reached
// only through an optional field, or only through a field whose target
// turns out unsatisfiable, still counts as referenced for lint's purposes
// even though Prune might have dropped that same path. Reusing
// reachableFromRoot here would silently change lint's semantics, so this
// file builds its own walk straight from §6.11's own pseudocode instead.

// Finding is one diagnostic lint(S) reports: a Code from the lint.*
// family (errors.go), its Severity (warning or info per §6.11's table),
// a Location string (a record name, or for duplicate-record a
// comma-joined sorted list of names, or for any-field a
// "RecordName.label" string), and a human-readable Message.
type Finding struct {
	Code     Code
	Severity Severity
	Location string
	Message  string
}

// reachablePlain implements §6.11's own reachable(S) pseudocode: a
// stack-based walk from the root following every Ref-typed field
// unconditionally, regardless of cardinality or satisfiability. This is
// intentionally not algebra.go's reachableFromRoot (used by Prune), which
// computes a genuinely different, language-aware set — see the file
// comment above.
func reachablePlain(s Schema) map[string]bool {
	seen := make(map[string]bool)
	stack := []string{s.Root}
	for len(stack) > 0 {
		name := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[name] {
			continue
		}
		rec, ok := s.Env[name]
		if !ok {
			continue
		}
		seen[name] = true
		for _, f := range rec.Fields {
			if f.Type.Kind == TypeRefKind {
				stack = append(stack, f.Type.RefName)
			}
		}
	}
	return seen
}

// Lint implements §6.11's lint(S) exactly: the four finding categories in
// the order the pseudocode gives them, followed by a final deterministic
// sort by (code, location). Lint never mutates s — it only reads Env,
// EnvOrder, and Root.
func Lint(s Schema) []Finding {
	findings := []Finding{}

	reach := reachablePlain(s)
	sat := SatisfiableSet(s)

	// unsatisfiable-record: reach - sat. Iterate EnvOrder for determinism
	// (the final sort makes this immaterial for the returned order, but
	// every prior algebra issue iterates EnvOrder rather than native map
	// iteration for any per-record walk, and this file follows suit).
	for _, name := range s.EnvOrder {
		if reach[name] && !sat[name] {
			findings = append(findings, Finding{
				Code:     CodeLintUnsatisfiableRecord,
				Severity: SeverityWarning,
				Location: name,
				Message:  fmt.Sprintf("record %q is reachable from root but no finite Document can ever match it", name),
			})
		}
	}

	// unreachable-record: env.keys - reach.
	for _, name := range s.EnvOrder {
		if !reach[name] {
			findings = append(findings, Finding{
				Code:     CodeLintUnreachableRecord,
				Severity: SeverityWarning,
				Location: name,
				Message:  fmt.Sprintf("record %q is defined but not reachable from root by any reference", name),
			})
		}
	}

	// duplicate-record: equivalence_classes on the RAW schema (never
	// pruned first) -- one finding per oversized block, not per name.
	for _, block := range EquivalenceClasses(s) {
		if len(block) <= 1 {
			continue
		}
		group := make([]string, len(block))
		copy(group, block)
		sort.Strings(group)
		location := strings.Join(group, ", ")
		keep := group[0]
		others := strings.Join(group[1:], ", ")
		findings = append(findings, Finding{
			Code:     CodeLintDuplicateRecord,
			Severity: SeverityWarning,
			Location: location,
			Message:  fmt.Sprintf("records %s are structurally identical to %q; merge them with normalize", others, keep),
		})
	}

	// any-field: an inventory entry for every any-typed field, walking
	// records-then-fields in EnvOrder for determinism.
	for _, name := range s.EnvOrder {
		rec := s.Env[name]
		for _, f := range rec.Fields {
			if f.Type.Kind == TypeAnyKind {
				findings = append(findings, Finding{
					Code:     CodeLintAnyField,
					Severity: SeverityInfo,
					Location: name + "." + f.Label,
					Message:  fmt.Sprintf("field %q of record %q is `any`-typed", f.Label, name),
				})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].Location < findings[j].Location
	})

	return findings
}
