package xml

import (
	"io"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/internal/textpos"
)

// isXMLChar reports whether r is in XML 1.0's Char production (spec C-16):
// #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] | [#x10000-#x10FFFF].
func isXMLChar(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}

// scanXMLCharacters enforces C-16 ahead of the decoder: a numeric character
// reference must be well formed and name a Char, and a comment or processing
// instruction must hold only Chars. Go's decoder reads &#xD800; as U+FFFD and
// does not look inside comments or processing instructions. It returns the
// set of code points named by valid numeric references, so the entity
// sentinel can avoid them, or a *omnist.ParseError positioned at the fault.
//
// The scan is one left-to-right pass: a comment, CDATA section or processing
// instruction is skipped in one jump to its terminator (a reference inside is
// literal text), and an unterminated one ends the scan, since the decoder
// rejects the document anyway. Total work is linear in len(src).
func scanXMLCharacters(src string, pos *textpos.Index) (map[rune]bool, error) {
	named := map[rune]bool{}
	fail := func(offset int, msg string) error {
		line, col := pos.FromOffset(offset)
		return &omnist.ParseError{Line: line, Col: col, Path: strconv.Itoa(line) + ":" + strconv.Itoa(col), Code: omnist.CodeParseCodecSyntax, Message: msg}
	}
	checkChars := func(from, to int) error {
		for i, r := range src[from:to] {
			if !isXMLChar(r) {
				return fail(from+i, "XML: character is not an XML character")
			}
		}
		return nil
	}
	i := 0
	for {
		k := strings.IndexAny(src[i:], "<&")
		if k < 0 {
			return named, nil
		}
		i += k
		rest := src[i:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			end := strings.Index(rest[4:], "-->")
			if end < 0 {
				return named, nil
			}
			if err := checkChars(i, i+4+end+3); err != nil {
				return nil, err
			}
			i += 4 + end + 3
		case strings.HasPrefix(rest, "<![CDATA["):
			end := strings.Index(rest[9:], "]]>")
			if end < 0 {
				return named, nil
			}
			i += 9 + end + 3
		case strings.HasPrefix(rest, "<?"):
			end := strings.Index(rest[2:], "?>")
			if end < 0 {
				return named, nil
			}
			if err := checkChars(i, i+2+end+2); err != nil {
				return nil, err
			}
			i += 2 + end + 2
		case strings.HasPrefix(rest, "&#"):
			j := 2
			for j < len(rest) && rest[j] != ';' && rest[j] != '<' && rest[j] != '&' {
				j++
			}
			terminated := j < len(rest) && rest[j] == ';'
			v, ok := numericRefValue(rest[2:j], terminated)
			if !ok {
				return nil, fail(i, "XML: malformed or non-Char numeric character reference")
			}
			named[v] = true
			i += j
			if terminated {
				i++
			}
		default:
			i++
		}
	}
}

// numericRefValue parses the body of &#...; (body excludes "&#" and ";"). It
// reports false for a missing semicolon, a body that is not decimal digits or
// x and hex digits, or a value outside XML 1.0 Char.
func numericRefValue(body string, terminated bool) (rune, bool) {
	if !terminated || body == "" {
		return 0, false
	}
	base := 10
	if body[0] == 'x' {
		base, body = 16, body[1:]
	}
	body = strings.TrimLeft(body, "0")
	if len(body) > 8 {
		return 0, false
	}
	var v uint64
	for i := 0; i < len(body); i++ {
		c := body[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return 0, false
		}
		if d >= base {
			return 0, false
		}
		v = v*uint64(base) + uint64(d)
	}
	if !isXMLChar(rune(v)) {
		return 0, false
	}
	return rune(v), true
}

// anyCharset is Decoder.CharsetReader: an encoding declaration is ignored
// (spec 7.1, D-14 decides the input encoding), so the bytes are read as they
// are, already UTF-8, whatever encoding= says.
func anyCharset(_ string, in io.Reader) (io.Reader, error) {
	return in, nil
}
