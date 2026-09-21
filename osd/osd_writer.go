package osd

import (
	"fmt"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// Write renders s as OSD text per spec §5.9's canonical-output rules,
// which — unlike OML's chapter 4 — are fully normative for the
// pretty-printed layout: one record per block, fields one per line,
// four-space indent, a trailing comma after every field including the
// last, root last, cardinality omitted when [1,1], labels always quoted,
// types never quoted.
//
// compact selects the compact mode §5.9 explicitly permits ("A compact
// mode with no indentation is permitted and MUST round-trip"). The exact
// spacing chosen for compact mode (single spaces, ", " between fields) is
// this writer's own reasonable, deterministic, self-consistent choice —
// §5.9 gives one worked example of compact mode and does not otherwise
// pin its whitespace.
//
// # Failure (OSD-14)
//
// OSD text cannot spell a field label containing a C0 control character
// (U+0000 to U+001F, tab and newline included): §5.3.1 bans the raw byte in a
// string body, escape context included, and OSD's unescaping is weak (`\X`
// yields X and nothing else). Write therefore fails, unconditionally, with
// an omnist.Diagnostic (used as the error) carrying
// omnist.CodeWriteUnsupportedValue, and returns no text. Its Path is the
// Schema path of the record holding the field ("R", not "R.<label>": §8.4
// has no way to quote such a label inside a path). Only the first
// offending record, in declaration order, is reported. A schema Write
// refuses can still travel as OSD-OML, whose escaping is real. Parsed
// schemas never trigger this; a programmatically built one, or one inferred
// from documents whose keys contain control characters, can.
//
// # Escaping (OSD-15)
//
// A backslash is written `\\` and a double quote `\"`; nothing else is escaped.
func Write(s omnist.Schema, compact bool) (string, error) {
	if err := checkWritable(s); err != nil {
		return "", err
	}
	var b strings.Builder
	if compact {
		writeOSDCompact(&b, s)
	} else {
		writeOSDPretty(&b, s)
	}
	return b.String(), nil
}

// checkWritable implements OSD-14: it reports the first record, in
// declaration order, that holds a field whose label has no OSD spelling.
func checkWritable(s omnist.Schema) error {
	for _, name := range s.EnvOrder {
		rec := s.Env[name]
		for _, f := range rec.Fields {
			if hasC0Control(f.Label) {
				return omnist.Diagnostic{
					Path:     rec.Name,
					Code:     omnist.CodeWriteUnsupportedValue,
					Message:  "a field label contains a C0 control character (U+0000..U+001F), which has no OSD spelling; write the schema as OSD-OML instead (spec §5.9, OSD-14)",
					Severity: omnist.SeverityError,
				}
			}
		}
	}
	return nil
}

// hasC0Control reports whether s contains a byte below 0x20. Every C0 code
// point is a single byte and never occurs inside a multi-byte UTF-8 sequence,
// so scanning bytes is exact.
func hasC0Control(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 {
			return true
		}
	}
	return false
}

func writeOSDPretty(b *strings.Builder, s omnist.Schema) {
	for _, name := range s.EnvOrder {
		rec := s.Env[name]
		fmt.Fprintf(b, "record %s {\n", rec.Name)
		for _, f := range rec.Fields {
			b.WriteString("    ")
			writeOSDField(b, f)
			b.WriteString(",\n")
		}
		b.WriteString("}\n")
	}
	fmt.Fprintf(b, "root %s\n", s.Root)
}

func writeOSDCompact(b *strings.Builder, s omnist.Schema) {
	for _, name := range s.EnvOrder {
		rec := s.Env[name]
		fmt.Fprintf(b, "record %s {", rec.Name)
		if len(rec.Fields) > 0 {
			b.WriteByte(' ')
			for j, f := range rec.Fields {
				if j > 0 {
					b.WriteString(", ")
				}
				writeOSDField(b, f)
			}
			b.WriteByte(' ')
		}
		b.WriteString("} ")
	}
	fmt.Fprintf(b, "root %s", s.Root)
}

func writeOSDField(b *strings.Builder, f omnist.Field) {
	b.WriteByte('"')
	b.WriteString(escapeOSDLabel(f.Label))
	b.WriteByte('"')
	b.WriteString(osdCardinalityString(f.Cardinality))
	b.WriteString(": ")
	b.WriteString(osdTypeString(f.Type))
}

// osdCardinalityString implements §5.9's "cardinality omitted when [1,1]"
// rule. When not omitted, this always emits the two-bound bracket form
// ("[min,max]" or, for an unbounded field, "[min,]") rather than trying to
// reproduce whichever of the grammar's five equivalent bracketed spellings
// (spec §5.5's table) produced the original omnist.Cardinality — the omnist.Schema model
// only records (Min, Max, Unbounded), not which spelling was parsed, and
// every one of those forms is losslessly reconstructible from that triple
// alone, so there is exactly one canonical spelling to pick per triple.
// The leading space is part of this string (or the empty string when
// omitted) so callers can concatenate it directly after the label.
func osdCardinalityString(c omnist.Cardinality) string {
	if !c.Unbounded && c.Min == 1 && c.Max == 1 {
		return ""
	}
	if c.Unbounded {
		return fmt.Sprintf(" [%d,]", c.Min)
	}
	if c.Min == c.Max {
		return fmt.Sprintf(" [%d]", c.Min)
	}
	return fmt.Sprintf(" [%d,%d]", c.Min, c.Max)
}

func osdTypeString(t omnist.Type) string {
	switch t.Kind {
	case omnist.TypeAnyKind:
		return "any"
	case omnist.TypeRefKind:
		return t.RefName
	default: // omnist.TypeScalarKind
		s := t.ScalarKind.String()
		if t.Nullable {
			s += "?"
		}
		return s
	}
}

// escapeOSDLabel implements OSD-15, the writer-side half of spec §5.3.1's
// weak string-unescaping rule: the reader recognizes exactly one escape shape,
// a backslash followed by X yielding X, so the canonical spelling escapes a
// backslash as \\ and a double quote as \" and NOTHING else. Escaping an
// ordinary character is harmless on read but would break OSD-11's
// byte-identical guarantee between two writers that disagree about which
// characters to escape. There is no named-escape table: a writer that emitted
// \n for a newline would read back as the letter n. A label containing a C0
// control character never reaches this function; Write refuses it first
// (OSD-14).
//
// It scans bytes, not runes: a backslash and a quote are single bytes that
// never occur inside a multi-byte UTF-8 sequence, and every other byte passes
// through untouched, so nothing is decoded and nothing is repaired.
func escapeOSDLabel(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
