package yaml

import (
	"math"
	"strconv"
	"strings"

	yamllib "gopkg.in/yaml.v3"

	omnist "github.com/omnist-dev/omnist-go"
)

// Write renders d as YAML text (spec §7.3, docs/formats/yaml.md),
// schema-free: a writer MUST NOT accept a schema (§7.3), and this one
// doesn't. It applies §7.3's two rules (grouping by label, the count-1
// bare-value-vs-list rule) via writeYAMLNode/writeYAMLTarget below, then
// this file's own leaf-rendering rules — see the temporal-value reasoning
// below.
//
// omnist.Node-tree construction (buildYAMLNode/buildYAMLScalar) is done by hand;
// yaml.v3's own Marshal is used only for the final text emission from that
// tree (indentation, quoting punctuation, line wrapping), not for any
// typing decision — every Style and Tag choice below is this package's
// own, for the same reason ReadYAML doesn't trust the library's default
// resolution: this writer needs precise control over which scalars are
// quoted, since an unquoted mis-chosen spelling would silently change kind
// when read back (by this reader or any other YAML-1.1-conformant one).
//
// # Temporal-value writing: reasoning (issue #25)
//
// docs/formats/json.md states outright that a JSON writer MUST stringify
// every temporal leaf, because JSON has no native temporal type at all —
// there is no other option. docs/formats/yaml.md does not restate an
// equivalent rule for YAML, and per the issue this needed to be thought
// through rather than assumed to be "the same as JSON." The three temporal
// kinds do not all behave the same way under YAML's own core-schema
// resolution (the same resolution ReadYAML implements above), so they are
// not all written the same way:
//
//   - omnist.KindDate: an unquoted bare ISO date (2024-01-01) is exactly what
//     docs/formats/yaml.md's worked example shows resolving to a `date`
//     at stage 1 with no schema involved. Writing it bare is therefore
//     lossless all the way through stage 1 — strictly better than JSON,
//     where the same value can only round-trip as a string until stage 2
//     upgrades it with a schema. There is no reason to stringify a date.
//   - omnist.KindDateTime: the same reasoning applies — an unquoted bare
//     ISO-8601 datetime resolves to a `datetime` at stage 1 (confirmed
//     against reYAMLDateTime/resolveYAMLScalar above, the same regex this
//     writer's own reader half uses), so it is written bare too.
//   - omnist.KindTime is the one genuine exception, and it exists because of the
//     sharp edge docs/formats/yaml.md names explicitly: "YAML's core
//     schema has no standalone time type. A bare `12:00:00` resolves to
//     the integer 43200." An unquoted bare time-of-day would not merely
//     fail to round-trip as a time — it would silently become a
//     different scalar kind entirely (integer) on read-back, which is
//     worse than JSON's behavior (where it would at least come back as
//     the same string). There is no bare spelling that survives YAML's
//     own resolver as a time, so a omnist.KindTime leaf is quoted (forced to
//     omnist.KindString on read-back) — the same stringify-and-report treatment
//     JSON's writer gives every temporal kind unconditionally, applied
//     here to exactly the one YAML kind that has no lossless bare form.
//     This is reported via omnist.CodeFormatTemporalStringified (spec §8.3.8),
//     the same code JSON's writer would use if it collected diagnostics
//     today — see the TODO below on why the collection itself isn't wired
//     up yet, mirroring json_writer.go's identical TODO.
//
// The signature returns an error for the same reason WriteJSON's does: a
// stringification is a reportable adjustment (spec §7.4), not always a
// silent one, even though nothing below can currently fail (there is no
// strict/lenient toggle for YAML's time sharp edge — it isn't optional the
// way JSON's NaN/Infinity substitution is, so there is only one mode).
// Full diagnostic-collection plumbing is a TODO, exactly as noted on
// WriteJSON: spec §7.4 says only SHOULD, and no format.* adjustment-
// reporting mechanism exists elsewhere in this repo yet to hook into.
func Write(d omnist.Document) (string, error) {
	var root *yamllib.Node
	if d.IsNode {
		root = buildYAMLNode(d.Node)
	} else {
		root = buildYAMLTargetScalar(d.Value)
	}
	out, err := yamllib.Marshal(root)
	if err != nil {
		// This is reachable, not defensive: encode.go's writer refuses
		// to marshal a omnist.KindString omnist.Scalar whose Str holds bytes that are
		// not valid UTF-8, with exactly this error text
		// ("cannot marshal invalid UTF-8 data as !!str") — confirmed
		// empirically with a scalar built directly from invalid bytes.
		// omnist.Document.Value.Scalar.Str is a Go string, which (unlike Go
		// source-code string literals) is not required to hold valid
		// UTF-8 at the type level, so a omnist.Document built programmatically
		// (rather than only ever produced by a reader that validated its
		// own input text) can legitimately reach this. There is no
		// established taxonomy code (spec §8.3.9) more specific than
		// this to translate it into and no other codec's writer in this
		// package currently performs this translation either, so the
		// library's own error is surfaced as-is rather than wrapped in
		// a omnist.Diagnostic that would overstate precision this package
		// doesn't actually have here.
		return "", err
	}
	return string(out), nil
}

// yamlGroup mirrors jsonGroup (json_writer.go): one label's worth of
// grouped edges, in first-seen label order with within-label edge order
// preserved, per §7.3.1's `groups` construction.
type yamlGroup struct {
	label    string
	children []omnist.Target
}

func groupYAMLEdges(n *omnist.Node) []yamlGroup {
	var groups []yamlGroup
	index := make(map[string]int, len(n.Edges))
	for _, e := range n.Edges {
		if i, ok := index[e.Label]; ok {
			groups[i].children = append(groups[i].children, e.Target)
			continue
		}
		index[e.Label] = len(groups)
		groups = append(groups, yamlGroup{label: e.Label, children: []omnist.Target{e.Target}})
	}
	return groups
}

// buildYAMLNode implements §7.3.1's write(node, format) for the YAML
// format: group edges sharing a label (grouping rule), then render each
// group as a bare value when it has exactly one child or as a sequence
// otherwise (count-1 rule) — the same two rules writeJSONNode applies, on
// yamllib.Node values instead of directly-written text.
func buildYAMLNode(n *omnist.Node) *yamllib.Node {
	groups := groupYAMLEdges(n)
	m := &yamllib.Node{Kind: yamllib.MappingNode}
	for _, g := range groups {
		// Every label is written double-quoted unconditionally, never
		// bare. A label is always a string (spec §2.2.2), but a bare
		// spelling that happens to collide with YAML's own core-schema
		// resolution (an "on"/"yes"/"no" label, or one that looks like
		// a number or date) would silently stop being a string on
		// read-back — this reader's own readLabel would then refuse to
		// even build a omnist.Document from it (the Norway-problem rejection
		// this issue's reader half implements). Unconditional quoting
		// sidesteps needing to replicate resolveYAMLScalar's collision
		// logic here just to decide, case by case, which labels are
		// "safe" to leave bare — every label is safe when quoted, with
		// no exceptions to enumerate or get wrong.
		m.Content = append(m.Content, &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!str", Style: yamllib.DoubleQuotedStyle, Value: g.label})
		if len(g.children) == 1 {
			m.Content = append(m.Content, buildYAMLTarget(g.children[0]))
			continue
		}
		seq := &yamllib.Node{Kind: yamllib.SequenceNode}
		for _, t := range g.children {
			seq.Content = append(seq.Content, buildYAMLTarget(t))
		}
		m.Content = append(m.Content, seq)
	}
	return m
}

func buildYAMLTarget(t omnist.Target) *yamllib.Node {
	if node, ok := t.Node(); ok {
		return buildYAMLNode(node)
	}
	v, _ := t.Value()
	return buildYAMLTargetScalar(v)
}

func buildYAMLTargetScalar(v omnist.Value) *yamllib.Node {
	if v.IsNull {
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!null", Value: "null"}
	}
	return buildYAMLScalar(v.Scalar)
}

// buildYAMLScalar renders one leaf as a yamllib.Node, deciding bare-vs-quoted
// per kind. See Write's doc comment for the temporal reasoning; every
// other kind's bare spelling is chosen so it round-trips through this
// file's ReadYAML/resolveYAMLScalar unambiguously:
//
//   - omnist.KindString is always double-quoted (see buildYAMLNode's identical
//     reasoning for labels — a bare string can collide with core-schema
//     resolution just as easily as a label can).
//   - omnist.KindInteger/omnist.KindNumber (finite) are written bare in a spelling that
//     only that kind's branch of resolveYAMLScalar accepts (an integer
//     never contains '.'/'e'; a finite float always does, matching
//     writeJSONNumber's identical reasoning for the identical ambiguity).
//   - omnist.KindNumber (NaN/Infinity) is written bare using YAML's own core
//     schema spellings (.nan/.inf/-.inf) — unlike JSON, YAML's core
//     schema has a native spelling for these (resolveYAMLScalar resolves
//     them directly), so, unlike WriteJSON, no substitution or strict
//     mode is needed here at all.
//   - omnist.KindBoolean is written bare as true/false — unambiguous under
//     resolveYAMLScalar in every YAML-1.1-conformant reader, including
//     this one.
//   - omnist.KindDate/omnist.KindDateTime are written bare in ISO-8601 form (see
//     Write's doc comment).
//   - omnist.KindTime is double-quoted (forced to string on read-back — see
//     Write's doc comment for why this is the one unavoidable case).
func buildYAMLScalar(s omnist.Scalar) *yamllib.Node {
	switch s.Kind {
	case omnist.KindString:
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!str", Style: yamllib.DoubleQuotedStyle, Value: s.Str}
	case omnist.KindInteger:
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!int", Value: s.Int.String()}
	case omnist.KindNumber:
		return buildYAMLNumber(s.Num)
	case omnist.KindBoolean:
		v := "false"
		if s.Bool {
			v = "true"
		}
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!bool", Value: v}
	case omnist.KindDate:
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!timestamp", Value: omnist.FormatISODate(s.Date)}
	case omnist.KindTime:
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!str", Style: yamllib.DoubleQuotedStyle, Value: omnist.FormatISOTime(s.Time)}
	default: // omnist.KindDateTime
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!timestamp", Value: omnist.FormatISODate(s.DateTime.Date) + "T" + omnist.FormatISOTime(s.DateTime.Time)}
	}
}

// buildYAMLNumber renders a omnist.KindNumber leaf, bare. A finite value always
// gets a decimal point or exponent (mirroring writeJSONNumber's identical
// reasoning: without it, resolveYAMLScalar's own int-before-float check
// would read a whole-number float back as an integer, silently flipping
// its kind on round-trip). NaN/Infinity use YAML's own core-schema
// spellings directly — see buildYAMLScalar's doc comment for why, unlike
// WriteJSON, no substitution is needed.
func buildYAMLNumber(f float64) *yamllib.Node {
	if math.IsNaN(f) {
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!float", Value: ".nan"}
	}
	if math.IsInf(f, 1) {
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!float", Value: ".inf"}
	}
	if math.IsInf(f, -1) {
		return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!float", Value: "-.inf"}
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return &yamllib.Node{Kind: yamllib.ScalarNode, Tag: "!!float", Value: s}
}

// FormatISODate/FormatISOTime are temporal.go's shared, exported
// ISO-8601 renderers (promoted there in issue #45 once the move to
// formats/json revealed json_writer.go's private copies were a real
// cross-codec dependency, not the doc-comment-only kind issue #45
// originally assumed); this file does not redefine them.
