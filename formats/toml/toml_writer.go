package toml

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// Write renders d as TOML text (spec §7.3, docs/formats/toml.md),
// schema-free: a writer MUST NOT accept a schema (§7.3), and this one
// doesn't. It applies §7.3's two rules (grouping by label, the count-1
// bare-value-vs-list rule) via writeTOMLTopLevel/writeTOMLInlineTable
// below, then this file's own leaf-rendering rules.
//
// # Inline-only rendering: a deliberate simplification, not a spec gap
//
// docs/formats/toml.md's model-mapping table documents what `[section]`
// and `[[section]]` header syntax MEANS when READ (a single edge vs. a
// repeated label) — it does not require a writer to prefer header syntax
// over TOML's other, equally valid way of expressing the identical
// shape: inline tables (`{k = v, ...}`) and inline arrays (`[v, v, ...]`).
// This writer always uses the inline forms for every nested table and
// every repeated-label group, at every depth, and never emits a
// `[section]`/`[[section]]` header.
//
// This is a real simplification with a real, deliberate reason: header
// syntax carries TOML-specific positional rules with no equivalent in the
// omnist.Document model itself — a `[section]` header must appear after all of
// its parent table's bare key-values, `[[section]]` instances must be
// contiguous, and re-opening a dotted path later in the document is only
// legal under specific conditions (see navigateOrCreate's doc comment in
// toml_reader.go for the read side of this same asymmetry). None of that
// is a concern §7.3's schema-free write(node, format) algorithm has any
// input for — it just groups edges by label and asks for a value or a
// list. Inline tables/arrays have no positional constraints at all: they
// are ordinary values that can appear anywhere a value can, so this
// writer can implement §7.3.1 exactly as written, with no extra
// bookkeeping to keep a table's context "open" across non-adjacent
// writes, and no risk of ever emitting a header-ordering violation.
// docs/formats/toml.md's own worked example (rendered with `[section]`
// headers) and this writer's inline-only output are two different, both
// entirely valid, TOML spellings of the identical omnist.Document — Read
// reads either one back to the exact same edge structure (confirmed by
// TestWriteTOMLRoundTripsWorkedExample), which is what §7.3's writer
// contract actually requires (a correct rendering of the omnist.Document's
// shape), not any particular surface syntax.
//
// # Null: no TOML spelling exists at all
//
// docs/formats/toml.md is explicit: "A null-valued leaf cannot be
// written... Implementations MUST report this as a write-time
// adjustment rather than inventing a representation." Unlike WriteJSON's
// NaN/Infinity substitution (which has a lenient default and a strict
// opt-in that fails instead), TOML has no lenient option to fall back to
// here — there is no spelling of any kind, not even a lossy one, so this
// writer's only choice is to fail. It does so by returning a omnist.Diagnostic
// (omnist.CodeFormatNullUnrepresentable, spec §8.3.8, the code's own defined
// warning severity, carrying the omnist.Document path to the offending leaf) the
// first time it encounters a null anywhere in the tree — this is the
// same "return the omnist.Diagnostic itself as the error" mechanism
// WriteJSONStrict already uses for its own hard-failure case
// (write.unsupported-value), applied here to the one case TOML's writer
// has no lenient mode for at all.
//
// # Bare-scalar-root: a real error, not malformed output
//
// docs/formats/toml.md: "A bare scalar omnist.Document cannot be written as
// TOML." TOML has no syntax for a document whose entire content is one
// value with no key — every top-level statement is a `key = value` or a
// table header. Write checks d.IsNode before writing anything and
// fails immediately (omnist.CodeWriteUnsupportedValue) if the root is a scalar
// omnist.Value rather than a omnist.Node, rather than producing any output at all.
func Write(d omnist.Document) (string, error) {
	if !d.IsNode {
		return "", omnist.Diagnostic{
			Path:     "$",
			Code:     omnist.CodeWriteUnsupportedValue,
			Message:  "a TOML document's top level must be a table; a bare scalar omnist.Document has no TOML spelling",
			Severity: omnist.SeverityError,
		}
	}
	var b strings.Builder
	if err := writeTOMLTopLevel(&b, d.Node); err != nil {
		return "", err
	}
	return b.String(), nil
}

// tomlGroup mirrors jsonGroup/yamlGroup (json_writer.go/yaml_writer.go):
// one label's worth of grouped edges, in first-seen label order with
// within-label edge order preserved, per §7.3.1's `groups` construction.
type tomlGroup struct {
	label    string
	children []omnist.Target
}

func groupTOMLEdges(n *omnist.Node) []tomlGroup {
	var groups []tomlGroup
	index := make(map[string]int, len(n.Edges))
	for _, e := range n.Edges {
		if i, ok := index[e.Label]; ok {
			groups[i].children = append(groups[i].children, e.Target)
			continue
		}
		index[e.Label] = len(groups)
		groups = append(groups, tomlGroup{label: e.Label, children: []omnist.Target{e.Target}})
	}
	return groups
}

// writeTOMLTopLevel implements §7.3.1's write(node, format) at the
// document root: one `key = value` (or `key = [values]`) line per group,
// per the grouping and count-1 rules — see Write's doc comment for
// why nested tables are written as inline-table values here rather than
// `[section]` headers.
func writeTOMLTopLevel(b *strings.Builder, n *omnist.Node) error {
	groups := groupTOMLEdges(n)
	for _, g := range groups {
		writeTOMLKey(b, g.label)
		b.WriteString(" = ")
		if err := writeTOMLGroupValue(b, g, "$."+g.label); err != nil {
			return err
		}
		b.WriteByte('\n')
	}
	return nil
}

// writeTOMLGroupValue renders one group's value per §7.3.1's count-1
// rule: a bare rendering of the single target when the group has exactly
// one child, an inline array of renderings otherwise.
func writeTOMLGroupValue(b *strings.Builder, g tomlGroup, path string) error {
	if len(g.children) == 1 {
		return writeTOMLTarget(b, g.children[0], path)
	}
	b.WriteByte('[')
	for i, t := range g.children {
		if i > 0 {
			b.WriteString(", ")
		}
		if err := writeTOMLTarget(b, t, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	b.WriteByte(']')
	return nil
}

// writeTOMLTarget renders one Target: an inline table for a omnist.Node, a
// scalar literal (or the null failure) for a omnist.Value.
func writeTOMLTarget(b *strings.Builder, t omnist.Target, path string) error {
	if node, ok := t.Node(); ok {
		return writeTOMLInlineTable(b, node, path)
	}
	v, _ := t.Value()
	if v.IsNull {
		return omnist.Diagnostic{
			Path:     path,
			Code:     omnist.CodeFormatNullUnrepresentable,
			Message:  "a null leaf cannot be written in TOML, so it is dropped",
			Severity: omnist.SeverityWarning,
		}
	}
	writeTOMLScalar(b, v.Scalar)
	return nil
}

// writeTOMLInlineTable renders n as a TOML inline table (`{k = v, ...}`),
// applying the same grouping/count-1 rules writeTOMLTopLevel applies at
// the root — an inline table is exactly a nested write(node, format).
func writeTOMLInlineTable(b *strings.Builder, n *omnist.Node, path string) error {
	groups := groupTOMLEdges(n)
	b.WriteByte('{')
	for i, g := range groups {
		if i > 0 {
			b.WriteString(", ")
		}
		writeTOMLKey(b, g.label)
		b.WriteString(" = ")
		if err := writeTOMLGroupValue(b, g, path+"."+g.label); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

// writeTOMLKey renders a label as a TOML quoted (basic string) key,
// unconditionally — never bare. A bare TOML key is restricted to
// `[A-Za-z0-9_-]+`; rather than replicate that bareness test here (and
// risk missing a character class TOML's grammar excludes), every label is
// quoted, mirroring buildYAMLNode's identical unconditional-quoting
// reasoning (yaml_writer.go) for the identical reason: a quoted key is
// always valid, with no bareness rule to get right or wrong.
func writeTOMLKey(b *strings.Builder, label string) {
	writeTOMLString(b, label)
}

// writeTOMLScalar renders one leaf. Every kind but omnist.KindTime has a native
// TOML spelling with no stringification needed at all — see Write's
// doc comment and Read's "Offset vs. local datetime" section for the
// datetime reasoning. omnist.KindTime is the one kind that needs a small
// adjustment on write: see the case below.
func writeTOMLScalar(b *strings.Builder, s omnist.Scalar) {
	switch s.Kind {
	case omnist.KindString:
		writeTOMLString(b, s.Str)
	case omnist.KindInteger:
		b.WriteString(s.Int.String())
	case omnist.KindNumber:
		writeTOMLNumber(b, s.Num)
	case omnist.KindBoolean:
		if s.Bool {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case omnist.KindDate:
		b.WriteString(omnist.FormatISODate(s.Date))
	case omnist.KindTime:
		// TOML's local-time literal (partial-time in its grammar) has no
		// offset field at all — only a full datetime can carry one. No
		// reader in this package currently ever produces a omnist.KindTime
		// scalar with HasOffset set (only a omnist.KindDateTime's embedded
		// omnist.TimeValue carries an offset in practice), but the omnist.Document
		// model does not forbid the combination at the type level, so
		// this guards it explicitly rather than emitting a spelling
		// TOML's own time-literal grammar would reject: the offset is
		// dropped from a bare omnist.KindTime write (narrow/cosmetic — there is
		// no TOML spelling of "a bare time of day with a UTC offset" to
		// preserve it in).
		t := s.Time
		t.HasOffset = false
		b.WriteString(omnist.FormatISOTime(t))
	default: // omnist.KindDateTime
		b.WriteString(omnist.FormatISODate(s.DateTime.Date) + "T" + omnist.FormatISOTime(s.DateTime.Time))
	}
}

// writeTOMLNumber renders a omnist.KindNumber leaf. A finite value always gets a
// decimal point or exponent (mirroring writeJSONNumber's/buildYAMLNumber's
// identical reasoning: without it, this reader's own int-vs-float split —
// decided purely by the literal's shape, per Read's doc comment —
// would read a whole-number float back as an integer, silently flipping
// its kind on round-trip). NaN/Infinity use TOML's own native spellings
// (confirmed against the grammar and against Read's own
// parseTOMLFloat, which accepts them back) — unlike WriteJSON, no
// substitution or strict mode is needed, matching WriteYAML's identical
// reasoning for YAML's own native nan/inf spellings.
func writeTOMLNumber(b *strings.Builder, f float64) {
	switch {
	case math.IsNaN(f):
		b.WriteString("nan")
	case math.IsInf(f, 1):
		b.WriteString("inf")
	case math.IsInf(f, -1):
		b.WriteString("-inf")
	default:
		s := strconv.FormatFloat(f, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		b.WriteString(s)
	}
}

// writeTOMLString renders s as a TOML basic string literal (double
// quoted), escaping exactly what TOML's basic-string grammar requires:
// '"', '\\', the named short escapes for backspace/formfeed/newline/
// CR/tab, and \u00XX for every other control character — the same escape
// set writeJSONString (json_writer.go) uses, which TOML's basic-string
// grammar shares control-character-for-control-character with JSON's.
// Non-ASCII characters pass through as literal UTF-8: TOML basic strings
// are valid UTF-8 text and have no requirement to \u-escape non-ASCII
// content, mirroring writeJSONString's identical reasoning.
func writeTOMLString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
