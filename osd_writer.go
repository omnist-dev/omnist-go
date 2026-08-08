package omnist

import (
	"fmt"
	"strings"
)

// WriteOSD renders s as OSD text per spec §5.9's canonical-output rules,
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
func WriteOSD(s Schema, compact bool) string {
	var b strings.Builder
	if compact {
		writeOSDCompact(&b, s)
	} else {
		writeOSDPretty(&b, s)
	}
	return b.String()
}

func writeOSDPretty(b *strings.Builder, s Schema) {
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

func writeOSDCompact(b *strings.Builder, s Schema) {
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

func writeOSDField(b *strings.Builder, f Field) {
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
// (spec §5.5's table) produced the original Cardinality — the Schema model
// only records (Min, Max, Unbounded), not which spelling was parsed, and
// every one of those forms is losslessly reconstructible from that triple
// alone, so there is exactly one canonical spelling to pick per triple.
// The leading space is part of this string (or the empty string when
// omitted) so callers can concatenate it directly after the label.
func osdCardinalityString(c Cardinality) string {
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

func osdTypeString(t Type) string {
	switch t.Kind {
	case TypeAnyKind:
		return "any"
	case TypeRefKind:
		return t.RefName
	default: // TypeScalarKind
		s := t.ScalarKind.String()
		if t.Nullable {
			s += "?"
		}
		return s
	}
}

// escapeOSDLabel implements the writer-side half of spec §5.3.1's weak
// string-unescaping rule. This is the genuine trap the issue calls out:
// the reader recognizes exactly two meaningful escapes, \\ -> \ and
// \" -> ", and otherwise writes whatever character follows a backslash
// verbatim (including a literal newline or other control character) —
// there is no named-escape table, so a writer that (wrongly) emitted
// "\n" for a literal newline byte would round-trip it back as the two
// characters 'n', not a newline.
//
// So this function escapes only backslash and double-quote using the two
// meaningful escapes, and escapes any other control character (< 0x20,
// which the reader's tokenizer rejects unescaped as
// parse.control-character) by emitting a backslash followed by that exact
// control character byte — relying on the reader's "whatever follows a
// backslash is written verbatim" rule to reproduce it exactly. Every other
// character, ASCII or not, passes through completely literally.
func escapeOSDLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r < 0x20:
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
