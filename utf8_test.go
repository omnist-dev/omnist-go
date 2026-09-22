package omnist_test

// Spec §2.5 D-14: input MUST be valid UTF-8, reported as parse.invalid-encoding
// at 1:1 on every surface, before D-15's BOM strip and D-21's second-mark check.
// Go's string is a byte sequence, so the testable rule (§2.5) is: every reader
// input s with utf8.ValidString(s) == false is rejected. Every case here is
// written as bytes (\x escapes), never as a Go rune literal a compiler could
// normalise.

import (
	"strings"
	"testing"
	"unicode/utf8"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
	"github.com/omnist-dev/omnist-go/osd"
)

// wrappers place raw bytes inside plausible content on each surface (mid-string,
// where the vectors put them), so the rejection cannot be a side effect of the
// grammar failing on a dangling byte.
type utf8Surface struct {
	name string
	wrap func(bad string) string
	read func(string) error
}

func utf8Surfaces() []utf8Surface {
	l := omnist.DefaultLimits()
	return []utf8Surface{
		{"oml", func(b string) string { return "a: \"x" + b + "y\"\n" }, func(s string) error { _, e := oml.Read(s, l); return e }},
		{"json", func(b string) string { return "{\"a\": \"x" + b + "y\"}" }, func(s string) error { _, e := json.Read(s, l); return e }},
		{"yaml", func(b string) string { return "a: \"x" + b + "y\"\n" }, func(s string) error { _, e := yaml.Read(s, l); return e }},
		{"toml", func(b string) string { return "a = \"x" + b + "y\"\n" }, func(s string) error { _, e := toml.Read(s, l); return e }},
		{"xml", func(b string) string { return "<r><a>x" + b + "y</a></r>" }, func(s string) error { _, _, e := xml.Read(s, l); return e }},
		{"osd", func(b string) string { return "record R {\n    \"x" + b + "y\": string,\n}\nroot R\n" }, func(s string) error { _, e := osd.Read(s); return e }},
	}
}

func wantInvalidEncoding(t *testing.T, what string, err error) {
	t.Helper()
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Errorf("%s: err = %#v, want *omnist.ParseError parse.invalid-encoding at 1:1", what, err)
		return
	}
	if pe.Code != omnist.CodeParseInvalidEncoding || pe.Path != "1:1" || pe.Line != 1 || pe.Col != 1 {
		t.Errorf("%s: got %s at %s (%d:%d), want parse.invalid-encoding at 1:1", what, pe.Code, pe.Path, pe.Line, pe.Col)
	}
}

func TestD14RejectsInvalidUTF8OnEverySurface(t *testing.T) {
	bad := []struct{ name, bytes string }{
		{"truncated three-byte sequence", "\xe2\x82"},
		{"truncated two-byte sequence", "\xc3"},
		{"truncated four-byte sequence", "\xf0\x9f\x98"},
		{"overlong encoding", "\xc0\xaf"},
		{"overlong three-byte encoding", "\xe0\x80\xaf"},
		{"lone continuation byte", "\x80"},
		{"encoded surrogate", "\xed\xa0\x80"},
		{"byte above the Unicode range", "\xf5\x80\x80\x80"},
		{"code point above U+10FFFF", "\xf4\x90\x80\x80"},
		{"0xff", "\xff"},
		{"0xfe", "\xfe"},
	}
	for _, s := range utf8Surfaces() {
		for _, b := range bad {
			src := s.wrap(b.bytes)
			if utf8.ValidString(src) {
				t.Fatalf("%s/%s: test input is unexpectedly valid UTF-8", s.name, b.name)
			}
			wantInvalidEncoding(t, s.name+"/"+b.name, s.read(src))
		}
	}
}

// Fixed path: the bad byte at offset zero, on line 2, and after many lines all
// report 1:1, and only one diagnostic comes back however many sequences are bad.
func TestD14PathIsAlwaysOneOne(t *testing.T) {
	for _, s := range utf8Surfaces() {
		for _, src := range []string{
			"\x80" + s.wrap(""),
			"\n\n\n" + s.wrap("\x80"),
			s.wrap("\x80\xc0\xaf\xe2\x82\xff"),
			strings.Repeat(s.wrap(""), 50) + "\xff",
		} {
			wantInvalidEncoding(t, s.name, s.read(src))
		}
	}
}

// The read order is D-14, then D-15, then D-21, then the grammar or codec.
func TestD14RunsBeforeTheBOMRules(t *testing.T) {
	const bomBytes = "\xef\xbb\xbf"
	for _, s := range utf8Surfaces() {
		body := s.wrap("")
		cases := []struct{ name, in string }{
			{"truncated BOM (ef bb)", "\xef\xbb" + body},
			{"truncated BOM (ef)", "\xef" + body},
			{"BOM then a continuation byte", bomBytes + "\x80" + body},
			{"doubled BOM then bad bytes", bomBytes + bomBytes + "\x80"},
			{"doubled BOM then bad bytes inside content", bomBytes + bomBytes + s.wrap("\xc0\xaf")},
			{"BOM then bad bytes inside content", bomBytes + s.wrap("\xe2\x82")},
		}
		for _, tc := range cases {
			wantInvalidEncoding(t, s.name+"/"+tc.name, s.read(tc.in))
		}
	}
}

// A doubled BOM with otherwise valid bytes is still D-21's failure, not D-14's:
// the two rules are told apart by whether the bytes decode.
func TestValidDoubledBOMIsStillD21NotD14(t *testing.T) {
	const bomBytes = "\xef\xbb\xbf"
	for _, s := range utf8Surfaces() {
		err := s.read(bomBytes + bomBytes + s.wrap(""))
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code == omnist.CodeParseInvalidEncoding || pe.Path != "1:1" {
			t.Errorf("%s: got %#v, want a D-21 rejection at 1:1, not parse.invalid-encoding", s.name, err)
		}
	}
}

// Controls: valid multi-byte content is accepted. A reader that satisfied D-14
// by refusing everything above ASCII would pass every test above.
func TestD14AcceptsValidMultiByteUTF8(t *testing.T) {
	for _, s := range utf8Surfaces() {
		for _, ok := range []string{"\xc3\xa9", "\xe2\x82\xac", "\xf0\x9f\x98\x80", "\xc3\xa9\xe2\x82\xac\xf0\x9f\x98\x80"} {
			if err := s.read(s.wrap(ok)); err != nil {
				t.Errorf("%s: valid UTF-8 %q refused: %v", s.name, ok, err)
			}
		}
		// Boundary code points: U+D7FF and U+E000 (either side of the surrogate block),
		// U+10FFFF, U+00A0, U+07FF, U+0800 (U+0080 is left out: YAML refuses C1 controls).
		for _, ok := range []string{"\xed\x9f\xbf", "\xee\x80\x80", "\xf4\x8f\xbf\xbf", "\xc2\xa0", "\xdf\xbf", "\xe0\xa0\x80"} {
			if err := s.read(s.wrap(ok)); err != nil {
				t.Errorf("%s: valid boundary UTF-8 %q refused: %v", s.name, ok, err)
			}
		}
	}
}

// XML has a second read entry point.
func TestD14XMLReadWithSchema(t *testing.T) {
	sch, err := osd.Read("record R {\n    \"a\": string,\n}\nroot R\n")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = xml.ReadWithSchema("<R><a>\xe2\x82</a></R>", &sch, omnist.DefaultLimits())
	wantInvalidEncoding(t, "xml.ReadWithSchema", err)
}

func TestPrepareInput(t *testing.T) {
	got, err := omnist.PrepareInput("\xef\xbb\xbfhello", omnist.CodeParseCodecSyntax)
	if err != nil || got != "hello" {
		t.Errorf("valid input with one BOM: got %q, %v", got, err)
	}
	got, err = omnist.PrepareInput("plain", omnist.CodeParseCodecSyntax)
	if err != nil || got != "plain" {
		t.Errorf("plain: got %q, %v", got, err)
	}
	if got, err = omnist.PrepareInput("\xef\xbb", omnist.CodeParseCodecSyntax); err == nil || got != "" || err.Code != omnist.CodeParseInvalidEncoding {
		t.Errorf("truncated BOM: got %q, %v", got, err)
	}
	if _, err = omnist.PrepareInput("\xef\xbb\xbf\xef\xbb\xbfx", omnist.CodeParseUnexpectedToken); err == nil || err.Code != omnist.CodeParseUnexpectedToken {
		t.Errorf("doubled BOM: got %v", err)
	}
}
