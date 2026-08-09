package omnist_test

// External "omnist_test" package: TestJSONCrossFormatStructuralEqualityWithOML
// only ever needed OML-text fixtures plus exported API (oml.Read, json.Read,
// DocumentsEqual), so it moved out of json_reader_test.go to avoid the
// import cycle package oml would otherwise create (see
// osd_external_test_helpers_test.go's comment for the full explanation;
// this is the OML-side instance of the same issue #41 precedent).
//
// TestMaterializeUpgradesJSONSourcedStringsToTemporalKinds joined this file
// in issue #45: it only needs JSON-text fixtures plus omnist's exported
// API (json.Read, Materialize, Schema/Record/Field, DefaultCardinality),
// so once json_reader.go moved to package json (which imports package
// omnist), an *internal* omnist test file could no longer reach it without
// creating the same import-cycle problem mustOML already documents --
// this is the JSON-side instance of that same issue #43 precedent,
// applied to materialize_test.go per issue #45's explicit instruction.

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
)

// --- cross-format structural equality (JSON vs OML) ---

func TestJSONCrossFormatStructuralEqualityWithOML(t *testing.T) {
	omlText := `id: "A1"; total: 29.97; count: 3; ok: true; nothing: null; address: { street: "1 Main"; city: "London" }; items: "W"; items: "G"`
	jsonText := `{"id":"A1","total":29.97,"count":3,"ok":true,"nothing":null,"address":{"street":"1 Main","city":"London"},"items":["W","G"]}`

	omlDoc := mustOML(t, omlText)
	jsonDoc, err := json.Read(jsonText, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("json.Read failed: %v", err)
	}
	if !omnist.DocumentsEqual(omlDoc, jsonDoc) {
		t.Fatalf("format-independence violated:\nOML:  %#v\nJSON: %#v", omlDoc, jsonDoc)
	}
}

// --- cross-codec integration: JSON never auto-resolves temporal strings ---

func TestMaterializeUpgradesJSONSourcedStringsToTemporalKinds(t *testing.T) {
	doc, err := json.Read(`{"d": "2024-01-01", "t": "12:30:00", "dt": "2024-01-01T12:30:00"}`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	r := &omnist.Record{
		Name: "R",
		Fields: []omnist.Field{
			{Label: "d", Type: omnist.ScalarType(omnist.KindDate, false), Cardinality: omnist.DefaultCardinality()},
			{Label: "t", Type: omnist.ScalarType(omnist.KindTime, false), Cardinality: omnist.DefaultCardinality()},
			{Label: "dt", Type: omnist.ScalarType(omnist.KindDateTime, false), Cardinality: omnist.DefaultCardinality()},
		},
	}
	s := omnist.Schema{Root: "R", Env: map[string]*omnist.Record{"R": r}}

	got, diags, merr := omnist.Materialize(doc, s)
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
			if v.Scalar.Kind != omnist.KindDate {
				t.Fatalf("d: want KindDate, got %v", v.Scalar.Kind)
			}
		case "t":
			if v.Scalar.Kind != omnist.KindTime {
				t.Fatalf("t: want KindTime, got %v", v.Scalar.Kind)
			}
		case "dt":
			if v.Scalar.Kind != omnist.KindDateTime {
				t.Fatalf("dt: want KindDateTime, got %v", v.Scalar.Kind)
			}
		}
	}
}

// --- cross-format structural equality (TOML vs JSON) ---
//
// TestTOMLCrossFormatStructuralEqualityWithJSON moved here from
// toml_reader_test.go (issue #45), for the same import-cycle reason as
// TestJSONCrossFormatStructuralEqualityWithOML above: it needs json.Read
// plus omnist's exported API, which an *internal* omnist test file
// (package omnist, where toml_reader_test.go still lives pending toml's
// own move) cannot reach without creating an import cycle. It uses
// omnist.DocumentsEqual here instead of the unexported docEqual helper
// toml_reader_test.go still has access to, since DocumentsEqual is the
// exported, NaN-aware equivalent (see referee.go).

func TestTOMLCrossFormatStructuralEqualityWithJSON(t *testing.T) {
	src := `[order]
id = "A1"
status = "shipped"
total = 29.97

[order.address]
street = "1 Main"
city = "London"

[[order.items]]
sku = "W"
qty = 3
price = 9.99

[[order.items]]
sku = "G"
qty = 1
price = 9.99
`
	td, err := toml.Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	jsonSrc := `{"order":{"id":"A1","status":"shipped","total":29.97,"address":{"street":"1 Main","city":"London"},"items":[{"sku":"W","qty":3,"price":9.99},{"sku":"G","qty":1,"price":9.99}]}}`
	jd, err := json.Read(jsonSrc, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !omnist.DocumentsEqual(td, jd) {
		t.Errorf("TOML and JSON documents differ:\nTOML: %+v\nJSON: %+v", td, jd)
	}
}

// --- cross-format structural equality (YAML vs JSON) ---
//
// TestYAMLCrossFormatStructuralEqualityWithJSON and
// TestYAMLWorkedExampleCrossFormatEqualityWithJSON moved here from
// yaml_reader_test.go (issue #45), for the same reason as the TOML test
// above.

func TestYAMLCrossFormatStructuralEqualityWithJSON(t *testing.T) {
	yd, err := yaml.Read("a: 1\nb: \"two\"\nc:\n  d: true\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	jd, err := json.Read(`{"a":1,"b":"two","c":{"d":true}}`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !omnist.DocumentsEqual(yd, jd) {
		t.Errorf("YAML and JSON documents differ:\nYAML: %+v\nJSON: %+v", yd, jd)
	}
}

func TestYAMLWorkedExampleCrossFormatEqualityWithJSON(t *testing.T) {
	src := `order:
  id: A1
  status: shipped
  total: 29.97
  address: {street: 1 Main, city: London}
  items:
    - {sku: W, qty: 3, price: 9.99}
    - {sku: G, qty: 1, price: 9.99}
`
	d, err := yaml.Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	// "Byte-for-byte identical to the Document JSON produces."
	jsrc := `{"order":{"id":"A1","status":"shipped","total":29.97,"address":{"street":"1 Main","city":"London"},"items":[{"sku":"W","qty":3,"price":9.99},{"sku":"G","qty":1,"price":9.99}]}}`
	jd, err := json.Read(jsrc, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !omnist.DocumentsEqual(d, jd) {
		t.Errorf("YAML and JSON documents differ:\nYAML: %+v\nJSON: %+v", d, jd)
	}
}
