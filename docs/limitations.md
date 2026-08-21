# Limitations and status

`omnist-go` is a from-scratch Go implementation of the
[Omnist](https://spec.omnist.dev) data-interchange spec, built without
reference to the Python, TypeScript, or Rust implementations except as a
narrow, after-the-fact tie-breaker on spec gaps that already have a filed
`omnist-spec` issue. See `docs/workflow-playbook.md` for the full policy.

## Status

**`v0.2.0-alpha`.** Every core operation is implemented: the Document and Schema
models, OML and OSD (read and write), `validate`, `materialize`, the full
schema algebra (`satisfiable_set`, `is_empty`, `prune`, `compatible_with`,
`equivalent`, `normalize`, `extract`, `lint`, `infer`), all four interchange
codecs (JSON/YAML/TOML/XML, read and write), a CLI, both tracks of the
conformance harness, and fuzz tests on every reader (`go test -fuzz`).

Track 2 ([`tools/conformance/`](https://github.com/omnist-dev/omnist-go/tree/main/tools/conformance),
JSON-vector, run against `omnist-spec`'s `test-suite/`) currently reports
**151 pass / 0 fail / 1 skip** of 152 vectors. Track 1 (fixture-based,
`conformance/fixtures/`) reports **19 pass / 0 fail / 0 skip** of 19
fixtures. Both tracks are at zero real fails — the two prior fails, filed
as [`omnist-spec#41`](https://github.com/omnist-dev/omnist-spec/issues/41)
and [`omnist-spec#42`](https://github.com/omnist-dev/omnist-spec/issues/42),
were independently verified by the spec maintainer against the reference
implementation and found to be backwards diagnoses (issue #60): #41 was a
genuine `omnist-go` bug, not a spec-vector defect — `validate.go`'s
`conformScalar` did a bare kind-equality check with no `integer <:
number` value-level exception, which omnist-spec's now-formalized
`matches_kind` pseudocode (§3.6.1) confirms live against the Python
reference implementation is wrong; `validate` on an integer value against
a `number`-typed field must succeed. That's fixed. #42 needed no
production code change — the `lint/edge-case-unreachable-record` fixture
was always correct (the reference implementation genuinely emits the bare
`unreachable-record`, not a namespaced form); what was missing was
extending §8.5.2 rule 4's code-agnostic comparison (already used for
Track 2's diagnostics) to Track 1's finding `code` field, done in
`tools/conformance/fixtures.go`. The one Track 2 skip needs a TOML
strict-mode write parameter this repo hasn't built yet. Both conformance
tracks are strictly CI-gating as of issue #74.

### Codex audit cycle (#70–#81)

A 12-issue Codex audit cycle (#70–#81) resolved across 4 phases addressed all outstanding audit findings: a precision correctness fix for integer-to-number materialization (#70), a patch for CVE GO-2026-6088 via a Go toolchain pin (1.26.6) and scheduled CI `vulncheck` job (#73), strict CI gating for both conformance tracks (#74), two quadratic CPU-exhaustion DoS fixes across validation/materialization/subtyping path indexing (#71, #80) and OML/OSD zero-copy lexer scanning (#72), schema-aware XML pretyping per `omnist-spec#44` (#81), and design/hardening improvements including `Limits.Validate()` (#78), explicit acyclic validity contracts (#77), and CLI input size caps (#76).

## Versioning

**`v0.2.0-alpha`**, a minor bump per `docs/workflow-playbook.md` §1's
alpha-series rule: this release adds new public API (`xml.ReadWithSchema`,
`Limits.Validate()`), patches a real security vulnerability
(GO-2026-6088), and closes two real CPU-exhaustion DoS bugs — the Codex
audit cycle (#70-81, see above). Minor, not patch, since new public API
and a security fix cross the stable-surface threshold.

`v0.1.0-alpha` was the maintainer sign-off bump past `0.0.x` described in
`docs/workflow-playbook.md` §1: core document model, all four codecs, OML,
OSD, and the CLI are implemented, both conformance tracks pass with zero
real fails, the doc-example verification gate is CI-blocking (issue #62),
and a source-audited self-check of the §2.4 resource caps (depth/node-count/
integer-digit limits) against `omnist-spec`'s divergence ledger found no
gap — see the ledger's Go `Resource caps` row (source-audited clean,
`omnist-spec` commit `2af12e0`).

## Spec version targeted

`omnist-spec` at commit `f6ec180` (`v0.2.2-alpha`), pinned via the
`vendor/omnist-spec` git submodule. This repo does not track the spec's
`main` branch — the pin is bumped deliberately, in its own commit.

See `omnist-spec`'s own [§9.3 status table](https://github.com/omnist-dev/omnist-spec/blob/main/docs/09-divergence-ledger.md#93-status-table)
for the cross-implementation divergence ledger this repo reports into.

## Divergences from the spec

None known. `omnist-go` can represent all seven scalar kinds natively and
has a real temporal `Scalar` variant, so the D-6/D-7(1)-class divergences
that affect TypeScript and Rust (a collapsed integer/number type, a missing
temporal variant) don't apply here.
