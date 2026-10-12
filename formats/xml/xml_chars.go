package xml

import (
	"io"
	"regexp"
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

// markupScan finds, left to right, the constructs whose characters Go's
// decoder does not check itself: comments and processing instructions (whose
// raw characters must be Chars) and numeric character references outside a
// comment, CDATA section or processing instruction (a reference there is
// literal text). A CDATA section is matched only so it is skipped.
var markupScan = regexp.MustCompile(`(?s)<!--.*?-->|<!\[CDATA\[.*?\]\]>|<\?.*?\?>|&#[^;<&]*;?`)

var numericRef = regexp.MustCompile(`^&#(?:x([0-9A-Fa-f]+)|([0-9]+));$`)

// scanXMLCharacters enforces C-16 ahead of the decoder: a numeric character
// reference must be well formed and name a Char, and a comment or processing
// instruction must hold only Chars. Go's decoder reads &#xD800; as U+FFFD and
// does not look inside comments or processing instructions. It returns the
// set of code points named by valid numeric references, so the entity
// sentinel can avoid them, or a *omnist.ParseError positioned at the fault.
func scanXMLCharacters(src string, pos *textpos.Index) (map[rune]bool, error) {
	named := map[rune]bool{}
	fail := func(offset int, msg string) error {
		line, col := pos.FromOffset(offset)
		return &omnist.ParseError{Line: line, Col: col, Path: strconv.Itoa(line) + ":" + strconv.Itoa(col), Code: omnist.CodeParseCodecSyntax, Message: msg}
	}
	for _, loc := range markupScan.FindAllStringIndex(src, -1) {
		m := src[loc[0]:loc[1]]
		switch {
		case strings.HasPrefix(m, "&#"):
			sub := numericRef.FindStringSubmatch(m)
			if sub == nil {
				return nil, fail(loc[0], "XML: malformed numeric character reference")
			}
			digits, base := sub[2], 10
			if sub[1] != "" {
				digits, base = sub[1], 16
			}
			digits = strings.TrimLeft(digits, "0")
			if len(digits) > 8 {
				return nil, fail(loc[0], "XML: numeric character reference is not an XML character")
			}
			var v uint64
			if digits != "" {
				v, _ = strconv.ParseUint(digits, base, 64) // at most 8 digits: cannot overflow
			}
			if !isXMLChar(rune(v)) {
				return nil, fail(loc[0], "XML: numeric character reference is not an XML character")
			}
			named[rune(v)] = true
		case strings.HasPrefix(m, "<!--") || strings.HasPrefix(m, "<?"):
			for i, r := range m {
				if !isXMLChar(r) {
					return nil, fail(loc[0]+i, "XML: character is not an XML character")
				}
			}
		}
	}
	return named, nil
}

// anyCharset is Decoder.CharsetReader: an encoding declaration is ignored
// (spec 7.1, D-14 decides the input encoding), so the bytes are read as they
// are, already UTF-8, whatever encoding= says.
func anyCharset(_ string, in io.Reader) (io.Reader, error) {
	return in, nil
}
