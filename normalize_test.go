package omnist

import "testing"

// This file holds the normalize.go tests that need unexported access
// (computeLocalSignature/computeRefineKey/appendEscapedLabel/appendUint),
// so they stay in the internal "omnist" package. Every other normalize.go
// test moved to normalize_public_test.go (external "omnist_test" package)
// since it only needed mustParseOSD + exported API -- see
// referee_test.go's comment for why that split exists (osd imports
// omnist, so an internal omnist test file cannot import osd without an
// import cycle).
//
// The six tests below that need both a Schema/Record fixture AND
// unexported access build that fixture directly as a *Record struct
// literal instead of via mustParseOSD/OSD text, since computeLocalSignature
// and computeRefineKey only ever look at rec.Fields (and, for
// computeRefineKey, a caller-supplied blockOf map keyed by ref name) --
// they never need a full parsed+validated Schema. This is exactly the
// same "construct by hand" pattern already used elsewhere in this suite
// (e.g. algebra_test.go's TestPruneReachabilityDanglingRefStopsWithoutPanic).

// TestLocalSignatureUnboundedCardinality covers the unbounded-cardinality
// encoding branch of appendFieldSig/local_signature: a record with an
// unbounded field must have a different local signature than one with an
// otherwise-identical bounded field, and two unbounded fields must match.
func TestLocalSignatureUnboundedCardinality(t *testing.T) {
	boundedRec := &Record{Name: "Bounded", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 1, Max: 5}},
	}}
	unb1Rec := &Record{Name: "Unbounded1", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 1, Unbounded: true}},
	}}
	unb2Rec := &Record{Name: "Unbounded2", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 1, Unbounded: true}},
	}}

	bounded := computeLocalSignature(boundedRec)
	unb1 := computeLocalSignature(unb1Rec)
	unb2 := computeLocalSignature(unb2Rec)

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
	optionalRec := &Record{Name: "Optional", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 0, Max: 1}},
	}}
	mandatoryRec := &Record{Name: "Mandatory", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 1, Max: 1}},
	}}

	opt := computeLocalSignature(optionalRec)
	man := computeLocalSignature(mandatoryRec)
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
	holderRec := &Record{Name: "Holder", Fields: []Field{
		{Label: "f", Type: RefType("Leaf"), Cardinality: Cardinality{Min: 1, Unbounded: true}},
	}}
	blockOf := map[string]int{"Leaf": 0}
	key := computeRefineKey(holderRec, blockOf)
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

// TestLocalSignatureDistinguishesFieldCount confirms local signatures
// differ (and hence records land in different initial blocks) when field
// counts differ, and are equal for structurally identical records.
func TestLocalSignatureDistinguishesFieldCount(t *testing.T) {
	oneRec := &Record{Name: "One", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
	}}
	twoRec := &Record{Name: "Two", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
		{Label: "y", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
	}}
	same1Rec := &Record{Name: "Same1", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
	}}
	same2Rec := &Record{Name: "Same2", Fields: []Field{
		{Label: "x", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
	}}

	one := computeLocalSignature(oneRec)
	two := computeLocalSignature(twoRec)
	same1 := computeLocalSignature(same1Rec)
	same2 := computeLocalSignature(same2Rec)

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
	refHolderRec := &Record{Name: "RefHolder", Fields: []Field{
		{Label: "f", Type: RefType("Leaf"), Cardinality: DefaultCardinality()},
	}}
	scalarHolderRec := &Record{Name: "ScalarHolder", Fields: []Field{
		{Label: "f", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
	}}

	ref := computeLocalSignature(refHolderRec)
	scalar := computeLocalSignature(scalarHolderRec)
	if ref == scalar {
		t.Errorf("ref-field and scalar-field records should have different local signatures")
	}
}

// TestLocalSignatureIgnoresRefTarget confirms two records referencing
// different (but at this stage un-refined) targets share a local
// signature — the target-blindness the spec calls for in step 2, refined
// away only later by refine_key/fixpoint.
func TestLocalSignatureIgnoresRefTarget(t *testing.T) {
	holderARec := &Record{Name: "HolderA", Fields: []Field{
		{Label: "f", Type: RefType("LeafA"), Cardinality: DefaultCardinality()},
	}}
	holderBRec := &Record{Name: "HolderB", Fields: []Field{
		{Label: "f", Type: RefType("LeafB"), Cardinality: DefaultCardinality()},
	}}

	a := computeLocalSignature(holderARec)
	b := computeLocalSignature(holderBRec)
	if a != b {
		t.Errorf("HolderA and HolderB should share a local signature (target-blind), got %q vs %q", a, b)
	}
}
