# omnist-go

A from-scratch, spec-first Go implementation of [Omnist](https://spec.omnist.dev) — a data-interchange format built around one idea: a document is an ordered list of labeled edges, not a map.

Most formats model an object as a map from key to value. That works fine until a format needs to express "many" without a wrapper, or interleave repeated elements with other data — the shape XML uses and JSON, YAML, and TOML can't natively carry. Omnist's Document model handles all of it the same way: a node is a list of `(label, value)` edges, in order, with no special case for arrays. Two `item` edges *are* the array. There is no separate list type to define.

`omnist-go` reads and writes all five formats — JSON, YAML, TOML, XML, and OML (the native format) — into that one model, validates and materializes documents against a schema (OSD), and implements the schema algebra: `compatible_with`, `equivalent`, `normalize`, `extract`, `prune`, `lint`, `infer`.

## Why "spec-first"

This port is built directly from [omnist-spec](https://spec.omnist.dev), with no reference to the existing Python, TypeScript, or Rust implementations except as a narrow, after-the-fact tie-breaker on gaps that already have a filed spec issue. That's deliberate: it's the actual test of whether the spec is complete enough to build a fourth implementation from cold. Every place this port needed to guess became a real spec issue, not a silently-copied assumption. The [workflow playbook](workflow-playbook.md) has the full policy.

## Install

<!-- doc-illustrative -->
```bash
go install github.com/omnist-dev/omnist-go/cmd/omnist@latest
```

Or add the library to a module:

<!-- doc-illustrative -->
```bash
go get github.com/omnist-dev/omnist-go
```

Full API reference is on [pkg.go.dev](https://pkg.go.dev/github.com/omnist-dev/omnist-go) — this site covers usage and the project's own process, not generated API docs.

## Where to go next

- [Getting started](getting-started.md) — a working example in under a minute
- [CLI reference](cli.md) — every `omnist` subcommand
- [Status & limitations](limitations.md) — what's implemented, what isn't, current spec version targeted
