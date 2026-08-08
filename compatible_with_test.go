package omnist

import "testing"

// --- scalar_sub (§6.3) ---

func TestScalarSub(t *testing.T) {
	cases := []struct {
		name string
		a, b Type
		want bool
	}{
		{"same kind true", ScalarType(KindString, false), ScalarType(KindString, false), true},
		{"integer sub number true", ScalarType(KindInteger, false), ScalarType(KindNumber, false), true},
		{"number not sub integer false", ScalarType(KindNumber, false), ScalarType(KindInteger, false), false},
		{"unrelated kinds false", ScalarType(KindString, false), ScalarType(KindInteger, false), false},
		{"nullable narrowing false", ScalarType(KindString, true), ScalarType(KindString, false), false},
		{"nullable widening true", ScalarType(KindString, false), ScalarType(KindString, true), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scalarSub(c.a, c.b)
			if got != c.want {
				t.Errorf("scalarSub(%+v, %+v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// --- compatible_with / equivalent (§6.6, §6.7) ---

// TestCompatibleWithWorkedExample is §6.6's own worked example: A has an
// optional nick field, B doesn't. compatible_with(A, B) is false (A may
// emit nick, B is closed), compatible_with(B, A) is true (everything B
// emits, A accepts).
func TestCompatibleWithWorkedExample(t *testing.T) {
	a := mustParseOSD(t, `record User {
		"id": string,
		"name": string,
		"nick" [0,1]: string,
	} root User`)
	b := mustParseOSD(t, `record User {
		"id": string,
		"name": string,
	} root User`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false")
	}
	if !CompatibleWith(b, a) {
		t.Error("compatible_with(B, A) = false, want true")
	}
}

func TestEquivalentWorkedExample(t *testing.T) {
	a := mustParseOSD(t, `record User {
		"id": string,
		"name": string,
		"nick" [0,1]: string,
	} root User`)
	b := mustParseOSD(t, `record User {
		"id": string,
		"name": string,
	} root User`)

	if Equivalent(a, b) {
		t.Error("equivalent(A, B) = true, want false (only one direction holds)")
	}
}

func TestEquivalentBothDirections(t *testing.T) {
	a := mustParseOSD(t, `record User { "id": string } root User`)
	b := mustParseOSD(t, `record Person { "id": string } root Person`)

	if !Equivalent(a, b) {
		t.Error("equivalent(A, B) = false, want true (structurally identical, differing only in record name)")
	}
}

// TestCompatibleWithVacuousUnsatisfiableField: an A-side record with a
// mandatory field pointing at an unsatisfiable A-side record. A can never
// emit that field (it's unsatisfiable, so nothing beneath it terminates),
// so record_sub's pass 1 must skip it rather than requiring B to have a
// matching field, and this must not infinite-loop.
func TestCompatibleWithVacuousUnsatisfiableField(t *testing.T) {
	a := mustParseOSD(t, `
		record Bad { "self": Bad }
		record Root { "bad": Bad }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { }
		root Root
	`)

	if !CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = false, want true (A's mandatory field is unsatisfiable, so A vacuously emits nothing there)")
	}
}

// TestCompatibleWithVacuousOptionalUnsatisfiableField: an A-side field
// that is itself optional (min==0) and typed at an unsatisfiable A-side
// record. This hits record_sub pass 1's own vacuous-skip
// (`fa.min == 0 and fa.type is Ref and fa.type.name not in sat_a`)
// directly, distinct from TestCompatibleWithVacuousUnsatisfiableField's
// mandatory-field case (which is caught earlier, by sub's top-level
// vacuous check on the Ref type itself before record_sub is ever
// entered for that field).
func TestCompatibleWithVacuousOptionalUnsatisfiableField(t *testing.T) {
	a := mustParseOSD(t, `
		record Bad { "self": Bad }
		record Root { "bad" [0,1]: Bad }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { }
		root Root
	`)

	if !CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = false, want true (A's 'bad' field is optional and unsatisfiable, so A vacuously never emits it)")
	}
}

// TestCompatibleWithAnyAbsorbsOnRight: a differently-shaped A-side field is
// absorbed by a B-side any field.
func TestCompatibleWithAnyAbsorbsOnRight(t *testing.T) {
	a := mustParseOSD(t, `
		record Shape { "x": string, "y": integer }
		record Root { "field": Shape }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "field": any }
		root Root
	`)

	if !CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = false, want true (B's any field absorbs any A-side shape)")
	}
}

// TestCompatibleWithAnyNotAbsorbedOnLeft: an A-side any field is not
// absorbed by a differently-shaped B-side field — only any holds any.
func TestCompatibleWithAnyNotAbsorbedOnLeft(t *testing.T) {
	a := mustParseOSD(t, `
		record Root { "field": any }
		root Root
	`)
	b := mustParseOSD(t, `
		record Shape { "x": string, "y": integer }
		record Root { "field": Shape }
		root Root
	`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false (A's any field is not guaranteed to match B's specific shape)")
	}
}

// TestCompatibleWithRecursiveCompatible: mutually-recursive records on both
// sides that ARE compatible.
func TestCompatibleWithRecursiveCompatible(t *testing.T) {
	a := mustParseOSD(t, `
		record NodeA { "value": string, "next" [0,1]: NodeA }
		root NodeA
	`)
	b := mustParseOSD(t, `
		record NodeB { "value": string, "next" [0,1]: NodeB }
		root NodeB
	`)

	if !CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = false, want true (structurally identical recursive shapes)")
	}
}

// TestCompatibleWithRecursiveIncompatible: a recursive-schema case where the
// correct answer is false, to confirm the coinductive assumption doesn't
// just default everything to true.
func TestCompatibleWithRecursiveIncompatible(t *testing.T) {
	a := mustParseOSD(t, `
		record NodeA { "value": string, "extra": string, "next" [0,1]: NodeA }
		root NodeA
	`)
	b := mustParseOSD(t, `
		record NodeB { "value": string, "next" [0,1]: NodeB }
		root NodeB
	`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false (A's recursive record emits an extra field B is closed against)")
	}
}

// TestCompatibleWithAliasing constructs a schema by hand (not through the
// OSD parser, since two names bound to one *Record isn't expressible in
// OSD text) where two different reference names resolve to the very same
// underlying *Record. This is the aliasing case §6.6 explicitly warns
// about: keying the memo on reference names instead of resolved-definition
// identity would treat the two names as different nodes.
func TestCompatibleWithAliasing(t *testing.T) {
	shared := &Record{
		Name: "Shared",
		Fields: []Field{
			{Label: "value", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
		},
	}
	root := &Record{
		Name: "Root",
		Fields: []Field{
			// Two different labels, both typed at refs ("Alias1", "Alias2")
			// that resolve to the identical *Record pointer.
			{Label: "one", Type: RefType("Alias1"), Cardinality: DefaultCardinality()},
			{Label: "two", Type: RefType("Alias2"), Cardinality: DefaultCardinality()},
		},
	}
	a := Schema{
		Root: "Root",
		Env: map[string]*Record{
			"Root":   root,
			"Alias1": shared,
			"Alias2": shared,
		},
		EnvOrder: []string{"Root", "Alias1", "Alias2"},
	}

	bShared := &Record{
		Name: "Shared",
		Fields: []Field{
			{Label: "value", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
		},
	}
	bRoot := &Record{
		Name: "Root",
		Fields: []Field{
			{Label: "one", Type: RefType("Alias1"), Cardinality: DefaultCardinality()},
			{Label: "two", Type: RefType("Alias2"), Cardinality: DefaultCardinality()},
		},
	}
	b := Schema{
		Root: "Root",
		Env: map[string]*Record{
			"Root":   bRoot,
			"Alias1": bShared,
			"Alias2": bShared,
		},
		EnvOrder: []string{"Root", "Alias1", "Alias2"},
	}

	if !CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = false, want true (aliased refs, identical structure)")
	}
	if !Equivalent(a, b) {
		t.Error("equivalent(A, B) = false, want true (aliased refs, identical structure)")
	}
}

// TestCompatibleWithValueVersusObject: A's field is a record, B's
// same-label field is a scalar — the "value versus object" default case
// in sub, which the resolvedRecord/resolvedRecord fast path and the Any
// clauses don't reach.
func TestCompatibleWithValueVersusObject(t *testing.T) {
	a := mustParseOSD(t, `
		record Shape { "x": string }
		record Root { "field": Shape }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "field": string }
		root Root
	`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false (A's field is a record, B's is a scalar)")
	}
}

// TestCompatibleWithFieldNeverEmitted: an A-side field with max==0 (never
// emitted per record_sub's own skip, distinct from the optional-vacuous
// skip) that B doesn't declare at all — must not cause a false rejection.
func TestCompatibleWithFieldNeverEmitted(t *testing.T) {
	a := mustParseOSD(t, `
		record Root { "dead" [0,0]: string, "id": string }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "id": string }
		root Root
	`)

	if !CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = false, want true (A's 'dead' field has max 0, never emitted)")
	}
}

// TestCompatibleWithCardinalityBoundsTooNarrow: B declares the shared
// field but with bounds that don't cover A's — B's max is narrower than
// A's max.
func TestCompatibleWithCardinalityBoundsTooNarrow(t *testing.T) {
	a := mustParseOSD(t, `
		record Root { "tags" [0,5]: string }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "tags" [0,2]: string }
		root Root
	`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false (A may emit up to 5 tags, B only allows up to 2)")
	}
}

// TestCompatibleWithCardinalityMinTooHigh: B requires a higher minimum
// count for a shared field than B.min <= A.min allows.
func TestCompatibleWithCardinalityMinTooHigh(t *testing.T) {
	a := mustParseOSD(t, `
		record Root { "tags" [0,5]: string }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "tags" [1,5]: string }
		root Root
	`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false (B requires at least 1 tag, A guarantees only 0)")
	}
}

// TestRecordSubFailsPass1Only: A emits a field B doesn't allow at all
// (label B doesn't declare), but A guarantees everything B requires.
func TestRecordSubFailsPass1Only(t *testing.T) {
	a := mustParseOSD(t, `
		record Root { "id": string, "extra": string }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "id": string }
		root Root
	`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false (pass 1: A emits 'extra', B is closed against it)")
	}
}

// TestRecordSubFailsPass2Only: B requires a field A doesn't guarantee
// (A has it optional), while A emits nothing B disallows.
func TestRecordSubFailsPass2Only(t *testing.T) {
	a := mustParseOSD(t, `
		record Root { "id" [0,1]: string }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "id": string }
		root Root
	`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false (pass 2: B requires 'id', A only guarantees it optionally)")
	}
}

// TestRecordSubFailsPass2FieldAbsent: B requires a field that A doesn't
// declare at all (distinct from TestRecordSubFailsPass2Only, where A
// declares the field but only optionally).
func TestRecordSubFailsPass2FieldAbsent(t *testing.T) {
	a := mustParseOSD(t, `
		record Root { "id": string }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "id": string, "required": string }
		root Root
	`)

	if CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = true, want false (B requires 'required', A doesn't declare it at all)")
	}
}

// TestRecordSubPassesBoth: A emits nothing B disallows, and A guarantees
// everything B requires.
func TestRecordSubPassesBoth(t *testing.T) {
	a := mustParseOSD(t, `
		record Root { "id": string, "extra" [0,1]: string }
		root Root
	`)
	b := mustParseOSD(t, `
		record Root { "id": string, "extra" [0,1]: string }
		root Root
	`)

	if !CompatibleWith(a, b) {
		t.Error("compatible_with(A, B) = false, want true (both fields allowed by B with adequate bounds, B requires only id, A guarantees it)")
	}
}
