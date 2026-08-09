package oml

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// reOMLBareLabel matches a label text eligible for bare (unquoted)
// emission. It is anchored at both ends because a label is a whole token,
// not a prefix — unlike the lexer's reIdent, which only needs to match a
// prefix of the remaining source.
var reOMLBareLabel = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// omlQuotedLabelWords is the writer's quote list from spec §4.4: the
// three parser-level reserved words (null/true/false), plus nan/inf.
// nan/inf are not parser-level reserved words — the tokenizer claims them
// before IDENT is ever tried (§4.2 rule 7) — but a bare "nan"/"inf" label
// would tokenize back as NUMBER, not as a label, so the canonical writer
// must quote them anyway.
var omlQuotedLabelWords = map[string]bool{
	"null": true, "true": true, "false": true, "nan": true, "inf": true,
}

// Write renders d as OML text (spec ch.4): §4.4's label-quoting rule
// and §4.5's string-escaping rule are both followed exactly, since those
// are the two areas spec chapter 4 states as normative MUST/MUST NOT
// rules for the canonical writer.
//
// compact selects the compact form (§4.1: brace-delimited, semicolon
// separated, one line) over the pretty-printed form (§4.1: brace
// delimited, newline separated). Spec §4.1 shows an example of the
// pretty-printed form using two-space indentation but — unlike OSD's
// §5.9, which pins OSD's canonical pretty-printed layout explicitly
// (four-space indent, trailing comma, etc.) — chapter 4 never states that
// OML's pretty-printed indentation is itself part of canonical form. This
// writer follows §4.1's own example layout (two-space indent, opening
// brace on the label's line, closing brace aligned with the label) as a
// reasonable, deterministic, self-consistent choice, documented here per
// the issue's instruction to flag this as a judgment call rather than a
// spec requirement.
// Write's return also carries a []omnist.Diagnostic, matching every other
// codec's writer in this package after issue #49's ok:true+diagnostics
// channel — but not an error. Every other writer added an error return
// (or, for JSON/YAML, kept one already present) because at least one
// leaf shape or document shape genuinely has no spelling in that format
// (TOML's null, XML's multi-root, JSON's strict-mode NaN/Infinity). OML
// has no such case: every omnist.Kind has a native, lossless OML
// spelling (§4.2's grammar covers every scalar kind directly, including
// null, NaN, and Infinity), so nothing here can ever fail and nothing
// here ever needs to report an adjustment — the diagnostics slice is
// always nil today. Adding an always-nil error return anyway, purely for
// uniformity with the other five signatures, would be exactly the
// "unused signature element" the issue calls out as a judgment call to
// avoid: a caller (or a future maintainer) reading `(string, error)`
// reasonably infers there's a failure mode to check for, and OML has
// none. `(string, []omnist.Diagnostic)` is the honest middle ground:
// callers that already loop over every writer's diagnostics (the CLI,
// the conformance driver) can treat OML uniformly with the rest, without
// this package claiming a failure mode or an adjustment mode it doesn't
// have.
func Write(d omnist.Document, compact bool) (string, []omnist.Diagnostic) {
	var b strings.Builder
	if d.IsNode {
		if compact {
			writeOMLNodeEdgesCompact(&b, d.Node)
		} else {
			writeOMLNodeEdgesPretty(&b, d.Node, 0)
			if len(d.Node.Edges) > 0 {
				// The pretty-printed form is a text file, one edge per
				// line (§4.1's own layout convention this writer
				// follows) — a trailing newline after the last line is
				// the conventional text-file terminator, confirmed by
				// every one of this suite's formats-oml/basic/*-on-write
				// vectors expecting one.
				b.WriteByte('\n')
			}
		}
		return b.String(), nil
	}
	writeOMLValue(&b, d.Value)
	return b.String(), nil
}

// WriteCompact is a convenience wrapper for Write(d, true).
func WriteCompact(d omnist.Document) (string, []omnist.Diagnostic) { return Write(d, true) }

const omlIndentUnit = "  "

func writeOMLNodeEdgesPretty(b *strings.Builder, n *omnist.Node, level int) {
	indent := strings.Repeat(omlIndentUnit, level)
	for i, e := range n.Edges {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		writeOMLLabel(b, e.Label)
		b.WriteString(": ")
		writeOMLTargetPretty(b, e.Target, level)
	}
}

func writeOMLTargetPretty(b *strings.Builder, t omnist.Target, level int) {
	if node, ok := t.Node(); ok {
		if len(node.Edges) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		writeOMLNodeEdgesPretty(b, node, level+1)
		b.WriteByte('\n')
		b.WriteString(strings.Repeat(omlIndentUnit, level))
		b.WriteByte('}')
		return
	}
	v, _ := t.Value()
	writeOMLValue(b, v)
}

func writeOMLNodeEdgesCompact(b *strings.Builder, n *omnist.Node) {
	for i, e := range n.Edges {
		if i > 0 {
			b.WriteString("; ")
		}
		writeOMLLabel(b, e.Label)
		b.WriteString(": ")
		writeOMLTargetCompact(b, e.Target)
	}
}

func writeOMLTargetCompact(b *strings.Builder, t omnist.Target) {
	if node, ok := t.Node(); ok {
		if len(node.Edges) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{ ")
		writeOMLNodeEdgesCompact(b, node)
		b.WriteString(" }")
		return
	}
	v, _ := t.Value()
	writeOMLValue(b, v)
}

// writeOMLLabel implements spec §4.4's canonical label-quoting rule.
func writeOMLLabel(b *strings.Builder, label string) {
	if reOMLBareLabel.MatchString(label) && !omlQuotedLabelWords[label] {
		b.WriteString(label)
		return
	}
	b.WriteByte('"')
	b.WriteString(escapeOMLString(label))
	b.WriteByte('"')
}

func writeOMLValue(b *strings.Builder, v omnist.Value) {
	if v.IsNull {
		b.WriteString("null")
		return
	}
	writeOMLScalar(b, v.Scalar)
}

// writeOMLScalar renders an omnist.Scalar's canonical text. For omnist.KindInteger, s.Int
// is assumed non-nil: every omnist.Scalar of that kind reaching this function was
// either built by omnist.NewIntegerScalar (which always copies a non-nil
// *big.Int) or produced by Read/osd.Read, neither of which ever leaves
// Int nil for omnist.KindInteger. This mirrors the same trusted-precondition
// convention temporal.go's temporal decoders already use.
func writeOMLScalar(b *strings.Builder, s omnist.Scalar) {
	switch s.Kind {
	case omnist.KindString:
		b.WriteByte('"')
		b.WriteString(escapeOMLString(s.Str))
		b.WriteByte('"')
	case omnist.KindInteger:
		b.WriteString(s.Int.String())
	case omnist.KindNumber:
		b.WriteString(formatOMLNumber(s.Num))
	case omnist.KindBoolean:
		if s.Bool {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case omnist.KindDate:
		b.WriteString(formatOMLDate(s.Date))
	case omnist.KindTime:
		b.WriteString(formatOMLTime(s.Time))
	case omnist.KindDateTime:
		b.WriteString(formatOMLDate(s.DateTime.Date))
		b.WriteByte('T')
		b.WriteString(formatOMLTime(s.DateTime.Time))
	}
}

// formatOMLNumber renders a float64 so it re-lexes as NUMBER (spec §4.2
// rule 6, or rule 7 for the reserved spellings), never as INTEGER. NaN and
// the two infinities MUST use the reserved spellings "nan"/"inf"/"-inf"
// (§4.2 rule 7) since strconv cannot format them at all. Any other value
// that strconv would render with no '.', 'e', or 'E' (e.g. "5") is exactly
// the case that would tokenize back as INTEGER instead of NUMBER, so a
// ".0" is appended to force it into the NUMBER production.
func formatOMLNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func formatOMLDate(d omnist.DateValue) string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// formatOMLTime renders a omnist.TimeValue per the TIME production (spec §4.2's
// omnist.ISOTimeRegexp). Seconds are emitted only when Second or Nanosecond is nonzero,
// and the fractional part only when Nanosecond is nonzero — the omnist.Document
// model does not distinguish "seconds explicitly written as :00" from
// "seconds omitted", so omitting a zero seconds/fraction component is a
// reasonable, round-trip-safe canonicalization rather than a spec
// requirement.
// formatOMLTime renders a omnist.TimeValue per the canonical OML writer. Seconds
// are always emitted, even when zero (":00"): the omnist.Document model does not
// distinguish "seconds explicitly written in the source" from "seconds
// defaulted to zero" (omnist.TimeValue carries no such flag), so omitting a zero
// second field would silently drop information the value may have
// genuinely had — confirmed by
// formats-oml/basic/genuine-time-writes-bare, which pins "12:00:00" (not
// "12:00") as the canonical spelling for an exact-noon omnist.TimeValue. The
// fractional part is still omitted when Nanosecond is zero, since a
// fraction has no analogous "always show" convention to pin against.
func formatOMLTime(t omnist.TimeValue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%02d:%02d:%02d", t.Hour, t.Minute, t.Second)
	if t.Nanosecond != 0 {
		b.WriteByte('.')
		b.WriteString(formatOMLFraction(t.Nanosecond))
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

// formatOMLFraction converts a Nanosecond field back to the shortest 1-6
// digit fraction string that reproduces it. Nanosecond is always a
// product of fracToNanos (temporal.go), which right-pads a 1-6 digit
// source fraction to 9 digits with zeros — so Nanosecond is always an
// exact multiple of 1000, and trimming trailing zeros from its 6-digit
// microsecond form can never trim down to nothing given the caller's
// guard that Nanosecond != 0.
func formatOMLFraction(ns int) string {
	micros := ns / 1000
	s := fmt.Sprintf("%06d", micros)
	return strings.TrimRight(s, "0")
}

// escapeOMLString implements spec §4.5's canonical string-escaping rule:
// only \", \\, \n, \r, \t, and \u00XX (for any other control character)
// are ever emitted; \/, \b, \f, surrogate pairs, raw strings, and
// multiline strings are Extended read-only spellings the writer must
// never produce. Non-ASCII characters pass through literally.
func escapeOMLString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
