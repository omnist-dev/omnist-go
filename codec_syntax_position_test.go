package omnist_test

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
)

// E-31 (spec §8.4): a parse.codec-syntax path is a text position that lies
// inside the input: line in [1, LF+1], col in [1, code points on that line +1].
// A codec that reports no position is 1:1; never omitted, never 0:0.

var wellFormedPosition = regexp.MustCompile(`^[1-9][0-9]*:[1-9][0-9]*$`)

// positionInsideInput reports whether path is a well-formed text position
// that lies inside src per E-31 (lines split on LF only, E-29; columns in
// code points, E-28).
func positionInsideInput(src, path string) bool {
	if !wellFormedPosition.MatchString(path) {
		return false
	}
	lineStr, colStr, _ := strings.Cut(path, ":")
	line, _ := strconv.Atoi(lineStr)
	col, _ := strconv.Atoi(colStr)
	lines := strings.Split(src, "\n")
	if line > len(lines) {
		return false
	}
	return col <= utf8.RuneCountInString(lines[line-1])+1
}

type codecReader struct {
	name string
	read func(string) (omnist.Document, error)
}

func codecReaders() []codecReader {
	lim := omnist.DefaultLimits()
	return []codecReader{
		{"json", func(s string) (omnist.Document, error) { return json.Read(s, lim) }},
		{"yaml", func(s string) (omnist.Document, error) { return yaml.Read(s, lim) }},
		{"toml", func(s string) (omnist.Document, error) { return toml.Read(s, lim) }},
		{"xml", func(s string) (omnist.Document, error) { d, _, err := xml.Read(s, lim); return d, err }},
	}
}

func TestCodecSyntaxPositionsLieInsideInput(t *testing.T) {
	inputs := []string{
		"", " ", "\n", "\n\n", "<", "<a", "<a>", "<a>\n", "<a>\n\n", "<a><b></a>", "<a>é<b></a>",
		"\xef\xbb\xbf<a><b></a>",
		"<a>\r\n<b>\r\n</a>", "<a>\n  <b>é€😀</c>\n</a>", "<a/><b/>", "x<a/>", "<a>&bogus;</a>",
		"<a b=></a>", "<?xml version=\"1.0\"?>", "<a>\n<!--", "<a>\n<![CDATA[", "<a>\n\n\n",
		"{", "{\n", "{\"a\":", "{\"a\": }", "{\"é\": x}", "{\"a\": \"é€😀\", \"b\": x}", "{\"a\":1}\n}", "[1,", "tru",
		"{\"a\": 1,\n\"b\": \"é\"\n,}", "\"abc",
		"a: b: c", "a: [1, 2\n", "a: [\n", "a: \"é\n", "\"é€😀\": [", "a:\n  - b\n c: d\n", "a: *x\n", "{a: 1", "- a\nb: c\n",
		"a: 'é\n\n", "\t", "a: b\n\tc: d\n", "a: !!bogus\n", "a:\n\n\n  b: [\n",
		"a = ", "a = \n", "[a\nb = 1", "a = 1\na = 2\n", "é = [1,\n", "a = \"é€😀", "a = 1979-13-99", "[[a]]\n[a]\n", "a = {b = 1,\n}\n",
		"a = 'x\n", "a = 1 b = 2\n\n\n",
	}
	for _, rd := range codecReaders() {
		for _, in := range inputs {
			_, err := rd.read(in)
			if err == nil {
				continue
			}
			var pe *omnist.ParseError
			if !errors.As(err, &pe) || pe.Code != omnist.CodeParseCodecSyntax {
				continue
			}
			if !positionInsideInput(in, pe.Path) {
				t.Errorf("%s.Read(%q): parse.codec-syntax path %q is not a position inside the input (E-31)", rd.name, in, pe.Path)
			}
		}
	}
}

func TestPositionInsideInputHelper(t *testing.T) {
	cases := []struct {
		src, path string
		want      bool
	}{
		{"", "1:1", true},
		{"", "0:0", false},
		{"", "1:0", false},
		{"", "1:2", false},
		{"", "2:1", false},
		{"a\n", "2:1", true},
		{"a\n", "1:2", true},
		{"a\n", "1:3", false},
		{"é€😀", "1:4", true},
		{"é€😀", "1:5", false},
		{"a", "", false},
		{"a", "1:01", false},
		{"a", " 1:1", false},
	}
	for _, tc := range cases {
		if got := positionInsideInput(tc.src, tc.path); got != tc.want {
			t.Errorf("positionInsideInput(%q, %q) = %v, want %v", tc.src, tc.path, got, tc.want)
		}
	}
}

// The XML reader converts the decoder's byte column itself, so E-28 governs
// it: the column counts code points, not bytes.
func TestXMLSyntaxColumnCountsCodePoints(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"ASCII mismatched tag", "<a><b></a>", "1:11"},
		{"empty input", "", "1:1"},
		{"multibyte before the failure", "<a>é€😀<b></a>", "1:14"},
		{"failure on a later line", "<a>\n<é></a>", "2:8"},
	}
	for _, tc := range cases {
		_, _, err := xml.Read(tc.src, omnist.DefaultLimits())
		var pe *omnist.ParseError
		if !errors.As(err, &pe) || pe.Code != omnist.CodeParseCodecSyntax {
			t.Fatalf("%s: Read(%q) err = %v, want parse.codec-syntax", tc.name, tc.src, err)
		}
		if pe.Path != tc.want {
			t.Errorf("%s: Read(%q) path = %q, want %q", tc.name, tc.src, pe.Path, tc.want)
		}
	}
}

func TestJSONSyntaxColumnCountsCodePoints(t *testing.T) {
	_, err := json.Read("{\"é€😀\": x}", omnist.DefaultLimits())
	var pe *omnist.ParseError
	if !errors.As(err, &pe) || pe.Code != omnist.CodeParseCodecSyntax {
		t.Fatalf("err = %v, want parse.codec-syntax", err)
	}
	// The blamed character is the library's; the bound is what E-31 fixes.
	if !positionInsideInput("{\"é€😀\": x}", pe.Path) {
		t.Errorf("path %q is outside the input", pe.Path)
	}
}
