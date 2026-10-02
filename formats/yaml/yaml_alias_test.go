package yaml

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	yamllib "gopkg.in/yaml.v3"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- helpers ---

// parseRoot decodes text into the root content node, as Read does.
func parseRoot(t *testing.T, text string) *yamllib.Node {
	t.Helper()
	var doc yamllib.Node
	if err := yamllib.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("yaml parse: %v", err)
	}
	return doc.Content[0]
}

// slotsByAnchor runs the analysis with a limit too large to trip and returns
// each anchor name's (W, S).
func slotsByAnchor(t *testing.T, text string) map[string][2]uint64 {
	t.Helper()
	table, err := analyzeAliases(parseRoot(t, text), math.MaxInt, math.MaxInt)
	if err != nil {
		t.Fatalf("analyzeAliases(%q): %v", text, err)
	}
	out := map[string][2]uint64{}
	for n, v := range table {
		out[n.Anchor] = [2]uint64{v.w, v.s}
	}
	return out
}

func wantSlots(t *testing.T, name, text string, want map[string][2]uint64) {
	t.Helper()
	got := slotsByAnchor(t, text)
	if len(got) != len(want) {
		t.Errorf("%s: got %d anchors %v, want %d %v", name, len(got), got, len(want), want)
	}
	for a, w := range want {
		if got[a] != w {
			t.Errorf("%s: anchor %s: (W,S) = %v, want %v", name, a, got[a], w)
		}
	}
}

func aliasErrOf(t *testing.T, text string, limits omnist.Limits) *omnist.ParseError {
	t.Helper()
	_, err := Read(text, limits)
	if err == nil {
		t.Fatalf("Read(%.60q): want a document.limit.alias-expansion error, got none", text)
	}
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitAliasExpansion || pe.Path != "$" {
		t.Fatalf("Read(%.60q): got %#v, want document.limit.alias-expansion at $", text, err)
	}
	return pe
}

func factor(w, s uint64) float64 { return float64(w) / float64(s) }

// --- W/S/E arithmetic on small hand-computed graphs ---

func TestAliasSlotArithmetic(t *testing.T) {
	// Plain alias: W(b) = 1 + W(a) + W(a); S(b) counts each alias as one slot.
	wantSlots(t, "plain", "a: &a {k: 1}\nb: &b {x: *a, y: *a}\n", map[string][2]uint64{
		"a": {2, 2}, "b": {5, 3},
	})
	// The spec's nested merge worked example (§2.4.1).
	wantSlots(t, "nested merge", "p: &p {k: 1}\nq: &q {<<: *p, m: 2}\nr: &r {n: *q}\n", map[string][2]uint64{
		"p": {2, 2}, "q": {3, 3}, "r": {4, 2},
	})
	// The spec's merge-sequence worked example: W(z)=4, S(z)=3, E=1.33.
	wantSlots(t, "merge sequence", "p: &p {k: 1}\nq: &q {j: 2}\nz: &z {<<: [*p, *q], m: 3}\n", map[string][2]uint64{
		"p": {2, 2}, "q": {2, 2}, "z": {4, 3},
	})
	// Key override: W is a conservative bound, blind to collisions (D-19).
	wantSlots(t, "collision", "p: &p {k: 1}\nq: &q {<<: *p, k: 9}\n", map[string][2]uint64{
		"p": {2, 2}, "q": {3, 3},
	})
	// A repeated alias in one merge sequence counts once per occurrence in W
	// (the bound), once in S (one `<<` entry).
	wantSlots(t, "repeated merge alias", "p: &p {k: 1}\nz: &z {<<: [*p, *p]}\n", map[string][2]uint64{
		"p": {2, 2}, "z": {3, 2},
	})
	// A scalar anchor is one slot in both, E = 1.
	wantSlots(t, "scalar", "s: &s 5\nt: &t [*s, *s, *s]\n", map[string][2]uint64{
		"s": {1, 1}, "t": {4, 4},
	})
	// An anchor defined inside another anchor counts in both, as written.
	wantSlots(t, "nested anchor", "a: &a {x: &b [1, 2]}\n", map[string][2]uint64{
		"b": {3, 3}, "a": {4, 4},
	})
	// An anchor on a mapping key is a scalar anchor.
	wantSlots(t, "anchored key", "&k key: 1\nv: *k\n", map[string][2]uint64{"k": {1, 1}})
	// A merge whose value is an alias of a sequence of aliases: the alias
	// contributes W(s)-1 (a conservative bound), not its members' flattening.
	wantSlots(t, "merge alias to sequence", "p: &p {k: 1}\ns: &s [*p]\nz: &z {<<: *s}\n", map[string][2]uint64{
		"p": {2, 2}, "s": {3, 2}, "z": {2, 2},
	})
	// An inline mapping merged in place (not covered by the spec text): its
	// container is flattened (w-1) and the slots it writes count in S.
	wantSlots(t, "inline merge", "z: &z {<<: {k: 1}, m: 2}\n", map[string][2]uint64{"z": {3, 4}})
	wantSlots(t, "inline merge sequence", "p: &p {k: 1}\nz: &z {<<: [{j: 1}, *p]}\n", map[string][2]uint64{
		"p": {2, 2}, "z": {3, 3},
	})
	// An anchored literal sequence under a merge key is a real (not carrier)
	// sequence: it keeps its own W and S, and merges as a value.
	wantSlots(t, "anchored merge sequence", "p: &p {k: 1}\nz: &z {<<: &s [*p]}\n", map[string][2]uint64{
		"p": {2, 2}, "s": {3, 0}, "z": {2, 2},
	})
	// The spec's nested fan-out vector: E(e) = 341/5.
	wantSlots(t, "fan-out", "a: &a leaf\nb: &b {p: *a, q: *a, r: *a, s: *a}\nc: &c {p: *b, q: *b, r: *b, s: *b}\nd: &d {p: *c, q: *c, r: *c, s: *c}\ne: &e {p: *d, q: *d, r: *d, s: *d}\ntop: *e\n", map[string][2]uint64{
		"a": {1, 1}, "b": {5, 5}, "c": {21, 5}, "d": {85, 5}, "e": {341, 5},
	})
	// Aliases in key position add nothing, and an alias to a scalar anchor as
	// a key is fine.
	wantSlots(t, "alias key", "s: &s k\nm: &m {*s : 1}\n", map[string][2]uint64{"s": {1, 1}, "m": {2, 2}})
	// A complex key's anchors are registered but its slots are not the
	// parent's.
	wantSlots(t, "complex key", "? &c [1, 2]\n: v\nm: &m {k: *c}\n", map[string][2]uint64{"c": {3, 3}, "m": {4, 2}})
}

func TestAliasBoundaryExactAndOnePast(t *testing.T) {
	// E(y) = 4/2 = 2.00 exactly: accepted at a limit of 2, rejected at 1.
	const text = "x: &x {k: 1, m: 2}\ny: &y {a: *x}\n"
	if got := slotsByAnchor(t, text)["y"]; factor(got[0], got[1]) != 2.0 {
		t.Fatalf("test input: E(y) = %v, want 2.00", factor(got[0], got[1]))
	}
	for _, tc := range []struct {
		limit  int
		reject bool
	}{{2, false}, {1, true}, {3, false}} {
		l := omnist.DefaultLimits()
		l.MaxAliasExpansion = tc.limit
		_, err := Read(text, l)
		if tc.reject {
			_ = aliasErrOf(t, text, l)
		} else if err != nil {
			t.Errorf("limit %d: %v, want accepted", tc.limit, err)
		}
	}
	// 21/5 = 4.20: accepted at 5, rejected at 4 (another pair of limits).
	const chain = "a: &a leaf\nb: &b {p: *a, q: *a, r: *a, s: *a}\nc: &c {p: *b, q: *b, r: *b, s: *b}\n"
	l := omnist.DefaultLimits()
	l.MaxAliasExpansion = 5
	if _, err := Read(chain, l); err != nil {
		t.Errorf("limit 5: %v, want accepted", err)
	}
	l.MaxAliasExpansion = 4
	_ = aliasErrOf(t, chain, l)
}

func TestAliasRejectionNamesTheFirstOffendingAnchor(t *testing.T) {
	// c (line 3) is the first anchor over a limit of 4; d and e are worse but
	// never reached.
	l := omnist.DefaultLimits()
	l.MaxAliasExpansion = 4
	pe := aliasErrOf(t, "a: &a leaf\nb: &b {p: *a, q: *a, r: *a, s: *a}\nc: &c {p: *b, q: *b, r: *b, s: *b}\nd: &d {p: *c, q: *c, r: *c, s: *c}\n", l)
	if pe.Line != 3 {
		t.Errorf("error at line %d, want 3 (the first anchor over the limit)", pe.Line)
	}
}

// --- saturation ---

func TestAliasSaturatingHelpers(t *testing.T) {
	if satAdd(math.MaxUint64, 1) != math.MaxUint64 || satAdd(math.MaxUint64-1, 1) != math.MaxUint64 || satAdd(2, 3) != 5 {
		t.Error("satAdd")
	}
	if satMul(math.MaxUint64, 2) != math.MaxUint64 || satMul(1<<32, 1<<32) != math.MaxUint64 || satMul(6, 7) != 42 {
		t.Error("satMul")
	}
	if sub1(0) != 0 || sub1(1) != 0 || sub1(9) != 8 {
		t.Error("sub1")
	}
}

// chainText builds a0: &a0 x, then ai: &ai [*a(i-1) repeated fan times].
func chainText(levels, fan int) string {
	var b strings.Builder
	b.WriteString("a0: &a0 x\n")
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&b, "a%d: &a%d [", i, i)
		for j := 0; j < fan; j++ {
			if j > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "*a%d", i-1)
		}
		b.WriteString("]\n")
	}
	return b.String()
}

func TestAliasTrueWExceedingUint64IsRejectedNotWrapped(t *testing.T) {
	// fan 10 over 25 levels: true W(a25) is about 1.1e25, far past 2^64. With
	// the limit as loose as an int allows, only saturation can notice: a wrapped
	// count could land under the limit and accept the attack.
	text := chainText(25, 10)
	if _, err := analyzeAliases(parseRoot(t, text), math.MaxInt, math.MaxInt); err == nil {
		t.Fatal("a chain whose W exceeds uint64 was accepted: W wrapped or was compared saturated-equal")
	} else if err.Code != omnist.CodeDocumentLimitAliasExpansion {
		t.Fatalf("code = %s", err.Code)
	}
	l := omnist.DefaultLimits()
	l.MaxAliasExpansion = math.MaxInt
	_ = aliasErrOf(t, text, l)
	// A saturated W is rejected even where max*S itself saturates.
	if _, err := analyzeAliases(parseRoot(t, chainText(25, 10)), math.MaxInt, math.MaxInt); err == nil {
		t.Error("saturated W accepted")
	}
	// The same shape one order of magnitude under the wrap is an ordinary
	// rejection at the default limit.
	_ = aliasErrOf(t, chainText(8, 10), omnist.DefaultLimits())
}

// --- cycles (D-20) ---

func TestAliasCyclesAreRejected(t *testing.T) {
	for name, text := range map[string]string{
		"direct mapping":       "a: &a {x: *a}\n",
		"direct sequence":      "a: &a [*a]\n",
		"two anchors":          "a: &a {b: &b {c: *a}}\n",
		"inner self":           "a: &a {b: &b {c: *b}}\n",
		"via merge":            "a: &a {<<: *a, k: 1}\n",
		"via merge sequence":   "p: &p {k: 1}\na: &a {<<: [*p, *a]}\n",
		"through nested merge": "a: &a {b: &b {<<: *a}}\n",
		"alias key":            "a: &a {*a : 1}\n",
		"deep sequence":        "a: &a [[[[[*a]]]]]\n",
	} {
		pe := aliasErrOf(t, text, omnist.DefaultLimits())
		if !strings.Contains(pe.Message, "itself") {
			t.Errorf("%s: message %q does not name the self-reference", name, pe.Message)
		}
		// A cycle is not reported as any other limit, however loose the knobs.
		l := omnist.DefaultLimits()
		l.MaxAliasExpansion = math.MaxInt
		l.MaxDepth, l.MaxNodes = 1_000_000, 1_000_000
		_ = aliasErrOf(t, text, l)
	}
}

func TestAliasDeepNestingCycleDoesNotRecurse(t *testing.T) {
	// 5000 levels of nesting (yaml.v3 refuses 10000), the innermost entry an
	// alias of the outermost anchor.
	const depth = 5000
	text := "a0: &a0 " + strings.Repeat("{k: ", depth) + "*a0" + strings.Repeat("}", depth) + "\n"
	l := omnist.DefaultLimits()
	l.MaxDepth = 10_000
	_ = aliasErrOf(t, text, l)
}

func TestAlias100kDeepAnchorChainIsLinearAndIterative(t *testing.T) {
	// 100,000 anchors, each merging the previous: every E is 1.00, W stays 2.
	const n = 100_000
	var b strings.Builder
	b.WriteString("a0: &a0 {k: 1}\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "a%d: &a%d {<<: *a%d}\n", i, i, i-1)
	}
	table, err := analyzeAliases(parseRoot(t, b.String()), omnist.DefaultMaxAliasExpansion, omnist.DefaultMaxExpandedSlots)
	if err != nil {
		t.Fatalf("analysis rejected a factor-1 chain: %v", err)
	}
	if len(table) != n+1 {
		t.Fatalf("%d anchors analysed, want %d", len(table), n+1)
	}
	for node, v := range table {
		if v.w != 2 || v.s != 2 {
			t.Fatalf("anchor %s: (W,S) = (%d,%d), want (2,2)", node.Anchor, v.w, v.s)
		}
	}
	// And a 100k plain-alias chain: W grows by one per link, S stays 2, so the
	// factor crosses 50 near link 100 and the walk stops there, not at 100k.
	var c strings.Builder
	c.WriteString("a0: &a0 x\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&c, "a%d: &a%d [*a%d]\n", i, i, i-1)
	}
	pe := aliasErrOf(t, c.String(), omnist.DefaultLimits())
	if pe.Line > 300 {
		t.Errorf("rejected at line %d, want early (around link 100)", pe.Line)
	}
}

// --- the configuration option ---

func TestAliasOptionDefaultCustomInvalid(t *testing.T) {
	const text = "a: &a leaf\nb: &b {p: *a, q: *a, r: *a, s: *a}\nc: &c {p: *b, q: *b, r: *b, s: *b}\nd: &d {p: *c, q: *c, r: *c, s: *c}\ne: &e {p: *d, q: *d, r: *d, s: *d}\n"
	if omnist.DefaultLimits().MaxAliasExpansion != 50 || omnist.DefaultMaxAliasExpansion != 50 {
		t.Fatal("default is not 50")
	}
	// Default: E(e) = 68.2 rejected.
	_ = aliasErrOf(t, text, omnist.DefaultLimits())
	// Custom: raised to 100 it is accepted; lowered to 17 rejects d (17.00 is
	// fine, 68.2 is not) and to 16 rejects d itself.
	l := omnist.DefaultLimits()
	l.MaxAliasExpansion = 100
	if _, err := Read(text, l); err != nil {
		t.Errorf("limit 100: %v", err)
	}
	// Zero (a pre-existing Limits literal) and negatives fall back to the
	// finite default and never mean unbounded.
	for _, v := range []int{0, -1, math.MinInt} {
		l := omnist.Limits{MaxDepth: 200, MaxNodes: 1_000_000, MaxIntDigits: 4300, MaxAliasExpansion: v}
		if got := l.EffectiveMaxAliasExpansion(); got != 50 {
			t.Errorf("Effective(%d) = %d, want 50", v, got)
		}
		_ = aliasErrOf(t, text, l)
	}
}

// Limits literals written before the field existed keep working: legitimate
// input is accepted, the bomb is refused.
func TestAliasLegacyLimitsLiteral(t *testing.T) {
	legacy := omnist.Limits{MaxDepth: 200, MaxNodes: 1_000_000, MaxIntDigits: 4300}
	if _, err := Read(composeText(100), legacy); err != nil {
		t.Errorf("compose config with a legacy Limits literal: %v", err)
	}
	_ = aliasErrOf(t, fanText(4, 10), legacy)
}

// --- legitimate input and attack shapes ---

func composeText(refs int) string {
	var b strings.Builder
	b.WriteString("defaults: &defaults\n  adapter: postgres\n  host: localhost\n  port: 5432\n")
	for i := 0; i < refs; i++ {
		fmt.Fprintf(&b, "svc%d:\n  <<: *defaults\n  database: db%d\n", i, i)
	}
	return b.String()
}

func TestAliasLegitimateInputIsAccepted(t *testing.T) {
	for _, refs := range []int{100, 500} {
		text := composeText(refs)
		if _, err := Read(text, omnist.DefaultLimits()); err != nil {
			t.Fatalf("compose %d: %v", refs, err)
		}
		if got := slotsByAnchor(t, text)["defaults"]; factor(got[0], got[1]) != 1.0 {
			t.Errorf("compose %d: E(defaults) = %v, want 1.00", refs, factor(got[0], got[1]))
		}
	}
	// A scalar constant aliased 500 times, in an anchored list (E = 1.00) and
	// an unanchored one.
	items := strings.TrimSuffix(strings.Repeat("*c, ", 500), ", ")
	for _, text := range []string{"c: &c 5\nl: &l [" + items + "]\n", "c: &c 5\nl: [" + items + "]\n"} {
		if _, err := Read(text, omnist.DefaultLimits()); err != nil {
			t.Errorf("scalar constant x500: %v", err)
		}
	}
	// A depth-3 anchor chain: E of a few, well under 50.
	const chain = "a: &a {x: 1, y: 2}\nb: &b {p: *a, q: *a}\nc: &c {p: *b, q: *b}\nd: &d {p: *c, q: *c}\n"
	if _, err := Read(chain, omnist.DefaultLimits()); err != nil {
		t.Errorf("depth-3 chain: %v", err)
	}
	if got := slotsByAnchor(t, chain)["c"]; factor(got[0], got[1]) > 6 {
		t.Errorf("depth-3 chain: E(c) = %v, want a few", factor(got[0], got[1]))
	}
}

// fanText is the nested-anchor fan-out bomb: levels definitions, each naming
// the previous one branch times.
func fanText(branch, levels int) string {
	var b strings.Builder
	b.WriteString("a0: &a0 leaf\n")
	for l := 1; l <= levels; l++ {
		fmt.Fprintf(&b, "a%d: &a%d {", l, l)
		for i := 0; i < branch; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "k%d: *a%d", i, l-1)
		}
		b.WriteString("}\n")
	}
	fmt.Fprintf(&b, "top: *a%d\n", levels)
	return b.String()
}

// scalarHeavyText anchors a 300-item list, then references it from each of
// levels nested anchors (three times each).
func scalarHeavyText(levels int) string {
	var b strings.Builder
	b.WriteString("a0: &a0 [")
	for i := 0; i < 300; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d", i)
	}
	b.WriteString("]\n")
	for l := 1; l <= levels; l++ {
		fmt.Fprintf(&b, "a%d: &a%d {x: *a%d, y: *a%d, z: *a%d}\n", l, l, l-1, l-1, l-1)
	}
	return b.String()
}

func TestAliasAttackShapesAreRejectedFastAndBeforeExpansion(t *testing.T) {
	cases := map[string]string{
		"fan 4x10":        fanText(4, 10),
		"fan 10x6":        fanText(10, 6),
		"fan 10x7":        fanText(10, 7),
		"fan 10x9":        fanText(10, 9),
		"scalar-heavy x5": scalarHeavyText(5),
		"scalar-heavy x9": scalarHeavyText(9),
		"repeated merges": "p: &p {k: 1}\nq: &q {<<: [" + strings.TrimSuffix(strings.Repeat("*p, ", 200), ", ") + "]}\n",
	}
	for name, text := range cases {
		start := time.Now()
		_, err := Read(text, omnist.DefaultLimits())
		elapsed := time.Since(start)
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeDocumentLimitAliasExpansion || pe.Path != "$" {
			t.Errorf("%s: got %#v, want document.limit.alias-expansion at $ (never document.limit.nodes)", name, err)
		}
		// Expansion takes seconds (the nodes limit only trips after ~1M
		// nodes); the pre-expansion check is milliseconds. The bound is
		// generous so a loaded -race run cannot flake it.
		if elapsed > time.Second {
			t.Errorf("%s: took %v, want a pre-expansion rejection in milliseconds", name, elapsed)
		}
	}
}

// --- property: W never underestimates what is materialized ---

type aliasGen struct {
	r     *rand.Rand
	keys  int
	b     strings.Builder
	repea bool // allow the same alias twice in one merge sequence
	multi bool // a multi-source merge was written (sources can overlap)
}

func (g *aliasGen) key() string { g.keys++; return fmt.Sprintf("k%d", g.keys) }

// member writes one mapping body (without braces) at indent; nested anchored
// mappings it defines are queued in pending for later definitions to use.
func (g *aliasGen) body(indent string, avail []string, pending *[]string) {
	if len(avail) > 0 && g.r.Intn(3) > 0 {
		pick := func() string { return avail[g.r.Intn(len(avail))] }
		if g.r.Intn(2) == 0 {
			fmt.Fprintf(&g.b, "%s<<: *%s\n", indent, pick())
		} else {
			g.multi = true
			n := 2 + g.r.Intn(2)
			seen := map[string]bool{}
			var refs []string
			for len(refs) < n {
				p := pick()
				if !g.repea && seen[p] {
					if len(seen) >= len(avail) {
						break
					}
					continue
				}
				seen[p] = true
				refs = append(refs, "*"+p)
			}
			fmt.Fprintf(&g.b, "%s<<: [%s]\n", indent, strings.Join(refs, ", "))
		}
	}
	for i, n := 0, 1+g.r.Intn(3); i < n; i++ {
		switch c := g.r.Intn(5); {
		case c == 0 && len(avail) > 0:
			fmt.Fprintf(&g.b, "%s%s: *%s\n", indent, g.key(), avail[g.r.Intn(len(avail))])
		case c == 1:
			name := fmt.Sprintf("n%d", g.keys+1)
			fmt.Fprintf(&g.b, "%s%s: &%s\n%s  %s: 1\n", indent, g.key(), name, indent, g.key())
			*pending = append(*pending, name)
		default:
			fmt.Fprintf(&g.b, "%s%s: 1\n", indent, g.key())
		}
	}
}

// slotsUnder counts the materialized value slots beneath an edge target.
func slotsUnder(tg omnist.Target) uint64 {
	n, ok := tg.Node()
	if !ok {
		return 1
	}
	total := uint64(1)
	for _, e := range n.Edges {
		total += slotsUnder(e.Target)
	}
	return total
}

func TestAliasBoundNeverUnderestimatesMaterialization(t *testing.T) {
	const maxE = 3
	accepted, rejected := 0, 0
	for seed := int64(0); seed < 400; seed++ {
		g := &aliasGen{r: rand.New(rand.NewSource(seed)), repea: seed%2 == 1}
		var avail []string
		anchors := 2 + g.r.Intn(6)
		for i := 0; i < anchors; i++ {
			name := fmt.Sprintf("a%d", i)
			var pending []string
			fmt.Fprintf(&g.b, "d%d: &%s\n", i, name)
			g.body("  ", avail, &pending)
			avail = append(avail, name)
			avail = append(avail, pending...)
		}
		use := avail[g.r.Intn(len(avail))]
		fmt.Fprintf(&g.b, "use: *%s\n", use)
		text := g.b.String()

		// Static W/S with no limit in the way.
		table, aerr := analyzeAliases(parseRoot(t, text), math.MaxInt, math.MaxInt)
		if aerr != nil {
			t.Fatalf("seed %d: %v\n%s", seed, aerr, text)
		}
		var wUse, sUse uint64
		worst := 0.0
		for n, v := range table {
			if n.Anchor == use {
				wUse, sUse = v.w, v.s
			}
			worst = math.Max(worst, factor(v.w, v.s))
		}
		// With limit maxE the reader rejects exactly when some E exceeds it.
		l := omnist.DefaultLimits()
		l.MaxAliasExpansion = maxE
		l.MaxNodes = 10_000_000
		if wUse > 20_000 {
			// Keep the materializing half of the property cheap.
			l.MaxNodes = 1_000_000
		}
		if (worst > maxE) != (checkAliasExpansion(parseRoot(t, text), maxE, omnist.DefaultMaxExpandedSlots) != nil) {
			t.Fatalf("seed %d: worst E = %v but the limit-%d check disagrees\n%s", seed, worst, maxE, text)
		}
		if worst > maxE {
			rejected++
			_ = aliasErrOf(t, text, l)
			continue
		}
		accepted++
		doc, err := Read(text, l)
		if err != nil {
			t.Fatalf("seed %d: accepted by the check but Read failed: %v\n%s", seed, err, text)
		}
		var got uint64
		for _, e := range doc.Node.Edges {
			if e.Label == "use" {
				got = slotsUnder(e.Target)
			}
		}
		if got > wUse {
			t.Fatalf("seed %d: anchor %s materialized %d slots, static W = %d (underestimate)\n%s", seed, use, got, wUse, text)
		}
		if got > uint64(maxE)*sUse {
			t.Fatalf("seed %d: accepted anchor %s materialized %d > max*S = %d\n%s", seed, use, got, uint64(maxE)*sUse, text)
		}
		if !g.repea && !g.multi && got != wUse {
			t.Fatalf("seed %d: with unique keys and no multi-source merge W must be exact: materialized %d, W %d\n%s", seed, got, wUse, text)
		}
	}
	if accepted < 50 || rejected < 50 {
		t.Errorf("generator is unbalanced: %d accepted, %d rejected", accepted, rejected)
	}
}

// The merge-depth guard stays as a second line of defence: a long but
// acyclic merge chain (which the alias check accepts at E = 1.00) is still
// bounded by the depth limit.
func TestLongMergeChainIsStillBoundedByTheDepthLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("a0: &a0 {k: 1}\n")
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&b, "a%d: &a%d {<<: *a%d}\n", i, i, i-1)
	}
	_, err := Read(b.String(), omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("got %#v, want document.limit.depth", err)
	}
}

// The node cap is also still enforced for an expansion the alias check admits
// (here an unanchored document root referencing one anchor many times).
func TestNodeCapStillBoundsAdmittedExpansion(t *testing.T) {
	var b strings.Builder
	b.WriteString("a: &a {k: 1}\nl: {")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "m%d: *a, ", i)
	}
	b.WriteString("z: 1}\n")
	text := b.String()
	l := omnist.DefaultLimits()
	l.MaxNodes = 10
	_, err := Read(text, l)
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitNodes {
		t.Errorf("got %#v, want document.limit.nodes", err)
	}
}

// --- v0.25.0-beta: every mapping and sequence is a candidate (spec §2.4.1) ---

// blockText is "b: &b {k0: 0, ..., k<n-1>: n-1}\n", an anchored n-scalar block.
func blockText(n int) string {
	var b strings.Builder
	b.WriteString("b: &b {")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "k%d: %d", i, i)
	}
	b.WriteString("}\n")
	return b.String()
}

// mergeFanIn is an n-key anchored block plus an UNANCHORED mapping merging it
// m times.
func mergeFanIn(n, m int) string {
	return blockText(n) + "t: {<<: [" + strings.TrimSuffix(strings.Repeat("*b, ", m), ", ") + "]}\n"
}

// aliasKeys is n root entries "a<i>: *b".
func aliasKeys(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "a%d: *b\n", i)
	}
	return b.String()
}

func limitsWith(max int) omnist.Limits {
	l := omnist.DefaultLimits()
	l.MaxAliasExpansion = max
	return l
}

func readOK(t *testing.T, name, text string, l omnist.Limits) {
	t.Helper()
	if _, err := Read(text, l); err != nil {
		t.Errorf("%s: %v, want accepted", name, err)
	}
}

func TestAliasUnanchoredContainersAreCandidates(t *testing.T) {
	// The spec's worked unanchored case: W(t) = 13, S(t) = 2, E(t) = 6.50.
	const spec = "b: &b {k1: 1, k2: 2, k3: 3}\nt: {<<: [*b, *b, *b, *b]}\n"
	readOK(t, "limit 7", spec, limitsWith(7))
	pe := aliasErrOf(t, spec, limitsWith(6))
	if pe.Line != 2 {
		t.Errorf("rejected at line %d, want 2 (the unanchored t)", pe.Line)
	}
	// Sequence of aliasing mappings, unanchored: each {k: *b} has E = 51/2.
	list := blockText(50) + "l: [{k: *b}, {k: *b}]\n"
	readOK(t, "list at 26", list, limitsWith(26))
	_ = aliasErrOf(t, list, limitsWith(25))
	// Root-only: a root mapping whose unanchored items are each fine but
	// whose total is over: 100 plain aliases of a 100-scalar block.
	_ = aliasErrOf(t, blockText(100)+aliasKeys(100), omnist.DefaultLimits())
	// A root SEQUENCE is a candidate too.
	seq := "- &b [" + strings.TrimSuffix(strings.Repeat("1, ", 100), ", ") + "]\n" + strings.Repeat("- *b\n", 100)
	_ = aliasErrOf(t, seq, omnist.DefaultLimits())
	seq60 := "- &b [" + strings.TrimSuffix(strings.Repeat("1, ", 100), ", ") + "]\n" + strings.Repeat("- *b\n", 60)
	if err := checkAliasExpansion(parseRoot(t, seq60), 50, omnist.DefaultMaxExpandedSlots); err != nil {
		t.Errorf("root sequence aliased 60 times: %v, want accepted", err)
	}
}

func TestAliasUnanchoredBoundaryIsStrictlyGreaterThan(t *testing.T) {
	// E(t) = (1 + 9*11) / 2 = 50.00 exactly: accepted at the default 50
	// (kills ">=" mutants and a shifted default), rejected at 49.
	at := mergeFanIn(9, 11)
	readOK(t, "E == 50 at default", at, omnist.DefaultLimits())
	readOK(t, "E == 50 at explicit 50", at, limitsWith(50))
	_ = aliasErrOf(t, at, limitsWith(49))
	// E(t) = (1 + 9*12) / 2 = 54.50: rejected at the default.
	_ = aliasErrOf(t, mergeFanIn(9, 12), omnist.DefaultLimits())
	// Plain-alias container {a: *b}: E = (1 + W(b)) / 2 with W(b) = n + 1:
	// n = 99 is 50.50 (rejected), 98 is 50.00 and 97 is 49.50 (accepted).
	_ = aliasErrOf(t, blockText(99)+"t: {a: *b}\n", omnist.DefaultLimits())
	readOK(t, "n=98", blockText(98)+"t: {a: *b}\n", omnist.DefaultLimits())
	readOK(t, "n=97", blockText(97)+"t: {a: *b}\n", omnist.DefaultLimits())
}

func TestAliasBothBombsFromReviewAreRejectedFast(t *testing.T) {
	// Scaled-down (milliseconds under -race) and full-size.
	for _, tc := range []struct {
		name string
		text string
	}{
		{"merge fan-in 100x100", mergeFanIn(100, 100)},
		{"merge fan-in 2000x2000", mergeFanIn(2000, 2000)},
		{"root fan-out {k: *b} 1000x100", blockText(1000) + "l:\n" + strings.Repeat("  - {k: *b}\n", 100)},
		{"root fan-out {k: *b} 1000x100000", blockText(1000) + "l:\n" + strings.Repeat("  - {k: *b}\n", 100000)},
		{"root sequence of *b 1000x100000", "- &b [" + strings.TrimSuffix(strings.Repeat("1, ", 1000), ", ") + "]\n" + strings.Repeat("- *b\n", 100000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The check alone (the YAML text parse, which dominates a
			// 100,000-item document under -race, is not the check's cost).
			root := parseRoot(t, tc.text)
			start := time.Now()
			if err := checkAliasExpansion(root, 50, omnist.DefaultMaxExpandedSlots); err == nil {
				t.Error("checkAliasExpansion accepted the bomb")
			}
			check := time.Since(start)
			if check > time.Second {
				t.Errorf("the check took %v, want well under a second", check)
			}
			// And the full Read, parse included: rejected with the right
			// code, and far faster than materializing (which takes minutes
			// and gigabytes for the largest of these).
			start = time.Now()
			_, err := Read(tc.text, omnist.DefaultLimits())
			read := time.Since(start)
			pe, ok := err.(*omnist.ParseError)
			if !ok || pe.Code != omnist.CodeDocumentLimitAliasExpansion || pe.Path != "$" {
				t.Errorf("got %#v, want document.limit.alias-expansion at $", err)
			}
			if read > 20*time.Second {
				t.Errorf("Read took %v, want a pre-expansion rejection", read)
			}
			t.Logf("check %v, Read %v", check, read)
		})
	}
}

func TestAliasFalsePositiveGuardsForRealisticInput(t *testing.T) {
	// Compose-style: 100 services merging a 20-key defaults anchor. Each
	// service has E = (1 + 20 + 1) / 3 = 7.33, the root is far lower.
	var b strings.Builder
	b.WriteString("x-defaults: &defaults\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "  key%d: v%d\n", i, i)
	}
	b.WriteString("services:\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "  svc%d:\n    <<: *defaults\n    image: img%d\n", i, i)
	}
	readOK(t, "compose 100 x 20", b.String(), omnist.DefaultLimits())

	// A 100-key block aliased 60 times at the root: E = 6162/162 = 38.04
	// accepted; 100 times: 10202/202 = 50.50 rejected.
	readOK(t, "100-key x60", blockText(100)+aliasKeys(60), omnist.DefaultLimits())
	_ = aliasErrOf(t, blockText(100)+aliasKeys(100), omnist.DefaultLimits())
}

func TestAliasLargeAnchorMergeKeysPlusTwoOverThree(t *testing.T) {
	// job: {<<: *base, script: x} has E = (keys + 2) / 3 (stated, intended):
	// accepted with 60 base keys, 148 keys is E = 50.00 (accepted), 149 is
	// 50.33 (rejected) and 150 is 50.67 (rejected) at the default 50.
	job := func(keys int) string {
		return strings.Replace(blockText(keys), "b: &b", "base: &b", 1) + "job: {<<: *b, script: x}\n"
	}
	readOK(t, "60 keys", job(60), omnist.DefaultLimits())
	readOK(t, "148 keys (E = 50.00)", job(148), omnist.DefaultLimits())
	_ = aliasErrOf(t, job(149), omnist.DefaultLimits())
	_ = aliasErrOf(t, job(150), omnist.DefaultLimits())
	// The documented escape hatch: raise the maximum.
	readOK(t, "150 keys at 51", job(150), limitsWith(51))
	readOK(t, "150 keys at 10000", job(150), limitsWith(10000))
}

func TestAliasInlineMergeSourceSlotCounting(t *testing.T) {
	// The spec example: t: {<<: {a: 1}, z: 1} has W = 3, S = 4.
	wantSlots(t, "inline", "t: &t {<<: {a: 1}, z: 1}\n", map[string][2]uint64{"t": {3, 4}})
	// Aliases inside the inline mapping: the inline container is one slot in
	// the referrer's S (the `<<` slot), its written values add theirs; in W it
	// contributes w-1. Inline own: W = 1 + 2*2 = 5, S = 3.
	wantSlots(t, "inline aliases", "p: &p {k: 1}\nt: &t {<<: {x: *p, y: *p}}\n", map[string][2]uint64{"p": {2, 2}, "t": {5, 4}})
	// Inline one-past: {<<: {x: *b x M}} with an n-key block. The inline
	// mapping has E = (1 + M*(n+1)) / (1 + M); n = 99, M = 1 gives 50.50.
	_ = aliasErrOf(t, blockText(99)+"t: {<<: {x: *b}}\n", omnist.DefaultLimits())
	// The inline mapping is rejected on its own even when the referrer
	// dilutes it below the limit with many plain scalars (the referrer's E
	// would be about 3).
	var b strings.Builder
	b.WriteString(blockText(100))
	b.WriteString("t: {<<: {")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "x%d: *b, ", i)
	}
	b.WriteString("y: 1}")
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&b, ", o%d: 1", i)
	}
	b.WriteString("}\n")
	pe := aliasErrOf(t, b.String(), omnist.DefaultLimits())
	if pe.Line != 2 {
		t.Errorf("rejected at line %d, want 2", pe.Line)
	}
	// An inline mapping without aliases has E = 1.00: any size is accepted.
	readOK(t, "inline plain", "t: {<<: {"+strings.TrimSuffix(strings.Repeat("a: 1, ", 1), ", ")+"}, z: 1}\n", limitsWith(1))
}

func TestAliasScalarsAreNeverChecked(t *testing.T) {
	// A scalar document and scalar anchors are accepted at the smallest limit.
	readOK(t, "scalar root", "5\n", limitsWith(1))
	readOK(t, "scalar anchor + alias", "c: &c 5\nd: *c\n", limitsWith(1))
}

func TestAliasUnanchoredNestedSequenceIsCheckedOnItsOwn(t *testing.T) {
	// An unanchored sequence [*b x 10] over a 100-scalar block has
	// E = (1 + 10*101) / 11 = 91.9. Its enclosing mapping and the root stay
	// far under the limit thanks to 2000 filler scalars, so only the
	// sequence's own check can reject it.
	var b strings.Builder
	b.WriteString(blockText(100))
	b.WriteString("m:\n  s: [" + strings.TrimSuffix(strings.Repeat("*b, ", 10), ", ") + "]\n")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, "  f%d: 1\n", i)
	}
	text := b.String()
	pe := aliasErrOf(t, text, omnist.DefaultLimits())
	if pe.Line != 3 {
		t.Errorf("rejected at line %d, want 3 (the sequence)", pe.Line)
	}
	readOK(t, "same shape at 92", text, limitsWith(92))
}
