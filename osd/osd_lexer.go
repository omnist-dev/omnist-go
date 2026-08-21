package osd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	omnist "github.com/omnist-dev/omnist-go"
)

// osdTokenKind identifies the lexical category of an OSD token, per spec
// §5.3's single ordered alternation.
type osdTokenKind int

const (
	osdTokEOF osdTokenKind = iota
	osdTokName
	osdTokString
	osdTokLBrace
	osdTokRBrace
	osdTokLBracket
	osdTokRBracket
	osdTokColon
	osdTokComma
	osdTokQuestion
	osdTokInt    // whole-number cardinality bound, optionally signed
	osdTokNumber // a bound containing '.', kept as its own kind so the
	// parser can report schema.non-integer-cardinality rather than a
	// generic syntax error (spec §5.5's worked example `[1.5]`).
)

// osdToken is one OSD lexical token.
type osdToken struct {
	kind osdTokenKind
	text string // raw source text
	line int    // 1-based line of the token's first character
	col  int    // 1-based column of the token's first character

	strVal string // decoded string value, meaningful only for osdTokString
	intVal int64  // decoded integer value, meaningful only for osdTokInt
}

// osdLexer turns OSD source text into a stream of tokens per spec §5.3: a
// single ordered alternation, with whitespace and `#`-to-end-of-line
// comments discarded before the parser ever sees a token.
type osdLexer struct {
	raw     string
	bytePos int
	line    int
	col     int
}

func newOSDLexer(text string) *osdLexer {
	return &osdLexer{raw: text, bytePos: 0, line: 1, col: 1}
}

func (l *osdLexer) atEOF() bool { return l.bytePos >= len(l.raw) }

// peekRune returns the rune at the current position.
func (l *osdLexer) peekRune() rune {
	if l.atEOF() {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.raw[l.bytePos:])
	return r
}

func (l *osdLexer) advance() rune {
	r, width := utf8.DecodeRuneInString(l.raw[l.bytePos:])
	l.bytePos += width
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

// skipTrivia consumes whitespace and `#` line comments, which are
// discarded before the parser sees anything (spec §5.3: "a comment may
// appear anywhere whitespace may").
func (l *osdLexer) skipTrivia() {
	for !l.atEOF() {
		r := l.peekRune()
		switch r {
		case ' ', '\t', '\r', '\n':
			l.advance()
		case '#':
			for !l.atEOF() && l.peekRune() != '\n' {
				l.advance()
			}
		default:
			return
		}
	}
}

var (
	// reOSDName matches an OSD `name` token: [A-Za-z_][A-Za-z0-9_]* — note
	// the deliberate absence of a hyphen, unlike OML's IDENT (spec §5.3).
	reOSDName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
	// reOSDBound matches a cardinality-bound-shaped token: an optional
	// leading '-' (spec §5.5's note that a conformant tokenizer's number
	// token MAY include one, with rejection deferred to field
	// construction) followed by digits, and optionally a '.' plus more
	// digits so a non-integer bound like "1.5" is recognized as one token
	// rather than three ("1", ".", "5").
	reOSDBound = regexp.MustCompile(`^-?\d+(\.\d+)?`)
)

func (l *osdLexer) remainingString() string {
	return l.raw[l.bytePos:]
}

func (l *osdLexer) errAt(line, col int, code omnist.Code, msg string) *omnist.ParseError {
	return &omnist.ParseError{Line: line, Col: col, Path: fmt.Sprintf("%d:%d", line, col), Code: code, Message: msg}
}

// next returns the next token, or a *omnist.ParseError.
func (l *osdLexer) next() (osdToken, *omnist.ParseError) {
	l.skipTrivia()
	startLine, startCol := l.line, l.col

	if l.atEOF() {
		return osdToken{kind: osdTokEOF, line: startLine, col: startCol}, nil
	}

	r := l.peekRune()

	if r == '"' {
		return l.scanString()
	}

	if k, ok := osdPunctKind(r); ok {
		l.advance()
		return osdToken{kind: k, text: string(r), line: startLine, col: startCol}, nil
	}

	rest := l.remainingString()

	if m := reOSDBound.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		if strings.Contains(m, ".") {
			return osdToken{kind: osdTokNumber, text: m, line: startLine, col: startCol}, nil
		}
		iv, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			// Out of int64 range: not a realistic cardinality bound either
			// way, so it is reported the same as any other invalid
			// cardinality at field-construction time. Clamp rather than
			// fail the whole tokenizer over a pathological bound.
			iv = 0
		}
		return osdToken{kind: osdTokInt, text: m, intVal: iv, line: startLine, col: startCol}, nil
	}

	if m := reOSDName.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return osdToken{kind: osdTokName, text: m, line: startLine, col: startCol}, nil
	}

	return osdToken{}, l.errAt(startLine, startCol, omnist.CodeParseUnexpectedToken, fmt.Sprintf("unexpected character %q", r))
}

func osdPunctKind(r rune) (osdTokenKind, bool) {
	switch r {
	case '{':
		return osdTokLBrace, true
	case '}':
		return osdTokRBrace, true
	case '[':
		return osdTokLBracket, true
	case ']':
		return osdTokRBracket, true
	case ':':
		return osdTokColon, true
	case ',':
		return osdTokComma, true
	case '?':
		return osdTokQuestion, true
	}
	return 0, false
}

func (l *osdLexer) consumeRunes(n int) {
	for i := 0; i < n; i++ {
		l.advance()
	}
}

// scanString scans a `"..."` OSD string and unescapes it per spec §5.3.1:
// strip the quotes, then replace every backslash pair \X with the single
// character X. There is no named-escape table — \n becomes the letter n,
// not a newline — and, since every \X is accepted by this rule (there is
// no invalid-escape case), the only failure mode is running off the end of
// input before the closing quote.
func (l *osdLexer) scanString() (osdToken, *omnist.ParseError) {
	startLine, startCol := l.line, l.col
	l.advance() // opening '"'
	var b strings.Builder
	for {
		if l.atEOF() {
			return osdToken{}, l.errAt(startLine, startCol, omnist.CodeParseUnterminatedString, "unterminated string")
		}
		cLine, cCol := l.line, l.col
		r := l.advance()
		switch {
		case r == '"':
			return osdToken{kind: osdTokString, strVal: b.String(), line: startLine, col: startCol}, nil
		case r == '\\':
			if l.atEOF() {
				return osdToken{}, l.errAt(startLine, startCol, omnist.CodeParseUnterminatedString, "unterminated string")
			}
			// Weak unescaping (§5.3.1): the character right after the
			// backslash is written verbatim, whatever it is, including a
			// literal newline — the ABNF's string production explicitly
			// allows "\" followed by any code point at all (%x00-10FFFF).
			b.WriteRune(l.advance())
		case r < 0x20:
			return osdToken{}, l.errAt(cLine, cCol, omnist.CodeParseControlCharacter, "control character in string")
		default:
			b.WriteRune(r)
		}
	}
}
