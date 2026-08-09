package omnist_test

// External "omnist_test" package: see referee_test.go's comment for why.
// TestReachablePlainSkipsUnresolvedRef (the one lint.go test needing
// unexported access) stayed behind in lint_test.go.

import (
	"reflect"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- reachablePlain / unsatisfiable-record, unreachable-record (§6.11) ---

func TestLintUnsatisfiableRecordReachableNotRoot(t *testing.T) {
	// Root -> Mid -> Bad (mandatory self-cycle, unsatisfiable). Bad is
	// reachable from root (through Mid) but not the root itself.
	s := mustParseOSD(t, `
		record Root { "mid": Mid }
		record Mid { "bad": Bad }
		record Bad { "self": Bad }
		root Root
	`)
	findings := omnist.Lint(s)
	found := false
	for _, f := range findings {
		if f.Code == omnist.CodeLintUnsatisfiableRecord && f.Location == "Bad" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsatisfiable-record for Bad, got %+v", findings)
	}
}

func TestLintUnreachableAndUnsatisfiableNotDoubleReported(t *testing.T) {
	// Orphan is unsatisfiable AND unreachable from root. It must show up
	// only as unreachable-record, never also as unsatisfiable-record,
	// since unsatisfiable-record = reach - sat and Orphan isn't in reach.
	s := mustParseOSD(t, `
		record Root { "a": string }
		record Orphan { "self": Orphan }
		root Root
	`)
	findings := omnist.Lint(s)
	sawUnsat := false
	sawUnreach := false
	for _, f := range findings {
		if f.Location != "Orphan" {
			continue
		}
		if f.Code == omnist.CodeLintUnsatisfiableRecord {
			sawUnsat = true
		}
		if f.Code == omnist.CodeLintUnreachableRecord {
			sawUnreach = true
		}
	}
	if sawUnsat {
		t.Fatalf("Orphan should NOT be reported as unsatisfiable-record (not reachable): %+v", findings)
	}
	if !sawUnreach {
		t.Fatalf("Orphan should be reported as unreachable-record: %+v", findings)
	}
}

func TestLintUnreachableRecordNeverReferenced(t *testing.T) {
	s := mustParseOSD(t, `
		record Root { "a": string }
		record Dangling { "a": string }
		root Root
	`)
	findings := omnist.Lint(s)
	found := false
	for _, f := range findings {
		if f.Code == omnist.CodeLintUnreachableRecord && f.Location == "Dangling" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unreachable-record for Dangling, got %+v", findings)
	}
}

func TestLintPlainWalkDiffersFromPruneOptionalField(t *testing.T) {
	// Target is reachable ONLY through an optional field. Prune's
	// reachability would still count it as reachable too (optional fields
	// aren't stripped from reachability unless they point at an
	// unsatisfiable record) -- so instead construct the sharper case: a
	// field whose target is unsatisfiable, reached only via that
	// optional+unsatisfiable field. Prune's pruneRecord strips such a
	// field (min==0 and target unsatisfiable), so Prune's reachability
	// walk never follows it -- but lint's plain walk always does.
	s := mustParseOSD(t, `
		record Root { "maybe" [0,1]: Bad }
		record Bad { "self": Bad }
		root Root
	`)

	// Confirm Prune actually drops Bad (proving the two walks differ).
	pruned := omnist.Prune(s)
	if _, stillThere := pruned.Env["Bad"]; stillThere {
		t.Fatalf("test setup invalid: Prune should have dropped Bad, env = %+v", pruned.Env)
	}

	// lint's plain walk must still count Bad as reached (reach), so it
	// must NOT appear as unreachable-record.
	findings := omnist.Lint(s)
	for _, f := range findings {
		if f.Code == omnist.CodeLintUnreachableRecord && f.Location == "Bad" {
			t.Fatalf("Bad reached via optional+unsatisfiable field should NOT be unreachable-record under lint's plain walk: %+v", findings)
		}
	}
	// It's still unsatisfiable, and (per the plain walk) reachable, so it
	// must appear as unsatisfiable-record.
	found := false
	for _, f := range findings {
		if f.Code == omnist.CodeLintUnsatisfiableRecord && f.Location == "Bad" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsatisfiable-record for Bad (reached via plain walk): %+v", findings)
	}
}

// --- duplicate-record (§6.11 / §6.8) ---

func TestLintDuplicateRecordOneFindingPerBlock(t *testing.T) {
	s := mustParseOSD(t, `
		record Root { "a": A, "b": B }
		record A { "x": string }
		record B { "x": string }
		root Root
	`)
	findings := omnist.Lint(s)
	count := 0
	var loc string
	for _, f := range findings {
		if f.Code == omnist.CodeLintDuplicateRecord {
			count++
			loc = f.Location
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one duplicate-record finding, got %d: %+v", count, findings)
	}
	if loc != "A, B" {
		t.Fatalf("expected location %q, got %q", "A, B", loc)
	}
}

func TestLintDuplicateRecordFiresOnRawSchemaEvenIfUnreachable(t *testing.T) {
	// C is unreachable from root, and is a structural duplicate of A
	// (which IS reachable). normalize/prune would drop C entirely (it's
	// unreachable), but lint calls equivalence_classes on the schema as
	// authored, so the duplicate must still be reported.
	s := mustParseOSD(t, `
		record Root { "a": A }
		record A { "x": string }
		record C { "x": string }
		root Root
	`)
	// Confirm C really is unreachable (test setup sanity check).
	normalized := omnist.Normalize(s)
	if _, stillThere := normalized.Env["C"]; stillThere {
		t.Fatalf("test setup invalid: Normalize should have dropped unreachable C")
	}

	findings := omnist.Lint(s)
	found := false
	for _, f := range findings {
		if f.Code == omnist.CodeLintDuplicateRecord && f.Location == "A, C" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate-record for unreachable-but-duplicate A, C: %+v", findings)
	}
}

func TestLintDuplicateRecordBlockOfThreeIsOneFinding(t *testing.T) {
	s := mustParseOSD(t, `
		record Root { "a": A, "b": B, "c": C }
		record A { "x": string }
		record B { "x": string }
		record C { "x": string }
		root Root
	`)
	findings := omnist.Lint(s)
	count := 0
	var loc string
	for _, f := range findings {
		if f.Code == omnist.CodeLintDuplicateRecord {
			count++
			loc = f.Location
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one duplicate-record finding for a 3-block, got %d: %+v", count, findings)
	}
	if loc != "A, B, C" {
		t.Fatalf("expected location %q, got %q", "A, B, C", loc)
	}
}

// --- any-field (§6.11) ---

func TestLintAnyFieldMultipleAcrossRecords(t *testing.T) {
	s := mustParseOSD(t, `
		record Root { "child": Child, "x": any }
		record Child { "y": any }
		root Root
	`)
	findings := omnist.Lint(s)
	locs := map[string]bool{}
	for _, f := range findings {
		if f.Code == omnist.CodeLintAnyField {
			locs[f.Location] = true
		}
	}
	want := map[string]bool{"Root.x": true, "Child.y": true}
	if !reflect.DeepEqual(locs, want) {
		t.Fatalf("any-field locations = %+v, want %+v", locs, want)
	}
}

// --- determinism ---

func TestLintFindingsSortedByCodeThenLocation(t *testing.T) {
	s := mustParseOSD(t, `
		record Root { "a": A, "b": B, "orphanRef": Orphan }
		record A { "x": string, "opener": any }
		record B { "x": string }
		record Orphan { "self": Orphan }
		record Dangling { "y": string }
		root Root
	`)
	findings := omnist.Lint(s)
	if len(findings) < 2 {
		t.Fatalf("expected multiple findings to exercise sort, got %+v", findings)
	}
	for i := 1; i < len(findings); i++ {
		prev, cur := findings[i-1], findings[i]
		if prev.Code > cur.Code {
			t.Fatalf("findings not sorted by code: %+v then %+v", prev, cur)
		}
		if prev.Code == cur.Code && prev.Location > cur.Location {
			t.Fatalf("findings not sorted by location within code: %+v then %+v", prev, cur)
		}
	}
}

// --- zero findings ---

func TestLintZeroFindingsReturnsEmptySlice(t *testing.T) {
	s := mustParseOSD(t, `record R { "a": string } root R`)
	findings := omnist.Lint(s)
	if findings == nil {
		t.Fatalf("expected a non-nil empty slice, got nil")
	}
	if len(findings) != 0 {
		t.Fatalf("expected zero findings for a clean schema, got %+v", findings)
	}
}

// --- no mutation ---

func TestLintDoesNotMutateInput(t *testing.T) {
	s := mustParseOSD(t, `
		record Root { "a": A, "b": B }
		record A { "x": string }
		record B { "x": string }
		root Root
	`)
	envBefore := make(map[string]*omnist.Record, len(s.Env))
	for k, v := range s.Env {
		envBefore[k] = v
	}
	orderBefore := make([]string, len(s.EnvOrder))
	copy(orderBefore, s.EnvOrder)
	rootBefore := s.Root

	_ = omnist.Lint(s)

	if s.Root != rootBefore {
		t.Fatalf("Lint mutated Root: got %q, want %q", s.Root, rootBefore)
	}
	if !reflect.DeepEqual(s.EnvOrder, orderBefore) {
		t.Fatalf("Lint mutated EnvOrder: got %+v, want %+v", s.EnvOrder, orderBefore)
	}
	if len(s.Env) != len(envBefore) {
		t.Fatalf("Lint mutated Env size: got %d entries, want %d", len(s.Env), len(envBefore))
	}
	for k, v := range envBefore {
		if s.Env[k] != v {
			t.Fatalf("Lint mutated Env[%q]: pointer changed", k)
		}
	}
}
