package omnist

import (
	"sort"
	"testing"
)

// --- le (§6.2) ---

func TestLe(t *testing.T) {
	cases := []struct {
		name       string
		x          uint64
		xUnbounded bool
		y          uint64
		yUnbounded bool
		want       bool
	}{
		{"y unbounded always true", 100, false, 0, true, true},
		{"both unbounded true", 5, true, 5, true, true},
		{"x unbounded y bounded false", 5, true, 100, false, false},
		{"both bounded x<=y", 3, false, 5, false, true},
		{"both bounded x==y", 5, false, 5, false, true},
		{"both bounded x>y", 6, false, 5, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := le(c.x, c.xUnbounded, c.y, c.yUnbounded)
			if got != c.want {
				t.Errorf("le(%d,%v,%d,%v) = %v, want %v", c.x, c.xUnbounded, c.y, c.yUnbounded, got, c.want)
			}
		})
	}
}

// --- satisfiable_set / is_empty (§6.4) ---

func TestSatisfiableSetMandatorySelfCycleUnsatisfiable(t *testing.T) {
	s := mustParseOSD(t, `record Node { "child": Node } root Node`)
	sat := SatisfiableSet(s)
	if sat["Node"] {
		t.Fatalf("Node should be unsatisfiable, got sat = %+v", sat)
	}
}

func TestSatisfiableSetOptionalCycleSatisfiable(t *testing.T) {
	s := mustParseOSD(t, `record Node { "child" [0,1]: Node } root Node`)
	sat := SatisfiableSet(s)
	if !sat["Node"] {
		t.Fatalf("Node should be satisfiable, got sat = %+v", sat)
	}
}

func TestSatisfiableSetAnyFieldSatisfiable(t *testing.T) {
	s := mustParseOSD(t, `record R { "a": string, "b": any } root R`)
	sat := SatisfiableSet(s)
	if !sat["R"] {
		t.Fatalf("R (all scalar/any mandatory fields) should be satisfiable, got sat = %+v", sat)
	}
}

func TestSatisfiableSetTransitive(t *testing.T) {
	// Leaf satisfiable directly; Mid depends on Leaf; Root depends on Mid.
	// Confirms the fixpoint propagates across more than one pass.
	s := mustParseOSD(t, `
		record Leaf { "v": string }
		record Mid { "l": Leaf }
		record Root { "m": Mid }
		root Root
	`)
	sat := SatisfiableSet(s)
	for _, name := range []string{"Leaf", "Mid", "Root"} {
		if !sat[name] {
			t.Errorf("%s should be satisfiable, got sat = %+v", name, sat)
		}
	}
}

func TestSatisfiableSetUnsatisfiableBlocksDependents(t *testing.T) {
	s := mustParseOSD(t, `
		record Node { "child": Node }
		record Holder { "n": Node }
		root Holder
	`)
	sat := SatisfiableSet(s)
	if sat["Node"] {
		t.Errorf("Node should be unsatisfiable")
	}
	if sat["Holder"] {
		t.Errorf("Holder should be unsatisfiable (mandatory field of type unsatisfiable Node)")
	}
}

func TestIsEmptyTrueWhenRootUnsatisfiable(t *testing.T) {
	s := mustParseOSD(t, `record Node { "child": Node } root Node`)
	if !IsEmpty(s) {
		t.Fatalf("expected IsEmpty true for unsatisfiable root")
	}
}

func TestIsEmptyFalseWhenRootSatisfiable(t *testing.T) {
	s := mustParseOSD(t, `record R { "a": string } root R`)
	if IsEmpty(s) {
		t.Fatalf("expected IsEmpty false for satisfiable root")
	}
}

// TestSatisfiableSetDeterministicOrderIndependence rebuilds an equivalent
// schema several times (varying declaration order of otherwise-identical
// records isn't directly expressible via the OSD surface used elsewhere in
// this suite, so this instead confirms that repeated runs over the same
// schema produce byte-identical results, and that the result set itself,
// sorted, is stable across runs -- guarding against any reliance on Go's
// native (randomized) map iteration leaking into the result).
func TestSatisfiableSetDeterministicAcrossRuns(t *testing.T) {
	s := mustParseOSD(t, `
		record A { "b": B }
		record B { "c": C }
		record C { "v": string }
		root A
	`)
	var first []string
	for i := 0; i < 25; i++ {
		sat := SatisfiableSet(s)
		var names []string
		for name, ok := range sat {
			if ok {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		if first == nil {
			first = names
			continue
		}
		if len(first) != len(names) {
			t.Fatalf("run %d: got %v, want %v", i, names, first)
		}
		for j := range first {
			if first[j] != names[j] {
				t.Fatalf("run %d: got %v, want %v", i, names, first)
			}
		}
	}
}

// --- prune (§6.5) ---

func TestPruneRemovesUnreachableRecords(t *testing.T) {
	s := mustParseOSD(t, `
		record R { "a": string }
		record Unreachable { "x": string }
		root R
	`)
	p := Prune(s)
	if _, ok := p.Env["Unreachable"]; ok {
		t.Fatalf("Unreachable should have been removed, env = %+v", p.Env)
	}
	if _, ok := p.Env["R"]; !ok {
		t.Fatalf("R should remain")
	}
}

func TestPruneRemovesMaxZeroFields(t *testing.T) {
	s := mustParseOSD(t, `record R { "a" [0,0]: string, "b": string } root R`)
	p := Prune(s)
	rec := p.Env["R"]
	if len(rec.Fields) != 1 || rec.Fields[0].Label != "b" {
		t.Fatalf("expected only field b to survive, got %+v", rec.Fields)
	}
}

func TestPruneRemovesOptionalUnsatisfiableFields(t *testing.T) {
	s := mustParseOSD(t, `
		record Node { "child": Node }
		record R { "n" [0,1]: Node, "v": string }
		root R
	`)
	p := Prune(s)
	rec := p.Env["R"]
	if len(rec.Fields) != 1 || rec.Fields[0].Label != "v" {
		t.Fatalf("expected only field v to survive, got %+v", rec.Fields)
	}
}

func TestPruneRemovesRecordsLeftUnreachableAfterFieldPruning(t *testing.T) {
	s := mustParseOSD(t, `
		record Node { "child": Node }
		record R { "n" [0,1]: Node, "v": string }
		root R
	`)
	p := Prune(s)
	if _, ok := p.Env["Node"]; ok {
		t.Fatalf("Node should have been dropped once R's only reference to it was pruned, env = %+v", p.Env)
	}
}

func TestPruneRootUnsatisfiableKeepsRootFieldsUntouched(t *testing.T) {
	s := mustParseOSD(t, `
		record Node { "child": Node, "extra" [0,0]: string }
		root Node
	`)
	p := Prune(s)
	rec := p.Env["Node"]
	// The root is unsatisfiable, so field pruning (including the max==0
	// removal that would normally strip "extra") MUST NOT apply to it.
	if len(rec.Fields) != 2 {
		t.Fatalf("root-unsatisfiable case: root fields must be kept as-written, got %+v", rec.Fields)
	}
}

func TestPruneRootUnsatisfiableReachabilityFollowsEveryRootField(t *testing.T) {
	// Root is mandatorily self-recursive (unsatisfiable) and also has a
	// field, of cardinality [0,0], that would normally never be followed
	// once pruned -- but since the root is unsatisfiable, reachability MUST
	// follow every field of the root, not just the surviving ones. OnlyViaBadField
	// is reachable only through that [0,0] field, so it must be kept.
	s := mustParseOSD(t, `
		record Node { "child": Node, "hidden" [0,0]: OnlyViaBadField }
		record OnlyViaBadField { "v": string }
		root Node
	`)
	p := Prune(s)
	if _, ok := p.Env["OnlyViaBadField"]; !ok {
		t.Fatalf("OnlyViaBadField must be kept: reachability at an unsatisfiable root follows every field, env = %+v", p.Env)
	}
}

func TestPruneDeterministicOrderAcrossRuns(t *testing.T) {
	s := mustParseOSD(t, `
		record A { "b": B, "c": C }
		record B { "v": string }
		record C { "v": string }
		root A
	`)
	var first []string
	for i := 0; i < 25; i++ {
		p := Prune(s)
		if first == nil {
			first = append([]string(nil), p.EnvOrder...)
			continue
		}
		if len(first) != len(p.EnvOrder) {
			t.Fatalf("run %d: EnvOrder = %v, want %v", i, p.EnvOrder, first)
		}
		for j := range first {
			if first[j] != p.EnvOrder[j] {
				t.Fatalf("run %d: EnvOrder = %v, want %v", i, p.EnvOrder, first)
			}
		}
	}
}

func TestPruneEnvOrderMatchesDeclarationOrderFiltered(t *testing.T) {
	s := mustParseOSD(t, `
		record A { "b": B, "d": D }
		record B { "v": string }
		record C { "v": string }
		record D { "v": string }
		root A
	`)
	p := Prune(s)
	want := []string{"A", "B", "D"}
	if len(p.EnvOrder) != len(want) {
		t.Fatalf("EnvOrder = %v, want %v", p.EnvOrder, want)
	}
	for i := range want {
		if p.EnvOrder[i] != want[i] {
			t.Fatalf("EnvOrder = %v, want %v", p.EnvOrder, want)
		}
	}
}

func TestPruneUnboundedMaxFieldSurvives(t *testing.T) {
	s := mustParseOSD(t, `record R { "a" [0,]: string } root R`)
	p := Prune(s)
	rec := p.Env["R"]
	if len(rec.Fields) != 1 {
		t.Fatalf("unbounded-max field must survive prune, got %+v", rec.Fields)
	}
}

// TestPruneReachabilityDanglingRefStopsWithoutPanic constructs a Schema by
// hand (bypassing ReadOSD, whose S-6 well-formedness check would reject a
// dangling reference) to exercise reachableFromRoot's defensive nil-record
// branch: a Ref naming something absent from Env. Not reachable via any
// legal OSD input, but reachableFromRoot must not panic on it (mirrors
// resolveType's handling of the same situation in validate.go).
func TestPruneReachabilityDanglingRefStopsWithoutPanic(t *testing.T) {
	s := Schema{
		Root: "R",
		Env: map[string]*Record{
			"R": {Name: "R", Fields: []Field{{Label: "x", Type: RefType("Missing"), Cardinality: DefaultCardinality()}}},
		},
		EnvOrder: []string{"R"},
	}
	p := Prune(s)
	if _, ok := p.Env["R"]; !ok {
		t.Fatalf("R should remain, env = %+v", p.Env)
	}
	if _, ok := p.Env["Missing"]; ok {
		t.Fatalf("Missing should not appear, env = %+v", p.Env)
	}
}

func TestPruneKeepsSatisfiableOptionalRefField(t *testing.T) {
	s := mustParseOSD(t, `
		record Leaf { "v": string }
		record R { "n" [0,1]: Leaf }
		root R
	`)
	p := Prune(s)
	rec := p.Env["R"]
	if len(rec.Fields) != 1 || rec.Fields[0].Label != "n" {
		t.Fatalf("optional field to a satisfiable record must survive, got %+v", rec.Fields)
	}
	if _, ok := p.Env["Leaf"]; !ok {
		t.Fatalf("Leaf must remain reachable")
	}
}
