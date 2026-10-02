package osd

// Issue #121: osd.Write must not emit text its own reader rejects. A Schema
// built by hand can violate spec §3.3's S-1..S-8 and the label rules of §5.4;
// Write validates first and fails with the structured schema.* diagnostic the
// reader would have raised, at the same path (spec §8.4.1, E-30).

import (
	"errors"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
)

var strT = omnist.ScalarType(omnist.KindString, false)

func rec(name string, fields ...omnist.Field) *omnist.Record {
	return &omnist.Record{Name: name, Fields: fields}
}

func fld(label string, t omnist.Type, c omnist.Cardinality) omnist.Field {
	return omnist.Field{Label: label, Type: t, Cardinality: c}
}

func handSchema(root string, order []string, recs ...*omnist.Record) omnist.Schema {
	env := map[string]*omnist.Record{}
	for _, r := range recs {
		env[r.Name] = r
	}
	return omnist.Schema{Root: root, Env: env, EnvOrder: order}
}

type badSchemaCase struct {
	name   string
	schema omnist.Schema
	code   omnist.Code
	path   string
}

func badSchemaCases() []badSchemaCase {
	one := omnist.DefaultCardinality()
	return []badSchemaCase{
		{"empty label", handSchema("R", []string{"R"}, rec("R", fld("", strT, one))), omnist.CodeSchemaEmptyLabel, "R"},
		{"open bracket in label", handSchema("R", []string{"R"}, rec("R", fld("a[1", strT, one))), omnist.CodeSchemaBracketInLabel, "R"},
		{"close bracket in label", handSchema("R", []string{"R"}, rec("R", fld("a]", strT, one))), omnist.CodeSchemaBracketInLabel, "R"},
		{"duplicate field", handSchema("R", []string{"R"}, rec("R", fld("a", strT, one), fld("a", strT, one))), omnist.CodeSchemaDuplicateField, "R"},
		{"duplicate record", handSchema("R", []string{"R", "R"}, rec("R", fld("a", strT, one))), omnist.CodeSchemaDuplicateRecord, "R"},
		{"min above max", handSchema("R", []string{"R"}, rec("R", fld("a", strT, omnist.Cardinality{Min: 3, Max: 2}))), omnist.CodeSchemaInvalidCardinality, "R.a"},
		{"dangling root", handSchema("Nope", []string{"R"}, rec("R", fld("a", strT, one))), omnist.CodeSchemaUnknownType, "$"},
		{"no root", handSchema("", []string{"R"}, rec("R", fld("a", strT, one))), omnist.CodeSchemaNoRoot, "$"},
		{"dangling ref", handSchema("R", []string{"R"}, rec("R", fld("a", omnist.RefType("Gone"), one))), omnist.CodeSchemaUnknownType, "R.a"},
		{"reserved scalar name", handSchema("string", []string{"string"}, rec("string")), omnist.CodeSchemaReservedName, "string"},
		{"reserved any name", handSchema("any", []string{"any"}, rec("any")), omnist.CodeSchemaReservedName, "any"},
		{"nullable ref", handSchema("R", []string{"R"}, rec("R", fld("a", omnist.Type{Kind: omnist.TypeRefKind, RefName: "R", Nullable: true}, one))), omnist.CodeSchemaNullableRef, "R.a"},
		{"invalid-utf8 label", handSchema("R", []string{"R"}, rec("R", fld("a\xffb", strT, one))), omnist.CodeSchemaInvalidLabel, "R"},
		{"invalid record name", handSchema("a b", []string{"a b"}, rec("a b")), omnist.CodeSchemaInvalidName, "$"},
		{"invalid ref target", handSchema("R", []string{"R"}, rec("R", fld("a", omnist.RefType("x y"), one))), omnist.CodeSchemaInvalidName, "$"},
		{"ordering names absent record", handSchema("R", []string{"R", "Ghost"}, rec("R", fld("a", strT, one))), omnist.CodeSchemaUnknownRecord, "$"},
		{"nil record entry", omnist.Schema{Root: "R", Env: map[string]*omnist.Record{"R": rec("R"), "N": nil}, EnvOrder: []string{"N", "R"}}, omnist.CodeSchemaUnknownRecord, "$"},
		{"nullable any", handSchema("R", []string{"R"}, rec("R", fld("a", omnist.Type{Kind: omnist.TypeAnyKind, Nullable: true}, one))), omnist.CodeSchemaNullableAny, "R.a"},
	}
}

func TestWriteRefusesIllFormedSchemas(t *testing.T) {
	for _, c := range badSchemaCases() {
		for _, compact := range []bool{false, true} {
			text, err := Write(c.schema, compact)
			if err == nil {
				_, rerr := Read(text)
				t.Errorf("%s (compact=%v): Write succeeded; re-reading its text gives: %v", c.name, compact, rerr)
				continue
			}
			var d omnist.Diagnostic
			if !errors.As(err, &d) {
				t.Errorf("%s: error %T is not an omnist.Diagnostic", c.name, err)
				continue
			}
			if d.Code != c.code || d.Path != c.path {
				t.Errorf("%s: got %s at %q, want %s at %q", c.name, d.Code, d.Path, c.code, c.path)
			}
			if text != "" {
				t.Errorf("%s: Write returned text alongside an error", c.name)
			}
		}
	}
}

// OSD-16 / S-24: a [0,0] field has no OSD spelling; Write fails with
// write.unsupported-value at the record path, in both layouts, and prune is
// the way out.
func TestWriteRefusesMaxZero(t *testing.T) {
	one := omnist.DefaultCardinality()
	zero := omnist.Cardinality{Min: 0, Max: 0}
	cases := []struct {
		name   string
		schema omnist.Schema
		path   string
	}{
		{"only field", handSchema("R", []string{"R"}, rec("R", fld("a", strT, zero))), "R"},
		{"after a good field", handSchema("R", []string{"R"}, rec("R", fld("g", strT, one), fld("a", strT, zero))), "R"},
		{"second record", handSchema("R", []string{"R", "S"}, rec("R", fld("s", omnist.RefType("S"), one)), rec("S", fld("dead", strT, zero))), "S"},
		{"first offender wins", handSchema("R", []string{"R", "S"}, rec("R", fld("x", strT, zero)), rec("S", fld("y", strT, zero))), "R"},
	}
	for _, c := range cases {
		for _, compact := range []bool{false, true} {
			text, err := Write(c.schema, compact)
			var d omnist.Diagnostic
			if !errors.As(err, &d) || d.Code != omnist.CodeWriteUnsupportedValue || d.Path != c.path || text != "" {
				t.Errorf("%s (compact=%v): got %q, %v; want write.unsupported-value at %q", c.name, compact, text, err, c.path)
			}
		}
	}
	// [0,unbounded] and [0,1] are fine, even with Max == 0 when Unbounded.
	ok := handSchema("R", []string{"R"}, rec("R", fld("a", strT, omnist.Cardinality{Min: 0, Max: 0, Unbounded: true}), fld("b", strT, omnist.Cardinality{Min: 0, Max: 1})))
	mustWrite(t, ok, false)
}

// prune removes max = 0 fields, so Write succeeds on the pruned schema.
func TestPruneThenWriteMaxZero(t *testing.T) {
	s := handSchema("R", []string{"R"}, rec("R", fld("dead", strT, omnist.Cardinality{Min: 0, Max: 0}), fld("a", strT, omnist.DefaultCardinality())))
	p := algebra.Prune(s)
	got := mustWrite(t, p, false)
	if strings.Contains(got, "dead") || strings.Contains(got, "[0,0]") || strings.Contains(got, "[0]") {
		t.Errorf("pruned output still carries the field: %q", got)
	}
}

// Write never panics on a nil Env entry any more: it returns the structured
// schema.unknown-record error (S-23).
func TestWriteDoesNotPanicOnNilEnvEntry(t *testing.T) {
	s := omnist.Schema{Root: "R", Env: map[string]*omnist.Record{"R": nil}, EnvOrder: []string{"R"}}
	_, err := Write(s, false)
	var d omnist.Diagnostic
	if !errors.As(err, &d) || d.Code != omnist.CodeSchemaUnknownRecord || d.Path != "$" {
		t.Fatalf("got %v", err)
	}
}
