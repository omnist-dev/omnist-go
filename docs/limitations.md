# Limitations and status

`omnist-go` is a from-scratch Go implementation of the
[Omnist](https://spec.omnist.dev) data-interchange spec, built without
reference to the Python, TypeScript, or Rust implementations except as a
narrow, after-the-fact tie-breaker on spec gaps that already have a filed
`omnist-spec` issue. See `docs/workflow-playbook.md` for the full policy.

## Status

**Pre-alpha.** No operations are implemented yet. This file will track real
status as it changes — do not infer completeness from the port-order list in
the workflow playbook; that's a plan, not a status report.

## Versioning

Stays on `v0.0.x-alpha` until: core document model + all four codecs
(JSON/YAML/TOML/XML) + OML + OSD + CLI are implemented, and this repo's own
conformance harness (`tools/conformance/`) passes with zero real fails
(skips permitted and cited). See `docs/workflow-playbook.md` §1.

## Spec version targeted

`omnist-spec` **v0.2.0-alpha**, pinned via the `vendor/omnist-spec` git
submodule. This repo does not track the spec's `main` branch — the pin is
bumped deliberately, in its own commit.

## Divergences from the spec

None yet — nothing is implemented. Once codecs and operations exist, any
documented divergence from `omnist-spec` chapter 9's forbidden-variation
rules will be listed here, mirroring the spec's own divergence-ledger
entries (D-1 through D-7 as of the pinned spec version).
