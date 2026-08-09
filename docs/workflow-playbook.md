# omnist-go workflow playbook

This document is binding for this repo. It exists so the rules survive past
any one session. If you are an agent or contributor about to write code
here, read this first.

## 0. Spec-first, not reference-implementation-first

This is a deliberate inversion from how `omnist-rs` was built. `omnist-rs`
read Python (`omnist`) as its day-to-day reference and checked the spec
occasionally to settle disputes. `omnist-go` runs the loop the other way.

- **Primary source**: `vendor/omnist-spec`'s normative docs. If the spec says
  it, that's the contract.
- **Secondary, tie-breaking only, and only after a spec issue is already
  filed for the gap** — never as a way to avoid filing one. `omnist`,
  `omnist-ts`, `omnist-rs` source may be read only to resolve a case the spec
  doesn't pin down precisely enough to implement from prose alone, or to
  write a better-informed spec issue. Their agreement is evidence of intent,
  not proof.
- **Never**: treat "Python does X" (or Rust, or TypeScript) as sufficient
  justification for a Go behavior on its own, when the spec doesn't say X.

**When a gap is load-bearing** (shapes a design decision, affects more than
the immediate piece of work): file the `omnist-spec` issue, then actually
stop that piece of work until it resolves. Don't guess and continue.

**When a gap is narrow and cosmetic**: file the issue anyway, proceed with
the plainly-correct reading, and flag the assumption explicitly in the code
and the PR description.

Do not open `~/dev/omnist`, `~/dev/omnist-ts`, or `~/dev/omnist-rs` except as
a step-3 tie-breaker on a gap that already has a filed `omnist-spec` issue
(spec §10.4 step 2/3). Not "just to check something."

## 1. Versioning

`omnist-go` stays on `v0.0.x-alpha` until: core document model + all four
codecs (JSON/YAML/TOML/XML) + OML + OSD + CLI are implemented, and our own
conformance harness (`tools/conformance/`) passes with zero real fails
(skips permitted and cited). This mirrors `omnist-rs`'s rule. The bump past
`0.0.x` is a maintainer sign-off event, not a routine release mechanic — not
triggered by accumulated features or fixes alone.

The spec version this repo targets is stated here and in every release:
currently **omnist-spec v0.2.0-alpha** (pinned via `vendor/omnist-spec`
submodule). Ship pass/fail/skip counts alongside every release, per spec
§10.3.

Ship code, tests, docs, and any version bump together, in the same PR.

## 2. Foundational design decisions

Each decision below gets its own subsection with the choice and the "why."
Nothing in §2 of this doc is provisional — these are load-bearing choices
that later code depends on. If one needs to change, that's a new decision
recorded here with the old one struck through, not silently overwritten.

### 2.1 Document node representation

**Decision**: pointer-based tree (`*Node` holding `[]Edge`), not an
arena+index scheme.

**Why**: `omnist-rs` used an arena (`Vec<Entry>` + index newtype)
specifically to avoid `Rc<RefCell<_>>` under Rust's ownership model. Go has
a garbage collector and no borrow checker, so that motivation doesn't apply.
A plain pointer tree is the idiomatic Go answer and keeps the Document model
(spec §2.2 — a node is literally an ordered list of labeled edges) directly
representable without an extra indirection layer.

### 2.2 Scalar type shape

**Decision**: a tagged struct with a `Kind` discriminant and one field per
kind's Go-native representation:

```go
type ScalarKind int

const (
    KindString ScalarKind = iota
    KindInteger
    KindNumber
    KindBoolean
    KindDate
    KindTime
    KindDateTime
)

type Scalar struct {
    Kind ScalarKind
    Str  string
    Int  *big.Int
    Num  float64
    Bool bool
    Date DateValue
    Time TimeValue
    DateTime DateTimeValue
}
```

**Why**: spec §2.2.1 defines exactly seven scalar kinds and is explicit that
implementations MUST NOT add or collapse kinds — doing so changes the
Schema Algebra's subtyping lattice (§2.2.1, §D-5). Go can natively
distinguish `integer` from `number` (unlike TypeScript's single `number`
type, which forced the D-6 divergence-ledger entry), so there is no reason
to accept that collapse here. `integer` uses `math/big.Int` because spec
§2.4 requires supporting integers up to 4,300 decimal digits — `int64`
would silently violate that limit.

### 2.3 Ordered-map / repeated-label representation

**Decision**: the node stays edge-list-native everywhere —
`type Node struct { Edges []Edge }`, `type Edge struct { Label string;
Target Target }` — with no separate "canonical value" (map-collapsed) type
alongside it. This differs from `omnist-rs`'s `Value`/`RawNode` split.

**Why**: spec §2.1/§2.2 defines the Document model itself as an ordered edge
list with repeats permitted (`[(item,"pen"), (note,"rush"), (item,"pad")]`).
That's not a special case for OML/XML — it's the model, full stop; JSON,
YAML, and TOML documents are represented the identical way after reading
(spec's own worked table in §2.1 shows this). Introducing a second,
map-shaped type invites exactly the "note has moved" bug the model exists to
prevent, if any code path accidentally uses the collapsed type instead of
the edge list. If a later convenience API (e.g. a JSON-like accessor for
callers who don't need repeated-label fidelity) turns out to be useful, it
will be a derived read-only view computed from `Node`, never the source of
truth, and will get its own decision record here when it's built — not
assumed now.

**Open flag**: this is the one decision in this doc most likely to need
revisiting once OML/XML round-trip code is actually written. If it turns
out unworkable, update this section rather than silently diverging from it
in code.

### 2.4 Error-type hierarchy

**Decision**: structured errors from day one —

```go
type ParseError struct {
    Line    int
    Col     int
    Path    string
    Code    string
    Message string
}
```

with `ValidationResult` as `[]ValidationError{Path, Code, Message}` per spec
§3.6.

**Why**: the playbook notes that `omnist-rs`'s structured `ParseError{line,
col, message}` (vs. TypeScript's message-only error) let materially more
spec-conformance vectors run for real instead of being skipped, purely
because the error type could report *where* something failed, not just
*that* it failed. This costs nothing extra in Go and is the same shape spec
§8's error taxonomy (`(path, code, message)`) already expects implementations
to produce.

## 3. Port order

Per spec §9.5's recommended build order:

1. Document model core + §2.4 safety limits (depth, node count, integer
   digits — finite, documented, enforced)
2. OML reader, then canonical OML writer (Core, then Extended)
3. OSD reader, then canonical OSD writer
4. `validate`
5. `satisfiable_set`, `is_empty`, `prune`
6. `compatible_with`, then `equivalent`
7. `normalize` (needs `prune`)
8. `extract` (needs `prune` and `normalize`)
9. `lint` (needs `satisfiable_set` and `equivalence_classes`)
10. `infer`
11. Remaining codecs: JSON, YAML, TOML, XML
12. `materialize`
13. CLI
14. Fuzzing / property-based testing (Go's built-in `testing/quick`-successor,
    `go test -fuzz`, decided here rather than bolted on later)

Steps 1-6 are the useful core per the spec; a port that stops there is still
worth having.

Step 14 landed as issue #57: one `FuzzRead` per reader package (`oml`,
`osd`, `formats/{json,yaml,toml,xml}`), seeded from this repo's own test
literals plus vendored `omnist-spec` test-suite/conformance-fixture text.
Two real bugs surfaced immediately, both in `formats/toml`: go-toml/v2's
unstable parser can tag a value node `Kind=LocalDate`/`LocalTime`/
`LocalDateTime`/`DateTime` on text that does not actually have the full
digit layout `ParseISODate`/`ParseISOTime` require as their documented
precondition (e.g. bare `"00:"` reaching `Kind=LocalTime`), and
`MatchesISOKind` itself had a latent bug where `regexp.FindString("")`'s
ambiguity between "no match" and "an empty match" made it spuriously
return true for an empty string against every kind. Both fixed (validate
before calling the precondition-trusting parsers; reject `""` up front in
`MatchesISOKind`), both covered by unit tests, not just the fuzz corpus.
CI runs a short (10s/package, 60s total) `-fuzz` burst on every push,
separate from — and much shorter than — the 30-60s-per-package local runs
that found the above; see `.github/workflows/ci.yml`'s `fuzz` job for the
regression-vs-exploration distinction.

## 4. Conformance harness — built early, interleaved

Do not defer this to the end. Start it right after step 2 (OML) lands, and
extend it as each subsequent operation/codec lands, not as one big effort
after the library is "done." `omnist-rs` built its full library first and
only then built its harness, and real bugs (XML leaf-typing, YAML
sexagesimal ints, the OML temporal-provenance bug — see spec §9.4 D-7(2))
sat undetected through several releases as a result.

- **Referee first.** Port the 10-case `_referee-self-test` fixtures from
  `vendor/omnist-spec/conformance/fixtures/_referee-self-test/` and get them
  green before trusting any real comparison. Document comparison uses
  `Node`'s own order-sensitive equality (order is data, per spec D-1/D-3).
  Schema comparison needs both `exact` (record names must match — used for
  `normalize`/`prune`/`extract`) and `isomorphic` (same structure up to
  renaming — used only for `infer`, per spec §6.10) modes.
- **Track 1** (`tools/conformance/track1/`): walks
  `vendor/omnist-spec/conformance/fixtures/`, invokes operations via
  **direct library calls** (decided over a CLI wrapper — this port is
  primarily a library, matching `omnist-ts`'s and `omnist-rs`'s choice, not
  Python's CLI-subprocess approach), compares via the referee.
- **Track 2** (`tools/conformance/track2/`): walks
  `vendor/omnist-spec/test-suite/`, dispatches on each vector's `operation`
  field, compares `expect` per spec §8.5.2 (message text never compared,
  diagnostics compare as a set of `(path, code)`, no partial matching).
- **Diagnostics matching mode** (code-agnostic vs. exact) gets decided
  empirically against a real vector once error codes exist — don't assume
  Python's or Rust's answer transfers.
- **Skips** are permitted and MUST be reported; every skip cites either
  "not yet implemented" or a numbered `omnist-spec` divergence-ledger entry
  (§9.4). Never an ungrounded skip reason.
- **CI gate**: separate job from unit tests. Fails the build on nonzero real
  `fail` count, never on nonzero `skip` count (provided every skip is
  cited). Adversarially verify this gate once it exists: break something
  real, confirm the job goes genuinely red with nonzero exit, revert,
  confirm clean. Do not trust a read of the gating code alone.

## 5. The engineering loop

1. File an issue with the actual spec section cited in the body (design +
   reasoning, not "fix X").
2. Post the plan, wait for real sign-off — a distinct step from filing the
   issue.
3. Cut a branch referencing the issue, build to the spec.
4. Red before green for any behavior change: failing test first, shown
   failing, then implement to green, shown passing.
5. Run the full gate locally once, open the PR, let CI run it again for
   real.
6. Verify independently, tiered by risk: zero-production-diff PRs
   self-verify (read diff, confirm CI green); real production changes get a
   genuinely independent review (fresh agent/session, no memory of writing
   the change, reads the actual diff and runs the actual gates itself).
7. If review finds a real issue, fix and re-verify in the same PR. If the
   plan itself needs to change, say so as a comment, not a silent patch.
8. A surfaced separate problem gets its own new issue, not scope creep on
   the current PR.
9. Merge, with the version bump (when applicable) in the same PR.
10. Close the issue with what actually happened, including any deviation
    from the original plan.

## 6. Sharp edges (carried forward from the other three ports)

- A pinned "expected N passed / M failed / K skipped" regression assertion
  on the conformance runner will go stale after every real fix — that's
  expected maintenance. On a merge conflict in that assertion, re-run the
  harness on the combined branch and paste its real output; never add two
  deltas by hand.
- A CLI arg-parser can reject an input before library code ever sees it.
  Since this port drives the harness via direct library calls, this mostly
  applies to the CLI itself once built — check it doesn't pre-emptively
  reject cases the library handles fine.
- `some_command | tail; echo $?` reports the pipe's last command's exit
  code, not the command being checked. Redirect to a file or use
  `${PIPESTATUS[0]}` before trusting any pass/fail claim through a pipe.
- Use `git worktree` per parallel branch if more than one agent/session
  works this repo concurrently. Never a shared checkout.
- A PR merged after a sibling PR's changes can silently revert the
  sibling's fix on a conflict-free merge. Diff the merged content against
  the target branch's current file state after merging.
- Every skip in the conformance runner needs a cited reason in the source
  itself, not just in a PR description.
- Don't trust a "this gate is just flaky" claim without checking the gate's
  actual current behavior first.
