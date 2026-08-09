package omnist

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// WriteJSON renders d as JSON text (spec §7.3, docs/formats/json.md),
// schema-free: a writer MUST NOT accept a schema (§7.3), and this one
// doesn't. It applies §7.3's two rules (grouping by label, the count-1
// bare-value-vs-list rule) via writeJSONNode/writeJSONGroup below, plus
// JSON-specific leaf rendering: temporal leaves are stringified to
// ISO-8601, and NaN/Infinity — not valid JSON tokens — are substituted
// with `null` at the leaf (docs/formats/json.md's default lenient mode).
//
// The signature returns an error because a NaN/Infinity substitution is a
// reportable adjustment, not always a silent one (spec §7.4: "a reader or
// writer SHOULD be able to report the adjustments... this is what makes
// lossiness auditable"). In lenient mode (this function) the write itself
// never fails — substitution always succeeds — so the error return is
// currently always nil; it exists so the signature doesn't have to change
// later when full format-report plumbing lands, and so a caller reading
// only the signature sees, correctly, that writing JSON can have something
// to report. Full diagnostic-collection wiring is a TODO (spec §7.4 says
// only SHOULD, and no `format.*` adjustment-reporting mechanism exists
// elsewhere in this repo yet to hook into) — this is called out explicitly
// per the issue's instruction not to silently drop the requirement.
//
// See WriteJSONStrict for the spec's optional MAY: failing instead of
// substituting.
func WriteJSON(d Document) (string, error) {
	return writeJSONDocument(d, false)
}

// WriteJSONStrict renders d as JSON text like WriteJSON, except a
// NaN/Infinity leaf is a hard failure instead of a `null` substitution —
// the strict mode docs/formats/json.md's "no NaN or Infinity" rule
// describes as a spec MAY ("A strict mode MAY instead fail"). The
// returned error is a Diagnostic (code write.unsupported-value, spec
// §8.3.9), positioned by the Document path to the offending leaf.
func WriteJSONStrict(d Document) (string, error) {
	return writeJSONDocument(d, true)
}

func writeJSONDocument(d Document, strict bool) (string, error) {
	var b strings.Builder
	if d.IsNode {
		if err := writeJSONNode(&b, d.Node, "$", strict); err != nil {
			return "", err
		}
		return b.String(), nil
	}
	if err := writeJSONValue(&b, d.Value, "$", strict); err != nil {
		return "", err
	}
	return b.String(), nil
}

// jsonGroup is one label's worth of grouped edges, in the order
// write(node, format)'s pseudocode (spec §7.3.1) builds `groups`: keyed by
// label, first-seen label order, and — within a label — original edge
// order.
type jsonGroup struct {
	label    string
	children []Target
}

// writeJSONNode implements §7.3.1's write(node, format) for the JSON
// format: group edges sharing a label (grouping rule), then render each
// group as a bare value when it has exactly one child or as a list
// otherwise (count-1 rule).
func writeJSONNode(b *strings.Builder, n *Node, path string, strict bool) error {
	groups := groupJSONEdges(n)

	b.WriteByte('{')
	for i, g := range groups {
		if i > 0 {
			b.WriteString(", ")
		}
		writeJSONString(b, g.label)
		b.WriteString(": ")
		childPath := path + "." + g.label

		if len(g.children) == 1 {
			if err := writeJSONTarget(b, g.children[0], childPath, strict); err != nil {
				return err
			}
			continue
		}
		b.WriteByte('[')
		for j, t := range g.children {
			if j > 0 {
				b.WriteString(", ")
			}
			if err := writeJSONTarget(b, t, fmt.Sprintf("%s[%d]", childPath, j), strict); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	}
	b.WriteByte('}')
	return nil
}

// groupJSONEdges implements §7.3.1's `groups` construction: edges sharing
// a label collapse into one group, in first-seen label order, preserving
// each label's own children in their original edge order. Cross-label
// interleaving (e.g. [(m,A),(x,X),(m,B)]) is lost here, exactly as §7.3
// states JSON must ("no format in the JSON family can express it").
func groupJSONEdges(n *Node) []jsonGroup {
	var groups []jsonGroup
	index := make(map[string]int, len(n.Edges))
	for _, e := range n.Edges {
		if i, ok := index[e.Label]; ok {
			groups[i].children = append(groups[i].children, e.Target)
			continue
		}
		index[e.Label] = len(groups)
		groups = append(groups, jsonGroup{label: e.Label, children: []Target{e.Target}})
	}
	return groups
}

func writeJSONTarget(b *strings.Builder, t Target, path string, strict bool) error {
	if node, ok := t.Node(); ok {
		return writeJSONNode(b, node, path, strict)
	}
	v, _ := t.Value()
	return writeJSONValue(b, v, path, strict)
}

func writeJSONValue(b *strings.Builder, v Value, path string, strict bool) error {
	if v.IsNull {
		b.WriteString("null")
		return nil
	}
	return writeJSONScalar(b, v.Scalar, path, strict)
}

// writeJSONScalar renders one leaf. Per docs/formats/json.md: "a writer
// MUST stringify a temporal leaf to ISO-8601" (KindDate/KindTime/
// KindDateTime), and NaN/Infinity (KindNumber only — JSON's only floating
// kind) substitute to `null` unless strict, in which case they fail
// instead. For KindInteger, s.Int is assumed non-nil, mirroring
// oml_writer.go's writeOMLScalar precondition: every Scalar of that kind
// reaching this function was built by NewIntegerScalar (which always
// copies a non-nil *big.Int) or produced by ReadJSON, neither of which
// ever leaves Int nil for KindInteger.
func writeJSONScalar(b *strings.Builder, s Scalar, path string, strict bool) error {
	switch s.Kind {
	case KindString:
		writeJSONString(b, s.Str)
	case KindInteger:
		b.WriteString(s.Int.String())
	case KindNumber:
		return writeJSONNumber(b, s.Num, path, strict)
	case KindBoolean:
		if s.Bool {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case KindDate:
		writeJSONString(b, formatISODate(s.Date))
	case KindTime:
		writeJSONString(b, formatISOTime(s.Time))
	case KindDateTime:
		writeJSONString(b, formatISODate(s.DateTime.Date)+"T"+formatISOTime(s.DateTime.Time))
	}
	return nil
}

// writeJSONNumber renders a KindNumber leaf. A finite value always gets a
// decimal point or exponent (never a bare integer-looking spelling) so
// that ReadJSON's own integer/number split — decided purely by the
// literal's shape — reads it back as a number, not an integer; without
// this, writing 5.0 as the JSON text `5` would silently flip its kind on
// round-trip. NaN/Infinity have no valid JSON spelling at all (spec: "not
// valid JSON... a writer MUST NOT emit them"); the default lenient mode
// substitutes `null`, strict mode fails with write.unsupported-value.
func writeJSONNumber(b *strings.Builder, f float64, path string, strict bool) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		if strict {
			return Diagnostic{
				Path:     path,
				Code:     CodeWriteUnsupportedValue,
				Message:  "NaN/Infinity has no JSON representation",
				Severity: SeverityError,
			}
		}
		b.WriteString("null")
		return nil
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	b.WriteString(s)
	return nil
}

// formatISODate renders a DateValue per ISO-8601's calendar-date form
// (YYYY-MM-DD), matching oml_writer.go's formatOMLDate exactly — the same
// spelling is both OML's canonical date form and ISO-8601's, so there is
// no JSON-specific variation to introduce.
func formatISODate(d DateValue) string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// formatISOTime renders a TimeValue per ISO-8601's time-of-day form.
// Seconds are emitted only when Second or Nanosecond is nonzero, and the
// fractional part only when Nanosecond is nonzero, mirroring
// oml_writer.go's formatOMLTime — the Document model doesn't distinguish
// "seconds explicitly written as :00" from "seconds omitted" for either
// format, so this is the same reasonable, round-trip-safe
// canonicalization, not a spec requirement, applied consistently with
// OML's writer.
func formatISOTime(t TimeValue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%02d:%02d", t.Hour, t.Minute)
	if t.Second != 0 || t.Nanosecond != 0 {
		fmt.Fprintf(&b, ":%02d", t.Second)
		if t.Nanosecond != 0 {
			b.WriteByte('.')
			b.WriteString(formatISOFraction(t.Nanosecond))
		}
	}
	if t.HasOffset {
		off := t.OffsetSeconds
		sign := byte('+')
		if off < 0 {
			sign = '-'
			off = -off
		}
		fmt.Fprintf(&b, "%c%02d:%02d", sign, off/3600, (off/60)%60)
	}
	return b.String()
}

// formatISOFraction converts a Nanosecond field to the shortest 1-6 digit
// fraction string that reproduces it, mirroring oml_writer.go's
// formatOMLFraction. Nanosecond is always a product of a source fraction
// decoder that right-pads to 9 digits (oml_lexer.go's fracToNanos is the
// only producer of TimeValue.Nanosecond in this repo), so it is always an
// exact multiple of 1000, and trimming trailing zeros from its 6-digit
// microsecond form can never trim down to nothing given the caller's
// guard that Nanosecond != 0.
func formatISOFraction(ns int) string {
	micros := ns / 1000
	s := fmt.Sprintf("%06d", micros)
	return strings.TrimRight(s, "0")
}

// writeJSONString renders s as a JSON string literal, escaping exactly
// what the JSON grammar requires: '"', '\\', the named short escapes for
// backspace/formfeed/newline/CR/tab, and \u00XX for every other control
// character. Non-ASCII characters pass through as literal UTF-8 — valid
// JSON strings are UTF-8 text, and JSON has no requirement to \u-escape
// non-ASCII content.
func writeJSONString(b *strings.Builder, s string) {
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
