package algebra

import (
	"testing"

	"github.com/omnist-dev/omnist-go"
)

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
	boundedRec := &omnist.Record{Name: "Bounded", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 1, Max: 5}},
	}}
	unb1Rec := &omnist.Record{Name: "Unbounded1", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 1, Unbounded: true}},
	}}
	unb2Rec := &omnist.Record{Name: "Unbounded2", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 1, Unbounded: true}},
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
	optionalRec := &omnist.Record{Name: "Optional", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 0, Max: 1}},
	}}
	mandatoryRec := &omnist.Record{Name: "Mandatory", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 1, Max: 1}},
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
	holderRec := &omnist.Record{Name: "Holder", Fields: []omnist.Field{
		{Label: "f", Type: omnist.RefType("Leaf"), Cardinality: omnist.Cardinality{Min: 1, Unbounded: true}},
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
	oneRec := &omnist.Record{Name: "One", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
	}}
	twoRec := &omnist.Record{Name: "Two", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
		{Label: "y", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
	}}
	same1Rec := &omnist.Record{Name: "Same1", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
	}}
	same2Rec := &omnist.Record{Name: "Same2", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
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
	refHolderRec := &omnist.Record{Name: "RefHolder", Fields: []omnist.Field{
		{Label: "f", Type: omnist.RefType("Leaf"), Cardinality: omnist.DefaultCardinality()},
	}}
	scalarHolderRec := &omnist.Record{Name: "ScalarHolder", Fields: []omnist.Field{
		{Label: "f", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
	}}

	ref := computeLocalSignature(refHolderRec)
	scalar := computeLocalSignature(scalarHolderRec)
	if ref == scalar {
		t.Errorf("ref-field and scalar-field records should have different local signatures")
	}
}

// TestLocalSignatureDistinguishesScalarKindsAndAny is the regression test
// for issue #68: appendFieldSig used to collapse every non-Ref field type
// (string, integer, ..., and even Any) into a single 'S' byte, so records
// differing only in a field's scalar kind (or Any vs a concrete scalar)
// wrongly shared a local signature. Per §6.8's shape_of, Any, Ref, and each
// (kind, nullable) scalar pairing must all be distinct.
func TestLocalSignatureDistinguishesScalarKindsAndAny(t *testing.T) {
	stringRec := &omnist.Record{Name: "StringRec", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
	}}
	integerRec := &omnist.Record{Name: "IntegerRec", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindInteger, false), Cardinality: omnist.DefaultCardinality()},
	}}
	anyRec := &omnist.Record{Name: "AnyRec", Fields: []omnist.Field{
		{Label: "x", Type: omnist.AnyType(), Cardinality: omnist.DefaultCardinality()},
	}}

	str := computeLocalSignature(stringRec)
	integer := computeLocalSignature(integerRec)
	any := computeLocalSignature(anyRec)

	if str == integer {
		t.Errorf("string and integer fields should have different local signatures, both got %q", str)
	}
	if str == any {
		t.Errorf("string and any fields should have different local signatures, both got %q", str)
	}
	if integer == any {
		t.Errorf("integer and any fields should have different local signatures, both got %q", integer)
	}
}

// TestLocalSignatureDistinguishesNullability confirms a nullable scalar and
// a non-nullable scalar of the same kind get different local signatures,
// per §6.8's shape_of returning ("scalar", t.kind, t.nullable) — nullable
// was previously dropped entirely, not just merged with other kinds.
func TestLocalSignatureDistinguishesNullability(t *testing.T) {
	nullableRec := &omnist.Record{Name: "Nullable", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, true), Cardinality: omnist.DefaultCardinality()},
	}}
	nonNullableRec := &omnist.Record{Name: "NonNullable", Fields: []omnist.Field{
		{Label: "x", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
	}}

	nullable := computeLocalSignature(nullableRec)
	nonNullable := computeLocalSignature(nonNullableRec)
	if nullable == nonNullable {
		t.Errorf("nullable and non-nullable string fields should have different local signatures, both got %q", nullable)
	}
}

// TestLocalSignatureIgnoresRefTarget confirms two records referencing
// different (but at this stage un-refined) targets share a local
// signature — the target-blindness the spec calls for in step 2, refined
// away only later by refine_key/fixpoint.
func TestLocalSignatureIgnoresRefTarget(t *testing.T) {
	holderARec := &omnist.Record{Name: "HolderA", Fields: []omnist.Field{
		{Label: "f", Type: omnist.RefType("LeafA"), Cardinality: omnist.DefaultCardinality()},
	}}
	holderBRec := &omnist.Record{Name: "HolderB", Fields: []omnist.Field{
		{Label: "f", Type: omnist.RefType("LeafB"), Cardinality: omnist.DefaultCardinality()},
	}}

	a := computeLocalSignature(holderARec)
	b := computeLocalSignature(holderBRec)
	if a != b {
		t.Errorf("HolderA and HolderB should share a local signature (target-blind), got %q vs %q", a, b)
	}
}
