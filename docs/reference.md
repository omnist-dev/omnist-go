# API reference

`omnist-go`'s canonical, mechanically-generated API reference is
[pkg.go.dev](https://pkg.go.dev/github.com/omnist-dev/omnist-go) — once the
module is published there, every exported type, function, and doc comment
in this repo renders automatically, always in sync with the code. That's
the idiomatic Go answer to "API reference": don't hand-maintain a second
copy of `go doc`'s output here.

**Now indexed on pkg.go.dev.** As of `v0.2.0-alpha`, `https://pkg.go.dev/github.com/omnist-dev/omnist-go`
resolves and renders the full mechanical API dump — check that page first
for exact signatures. This page's curated map below stays useful as an
organized starting point, not a substitute. `go doc` still works locally
too:

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
- **`NewRecord(name, fields...) (*Record, error)`**,
  **`NewSchema(root, records...) (Schema, error)`** and
  **`Schema.Validate() error`** (`schema_validate.go`) — enforce spec §3.3's
  well-formedness rules S-1 to S-8, S-22 and S-23 and §5.4's label rules (a
  label is not empty, has no `[` or `]`, and is valid UTF-8) on a schema
  however it was built. `NewRecord`
  checks what concerns one record (name S-8, reserved name S-3, labels,
  cardinality S-2, nullability S-7, duplicate labels S-5); `NewSchema` lays the records out in
  the given order as `EnvOrder`, then runs `Validate`, which adds S-1 (a root
  exists and names a record), S-4 (unique record names), S-6 (every
  reference resolves) and S-23 (every `EnvOrder` entry names a record in
  `Env`). A failure is an `omnist.Diagnostic` (used as the error)
  with the `schema.*` code and the Schema path the spec fixes for it (§8.4.1,
  E-30), the same error `osd.Read` reports for that violation in text; only the
  first violation is returned. `Schema`, `Record` and `Field` stay plain public
  structs, so **a struct literal is unchecked** until `Validate` is called or
  the schema reaches an operation that validates. `osd.Write` does; the
  validate, materialize and algebra operations do not, and assume a well-formed
  schema (as every parsed or algebra-produced schema is). Three codes are new in v0.9.0-alpha
  (spec v0.28.0-beta, none pinned by a conformance vector, DIV-5):
  `schema.invalid-name` (S-8: a record name or a reference target that does
  not match `[A-Za-z_][A-Za-z0-9_]*`; path `$`, the name only in the message),
  `schema.invalid-label` (S-22: a label that is not valid UTF-8; the record
  path, never the label) and `schema.unknown-record` (S-23: an `EnvOrder`
  entry with no record in `Env`; path `$`). Cardinality `[0,0]` stays valid
  (S-15), but `osd.Write` refuses it (OSD-16, below). Message text puts any
  name or label through `%q`, so non-printable bytes are escaped.

  <!-- verified-by: doc_examples_reference_test.go::Example_schemaConstruction -->
  ```go
  rec, err := omnist.NewRecord("Person",
      omnist.Field{Label: "name", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
  )
  if err != nil {
      panic(err)
  }
  schema, err := omnist.NewSchema("Person", rec)
  if err != nil {
      panic(err)
  }
  text, err := osd.Write(schema, true)
  if err != nil {
      panic(err)
  }
  fmt.Println(text)

  _, err = omnist.NewRecord("Person",
      omnist.Field{Label: "", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
  )
  fmt.Println(err)

  unchecked := omnist.Schema{
      Root:     "Person",
      Env:      map[string]*omnist.Record{"Person": {Name: "Person", Fields: []omnist.Field{{Label: "a[1]", Type: omnist.AnyType(), Cardinality: omnist.DefaultCardinality()}}}},
      EnvOrder: []string{"Person"},
  }
  fmt.Println(unchecked.Validate())

  _, err = omnist.NewRecord("Person",
      omnist.Field{Label: "a\xffb", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
  )
  fmt.Println(err)

  // An ordering that names no record is reported, not panicked on.
  ordered := omnist.Schema{Root: "Person", Env: map[string]*omnist.Record{"Person": rec}, EnvOrder: []string{"Person", "Ghost"}}
  fmt.Println(ordered.Validate())

  // [0,0] is a legal model value that OSD cannot spell: Write refuses it.
  dead := omnist.Schema{
      Root:     "Person",
      Env:      map[string]*omnist.Record{"Person": {Name: "Person", Fields: []omnist.Field{{Label: "x", Type: omnist.AnyType(), Cardinality: omnist.Cardinality{Min: 0, Max: 0}}}}},
      EnvOrder: []string{"Person"},
  }
  fmt.Println(dead.Validate())
  _, err = osd.Write(dead, true)
  fmt.Println(err)
  // record Person { "name": string } root Person
  // Person: schema.empty-label: field label must not be empty
  // Person: schema.bracket-in-label: field label must not contain '[' or ']'
  // Person: schema.invalid-label: field label "a\xffb" is not valid UTF-8
  // $: schema.unknown-record: record ordering names "Ghost", which is not a record in the environment
  // <nil>
  // Person: write.unsupported-value: a field has cardinality [0,0], which has no OSD spelling; prune the schema before writing (spec §5.9, OSD-16)
  ```

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
  // 1 10 parse.codec-syntax
  ```

### Limits (`limits.go`)

- **`Limits`** / **`DefaultLimits()`** — the finite, documented safety
  bounds (depth, node count, integer digit count, and for YAML the alias
  expansion factor `MaxAliasExpansion`, default 50, and the expanded size
  `MaxExpandedSlots`, default 1,000,000) spec §2.4 requires every
  implementation to enforce. Passed to every format reader. A zero or
  negative `MaxAliasExpansion` selects the default, never "no limit"
  (`Limits.EffectiveMaxAliasExpansion()` gives the enforced value); an
  over-limit mapping or sequence (anchored or not, the document root and inline
  merge sources included), or an anchor that refers to itself, is rejected
  before any expansion with `document.limit.alias-expansion`. A mapping that
  merges a large anchor has `E` of about `(keys + 2) / 3`, so raise
  `MaxAliasExpansion` for one (see
  [Limitations](limitations.md#safety-limits-d-9-to-d-11-d-18-to-d-20)). A
  YAML input that contains an alias or a merge key is also rejected, with
  `document.limit.expanded-size` at `$`, when the slots it materializes exceed
  `MaxExpandedSlots` (the ratio and the size are independent limits; an input
  with neither an alias nor a merge key is exempt, so adding one alias to a huge
  plain file subjects it to the cap). A zero or negative `MaxExpandedSlots`
  selects the default (`Limits.EffectiveMaxExpandedSlots()`). A malformed merge
  value is `parse.codec-syntax`, reported before any limit.
- **`Limits.MaxInputBytes`** (default `DefaultMaxInputBytes`, 64 MiB) — the
  maximum input size in bytes (spec D-23). Every document reader
  (`oml`, `json`, `yaml`, `toml`, `xml`) refuses a larger input with
  `document.limit.input-size` at `$` before decoding or parsing, counts a
  leading byte-order mark, and accepts an input of exactly the maximum. A zero
  or negative value selects the default (`Limits.EffectiveMaxInputBytes()`),
  never "no limit". `CheckInputSize(text, limits)` is the check and
  `PrepareDocumentInput(text, bomCode, limits)` is the size check followed by
  `PrepareInput`. OSD reads a schema and is not covered. See
  [Limitations](limitations.md#safety-limits-d-9-to-d-11-d-18-to-d-20) for the
  default's measurement.
- **`Limits.Validate()`** — opt-in sanity check verifying that configured
  limits are strictly positive and within recommended safety ceilings
  (`MaxRecommendedDepth = 10_000`, `MaxRecommendedNodes = 100_000_000`,
  `MaxRecommendedIntDigits = 1_000_000`, `MaxRecommendedAliasExpansion =
  10_000`, `MaxRecommendedExpandedSlots = 10_000_000`,
  `MaxRecommendedInputBytes = 1 << 30`); an unset (zero) `MaxAliasExpansion`,
  `MaxExpandedSlots` or `MaxInputBytes` is accepted as "use the default", a
  negative one is an error.

<!-- verified-by: doc_examples_reference_test.go::Example_limitsValidate -->
```go
// DefaultLimits() satisfies all recommended bounds:
fmt.Println(omnist.DefaultLimits().Validate())

// Setting astronomically large limits triggers an error:
huge := omnist.Limits{
    MaxDepth:     200,
    MaxNodes:     200_000_000, // exceeds MaxRecommendedNodes (100,000,000)
    MaxIntDigits: 4300,
}
fmt.Println(huge.Validate())
// <nil>
// MaxNodes 200000000 exceeds recommended safety ceiling (100000000)
```

Raising or lowering the alias expansion factor, and what a rejection looks
like (the chain below has `E(c) = 21/5 = 4.20`):

<!-- verified-by: doc_examples_reference_test.go::Example_limitsAliasExpansion -->
```go
text := "a: &a leaf\nb: &b {p: *a, q: *a, r: *a, s: *a}\nc: &c {p: *b, q: *b, r: *b, s: *b}\n"

limits := omnist.DefaultLimits() // MaxAliasExpansion is 50
_, err := yaml.Read(text, limits)
fmt.Println(err)

limits.MaxAliasExpansion = 4
_, err = yaml.Read(text, limits)
pe := err.(*omnist.ParseError)
fmt.Println(pe.Code, pe.Path)
```

The expanded-size cap, on a document whose factor passes (`t` is 17 slots of
5 written, `E = 3.4`, and the whole input expands to 22 slots):

<!-- verified-by: doc_examples_reference_test.go::Example_limitsExpandedSlots -->
```go
text := "base: &base {k1: 1, k2: 2, k3: 3}\nt: {a: *base, b: *base, c: *base, d: *base}\n"

limits := omnist.DefaultLimits() // MaxExpandedSlots is 1,000,000
limits.MaxExpandedSlots = 21
_, err := yaml.Read(text, limits)
pe := err.(*omnist.ParseError)
fmt.Println(pe.Code, pe.Path)

limits.MaxExpandedSlots = 22
_, err = yaml.Read(text, limits)
fmt.Println(err)
// document.limit.expanded-size $
// <nil>
```

## `oml` — `github.com/omnist-dev/omnist-go/oml`

OML (the native format) reader and writer: `Read(text string, limits
omnist.Limits) (omnist.Document, error)`, `Write(d omnist.Document, compact
bool) (string, []omnist.Diagnostic, error)` (`WriteCompact` is a convenience
wrapper for `Write(d, true)`). The error is the C-9 failure only: a string value
or edge label that is not well-formed UTF-8 is refused with
`write.unsupported-value` (see Writers, below), never written as U+FFFD.
Round-trips Core and Extended OML per spec.

<!-- verified-by: doc_examples_reference_test.go::Example_omlRoundTrip -->
```go
doc, err := oml.Read(`name: "Ann"
tags: "a"
tags: "b"
`, omnist.DefaultLimits())
if err != nil {
    panic(err)
}

text, diagnostics, err := oml.WriteCompact(doc)
if err != nil || len(diagnostics) != 0 {
    panic("unexpected diagnostics")
}
fmt.Println(text)
// name: "Ann"; tags: "a"; tags: "b"
```

## `osd` — `github.com/omnist-dev/omnist-go/osd`

OSD (the schema definition format) reader and writer: `Read(text string)
(omnist.Schema, error)`, `Write(s omnist.Schema, compact bool) (string, error)`.

`Write` fails, unconditionally, when a field label contains a C0 control
character (U+0000 to U+001F, tab and newline included): OSD text has no
spelling for one (spec §5.9, OSD-14). The error is an `omnist.Diagnostic`
with code `write.unsupported-value` and, as its path, the *record* holding the
field (`R`, never `R.<label>`). A schema that only a programmatic build or
`algebra.Infer` over documents with such keys can produce; a parsed schema
never has one. Every other label is written with exactly two escapes, `\\`
and `\"` (OSD-15). Before checking labels, `Write` calls `Schema.Validate`: an
ill-formed schema (empty label, `[` or `]` in a label, duplicate field or
record, bad cardinality, dangling root or reference, and so on) fails with the
`schema.*` diagnostic `osd.Read` would raise for it, instead of text the reader
rejects (v0.8.0-alpha). Since v0.9.0-alpha that includes a label that is not
valid UTF-8 (`schema.invalid-label`, which `Write` used to copy through byte for
byte) and an `EnvOrder` entry naming no record (`schema.unknown-record`, which
used to panic). `Write` also fails on a field of cardinality `[0,0]`
(`Max == 0`, not `Unbounded`): the model allows it (S-15) but no OSD text
spells it, so it returns `write.unsupported-value` at the record path `R`, the
same mechanism as OSD-14 (OSD-16, S-24). `algebra.Prune` drops such fields, so
prune first. Before v0.5.0-alpha `Write` returned a bare `string` and
emitted text its own reader rejected for such a label.

<!-- verified-by: doc_examples_reference_test.go::Example_osdRoundTrip -->
```go
schema, err := osd.Read(`
    record Person { "name": string, "tags" [0,]: string }
    root Person
`)
if err != nil {
    panic(err)
}

text, err := osd.Write(schema, true)
if err != nil {
    panic(err)
}
fmt.Println(text)
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

Every writer, `oml.Write` included, first calls `omnist.CheckEncodable(d)` and
fails with `write.unsupported-value` (spec C-9) on a string value or an edge
label that is not well-formed UTF-8 (a Go string can hold any bytes), at the
Document path of the node holding the string (for a label, the node holding the
edge; indexed per E-10), never substituting U+FFFD or an escape. The XML writer
likewise refuses a null leaf (C-10), at its indexed path.

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
doc, _, err := xml.Read(`<root><age>42</age></root>`, omnist.DefaultLimits())
if err != nil {
    panic(err)
}

ageNode, _ := doc.Node.Edges[0].Target.Node()
v, _ := ageNode.Edges[0].Target.Value()
fmt.Println(v.Scalar.Kind, v.Scalar.Str)
// string 42
```

`xml` also provides **`ReadWithSchema(src string, schema *omnist.Schema, limits omnist.Limits)`**
to pre-type numeric, boolean, and temporal leaves during ingestion when an OSD schema is
available (per `omnist-spec#44`):

<!-- verified-by: doc_examples_reference_test.go::Example_xmlReadWithSchema -->
```go
schema, err := osd.Read(`
    record User { "name": string, "age": integer, "active": boolean }
    root User
`)
if err != nil {
    panic(err)
}

src := `<User><name>Ann</name><age>42</age><active>true</active></User>`
doc, _, err := xml.ReadWithSchema(src, &schema, omnist.DefaultLimits())
if err != nil {
    panic(err)
}

rootNode, _ := doc.Node.Edges[0].Target.Node()
for _, edge := range rootNode.Edges {
    v, _ := edge.Target.Value()
    fmt.Printf("%s: %s\n", edge.Label, v.Scalar.Kind)
}
// name: string
// age: integer
// active: boolean
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

`Prune` — removes a field that can never be emitted (optional, referencing
a record that is itself unsatisfiable) and, as a consequence, the
now-unreachable record it alone referenced:

<!-- verified-by: doc_examples_reference_test.go::Example_algebraPrune -->
```go
s, _ := osd.Read(`
    record Root { "id": string, "dead" [0,1]: Orphan }
    record Orphan { "self": Orphan }
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
text, err := osd.Write(schema, true)
if err != nil {
    panic(err)
}
fmt.Println(text)
// record Root { "name": string, "tags" [0,1]: string } root Root
```

## Command-line interface

`cmd/omnist` is a thin binding over the library (direct calls, not a
subprocess wrapper internally) — see the [CLI reference](cli.md) for its
own documented contract, which is `omnist-go`'s own design, not
spec-mandated.
