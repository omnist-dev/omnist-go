package omnist_test

// This file backs the runnable Go code blocks in docs/getting-started.md
// with real godoc Example functions (issue #62). `go test` executes each
// Example and checks its "// Output:" comment verbatim, so these functions
// are not just "tests with a plausible name" -- they assert the doc's
// literal displayed output. Each Example's body is kept in sync,
// character-for-character where feasible, with the corresponding fenced
// code block in docs/getting-started.md; if you change one, change the
// other and re-run `go test .` to confirm the Output: comment still
// matches.
//
// Document has no String() method (spec's Document model is an edge list,
// not a map, and there's no single obviously-right stringification -- see
// docs/workflow-playbook.md Sec2.3). formatDocument below is a small,
// test-local helper that renders a flat Node's edges as
// `(label,"value"), ...`, purely so these examples have deterministic,
// human-readable Output: text. It is intentionally not part of the public
// API.

import (
	"fmt"
	"strconv"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/osd"
)

// formatDocument renders a flat (non-nested) Node document as a
// comma-separated list of (label,value) pairs, in edge order, matching the
// style used in docs/getting-started.md's prose. Only the scalar kinds
// exercised by these examples are handled.
func formatDocument(d omnist.Document) string {
	if !d.IsNode {
		return formatValue(d.Value)
	}
	parts := make([]string, 0, len(d.Node.Edges))
	for _, e := range d.Node.Edges {
		v, ok := e.Target.Value()
		if !ok {
			parts = append(parts, fmt.Sprintf("(%s,<node>)", e.Label))
			continue
		}
		parts = append(parts, fmt.Sprintf("(%s,%s)", e.Label, formatValue(v)))
	}
	out := "["
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out + "]"
}

func formatValue(v omnist.Value) string {
	if v.IsNull {
		return "null"
	}
	switch v.Scalar.Kind {
	case omnist.KindString:
		return strconv.Quote(v.Scalar.Str)
	case omnist.KindBoolean:
		return strconv.FormatBool(v.Scalar.Bool)
	default:
		return "<scalar>"
	}
}

// Example_readDocument backs the first code block in docs/getting-started.md
// ("Read a document, no schema"): reading JSON produces a flat edge list
// where a JSON array becomes repeated edges sharing one label, not one edge
// holding a list.
func Example_readDocument() {
	doc, err := json.Read(`{"name": "Ann", "tags": ["a", "b"]}`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}
	fmt.Println(formatDocument(doc))
	// Output: [(name,"Ann"), (tags,"a"), (tags,"b")]
}

// Example_validateDocument backs the second code block in
// docs/getting-started.md ("Validate against a schema").
func Example_validateDocument() {
	schema, err := osd.Read(`
		record Person { "name": string, "tags" [0,]: string }
		root Person
	`)
	if err != nil {
		panic(err)
	}

	doc, err := json.Read(`{"name": "Ann", "tags": ["a", "b"]}`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}

	diagnostics := omnist.Validate(doc, schema)
	if len(diagnostics) == 0 {
		fmt.Println("valid")
	}
	// Output: valid
}

// Example_convertFormats backs the fourth code block in
// docs/getting-started.md ("Convert between formats"): writers are
// schema-free and serialize whatever Document they're given.
func Example_convertFormats() {
	doc, _ := json.Read(`{"name": "Ann"}`, omnist.DefaultLimits())
	text, diagnostics, err := yaml.Write(doc)
	if err != nil {
		panic(err)
	}
	if len(diagnostics) != 0 {
		panic("unexpected diagnostics")
	}
	fmt.Print(text)
	// Output: "name": "Ann"
}
