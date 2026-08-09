package algebra_test

// External "algebra_test" package: see
// algebra_external_test_helpers_test.go's comment for why. The
// normalize.go tests needing unexported access
// (computeLocalSignature/computeRefineKey/appendEscapedLabel/appendUint)
// stayed behind in normalize_test.go.

import (
	"reflect"
	"sort"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
)

// --- normalize (§6.8) ---

// TestNormalizeWorkedExample is the exact §6.8 worked example: A and B
// have identical local signatures and no reference fields, so they merge
// on the first pass, and Top is rewritten to reference the survivor (the
// lexicographic minimum, A).
func TestNormalizeWorkedExample(t *testing.T) {
	s := mustParseOSD(t, `
		record A { "x": string }
		record B { "x": string }
		record Top { "a": A, "b": B }
		root Top
	`)
	got := algebra.Normalize(s)

	if got.Root != "Top" {
		t.Fatalf("root = %q, want Top", got.Root)
	}
	wantNames := []string{"A", "Top"}
	if !reflect.DeepEqual(got.EnvOrder, wantNames) {
		t.Fatalf("EnvOrder = %v, want %v", got.EnvOrder, wantNames)
	}
	if _, ok := got.Env["B"]; ok {
		t.Fatalf("B should have been merged away, still present in env")
	}
	top := got.Env["Top"]
	if top == nil {
		t.Fatalf("Top missing from normalized env")
	}
	for _, f := range top.Fields {
		if f.Type.Kind != omnist.TypeRefKind || f.Type.RefName != "A" {
			t.Errorf("field %q = %+v, want ref to A", f.Label, f.Type)
		}
	}
}

// TestNormalizeDeterministicRepresentative confirms the same
// representative (lexicographic minimum) is chosen across repeated runs
// on schemas with several structurally-identical blocks, and confirms the
// choice really is the minimum rather than e.g. declaration order or map
// iteration order (built here by declaring names out of lexicographic
// order: Zeta before Alpha before Mimi, all with the same signature).
func TestNormalizeDeterministicRepresentative(t *testing.T) {
	src := `
		record Zeta { "x": string }
		record Alpha { "x": string }
		record Mimi { "x": string }
		record Top { "z": Zeta, "a": Alpha, "m": Mimi }
		root Top
	`
	for i := 0; i < 5; i++ {
		s := mustParseOSD(t, src)
		got := algebra.Normalize(s)
		if _, ok := got.Env["Alpha"]; !ok {
			t.Fatalf("run %d: expected representative Alpha to survive, got env %v", i, got.EnvOrder)
		}
		if len(got.EnvOrder) != 2 {
			t.Fatalf("run %d: expected 2 surviving records (Alpha, Top), got %v", i, got.EnvOrder)
		}
		top := got.Env["Top"]
		for _, f := range top.Fields {
			if f.Type.RefName != "Alpha" {
				t.Errorf("run %d: field %q references %q, want Alpha", i, f.Label, f.Type.RefName)
			}
		}
	}
}

// TestNormalizePrunesBeforeMerging builds a schema with an unreachable
// record (Orphan) that is structurally identical to a reachable one (A),
// and confirms normalize does not accidentally merge Orphan in — prune
// must run first and drop it, so it should never appear (nor influence
// which name survives) in the normalized output.
func TestNormalizePrunesBeforeMerging(t *testing.T) {
	s := mustParseOSD(t, `
		record A { "x": string }
		record Orphan { "x": string }
		record Top { "a": A }
		root Top
	`)
	got := algebra.Normalize(s)

	if _, ok := got.Env["Orphan"]; ok {
		t.Fatalf("Orphan should have been pruned before normalization, still present")
	}
	wantNames := []string{"A", "Top"}
	if !reflect.DeepEqual(got.EnvOrder, wantNames) {
		t.Fatalf("EnvOrder = %v, want %v", got.EnvOrder, wantNames)
	}
}

// TestNormalizeEmptyShortCircuit confirms that when the root is
// unsatisfiable (pruned schema is_empty), normalize returns the pruned
// result unchanged — matching what Prune alone produces (root fields
// untouched, per issue #9's root-unsatisfiable case), not further
// processed by equivalence-class merging.
func TestNormalizeEmptyShortCircuit(t *testing.T) {
	s := mustParseOSD(t, `record Node { "child": Node } root Node`)

	pruned := algebra.Prune(s)
	if !algebra.IsEmpty(pruned) {
		t.Fatalf("test setup: expected pruned schema to be empty")
	}

	got := algebra.Normalize(s)
	if !reflect.DeepEqual(got, pruned) {
		t.Fatalf("Normalize(s) = %+v, want unchanged Prune(s) = %+v", got, pruned)
	}
	// The root-unsatisfiable special case: fields untouched, not merged
	// away, even though Node's single self-referential field would give it
	// a signature theoretically groupable with itself.
	if len(got.Env["Node"].Fields) != 1 {
		t.Fatalf("root record's fields should be untouched by the empty short-circuit")
	}
}

// TestNormalizeFixpointRefinementKeepsDivergentBlocksSeparate is the test
// most likely to catch a real refinement bug: LeftHolder and RightHolder
// share the same initial local signature (both have one mandatory
// reference field named "target"), but LeftHolder points at LeftLeaf and
// RightHolder points at RightLeaf, and LeftLeaf/RightLeaf are NOT
// equivalent (different field labels, so different local signatures). An
// implementation that only does one pass of grouping-by-local-signature,
// never re-checking where references point, would wrongly merge
// LeftHolder and RightHolder. Correct fixpoint refinement must keep them
// apart.
func TestNormalizeFixpointRefinementKeepsDivergentBlocksSeparate(t *testing.T) {
	s := mustParseOSD(t, `
		record LeftLeaf { "left_only": string }
		record RightLeaf { "right_only": string }
		record LeftHolder { "target": LeftLeaf }
		record RightHolder { "target": RightLeaf }
		record Top { "l": LeftHolder, "r": RightHolder }
		root Top
	`)
	got := algebra.Normalize(s)

	// Nothing should have merged: 5 distinct records survive.
	wantNames := []string{"LeftHolder", "LeftLeaf", "RightHolder", "RightLeaf", "Top"}
	gotNames := append([]string{}, got.EnvOrder...)
	sort.Strings(gotNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("EnvOrder (sorted) = %v, want %v (nothing should have merged)", gotNames, wantNames)
	}
	top := got.Env["Top"]
	refs := map[string]string{}
	for _, f := range top.Fields {
		refs[f.Label] = f.Type.RefName
	}
	if refs["l"] != "LeftHolder" || refs["r"] != "RightHolder" {
		t.Fatalf("Top's references changed unexpectedly: %+v", refs)
	}
}

// TestNormalizeFixpointRefinementMergesTrulyEquivalentBlocks is the
// positive counterpart: LeftHolder2/RightHolder2 also share the same
// initial signature AND their reference targets (LeftLeaf2 vs RightLeaf2)
// are themselves genuinely equivalent (same fields), so after refinement
// LeftHolder2/RightHolder2 (and LeftLeaf2/RightLeaf2) SHOULD merge.
func TestNormalizeFixpointRefinementMergesTrulyEquivalentBlocks(t *testing.T) {
	s := mustParseOSD(t, `
		record LeftLeaf2 { "v": string }
		record RightLeaf2 { "v": string }
		record LeftHolder2 { "target": LeftLeaf2 }
		record RightHolder2 { "target": RightLeaf2 }
		record Top { "l": LeftHolder2, "r": RightHolder2 }
		root Top
	`)
	got := algebra.Normalize(s)

	wantNames := []string{"LeftHolder2", "LeftLeaf2", "Top"}
	gotNames := append([]string{}, got.EnvOrder...)
	sort.Strings(gotNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("EnvOrder (sorted) = %v, want %v", gotNames, wantNames)
	}
	top := got.Env["Top"]
	for _, f := range top.Fields {
		if f.Type.RefName != "LeftHolder2" {
			t.Errorf("field %q references %q, want LeftHolder2", f.Label, f.Type.RefName)
		}
	}
}

// --- equivalence_classes (§6.8), called directly (no prune) ---

// TestEquivalenceClassesDoesNotPrune confirms EquivalenceClasses operates
// on S.env exactly as given, per the spec's explicit note that it does
// NOT prune first (normalize prunes before calling it; a later issue,
// lint, calls it directly on the raw schema). An unreachable record must
// still show up in some block.
func TestEquivalenceClassesDoesNotPrune(t *testing.T) {
	s := mustParseOSD(t, `
		record A { "x": string }
		record Orphan { "y": string }
		record Top { "a": A }
		root Top
	`)
	blocks := algebra.EquivalenceClasses(s)

	found := false
	for _, block := range blocks {
		for _, name := range block {
			if name == "Orphan" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("Orphan should still appear in some equivalence class (EquivalenceClasses must not prune), blocks = %v", blocks)
	}
}

// TestEquivalenceClassesOrdering confirms blocks and names within blocks
// come back in deterministic sorted order.
func TestEquivalenceClassesOrdering(t *testing.T) {
	s := mustParseOSD(t, `
		record Zeta { "x": string }
		record Alpha { "x": string }
		record Top { "z": Zeta, "a": Alpha }
		root Top
	`)
	blocks := algebra.EquivalenceClasses(s)

	var flat []string
	for _, block := range blocks {
		var b []string
		b = append(b, block...)
		if !sort.StringsAreSorted(b) {
			t.Errorf("block %v not internally sorted", block)
		}
		flat = append(flat, block...)
	}
	if len(flat) != 3 {
		t.Fatalf("expected 3 names across all blocks, got %v", flat)
	}
}
