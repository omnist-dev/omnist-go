package omnist_test

// This file backs the runnable Go code blocks added to docs/reference.md
// by issue #64: real godoc Example functions covering package areas
// getting-started.md doesn't touch -- oml, osd, a non-JSON/YAML codec
// (toml), and the algebra package's schema operations. Same convention as
// doc_examples_test.go: `go test` executes each Example and checks its
// "// Output:" comment verbatim, so keep each Example's body in sync,
// character-for-character where feasible, with its corresponding fenced
// block in docs/reference.md.

import (
	"fmt"
	"math/big"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
	"github.com/omnist-dev/omnist-go/osd"
)

// Example_omlRoundTrip backs reference.md's `oml` section: reading OML
// text to a Document, then writing it back out compact.
func Example_omlRoundTrip() {
	doc, err := oml.Read(`name: "Ann"
tags: "a"
tags: "b"
`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	text, diagnostics := oml.WriteCompact(doc)
	if len(diagnostics) != 0 {
		panic("unexpected diagnostics")
	}
	fmt.Println(text)
	// Output: name: "Ann"; tags: "a"; tags: "b"
}

// Example_osdRoundTrip backs reference.md's `osd` section: parsing an OSD
// schema definition, then writing it back out.
func Example_osdRoundTrip() {
	schema, err := osd.Read(`
		record Person { "name": string, "tags" [0,]: string }
		root Person
	`)
	if err != nil {
		panic(err)
	}

	fmt.Println(osd.Write(schema, true))
	// Output: record Person { "name": string, "tags" [0,]: string } root Person
}

// Example_tomlWrite backs reference.md's `formats/toml` section, covering
// a codec beyond JSON/YAML: writers are schema-free and serialize
// whatever Document they're given.
func Example_tomlWrite() {
	doc, err := oml.Read(`name: "Ann"`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	text, diagnostics, err := toml.Write(doc)
	if err != nil {
		panic(err)
	}
	if len(diagnostics) != 0 {
		panic("unexpected diagnostics")
	}
	fmt.Print(text)
	// Output: "name" = "Ann"
}

// Example_algebraCompatibleWith backs reference.md's `algebra` section:
// §6.6's own worked example. A has an optional "nick" field B doesn't;
// compatible_with(A, B) is false (A may emit nick, B is closed) while
// compatible_with(B, A) is true (everything B emits, A accepts).
func Example_algebraCompatibleWith() {
	a, err := osd.Read(`record User {
		"id": string,
		"name": string,
		"nick" [0,1]: string,
	} root User`)
	if err != nil {
		panic(err)
	}
	b, err := osd.Read(`record User {
		"id": string,
		"name": string,
	} root User`)
	if err != nil {
		panic(err)
	}

	fmt.Println(algebra.CompatibleWith(a, b))
	fmt.Println(algebra.CompatibleWith(b, a))
	// Output:
	// false
	// true
}

// Example_algebraNormalize backs reference.md's `algebra` section:
// Normalize collapses two structurally-identical records (same fields,
// different names) to a canonical shared form.
func Example_algebraNormalize() {
	s, err := osd.Read(`
		record A { "id": string }
		record B { "id": string }
		record Root { "a": A, "b": B }
		root Root
	`)
	if err != nil {
		panic(err)
	}

	classes := algebra.EquivalenceClasses(algebra.Normalize(s))
	fmt.Println(len(classes))
	// Output: 2
}

// Example_algebraExtract backs reference.md's `algebra` section: Extract
// trims a schema down to only the reachable records/fields needed to keep
// a chosen field set, erroring if the root itself is invalidated.
func Example_algebraExtract() {
	s, err := osd.Read(`
		record Root { "keep": string, "drop" [0,1]: string }
		root Root
	`)
	if err != nil {
		panic(err)
	}

	out, err := algebra.Extract(s, map[string]bool{"keep": true})
	if err != nil {
		panic(err)
	}
	root := out.Env[out.Root]
	fmt.Println(len(root.Fields))
	// Output: 1
}

// Example_algebraLint backs reference.md's `algebra` section: Lint
// reports schema diagnostics -- here, an unreachable record no field ever
// references.
func Example_algebraLint() {
	s, err := osd.Read(`
		record Root { "id": string }
		record Orphan { "id": string, "note": string }
		root Root
	`)
	if err != nil {
		panic(err)
	}

	findings := algebra.Lint(s)
	for _, f := range findings {
		fmt.Println(f.Code, f.Location)
	}
	// Output: lint.unreachable-record Orphan
}

// Example_algebraPrune backs reference.md's `algebra` section: Prune
// removes a never-emittable field (cardinality max 0) and, as a
// consequence, the now-unreachable record it alone referenced.
func Example_algebraPrune() {
	s, err := osd.Read(`
		record Root { "id": string, "dead" [0,0]: Orphan }
		record Orphan { "note": string }
		root Root
	`)
	if err != nil {
		panic(err)
	}

	pruned := algebra.Prune(s)
	root := pruned.Env[pruned.Root]
	fmt.Println(len(root.Fields))
	fmt.Println(len(pruned.Env))
	// Output:
	// 1
	// 1
}

// Example_algebraEquivalent backs reference.md's `algebra` section:
// Equivalent is strictly stronger than CompatibleWith in one direction --
// two schemas that only differ in record naming and declaration order are
// equivalent even though they are not structurally equal.
func Example_algebraEquivalent() {
	a, err := osd.Read(`
		record Person { "id": string, "name": string }
		root Person
	`)
	if err != nil {
		panic(err)
	}
	b, err := osd.Read(`
		record User { "id": string, "name": string }
		root User
	`)
	if err != nil {
		panic(err)
	}

	fmt.Println(algebra.Equivalent(a, b))
	// Output: true
}

// Example_algebraInfer backs reference.md's `algebra` section: Infer
// drafts a Schema from sample Documents -- here, two JSON samples that
// disagree on whether "tags" is present, producing an optional [0,1]
// field in the inferred schema.
func Example_algebraInfer() {
	s1, err := json.Read(`{"name": "Ann", "tags": ["a"]}`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}
	s2, err := json.Read(`{"name": "Bo"}`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	schema, err := algebra.Infer([]omnist.Document{s1, s2}, "", false)
	if err != nil {
		panic(err)
	}
	fmt.Println(osd.Write(schema, true))
	// Output: record Root { "name": string, "tags" [0,1]: string } root Root
}

// Example_validate backs reference.md's Operations section: Validate
// checks shape and cardinality without ever converting a value's type --
// here a schema-typed "age" field rejects a JSON string, producing a
// populated Diagnostic with a real Path/Code/Message.
func Example_validate() {
	schema, err := osd.Read(`
		record Person { "name": string, "age": integer }
		root Person
	`)
	if err != nil {
		panic(err)
	}
	doc, err := json.Read(`{"name": "Ann", "age": "42"}`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	diagnostics := omnist.Validate(doc, schema)
	for _, d := range diagnostics {
		fmt.Println(d.Path, d.Code, d.Severity)
	}
	// Output: $.age validate.type-mismatch error
}

// Example_materialize backs reference.md's Operations section:
// Materialize upgrades a leaf scalar to its schema-declared kind only
// when the conversion is value-exact -- a JSON string that looks like a
// date becomes a real `date` scalar.
func Example_materialize() {
	schema, err := osd.Read(`
		record Event { "when": date }
		root Event
	`)
	if err != nil {
		panic(err)
	}
	doc, err := json.Read(`{"when": "2024-01-01"}`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	result, diagnostics, err := omnist.Materialize(doc, schema)
	if err != nil {
		panic(err)
	}
	if len(diagnostics) != 0 {
		panic("unexpected diagnostics")
	}
	edge := result.Node.Edges[0]
	v, _ := edge.Target.Value()
	d := v.Scalar.Date
	fmt.Printf("%s %04d-%02d-%02d\n", v.Scalar.Kind, d.Year, d.Month, d.Day)
	// Output: date 2024-01-01
}

// Example_newIntegerScalar backs reference.md's Document model section:
// integer scalars use *big.Int, not int64, to support the spec's
// 4,300-decimal-digit limit -- constructing one from a small literal
// still goes through big.NewInt.
func Example_newIntegerScalar() {
	s := omnist.NewIntegerScalar(big.NewInt(42))
	fmt.Println(s.Kind, s.Int)
	// Output: integer 42
}

// Example_parseError backs reference.md's Diagnostics and errors section:
// a stage-1 reader's failure is a *ParseError with a real Line/Col text
// position, not yet a Document-relative Path (no Document exists yet).
func Example_parseError() {
	_, err := json.Read(`{"name": }`, omnist.DefaultLimits())
	perr, ok := err.(*omnist.ParseError)
	if !ok {
		panic("expected a *ParseError")
	}
	fmt.Println(perr.Line, perr.Col, perr.Code)
	// Output: 1 10 parse.unexpected-token
}

// Example_documentsEqual backs reference.md's Operations section:
// DocumentsEqual is order-sensitive -- two documents with the same edges
// in different order are not equal.
func Example_documentsEqual() {
	a, err := oml.Read(`x: "1"
y: "2"
`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}
	b, err := oml.Read(`y: "2"
x: "1"
`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	fmt.Println(omnist.DocumentsEqual(a, a))
	fmt.Println(omnist.DocumentsEqual(a, b))
	// Output:
	// true
	// false
}

// Example_schemasEqual backs reference.md's Operations section:
// SchemasEqual's two modes differ on record naming -- ModeExact requires
// matching record names, ModeIsomorphic accepts structurally identical
// schemas that merely name their records differently.
func Example_schemasEqual() {
	a, err := osd.Read(`record Person { "id": string } root Person`)
	if err != nil {
		panic(err)
	}
	b, err := osd.Read(`record User { "id": string } root User`)
	if err != nil {
		panic(err)
	}

	fmt.Println(omnist.SchemasEqual(a, b, omnist.ModeExact))
	fmt.Println(omnist.SchemasEqual(a, b, omnist.ModeIsomorphic))
	// Output:
	// false
	// true
}

// Example_jsonRoundTrip backs reference.md's `formats/json` section.
func Example_jsonRoundTrip() {
	doc, err := json.Read(`{"name": "Ann"}`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	text, diagnostics, err := json.Write(doc)
	if err != nil {
		panic(err)
	}
	if len(diagnostics) != 0 {
		panic("unexpected diagnostics")
	}
	fmt.Print(text)
	// Output: {"name": "Ann"}
}

// Example_yamlSexagesimal backs reference.md's `formats/yaml` section:
// YAML 1.1's sexagesimal-integer sharp edge -- a bare `1:30:00` resolves
// to the base-60 integer 5400, NOT a time value, even though it looks
// like one.
func Example_yamlSexagesimal() {
	doc, err := yaml.Read("n: 1:30:00\n", omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	v, _ := doc.Node.Edges[0].Target.Value()
	fmt.Println(v.Scalar.Kind, v.Scalar.Int)
	// Output: integer 5400
}

// Example_xmlLeafTyping backs reference.md's `formats/xml` section: XML
// carries no type information at all, so every leaf arrives as a string
// scalar -- unlike JSON/YAML/TOML, `42` inside an element is never
// resolved to an integer.
func Example_xmlLeafTyping() {
	doc, err := xml.Read(`<root><age>42</age></root>`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	ageNode, _ := doc.Node.Edges[0].Target.Node()
	v, _ := ageNode.Edges[0].Target.Value()
	fmt.Println(v.Scalar.Kind, v.Scalar.Str)
	// Output: string 42
}
