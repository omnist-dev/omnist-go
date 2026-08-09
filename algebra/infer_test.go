package algebra

import (
	"math/big"
	"testing"

	"github.com/omnist-dev/omnist-go"
)

// --- helpers -----------------------------------------------------------

func strDoc(fields ...omnist.Edge) omnist.Document {
	n := omnist.NewNode()
	n.Edges = append(n.Edges, fields...)
	return omnist.NodeDocument(n)
}

func strEdge(label, s string) omnist.Edge {
	return omnist.Edge{Label: label, Target: omnist.ValueTarget(omnist.ScalarValue(omnist.NewStringScalar(s)))}
}

func intEdge(label string, i int64) omnist.Edge {
	return omnist.Edge{Label: label, Target: omnist.ValueTarget(omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(i))))}
}

func nullEdge(label string) omnist.Edge {
	return omnist.Edge{Label: label, Target: omnist.ValueTarget(omnist.NullValue())}
}

func nodeEdge(label string, n *omnist.Node) omnist.Edge {
	return omnist.Edge{Label: label, Target: omnist.NodeTarget(n)}
}

func mustField(t *testing.T, s omnist.Schema, recName, label string) omnist.Field {
	t.Helper()
	rec, ok := s.Env[recName]
	if !ok {
		t.Fatalf("record %q not found in env %v", recName, s.EnvOrder)
	}
	for _, f := range rec.Fields {
		if f.Label == label {
			return f
		}
	}
	t.Fatalf("field %q not found in record %q (fields=%+v)", label, recName, rec.Fields)
	return omnist.Field{}
}

// --- worked example, §6.10 ----------------------------------------------

func TestInferWorkedExampleDefaultFails(t *testing.T) {
	samples := []omnist.Document{
		strDoc(intEdge("id", 7)),
		strDoc(strEdge("id", "seven")),
	}
	_, err := Infer(samples, "", false)
	if err == nil {
		t.Fatal("expected infer to fail on conflicting scalar kinds")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("expected a omnist.Diagnostic, got %T: %v", err, err)
	}
	if diag.Code != omnist.CodeAlgebraInferConflictingScalars {
		t.Fatalf("expected omnist.CodeAlgebraInferConflictingScalars, got %s", diag.Code)
	}
}

func TestInferWorkedExampleAllowAnySucceedsWithReport(t *testing.T) {
	samples := []omnist.Document{
		strDoc(intEdge("id", 7)),
		strDoc(strEdge("id", "seven")),
	}
	schema, fallbacks, err := InferWithReport(samples, "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := mustField(t, schema, "Root", "id")
	if f.Type.Kind != omnist.TypeAnyKind {
		t.Fatalf("expected id field to be any, got %+v", f.Type)
	}
	if len(fallbacks) != 1 {
		t.Fatalf("expected exactly one fallback, got %+v", fallbacks)
	}
	want := AnyFallback{Location: "Root.id", Reason: "values of more than one scalar kind (integer, string)"}
	if fallbacks[0] != want {
		t.Fatalf("fallback mismatch: got %+v, want %+v", fallbacks[0], want)
	}
}

func TestInferPlainWrapperAllowAnyDiscardsReport(t *testing.T) {
	samples := []omnist.Document{
		strDoc(intEdge("id", 7)),
		strDoc(strEdge("id", "seven")),
	}
	schema, err := Infer(samples, "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := mustField(t, schema, "Root", "id")
	if f.Type.Kind != omnist.TypeAnyKind {
		t.Fatalf("expected id field to be any, got %+v", f.Type)
	}
	// No way to retrieve fallbacks from Infer's return signature -- this
	// is asserted by the function signature itself (single Schema return,
	// no fallback slot) rather than by any runtime check.
}

// --- [0,] array rule ------------------------------------------------------

func TestInferRepeatedLabelBecomesUnboundedMinZero(t *testing.T) {
	// Every sample has >=2 occurrences of "tag" -- min must still be 0,
	// not the observed minimum count (2).
	samples := []omnist.Document{
		strDoc(strEdge("tag", "a"), strEdge("tag", "b")),
		strDoc(strEdge("tag", "x"), strEdge("tag", "y"), strEdge("tag", "z")),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := mustField(t, schema, "Root", "tag")
	if !f.Cardinality.Unbounded {
		t.Fatalf("expected unbounded cardinality, got %+v", f.Cardinality)
	}
	if f.Cardinality.Min != 0 {
		t.Fatalf("expected min=0 (not observed min), got %d", f.Cardinality.Min)
	}
}

// --- two-pass order independence ------------------------------------------

func TestInferMissingLabelOrderIndependence(t *testing.T) {
	forward := []omnist.Document{
		strDoc(strEdge("a", "1")),
		strDoc(strEdge("a", "1"), strEdge("b", "2")),
	}
	reverse := []omnist.Document{
		strDoc(strEdge("a", "1"), strEdge("b", "2")),
		strDoc(strEdge("a", "1")),
	}

	sf, err := Infer(forward, "", false)
	if err != nil {
		t.Fatalf("forward: unexpected error: %v", err)
	}
	sr, err := Infer(reverse, "", false)
	if err != nil {
		t.Fatalf("reverse: unexpected error: %v", err)
	}

	fb := mustField(t, sf, "Root", "b")
	if fb.Cardinality.Min != 0 || fb.Cardinality.Max != 1 || fb.Cardinality.Unbounded {
		t.Fatalf("expected b to be [0,1], got %+v", fb.Cardinality)
	}

	recF := sf.Env["Root"]
	recR := sr.Env["Root"]
	if len(recF.Fields) != len(recR.Fields) {
		t.Fatalf("field count differs: forward=%d reverse=%d", len(recF.Fields), len(recR.Fields))
	}
	for i := range recF.Fields {
		ff, fr := recF.Fields[i], recR.Fields[i]
		if ff.Label != fr.Label {
			t.Fatalf("field order differs at %d: forward=%s reverse=%s", i, ff.Label, fr.Label)
		}
		if ff.Cardinality != fr.Cardinality {
			t.Fatalf("cardinality differs for %s: forward=%+v reverse=%+v", ff.Label, ff.Cardinality, fr.Cardinality)
		}
	}
}

// --- integer/number collapse ----------------------------------------------

func TestInferIntegerNumberCollapse(t *testing.T) {
	samples := []omnist.Document{
		strDoc(intEdge("n", 3)),
		strDoc(omnist.Edge{Label: "n", Target: omnist.ValueTarget(omnist.ScalarValue(omnist.NewNumberScalar(3.5)))}),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := mustField(t, schema, "Root", "n")
	if f.Type.Kind != omnist.TypeScalarKind || f.Type.ScalarKind != omnist.KindNumber {
		t.Fatalf("expected number scalar, got %+v", f.Type)
	}
	if f.Type.Nullable {
		t.Fatalf("expected non-nullable, got nullable")
	}
}

// --- null handling ----------------------------------------------------

func TestInferNullableScalar(t *testing.T) {
	samples := []omnist.Document{
		strDoc(strEdge("name", "alice")),
		strDoc(nullEdge("name")),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := mustField(t, schema, "Root", "name")
	if f.Type.Kind != omnist.TypeScalarKind || f.Type.ScalarKind != omnist.KindString {
		t.Fatalf("expected string scalar, got %+v", f.Type)
	}
	if !f.Type.Nullable {
		t.Fatalf("expected nullable, got non-nullable")
	}
}

func TestInferAllNullDefaultsToNullableString(t *testing.T) {
	samples := []omnist.Document{
		strDoc(nullEdge("x")),
		strDoc(nullEdge("x")),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := mustField(t, schema, "Root", "x")
	if f.Type.Kind != omnist.TypeScalarKind || f.Type.ScalarKind != omnist.KindString || !f.Type.Nullable {
		t.Fatalf("expected nullable string, got %+v", f.Type)
	}
}

// --- mixed objects and values -----------------------------------------

func TestInferMixedShapeFailsByDefault(t *testing.T) {
	samples := []omnist.Document{
		strDoc(nodeEdge("thing", omnist.NewNode().AddValue("inner", omnist.ScalarValue(omnist.NewStringScalar("v"))))),
		strDoc(strEdge("thing", "scalar")),
	}
	_, err := Infer(samples, "", false)
	if err == nil {
		t.Fatal("expected mixed-shape failure")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("expected omnist.Diagnostic, got %T", err)
	}
	if diag.Code != omnist.CodeAlgebraInferMixedShape {
		t.Fatalf("expected omnist.CodeAlgebraInferMixedShape, got %s", diag.Code)
	}
}

func TestInferMixedShapeAllowAnyOpensWithReason(t *testing.T) {
	samples := []omnist.Document{
		strDoc(nodeEdge("thing", omnist.NewNode().AddValue("inner", omnist.ScalarValue(omnist.NewStringScalar("v"))))),
		strDoc(strEdge("thing", "scalar")),
	}
	schema, fallbacks, err := InferWithReport(samples, "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := mustField(t, schema, "Root", "thing")
	if f.Type.Kind != omnist.TypeAnyKind {
		t.Fatalf("expected any, got %+v", f.Type)
	}
	want := AnyFallback{Location: "Root.thing", Reason: "mixes objects and values"}
	found := false
	for _, fb := range fallbacks {
		if fb == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected fallback %+v, got %+v", want, fallbacks)
	}
}

// --- nested records + name uniqueness -----------------------------------

func TestInferNestedRecordsAndNameCollisionsMadeUnique(t *testing.T) {
	// Two different labels ("address") appear at two different points in
	// the tree, both node-valued: top-level "address" and
	// "office.address". Both would naturally generate the base name
	// "Address" -- confirm they're disambiguated.
	office := omnist.NewNode().AddNode("address", omnist.NewNode().AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("nyc"))))
	samples := []omnist.Document{
		strDoc(
			nodeEdge("address", omnist.NewNode().AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("sf")))),
			nodeEdge("office", office),
		),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rootRec := schema.Env["Root"]
	var addrRef, officeRef string
	for _, f := range rootRec.Fields {
		if f.Label == "address" {
			addrRef = f.Type.RefName
		}
		if f.Label == "office" {
			officeRef = f.Type.RefName
		}
	}
	if addrRef == "" || officeRef == "" {
		t.Fatalf("expected both address and office to be refs: %+v", rootRec.Fields)
	}
	officeRec := schema.Env[officeRef]
	var nestedAddrRef string
	for _, f := range officeRec.Fields {
		if f.Label == "address" {
			nestedAddrRef = f.Type.RefName
		}
	}
	if nestedAddrRef == "" {
		t.Fatalf("expected office.address to be a ref: %+v", officeRec.Fields)
	}
	if addrRef == nestedAddrRef {
		t.Fatalf("expected distinct generated names for the two 'address' records, both got %q", addrRef)
	}
}

// --- infer does not normalize -------------------------------------------

func TestInferDoesNotNormalizeDuplicateRecordsSurvive(t *testing.T) {
	// "a" and "b" are two different labels whose node values are
	// structurally identical (same single scalar field "v"). infer must
	// generate two SEPARATE records (A, B), not merge them the way
	// Normalize would.
	samples := []omnist.Document{
		strDoc(
			nodeEdge("a", omnist.NewNode().AddValue("v", omnist.ScalarValue(omnist.NewStringScalar("x")))),
			nodeEdge("b", omnist.NewNode().AddValue("v", omnist.ScalarValue(omnist.NewStringScalar("y")))),
		),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rootRec := schema.Env["Root"]
	var aRef, bRef string
	for _, f := range rootRec.Fields {
		if f.Label == "a" {
			aRef = f.Type.RefName
		}
		if f.Label == "b" {
			bRef = f.Type.RefName
		}
	}
	if aRef == "" || bRef == "" {
		t.Fatalf("expected both a and b to be refs: %+v", rootRec.Fields)
	}
	if aRef == bRef {
		t.Fatalf("expected two distinct structurally-identical records, got one shared %q", aRef)
	}
	if len(schema.Env) != 3 { // Root, A-ish, B-ish
		t.Fatalf("expected 3 records (Root + 2 structurally-identical), got %d: %v", len(schema.Env), schema.EnvOrder)
	}
}

// --- error cases -----------------------------------------------------

func TestInferZeroSamplesErrors(t *testing.T) {
	_, err := Infer(nil, "", false)
	if err == nil {
		t.Fatal("expected error for zero samples")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("expected omnist.Diagnostic, got %T", err)
	}
	if diag.Code != omnist.CodeAlgebraInferNoSamples {
		t.Fatalf("expected omnist.CodeAlgebraInferNoSamples, got %s", diag.Code)
	}
	// issue #33: algebra.infer-no-samples is a document.*-family code, so
	// per spec §8.4 its path MUST be "$" (the pre-schema/whole-schema
	// fallback), never a text-position or empty path.
	if diag.Path != "$" {
		t.Fatalf("path = %q, want %q", diag.Path, "$")
	}
}

func TestInferScalarRootErrors(t *testing.T) {
	samples := []omnist.Document{omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("bare")))}
	_, err := Infer(samples, "", false)
	if err == nil {
		t.Fatal("expected error for scalar-rooted sample")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("expected omnist.Diagnostic, got %T", err)
	}
	if diag.Code != omnist.CodeAlgebraInferScalarRoot {
		t.Fatalf("expected omnist.CodeAlgebraInferScalarRoot, got %s", diag.Code)
	}
	// issue #33: algebra.infer-scalar-root's path is "$" per spec §8.4,
	// not "samples[N]" — see TestInferZeroSamplesErrors for the same rule
	// applied to the sibling code.
	if diag.Path != "$" {
		t.Fatalf("path = %q, want %q", diag.Path, "$")
	}
}

func TestInferScalarRootAmongMultipleSamplesErrors(t *testing.T) {
	samples := []omnist.Document{
		strDoc(strEdge("a", "1")),
		omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("bare"))),
	}
	_, err := Infer(samples, "", false)
	if err == nil {
		t.Fatal("expected error for scalar-rooted sample")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("expected omnist.Diagnostic, got %T", err)
	}
	if diag.Code != omnist.CodeAlgebraInferScalarRoot {
		t.Fatalf("expected omnist.CodeAlgebraInferScalarRoot, got %s", diag.Code)
	}
	// issue #33: algebra.infer-scalar-root's path is "$" per spec §8.4,
	// not "samples[N]" — see TestInferZeroSamplesErrors for the same rule
	// applied to the sibling code.
	if diag.Path != "$" {
		t.Fatalf("path = %q, want %q", diag.Path, "$")
	}
}

// --- root name default + custom -----------------------------------------

func TestInferDefaultRootName(t *testing.T) {
	samples := []omnist.Document{strDoc(strEdge("a", "1"))}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema.Root != "Root" {
		t.Fatalf("expected default root name 'Root', got %q", schema.Root)
	}
}

func TestInferCustomRootName(t *testing.T) {
	samples := []omnist.Document{strDoc(strEdge("a", "1"))}
	schema, err := Infer(samples, "Custom", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema.Root != "Custom" {
		t.Fatalf("expected root name 'Custom', got %q", schema.Root)
	}
}

// --- required vs optional (non-array) ------------------------------------

func TestInferRequiredWhenPresentInEverySample(t *testing.T) {
	samples := []omnist.Document{
		strDoc(strEdge("a", "1")),
		strDoc(strEdge("a", "2")),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := mustField(t, schema, "Root", "a")
	if f.Cardinality.Min != 1 || f.Cardinality.Max != 1 || f.Cardinality.Unbounded {
		t.Fatalf("expected [1,1], got %+v", f.Cardinality)
	}
}

// --- nested record error propagation -------------------------------------

func TestInferErrorPropagatesFromNestedRecord(t *testing.T) {
	// The error occurs inside the nested "child" record's own field, not
	// at the root -- confirms inferType's recursive inferRecord error path
	// (not just inferRecord's own top-level field loop) surfaces the
	// error to the caller.
	samples := []omnist.Document{
		strDoc(nodeEdge("child", omnist.NewNode().AddValue("v", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))))),
		strDoc(nodeEdge("child", omnist.NewNode().AddValue("v", omnist.ScalarValue(omnist.NewStringScalar("s"))))),
	}
	_, err := Infer(samples, "", false)
	if err == nil {
		t.Fatal("expected error to propagate from nested record")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("expected omnist.Diagnostic, got %T", err)
	}
	if diag.Code != omnist.CodeAlgebraInferConflictingScalars {
		t.Fatalf("expected omnist.CodeAlgebraInferConflictingScalars, got %s", diag.Code)
	}
}

// --- sanitizeIdentifier edge cases (for 100% coverage of uniqueNameFrom) --

func TestInferLabelSanitizedToValidRecordName(t *testing.T) {
	samples := []omnist.Document{
		strDoc(nodeEdge("9-strange label!", omnist.NewNode().AddValue("v", omnist.ScalarValue(omnist.NewStringScalar("x"))))),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rootRec := schema.Env["Root"]
	if len(rootRec.Fields) != 1 {
		t.Fatalf("expected 1 field, got %+v", rootRec.Fields)
	}
	refName := rootRec.Fields[0].Type.RefName
	if refName == "" {
		t.Fatalf("expected a generated ref name, got %+v", rootRec.Fields[0])
	}
	if _, ok := schema.Env[refName]; !ok {
		t.Fatalf("generated name %q not present in env", refName)
	}
}

func TestInferEmptyLabelFallsBackToFieldName(t *testing.T) {
	// A label with nothing identifier-shaped in it (here, the empty
	// string) sanitizes to "" and must fall back to the "Field" base name.
	samples := []omnist.Document{
		strDoc(nodeEdge("", omnist.NewNode().AddValue("v", omnist.ScalarValue(omnist.NewStringScalar("x"))))),
	}
	schema, err := Infer(samples, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rootRec := schema.Env["Root"]
	if len(rootRec.Fields) != 1 {
		t.Fatalf("expected 1 field, got %+v", rootRec.Fields)
	}
	refName := rootRec.Fields[0].Type.RefName
	if refName != "omnist.Field" {
		t.Fatalf("expected generated name 'omnist.Field', got %q", refName)
	}
}
