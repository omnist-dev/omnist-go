package oml

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// tokenKind identifies the lexical category of a token, per spec §4.2's
// fixed tokenizer priority order.
type tokenKind int

const (
	tokEOF tokenKind = iota
	tokString
	tokLBrace
	tokRBrace
	tokLBracket
	tokRBracket
	tokColon
	tokComma
	tokDateTime
	tokDate
	tokTime
	tokNumber
	tokInteger
	tokIdent
)

// token is one lexical token, carrying enough decoded value to build a
// omnist.Document leaf directly, plus position info for omnist.ParseError.
type token struct {
	kind tokenKind
	text string // raw source text (identifier/label text, or number's raw digits)
	line int    // 1-based line of the token's first character
	col  int    // 1-based column of the token's first character

	// sepBefore reports whether a separator (a run of skipped trivia
	// containing at least one newline or ';', per spec §4.2.1) preceded
	// this token. hspace/comment-only runs do not set this.
	sepBefore bool
	// sepLine/sepCol locate the first newline or ';' of that separator,
	// for error reporting when a separator appears somewhere it must not
	// (inside an array, per §4.3.1).
	sepLine, sepCol int

	// Decoded values, meaningful only for the corresponding kind.
	strVal      string
	intVal      *big.Int
	intDigits   int
	numVal      float64
	dateVal     omnist.DateValue
	timeVal     omnist.TimeValue
	dateTimeVal omnist.DateTimeValue
}

// lexer turns OML source text into a stream of tokens per spec §4.2's
// maximal-munch-under-fixed-priority-order algorithm: at each position the
// first matching rule (in the fixed order) wins outright, consuming only
// that rule's own match — there is no cross-rule "longest match wins" and
// no backtracking. This means, for example, that reserved-float matches
// ("nan", "inf", "-inf") win over IDENT even when a longer IDENT would
// otherwise be extractable from the same position (e.g. "nano" lexes as
// NUMBER("nan") followed by IDENT("o")); this is the literal, intended
// consequence of the algorithm as specified in §4.2, not a bug worked
// around here.
type lexer struct {
	src  []rune
	pos  int
	line int
	col  int

	limits *omnist.LimitChecker

	// valuePath is the omnist.Document path (spec §8.4) of the edge value the
	// parser is about to tokenize, e.g. "$.n" — set by the parser right
	// before it calls advance() to read a value token, and used only to
	// path a document.limit.int-digits diagnostic correctly (that code is
	// document.*, so per §8.4 it MUST carry an omnist.Document path, never the
	// text-position path every other diagnostic in this file uses; a
	// parse.* diagnostic never consults this field). Left empty when no
	// value is pending (e.g. while reading a label or punctuation), in
	// which case CheckIntDigits falls back to a text-position path — this
	// only matters for a lone top-level integer with no enclosing label,
	// which document.limit.int-digits cannot presently pin an example of
	// an omnist.Document path for since there's no label to name.
	valuePath string
}

func newLexer(text string, limits *omnist.LimitChecker) *lexer {
	return &lexer{src: []rune(text), pos: 0, line: 1, col: 1, limits: limits}
}

func (l *lexer) atEOF() bool { return l.pos >= len(l.src) }

func (l *lexer) peekRune() rune {
	if l.atEOF() {
		return 0
	}
	return l.src[l.pos]
}

func (l *lexer) peekRuneAt(offset int) rune {
	if l.pos+offset >= len(l.src) {
		return 0
	}
	return l.src[l.pos+offset]
}

// advance consumes one rune, updating line/col.
func (l *lexer) advance() rune {
	r := l.src[l.pos]
	l.pos++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

// skipTrivia consumes horizontal space and comments (which emit no
// token), and tracks whether a newline or ';' was seen among them (which
// makes the following token separator-preceded, per §4.2.1).
func (l *lexer) skipTrivia() (sawSep bool, sepLine, sepCol int) {
	for !l.atEOF() {
		r := l.peekRune()
		switch r {
		case ' ', '\t':
			l.advance()
		case '\r':
			// CRLF or lone CR both count as a newline separator. The
			// reported position is where valid content would have had to
			// start instead of the separator — i.e. just past it, not the
			// separator character's own position — per the conformance
			// vector oml-grammar/arrays/newline-inside-array-is-an-error.
			l.advance()
			if l.peekRune() == '\n' {
				l.advance()
			}
			if !sawSep {
				sawSep, sepLine, sepCol = true, l.line, l.col
			}
		case '\n':
			l.advance()
			if !sawSep {
				sawSep, sepLine, sepCol = true, l.line, l.col
			}
		case ';':
			l.advance()
			if !sawSep {
				sawSep, sepLine, sepCol = true, l.line, l.col
			}
		case '#':
			for !l.atEOF() && l.peekRune() != '\n' {
				l.advance()
			}
		default:
			return sawSep, sepLine, sepCol
		}
	}
	return sawSep, sepLine, sepCol
}

var (
	reNumber  = regexp.MustCompile(`^-?\d+(\.\d+([eE][+-]?\d+)?|[eE][+-]?\d+)`)
	reInteger = regexp.MustCompile(`^-?\d+`)
	reIdent   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*`)
)

// remainingString returns the source from the current position onward, as
// a string, for regexp matching. Cheap enough: called once per token.
func (l *lexer) remainingString() string {
	return string(l.src[l.pos:])
}

// next returns the next token, or a *omnist.ParseError. It is the single entry
// point implementing §4.2's priority order.
func (l *lexer) next() (token, *omnist.ParseError) {
	sawSep, sepLine, sepCol := l.skipTrivia()
	startLine, startCol := l.line, l.col

	if l.atEOF() {
		return token{kind: tokEOF, line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	r := l.peekRune()

	// Rule 1: STRING family.
	if r == '"' || r == '\'' {
		tok, err := l.scanString()
		if err != nil {
			return token{}, err
		}
		tok.line, tok.col = startLine, startCol
		tok.sepBefore, tok.sepLine, tok.sepCol = sawSep, sepLine, sepCol
		return tok, nil
	}

	// Rule 2: punctuation.
	if k, ok := punctKind(r); ok {
		l.advance()
		return token{kind: k, text: string(r), line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	rest := l.remainingString()

	// Rule 3: DATETIME.
	if m := omnist.ISODateTimeRegexp.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokDateTime, text: m, dateTimeVal: omnist.ParseISODateTime(m), line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 4: DATE (not followed by a valid DATETIME, already excluded above).
	if m := omnist.ISODateRegexp.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokDate, text: m, dateVal: omnist.ParseISODate(m), line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 5: TIME.
	if m := omnist.ISOTimeRegexp.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokTime, text: m, timeVal: omnist.ParseISOTime(m), line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 6: NUMBER (decimal or exponent form).
	if m := reNumber.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		f, _ := strconv.ParseFloat(m, 64)
		return token{kind: tokNumber, text: m, numVal: f, line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 7: reserved float spellings, emitted as NUMBER.
	if m, val, ok := matchReservedFloat(rest); ok {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokNumber, text: m, numVal: val, line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 8: INTEGER.
	if m := reInteger.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		digits := len(m)
		if strings.HasPrefix(m, "-") {
			digits--
		}
		digitPath := fmt.Sprintf("%d:%d", startLine, startCol)
		if l.valuePath != "" {
			digitPath = l.valuePath
		}
		if diag := l.limits.CheckIntDigits(digitPath, digits); diag != nil {
			return token{}, &omnist.ParseError{Line: startLine, Col: startCol, Path: digitPath, Code: diag.Code, Message: diag.Message}
		}
		bi, _ := new(big.Int).SetString(m, 10)
		return token{kind: tokInteger, text: m, intVal: bi, intDigits: digits, line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 9: IDENT.
	if m := reIdent.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokIdent, text: m, line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	return token{}, &omnist.ParseError{
		Line: startLine, Col: startCol,
		Path:    fmt.Sprintf("%d:%d", startLine, startCol),
		Code:    omnist.CodeParseUnexpectedToken,
		Message: fmt.Sprintf("unexpected character %q", r),
	}
}

func punctKind(r rune) (tokenKind, bool) {
	switch r {
	case '{':
		return tokLBrace, true
	case '}':
		return tokRBrace, true
	case '[':
		return tokLBracket, true
	case ']':
		return tokRBracket, true
	case ':':
		return tokColon, true
	case ',':
		return tokComma, true
	}
	return 0, false
}

// consumeRunes advances n runes, tracking line/col (none of the matched
// texts here (dates/times/numbers/idents) can themselves contain a
// newline, so this does not need the newline-handling in advance, but
// reuses it for a single code path).
func (l *lexer) consumeRunes(n int) {
	for i := 0; i < n; i++ {
		l.advance()
	}
}

// matchReservedFloat matches the literal reserved-float spellings "nan",
// "inf", "-inf" per §4.2 rule 7 and the reserved-number ABNF production.
// Per the priority-order semantics documented on lexer, this is a fixed
// literal match, not extended to consume further ident-continuation
// characters that might follow.
func matchReservedFloat(rest string) (matched string, val float64, ok bool) {
	switch {
	case strings.HasPrefix(rest, "nan"):
		return "nan", math.NaN(), true
	case strings.HasPrefix(rest, "-inf"):
		return "-inf", math.Inf(-1), true
	case strings.HasPrefix(rest, "inf"):
		return "inf", math.Inf(1), true
	}
	return "", 0, false
}

// --- string scanning ---

func (l *lexer) scanString() (token, *omnist.ParseError) {
	if l.peekRune() == '\'' {
		return l.scanRawString()
	}
	// '"' next: distinguish multiline ("""...""") from single dquote.
	if l.peekRune() == '"' && l.peekRuneAt(1) == '"' && l.peekRuneAt(2) == '"' {
		return l.scanMultilineString()
	}
	return l.scanDQuoteString()
}

func (l *lexer) errAt(line, col int, code omnist.Code, msg string) *omnist.ParseError {
	return &omnist.ParseError{Line: line, Col: col, Path: fmt.Sprintf("%d:%d", line, col), Code: code, Message: msg}
}

func (l *lexer) scanRawString() (token, *omnist.ParseError) {
	startLine, startCol := l.line, l.col
	l.advance() // opening '
	var b strings.Builder
	for {
		if l.atEOF() {
			return token{}, l.errAt(startLine, startCol, omnist.CodeParseUnterminatedString, "unterminated raw string")
		}
		r := l.advance()
		if r == '\'' {
			return token{kind: tokString, strVal: b.String()}, nil
		}
		b.WriteRune(r)
	}
}

func (l *lexer) scanDQuoteString() (token, *omnist.ParseError) {
	startLine, startCol := l.line, l.col
	l.advance() // opening "
	var b strings.Builder
	for {
		if l.atEOF() {
			return token{}, l.errAt(startLine, startCol, omnist.CodeParseUnterminatedString, "unterminated string")
		}
		r := l.advance()
		switch {
		case r == '"':
			return token{kind: tokString, strVal: b.String()}, nil
		case r == '\\':
			if l.atEOF() {
				return token{}, l.errAt(startLine, startCol, omnist.CodeParseUnterminatedString, "unterminated string")
			}
			if err := l.scanEscape(&b, startLine, startCol); err != nil {
				return token{}, err
			}
		case r < 0x20:
			return token{}, l.errAt(startLine, startCol, omnist.CodeParseControlCharacter, "control character in string")
		default:
			b.WriteRune(r)
		}
	}
}

func (l *lexer) scanEscape(b *strings.Builder, escLine, escCol int) *omnist.ParseError {
	e := l.advance()
	switch e {
	case '"':
		b.WriteRune('"')
	case '\\':
		b.WriteRune('\\')
	case '/':
		b.WriteRune('/')
	case 'b':
		b.WriteRune('\b')
	case 'f':
		b.WriteRune('\f')
	case 'n':
		b.WriteRune('\n')
	case 'r':
		b.WriteRune('\r')
	case 't':
		b.WriteRune('\t')
	case 'u':
		cp, err := l.scanHex4(escLine, escCol)
		if err != nil {
			return err
		}
		switch {
		case cp >= 0xD800 && cp <= 0xDBFF:
			// High surrogate: must be immediately followed by \uXXXX low surrogate.
			if l.peekRune() != '\\' || l.peekRuneAt(1) != 'u' {
				return l.errAt(escLine, escCol, omnist.CodeParseUnpairedSurrogate, "unpaired surrogate escape")
			}
			l.advance() // backslash
			l.advance() // u
			low, err := l.scanHex4(escLine, escCol)
			if err != nil {
				return err
			}
			if low < 0xDC00 || low > 0xDFFF {
				return l.errAt(escLine, escCol, omnist.CodeParseUnpairedSurrogate, "unpaired surrogate escape")
			}
			combined := 0x10000 + (cp-0xD800)*0x400 + (low - 0xDC00)
			b.WriteRune(rune(combined))
		case cp >= 0xDC00 && cp <= 0xDFFF:
			// Lone low surrogate.
			return l.errAt(escLine, escCol, omnist.CodeParseUnpairedSurrogate, "unpaired surrogate escape")
		default:
			b.WriteRune(rune(cp))
		}
	default:
		return l.errAt(escLine, escCol, omnist.CodeParseInvalidEscape, fmt.Sprintf("invalid escape \\%c", e))
	}
	return nil
}

func (l *lexer) scanHex4(errLine, errCol int) (int, *omnist.ParseError) {
	if l.pos+4 > len(l.src) {
		return 0, l.errAt(errLine, errCol, omnist.CodeParseUnterminatedString, "unterminated \\u escape")
	}
	digits := string(l.src[l.pos : l.pos+4])
	v, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return 0, l.errAt(errLine, errCol, omnist.CodeParseInvalidEscape, "invalid \\u escape")
	}
	l.consumeRunes(4)
	return int(v), nil
}

// scanMultilineString implements §4.5's multiline-string rule: closes at
// the first run of three or more '"'; the first three are consumed as the
// terminator and any further quotes in that run are left in the buffer
// for the scanner to re-tokenize (this is what makes the four-quote
// worked example in §4.8 produce an unterminated-string error rather than
// consuming a fourth literal quote character).
func (l *lexer) scanMultilineString() (token, *omnist.ParseError) {
	startLine, startCol := l.line, l.col
	l.advance()
	l.advance()
	l.advance() // opening """
	if l.peekRune() == '\r' {
		l.advance()
		if l.peekRune() == '\n' {
			l.advance()
		}
	} else if l.peekRune() == '\n' {
		l.advance()
	}
	var b strings.Builder
	for {
		if l.atEOF() {
			return token{}, l.errAt(startLine, startCol, omnist.CodeParseUnterminatedString, "unterminated multiline string")
		}
		cLine, cCol := l.line, l.col
		r := l.peekRune()
		if r == '"' {
			run := 0
			for l.peekRuneAt(run) == '"' {
				run++
			}
			if run >= 3 {
				l.consumeRunes(3)
				return token{kind: tokString, strVal: b.String()}, nil
			}
			// A run of one or two quotes is literal content.
			for i := 0; i < run; i++ {
				b.WriteRune(l.advance())
			}
			continue
		}
		if r == '\t' || r == '\n' || r >= 0x20 {
			b.WriteRune(l.advance())
			continue
		}
		return token{}, l.errAt(cLine, cCol, omnist.CodeParseControlCharacter, "control character in multiline string")
	}
}
