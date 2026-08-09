package omnist

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// WriteXML renders d as XML text (spec §7.3, docs/formats/xml.md),
// schema-free: a writer MUST NOT accept a schema (§7.3), and this one
// doesn't.
//
// # No grouping — the write-side half of interleaving preservation
//
// Every other writer in this package (writeTOMLTopLevel/writeJSONNode/
// buildYAMLNode) implements §7.3.1's write(node, format) by first grouping
// edges sharing a label (groupTOMLEdges/groupJSONEdges/groupYAMLEdges),
// because JSON/YAML/TOML's own syntax has no way to express two edges of
// the same label as anything but one key holding a list — §7.3 states this
// explicitly ("no format in the JSON family can express [interleaving]").
// XML has no such limitation: sibling elements are already a plain ordered
// sequence with no uniqueness constraint on tag names, so this writer skips
// grouping entirely and emits writeXMLElement once per Edge, in exact Node
// order — an edge list [(m,A),(x,X),(m,B)] therefore writes as
// `<m>A</m><x>X</x><m>B</m>`, not `<m>A</m><m>B</m><x>X</x>`. This is the
// write-side half of the interleaving-preservation property
// docs/formats/xml.md and xml_reader.go's ReadXML doc comment describe;
// see TestWriteXMLPreservesInterleaving for the test exercising it, and
// docs/formats/xml.md: "the writer emits edges as literal sibling elements
// in their exact source order."
//
// # Single document element: a real write-time error
//
// docs/formats/xml.md: "A Document with several top-level edges cannot be
// written as XML." XML's grammar requires exactly one root element; a
// Document whose root Node has zero or more-than-one top-level edges has no
// faithful single-element XML spelling. WriteXML checks this before writing
// anything: more than one top-level edge fails with CodeFormatMultipleRoots
// (spec §8.3.8/9.4, the taxonomy code named for exactly this situation — see
// this issue's design-continuity note confirming it already exists). Zero
// top-level edges (an empty Node) is a different, narrower case the
// taxonomy has no dedicated name for — there's no "multiple" root here,
// just nothing to serve as the one required root — so it reuses
// CodeWriteUnsupportedValue, the same general "this Document shape has no
// spelling in this format" code the bare-scalar-root case below already
// uses; both are "no candidate root element" situations, just for two
// different reasons.
//
// # Bare-scalar-root: a real error, not malformed output
//
// Mirroring issue #27's TOML precedent (see WriteTOML's doc comment):
// docs/formats/xml.md doesn't discuss a bare-scalar Document explicitly,
// but XML's grammar has no representation for "a value with no wrapping
// element" any more than TOML's has for "a value with no key" — every XML
// document is fundamentally one element. WriteXML checks d.IsNode before
// writing anything and fails immediately (CodeWriteUnsupportedValue,
// staying consistent with WriteTOML's identical call) if the root is a
// scalar Value rather than a Node.
func WriteXML(d Document) (string, error) {
	if !d.IsNode {
		return "", Diagnostic{
			Path:     "$",
			Code:     CodeWriteUnsupportedValue,
			Message:  "an XML document's top level must be a single element; a bare scalar Document has no XML spelling",
			Severity: SeverityError,
		}
	}
	switch len(d.Node.Edges) {
	case 0:
		return "", Diagnostic{
			Path:     "$",
			Code:     CodeWriteUnsupportedValue,
			Message:  "an XML document needs exactly one top-level element; this Document has none",
			Severity: SeverityError,
		}
	case 1:
		// fall through
	default:
		return "", Diagnostic{
			Path:     "$",
			Code:     CodeFormatMultipleRoots,
			Message:  "an XML document needs exactly one top-level element; this Document has more than one top-level edge",
			Severity: SeverityError,
		}
	}

	root := d.Node.Edges[0]
	var b strings.Builder
	if err := writeXMLElement(&b, root.Label, root.Target, "$."+root.Label); err != nil {
		return "", err
	}
	return b.String(), nil
}

// writeXMLElement renders one Edge as a `<label>...</label>` element (or
// the self-closing `<label/>` for an empty-string leaf — a plain,
// unremarkable strconv-level rendering choice, not a distinct case that
// needs its own branch: writeXMLScalarText already returns "" for an
// empty-string KindString leaf, and xml.EscapeText writes nothing for an
// empty byte slice, so the element naturally comes out as `<label></label>`
// either way; nothing here special-cases self-closing form specifically).
func writeXMLElement(b *strings.Builder, label string, t Target, path string) error {
	if !isValidXMLName(label) {
		return Diagnostic{
			Path:     path,
			Code:     CodeWriteUnsupportedValue,
			Message:  fmt.Sprintf("label %q is not a valid XML element name", label),
			Severity: SeverityError,
		}
	}

	if node, ok := t.Node(); ok {
		b.WriteByte('<')
		b.WriteString(label)
		b.WriteByte('>')
		for _, e := range node.Edges {
			if err := writeXMLElement(b, e.Label, e.Target, path+"."+e.Label); err != nil {
				return err
			}
		}
		b.WriteString("</")
		b.WriteString(label)
		b.WriteByte('>')
		return nil
	}

	v, _ := t.Value()
	b.WriteByte('<')
	b.WriteString(label)
	b.WriteByte('>')
	if v.IsNull {
		// docs/formats/xml.md does not discuss null explicitly, but XML
		// text has no spelling for "absent" distinct from "empty" any more
		// than TOML has a spelling for null at all (see WriteTOML's doc
		// comment on CodeFormatNullUnrepresentable) — the plainly-correct,
		// narrow/cosmetic reading taken here is the same one: report the
		// same warning-severity adjustment code TOML's writer already
		// uses for "this leaf has no representation in this format, so it
		// is written as empty/dropped", applied to XML's own empty-element
		// spelling instead of TOML's outright omission.
		diag := Diagnostic{
			Path:     path,
			Code:     CodeFormatNullUnrepresentable,
			Message:  "a null leaf cannot be written in XML, so it is written as an empty element",
			Severity: SeverityWarning,
		}
		b.WriteString("</")
		b.WriteString(label)
		b.WriteByte('>')
		return diag
	}
	// xml.EscapeText's only failure mode is its io.Writer returning an
	// error; xmlTextWriter wraps a *strings.Builder, whose Write never
	// does (per xmlTextWriter's own doc comment), so this error is never
	// non-nil for any input — mirroring this package's established
	// no-dead-branch convention (see e.g. toml_reader.go's parseTOMLInt
	// doc comment) rather than carrying a permanently-unreachable check.
	_ = xml.EscapeText(&xmlTextWriter{b}, []byte(writeXMLScalarText(v.Scalar)))
	b.WriteString("</")
	b.WriteString(label)
	b.WriteByte('>')
	return nil
}

// xmlTextWriter adapts *strings.Builder to io.Writer for xml.EscapeText,
// which wants an io.Writer, not the strings.Builder itself (which already
// satisfies io.Writer via its Write method — this type exists purely so
// call sites read as an explicit, deliberate choice of xml.EscapeText's
// escaping rules rather than an incidental one; strings.Builder.Write
// never returns an error, so the adapter cannot introduce a new failure
// mode).
type xmlTextWriter struct{ b *strings.Builder }

func (w *xmlTextWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// writeXMLScalarText renders one leaf Scalar as plain text — XML text is
// always untyped (docs/formats/xml.md), so unlike WriteJSON/WriteTOML/
// WriteYAML there is no format-native spelling to choose between for any
// kind: every kind reduces to a string, the same conversion ReadXML's own
// leaves would already have produced had this text been read back in.
func writeXMLScalarText(s Scalar) string {
	switch s.Kind {
	case KindString:
		return s.Str
	case KindInteger:
		return s.Int.String()
	case KindNumber:
		return writeXMLNumberText(s.Num)
	case KindBoolean:
		if s.Bool {
			return "true"
		}
		return "false"
	case KindDate:
		return FormatISODate(s.Date)
	case KindTime:
		return FormatISOTime(s.Time)
	default: // KindDateTime
		return FormatISODate(s.DateTime.Date) + "T" + FormatISOTime(s.DateTime.Time)
	}
}

// writeXMLNumberText renders a KindNumber leaf as text. Unlike
// WriteJSON/WriteJSONStrict, NaN/Infinity need no substitution or strict
// failure mode here — they are being written as plain text, which can hold
// any string at all, so their ordinary Go spelling ("NaN", "+Inf", "-Inf")
// is used directly, mirroring how any other unrepresentable-as-a-native-
// type value already becomes text in this writer.
func writeXMLNumberText(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	default:
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}

// isValidXMLName reports whether label is safe to emit as a bare XML
// element name. docs/formats/xml.md never discusses what happens when a
// Document's label isn't a legal XML Name in the first place (a JSON/YAML
// key can be any string at all, quoted; TOML's writer sidesteps this by
// always quoting too — see writeTOMLKey's doc comment — but XML element
// names have no quoted alternative spelling, so an arbitrary label cannot
// always be written). This is a narrow, cosmetic gap: the plainly-correct
// reading is to validate before writing and fail cleanly
// (CodeWriteUnsupportedValue) rather than emit malformed XML, using a
// deliberately conservative (ASCII-only) subset of the real XML 1.0 Name
// grammar — which also permits many non-ASCII code points — since no
// Document produced by any reader in this package (or by hand in this
// issue's own tests) exercises that wider range, and a conservative
// under-approximation only ever rejects labels a fuller check would also
// need to accept, never the reverse for any input this package's own
// round-trip tests construct. A colon is deliberately excluded even though
// XML's own Name grammar allows it (colons are reserved for namespace
// prefixes there); allowing a literal colon through here could make a
// written label look, on a subsequent read, like a namespace-prefixed
// name whose prefix ReadXML would then silently drop — a surprising,
// asymmetric round-trip failure this writer avoids by refusing to emit one
// in the first place.
func isValidXMLName(label string) bool {
	if label == "" {
		return false
	}
	for i, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			// valid anywhere
		case r >= '0' && r <= '9', r == '-', r == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
