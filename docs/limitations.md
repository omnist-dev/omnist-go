# Limitations and status

`omnist-go` is a from-scratch Go implementation of the
[Omnist](https://spec.omnist.dev) data-interchange spec, built without
reference to the Python, TypeScript, or Rust implementations except as a
narrow, after-the-fact tie-breaker on spec gaps that already have a filed
`omnist-spec` issue. See `docs/workflow-playbook.md` for the full policy.

## Status

**Alpha.** Every core operation is implemented: the Document and Schema
models, OML and OSD (read and write), `validate`, `materialize`, the full
schema algebra (`satisfiable_set`, `is_empty`, `prune`, `compatible_with`,
`equivalent`, `normalize`, `extract`, `lint`, `infer`), all four interchange
codecs (JSON/YAML/TOML/XML, read and write), a CLI, both tracks of the
conformance harness, and fuzz tests on every reader (`go test -fuzz`).

Track 2 ([`tools/conformance/`](https://github.com/omnist-dev/omnist-go/tree/main/tools/conformance),
JSON-vector, run against `omnist-spec`'s `test-suite/`) currently reports
**149 pass / 1 fail / 1 skip** of 151 vectors. Track 1 (fixture-based,
`conformance/fixtures/`) reports **18 pass / 1 fail / 0 skip** of 19
fixtures. Both remaining fails are confirmed not `omnist-go` bugs — the
Track 2 one is a genuine spec-vector defect at `test-suite/validate/
basic_validate.json`'s `integer-satisfies-number-typed-field` case (the
vector expects `validate` to accept an integer value against a
`number`-typed field, conflating `materialize`'s §7.2 upgrade rule with
`validate`'s stricter §3.6 kind-equality check — filed as
[`omnist-spec#41`](https://github.com/omnist-dev/omnist-spec/issues/41));
the Track 1 one is a fixture using an un-namespaced `lint.*` code
(`unreachable-record` instead of `lint.unreachable-record`) in
`conformance/fixtures/lint/edge-case-unreachable-record` — filed as
[`omnist-spec#42`](https://github.com/omnist-dev/omnist-spec/issues/42).
The one Track 2 skip needs a TOML strict-mode write parameter this repo
hasn't built yet. Neither track's remaining item is CI-gating yet, per
`docs/workflow-playbook.md` §4.

## Versioning

Stays on `v0.0.x-alpha` until both conformance tracks pass with zero real
fails (skips permitted and cited) — the 2 current fails are spec-side, not
`omnist-go`'s to fix, so the bump is gated on upstream vector corrections
landing and this repo's submodule pin catching up. See
`docs/workflow-playbook.md` §1.

## Spec version targeted

`omnist-spec` at commit `896e14e` (post-`v0.2.0-alpha`), pinned via the
`vendor/omnist-spec` git submodule. This repo does not track the spec's
`main` branch — the pin is bumped deliberately, in its own commit.

## Divergences from the spec

None known. `omnist-go` can represent all seven scalar kinds natively and
has a real temporal `Scalar` variant, so the D-6/D-7(1)-class divergences
that affect TypeScript and Rust (a collapsed integer/number type, a missing
temporal variant) don't apply here.
