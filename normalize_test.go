package omnist

import (
	"reflect"
	"sort"
	"testing"
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
	got := Normalize(s)

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
		if f.Type.Kind != TypeRefKind || f.Type.RefName != "A" {
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
		got := Normalize(s)
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
	got := Normalize(s)

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

	pruned := Prune(s)
	if !IsEmpty(pruned) {
		t.Fatalf("test setup: expected pruned schema to be empty")
	}

	got := Normalize(s)
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
	got := Normalize(s)

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
	got := Normalize(s)

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

// TestLocalSignatureUnboundedCardinality covers the unbounded-cardinality
// encoding branch of appendFieldSig/local_signature: a record with an
// unbounded field must have a different local signature than one with an
// otherwise-identical bounded field, and two unbounded fields must match.
func TestLocalSignatureUnboundedCardinality(t *testing.T) {
	s := mustParseOSD(t, `
		record Bounded { "x" [1,5]: string }
		record Unbounded1 { "x" [1,]: string }
		record Unbounded2 { "x" [1,]: string }
		record Top { "b": Bounded, "u1": Unbounded1, "u2": Unbounded2 }
		root Top
	`)
	bounded := computeLocalSignature(s.Env["Bounded"])
	unb1 := computeLocalSignature(s.Env["Unbounded1"])
	unb2 := computeLocalSignature(s.Env["Unbounded2"])

	if bounded == unb1 {
		t.Errorf("bounded and unbounded cardinalities should differ")
	}
	if unb1 != unb2 {
		t.Errorf("two unbounded fields should share a local signature")
	}
}

// TestLocalSignatureOptionalFieldZeroMin covers the min==0 encoding
// branch (appendUint's zero case) via an optional field.
func TestLocalSignatureOptionalFieldZeroMin(t *testing.T) {
	s := mustParseOSD(t, `
		record Optional { "x" [0,1]: string }
		record Mandatory { "x" [1,1]: string }
		root Optional
	`)
	opt := computeLocalSignature(s.Env["Optional"])
	man := computeLocalSignature(s.Env["Mandatory"])
	if opt == man {
		t.Errorf("optional (min=0) and mandatory (min=1) fields should have different local signatures")
	}
}

// TestAppendEscapedLabelEscapesControlBytes exercises
// appendEscapedLabel's escape branch directly: since OSD label content
// reaching the parser cannot itself contain raw control bytes, this
// checks the helper in isolation to confirm two labels that would
// otherwise collide once delimiters are involved are kept distinct.
func TestAppendEscapedLabelEscapesControlBytes(t *testing.T) {
	withControl := string(appendEscapedLabel(nil, "a\x00b"))
	plain := string(appendEscapedLabel(nil, "a\x02\x00b"))
	if withControl == "" {
		t.Fatalf("expected non-empty escaped output")
	}
	if len(withControl) <= len("a\x00b") {
		t.Errorf("expected escape byte to lengthen output, got %q", withControl)
	}
	// Distinct inputs sharing the raw \x00 byte once escaped must not
	// collide with an unrelated input containing the escape byte itself.
	if withControl == plain {
		t.Errorf("escaping collision: %q == %q", withControl, plain)
	}
}

// TestComputeRefineKeyUnboundedRefField covers computeRefineKey's
// unbounded-cardinality-on-a-reference-field encoding branch, which
// local_signature-level tests never reach since they use non-ref fields.
func TestComputeRefineKeyUnboundedRefField(t *testing.T) {
	s := mustParseOSD(t, `
		record Leaf { "v": string }
		record Holder { "f" [1,]: Leaf }
		root Holder
	`)
	blockOf := map[string]int{"Leaf": 0}
	key := computeRefineKey(s.Env["Holder"], blockOf)
	if key == "" {
		t.Fatalf("expected non-empty refine key")
	}
}

// TestAppendUintMultiDigit covers appendUint's digit-reversal loop, which
// a single-digit cardinality (the only kind used elsewhere in this file)
// never exercises.
func TestAppendUintMultiDigit(t *testing.T) {
	got := string(appendUint(nil, 12345))
	if got != "12345" {
		t.Fatalf("appendUint(12345) = %q, want %q", got, "12345")
	}
	single := string(appendUint(nil, 7))
	if single != "7" {
		t.Fatalf("appendUint(7) = %q, want %q", single, "7")
	}
	zero := string(appendUint(nil, 0))
	if zero != "0" {
		t.Fatalf("appendUint(0) = %q, want %q", zero, "0")
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
	blocks := EquivalenceClasses(s)

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
	blocks := EquivalenceClasses(s)

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

// TestLocalSignatureDistinguishesFieldCount confirms local signatures
// differ (and hence records land in different initial blocks) when field
// counts differ, and are equal for structurally identical records.
func TestLocalSignatureDistinguishesFieldCount(t *testing.T) {
	s := mustParseOSD(t, `
		record One { "x": string }
		record Two { "x": string, "y": string }
		record Same1 { "x": string }
		record Same2 { "x": string }
		record Top { "o": One, "t": Two, "s1": Same1, "s2": Same2 }
		root Top
	`)
	one := computeLocalSignature(s.Env["One"])
	two := computeLocalSignature(s.Env["Two"])
	same1 := computeLocalSignature(s.Env["Same1"])
	same2 := computeLocalSignature(s.Env["Same2"])

	if one == two {
		t.Errorf("One and Two should have different local signatures")
	}
	if same1 != same2 {
		t.Errorf("Same1 and Same2 should have identical local signatures")
	}
}

// TestLocalSignatureRefVsScalarDiffers confirms a reference field and a
// scalar field with the same label/cardinality produce different local
// signatures (the is-scalar-or-not component), even though the reference
// target itself is ignored by local_signature.
func TestLocalSignatureRefVsScalarDiffers(t *testing.T) {
	s := mustParseOSD(t, `
		record Leaf { "v": string }
		record RefHolder { "f": Leaf }
		record ScalarHolder { "f": string }
		record Top { "r": RefHolder, "s": ScalarHolder }
		root Top
	`)
	ref := computeLocalSignature(s.Env["RefHolder"])
	scalar := computeLocalSignature(s.Env["ScalarHolder"])
	if ref == scalar {
		t.Errorf("ref-field and scalar-field records should have different local signatures")
	}
}

// TestLocalSignatureIgnoresRefTarget confirms two records referencing
// different (but at this stage un-refined) targets share a local
// signature — the target-blindness the spec calls for in step 2, refined
// away only later by refine_key/fixpoint.
func TestLocalSignatureIgnoresRefTarget(t *testing.T) {
	s := mustParseOSD(t, `
		record LeafA { "v": string }
		record LeafB { "v": string, "w": string }
		record HolderA { "f": LeafA }
		record HolderB { "f": LeafB }
		record Top { "a": HolderA, "b": HolderB }
		root Top
	`)
	a := computeLocalSignature(s.Env["HolderA"])
	b := computeLocalSignature(s.Env["HolderB"])
	if a != b {
		t.Errorf("HolderA and HolderB should share a local signature (target-blind), got %q vs %q", a, b)
	}
}
