package omnist_test

// This file hand-ports the spec's own 10-case referee self-test
// (vendor/omnist-spec/conformance/fixtures/_referee-self-test/) as real Go
// tests, per the porting guide's step 1: "Prove the referee trustworthy
// before it judges anything ... all three existing ports did this as
// their literal step one." Each test's name and comment quote the
// fixture's own purpose.txt. This MUST pass before tools/conformance's
// vector runner is trusted to judge anything using omnist.SchemasEqual/
// omnist.DocumentsEqual.
//
// This file (and referee_coverage_test.go) lives in the external
// "omnist_test" package, not "omnist" -- it only ever needed exported API
// (omnist.SchemasEqual/omnist.DocumentsEqual/omnist.ModeExact/omnist.ModeIsomorphic), so moving it out
// avoids the import cycle package osd would otherwise create (osd imports
// omnist; an internal omnist test file importing osd is not allowed).
// mustOSD/mustOML are defined once, for both files, in
// osd_external_test_helpers_test.go.

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// Case 01: fields are an unordered set at the model layer (§3.1) --
// declaration order must not affect equality.
func TestReferee01SchemaExactEqualDifferentFieldOrder(t *testing.T) {
	a := mustOSD(t, "record R {\n    \"x\": string,\n    \"y\": integer,\n}\nroot R\n")
	b := mustOSD(t, "record R {\n    \"y\": integer,\n    \"x\": string,\n}\nroot R\n")
	if !omnist.SchemasEqual(a, b, omnist.ModeExact) {
		t.Fatal("want equal (field order must not affect exact equality)")
	}
}

// Case 02: one field's cardinality differs ([1,1] vs [0,1]) -- must not
// compare equal even though everything else matches.
func TestReferee02SchemaExactNotEqualCardinalityDiff(t *testing.T) {
	a := mustOSD(t, "record R {\n    \"x\": string,\n}\nroot R\n")
	b := mustOSD(t, "record R {\n    \"x\" [0,1]: string,\n}\nroot R\n")
	if omnist.SchemasEqual(a, b, omnist.ModeExact) {
		t.Fatal("want not-equal (cardinality differs)")
	}
}

// Case 03: same structure, different record name -- isomorphic mode
// (used for infer, §6.2) must treat these as equal.
func TestReferee03SchemaIsomorphicEqualRenamedRecords(t *testing.T) {
	a := mustOSD(t, "record A {\n    \"x\": string,\n}\nroot A\n")
	b := mustOSD(t, "record B {\n    \"x\": string,\n}\nroot B\n")
	if !omnist.SchemasEqual(a, b, omnist.ModeIsomorphic) {
		t.Fatal("want equal under isomorphic mode")
	}
}

// Case 04: same two schemas as case 03, but exact mode (used for
// normalize/prune/extract, §6.2) must treat a record-name difference as
// NOT equal -- the companion case proving the two modes genuinely differ.
func TestReferee04SchemaExactNotEqualRenamedRecords(t *testing.T) {
	a := mustOSD(t, "record A {\n    \"x\": string,\n}\nroot A\n")
	b := mustOSD(t, "record B {\n    \"x\": string,\n}\nroot B\n")
	if omnist.SchemasEqual(a, b, omnist.ModeExact) {
		t.Fatal("want not-equal under exact mode")
	}
}

// Case 05: edge order is data (D-1/D-3, §2.3) -- two documents with the
// same edges in a different order are genuinely different documents, not
// a false mismatch to paper over.
func TestReferee05DocumentNotEqualReorderedEdges(t *testing.T) {
	a := mustOML(t, "a: 1\nb: 2\n")
	b := mustOML(t, "b: 2\na: 1\n")
	if omnist.DocumentsEqual(a, b) {
		t.Fatal("want not-equal (edge order differs)")
	}
}

// Case 06: array sugar desugars into repeated labels at parse time (§2)
// -- both spellings must produce the identical Document.
func TestReferee06DocumentEqualArraySugarVsRepeatedLabels(t *testing.T) {
	a := mustOML(t, "tag: [\"x\", \"y\"]\n")
	b := mustOML(t, "tag: \"x\"\ntag: \"y\"\n")
	if !omnist.DocumentsEqual(a, b) {
		t.Fatal("want equal (array sugar == repeated labels)")
	}
}

// Case 07: record declaration order in the schema's env is preserved for
// OSD-text readability (§3.1) but is not semantically significant --
// equality must not be sensitive to it.
func TestReferee07SchemaExactEqualDifferentEnvDeclarationOrder(t *testing.T) {
	a := mustOSD(t, "record Zeta {\n    \"x\": string,\n}\nrecord Root {\n    \"z\": Zeta,\n}\nroot Root\n")
	b := mustOSD(t, "record Root {\n    \"z\": Zeta,\n}\nrecord Zeta {\n    \"x\": string,\n}\nroot Root\n")
	if !omnist.SchemasEqual(a, b, omnist.ModeExact) {
		t.Fatal("want equal (env declaration order must not affect equality)")
	}
}

// Case 08: one field's scalar kind differs -- must not compare equal.
func TestReferee08SchemaExactNotEqualScalarKindDiff(t *testing.T) {
	a := mustOSD(t, "record R {\n    \"x\": string,\n}\nroot R\n")
	b := mustOSD(t, "record R {\n    \"x\": integer,\n}\nroot R\n")
	if omnist.SchemasEqual(a, b, omnist.ModeExact) {
		t.Fatal("want not-equal (scalar kind differs)")
	}
}

// Case 09: one field's nullable flag differs (string vs string?) -- must
// not compare equal.
func TestReferee09SchemaExactNotEqualNullableDiff(t *testing.T) {
	a := mustOSD(t, "record R {\n    \"x\": string,\n}\nroot R\n")
	b := mustOSD(t, "record R {\n    \"x\": string?,\n}\nroot R\n")
	if omnist.SchemasEqual(a, b, omnist.ModeExact) {
		t.Fatal("want not-equal (nullable flag differs)")
	}
}

// Case 10: two byte-identical nested documents (repeated item labels,
// same order) -- the baseline case confirming equal really means equal,
// not just "not obviously different."
func TestReferee10DocumentEqualIdenticalNestedStructure(t *testing.T) {
	text := "order: {\n    id: \"A1\"\n    item: { sku: \"W\"; qty: 3 }\n    item: { sku: \"G\"; qty: 1 }\n}\n"
	a := mustOML(t, text)
	b := mustOML(t, text)
	if !omnist.DocumentsEqual(a, b) {
		t.Fatal("want equal (identical nested structure)")
	}
}
