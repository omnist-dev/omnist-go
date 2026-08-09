# CLI reference

```bash
go install github.com/omnist-dev/omnist-go/cmd/omnist@latest
```

There's no spec-mandated CLI contract — `omnist-spec` documents Python's CLI shape as one worked example of a binding, not something every port has to replicate. This CLI's flag names and exit-code convention are `omnist-go`'s own deliberate design.

## Commands

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

Convert JSON to YAML:

```bash
echo '{"name": "Ann", "tags": ["a", "b"]}' | omnist parse --from json --to yaml -
```

Validate a document against a schema:

```bash
omnist validate --from json --schema person.osd person.json
```

Check whether a schema change is backward-compatible:

```bash
omnist schema compatible-with new.osd old.osd
```
