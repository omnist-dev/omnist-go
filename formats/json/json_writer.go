package json

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// Write renders d as JSON text (spec section 7.3, docs/formats/json.md),
// schema-free: a writer MUST NOT accept a schema (section 7.3), and this
// one doesn't. It applies section 7.3's two rules (grouping by label, the
// count-1 bare-value-vs-list rule) via writeJSONNode/writeJSONGroup
// below, plus JSON-specific leaf rendering: temporal leaves are
// stringified to ISO-8601, and NaN/Infinity -- not valid JSON tokens --
// now fail the write unconditionally (spec section 8.3.8/8.3.9, updated
// 2026-08-24, issue #98). Grouping itself can also lose cross-label
// interleaving; see groupJSONEdges's doc comment.
//
// # NaN/Infinity: unconditional failure, not a lenient substitution
//
// A previous version of this writer substituted `null` for NaN/Infinity
// by default, with a `WriteStrict` variant that failed instead (the
// spec's then-optional MAY: "a strict mode MAY instead fail"). That
// default was removed per spec section 8.3.8/8.3.9: writing a genuine
// `null` value and writing NaN produce the identical JSON token, and both
// read back as the identical Document value -- there is no way, after the
// fact, to tell a substituted NaN from an original null. The correct
// behavior is write.unsupported-value, unconditionally, matching the
// null-unrepresentable fix for TOML (issue #97) and XML (issue #96/#97)
// and the label-sanitization fix (issue #96) -- the same "does the
// fallback collide with a genuinely different, independently-valid
// input" test applied consistently.
//
// The signature still returns diagnostics alongside text and error
// because JSON's other adjustment remains reportable per spec section 7.4
// ("a reader or writer SHOULD be able to report the adjustments... this
// is what makes lossiness auditable") and section 8.5.3, which documents
// write as the one operation where a successful `{ok: true, ...}` result
// and a non-empty diagnostics list are not mutually exclusive: every
// temporal leaf stringified to ISO-8601 (format.temporal-stringified) and
// every grouping that loses cross-label interleaving
// (format.interleaving-lost, spec section 8.3.8, D-3) is reported this
// way.
func Write(d omnist.Document) (string, []omnist.Diagnostic, error) {
	return writeJSONDocument(d)
}

// WriteStrict is Deprecated: NaN/Infinity now fails unconditionally in
// Write (spec section 8.3.8/8.3.9, updated 2026-08-24, issue #98), so
// WriteStrict is now functionally identical to Write -- the distinction
// this function used to draw (lenient substitution vs. strict failure)
// no longer exists. Kept only for source compatibility with existing
// callers; new code should call Write directly.
func WriteStrict(d omnist.Document) (string, []omnist.Diagnostic, error) {
	return writeJSONDocument(d)
}

func writeJSONDocument(d omnist.Document) (string, []omnist.Diagnostic, error) {
	var b strings.Builder
	var diags []omnist.Diagnostic
	if d.IsNode {
		if err := writeJSONNode(&b, d.Node, "$", &diags); err != nil {
			return "", nil, err
		}
		return b.String(), diags, nil
	}
	if err := writeJSONValue(&b, d.Value, "$", &diags); err != nil {
		return "", nil, err
	}
	return b.String(), diags, nil
}

// jsonGroup is one label's worth of grouped edges, in the order
// write(node, format)'s pseudocode (spec §7.3.1) builds `groups`: keyed by
// label, first-seen label order, and — within a label — original edge
// order.
type jsonGroup struct {
	label    string
	children []omnist.Target
}

// writeJSONNode implements §7.3.1's write(node, format) for the JSON
// format: group edges sharing a label (grouping rule), then render each
// group as a bare value when it has exactly one child or as a list
// otherwise (count-1 rule).
func writeJSONNode(b *strings.Builder, n *omnist.Node, path string, diags *[]omnist.Diagnostic) error {
	groups := groupJSONEdges(n)
	if n.HasLostInterleaving() {
		*diags = append(*diags, omnist.Diagnostic{
			Path:     path,
			Code:     omnist.CodeFormatInterleavingLost,
			Message:  "cross-label interleaving cannot be expressed in JSON, so it is lost",
			Severity: omnist.SeverityWarning,
		})
	}

	b.WriteByte('{')
	for i, g := range groups {
		if i > 0 {
			b.WriteString(", ")
		}
		writeJSONString(b, g.label)
		b.WriteString(": ")
		childPath := path + "." + g.label

		if len(g.children) == 1 {
			if err := writeJSONTarget(b, g.children[0], childPath, diags); err != nil {
				return err
			}
			continue
		}
		b.WriteByte('[')
		for j, t := range g.children {
			if j > 0 {
				b.WriteString(", ")
			}
			if err := writeJSONTarget(b, t, fmt.Sprintf("%s[%d]", childPath, j), diags); err != nil {
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
// states JSON must ("no format in the JSON family can express it") --
// writeJSONNode reports this loss via omnist.CodeFormatInterleavingLost
// (spec §8.3.8, D-3) whenever it actually happens (omnist.Node.HasLostInterleaving,
// document.go), rather than silently, as it did before.
func groupJSONEdges(n *omnist.Node) []jsonGroup {
	var groups []jsonGroup
	index := make(map[string]int, len(n.Edges))
	for _, e := range n.Edges {
		if i, ok := index[e.Label]; ok {
			groups[i].children = append(groups[i].children, e.Target)
			continue
		}
		index[e.Label] = len(groups)
		groups = append(groups, jsonGroup{label: e.Label, children: []omnist.Target{e.Target}})
	}
	return groups
}

func writeJSONTarget(b *strings.Builder, t omnist.Target, path string, diags *[]omnist.Diagnostic) error {
	if node, ok := t.Node(); ok {
		return writeJSONNode(b, node, path, diags)
	}
	v, _ := t.Value()
	return writeJSONValue(b, v, path, diags)
}

func writeJSONValue(b *strings.Builder, v omnist.Value, path string, diags *[]omnist.Diagnostic) error {
	if v.IsNull {
		b.WriteString("null")
		return nil
	}
	return writeJSONScalar(b, v.Scalar, path, diags)
}

// writeJSONScalar renders one leaf. Per docs/formats/json.md: "a writer
// MUST stringify a temporal leaf to ISO-8601" (KindDate/KindTime/
// KindDateTime), and NaN/Infinity (KindNumber only -- JSON's only
// floating kind) now fail the write unconditionally (spec section
// 8.3.8/8.3.9, issue #98) rather than substituting -- see Write's doc
// comment. For KindInteger, s.Int is assumed non-nil, mirroring
// oml_writer.go's writeOMLScalar precondition: every omnist.Scalar of that kind
// reaching this function was built by omnist.NewIntegerScalar (which always
// copies a non-nil *big.Int) or produced by ReadJSON, neither of which
// ever leaves Int nil for KindInteger.
func writeJSONScalar(b *strings.Builder, s omnist.Scalar, path string, diags *[]omnist.Diagnostic) error {
	switch s.Kind {
	case omnist.KindString:
		writeJSONString(b, s.Str)
	case omnist.KindInteger:
		b.WriteString(s.Int.String())
	case omnist.KindNumber:
		return writeJSONNumber(b, s.Num, path)
	case omnist.KindBoolean:
		if s.Bool {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case omnist.KindDate:
		writeJSONString(b, omnist.FormatISODate(s.Date))
		*diags = append(*diags, omnist.Diagnostic{
			Path:     path,
			Code:     omnist.CodeFormatTemporalStringified,
			Message:  "a date leaf has no native JSON representation, so it is stringified to ISO-8601",
			Severity: omnist.SeverityWarning,
		})
	case omnist.KindTime:
		writeJSONString(b, omnist.FormatISOTime(s.Time))
		*diags = append(*diags, omnist.Diagnostic{
			Path:     path,
			Code:     omnist.CodeFormatTemporalStringified,
			Message:  "a time leaf has no native JSON representation, so it is stringified to ISO-8601",
			Severity: omnist.SeverityWarning,
		})
	case omnist.KindDateTime:
		writeJSONString(b, omnist.FormatISODate(s.DateTime.Date)+"T"+omnist.FormatISOTime(s.DateTime.Time))
		*diags = append(*diags, omnist.Diagnostic{
			Path:     path,
			Code:     omnist.CodeFormatTemporalStringified,
			Message:  "a datetime leaf has no native JSON representation, so it is stringified to ISO-8601",
			Severity: omnist.SeverityWarning,
		})
	}
	return nil
}

// writeJSONNumber renders a KindNumber leaf. A finite value always gets a
// decimal point or exponent (never a bare integer-looking spelling) so
// that ReadJSON's own integer/number split -- decided purely by the
// literal's shape -- reads it back as a number, not an integer; without
// this, writing 5.0 as the JSON text `5` would silently flip its kind on
// round-trip. NaN/Infinity have no valid JSON spelling at all (spec: "not
// valid JSON... a writer MUST NOT emit them") and now fail the write
// unconditionally (spec section 8.3.8/8.3.9, issue #98) -- see Write's
// doc comment for why the old lenient-substitution default was removed.
func writeJSONNumber(b *strings.Builder, f float64, path string) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return omnist.Diagnostic{
			Path:     path,
			Code:     omnist.CodeWriteUnsupportedValue,
			Message:  "NaN/Infinity has no JSON representation",
			Severity: omnist.SeverityError,
		}
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	b.WriteString(s)
	return nil
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
