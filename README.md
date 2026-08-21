# omnist-go

A from-scratch, spec-first Go implementation of
[Omnist](https://spec.omnist.dev), a data-interchange format and schema
model built around one idea: a document is an ordered list of labeled
edges, not a map. See [`vendor/omnist-spec`](vendor/omnist-spec) for the
full specification.

**Status: `v0.1.0-alpha`.** Every core operation is implemented: the
Document and Schema models, OML and OSD (read and write), `validate`,
`materialize`, the full schema algebra, all four interchange codecs
(JSON/YAML/TOML/XML, read and write), and a CLI. Both tracks of this
repo's conformance harness pass with zero real fails against `omnist-spec`.

**Documentation lives at [go.omnist.dev](https://go.omnist.dev)** —
getting started, the CLI reference, an API reference, and current
status/limitations. This README is intentionally just an entry point; go
there for anything beyond install and a local build check.

This port is built spec-first: `omnist-spec` is the primary and only
day-to-day reference. The existing Python, TypeScript, and Rust
implementations are consulted only as narrow tie-breakers on spec gaps that
already have a filed issue against `omnist-spec` — never as a substitute for
what the spec says. Every place this port needed to guess is tracked as a
spec issue, not silently resolved by copying another implementation. See
[go.omnist.dev/workflow-playbook](https://go.omnist.dev/workflow-playbook/)
for the full policy.

## Install

```bash
go install github.com/omnist-dev/omnist-go/cmd/omnist@latest
```

Or add the library to a module: `go get github.com/omnist-dev/omnist-go`.

## Development

Requires Go 1.24+ (Go 1.26.6+ recommended for stdlib `encoding/xml` recursion security patch GO-2026-6088).

```bash
git submodule update --init --recursive
go build ./...
go test ./...
```
