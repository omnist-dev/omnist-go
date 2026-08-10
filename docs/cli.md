# CLI reference

<!-- doc-illustrative -->
```bash
go install github.com/omnist-dev/omnist-go/cmd/omnist@latest
```

There's no spec-mandated CLI contract — `omnist-spec` documents Python's CLI shape as one worked example of a binding, not something every port has to replicate. This CLI's flag names and exit-code convention are `omnist-go`'s own deliberate design.

## Commands

<!-- doc-illustrative -->
```
omnist parse --from FORMAT [--to FORMAT] [-o FILE] INPUT
omnist validate --from FORMAT --schema SCHEMA INPUT
omnist materialize --from FORMAT --schema SCHEMA [--to FORMAT] [-o FILE] INPUT
omnist schema normalize [-o FILE] SCHEMA
omnist schema prune [-o FILE] SCHEMA
omnist schema extract --keep label1,label2,... [-o FILE] SCHEMA
omnist schema compatible-with A B
omnist schema equivalent A B
omnist schema is-empty SCHEMA
omnist infer --from FORMAT [--allow-any] [-o FILE] FILE [FILE...]
omnist lint SCHEMA
```

`INPUT`, `SCHEMA`, and `FILE` accept `-` for stdin (or, for `INPUT`, an omitted argument in some commands). Omitting `-o` writes to stdout. Formats: `json`, `yaml`, `toml`, `xml`, `oml`.

**Flags must come before the positional argument.** Go's `flag.FlagSet` stops parsing at the first non-flag argument, unlike getopt-style permutation — `omnist parse --from json -` works, `omnist parse - --from json` doesn't. Accepted as a reasonable tradeoff for staying dependency-free; run `omnist SUBCOMMAND -h` for a subcommand's own flags.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Usage or tool error — bad flags, file not found, bad format name |
| `2` | The operation ran and reported a problem — a parse error, validation/materialize diagnostics, an infer failure, lint findings |

The three boolean schema commands (`compatible-with`, `equivalent`, `is-empty`) always exit `0` and print `true`/`false` on stdout — a deliberate departure from the convention some other Omnist tooling uses of encoding the boolean in the exit code.

## Examples

Every example below shows the exact command and its exact captured
output, backed by a real subprocess test that compiles and runs the
actual `omnist` binary — see `cmd/omnist/doc_examples_cli_test.go`'s doc
comment for why this differs mechanically from `reference.md`'s godoc
`Example` functions (a CLI transcript is a stdout/stderr/exit-code
triple, not a single `fmt.Println` value).

Convert JSON to YAML:

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleParseJSONToYAML -->
```console
$ echo '{"name": "Ann", "tags": ["a", "b"]}' | omnist parse --from json --to yaml -
"name": "Ann"
"tags":
    - "a"
    - "b"
```

Validate a document against a schema (exit code `2`: validation ran and
found a problem — `age` is a JSON string but the schema declares
`integer`, and `Validate` never converts a value's type):

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleValidate -->
```console
$ omnist validate --from json --schema person.osd person.json
$.age: validate.type-mismatch: value does not match declared kind
```

Check whether a schema change is backward-compatible (`new.osd` adds an
optional `nick` field `old.osd` doesn't have, so `new` may emit something
`old`'s closed shape rejects):

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleSchemaCompatibleWith -->
```console
$ omnist schema compatible-with new.osd old.osd
false
```

Materialize a document against a schema, upgrading a JSON string that
looks like a date to TOML's native date kind:

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleMaterialize -->
```console
$ omnist materialize --from json --schema event.osd --to toml event.json
"when" = 2024-01-01
```

Normalize a schema, collapsing two structurally-identical records (`A`,
`B`) into one shared shape:

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleSchemaNormalize -->
```console
$ omnist schema normalize dup.osd
record A {
    "id": string,
}
record Root {
    "a": A,
    "b": A,
}
root Root
```

Prune a schema, removing a never-emittable field (`[0,0]`) and, as a
consequence, the record it alone referenced:

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleSchemaPrune -->
```console
$ omnist schema prune dead.osd
record Root {
    "id": string,
}
root Root
```

Extract a schema down to only the fields named by `--keep`:

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleSchemaExtract -->
```console
$ omnist schema extract --keep keep extract.osd
record Root {
    "keep": string,
}
root Root
```

Check whether two schemas accept exactly the same documents, even though
they name their records differently:

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleSchemaEquivalent -->
```console
$ omnist schema equivalent person.osd user.osd
true
```

Check whether a schema is unsatisfiable — here, a record whose only field
requires itself at the OSD-default cardinality `[1,1]` can never be
satisfied by any finite document:

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleSchemaIsEmpty -->
```console
$ omnist schema is-empty empty.osd
true
```

Infer a schema from sample documents — two samples disagreeing on
whether `tags` is present produce an optional `[0,1]` field:

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleInfer -->
```console
$ omnist infer --from json s1.json s2.json
record Root {
    "name": string,
    "tags" [0,1]: string,
}
root Root
```

`--allow-any`'s effect: without it, a label whose samples disagree on
scalar kind is a hard failure (exit `2`):

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleInferAllowAny -->
```console
$ omnist infer --from json a1.json a2.json
omnist infer: Root.val: algebra.infer-conflicting-scalars: label "val" has values of more than one scalar kind
```

With it, the field opens to `any` instead, and the CLI reports what it
opened on stderr (exit `0`):

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleInferAllowAny -->
```console
$ omnist infer --from json --allow-any a1.json a2.json
record Root {
    "val": any,
}
root Root
$ # stderr: Root.val: opened to any: values of more than one scalar kind (integer, string)
```

Lint a schema — an unreachable record is a warning-level finding, which
also makes `lint`'s exit code `2` (an operation-reported problem):

<!-- verified-by: cmd/omnist/doc_examples_cli_test.go::TestCLIExampleLint -->
```console
$ omnist lint orphan.osd
Orphan: warning: lint.unreachable-record: record "Orphan" is defined but not reachable from root by any reference
```
