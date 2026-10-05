package xml

import (
	"strings"
	"testing"
	"unicode/utf8"

	omnist "github.com/omnist-dev/omnist-go"
)

// inXMLChar is the XML 1.0 (Fifth Edition) Char production, written out
// independently of the writer so the tests below check the writer against the
// definition and not against itself:
//
//	Char ::= #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] | [#x10000-#x10FFFF]
func inXMLChar(r rune) bool {
	switch {
	case r == 0x9, r == 0xA, r == 0xD:
		return true
	case r >= 0x20 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	}
	return false
}

func isSurrogate(r rune) bool { return r >= 0xD800 && r <= 0xDFFF }

// A string value holding a code point outside the XML 1.0 Char production has
// no XML spelling (spec E-6: a string containing a character the target format
// cannot represent at all). The writer must fail with write.unsupported-value
// at the leaf rather than alter the character (encoding/xml silently turns
// U+FFFE and U+FFFF into U+FFFD) -- issue #133. Every code point is checked, so
// no future gap between this writer and the Char production goes unnoticed;
// U+FFFD itself is a legal Char and must pass through unchanged.
func TestWriteValueCodePointsMatchXMLCharProduction(t *testing.T) {
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if isSurrogate(r) {
			continue // not encodable in UTF-8; C-9 covers these (xml_writer_c9_test.go)
		}
		s := "a" + string(r) + "b"
		d := omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().AddValue("s", omnist.ScalarValue(omnist.NewStringScalar(s)))))
		out, _, err := Write(d)
		if inXMLChar(r) {
			if err != nil {
				t.Fatalf("U+%04X is an XML Char but the write failed: %v", r, err)
			}
			if strings.ContainsRune(out, 0xFFFD) != (r == 0xFFFD) {
				t.Fatalf("U+%04X: output %q has a U+FFFD the value did not carry (or lost the one it did)", r, out)
			}
			continue
		}
		diag, ok := err.(omnist.Diagnostic)
		if !ok || diag.Code != omnist.CodeWriteUnsupportedValue || diag.Path != "$.root.s" {
			t.Fatalf("U+%04X is not an XML Char: got out=%q err=%#v, want write.unsupported-value at $.root.s", r, out, err)
		}
		if !strings.Contains(diag.Message, "U+") {
			t.Fatalf("U+%04X: message should name the character: %q", r, diag.Message)
		}
	}
}

// The failure is reported at the value's own path, indexed when its label
// repeats (E-10), and whichever of the non-characters is at fault.
func TestWriteNoncharacterPaths(t *testing.T) {
	str := func(s string) omnist.Value { return omnist.ScalarValue(omnist.NewStringScalar(s)) }
	for _, bad := range []string{"\uFFFE", "\uFFFF", "ab\uFFFEcd", "\uFFFF\uFFFF"} {
		d := omnist.NodeDocument(omnist.NewNode().AddNode("root",
			omnist.NewNode().AddValue("item", str("ok")).AddValue("item", str(bad))))
		_, _, err := Write(d)
		diag, ok := err.(omnist.Diagnostic)
		if !ok || diag.Code != omnist.CodeWriteUnsupportedValue || diag.Path != "$.root.item[1]" {
			t.Errorf("%q: got %#v, want write.unsupported-value at $.root.item[1]", bad, err)
		}
	}
	// U+10FFFF is the last code point and a legal Char; U+FFFD is the
	// replacement character itself and must be written as is.
	for _, good := range []string{"\U0010FFFF", "\uFFFD", "\uE000", "\uD7FF"} {
		d := omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().AddValue("s", str(good))))
		out, _, err := Write(d)
		if err != nil || !strings.Contains(out, good) {
			t.Errorf("%q: got %q, %v, want it written unchanged", good, out, err)
		}
	}
}

// A label with a code point outside the Char production is not a valid element
// name either (isValidXMLName is stricter than the Name production), so the
// write fails there; this pins that no non-Char label reaches the output.
func TestWriteLabelCodePointsOutsideXMLChar(t *testing.T) {
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if isSurrogate(r) || inXMLChar(r) {
			continue
		}
		d := omnist.NodeDocument(omnist.NewNode().AddNode("a"+string(r)+"b", omnist.NewNode().AddValue("s", omnist.ScalarValue(omnist.NewStringScalar("x")))))
		out, _, err := Write(d)
		diag, ok := err.(omnist.Diagnostic)
		if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
			t.Fatalf("label with U+%04X: got out=%q err=%#v, want write.unsupported-value", r, out, err)
		}
	}
}

// BenchmarkWriteLargeDocument writes 200,000 string leaves (the shape the C-9
// scan overhead was measured on); the per-leaf Char check is its extra cost.
func BenchmarkWriteLargeDocument(b *testing.B) {
	root := omnist.NewNode()
	for i := 0; i < 200000; i++ {
		root.AddValue("item", omnist.ScalarValue(omnist.NewStringScalar("some ordinary text value "+strings.Repeat("x", i%40))))
	}
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", root))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Write(d); err != nil {
			b.Fatal(err)
		}
	}
}
