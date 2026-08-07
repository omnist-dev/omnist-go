# omnist-go

A Go implementation of [Omnist](https://spec.omnist.dev), a data-interchange
format and schema model built around one idea: a document is an ordered list
of labeled edges, not a map. See
[`vendor/omnist-spec`](vendor/omnist-spec) for the full specification.

**Status: pre-alpha, under active development. Nothing is implemented yet.**
See [`docs/limitations.md`](docs/limitations.md) for current status and
[`docs/workflow-playbook.md`](docs/workflow-playbook.md) for how this port is
built.

This port is built spec-first: `omnist-spec` is the primary and only
day-to-day reference. The existing Python, TypeScript, and Rust
implementations are consulted only as narrow tie-breakers on spec gaps that
already have a filed issue against `omnist-spec` — never as a substitute for
what the spec says. Every place this port needed to guess is tracked as a
spec issue, not silently resolved by copying another implementation.

## Development

Requires Go 1.26+.

```bash
git submodule update --init --recursive
go build ./...
go test ./...
```
