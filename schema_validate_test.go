package omnist

import (
	"errors"
	"testing"
)

// Issue #121: spec §3.3 S-1..S-7 and the §5.4 label rules, enforced by
// Schema.Validate, NewRecord and NewSchema, each rule at the code and Schema
// path §8.4.1 (E-13, E-30) fixes for it.

var (
	tStr = ScalarType(KindString, false)
	card = DefaultCardinality()
)

func fl(label string, t Type, c Cardinality) Field {
	return Field{Label: label, Type: t, Cardinality: c}
}

func lit(root string, order []string, recs ...*Record) Schema {
	env := map[string]*Record{}
	for _, r := range recs {
		env[r.Name] = r
	}
	return Schema{Root: root, Env: env, EnvOrder: order}
}

func wantDiag(t *testing.T, label string, err error, code Code, path string) {
	t.Helper()
	var d Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("%s: error %v (%T) is not a Diagnostic", label, err, err)
	}
	if d.Code != code || d.Path != path || d.Severity != SeverityError || d.Message == "" {
		t.Errorf("%s: got %+v, want code %s path %q", label, d, code, path)
	}
}

type wfCase struct {
	name   string
	schema Schema
	code   Code
	path   string
}

func wfCases() []wfCase {
	rec := func(n string, f ...Field) *Record { return &Record{Name: n, Fields: f} }
	return []wfCase{
		// S-1
		{"S-1 no root", lit("", []string{"R"}, rec("R", fl("a", tStr, card))), CodeSchemaNoRoot, "$"},
		{"S-1 dangling root", lit("X", []string{"R"}, rec("R")), CodeSchemaUnknownType, "$"},
		// S-2
		{"S-2 min above max", lit("R", []string{"R"}, rec("R", fl("a", tStr, Cardinality{Min: 2, Max: 1}))), CodeSchemaInvalidCardinality, "R.a"},
		// S-3
		{"S-3 scalar keyword", lit("integer", []string{"integer"}, rec("integer")), CodeSchemaReservedName, "integer"},
		{"S-3 datetime", lit("datetime", []string{"datetime"}, rec("datetime")), CodeSchemaReservedName, "datetime"},
		{"S-3 any", lit("any", []string{"any"}, rec("any")), CodeSchemaReservedName, "any"},
		// S-4
		{"S-4 duplicate record", lit("R", []string{"R", "S", "R"}, rec("R"), rec("S")), CodeSchemaDuplicateRecord, "R"},
		// S-5
		{"S-5 duplicate field", lit("R", []string{"R"}, rec("R", fl("a", tStr, card), fl("b", tStr, card), fl("a", tStr, card))), CodeSchemaDuplicateField, "R"},
		// S-6
		{"S-6 dangling ref", lit("R", []string{"R"}, rec("R", fl("a", tStr, card), fl("b", RefType("Z"), card))), CodeSchemaUnknownType, "R.b"},
		// S-7
		{"S-7 nullable ref", lit("R", []string{"R"}, rec("R", fl("a", Type{Kind: TypeRefKind, RefName: "R", Nullable: true}, card))), CodeSchemaNullableRef, "R.a"},
		{"S-7 nullable any", lit("R", []string{"R"}, rec("R", fl("a", Type{Kind: TypeAnyKind, Nullable: true}, card))), CodeSchemaNullableAny, "R.a"},
		// §5.4 label rules
		{"empty label", lit("R", []string{"R"}, rec("R", fl("", tStr, card))), CodeSchemaEmptyLabel, "R"},
		{"open bracket", lit("R", []string{"R"}, rec("R", fl("a[", tStr, card))), CodeSchemaBracketInLabel, "R"},
		{"close bracket", lit("R", []string{"R"}, rec("R", fl("]", tStr, card))), CodeSchemaBracketInLabel, "R"},
		{"bracket pair", lit("R", []string{"R"}, rec("R", fl("a[1]", tStr, card))), CodeSchemaBracketInLabel, "R"},
		// Ordering: a record-level problem in an earlier record wins over a later field's.
		{"first record wins", lit("R", []string{"S", "R"}, rec("S", fl("", tStr, card)), rec("R", fl("a[", tStr, card))), CodeSchemaEmptyLabel, "S"},
		// Within a field the label is checked before the cardinality.
		{"label before cardinality", lit("R", []string{"R"}, rec("R", fl("", tStr, Cardinality{Min: 2, Max: 1}))), CodeSchemaEmptyLabel, "R"},
	}
}

func TestSchemaValidateRules(t *testing.T) {
	for _, c := range wfCases() {
		wantDiag(t, c.name, c.schema.Validate(), c.code, c.path)
	}
}

func TestSchemaValidateAcceptsWellFormed(t *testing.T) {
	ok := []Schema{
		lit("R", []string{"R"}, &Record{Name: "R"}), // empty record
		lit("R", []string{"R", "S"},
			&Record{Name: "R", Fields: []Field{
				fl("a", tStr, card),
				fl("n", ScalarType(KindInteger, true), Cardinality{Min: 0, Unbounded: true}),
				fl("s", RefType("S"), Cardinality{Min: 0, Max: 1}),
				fl("x", AnyType(), Cardinality{Min: 2, Max: 5}),
				fl("self", RefType("R"), Cardinality{Min: 0, Max: 1}),
				fl("a b\"\\é", tStr, card),
				fl("zero", tStr, Cardinality{Min: 0, Max: 0}), // §3.4/S-15: representable
				fl("String", tStr, card),
			}},
			&Record{Name: "S"}),
		// Unbounded ignores Max entirely.
		lit("R", []string{"R"}, &Record{Name: "R", Fields: []Field{fl("a", tStr, Cardinality{Min: 5, Max: 0, Unbounded: true})}}),
		// Case-sensitive reserved names.
		lit("String", []string{"String"}, &Record{Name: "String"}),
	}
	for i, s := range ok {
		if err := s.Validate(); err != nil {
			t.Errorf("schema %d: unexpected error %v", i, err)
		}
	}
}

// An EnvOrder entry with no record in Env is skipped (no spec code covers it)
// rather than panicking.
func TestSchemaValidateSkipsAbsentEnvEntries(t *testing.T) {
	s := Schema{Root: "R", Env: map[string]*Record{"R": {Name: "R", Fields: []Field{fl("a", RefType("Ghost"), card)}}, "N": nil}, EnvOrder: []string{"Ghost", "N", "R"}}
	wantDiag(t, "ghost", s.Validate(), CodeSchemaUnknownType, "R.a")
	s.Env["R"].Fields = nil
	if err := s.Validate(); err != nil {
		t.Errorf("unexpected error %v", err)
	}
}

func TestNewRecord(t *testing.T) {
	r, err := NewRecord("R", fl("a", tStr, card), fl("b", RefType("R"), Cardinality{Min: 0, Max: 1}))
	if err != nil || r == nil || r.Name != "R" || len(r.Fields) != 2 {
		t.Fatalf("NewRecord valid: %v %v", r, err)
	}
	cases := []struct {
		name   string
		rname  string
		fields []Field
		code   Code
		path   string
	}{
		{"reserved", "string", nil, CodeSchemaReservedName, "string"},
		{"empty label", "R", []Field{fl("", tStr, card)}, CodeSchemaEmptyLabel, "R"},
		{"bracket", "R", []Field{fl("x]", tStr, card)}, CodeSchemaBracketInLabel, "R"},
		{"duplicate", "R", []Field{fl("a", tStr, card), fl("a", tStr, card)}, CodeSchemaDuplicateField, "R"},
		{"cardinality", "R", []Field{fl("a", tStr, Cardinality{Min: 3, Max: 1})}, CodeSchemaInvalidCardinality, "R.a"},
		{"nullable ref", "R", []Field{fl("a", Type{Kind: TypeRefKind, RefName: "R", Nullable: true}, card)}, CodeSchemaNullableRef, "R.a"},
		{"nullable any", "R", []Field{fl("a", Type{Kind: TypeAnyKind, Nullable: true}, card)}, CodeSchemaNullableAny, "R.a"},
	}
	for _, c := range cases {
		r, err := NewRecord(c.rname, c.fields...)
		if r != nil {
			t.Errorf("%s: want nil record on error", c.name)
		}
		wantDiag(t, c.name, err, c.code, c.path)
	}
}

func TestNewSchema(t *testing.T) {
	a, _ := NewRecord("A", fl("b", RefType("B"), card))
	b, _ := NewRecord("B", fl("a", RefType("A"), Cardinality{Min: 0, Max: 1}))
	s, err := NewSchema("A", a, nil, b)
	if err != nil {
		t.Fatal(err)
	}
	if s.Root != "A" || len(s.EnvOrder) != 2 || s.EnvOrder[0] != "A" || s.EnvOrder[1] != "B" || s.Env["A"] != a || s.Env["B"] != b {
		t.Errorf("unexpected schema %+v", s)
	}

	_, err = NewSchema("A", a)
	wantDiag(t, "dangling ref", err, CodeSchemaUnknownType, "A.b")
	_, err = NewSchema("", a, b)
	wantDiag(t, "no root", err, CodeSchemaNoRoot, "$")
	_, err = NewSchema("Q", a, b)
	wantDiag(t, "dangling root", err, CodeSchemaUnknownType, "$")
	_, err = NewSchema("A", a, b, a)
	wantDiag(t, "duplicate record", err, CodeSchemaDuplicateRecord, "A")
	// A record literal that skipped NewRecord is still caught.
	_, err = NewSchema("R", &Record{Name: "R", Fields: []Field{fl("", tStr, card)}})
	wantDiag(t, "unchecked record literal", err, CodeSchemaEmptyLabel, "R")
	zero, _ := NewSchema("R", &Record{Name: "R", Fields: []Field{fl("", tStr, card)}})
	if zero.Root != "" || zero.Env != nil || zero.EnvOrder != nil {
		t.Errorf("want zero Schema on error, got %+v", zero)
	}
}
