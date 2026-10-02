package osd

// Issue #121: osd.Write must not emit text its own reader rejects. A Schema
// built by hand can violate spec §3.3's S-1..S-8 and the label rules of §5.4;
// Write validates first and fails with the structured schema.* diagnostic the
// reader would have raised, at the same path (spec §8.4.1, E-30).

import (
	"errors"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
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
