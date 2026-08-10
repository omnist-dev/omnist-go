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

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
	"github.com/omnist-dev/omnist-go/formats/toml"
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
