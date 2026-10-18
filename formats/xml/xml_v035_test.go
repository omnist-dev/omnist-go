package xml

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/internal/textpos"
	"github.com/omnist-dev/omnist-go/osd"
)

func xmlParseErr(t *testing.T, src string) *omnist.ParseError {
	t.Helper()
	_, _, err := Read(src, omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("Read(%q) error = %v (%T), want *ParseError", src, err, err)
	}
	return pe
}

// C-16: numeric references must name an XML Char; comments and processing
// instructions hold only Chars.
func TestCharacterReferencesAndMarkupCharacters(t *testing.T) {
	bad := []string{
		"<a>&#xD800;</a>", "<a>&#55296;</a>", "<a>&#0;</a>", "<a>&#00;</a>",
		"<a>&#x110000;</a>", "<a>&#x1000000000;</a>", "<a>&#;</a>", "<a>&#65</a>",
		"<a>&#X41;</a>", "<a x=\"&#1;\">y</a>",
		"<a><!-- p\x01q -->y</a>", "<a><?t p\x01q?>y</a>",
	}
	for _, src := range bad {
		if pe := xmlParseErr(t, src); pe.Code != omnist.CodeParseCodecSyntax {
			t.Errorf("%q: code = %s, want parse.codec-syntax", src, pe.Code)
		}
	}
	// A reference inside a CDATA section, comment or processing instruction is
	// literal text, not a reference; a valid Char reference reads as itself.
	good := map[string]string{
		"<a><![CDATA[&#xD800; &#1;]]></a>": "&#xD800; &#1;",
		"<a>&#xE000;</a>":                  "",
		"<a>&#x0000041;</a>":               "A",
		"<a>&#xFFFD;</a>":                  "�",
		"<a><!-- &#1; -->x</a>":            "x",
		"<a><?t &#1;?>x</a>":               "x",
	}
	for src, want := range good {
		doc, _, err := Read(src, omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if v, _ := doc.Node.Edges[0].Target.Value(); v.Scalar.Str != want {
			t.Errorf("%q: got %q, want %q", src, v.Scalar.Str, want)
		}
	}
	// The error is positioned at the offending reference (code points).
	if pe := xmlParseErr(t, "<a>é&#xD800;</a>"); pe.Line != 1 || pe.Col != 5 {
		t.Errorf("position = %d:%d, want 1:5", pe.Line, pe.Col)
	}
}

// The entity sentinel must not be a code point a numeric reference produces.
func TestEntitySentinelAvoidsNumericReferences(t *testing.T) {
	named, err := scanXMLCharacters("<a>&#xE000;&#57345;</a>", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := entitySentinel("<a/>", named); r != 0xE002 {
		t.Errorf("sentinel = %U, want U+E002", r)
	}
	doc, _, rerr := Read("<a>&#xE000;</a>", omnist.DefaultLimits())
	if rerr != nil {
		t.Fatal(rerr)
	}
	if v, _ := doc.Node.Edges[0].Target.Value(); v.Scalar.Str != "" {
		t.Errorf("got %q", v.Scalar.Str)
	}
}

// The encoding declaration is ignored: the input is read as UTF-8 (D-14).
func TestEncodingDeclarationIsIgnored(t *testing.T) {
	for _, enc := range []string{"ISO-8859-1", "UTF-16", "US-ASCII", "no-such-encoding", "utf-8"} {
		src := `<?xml version="1.0" encoding="` + enc + `"?><a>é</a>`
		doc, _, err := Read(src, omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("%s: %v", enc, err)
		}
		if v, _ := doc.Node.Edges[0].Target.Value(); v.Scalar.Str != "é" {
			t.Errorf("%s: got %q", enc, v.Scalar.Str)
		}
	}
}

// E-4a: a schema-typed integer leaf over the digit limit is refused at the
// leaf's Document path.
func TestPretypedIntegerOverTheDigitLimit(t *testing.T) {
	schema, err := osd.Read(`
record R { "n" [0,]: integer, "s": string }
record Root { "r": R, "m": integer }
root Root
`)
	if err != nil {
		t.Fatal(err)
	}
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 5
	long := strings.Repeat("9", 6)
	cases := []struct {
		src, path string
	}{
		{"<r><n>" + long + "</n></r>", "$.r.n"},
		{"<r><n>1</n><n>" + long + "</n><n>" + long + "7</n></r>", "$.r.n[1]"},
		{"<r><n>-" + long + "</n></r>", "$.r.n"},
		{"<m>" + long + "</m>", "$.m"},
	}
	for _, c := range cases {
		_, _, err := ReadWithSchema(c.src, &schema, limits)
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits || pe.Path != c.path {
			t.Errorf("%.30q: err = %+v, want document.limit.int-digits at %s", c.src, err, c.path)
		}
	}
	// Within the limit, leading zeros do not count, and a non-integer text is
	// left to the ordinary fallback (a string).
	for _, src := range []string{
		"<r><n>00000" + "12345</n></r>", "<r><n>000000</n></r>", "<r><n>-0</n></r>",
		"<r><n>" + long + "x</n></r>", "<r><n>+" + long + "</n></r>", "<r><n>+-1</n></r>", "<r><n>+</n></r>", "<r><s>" + long + "</s></r>",
	} {
		if _, _, err := ReadWithSchema(src, &schema, limits); err != nil {
			t.Errorf("%.30q: %v", src, err)
		}
	}
}

// The pre-scan stops quietly at an unterminated opener (the decoder rejects
// the document) and parses hex digits of either case.
func TestScanXMLCharactersEdges(t *testing.T) {
	for _, src := range []string{"<a><?pi", "<a><!-- x", "<a><![CDATA[ x", "<a>&#x4A;&#xbc;</a>"} {
		if _, err := scanXMLCharacters(src, textpos.NewIndex(src)); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	for _, src := range []string{"<a>&#6a;</a>", "<a>&#xg;</a>", "<a>&#x4G;</a>"} {
		if _, err := scanXMLCharacters(src, textpos.NewIndex(src)); err == nil {
			t.Errorf("%q: want an error", src)
		}
	}
}
