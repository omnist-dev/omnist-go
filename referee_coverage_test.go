package omnist

import "testing"

// This file closes the remaining branch coverage in referee.go that the
// 10-case spec self-test (referee_test.go) doesn't happen to exercise --
// referee.go is core comparison logic (like Scalar.Equal from issue #1),
// not conformance-harness execution plumbing, so it holds the repo's
// normal 100%-per-function coverage bar rather than tools/conformance/**'s
// documented exception (see .github/workflows/ci.yml).

func TestRefereeCoverageDocumentsEqualNodeVsValueMismatch(t *testing.T) {
	node := mustOML(t, "a: 1")
	value := mustOML(t, "42")
	if DocumentsEqual(node, value) {
		t.Fatal("a node-shaped and a value-shaped Document must not compare equal")
	}
	if DocumentsEqual(value, node) {
		t.Fatal("comparison must be symmetric")
	}
}

func TestRefereeCoverageValuesEqualNullMismatch(t *testing.T) {
	null := mustOML(t, "null")
	nonNull := mustOML(t, "1")
	if DocumentsEqual(null, nonNull) {
		t.Fatal("null and non-null bare values must not compare equal")
	}
}

func TestRefereeCoverageValuesEqualBothNull(t *testing.T) {
	a := mustOML(t, "null")
	b := mustOML(t, "null")
	if !DocumentsEqual(a, b) {
		t.Fatal("two null bare values must compare equal")
	}
}

func TestRefereeCoverageScalarsEqualBothNaN(t *testing.T) {
	a := mustOML(t, "nan")
	b := mustOML(t, "nan")
	if !DocumentsEqual(a, b) {
		t.Fatal("two NaN-valued number scalars must compare equal under the referee (unlike plain Scalar.Equal / Go's ==)")
	}
}

func TestRefereeCoverageScalarsEqualKindMismatch(t *testing.T) {
	integer := mustOML(t, "1")
	number := mustOML(t, "1.0")
	if DocumentsEqual(integer, number) {
		t.Fatal("an integer-kind and a number-kind scalar of the same magnitude must not compare equal (spec D-5)")
	}
}

func TestRefereeCoverageTargetsEqualNodeVsValueMismatch(t *testing.T) {
	a := mustOML(t, "m: {}")
	b := mustOML(t, "m: 1")
	if DocumentsEqual(a, b) {
		t.Fatal("an edge whose target is a node must not compare equal to one whose target is a value")
	}
}

func TestRefereeCoverageNodesEqualLengthMismatch(t *testing.T) {
	a := mustOML(t, "a: 1")
	b := mustOML(t, "a: 1; b: 2")
	if DocumentsEqual(a, b) {
		t.Fatal("nodes with a different edge count must not compare equal")
	}
}

func TestRefereeCoverageNodesEqualLabelMismatch(t *testing.T) {
	a := mustOML(t, "a: 1")
	b := mustOML(t, "b: 1")
	if DocumentsEqual(a, b) {
		t.Fatal("nodes whose edges have different labels must not compare equal")
	}
}

func TestRefereeCoverageSchemaExactEqualRootMismatch(t *testing.T) {
	a := mustOSD(t, `record R { "x": string } root R`)
	b := mustOSD(t, `record R { "x": string } record S { "x": string } root S`)
	if SchemasEqual(a, b, ModeExact) {
		t.Fatal("schemas with different roots must not compare equal")
	}
}

func TestRefereeCoverageSchemaExactEqualEnvLengthMismatch(t *testing.T) {
	a := mustOSD(t, `record R { "x": string } root R`)
	b := mustOSD(t, `record R { "x": string } record S { "y": string } root R`)
	if SchemasEqual(a, b, ModeExact) {
		t.Fatal("schemas with a different number of records must not compare equal")
	}
}

func TestRefereeCoverageSchemaExactEqualMissingRecordInOther(t *testing.T) {
	a := mustOSD(t, `record R { "x": string } record S { "y": string } root R`)
	b := mustOSD(t, `record R { "x": string } record T { "y": string } root R`)
	if SchemasEqual(a, b, ModeExact) {
		t.Fatal("a record name present in one env but absent from the other must not compare equal")
	}
}

func TestRefereeCoverageRecordFieldSetEqualExactFieldCountMismatch(t *testing.T) {
	a := mustOSD(t, `record R { "x": string } root R`)
	b := mustOSD(t, `record R { "x": string, "y": string } root R`)
	if SchemasEqual(a, b, ModeExact) {
		t.Fatal("records with a different field count must not compare equal")
	}
}

func TestRefereeCoverageRecordFieldSetEqualExactMissingLabel(t *testing.T) {
	a := mustOSD(t, `record R { "x": string } root R`)
	b := mustOSD(t, `record R { "y": string } root R`)
	if SchemasEqual(a, b, ModeExact) {
		t.Fatal("records whose fields have different labels must not compare equal")
	}
}

func TestRefereeCoverageTypesEqualExactKindMismatch(t *testing.T) {
	a := mustOSD(t, `record R { "x": string } root R`)
	b := mustOSD(t, `record R { "x": R } root R`)
	if SchemasEqual(a, b, ModeExact) {
		t.Fatal("a scalar-typed field and a ref-typed field must not compare equal")
	}
}

func TestRefereeCoverageTypesEqualExactRefNameMismatch(t *testing.T) {
	a := mustOSD(t, `record R { "x": S } record S { "y": string } root R`)
	b := mustOSD(t, `record R { "x": T } record T { "y": string } root R`)
	if SchemasEqual(a, b, ModeExact) {
		t.Fatal("ref fields pointing at differently-named records must not compare equal under exact mode (no renaming permitted)")
	}
}

func TestRefereeCoverageTypesEqualExactAnyKind(t *testing.T) {
	a := mustOSD(t, `record R { "x": any } root R`)
	b := mustOSD(t, `record R { "x": any } root R`)
	if !SchemasEqual(a, b, ModeExact) {
		t.Fatal("two any-typed fields must compare equal")
	}
}

// recordsIsomorphic's "bName already claimed by a different aName" branch
// needs a genuine non-isomorphic pair: two records in A mapping to the
// same single record in B is impossible from well-formed distinct A
// records unless B's graph is smaller/differently shaped, so build one
// side where two different A records would both need to match the same B
// record.
func TestRefereeCoverageIsomorphicBNameAlreadyClaimed(t *testing.T) {
	// Left and Right are structurally identical (both single field "v":
	// string) so the first pairing (Left<->Same) succeeds fully on its own
	// merits, rather than short-circuiting early on a shape mismatch --
	// only then does the second pairing attempt (Right vs Same) reach the
	// "bName already claimed by a different aName" branch.
	a := mustOSD(t, `
		record Left  { "v": string }
		record Right { "v": string }
		record Root  { "p": Left, "q": Right }
		root Root
	`)
	b := mustOSD(t, `
		record Same { "v": string }
		record Root { "p": Same, "q": Same }
		root Root
	`)
	if SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("Left and Right cannot both isomorphically map to the same B record Same")
	}
}

func TestRefereeCoverageIsomorphicDanglingRef(t *testing.T) {
	// Not constructible through ReadOSD (S-6 rejects a dangling ref), so
	// build the Schema directly to exercise recordsIsomorphic's
	// !okA || !okB branch.
	a := Schema{
		Root:     "R",
		EnvOrder: []string{"R"},
		Env: map[string]*Record{
			"R": {Name: "R", Fields: []Field{
				{Label: "x", Type: Type{Kind: TypeRefKind, RefName: "Missing"}, Cardinality: Cardinality{Min: 1, Max: 1}},
			}},
		},
	}
	b := mustOSD(t, `record R { "x": S } record S { "y": string } root R`)
	if SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("a dangling ref on one side must not compare isomorphic")
	}
}

func TestRefereeCoverageIsomorphicFieldCountMismatch(t *testing.T) {
	a := mustOSD(t, `record S { "x": string } record R { "a": S } root R`)
	b := mustOSD(t, `record S { "x": string, "y": string } record R { "a": S } root R`)
	if SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("records with a different field count must not compare isomorphic")
	}
}

func TestRefereeCoverageIsomorphicMissingLabel(t *testing.T) {
	a := mustOSD(t, `record S { "x": string } record R { "a": S } root R`)
	b := mustOSD(t, `record S { "y": string } record R { "a": S } root R`)
	if SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("records whose fields have different labels must not compare isomorphic")
	}
}

func TestRefereeCoverageIsomorphicCardinalityMismatch(t *testing.T) {
	a := mustOSD(t, `record S { "x" [0,1]: string } record R { "a": S } root R`)
	b := mustOSD(t, `record S { "x": string } record R { "a": S } root R`)
	if SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("differing cardinality on a matched field must not compare isomorphic")
	}
}

func TestRefereeCoverageIsomorphicTypeKindMismatch(t *testing.T) {
	a := mustOSD(t, `record S { "x": string } record R { "a": S } root R`)
	b := mustOSD(t, `record T { "x": T } record R { "a": T } root R`)
	if SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("a scalar-typed field and a ref-typed field must not compare isomorphic")
	}
}

func TestRefereeCoverageIsomorphicScalarKindMismatch(t *testing.T) {
	a := mustOSD(t, `record S { "x": string } record R { "a": S } root R`)
	b := mustOSD(t, `record S { "x": integer } record R { "a": S } root R`)
	if SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("differing scalar kind on a matched field must not compare isomorphic")
	}
}

func TestRefereeCoverageIsomorphicNullableMismatch(t *testing.T) {
	a := mustOSD(t, `record S { "x": string } record R { "a": S } root R`)
	b := mustOSD(t, `record S { "x": string? } record R { "a": S } root R`)
	if SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("differing nullability on a matched field must not compare isomorphic")
	}
}

// A record reachable via two different fields back to an already-mapped
// name exercises recordsIsomorphic's "already mapped, consistent" early
// return (as opposed to the dangling/first-encounter path every other
// isomorphic test above exercises).
func TestRefereeCoverageIsomorphicRevisitsAlreadyMappedName(t *testing.T) {
	a := mustOSD(t, `record R { "a": R, "b": R } root R`)
	b := mustOSD(t, `record S { "a": S, "b": S } root S`)
	if !SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("a self-referential record reached twice must still compare isomorphic once R<->S is committed")
	}
}

func TestRefereeCoverageIsomorphicAnyKind(t *testing.T) {
	a := mustOSD(t, `record R { "x": any } root R`)
	b := mustOSD(t, `record R { "x": any } root R`)
	if !SchemasEqual(a, b, ModeIsomorphic) {
		t.Fatal("two any-typed fields must compare isomorphic")
	}
}
