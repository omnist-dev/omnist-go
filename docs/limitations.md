# Limitations and status

`omnist-go` is a from-scratch Go implementation of the
[Omnist](https://spec.omnist.dev) data-interchange spec, built without
reference to the Python, TypeScript, or Rust implementations except as a
narrow, after-the-fact tie-breaker on spec gaps that already have a filed
`omnist-spec` issue. See `CONTRIBUTING.md` for the full policy.

## Status

**`v0.10.0-alpha`.** Every core operation is implemented: the Document and Schema
models, OML and OSD (read and write), `validate`, `materialize`, the full
schema algebra (`satisfiable_set`, `is_empty`, `prune`, `compatible_with`,
`equivalent`, `normalize`, `extract`, `lint`, `infer`), all four interchange
codecs (JSON/YAML/TOML/XML, read and write), a CLI, both tracks of the
conformance harness, and fuzz tests on every reader (`go test -fuzz`).

Track 2 ([`tools/conformance/`](https://github.com/omnist-dev/omnist-go/tree/main/tools/conformance),
JSON-vector, run against `omnist-spec`'s `test-suite/`) currently reports
**339 pass / 0 fail / 28 skip** of 367 vectors (`omnist-spec` v0.33.0-beta
pin), compared as a set of `(path, code)` per §8.5.2 — not code-agnostically.
All 28 skips are the OSD-OML extension
(`parse_schema_oml`/`write_schema_oml`) — not yet implemented in this
port, cited honestly per §9.5 rather than crashing or failing the
driver; see [issue #111](https://github.com/omnist-dev/omnist-go/issues/111)
for implementing it. All 28 are E-20's "not yet implemented" category;
none is a documented divergence (E-21). The 42 YAML alias-expansion
vectors run and pass: the runner passes a vector's `declared_max_alias_expansion`
to the reader as `Limits.MaxAliasExpansion` and its `declared_max_expanded_slots`
as `Limits.MaxExpandedSlots`, and only for a vector that carries the key. The
ten input-size vectors run and pass the same way (`declared_max_input_bytes` is
`Limits.MaxInputBytes`). The runner is an allowlist (spec E-20a): a vector that
carries any other `declared_*` key, or a known one on an operation whose driver
does not honour it, is reported as an E-20 skip and never run against this
implementation's own default, which is how the at-cap input-size vectors passed
without testing the boundary before the runner learned the key.
Track 1
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

### Safety limits (D-9 to D-11, D-18 to D-20)

Every reader enforces finite limits, set through `omnist.Limits` and defaulted
by `omnist.DefaultLimits()` (spec §2.4, D-11). `Limits.Validate()` is an opt-in
sanity check.

| Limit | `Limits` field | Default | Applies to |
|---|---|---|---|
| Nesting depth | `MaxDepth` | 200 | every reader |
| Node count | `MaxNodes` | 1,000,000 | every reader |
| Integer digits | `MaxIntDigits` | 4,300 | every reader |
| Alias expansion factor | `MaxAliasExpansion` | 50 | YAML only (the one codec with an anchor/alias mechanism); every other reader ignores it |
| Expanded size | `MaxExpandedSlots` | 1,000,000 | YAML only, and only for an input that contains an alias or a merge key |
| Input size | `MaxInputBytes` | 64 MiB (67,108,864 bytes) | every document reader (OML, JSON, YAML, TOML, XML); not OSD, which reads a schema |

**The alias expansion factor** (D-18, D-19, D-20). For each candidate node `a`,
`E(a) = W(a) / S(a)`, where `W` is the number of value slots materialized when
`a` is expanded and `S` the number written in its own definition (an alias
counts as one written slot; a merge key `<<` likewise, however many aliases it
holds). A candidate is every anchored definition and every mapping and
sequence, anchored or not: the document root when it is a container, an
unanchored mapping that merely holds aliases, and a mapping written inline as a
merge source (`<<: {k: 1}`) all count. Scalars are never checked (`E` is 1.00).
The YAML reader computes `E` for every candidate from the `yaml.Node`
reference graph in one linear memoized pass, BEFORE it expands anything, with
saturating arithmetic, and rejects the first candidate whose `E` exceeds the
maximum with `document.limit.alias-expansion` at path `$`. An anchor that refers
to itself, directly or through other anchors (merge keys included), is rejected
with the same code (D-20). `W` is the spec's conservative structural count:
where a merged key is overridden locally it may exceed what is finally
materialized, never the reverse. A programmatically built Document, and every
other format, is unaffected.

What `MaxNodes` and `MaxDepth` still do: `MaxNodes` counts only materialized
mappings, not scalars, so it does not bound a scalar-heavy expansion; that is
bounded only by the alias check, which caps the whole document at
`W <= MaxAliasExpansion x S(root)`, and by the expanded-size cap above, which
bounds `W(root)` itself at the default 1,000,000 slots (about 780 bytes each in
Go, so roughly 780 MB at the default). A long acyclic merge chain (each link at
`E = 1.00`) is bounded by `MaxDepth`.

**Merge sequences are carriers (D-18a).** A sequence in merge-value position is
a syntactic carrier, whether it is written in place or anchored: `<<: [*p, *q]`
and `<<: &s [*p, *q]` hold no slot in `W` or `S`, are not candidates, and give
the same verdict on the mapping that holds them. `<<: *s` where `s` is a
sequence contributes the sum over its members of `W(member) - 1` (never
`W(s) - 1`) and one `<<` slot in `S`; a plain alias to an anchored carrier
materializes the list (`1 + sum W(member)`), and an ordinary `s: &s [*p, *q]`
stays a candidate. Merge shapes are validated BEFORE anything is counted: a
scalar merge value, a scalar member of a merge sequence, a sequence inside a
merge sequence, and an alias to a sequence of scalars are `parse.codec-syntax` (at the offending node's `line:col`), and win over
every `document.limit.*` code, so a document with both a bomb and a malformed
merge reports the syntax error.

**Empty merge sequence** (D-18a, omnist-spec#127, v0.27.0-beta). `<<: []`, an
anchored `<<: &s []`, and `s: &s []` followed by `<<: *s` are well-formed
carriers that merge nothing: `W` contribution 0, and the `<<` entry is still one
slot in `S`. `config: {<<: []}` is an empty mapping. The input still contains a
merge key, so it is subject to the expanded-size cap. An empty sequence outside
merge position (`k: []`) is an ordinary node with `W = S = 1`, and reads to zero
edges, the same as an empty JSON array. (Before v0.7.1-alpha this port rejected
`<<: []` as `parse.codec-syntax` and `k: []` as `parse.empty-array`.)

**The expanded-size cap** (D-22). The factor bounds amplification, not
absolute size: a large document in which every container sits just under
`E = 50` is accepted by it and can still allocate gigabytes (30,000 such
containers, `W` about 3.2 million, took 16 to 22 seconds and 2.5 GB). So a YAML
input that contains at least one alias or merge key is also rejected when
`W(root)` (the total slots it materializes, from the same memoized pass) is
greater than `MaxExpandedSlots` (default 1,000,000; equal is accepted), with
`document.limit.expanded-size` at `$`. The cap is checked after every candidate
has passed the factor, so when both limits are crossed `alias-expansion` is
reported, and before anything is materialized. The two limits are independent:
an input can pass the ratio and fail the size, or the reverse. Compose-style
files sit far under the default: 100 services merging a 20-key block measure
2,223 slots, 100 merging a 60-key block 6,263, and 1,000 merging a 60-key block
62,063.

**The cliff.** An input with no alias and no merge key is exempt: a plain
two-million-slot YAML file passes, as a JSON or OML file of that size does (the
node limit and your own input-size bound govern it). Adding one alias to it
subjects the whole document to the cap, and it is rejected. This is deliberate
spec behaviour, not an accident of this reader. `W` is the conservative
structural count, so a document whose merged keys are overridden can be refused
by the cap although it materializes fewer slots.

**A mapping that merges a large anchor has a high `E` by design.** `E` of
`job: {<<: *base, script: x}` is `(keys + 2) / 3`: the referrer writes three
slots (itself, the `<<` entry, `script`) and materializes every key of `base`
as well. With 60 keys in `base` it is accepted; with 148 keys `E` is exactly
50.00 (accepted); with 150 keys it is about 50.67 and rejected at the default.
This is the spec's stated behaviour, not a false positive to work around in
the reader. If you merge a genuinely large anchor, raise
`Limits.MaxAliasExpansion` (`Validate()` accepts up to 10,000); if the document
as a whole then expands past the size cap, raise `Limits.MaxExpandedSlots` too
(`Validate()` accepts up to 10,000,000, the spec's recommended ceiling, about
8 GB at Go's cost per slot). The compose
pattern of many services merging a modest defaults block is nowhere near it: 100
services merging a 20-key anchor is accepted at the default.

**Invalid values.** "No limit" is not a legal choice (D-10). A zero
`MaxAliasExpansion` means "unset" and selects the default of 50, so a `Limits`
literal written before the field existed keeps a finite limit; a negative
value is also treated as unset (never as unbounded) and `Validate()` reports
it as an error, as it does a value above `MaxRecommendedAliasExpansion`
(10,000). `MaxExpandedSlots` follows the same convention: zero or negative
selects the default of 1,000,000, `Validate()` rejects a negative value and one
above `MaxRecommendedExpandedSlots` (10,000,000), and
`Limits.EffectiveMaxExpandedSlots()` returns the value a reader enforces.
`Limits.EffectiveMaxAliasExpansion()` returns the factor.

**The input size** (D-23 to D-26, v0.10.0-alpha). Every document reader first
compares the byte length of its input, as received, with
`Limits.EffectiveMaxInputBytes()`. The check runs before decoding and before
any parse, so it takes precedence over `parse.invalid-encoding` and every
other diagnostic: an input of more bytes than the maximum is refused with
`document.limit.input-size` at path `$`, even if it is also invalid UTF-8 or
malformed, and an input of exactly the maximum is accepted. The unit is bytes,
not characters (`é` is two), and a leading byte-order mark is counted (three
bytes) because the length is taken before the mark is stripped (D-15). It
applies to JSON, YAML, TOML, XML and OML alike, a YAML input with no alias
included, which D-22 does not reach. `omnist.CheckInputSize` is the check and
`omnist.PrepareDocumentInput` is what each reader calls (the size check, then
`PrepareInput`). OSD reads a schema, not a Document, and is not covered
(D-25 gives schema size a bound with no code and no vector).

`MaxInputBytes` follows the convention of the other defaulted limits: zero or
negative selects the default, never "no limit" (D-10); `Validate()` rejects a
negative value and one above `MaxRecommendedInputBytes` (1 GiB, a reader holds
the input and then a Document several times its size);
`Limits.EffectiveMaxInputBytes()` returns the value a reader enforces.

*Why 64 MiB (D-24 asks for a documented number and, for any value above
10 MiB, a measurement).* It matches the Python port's default and replaces the
CLI's former fixed 100 MiB read cap (issue #76), so a file the library accepts
the CLI accepts too. The cost behind it was measured on this implementation
(Go 1.26, one core, flat documents, 2026-10-05). Linear readers: JSON, YAML and
OML parse about 0.5 MiB of one-key-per-line mapping in 0.4 to 0.6 s and 9 to
10 MiB (900,000 keys, near the 1,000,000-node limit) in 8.7 s (JSON), 7.8 s
(YAML) and 15.8 s (OML, slightly superlinear); a single 64 MiB string takes
0.2 to 2.9 s depending on the codec. **TOML and XML are quadratic in the number of keys
today:** 50,000 flat keys took about 77 s (TOML) and 49 s (XML), against under
1 s for the others, because each key or element computes its `line:col`
eagerly. No byte cap bounds that, so for those two codecs the maximum limits
memory but not time until the readers are fixed (issue #132, not part of this
release). Lower `MaxInputBytes` for untrusted TOML or XML input.

**The CLI** takes `--max-input-bytes N` on every command that reads an input
(default 64 MiB). It stops reading at N+1 bytes and refuses the input with a
message that names `document.limit.input-size` and the flag. A value that is
not a positive integer, or is above `MaxRecommendedInputBytes`, is a usage
error. An oversized input exits `1` (an input the CLI could not read), as the
former 100 MiB refusal did.

### Codex audit cycle (#70–#81)

A 12-issue Codex audit cycle (#70–#81) resolved across 4 phases addressed all outstanding audit findings: a precision correctness fix for integer-to-number materialization (#70), a patch for CVE GO-2026-6088 via a Go toolchain pin (1.26.6) and scheduled CI `vulncheck` job (#73), strict CI gating for both conformance tracks (#74), two quadratic CPU-exhaustion DoS fixes across validation/materialization/subtyping path indexing (#71, #80) and OML/OSD zero-copy lexer scanning (#72), schema-aware XML pretyping per `omnist-spec#44` (#81), and design/hardening improvements including `Limits.Validate()` (#78), explicit acyclic validity contracts (#77), and CLI input size caps (#76).

## Versioning

**`v0.10.0-alpha`**, a minor per `CONTRIBUTING.md` §1: new public API
(`Limits.MaxInputBytes`, `DefaultMaxInputBytes`, `MaxRecommendedInputBytes`,
`Limits.EffectiveMaxInputBytes`, `CheckInputSize`, `PrepareDocumentInput`,
`CodeDocumentLimitInputSize`, `CheckEncodable`, the CLI flag
`--max-input-bytes`), an API change (`oml.Write` and `oml.WriteCompact` gain a
third result, an error), and a closed silent-corruption bug (the writers
replaced invalid UTF-8 bytes with U+FFFD). It adopts `omnist-spec` v0.33.0-beta
(commit `64cbb68`, tag `v0.33.0-beta`) from v0.28.0-beta, closing issues #129
and #130. Conformance, Track 2: **339 pass / 0 fail / 28 skip of 367** (the 28
skips are the OSD-OML extension, issue #111), Track 1 19/19. Before this
release, run against the v0.33.0-beta suite, it was 333 pass / 6 fail / 28 skip:
the five over-cap input-size vectors (D-23 was not enforced), and `DIV-24`
(`formats-xml/nulls/null-leaf-in-repeated-label-is-indexed`); the five at-cap
input-size vectors passed without testing anything because the runner ignored
`declared_max_input_bytes`. What changed:

- **E-10, the index on every edge of a repeated label** (v0.30.0-beta, #130,
  `DIV-24`). `validate` and `materialize` already followed it (the seven
  `repeated_label_paths` vectors pass). The XML reader's
  `format.attribute-dropped` and `format.namespace-dropped` diagnostics are now
  resolved once the document is read, because a first occurrence cannot know it
  repeats until its later siblings are seen (`$.root.item[0]`, not
  `$.root.item`). The XML writer's `write.unsupported-value` paths carry the
  index too (null leaf, empty internal node, illegal control character).
- **OML-29 and the colon gap** (v0.31.0-beta, #129). A gap after the colon
  already worked (the seven vectors pass). A newline, `;` or comment-then-newline
  before the colon was wrongly accepted: `a\n: 1`, `a;: 1`, `a #c\n: 1`, CRLF
  forms. oml.abnf says `edge = label skip COLON gap value`, so a separator before
  the colon is a SEP where the colon is owed: `parse.bare-word` at the label at
  the top level (the lookahead sees no label-colon pair, as Python does) and
  `parse.unexpected-token` at the end of the label, where the separator run
  starts, inside braces and for a later top-level edge. A **lone CR** is not a
  newline in oml.abnf (`newline = CRLF / LF`, E-29) and not horizontal space, so
  it is no longer skipped as a separator anywhere (`a: 1\rb: 2` now fails with
  `parse.unexpected-token` at the CR); the position of that rejection is the one
  thing E-29 leaves open, and this port reports the CR itself.
- **D-23 to D-26, the input size** (v0.30.0-beta). See "The input size" above.
- **C-9, every writer refuses a string with no UTF-8 encoding** (v0.32.0-beta).
  `omnist.CheckEncodable` runs first in the JSON, YAML, TOML, XML and OML
  writers and fails with `write.unsupported-value` at the Document path of the
  node holding the string (a label: the node holding the edge; both indexed
  per E-10), unconditionally, never with an escape. Before: JSON, TOML, OML and
  XML values silently got U+FFFD, YAML failed with an uncoded library error, and
  XML refused a label with the label in the path. The check is a pathless walk
  that only answers yes or no; the path is built by a second walk only on
  failure. Measured on 200,000-edge documents (median of 11 to 21 writes,
  alternated), the added time of the check is about 5% to 10% for JSON, 1% to
  3% for YAML, 0% to 5% for TOML and 15% to 19% for XML (flat 200,000 children:
  about 52 ms without it, 60 ms with it). The XML writer's E-10 indexing builds
  its path only when a write fails, so a successful write pays nothing for it.
  **`oml.Write` and `oml.WriteCompact` return
  `(string, []omnist.Diagnostic, error)`** now: OML had no failure mode before,
  and this is its one. Migrating:

  <!-- doc-illustrative -->
  ```go
  // before v0.10.0-alpha
  text, diagnostics := oml.Write(doc, false)
  
  // from v0.10.0-alpha: a third result, the C-9 failure
  text, diagnostics, err := oml.Write(doc, false)
  if err != nil {
      return err // write.unsupported-value: a string or label is not valid UTF-8
  }
  ```

  The XML writer also turns U+FFFE and U+FFFF in a string value into U+FFFD
  without an error (unchanged by this release). That is the same silent change
  C-9 forbids for invalid UTF-8, though outside its letter; it is tracked
  separately.
- **C-10, the XML writer's null leaf** (v0.33.0-beta). It already failed with
  `write.unsupported-value` (top level, nested, `strict` irrelevant); with the
  E-10 index the repeated-label path is `$.root.item[1]`. New tests pin the
  five `formats-xml/nulls/*` cases and the read-side neighbour.

**`v0.9.0-alpha`**, a minor per `CONTRIBUTING.md` §1: new public API (the error
codes `CodeSchemaInvalidName`, `CodeSchemaInvalidLabel`,
`CodeSchemaUnknownRecord`) and behaviour callers can observe, closing issue
#121. It adopts `omnist-spec` v0.28.0-beta (commit `1a7d0de`, tag `v0.28.0-beta`) from v0.27.0-beta.
The spec adds no vectors (DIV-5), so conformance is unchanged: Track 2 **310
pass / 0 fail / 28 skip of 338**, Track 1 19/19. Go's unit tests are the only
pin for the four rules below. What changed:

- **S-22, `schema.invalid-label`.** A field label that is not valid UTF-8 (a Go
  string can hold any bytes) fails `NewRecord`, `NewSchema`, `Schema.Validate`
  and `osd.Write` at the record path `R`. The label is never put in the path,
  and the message shows it through `%q`, so non-printables are escaped.
  `osd.Write` used to write the bytes unrepaired (`osd.Read` then rejected them
  with `parse.invalid-encoding`).
- **S-8, `schema.invalid-name`, now enforced at `$`.** A record name, an
  `EnvOrder` entry, or a reference target that does not match
  `[A-Za-z_][A-Za-z0-9_]*` is reported at `$` with the name in the message only.
  v0.8.0-alpha did not check S-8 at all, so a hand-built schema with such a name
  was accepted (the path it would have used, a Document path, was
  inapplicable). Nothing that was reported before changes path; the rule is
  newly enforced. OSD-OML keeps its Document path, but Go has no OSD-OML schema
  reader or writer (issue #111).
- **S-23, `schema.unknown-record`.** An `EnvOrder` entry with no record in
  `Env` (absent or nil) is reported at `$`. v0.8.0-alpha skipped it in
  `Validate` and `osd.Write` could panic on it; it now returns the structured
  error.
- **OSD-16 / S-24, `[0,0]`.** The model still represents it (S-15, and
  `Validate` accepts it), but `osd.Write` fails on a field with `Max == 0` and
  not `Unbounded` with `write.unsupported-value` at the record path `R`, in
  both layouts. The spec's wording is for `prune`, not `normalize` or
  `minimize`: `algebra.Prune` already removes `max = 0` fields from every record
  it rebuilds but keeps an unsatisfiable root intact, so prune before writing.
  Go has no `minimize` and `Normalize` only reorders and copies cardinalities,
  as before.
- **`Infer` names records ASCII-only** (S-8): non-ASCII and other characters
  in a key become `_`, a leading digit gets a `_` prefix, a leading lowercase
  letter is capitalised, an empty key becomes `Rec`, and a name already used
  gets the smallest numeric suffix (`2`, `3`, ...). `éclair` is `_clair`,
  `日本` is `__`, `a b` is `A_b`. Before this, a non-ASCII key produced a record
  name that `osd.Read` rejected and an empty key produced `omnist.Field`; both
  would now fail `osd.Write`, so `omnist infer` would have exited 2.
- A record present in `Env` but omitted from `EnvOrder` is accepted by
  `Validate` (the spec does not require otherwise) and `osd.Write` does not
  write it.
- When a schema has several violations, which one is reported is
  implementation-defined; `Validate` returns the first in the order documented on it.

Issue #121's three cases (an empty label, `[` or `]` in a label, invalid UTF-8)
all fail with a spec code before `osd.Write` emits text, so
`Read(Write(s)) == s` holds for every schema `Write` accepts.

### Previous: v0.8.0-alpha

**`v0.8.0-alpha`**, a minor per `CONTRIBUTING.md` §1: new public API
(`NewRecord`, `NewSchema`, `Schema.Validate`), issue #121. A hand-built `Schema`
could violate spec §3.3's S-1 to S-7 and §5.4's label rules, and `osd.Write`
then emitted text `osd.Read` rejects (`schema.empty-label`,
`schema.bracket-in-label`, `schema.duplicate-field`, and the rest). Now
`Schema.Validate` reports the first violation as an `omnist.Diagnostic` with the
spec's `schema.*` code and Schema path (§8.4.1, E-30), `NewRecord`/`NewSchema`
check at construction, and `osd.Write` validates first. `Schema`, `Record` and
`Field` remain public structs, so a literal is unchecked until `Validate` or
`osd.Write`; the validate, materialize and algebra operations assume a
well-formed schema and do not re-validate. A schema that was valid before is
unaffected. Spec pin unchanged (v0.27.0-beta, `a6a6090`); conformance unchanged
(Track 2 310 pass / 0 fail / 28 skip of 338, Track 1 19/19). Four things were left open then, because the spec assigned no code or path for
them. omnist-spec v0.28.0-beta specifies all four and v0.9.0-alpha implements
them (see above): S-8 (`schema.invalid-name` at `$`), invalid UTF-8 in a label
(`schema.invalid-label`), cardinality `[0,0]` in a writer (`write.unsupported-value`)
and an `EnvOrder` naming no record (`schema.unknown-record`). At v0.8.0-alpha
`osd.Write` emitted the first three as text and could panic on the fourth.

### Previous: v0.7.1-alpha

**`v0.7.1-alpha`**, a patch per `CONTRIBUTING.md` §1: no new public API, and the
change is a correctness fix of input wrongly rejected, not a security or
data-corruption fix. It adopts `omnist-spec` v0.27.0-beta (`a6a6090`, from
v0.26.0-beta). Conformance, Track 2: **303 pass / 7 fail / 28 skip of 338**
before any code change (the seven new empty-merge-sequence vectors),
**310 pass / 0 fail / 28 skip of 338** after. Track 1 stayed 19/19. What changed:

- **Empty merge sequence accepted** (D-18a): see "Empty merge sequence" above.
  The merge-shape pass no longer refuses an empty sequence; the carrier
  arithmetic already gave W 0 and one `<<` slot in `S`.
- **Empty YAML sequence reads to zero edges.** The v0.27.0-beta vectors pin that
  an ordinary empty sequence materializes no edge (`t: {<<: [], k: []}` is an
  empty mapping). The YAML reader previously raised `parse.empty-array`; it now
  matches the JSON reader. OML's and TOML's `[]` are unchanged.
- The `Go-specific, spec-undecided` note on `<<: []` is removed.

### Previous: v0.7.0-alpha

**`v0.7.0-alpha`**, a minor bump per `CONTRIBUTING.md` §1: it adds new public
API (`Limits.MaxExpandedSlots`, `DefaultMaxExpandedSlots`,
`Limits.EffectiveMaxExpandedSlots`, `MaxRecommendedExpandedSlots`,
`CodeDocumentLimitExpandedSize`) and closes a memory-exhaustion path (an input
every container of which sat under the alias factor, but whose total expansion
was gigabytes). It adopts `omnist-spec` v0.26.0-beta (`7744a5c`, from
v0.25.0-beta). Conformance, Track 2: **296 pass / 7 fail / 28 skip of 331**
before any code change, **303 pass / 0 fail / 28 skip of 331** after. The seven:
the anchored-carrier and alias-to-sequence vectors at the limit (wrongly
rejected, D-18a), four D-22 expanded-size vectors (accepted where
`document.limit.expanded-size` was expected), and
`malformed-merge-after-a-bomb-reports-codec-syntax` (the bomb check fired before
the malformed merge was seen). Track 1 stayed 19/19. What changed:

- **Carrier rules (D-18a).** An anchored merge sequence, and `<<: *s` with `s` a
  sequence, are counted per "Safety limits" above; the old "known edge" (an
  anchored carrier counted as an ordinary merge value) is gone.
- **Merge shapes first.** A malformed merge is `parse.codec-syntax`, detected in
  a pass before counting, so it wins over every limit. The reader's own
  merge-shape checks are now that pass.
- **Expanded-size cap (D-22).** `Limits.MaxExpandedSlots`, default 1,000,000, for
  inputs with an alias or merge key; see "The expanded-size cap" and "The
  cliff" above.
- **Conformance runner.** `declared_max_expanded_slots` is passed through for
  vectors that carry it only; `(path, code)` sets are still compared strictly and
  there is no known-failing list.
- **Container keys and long chains.** A container as a mapping key is measured
  on its own subtree and folds into nothing (the reader refuses it as
  `document.unlabeled-element`). The walk is iterative, so a 100,000-link alias
  chain is safe with no port-specific depth guard.

### Previous: v0.6.0-alpha

**`v0.6.0-alpha`**, a minor bump per `CONTRIBUTING.md` §1: it adds new public
API (`Limits.MaxAliasExpansion`, `DefaultMaxAliasExpansion`,
`Limits.EffectiveMaxAliasExpansion`, `MaxRecommendedAliasExpansion`,
`CodeDocumentLimitAliasExpansion`) and closes a CPU-exhaustion path (a YAML
alias bomb was bounded only after the fact, by the node cap). It implements
omnist-spec D-18, D-19 and D-20 (`DIV-3`, issue #117) as amended by
`omnist-spec` v0.25.0-beta (PR #122): the pin moves from v0.24.0-beta to
v0.25.0-beta (`3febae9`), where every mapping and sequence is a candidate.
Conformance, Track 2: **268 pass / 0 fail / 34 skip of 302** at v0.24.0-beta
before this work; **278 pass / 6 fail / 28 skip of 312** at v0.25.0-beta with
the anchored-only check (the six new rejection vectors fail: unanchored merge
fan-in, list of aliasing mappings, root-level aliases, unanchored one-past,
large-anchor-over-limit, inline-merge-source one-past); **284 pass / 0 fail /
28 skip of 312** with the container check. The 28 remaining skips are the
OSD-OML extension. Track 1 stayed 19/19. What changed:

- **Alias expansion limit (D-18, D-19, D-20).** `yaml.Read` now computes the
  expansion factor of every mapping and sequence (anchored or not, root and
  inline merge sources included) from the node graph before building the
  Document (see "Safety limits" above) and rejects an over-limit container, or
  an anchor that refers to itself, with `document.limit.alias-expansion` at
  `$`. The nested-anchor bomb shapes that took seconds (branching 4 over 10
  levels accepted after 5 s; branching 10 over 7 levels refused by the node cap
  after 25 s) are now refused in well under a millisecond. The self-merge
  `a: &a {<<: *a}` was stopped only by the depth limit; it is now a D-20
  rejection. Two shapes an anchored-only check let through are refused too: an
  unanchored merge fan-in (an anchored N-key block and `t: {<<: [*b x M]}`) and
  root-level alias fan-out (`- {k: *b}` or `- *b` repeated over a large
  anchor).
- **Configurable.** `Limits.MaxAliasExpansion`, default 50; zero or negative
  means the default, never unbounded.
- **Conformance runner.** `declared_max_alias_expansion` is no longer a skip
  key: the driver passes the vector's declared maximum through the new option,
  for vectors that carry it only. `(path, code)` sets are still compared
  strictly and there is no known-failing list.
- **Inline merge sources.** Per §2.4.1, an inline mapping merged in place
  (`<<: {k: 1}`) is a candidate itself (`E` is 1.00 only when it holds no
  aliases). In the referrer the `<<` entry is one slot of `S`, the inline
  mapping's own written values add theirs, and its container is flattened in
  `W` (contribution `w-1`).
- **Large-anchor merges.** A mapping merging a big anchor has `E` of about
  `(keys + 2) / 3`; see "Safety limits" above. Raise `Limits.MaxAliasExpansion`
  if needed.

### Previous: v0.5.2-alpha

**`v0.5.2-alpha`**, a patch bump per `CONTRIBUTING.md` §1 (no new public
API, no DoS or correctness-corruption fix: parse diagnostics and the set of
accepted OML arrays move to what the spec now states), adopting `omnist-spec`
v0.24.0-beta (from v0.22.0-beta; `v0.5.1-alpha` was never tagged, so this is
the first tag to carry both adoptions). Conformance against the new pin,
Track 2: **255 pass / 13 fail / 34 skip of 302** before any code change (the
five OML-28 vectors and four valid-array vectors, plus the four codec-syntax
vectors whose expected path is E-32's `"line:col"` placeholder, which the
runner did not know); **268 pass / 0 fail / 34 skip of 302** after. Track 1
stayed 19/19; the 28 OSD-OML and 6 alias-expansion skips are unchanged. What
changed:

- **OML-28 and the array rule (omnist-spec#115, #117; DIV-8, DIV-9).** Inside
  `[...]`, a newline or `;` where a comma was owed is
  `parse.separator-in-array` only when a value-start token follows (a scalar
  token, any `IDENT`, `{` or `[`), at that token. When `}`, `:` or the end of
  input follows, it is `parse.unexpected-token` at that token (an unterminated
  `a: [1, 2` newline is `2:1`, without the newline `1:9`). A newline or `;`
  before `,` or `]`, after any element, is insignificant: `a: [1` newline `]`,
  `a: [1` newline `, 2]`, `a: [1, 2` newline `]` and `a: [1;]` are valid.
  The ABNF also allows a separator after `[` and after a `,`, and the reader
  now accepts both (`a: [` newline `1]`, `a: [1,` newline `2]`), which it had
  rejected; the spec calls both decorative. The position is now always the
  blamed token's own, not the first newline or `;` of a longer run.
- **E-31 codec positions (omnist-spec#114, DIV-8).** A `parse.codec-syntax`
  path is now a `line:col` inside the input. The XML reader reported `1:0` for
  empty input and a byte column otherwise; it now reports `1:1` and converts
  the decoder's line and byte column to code points (E-28), clamped to the
  input. The JSON and TOML readers counted bytes for the column; both now
  count code points through the same helper (`internal/textpos`). YAML is
  unchanged: yaml.v3 exposes a line only in its message text, so the column
  stays `1` and the line is the library's. D-21 and D-14 stay `1:1`.
- **E-32 runner placeholder.** The Track 2 runner accepts the literal
  `"line:col"` only on a lone `parse.codec-syntax` entry of a JSON, YAML, TOML
  or XML parse vector: the code must match, the path must match
  `^[1-9][0-9]*:[1-9][0-9]*$` and lie inside the input. Every other path is
  still compared byte for byte, and elsewhere the placeholder is an ordinary
  string that cannot match.
- Not in this release: D-18/D-19/D-20 (`DIV-3`, issue #117) and construction
  label checks (issue #121).

### Previous: v0.5.1-alpha

**`v0.5.1-alpha`**, a patch bump per `CONTRIBUTING.md` §1 (no new public
API; a narrow, spec-driven error-code fix), adopting `omnist-spec`
v0.22.0-beta (from v0.21.0-beta). Conformance against the new pin, Track 2:
**243 pass / 10 fail / 34 skip of 287** before any code change (the 14 new
vectors, of which the port already passed 4: the two code-point column
vectors, the scalar-then-`}` vector and the two-edge negative control);
**253 pass / 0 fail / 34 skip of 287** after. Track 1 stayed 19/19. What
changed:

- **OML-26 with a separator (omnist-spec#109, DIV-7).** After a complete
  top-level edge, the edge list continues only if a separator is followed by
  a STRING or an IDENT. Any other leftover token (`}`, `]`, `,`, `:`, `{`,
  `[`, and every scalar-only token, `nan`/`inf` included) is
  `parse.trailing-content` at that token, with or without a separator. The
  port used to report `parse.unexpected-token` when a separator preceded it.
  A STRING/IDENT after a separator is the next edge and reports its own error
  if malformed (`a: 1` newline `null: 2` is `parse.reserved-word-label` at
  `2:1`). Inside `{...}` and `[...]` (OML-27) nothing changed.
- **E-28/E-29.** OML and OSD columns already counted code points and a CRLF
  as one break; tests now pin combining marks, tabs, BMP non-ASCII and CRLF.
  Codec positions are untouched (omnist-spec#114).
- Unspecified and unchanged: a document of only `}`, `,` or `]`, a lone CR,
  and an unterminated array (`a: [1` newline reports `parse.separator-in-array`
  here; the spec has no vector or rule for it).

### Previous: v0.5.0-alpha

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

`omnist-spec` at commit `64cbb68` (`v0.33.0-beta`), pinned via the
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
