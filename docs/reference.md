# API reference

`omnist-go`'s canonical, mechanically-generated API reference is
[pkg.go.dev](https://pkg.go.dev/github.com/omnist-dev/omnist-go) — once the
module is published there, every exported type, function, and doc comment
in this repo renders automatically, always in sync with the code. That's
the idiomatic Go answer to "API reference": don't hand-maintain a second
copy of `go doc`'s output here.

**Not yet indexed.** As of this page, `omnist-go` has no tagged release
(`git tag -l` is empty) — pkg.go.dev indexes tagged versions (and the
`main` branch's latest commit as a pseudo-version) once a module has been
`go get`-ed at least once from a public proxy. Until the first tag lands
(tracked by `docs/workflow-playbook.md` §1's `v0.0.x-alpha` -> real version
bump), treat this page's curated map below as authoritative, and use `go
doc` locally for the mechanical dump:

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
- **`Materialize(doc Document, s Schema) (Document, []Diagnostic, error)`**
  (`materialize.go`) — walks the document against the schema in one pass,
  upgrading leaf scalars to their schema-declared kind only when the
  conversion is value-exact (e.g. `"2024-01-01"` -> `date`, but never
  string -> number).
- **`DocumentsEqual`, `SchemasEqual`** (`referee.go`) — order-sensitive
  document equality and two schema-equality modes (`exact`: record names
  must match; `isomorphic`: same structure up to renaming), used by this
  repo's own conformance harness and available for any caller comparing
  documents/schemas the same way.

### Diagnostics and errors (`errors.go`)

- **`Diagnostic`** — `(Path, Code, Severity, Message)`, spec §8's
  `(path, code, message)` error taxonomy plus a severity.
- **`ParseError`** — the structured error a stage-1 (text to `Document`)
  reader reports: `(Line, Col, Path, Code, Message)`. A text-position path
  (`"14:8"`), since no `Document` exists yet when a parse error fires.

### Limits (`limits.go`)

- **`Limits`** / **`DefaultLimits()`** — the finite, documented safety
  bounds (depth, node count, integer digit count) spec §2.4 requires every
  implementation to enforce. Passed to every format reader.

## `oml` — `github.com/omnist-dev/omnist-go/oml`

OML (the native format) reader and writer: `Read(text string, limits
omnist.Limits) (omnist.Document, error)`, `Write(d omnist.Document, compact
bool) (string, []omnist.Diagnostic)` (`WriteCompact` is a convenience
wrapper for `Write(d, true)`). Round-trips Core and Extended OML per spec.

## `osd` — `github.com/omnist-dev/omnist-go/osd`

OSD (the schema definition format) reader and writer: `Read(text string)
(omnist.Schema, error)`, `Write(s omnist.Schema, compact bool) string`.

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

## `algebra` — `github.com/omnist-dev/omnist-go/algebra`

The Schema Algebra operations (spec §6): `CompatibleWith`, `Equivalent`,
`Normalize`, `Extract`, `Prune`, `Lint`, `Infer`. Each operates purely on
`omnist.Schema` values — no document, no I/O. See the package doc comment
for the operation-by-operation contract (each mirrors its spec §6
subsection).

## Command-line interface

`cmd/omnist` is a thin binding over the library (direct calls, not a
subprocess wrapper internally) — see the [CLI reference](cli.md) for its
own documented contract, which is `omnist-go`'s own design, not
spec-mandated.
