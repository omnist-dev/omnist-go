package omnist

import (
	"math"
	"math/big"
	"testing"
)

// scalarRecordSchema builds a one-field record `record R { "n": <kind> }`
// (optionally nullable), used throughout this file's table-driven tests
// since every §7.2 materialization-table row is single-leaf.
func scalarRecordSchema(kind ScalarKind, nullable bool) Schema {
	r := &Record{
		Name:   "R",
		Fields: []Field{{Label: "n", Type: ScalarType(kind, nullable), Cardinality: DefaultCardinality()}},
	}
	return Schema{Root: "R", Env: map[string]*Record{"R": r}}
}

func oneFieldDoc(v Value) Document {
	n := NewNode()
	n.AddValue("n", v)
	return NodeDocument(n)
}

// --- §7.2 materialization table, both directions where applicable ---

func TestMaterializeStringToDate(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewStringScalar("2024-01-01")))
	got, diags, err := Materialize(doc, scalarRecordSchema(KindDate, false))
	if err != nil || len(diags) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags, err)
	}
	want := NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 1})
	gotScalar := got.Node.Edges[0].Target
	v, _ := gotScalar.Value()
	if !v.Scalar.Equal(want) {
		t.Fatalf("got %+v want %+v", v.Scalar, want)
	}
}

func TestMaterializeStringToTime(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewStringScalar("12:30:00")))
	got, diags, err := Materialize(doc, scalarRecordSchema(KindTime, false))
	if err != nil || len(diags) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags, err)
	}
	want := NewTimeScalar(TimeValue{Hour: 12, Minute: 30})
	v, _ := got.Node.Edges[0].Target.Value()
	if !v.Scalar.Equal(want) {
		t.Fatalf("got %+v want %+v", v.Scalar, want)
	}
}

func TestMaterializeStringToDateTime(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewStringScalar("2024-01-01T12:30:00")))
	got, diags, err := Materialize(doc, scalarRecordSchema(KindDateTime, false))
	if err != nil || len(diags) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags, err)
	}
	want := NewDateTimeScalar(DateTimeValue{
		Date: DateValue{Year: 2024, Month: 1, Day: 1},
		Time: TimeValue{Hour: 12, Minute: 30},
	})
	v, _ := got.Node.Edges[0].Target.Value()
	if !v.Scalar.Equal(want) {
		t.Fatalf("got %+v want %+v", v.Scalar, want)
	}
}

func TestMaterializeWholeFloatToIntegerIsValueExact(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewNumberScalar(1.0)))
	got, diags, err := Materialize(doc, scalarRecordSchema(KindInteger, false))
	if err != nil || len(diags) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags, err)
	}
	v, _ := got.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindInteger {
		t.Fatalf("want KindInteger, got %v", v.Scalar.Kind)
	}
	if v.Scalar.Int.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("want 1, got %v", v.Scalar.Int)
	}
}

func TestMaterializeFractionalFloatToIntegerFails(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewNumberScalar(1.5)))
	_, diags, err := Materialize(doc, scalarRecordSchema(KindInteger, false))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeMaterializeInexactConversion) {
		t.Fatalf("want materialize.inexact-conversion, got %v", codesOf(diags))
	}
}

func TestMaterializeIntegerToNumberYieldsFloatTypedResult(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewIntegerScalar(big.NewInt(1))))
	got, diags, err := Materialize(doc, scalarRecordSchema(KindNumber, false))
	if err != nil || len(diags) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags, err)
	}
	v, _ := got.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindNumber {
		t.Fatalf("want KindNumber (float-typed), got %v", v.Scalar.Kind)
	}
	if v.Scalar.Num != 1.0 {
		t.Fatalf("want 1.0, got %v", v.Scalar.Num)
	}
}

func TestMaterializeStringNumeralNeverUpgradesToInteger(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewStringScalar("1")))
	_, diags, err := Materialize(doc, scalarRecordSchema(KindInteger, false))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeMaterializeInexactConversion) {
		t.Fatalf("want materialize.inexact-conversion, got %v", codesOf(diags))
	}
}

func TestMaterializeArbitraryStringNeverUpgradesToBoolean(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewStringScalar("maybe")))
	_, diags, err := Materialize(doc, scalarRecordSchema(KindBoolean, false))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeMaterializeInexactConversion) {
		t.Fatalf("want materialize.inexact-conversion, got %v", codesOf(diags))
	}
}

// --- boolean never treated as integer/number in either direction ---

func TestMaterializeBooleanNeverUpgradesToIntegerOrNumber(t *testing.T) {
	for _, kind := range []ScalarKind{KindInteger, KindNumber} {
		doc := oneFieldDoc(ScalarValue(NewBooleanScalar(true)))
		_, diags, err := Materialize(doc, scalarRecordSchema(kind, false))
		if err != nil {
			t.Fatal(err)
		}
		if !hasCode(diags, CodeMaterializeInexactConversion) {
			t.Fatalf("kind=%v: want materialize.inexact-conversion, got %v", kind, codesOf(diags))
		}
	}
}

func TestMaterializeIntegerAndNumberNeverUpgradeToBoolean(t *testing.T) {
	cases := []Scalar{NewIntegerScalar(big.NewInt(1)), NewNumberScalar(1.0)}
	for _, s := range cases {
		doc := oneFieldDoc(ScalarValue(s))
		_, diags, err := Materialize(doc, scalarRecordSchema(KindBoolean, false))
		if err != nil {
			t.Fatal(err)
		}
		if !hasCode(diags, CodeMaterializeInexactConversion) {
			t.Fatalf("source=%+v: want materialize.inexact-conversion, got %v", s, codesOf(diags))
		}
	}
}

// --- the any-boundary rule ---

func TestMaterializeAnyFieldPassesSubtreeThroughUntouched(t *testing.T) {
	inner := NewNode()
	inner.AddValue("x", ScalarValue(NewStringScalar("2024-01-01"))) // date-shaped, but must NOT upgrade
	root := NewNode()
	root.AddNode("data", inner)
	doc := NodeDocument(root)

	r := &Record{Name: "R", Fields: []Field{{Label: "data", Type: AnyType(), Cardinality: DefaultCardinality()}}}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}

	got, diags, err := Materialize(doc, s)
	if err != nil || len(diags) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags, err)
	}
	if !DocumentsEqual(got, doc) {
		t.Fatalf("subtree under `any` must pass through byte-for-byte unchanged: got %+v want %+v", got, doc)
	}
}

// --- collect-all, not fail-fast ---

func TestMaterializeCollectsEveryProblemNotJustTheFirst(t *testing.T) {
	r := &Record{
		Name: "R",
		Fields: []Field{
			{Label: "a", Type: ScalarType(KindInteger, false), Cardinality: DefaultCardinality()},
			{Label: "b", Type: ScalarType(KindBoolean, false), Cardinality: DefaultCardinality()},
		},
	}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}
	n := NewNode()
	n.AddValue("a", ScalarValue(NewStringScalar("x")))
	n.AddValue("b", ScalarValue(NewStringScalar("y")))
	doc := NodeDocument(n)

	_, diags, err := Materialize(doc, s)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, d := range diags {
		if d.Code == CodeMaterializeInexactConversion {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("want 2 materialize.inexact-conversion diagnostics, got %d: %v", count, diags)
	}
}

// --- disjoint temporal shapes ---

func TestMaterializeBareDateNeverUpgradesToDateTime(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewStringScalar("2024-01-01")))
	_, diags, err := Materialize(doc, scalarRecordSchema(KindDateTime, false))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeMaterializeInexactConversion) {
		t.Fatalf("want materialize.inexact-conversion, got %v", codesOf(diags))
	}
}

func TestMaterializeDateTimeShapedStringNeverUpgradesToDate(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewStringScalar("2024-01-01T12:30:00")))
	_, diags, err := Materialize(doc, scalarRecordSchema(KindDate, false))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeMaterializeInexactConversion) {
		t.Fatalf("want materialize.inexact-conversion, got %v", codesOf(diags))
	}
}

func TestMaterializeDateShapedStringNeverUpgradesToTime(t *testing.T) {
	doc := oneFieldDoc(ScalarValue(NewStringScalar("2024-01-01")))
	_, diags, err := Materialize(doc, scalarRecordSchema(KindTime, false))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeMaterializeInexactConversion) {
		t.Fatalf("want materialize.inexact-conversion, got %v", codesOf(diags))
	}
}

// --- cross-codec integration: JSON never auto-resolves temporal strings ---

func TestMaterializeUpgradesJSONSourcedStringsToTemporalKinds(t *testing.T) {
	doc, err := ReadJSON(`{"d": "2024-01-01", "t": "12:30:00", "dt": "2024-01-01T12:30:00"}`, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	r := &Record{
		Name: "R",
		Fields: []Field{
			{Label: "d", Type: ScalarType(KindDate, false), Cardinality: DefaultCardinality()},
			{Label: "t", Type: ScalarType(KindTime, false), Cardinality: DefaultCardinality()},
			{Label: "dt", Type: ScalarType(KindDateTime, false), Cardinality: DefaultCardinality()},
		},
	}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}

	got, diags, merr := Materialize(doc, s)
	if merr != nil || len(diags) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags, merr)
	}
	// A JSON reader can never itself produce a KindDate/KindTime/KindDateTime
	// scalar (JSON has no such literal syntax) -- confirming these came out
	// upgraded is the concrete proof that materialize, not the reader, did
	// the work, which is the whole reason this operation exists.
	for _, e := range got.Node.Edges {
		v, _ := e.Target.Value()
		switch e.Label {
		case "d":
			if v.Scalar.Kind != KindDate {
				t.Fatalf("d: want KindDate, got %v", v.Scalar.Kind)
			}
		case "t":
			if v.Scalar.Kind != KindTime {
				t.Fatalf("t: want KindTime, got %v", v.Scalar.Kind)
			}
		case "dt":
			if v.Scalar.Kind != KindDateTime {
				t.Fatalf("dt: want KindDateTime, got %v", v.Scalar.Kind)
			}
		}
	}
}

// --- the lockstep invariant with validate ---

// TestMaterializeValidateLockstepOnShape is the shape/cardinality half of
// the issue's lockstep requirement. Shape/cardinality checking involves no
// conversion at all (walkRecordShape is the literal same code for both
// operations, see validate.go), so for a document whose only problem is
// shape (an unexpected field), Materialize and Validate must agree on the
// *original*, pre-materialize document -- there is nothing for materialize
// to convert here, so "original" vs. "materialized" input is not even a
// meaningful distinction to draw for this half of the invariant.
func TestMaterializeValidateLockstepOnShape(t *testing.T) {
	r := &Record{
		Name:   "R",
		Fields: []Field{{Label: "n", Type: ScalarType(KindInteger, false), Cardinality: DefaultCardinality()}},
	}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}

	ok := func() Document {
		n := NewNode()
		n.AddValue("n", ScalarValue(NewIntegerScalar(big.NewInt(1))))
		return NodeDocument(n)
	}()
	shapeBad := func() Document {
		n := NewNode()
		n.AddValue("n", ScalarValue(NewIntegerScalar(big.NewInt(1))))
		n.AddValue("extra", ScalarValue(NewStringScalar("x"))) // unexpected field
		return NodeDocument(n)
	}()

	for _, tc := range []struct {
		name   string
		doc    Document
		wantOK bool
	}{
		{"shape-ok", ok, true},
		{"shape-bad", shapeBad, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, mdiags, merr := Materialize(tc.doc, s)
			if merr != nil {
				t.Fatal(merr)
			}
			vdiags := Validate(tc.doc, s)
			mOK, vOK := len(mdiags) == 0, len(vdiags) == 0
			if mOK != tc.wantOK || vOK != tc.wantOK {
				t.Fatalf("Materialize ok=%v Validate ok=%v want both %v", mOK, vOK, tc.wantOK)
			}
		})
	}
}

// TestMaterializeValidateLockstepOnLeafUpgrade is the leaf-conversion half
// of the invariant. Unlike shape checking, a value-exact upgrade (e.g.
// 1.0 -> integer) is something Validate has no concept of at all: Validate
// checks a leaf's *existing* kind against the declared kind and never
// coerces, so a number-kind 1.0 against a declared `integer` field
// legitimately fails Validate on the ORIGINAL document even though
// Materialize accepts it (that is the entire point of materialize
// existing as a separate operation from validate). The meaningful
// lockstep comparison here is therefore post-materialization: whatever
// Materialize successfully upgrades, running Validate against the
// *materialized* result must accept, since every leaf now literally has
// its declared kind.
func TestMaterializeValidateLockstepOnLeafUpgrade(t *testing.T) {
	r := &Record{
		Name:   "R",
		Fields: []Field{{Label: "n", Type: ScalarType(KindInteger, false), Cardinality: DefaultCardinality()}},
	}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}

	n := NewNode()
	n.AddValue("n", ScalarValue(NewNumberScalar(1.0))) // value-exact upgrade target
	original := NodeDocument(n)

	// Validate rejects the ORIGINAL: a number-kind value never matches a
	// declared integer kind without conversion, which validate never does.
	if vdiags := Validate(original, s); len(vdiags) == 0 {
		t.Fatal("want Validate to reject the original unconverted document, got ok")
	}

	materialized, mdiags, merr := Materialize(original, s)
	if merr != nil || len(mdiags) != 0 {
		t.Fatalf("want Materialize to accept, got diags=%v err=%v", mdiags, merr)
	}
	// Lockstep: once materialized, Validate must now accept it -- the
	// upgraded leaf's kind literally matches the declared kind.
	if vdiags := Validate(materialized, s); len(vdiags) != 0 {
		t.Fatalf("lockstep violated: Validate rejects materialize's own output: %v", vdiags)
	}
}

// --- shape/cardinality: same codes as validate, unexpected field kept ---

func TestMaterializeUnexpectedFieldIsKeptNotDroppedButStillFails(t *testing.T) {
	r := &Record{
		Name:   "R",
		Fields: []Field{{Label: "n", Type: ScalarType(KindInteger, false), Cardinality: DefaultCardinality()}},
	}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}
	n := NewNode()
	n.AddValue("n", ScalarValue(NewIntegerScalar(big.NewInt(1))))
	n.AddValue("extra", ScalarValue(NewStringScalar("x")))
	doc := NodeDocument(n)

	got, diags, err := Materialize(doc, s)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeValidateUnexpectedField) {
		t.Fatalf("want validate.unexpected-field, got %v", codesOf(diags))
	}
	found := false
	for _, e := range got.Node.Edges {
		if e.Label == "extra" {
			found = true
			v, _ := e.Target.Value()
			if v.Scalar.Kind != KindString || v.Scalar.Str != "x" {
				t.Fatalf("extra field mutated: got %+v", v.Scalar)
			}
		}
	}
	if !found {
		t.Fatal("unexpected field was dropped from output, want it kept unchanged")
	}
}

func TestMaterializeCardinalityUsesValidateCode(t *testing.T) {
	r := &Record{
		Name:   "R",
		Fields: []Field{{Label: "n", Type: ScalarType(KindInteger, false), Cardinality: Cardinality{Min: 2, Max: 2}}},
	}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}
	n := NewNode()
	n.AddValue("n", ScalarValue(NewIntegerScalar(big.NewInt(1))))
	doc := NodeDocument(n)

	_, diags, err := Materialize(doc, s)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeValidateCardinality) {
		t.Fatalf("want validate.cardinality, got %v", codesOf(diags))
	}
}

// --- remaining coverage: shape mismatches, null handling, nested records,
// depth limit, non-node root, same-kind identity leaves ---

func TestMaterializeScalarTargetButGotObjectFails(t *testing.T) {
	r := &Record{
		Name:   "R",
		Fields: []Field{{Label: "n", Type: ScalarType(KindInteger, false), Cardinality: DefaultCardinality()}},
	}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}
	n := NewNode()
	n.AddNode("n", NewNode())
	doc := NodeDocument(n)

	_, diags, err := Materialize(doc, s)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeValidateShapeMismatch) {
		t.Fatalf("want validate.shape-mismatch, got %v", codesOf(diags))
	}
}

func TestMaterializeRecordTargetButGotScalarFails(t *testing.T) {
	r := &Record{
		Name:   "R",
		Fields: []Field{{Label: "n", Type: ScalarType(KindInteger, false), Cardinality: DefaultCardinality()}},
	}
	s := Schema{Root: "R", Env: map[string]*Record{"R": r}}
	doc := ValueDocument(ScalarValue(NewStringScalar("not a record")))

	got, diags, err := Materialize(doc, s)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeValidateShapeMismatch) {
		t.Fatalf("want validate.shape-mismatch, got %v", codesOf(diags))
	}
	// A non-node root has no edges to rebuild; the best-effort output
	// hands the original (non-node) target back unchanged.
	if got.IsNode {
		t.Fatal("want the original non-node document back, got a node")
	}
}

func TestMaterializeNullLeafAllowedOnlyWhenNullable(t *testing.T) {
	// Not nullable: rejected, and never attempts an upgrade.
	doc := oneFieldDoc(NullValue())
	_, diags, err := Materialize(doc, scalarRecordSchema(KindInteger, false))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeValidateNullNotAllowed) {
		t.Fatalf("want validate.null-not-allowed, got %v", codesOf(diags))
	}

	// Nullable: accepted, null stays null (not "upgraded" to anything).
	got, diags2, err2 := Materialize(doc, scalarRecordSchema(KindInteger, true))
	if err2 != nil || len(diags2) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags2, err2)
	}
	v, _ := got.Node.Edges[0].Target.Value()
	if !v.IsNull {
		t.Fatalf("want null to stay null, got %+v", v)
	}
}

func TestMaterializeNestedRecordUpgradesLeavesAtEveryDepth(t *testing.T) {
	inner := &Record{
		Name:   "Inner",
		Fields: []Field{{Label: "n", Type: ScalarType(KindDate, false), Cardinality: DefaultCardinality()}},
	}
	outer := &Record{
		Name:   "Outer",
		Fields: []Field{{Label: "child", Type: RefType("Inner"), Cardinality: DefaultCardinality()}},
	}
	s := Schema{Root: "Outer", Env: map[string]*Record{"Outer": outer, "Inner": inner}}

	innerNode := NewNode()
	innerNode.AddValue("n", ScalarValue(NewStringScalar("2024-01-01")))
	root := NewNode()
	root.AddNode("child", innerNode)
	doc := NodeDocument(root)

	got, diags, err := Materialize(doc, s)
	if err != nil || len(diags) != 0 {
		t.Fatalf("want ok, got diags=%v err=%v", diags, err)
	}
	childNode, _ := got.Node.Edges[0].Target.Node()
	v, _ := childNode.Edges[0].Target.Value()
	if v.Scalar.Kind != KindDate {
		t.Fatalf("want nested leaf upgraded to KindDate, got %v", v.Scalar.Kind)
	}
}

func TestMaterializeRespectsDepthLimit(t *testing.T) {
	// A schema that recurses through itself lets a document exceed the
	// depth limit; materialize must report document.limit.depth (via the
	// shared LimitChecker), not a materialize.* or validate.* code, and
	// must not panic (mirrors validate's own depth-limit test intent).
	rec := &Record{Name: "Self", Fields: []Field{{Label: "child", Type: RefType("Self"), Cardinality: Cardinality{Min: 0, Max: 1}}}}
	s := Schema{Root: "Self", Env: map[string]*Record{"Self": rec}}

	limits := DefaultLimits()
	limits.MaxDepth = 2
	checker := NewLimitChecker(limits)

	deep := NewNode() // depth 3: root(1) -> child(2) -> child(3), exceeds MaxDepth=2
	leaf := NewNode()
	mid := NewNode()
	mid.AddNode("child", leaf)
	deep.AddNode("child", mid)
	doc := NodeDocument(deep)

	result := []Diagnostic{}
	out := materialize(documentTarget(doc), s, RefType(s.Root), RootPath(), checker, &result)
	_ = out
	if !hasCode(result, CodeDocumentLimitDepth) {
		t.Fatalf("want document.limit.depth, got %v", codesOf(result))
	}
}

func TestMaterializeSameKindLeafIsAcceptedUnchanged(t *testing.T) {
	for _, s := range []Scalar{
		NewStringScalar("x"),
		NewBooleanScalar(true),
		NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 1}),
		NewTimeScalar(TimeValue{Hour: 1, Minute: 2}),
		NewDateTimeScalar(DateTimeValue{Date: DateValue{Year: 2024, Month: 1, Day: 1}, Time: TimeValue{Hour: 1, Minute: 2}}),
		NewNumberScalar(2.5),
	} {
		doc := oneFieldDoc(ScalarValue(s))
		got, diags, err := Materialize(doc, scalarRecordSchema(s.Kind, false))
		if err != nil || len(diags) != 0 {
			t.Fatalf("kind=%v: want ok, got diags=%v err=%v", s.Kind, diags, err)
		}
		v, _ := got.Node.Edges[0].Target.Value()
		if !v.Scalar.Equal(s) {
			t.Fatalf("kind=%v: want unchanged %+v, got %+v", s.Kind, s, v.Scalar)
		}
	}
}

func TestMaterializeNilRecordSchemaReportsUnexpectedFieldEveryEdge(t *testing.T) {
	// A Ref whose name is absent from Env (not-well-formed schema) resolves
	// to a nil Record -- validate.go's resolveType/conformRecord already
	// handle this by treating a nil record as "no fields, closed"; confirm
	// materialize's shared walkRecordShape path does too, and does not
	// panic on a nil rec.
	s := Schema{Root: "Missing", Env: map[string]*Record{}}
	n := NewNode()
	n.AddValue("x", ScalarValue(NewStringScalar("y")))
	doc := NodeDocument(n)

	got, diags, err := Materialize(doc, s)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(diags, CodeValidateUnexpectedField) {
		t.Fatalf("want validate.unexpected-field, got %v", codesOf(diags))
	}
	if len(got.Node.Edges) != 1 || got.Node.Edges[0].Label != "x" {
		t.Fatalf("want the edge kept unchanged, got %+v", got.Node.Edges)
	}
}

func TestMaterializeInfiniteAndNaNFloatsNeverUpgradeToInteger(t *testing.T) {
	for _, f := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		doc := oneFieldDoc(ScalarValue(NewNumberScalar(f)))
		_, diags, err := Materialize(doc, scalarRecordSchema(KindInteger, false))
		if err != nil {
			t.Fatal(err)
		}
		if !hasCode(diags, CodeMaterializeInexactConversion) {
			t.Fatalf("f=%v: want materialize.inexact-conversion, got %v", f, codesOf(diags))
		}
	}
}

// TestMaterializeDefaultCaseExercisesDefensiveDefault mirrors
// validate_test.go's TestResolveTypeDefaultCase: materialize's switch on
// resolveType's resolvedKind has the same defensive default branch as
// validate.go's conform, guarding against a future resolvedKind constant
// added without updating materialize. schema.go's exported Type struct
// makes an actual illegal TypeKind constructible from outside the
// package, so this is a real (if narrow) reachable path, not dead code.
func TestMaterializeDefaultCaseExercisesDefensiveDefault(t *testing.T) {
	checker := NewLimitChecker(DefaultLimits())
	result := []Diagnostic{}
	original := ValueTarget(ScalarValue(NewStringScalar("x")))
	got := materialize(original, personSchema(), Type{Kind: TypeKind(99)}, RootPath(), checker, &result)
	if len(result) != 0 {
		t.Fatalf("want no diagnostics, got %v", result)
	}
	v, _ := got.Value()
	if v.Scalar.Str != "x" {
		t.Fatalf("want the original target back unchanged, got %+v", v)
	}
}
