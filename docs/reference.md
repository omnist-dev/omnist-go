# API reference

`omnist-go`'s canonical, mechanically-generated API reference is
[pkg.go.dev](https://pkg.go.dev/github.com/omnist-dev/omnist-go) — once the
module is published there, every exported type, function, and doc comment
in this repo renders automatically, always in sync with the code. That's
the idiomatic Go answer to "API reference": don't hand-maintain a second
copy of `go doc`'s output here.

**Tagged, but pkg.go.dev hasn't indexed it yet.** `v0.1.0-alpha` is tagged
and pushed, and the Go module proxy already knows about it —
`curl https://proxy.golang.org/github.com/omnist-dev/omnist-go/@v/v0.1.0-alpha.info`
returns a real result. pkg.go.dev itself, though, still 404s on
`https://pkg.go.dev/github.com/omnist-dev/omnist-go` as of this page
(indexing lags the proxy by anywhere from minutes to a few hours after
the first `go get` of a new module path). Until that page comes up, treat
this page's curated map below as authoritative, and use `go doc` locally
for the mechanical dump:

<!-- doc-illustrative -->
```bash
go doc github.com/omnist-dev/omnist-go
go doc github.com/omnist-dev/omnist-go.Validate
```

## What this page is

A curated map of the public API surface, organized by package, with a
one-line description and doc-comment excerpt per major exported group —
not a full mechanical dump. Read the linked source doc comments (or run
`go doc`) for the complete, authoritative signatures.

## Root package — `github.com/omnist-dev/omnist-go`

The Document model, Schema model, and the two document/schema operations
that don't belong to a specific codec or the algebra package.

### Document model (`document.go`)

- **`Document`** — a node or a bare value (spec §2.2:
  `Document = node | value`). `IsNode` selects which.
- **`Node`** — an ordered list of labeled `Edge`s (spec §2.1/§2.2). Labels
  may repeat; a repeated label is `omnist-go`'s only representation of "an
  array" — there is no separate list type.
- **`Edge`** — a single `(Label, Target)` pair.
- **`Target`** — what an edge points to: exactly a `Value` or a `*Node`
  (spec D-4). Constructed via `ValueTarget` / `NodeTarget`, read via
  `IsNode`/`Node()`/`Value()`.
- **`Value`** / **`Scalar`** — `Value` is a `Scalar` or null (`IsNull`).
  `Scalar` is a tagged union over the seven kinds spec §2.2.1 defines
  (`ScalarKind`: string, integer, number, boolean, date, time, datetime) —
  implementations must not add or collapse kinds. `integer` uses
  `*math/big.Int` (not `int64`) to support the spec's 4,300-decimal-digit
  limit. Construct with `NewStringScalar`, `NewIntegerScalar`,
  `NewNumberScalar`, `NewBooleanScalar`, `NewDateScalar`, `NewTimeScalar`,
  `NewDateTimeScalar`; compare with `Scalar.Equal`.

  `NewIntegerScalar` takes a `*big.Int`, not a plain `int` — the one
  constructor here that needs a concrete example, since `big.Int` isn't
  the obvious first reach for a small literal:

  <!-- verified-by: doc_examples_reference_test.go::Example_newIntegerScalar -->
  ```go
  s := omnist.NewIntegerScalar(big.NewInt(42))
  fmt.Println(s.Kind, s.Int)
  // integer 42
  ```

### Schema model (`schema.go`)

- **`Schema`** — an environment of named `Record`s plus a `Root` type
  reference.
- **`Record`** / **`Field`** — a record's ordered field list; each `Field`
  carries a `Type` and a `Cardinality`.
- **`Type`** — a scalar type, a `Record` reference (`RefType`), or `any`
  (`AnyType`). `ScalarType(kind, nullable)` builds a scalar field type.
- **`Cardinality`** — `[min, max]` occurrence bounds on a field;
  `DefaultCardinality()` is `[1,1]` (exactly one, per spec's default).

### Operations

- **`Validate(doc Document, s Schema) []Diagnostic`** (`validate.go`) —
  checks shape and cardinality against the schema. Never converts a
  value's type; a JSON string in a `date`-typed field fails, because
  validation checks what's already there.

  A schema-typed `age: integer` field rejects a JSON string outright,
  producing a real, populated `Diagnostic`:

  <!-- verified-by: doc_examples_reference_test.go::Example_validate -->
  ```go
  schema, _ := osd.Read(`
      record Person { "name": string, "age": integer }
      root Person
  `)
  doc, _ := json.Read(`{"name": "Ann", "age": "42"}`, omnist.DefaultLimits())

  diagnostics := omnist.Validate(doc, schema)
  for _, d := range diagnostics {
      fmt.Println(d.Path, d.Code, d.Severity)
  }
  // $.age validate.type-mismatch error
  ```

- **`Materialize(doc Document, s Schema) (Document, []Diagnostic, error)`**
  (`materialize.go`) — walks the document against the schema in one pass,
  upgrading leaf scalars to their schema-declared kind only when the
  conversion is value-exact (e.g. `"2024-01-01"` -> `date`, but never
  string -> number).

  A JSON string that looks like a date becomes a real `date` scalar once
  materialized against a schema that says so:

  <!-- verified-by: doc_examples_reference_test.go::Example_materialize -->
  ```go
  schema, _ := osd.Read(`record Event { "when": date } root Event`)
  doc, _ := json.Read(`{"when": "2024-01-01"}`, omnist.DefaultLimits())

  result, diagnostics, err := omnist.Materialize(doc, schema)
  if err != nil {
      panic(err)
  }
  if len(diagnostics) != 0 {
      panic("unexpected diagnostics")
  }
  v, _ := result.Node.Edges[0].Target.Value()
  fmt.Println(v.Scalar.Kind, v.Scalar.Date)
  // date {2024 1 1}
  ```

- **`DocumentsEqual`, `SchemasEqual`** (`referee.go`) — order-sensitive
  document equality and two schema-equality modes (`exact`: record names
  must match; `isomorphic`: same structure up to renaming), used by this
  repo's own conformance harness and available for any caller comparing
  documents/schemas the same way.

  `DocumentsEqual` is order-sensitive — the same edges in a different
  order are not equal:

  <!-- verified-by: doc_examples_reference_test.go::Example_documentsEqual -->
  ```go
  a, _ := oml.Read("x: \"1\"\ny: \"2\"\n", omnist.DefaultLimits())
  b, _ := oml.Read("y: \"2\"\nx: \"1\"\n", omnist.DefaultLimits())

  fmt.Println(omnist.DocumentsEqual(a, a))
  fmt.Println(omnist.DocumentsEqual(a, b))
  // true
  // false
  ```

  `SchemasEqual`'s two modes differ on record naming — `ModeExact`
  requires matching names, `ModeIsomorphic` accepts the same structure
  under any consistent renaming:

  <!-- verified-by: doc_examples_reference_test.go::Example_schemasEqual -->
  ```go
  a, _ := osd.Read(`record Person { "id": string } root Person`)
  b, _ := osd.Read(`record User { "id": string } root User`)

  fmt.Println(omnist.SchemasEqual(a, b, omnist.ModeExact))
  fmt.Println(omnist.SchemasEqual(a, b, omnist.ModeIsomorphic))
  // false
  // true
  ```

### Diagnostics and errors (`errors.go`)

- **`Diagnostic`** — `(Path, Code, Severity, Message)`, spec §8's
  `(path, code, message)` error taxonomy plus a severity. See
  `Validate`'s example above for one populated from a real failing call.
- **`ParseError`** — the structured error a stage-1 (text to `Document`)
  reader reports: `(Line, Col, Path, Code, Message)`. A text-position path
  (`"14:8"`), since no `Document` exists yet when a parse error fires.

  Triggering a real parse error shows the shape:

  <!-- verified-by: doc_examples_reference_test.go::Example_parseError -->
  ```go
  _, err := json.Read(`{"name": }`, omnist.DefaultLimits())
  perr := err.(*omnist.ParseError)
  fmt.Println(perr.Line, perr.Col, perr.Code)
  // 1 10 parse.unexpected-token
  ```

### Limits (`limits.go`)

- **`Limits`** / **`DefaultLimits()`** — the finite, documented safety
  bounds (depth, node count, integer digit count) spec §2.4 requires every
  implementation to enforce. Passed to every format reader.

## `oml` — `github.com/omnist-dev/omnist-go/oml`

OML (the native format) reader and writer: `Read(text string, limits
omnist.Limits) (omnist.Document, error)`, `Write(d omnist.Document, compact
bool) (string, []omnist.Diagnostic)` (`WriteCompact` is a convenience
wrapper for `Write(d, true)`). Round-trips Core and Extended OML per spec.

<!-- verified-by: doc_examples_reference_test.go::Example_omlRoundTrip -->
```go
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
// name: "Ann"; tags: "a"; tags: "b"
```

## `osd` — `github.com/omnist-dev/omnist-go/osd`

OSD (the schema definition format) reader and writer: `Read(text string)
(omnist.Schema, error)`, `Write(s omnist.Schema, compact bool) string`.

<!-- verified-by: doc_examples_reference_test.go::Example_osdRoundTrip -->
```go
schema, err := osd.Read(`
    record Person { "name": string, "tags" [0,]: string }
    root Person
`)
if err != nil {
    panic(err)
}

fmt.Println(osd.Write(schema, true))
// record Person { "name": string, "tags" [0,]: string } root Person
```

## `formats/{json,yaml,toml,xml}`

One reader/writer pair per format, all with the same reader shape —
`Read(text string, limits omnist.Limits) (omnist.Document, error)` — and a
writer shape shared by `xml`/`toml`/`yaml`/`json`:
`Write(d omnist.Document) (string, []omnist.Diagnostic, error)`. Writers
are schema-free by design — they serialize whatever `Document` they're
given, faithfully, and report non-fatal adjustments (a dropped null, a
stringified temporal value, a substituted `NaN`) as diagnostics rather than
errors. See each package's doc comment for format-specific caveats (e.g.
XML leaf-typing, YAML sexagesimal integers, TOML's native date/time
kinds).

`json`, round-tripping the shared reader/writer shape:

<!-- verified-by: doc_examples_reference_test.go::Example_jsonRoundTrip -->
```go
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
// {"name": "Ann"}
```

`yaml`'s sexagesimal-integer sharp edge — YAML 1.1 resolves a bare
`1:30:00` to the base-60 integer `5400`, not a time value, even though it
looks like one:

<!-- verified-by: doc_examples_reference_test.go::Example_yamlSexagesimal -->
```go
doc, err := yaml.Read("n: 1:30:00\n", omnist.DefaultLimits())
if err != nil {
    panic(err)
}

v, _ := doc.Node.Edges[0].Target.Value()
fmt.Println(v.Scalar.Kind, v.Scalar.Int)
// integer 5400
```

`xml`'s leaf-typing caveat — XML carries no type information at all, so
every leaf arrives as a string scalar, never resolved to an integer or
other kind the way JSON/YAML/TOML would:

<!-- verified-by: doc_examples_reference_test.go::Example_xmlLeafTyping -->
```go
doc, err := xml.Read(`<root><age>42</age></root>`, omnist.DefaultLimits())
if err != nil {
    panic(err)
}

ageNode, _ := doc.Node.Edges[0].Target.Node()
v, _ := ageNode.Edges[0].Target.Value()
fmt.Println(v.Scalar.Kind, v.Scalar.Str)
// string 42
```

A codec beyond JSON/YAML, showing the shared writer shape:

<!-- verified-by: doc_examples_reference_test.go::Example_tomlWrite -->
```go
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
// "name" = "Ann"
```

## `algebra` — `github.com/omnist-dev/omnist-go/algebra`

The Schema Algebra operations (spec §6): `CompatibleWith`, `Equivalent`,
`Normalize`, `Extract`, `Prune`, `Lint`, `Infer`. Each operates purely on
`omnist.Schema` values — no document, no I/O. See the package doc comment
for the operation-by-operation contract (each mirrors its spec §6
subsection).

`CompatibleWith` — §6.6's own worked example: `A` has an optional `nick`
field `B` doesn't, so `A` may emit something `B`'s closed shape rejects,
but everything `B` emits, `A` accepts:

<!-- verified-by: doc_examples_reference_test.go::Example_algebraCompatibleWith -->
```go
a, _ := osd.Read(`record User {
    "id": string,
    "name": string,
    "nick" [0,1]: string,
} root User`)
b, _ := osd.Read(`record User {
    "id": string,
    "name": string,
} root User`)

fmt.Println(algebra.CompatibleWith(a, b))
fmt.Println(algebra.CompatibleWith(b, a))
// false
// true
```

`Normalize` — collapses structurally-identical records (same fields,
different names) into shared equivalence classes:

<!-- verified-by: doc_examples_reference_test.go::Example_algebraNormalize -->
```go
s, _ := osd.Read(`
    record A { "id": string }
    record B { "id": string }
    record Root { "a": A, "b": B }
    root Root
`)

classes := algebra.EquivalenceClasses(algebra.Normalize(s))
fmt.Println(len(classes))
// 2
```

`Extract` — trims a schema down to only what's needed to keep a chosen
field set, erroring if that would invalidate the root:

<!-- verified-by: doc_examples_reference_test.go::Example_algebraExtract -->
```go
s, _ := osd.Read(`
    record Root { "keep": string, "drop" [0,1]: string }
    root Root
`)

out, err := algebra.Extract(s, map[string]bool{"keep": true})
if err != nil {
    panic(err)
}
root := out.Env[out.Root]
fmt.Println(len(root.Fields))
// 1
```

`Lint` — reports schema diagnostics such as an unreachable record no
field ever references:

<!-- verified-by: doc_examples_reference_test.go::Example_algebraLint -->
```go
s, _ := osd.Read(`
    record Root { "id": string }
    record Orphan { "id": string, "note": string }
    root Root
`)

findings := algebra.Lint(s)
for _, f := range findings {
    fmt.Println(f.Code, f.Location)
}
// lint.unreachable-record Orphan
```

`Prune` — removes a never-emittable field (cardinality `[0,0]`) and, as a
consequence, the now-unreachable record it alone referenced:

<!-- verified-by: doc_examples_reference_test.go::Example_algebraPrune -->
```go
s, _ := osd.Read(`
    record Root { "id": string, "dead" [0,0]: Orphan }
    record Orphan { "note": string }
    root Root
`)

pruned := algebra.Prune(s)
root := pruned.Env[pruned.Root]
fmt.Println(len(root.Fields))
fmt.Println(len(pruned.Env))
// 1
// 1
```

`Equivalent` — strictly stronger than `CompatibleWith` in one direction:
two schemas differing only in record naming and declaration order are
equivalent even though they aren't structurally equal:

<!-- verified-by: doc_examples_reference_test.go::Example_algebraEquivalent -->
```go
a, _ := osd.Read(`record Person { "id": string, "name": string } root Person`)
b, _ := osd.Read(`record User { "id": string, "name": string } root User`)

fmt.Println(algebra.Equivalent(a, b))
// true
```

`Infer` — drafts a schema from sample documents; two samples that
disagree on whether `tags` is present produce an optional `[0,1]` field:

<!-- verified-by: doc_examples_reference_test.go::Example_algebraInfer -->
```go
s1, _ := json.Read(`{"name": "Ann", "tags": ["a"]}`, omnist.DefaultLimits())
s2, _ := json.Read(`{"name": "Bo"}`, omnist.DefaultLimits())

schema, err := algebra.Infer([]omnist.Document{s1, s2}, "", false)
if err != nil {
    panic(err)
}
fmt.Println(osd.Write(schema, true))
// record Root { "name": string, "tags" [0,1]: string } root Root
```

## Command-line interface

`cmd/omnist` is a thin binding over the library (direct calls, not a
subprocess wrapper internally) — see the [CLI reference](cli.md) for its
own documented contract, which is `omnist-go`'s own design, not
spec-mandated.
