# Limitations and status

`omnist-go` is a from-scratch Go implementation of the
[Omnist](https://spec.omnist.dev) data-interchange spec, built without
reference to the Python, TypeScript, or Rust implementations except as a
narrow, after-the-fact tie-breaker on spec gaps that already have a filed
`omnist-spec` issue. See `CONTRIBUTING.md` for the full policy.

## Status

**`v0.5.0-alpha`.** Every core operation is implemented: the Document and Schema
models, OML and OSD (read and write), `validate`, `materialize`, the full
schema algebra (`satisfiable_set`, `is_empty`, `prune`, `compatible_with`,
`equivalent`, `normalize`, `extract`, `lint`, `infer`), all four interchange
codecs (JSON/YAML/TOML/XML, read and write), a CLI, both tracks of the
conformance harness, and fuzz tests on every reader (`go test -fuzz`).

Track 2 ([`tools/conformance/`](https://github.com/omnist-dev/omnist-go/tree/main/tools/conformance),
JSON-vector, run against `omnist-spec`'s `test-suite/`) currently reports
**239 pass / 0 fail / 34 skip** of 273 vectors (`omnist-spec` v0.21.0-beta
pin), compared as a set of `(path, code)` per §8.5.2 — not code-agnostically.
28 of the 34 skips are the OSD-OML extension
(`parse_schema_oml`/`write_schema_oml`) — not yet implemented in this
port, cited honestly per §9.5 rather than crashing or failing the
driver; see [issue #111](https://github.com/omnist-dev/omnist-go/issues/111)
for implementing it. The other 6 are the YAML alias-expansion vectors, which
carry `declared_max_alias_expansion`: this port does not enforce D-18 and has
no configuration surface for it, so the runner skips them citing `DIV-3` and
[issue #117](https://github.com/omnist-dev/omnist-go/issues/117) rather than
running them against the wrong limit. All 34 skips are E-20's "not yet
implemented" category; none is a documented divergence (E-21). Track 1
(fixture-based, `conformance/fixtures/`) reports **19 pass / 0 fail / 0
skip** of 19 fixtures. Both tracks are at zero real fails — the two prior fails, filed
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
`tools/conformance/fixtures.go`. Both conformance
tracks are strictly CI-gating as of issue #74.

### Codex audit cycle (#70–#81)

A 12-issue Codex audit cycle (#70–#81) resolved across 4 phases addressed all outstanding audit findings: a precision correctness fix for integer-to-number materialization (#70), a patch for CVE GO-2026-6088 via a Go toolchain pin (1.26.6) and scheduled CI `vulncheck` job (#73), strict CI gating for both conformance tracks (#74), two quadratic CPU-exhaustion DoS fixes across validation/materialization/subtyping path indexing (#71, #80) and OML/OSD zero-copy lexer scanning (#72), schema-aware XML pretyping per `omnist-spec#44` (#81), and design/hardening improvements including `Limits.Validate()` (#78), explicit acyclic validity contracts (#77), and CLI input size caps (#76).

## Versioning

**`v0.5.0-alpha`**, a minor bump per `CONTRIBUTING.md` §1's alpha-series
rule, and a **breaking** one: `osd.Write` now returns `(string, error)`
instead of a bare `string` (OSD-14, below), and it adopts `omnist-spec`
v0.21.0-beta (from v0.19.0-beta). Conformance against the new pin, Track 2:
**224 pass / 15 fail / 34 skip of 273** before any code change (the 24 new
vectors plus one behavior the port had had wrong); **239 pass / 0 fail / 34
skip of 273** after. Track 1 stayed 19/19. Compared as `(path, code)` sets,
not code-agnostically. What changed:

- **`osd.Write` returns an error (OSD-14, §5.9/§8.3.9). Breaking.** A field
  label containing a C0 control character (U+0000 to U+001F, tab and newline
  included) has no OSD spelling: §5.3.1 bans the raw byte in a string body,
  escape context included, and OSD's unescaping is weak. The writer used to
  emit a backslash plus the raw byte, which this port's own reader rejects.
  It now fails, unconditionally, with an `omnist.Diagnostic` (code
  `write.unsupported-value`, path the *record* holding the field, `R` and
  never `R.<label>`) and returns no text. The signature changed rather than a
  second checked function being added because a `Write` that kept returning
  text for a schema it must refuse would itself be the non-conformant entry
  point, and the compiler is the only thing that makes every caller face the
  new failure; the alpha series exists to make this change cheaply. Callers:
  `text, err := osd.Write(s, compact)`. `omnist infer` is the one CLI route
  that can reach the failure (a JSON key may carry such a character) and now
  exits 2 with `write.unsupported-value` instead of printing unreadable text.
  No conformance vector can pin OSD-14 (`DIV-5`: a vector gives a schema as
  OSD text, and such a schema has none), so it is held by unit tests only,
  including a randomized `Read(Write(s)) == s` property over arbitrary
  labels.
- **OSD-15, canonical escaping (§5.9).** Already what the writer did (a
  backslash as `\\`, a quote as `\"`, nothing else); the four
  `osd-grammar/canonical-output/label-*` vectors passed on arrival. What
  changed is that the escaper now works on bytes, so a programmatically built
  label holding invalid UTF-8 is written as given instead of being repaired to
  U+FFFD by a rune loop.
- **D-14, invalid UTF-8 (§2.5, E-27).** Every read surface (OML, OSD, JSON,
  YAML, TOML, XML, and `xml.ReadWithSchema`) now rejects a `string` for which
  `utf8.ValidString` is false with **`parse.invalid-encoding` at `1:1`**,
  before D-15's BOM strip and D-21's second-mark check, so a truncated BOM
  (`EF BB`) is a D-14 failure. JSON, OML and OSD used to accept invalid UTF-8
  silently, and YAML, TOML and XML reported `parse.codec-syntax` (#119). The
  check lives in one place, `omnist.PrepareInput` (`bom.go`), which every
  reader calls first and which delegates the BOM rules to
  `omnist.StripLeadingBOM`. New public API: `omnist.PrepareInput` and
  `omnist.CodeParseInvalidEncoding`. There are no `[]byte` reader variants;
  the CLI reads bytes, converts losslessly with `string(b)` and reaches the
  same check, so `omnist parse` on invalid input exits 2 with
  `parse.invalid-encoding` at `1:1` instead of repairing anything.
- **OML-26 / OML-27 (§4.6.1).** A missing separator between two top-level
  edges (`a: 1 b: 2`) is now `parse.trailing-content` at the first leftover
  token, like `a: 1 }`, `a: 1 ,` and `a: 2024-01-01T99`; it used to be
  `parse.unexpected-token` whenever the leftover token looked like the start
  of another edge. Inside `{...}` and `[...]` the same missing separator stays
  `parse.unexpected-token`. A separator *followed* by a stray token
  (`a: 1` newline `}`) is outside the rule's "no separator in front of it"
  wording, no vector pins it, and it is still `parse.unexpected-token`.
- **Conformance runner (E-27).** Read-side vectors (`parse`, `parse_schema`)
  now take their input as `text` or `bytes_hex`, exactly one. A `bytes_hex`
  input is decoded to its raw bytes and handed, unchanged, to the same
  string-taking reader (Go's `string` is a byte sequence, which is the rule §2.5
  states for Go); it is never decoded with replacement. A missing or doubled
  input field is a `fail`, not an empty input. All 14 `bytes_hex` vectors (8
  invalid, 6 valid multi-byte controls) run and pass. There is no
  runner-side list of known failures. The 34 skips are unchanged and all E-20
  "not yet implemented": 28 OSD-OML extension vectors (#111) and 6 YAML
  alias-expansion vectors (`DIV-3`, #117).
- **Not changed.** D-18/D-19/D-20 (alias expansion) are still not
  implemented (#117).

**`v0.4.0-alpha`**, a minor bump per `CONTRIBUTING.md` §1's alpha-series
rule: this release changes observable reader and writer behavior (new
diagnostic codes, inputs that used to be accepted now refused) and adopts
`omnist-spec` v0.19.0-beta (from v0.9.1-beta). Conformance against the new
pin, Track 2: **194 pass / 26 fail / 29 skip of 249** before any code change;
**215 pass / 0 fail / 34 skip of 249** after (Track 1 stayed 19/19).
Compared as `(path, code)` sets, not code-agnostically. What changed:

- **D-15 / D-21 (byte-order mark), every read surface.** One leading `U+FEFF`
  is stripped on OML, OSD, JSON, YAML, TOML and XML (OML, OSD, JSON, TOML and
  XML used to reject it), and a second is rejected at `1:1` —
  `parse.unexpected-token` on OML and OSD, `parse.codec-syntax` on the four
  codecs. YAML used to swallow the second mark silently, because `yaml.v3`
  discards one on its own; the check now runs before the library sees the
  text. Stripping lives in one place, `omnist.StripLeadingBOM` (`bom.go`).
  A mark anywhere else is ordinary content. No writer emits one, and a test
  fails if any file in the repository contains a raw `U+FEFF`.
- **`parse.codec-syntax`** (§8.3.1) is now the code for input a codec cannot
  accept: malformed JSON, YAML, TOML and XML used to report
  `parse.unexpected-token` (and XML/JSON content after the document,
  `parse.trailing-content`). Callers matching on the old codes must match on
  the new one.
- **The data-XML profile** (fixes #116). A `DOCTYPE` is refused on sight
  (`format.dtd-forbidden`), an entity reference other than the five predefined
  ones is refused (`format.entity-forbidden`), and mixed content — which used
  to be silently dropped — is refused (`format.mixed-content`), each at path
  `$`. The refusal runs after well-formedness: a malformed document is a
  `parse.codec-syntax` error even when it also contains one of the three.
  Writing a string with a C0 control character other than tab, LF and CR now
  fails with `write.unsupported-value` instead of emitting ill-formed XML.
- **YAML merge keys** (`<<: *a`, `<<: [*a, *b]`) were not implemented — `<<`
  was read as an ordinary label. They now flatten into the referring mapping in
  source order, merged entries first, with the collision, nested-merge and
  repeated-alias rules of `docs/formats/yaml.md`.
- **OML-25.** Any scalar followed by leftover content at document level is
  `parse.trailing-content` (`nan: 1`, `inf: 1`, `5: 1`), not
  `parse.unexpected-token`.
- **E-23.** String-body errors report the string's opening quote: a control
  character inside an OML multiline string, and inside any OSD string, used to
  report the character's own position. An OSD control character immediately
  after a backslash is now `parse.control-character`, as §5.3.1 requires.
- **Conformance runner.** `declared_max_alias_expansion` joins the limit-key
  allowlist (six vectors skipped, citing `DIV-3` and #117), and the
  TOML `strict` write vector now runs instead of being skipped with a reason
  that was not true (TOML's failures are unconditional, so there is no strict
  mode to lack).
- **Not changed.** D-18 (alias expansion limit) is not implemented — see #117.
  `infer` was checked against S-21 and already complies (`any` only on
  request, both openings reported, nested included); a regression test pins it.
  The OSD writer still emits a backslash before a control character in a label,
  which the reader now rejects; OSD has no spelling for such a label, which
  sits uneasily with OSD-11 and is raised as an open spec question in the PR.

**`v0.3.1-alpha`**, a patch bump per `CONTRIBUTING.md` §1's
alpha-series rule: no library API or observable behavior changed, just
conformance-harness/tooling and docs work — bumping the `omnist-spec`
pin to `v0.7.0-beta` (`c4141d0`), adding an honest, cited skip for the
24 new OSD-OML-extension vectors this port doesn't implement yet
(issue #111 tracks that as future work) rather than failing the
driver, and normalizing insignificant XML whitespace before comparing
`write` vectors (PR #110, per `omnist-spec#52`). Patch, not minor,
since nothing here touches the public contract.

**`v0.3.0-alpha`**, a minor bump per `CONTRIBUTING.md` §1's
alpha-series rule: this release adds new public API
(`ValidDate`/`ValidTime`/`ValidOffsetText` in the root package; six new
`schema.*`/`parse.*` diagnostic codes) and closes several real
correctness bugs where a value was silently corrupted or data was
silently lost, not just narrow bug fixes -- the #95-#103 spec-correctness
audit batch (`omnist-spec` v0.4.0-beta pin, commit `0ac1eac`):
forbidding redundant `[0,0]` cardinality (#95); rejecting an empty or
bracket-containing field label (#100, #103); making a null leaf, a
NaN/Infinity leaf, and an empty internal node fail their write outright
instead of substituting a lossy fallback that collided with a
genuinely different, independently-valid input (#96-#98); escaping an
XML carriage return as `&#13;` instead of writing it raw (#99); and
rejecting a leading-zero numeric literal and an out-of-range calendar
date or clock value (#101-#102) -- including a real bug where a
tz-offset like `+00:60` silently normalized to a valid-looking
`+01:00` instead of being rejected, the same "does the fallback
collide with a different valid input" defect shape as the write-side
fixes. Filed `omnist-spec#52` for a conformance-vector-authoring defect
found along the way (an XML write vector baking in pretty-printed
whitespace no other exact-text vector in its track requires). Minor,
not patch, since new public API and several corrected-not-just-narrowed
correctness bugs cross the stable-surface threshold.

**`v0.2.0-alpha`** was a minor bump per `CONTRIBUTING.md` §1's
alpha-series rule: that release added new public API (`xml.ReadWithSchema`,
`Limits.Validate()`), patched a real security vulnerability
(GO-2026-6088), and closed two real CPU-exhaustion DoS bugs — the Codex
audit cycle (#70-81, see above). Minor, not patch, since new public API
and a security fix cross the stable-surface threshold.

`v0.1.0-alpha` was the maintainer sign-off bump past `0.0.x` described in
`CONTRIBUTING.md` §1: core document model, all four codecs, OML,
OSD, and the CLI are implemented, both conformance tracks pass with zero
real fails, the doc-example verification gate is CI-blocking (issue #62),
and a source-audited self-check of the §2.4 resource caps (depth/node-count/
integer-digit limits) against `omnist-spec`'s divergence ledger found no
gap — see the ledger's Go `Resource caps` row (source-audited clean,
`omnist-spec` commit `2af12e0`).

## Spec version targeted

`omnist-spec` at commit `103a8c9` (`v0.21.0-beta`), pinned via the
`vendor/omnist-spec` git submodule. This repo does
not track the spec's `main` branch — the pin is bumped deliberately, in
its own commit. Past `c4141d0` (`v0.7.0-beta`), this pin also carries a
new §3.3 S-8 rule (a `Name` — record name or ref target — MUST match
`[A-Za-z_][A-Za-z0-9_]*`) and a characterization-only clarification of
S-3 (reserved-name matching is exact, case-sensitive) — both are
no-op for this port: the OSD grammar already enforced S-8 implicitly,
and `osd_parser.go`'s `scalarKeyword(name)`/`name == "any"` checks
already match S-3 as clarified. The new
`osd-grammar/reserved-names/case-mismatched-name-is-not-reserved`
vector passed green-on-arrival, spot-checked against the actual
parser rather than trusting a clean run alone. `extensions/osd-oml.md`
was also reformalized into a numbered-rule grammar (E.5-E.9, extension
v1.1) with a new `schema.invalid-name` error code; this doesn't affect
Track 2 since Go doesn't implement the OSD-OML extension yet (4
`extensions-osd-oml/*` vectors report as skip, tracked in
[issue #111](https://github.com/omnist-dev/omnist-go/issues/111),
which now also notes the new S-8/`schema.invalid-name` requirement
for whenever that extension is implemented). Past `0ac1eac` (the
#95-103 spec-correctness audit batch), this pin also carries the fix
for `omnist-spec#52` — a new §8.5.3 rule requiring a harness to strip
insignificant inter-tag XML whitespace before comparing a `write`
vector's expected/actual text, since the spec places no requirement on
XML writer whitespace at all. This repo's own conformance-test skip
for that vector (cited as `omnist-spec#52` in a prior revision of this
doc) is removed —
`formats-xml/basic/carriage-return-written-as-numeric-character-reference`
now passes outright. Past `aac3ce0`, this pin also carries 3 new §3.3
canonical-serialization-order characterization vectors
(`osd-grammar/canonical-output/declaration-order-round-trips-exactly`,
`prune/basic/survivors-keep-declaration-order-not-alphabetical`,
`normalize/basic/output-order-is-alphabetical-not-declaration-order`) —
all three passed green-on-arrival against this repo's existing behavior,
no code change needed.

See `omnist-spec`'s own [§9.3 status table](https://github.com/omnist-dev/omnist-spec/blob/main/docs/09-divergence-ledger.md#93-status-table)
for the cross-implementation divergence ledger this repo reports into.

## Divergences from the spec

None known. `omnist-go` can represent all seven scalar kinds natively and
has a real temporal `Scalar` variant, so the D-6/D-7(1)-class divergences
that affect TypeScript and Rust (a collapsed integer/number type, a missing
temporal variant) don't apply here.
