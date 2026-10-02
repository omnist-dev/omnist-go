package yaml

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- helpers (D-18a carrier rules, D-22 expanded size) ---

var lineColPath = regexp.MustCompile(`^\d+:\d+$`)

// limitsEx builds Limits with the given alias factor and expanded-size cap
// (zero keeps the default for that field).
func limitsEx(maxE, maxSlots int) omnist.Limits {
	l := omnist.DefaultLimits()
	if maxE != 0 {
		l.MaxAliasExpansion = maxE
	}
	if maxSlots != 0 {
		l.MaxExpandedSlots = maxSlots
	}
	return l
}

// wantReject asserts Read rejects text with code at path "$".
func wantReject(t *testing.T, name, text string, l omnist.Limits, code omnist.Code) *omnist.ParseError {
	t.Helper()
	_, err := Read(text, l)
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != code || pe.Path != "$" {
		t.Fatalf("%s: got %#v, want %s at $", name, err, code)
	}
	return pe
}

// wantSyntax asserts Read rejects text as parse.codec-syntax at a line:col path.
func wantSyntax(t *testing.T, name, text string, l omnist.Limits) *omnist.ParseError {
	t.Helper()
	_, err := Read(text, l)
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseCodecSyntax || !lineColPath.MatchString(pe.Path) {
		t.Fatalf("%s: got %#v, want parse.codec-syntax at line:col", name, err)
	}
	return pe
}

func wantAccept(t *testing.T, name, text string, l omnist.Limits) {
	t.Helper()
	if _, err := Read(text, l); err != nil {
		t.Fatalf("%s: unexpected error: %v", name, err)
	}
}

// --- D-18a: the carrier rules ---

func TestCarrierSlotArithmeticSpecExamples(t *testing.T) {
	const pq = "p: &p {k: 1}\nq: &q {j: 2}\n"
	// Anchored carrier: z is 4/3, the same as the unanchored carrier; the
	// anchored sequence is not a candidate and holds no slot.
	wantSlots(t, "anchored carrier", pq+"z: &z {<<: &s [*p, *q], m: 3}\n", map[string][2]uint64{
		"p": {2, 2}, "q": {2, 2}, "z": {4, 3}, "s": {5, 0},
	})
	wantSlots(t, "unanchored carrier", pq+"z: &z {<<: [*p, *q], m: 3}\n", map[string][2]uint64{
		"p": {2, 2}, "q": {2, 2}, "z": {4, 3},
	})
	// An ordinary anchored sequence stays a candidate: W(s) = 1 + 2 + 2 = 5, S = 3.
	// An alias to it in merge position flattens its members: W(y) = 4, not 6.
	wantSlots(t, "ordinary sequence and merge alias", pq+"s: &s [*p, *q]\ny: &y {<<: *s, m: 3}\n", map[string][2]uint64{
		"p": {2, 2}, "q": {2, 2}, "s": {5, 3}, "y": {4, 3},
	})
	// A plain alias to an anchored carrier materializes the list: 1 + W(p) + W(q).
	wantSlots(t, "plain alias to carrier", pq+"z: {<<: &s [*p, *q]}\nt: &t {v: *s}\n", map[string][2]uint64{
		"p": {2, 2}, "q": {2, 2}, "s": {5, 0}, "t": {6, 2},
	})
	// Inline and anchored members of a carrier (spec worked example): W = S = 5.
	wantSlots(t, "carrier with inline and anchored members", "z: &z {<<: [{x: 1}, &m {y: 2}, *m], k: 3}\n", map[string][2]uint64{
		"z": {5, 5}, "m": {2, 2},
	})
	// A merge alias to a carrier-written anchor behaves like one to an ordinary
	// anchored sequence.
	wantSlots(t, "merge alias to anchored carrier", pq+"a: {<<: &s [*p, *q]}\ny: &y {<<: *s, m: 3}\n", map[string][2]uint64{
		"p": {2, 2}, "q": {2, 2}, "s": {5, 0}, "y": {4, 3},
	})
}

// The four vectors of the carrier rules, at the limit (accepted) and one past
// (rejected), through the real reader.
func TestCarrierBoundaries(t *testing.T) {
	const atCarrier = "p: &p {a1: 1, a2: 2, a3: 3}\nq: &q {b1: 4, b2: 5, b3: 6}\nz: {<<: &s [*p, *q], m1: 7, m2: 8, m3: 9}\n"
	const pastCarrier = "p: &p {a1: 1, a2: 2, a3: 3, a4: 4}\nq: &q {b1: 5, b2: 6, b3: 7, b4: 8}\nz: {<<: &s [*p, *q], m1: 9, m2: 10, m3: 11}\n"
	wantAccept(t, "anchored carrier at limit", atCarrier, limitsEx(2, 0))
	_ = wantReject(t, "anchored carrier one past", pastCarrier, limitsEx(2, 0), omnist.CodeDocumentLimitAliasExpansion)
	// The same text without the anchor gives the same verdicts.
	wantAccept(t, "unanchored carrier at limit", strings.Replace(atCarrier, "&s ", "", 1), limitsEx(2, 0))
	_ = wantReject(t, "unanchored carrier one past", strings.Replace(pastCarrier, "&s ", "", 1), limitsEx(2, 0), omnist.CodeDocumentLimitAliasExpansion)

	const atAlias = "p: &p {a: 1}\nq: &q {b: 2}\ns: &s [*p, *q, *p, *q]\nz: {<<: *s, m: 3}\n"
	const pastAlias = "p: &p {a: 1}\nq: &q {b: 2}\ns: &s [*p, *q, *p, *q]\nz: {<<: *s}\n"
	wantAccept(t, "merge alias to sequence at limit", atAlias, limitsEx(2, 0))
	_ = wantReject(t, "merge alias to sequence one past", pastAlias, limitsEx(2, 0), omnist.CodeDocumentLimitAliasExpansion)

	wantAccept(t, "carrier with inline and anchored members", "z: {<<: [{x: 1}, &m {y: 2}, *m], k: 3}\n", limitsEx(1, 0))
	wantAccept(t, "single anchored inline merge source", "z: {<<: &m {y: 2}, k: 3}\n", limitsEx(1, 0))
}

func TestCarrierMergesTheRightEdges(t *testing.T) {
	doc, err := Read("p: &p {a: 1}\nq: &q {b: 2}\nz: {<<: &s [*p, *q], m: 3}\ny: {<<: *s}\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"z", "y"} {
		tg := doc.Node.Edges
		var n *omnist.Node
		for _, e := range tg {
			if e.Label == name {
				n, _ = e.Target.Node()
			}
		}
		if n == nil || n.Edges == nil {
			t.Fatalf("%s: no node", name)
		}
		got := map[string]bool{}
		for _, e := range n.Edges {
			got[e.Label] = true
		}
		if !got["a"] || !got["b"] {
			t.Errorf("%s: merged edges = %v, want a and b", name, got)
		}
	}
}

// --- malformed merge shapes are parse.codec-syntax and win over any limit ---

var malformedMerges = []struct{ name, text string }{
	{"scalar merge value", "a:\n  <<: 1\n"},
	{"scalar member in a carrier", "a:\n  <<: [1]\n"},
	{"sequence of sequences", "a:\n  <<: [[{a: 1}]]\n"},
	{"alias to a sequence of scalars", "s: &s [1, 2]\nz:\n  <<: *s\n"},
	{"alias to a scalar", "s: &s 1\nz:\n  <<: *s\n"},
	{"alias member that is a sequence", "s: &s [{a: 1}]\nz:\n  <<: [*s]\n"},
	{"alias to a sequence containing a sequence", "s: &s [[{a: 1}]]\nz:\n  <<: *s\n"},
	{"mixed members", "p: &p {a: 1}\nz:\n  <<: [*p, 2]\n"},
	{"empty merge sequence", "z:\n  <<: []\n"},
	{"null merge value", "z:\n  <<: ~\n"},
	{"anchored carrier with a scalar", "z:\n  <<: &s [{a: 1}, 2]\n"},
}

func TestMalformedMergeShapesAreCodecSyntax(t *testing.T) {
	for _, tc := range malformedMerges {
		wantSyntax(t, tc.name, tc.text, omnist.DefaultLimits())
	}
}

func TestMalformedMergeWinsOverEveryLimit(t *testing.T) {
	// A bomb first (the spec vector), then a malformed merge.
	const bomb = "p: &p {a: 1, b: 2, c: 3}\nt: {<<: [*p, *p, *p, *p]}\n"
	for _, tc := range malformedMerges {
		wantSyntax(t, "bomb then "+tc.name, bomb+tc.text, limitsEx(2, 0))
	}
	// A size bomb (ratio passes, cap crossed) then a malformed merge.
	const sizeBomb = "base: &base {k1: 1, k2: 2, k3: 3}\nt: {a: *base, b: *base, c: *base, d: *base}\n"
	wantSyntax(t, "size bomb then malformed", sizeBomb+"bad: {<<: 1}\n", limitsEx(0, 21))
	// Malformed first, then a bomb: still the syntax error.
	wantSyntax(t, "malformed then bomb", "bad: {<<: 1}\n"+bomb, limitsEx(2, 0))
	// A self-referential anchor (D-20) next to a malformed merge: syntax wins.
	wantSyntax(t, "cycle and malformed", "a: &a {x: *a}\nbad: {<<: 1}\n", omnist.DefaultLimits())
	// The first malformed merge in document order is the one reported.
	pe := wantSyntax(t, "first of two", "a: {<<: 1}\nb: {<<: [2]}\n", omnist.DefaultLimits())
	if pe.Line != 1 {
		t.Errorf("reported line %d, want 1", pe.Line)
	}
}

func TestValidMergeShapesPass(t *testing.T) {
	for _, text := range []string{
		"p: &p {a: 1}\nz: {<<: *p}\n",
		"p: &p {a: 1}\nz: {<<: [*p, {b: 2}, &m {c: 3}, *m]}\n",
		"p: &p {a: 1}\ns: &s [*p, {b: 2}]\nz: {<<: *s}\n",
		"z: {<<: [{a: 1}]}\n",
	} {
		wantAccept(t, text, text, omnist.DefaultLimits())
	}
}

// --- D-22: the expanded-size cap ---

// size22 has W(root) = 22: base is 4 slots, t is 1 + 4*(1+4).
const size22 = "base: &base {k1: 1, k2: 2, k3: 3}\nt: {a: *base, b: *base, c: *base, d: *base}\n"

func TestExpandedSizeBoundaryIsStrictlyGreaterThan(t *testing.T) {
	wantAccept(t, "at the cap", size22, limitsEx(0, 22))
	pe := wantReject(t, "one past", size22, limitsEx(0, 21), omnist.CodeDocumentLimitExpandedSize)
	if pe.Line != 1 || pe.Col != 1 {
		t.Errorf("position %d:%d, want 1:1", pe.Line, pe.Col)
	}
	if !strings.Contains(pe.Message, "22") || !strings.Contains(pe.Message, "21") {
		t.Errorf("message %q does not name W and the cap", pe.Message)
	}
}

func TestExpandedSizeAndRatioAreIndependent(t *testing.T) {
	// E(t) = 17/5 = 3.4: the ratio passes at 4, the size fails at 21.
	wantReject(t, "size fails where ratio passes", size22, limitsEx(4, 21), omnist.CodeDocumentLimitExpandedSize)
	// The ratio fails at 3, the size passes at 22.
	wantReject(t, "ratio fails where size passes", size22, limitsEx(3, 22), omnist.CodeDocumentLimitAliasExpansion)
	// Both fail: alias-expansion is reported.
	wantReject(t, "both fail", size22, limitsEx(3, 21), omnist.CodeDocumentLimitAliasExpansion)
	// The ratio check runs before the size check even when the offending
	// candidate is the document root itself.
	wantReject(t, "root over both", "b: &b {k1: 1, k2: 2}\nr1: *b\nr2: *b\nr3: *b\nr4: *b\n", limitsEx(1, 5), omnist.CodeDocumentLimitAliasExpansion)
}

func TestExpandedSizeExemptsAliasFreeDocuments(t *testing.T) {
	// 7 keys + root = 8 slots over a cap of 3: no alias, no merge key.
	wantAccept(t, "alias-free", "k1: 1\nk2: 2\nk3: 3\nk4: 4\nk5: 5\nk6: 6\nk7: 7\n", limitsEx(0, 3))
	// An anchor that nothing refers to is not an alias.
	wantAccept(t, "unused anchor", "k1: &x 1\nk2: 2\nk3: 3\nk4: 4\nk5: 5\nk6: 6\nk7: 7\n", limitsEx(0, 3))
	// One harmless alias subjects the whole document to the cap (the cliff).
	wantReject(t, "one harmless alias", "k1: &x 1\nk2: 2\nk3: 3\nk4: 4\nk5: 5\nk6: 6\nk7: 7\nk8: *x\n", limitsEx(0, 3), omnist.CodeDocumentLimitExpandedSize)
	// A merge key alone, with no alias, also does.
	wantReject(t, "merge key without alias", "t: {<<: {a: 1}}\n", limitsEx(0, 2), omnist.CodeDocumentLimitExpandedSize)
	wantAccept(t, "merge key at cap", "t: {<<: {a: 1}}\n", limitsEx(0, 3))
	// An alias in a mapping key position counts as an alias too.
	wantReject(t, "alias as a key", "&x k: 1\n*x : 2\nm: 3\nn: 4\n", limitsEx(0, 2), omnist.CodeDocumentLimitExpandedSize)
}

func TestExpandedSizeOnTopLevelShapes(t *testing.T) {
	// A top-level scalar and an alias-free sequence never reach the cap; a
	// top-level sequence with an alias is measured, then refused for its shape.
	wantAccept(t, "scalar", "5\n", limitsEx(0, 1))
	_, err := Read("- &a {k: 1}\n- *a\n- *a\n", limitsEx(0, 1))
	if pe, ok := err.(*omnist.ParseError); !ok || pe.Code != omnist.CodeDocumentLimitExpandedSize {
		t.Fatalf("got %#v, want expanded-size", err)
	}
}

func TestExpandedSizeOptionDefaultsAndValidation(t *testing.T) {
	// Zero and negative select the default: a 22-slot document passes at both,
	// a document over 1,000,000 slots is refused at both.
	big := bigAliasText(40_000)
	for _, v := range []int{0, -1, -1000} {
		l := omnist.DefaultLimits()
		l.MaxExpandedSlots = v
		wantAccept(t, fmt.Sprintf("small at %d", v), size22, l)
		wantReject(t, fmt.Sprintf("big at %d", v), big, l, omnist.CodeDocumentLimitExpandedSize)
	}
	// A Limits literal written before the field existed keeps the cap.
	legacy := omnist.Limits{MaxDepth: 200, MaxNodes: 1_000_000, MaxIntDigits: 4300}
	wantReject(t, "legacy literal", big, legacy, omnist.CodeDocumentLimitExpandedSize)
	// A raised cap admits it to the check (only the check is run: reading it
	// would materialize two million slots).
	if err := checkAliasExpansion(parseRoot(t, big), omnist.DefaultMaxAliasExpansion, omnist.MaxRecommendedExpandedSlots); err != nil {
		t.Fatalf("raised cap: %v", err)
	}
}

// bigAliasText is n containers each holding one alias of a 49-slot block:
// each container is 1 + 49 = 50 slots from 2 written (E = 25, under 50), so
// the ratio passes and only the size cap can see W(root) = about 50n.
func bigAliasText(n int) string {
	var b strings.Builder
	b.WriteString("block: &block {")
	for i := 0; i < 48; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "k%d: %d", i, i)
	}
	b.WriteString("}\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "c%d: {x: *block}\n", i)
	}
	return b.String()
}

// The full-size memory bomb: 40,000 containers, W(root) about 2,000,000,
// every E = 25. Accepted by D-18, it must be refused by D-22 at the default
// before anything is materialized; only the check's time is bounded here.
func TestExpandedSizeMemoryBombRejectedAtDefault(t *testing.T) {
	text := bigAliasText(40_000)
	root := parseRoot(t, text)
	start := time.Now()
	err := checkAliasExpansion(root, omnist.DefaultMaxAliasExpansion, omnist.DefaultMaxExpandedSlots)
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("check took %v, want a linear pass", took)
	}
	if err == nil || err.Code != omnist.CodeDocumentLimitExpandedSize || err.Path != "$" {
		t.Fatalf("got %#v, want document.limit.expanded-size at $", err)
	}
	// The same text through Read is refused the same way.
	wantReject(t, "Read", text, omnist.DefaultLimits(), omnist.CodeDocumentLimitExpandedSize)
	// And its ratio really is under the limit: with the cap lifted it reads.
	l := omnist.DefaultLimits()
	l.MaxExpandedSlots = 3_000_000
	l.MaxNodes = 3_000_000
	if err := checkAliasExpansion(root, omnist.DefaultMaxAliasExpansion, 3_000_000); err != nil {
		t.Errorf("ratio unexpectedly rejected the bomb: %v", err)
	}
}

// Saturation: W far past 2^64 from a 2x70 fan-out is rejected, not wrapped,
// both directly (any factor limit) and through the reader at the defaults.
func TestExpandedSizeSaturationIsRejectedNotWrapped(t *testing.T) {
	text := chainText(70, 2)
	if _, err := analyzeAliases(parseRoot(t, text), math.MaxInt, math.MaxInt); err == nil {
		t.Error("a 2x70 fan-out was accepted: W wrapped under the limits")
	}
	pe := wantReject(t, "Read", text, omnist.DefaultLimits(), omnist.CodeDocumentLimitAliasExpansion)
	if pe.Line < 1 {
		t.Errorf("no position on %#v", pe)
	}
	// The same fan-out in a document whose factor limit is lifted still stops
	// at the size cap through the saturated root.
	if _, err := analyzeAliases(parseRoot(t, text), math.MaxInt, 1000); err == nil {
		t.Error("saturated W passed a 1000-slot cap")
	}
}

// Compose-style documents the default must never refuse (spec §2.4.1 measured
// W(root) of 2,223 / 6,263 / 62,063): pinned exactly, then accepted.
func composeBlockText(services, keys int) string {
	var b strings.Builder
	b.WriteString("x-base: &base {")
	for i := 0; i < keys; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "k%d: %d", i, i)
	}
	b.WriteString("}\nservices:\n")
	for i := 0; i < services; i++ {
		fmt.Fprintf(&b, "  s%d: {<<: *base, image: i%d}\n", i, i)
	}
	return b.String()
}

func TestExpandedSizeComposeStyleFalsePositiveGuards(t *testing.T) {
	for _, tc := range []struct{ services, keys, w int }{
		{100, 20, 2223}, {100, 60, 6263}, {1000, 60, 62063},
	} {
		text := composeBlockText(tc.services, tc.keys)
		wantAccept(t, "default", text, omnist.DefaultLimits())
		// W(root) is exactly tc.w: accepted at the cap, refused one below.
		wantAccept(t, "at W", text, limitsEx(0, tc.w))
		wantReject(t, "below W", text, limitsEx(0, tc.w-1), omnist.CodeDocumentLimitExpandedSize)
	}
	// A GitLab-style file of 200 jobs, and a Kubernetes-style one of 500 objects.
	var b strings.Builder
	b.WriteString(".defaults: &d {image: ruby, stage: test, retry: 2, tags: [a, b]}\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "job%d: {<<: *d, script: [run%d]}\n", i, i)
	}
	wantAccept(t, "gitlab", b.String(), omnist.DefaultLimits())
}

// A 100k-level alias chain is safe: the walk is iterative, and the reader's
// own depth limit bounds what it materializes.
func TestExpandedSizeLongAliasChainsAreSafe(t *testing.T) {
	const n = 100_000
	var b strings.Builder
	b.WriteString("a0: &a0 {k: 1}\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "a%d: &a%d {<<: *a%d}\n", i, i, i-1)
	}
	// Through Read: refused by the merge-depth limit (a document.limit code),
	// never a crash or a hang.
	_, err := Read(b.String(), omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Fatalf("got %#v, want document.limit.depth", err)
	}
	// A merge-by-sequence chain, each link a carrier of the previous.
	var c strings.Builder
	c.WriteString("a0: &a0 {k: 1}\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&c, "a%d: &a%d {<<: [*a%d]}\n", i, i, i-1)
	}
	if err := checkAliasExpansion(parseRoot(t, c.String()), 50, 10_000_000); err != nil {
		t.Fatalf("carrier chain: %v", err)
	}
}

// All the YAML entry points still apply the rules: the reader here, and the
// command line and conformance runner call exactly this function (no second
// path parses YAML).
func TestExpandedSizeAppliesThroughRead(t *testing.T) {
	wantReject(t, "Read", size22, limitsEx(0, 21), omnist.CodeDocumentLimitExpandedSize)
	wantSyntax(t, "Read", "a: {<<: 1}\n", omnist.DefaultLimits())
}

// A container as a mapping key is measured on its own subtree but never counted
// into its parent (the reader refuses it as an unlabeled element).
func TestExpandedSizeContainerKeysFoldIntoNothing(t *testing.T) {
	const text = "p: &p {a: 1}\n? [*p, *p]\n: v\n"
	if _, err := analyzeAliases(parseRoot(t, text), 50, 1000); err != nil {
		t.Fatalf("analysis: %v", err)
	}
	_, err := Read(text, omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentUnlabeledElement {
		t.Fatalf("got %#v, want document.unlabeled-element", err)
	}
}
