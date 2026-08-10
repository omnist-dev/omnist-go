# Getting started

## Read a document, no schema

Every reader takes format text and returns a `Document` — untyped at this stage, since no schema is involved yet.

```go
package main

import (
	"fmt"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
)

func main() {
	doc, err := json.Read(`{"name": "Ann", "tags": ["a", "b"]}`, omnist.DefaultLimits())
	if err != nil {
		panic(err)
	}
	fmt.Println(doc) // two edges, not one list: (name,"Ann"), (tags,"a"), (tags,"b")
}
```
<!-- verified-by: doc_examples_test.go::Example_readDocument -->

`tags` reads to two edges sharing one label, not one edge holding a two-element list. That's the whole model: an array is a repeated label, handled the same way whether it came from JSON's `[...]`, OML's repeated `tags:` lines, or XML's repeated `<tag>` elements.

## Validate against a schema

```go
package main

import (
	"fmt"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/osd"
)

func main() {
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
}
```
<!-- verified-by: doc_examples_test.go::Example_validateDocument -->

`Validate` checks shape and cardinality against the schema — it never converts a value's type. A JSON `date` field that arrives as a plain string (JSON has no native temporal type) stays a string here; validating it against a `date`-typed field fails, and that's correct: validation checks what's already there.

## Materialize: read, then upgrade

To actually get a `date`-kind value out of a JSON string, use `Materialize` instead — it walks the document against the schema in one pass, upgrading leaves only when the conversion is value-exact (`"2024-01-01"` → `date`: yes; `"1"` → `integer`: no, a string is never coerced to a number).

<!-- doc-illustrative -->
```go
materialized, diagnostics, err := omnist.Materialize(doc, schema)
```

## Convert between formats

Writers are schema-free by design — they serialize whatever `Document` they're given, faithfully:

```go
package main

import (
	"fmt"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/yaml"
)

func main() {
	doc, _ := json.Read(`{"name": "Ann"}`, omnist.DefaultLimits())
	text, diagnostics, err := yaml.Write(doc)
	if err != nil {
		panic(err)
	}
	fmt.Print(text)
}
```
<!-- verified-by: doc_examples_test.go::Example_convertFormats -->

`diagnostics` reports non-fatal adjustments a write made — a dropped null (TOML has none), a stringified temporal value (JSON has no native date type), a substituted `NaN`. A write can succeed and still have something worth knowing about.

## From the command line

<!-- doc-illustrative -->
```bash
echo '{"name": "Ann", "tags": ["a", "b"]}' | omnist parse --from json --to yaml -
```

See the [CLI reference](cli.md) for the full command set.
