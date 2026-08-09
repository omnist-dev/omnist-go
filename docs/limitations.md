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
codecs (JSON/YAML/TOML/XML, read and write), and a CLI.

The conformance harness ([`tools/conformance/`](https://github.com/omnist-dev/omnist-go/tree/main/tools/conformance),
run for real against `omnist-spec`'s own `test-suite/`) currently reports
**147 pass / 3 fail / 1 skip** of 151 vectors. The 3 real fails are
confirmed spec-vector defects (two rely on OML indentation-based nesting the
grammar doesn't support; one conflates `materialize`'s upgrade rule with
`validate`'s stricter check) — filed upstream, not `omnist-go` bugs. The 1
skip needs a TOML strict-mode write parameter this repo hasn't built yet.

Track 1 (fixture-based) conformance, CI gating on the conformance job, and a
fuzz harness are not yet built.

## Versioning

Stays on `v0.0.x-alpha` until the conformance harness passes with zero real
fails (skips permitted and cited) — the 3 current fails are spec-side, not
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
