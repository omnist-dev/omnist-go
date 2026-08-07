package omnist

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
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
// Document leaf directly, plus position info for ParseError.
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
	dateVal     DateValue
	timeVal     TimeValue
	dateTimeVal DateTimeValue
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

	limits *LimitChecker
}

func newLexer(text string, limits *LimitChecker) *lexer {
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
			// CRLF or lone CR both count as a newline separator.
			if !sawSep {
				sawSep, sepLine, sepCol = true, l.line, l.col
			}
			l.advance()
			if l.peekRune() == '\n' {
				l.advance()
			}
		case '\n':
			if !sawSep {
				sawSep, sepLine, sepCol = true, l.line, l.col
			}
			l.advance()
		case ';':
			if !sawSep {
				sawSep, sepLine, sepCol = true, l.line, l.col
			}
			l.advance()
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
	reDateTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?([+-]\d{2}:\d{2})?`)
	reDate     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
	reTime     = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?([+-]\d{2}:\d{2})?`)
	reNumber   = regexp.MustCompile(`^-?\d+(\.\d+([eE][+-]?\d+)?|[eE][+-]?\d+)`)
	reInteger  = regexp.MustCompile(`^-?\d+`)
	reIdent    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*`)
)

// remainingString returns the source from the current position onward, as
// a string, for regexp matching. Cheap enough: called once per token.
func (l *lexer) remainingString() string {
	return string(l.src[l.pos:])
}

// next returns the next token, or a *ParseError. It is the single entry
// point implementing §4.2's priority order.
func (l *lexer) next() (token, *ParseError) {
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
	if m := reDateTime.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokDateTime, text: m, dateTimeVal: parseDateTimeValue(m), line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 4: DATE (not followed by a valid DATETIME, already excluded above).
	if m := reDate.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokDate, text: m, dateVal: parseDateValue(m), line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 5: TIME.
	if m := reTime.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokTime, text: m, timeVal: parseTimeValue(m), line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
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
		if diag := l.limits.CheckIntDigits(fmt.Sprintf("%d:%d", startLine, startCol), digits); diag != nil {
			return token{}, &ParseError{Line: startLine, Col: startCol, Path: fmt.Sprintf("%d:%d", startLine, startCol), Code: diag.Code, Message: diag.Message}
		}
		bi, _ := new(big.Int).SetString(m, 10)
		return token{kind: tokInteger, text: m, intVal: bi, intDigits: digits, line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	// Rule 9: IDENT.
	if m := reIdent.FindString(rest); m != "" {
		l.consumeRunes(len([]rune(m)))
		return token{kind: tokIdent, text: m, line: startLine, col: startCol, sepBefore: sawSep, sepLine: sepLine, sepCol: sepCol}, nil
	}

	return token{}, &ParseError{
		Line: startLine, Col: startCol,
		Path:    fmt.Sprintf("%d:%d", startLine, startCol),
		Code:    CodeParseUnexpectedToken,
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

// --- temporal value construction ---
//
// parseDateValue/parseTimeValue/parseDateTimeValue are only ever called on
// text already matched by reDate/reTime/reDateTime, which fully pin the
// digit layout these functions assume (down to field widths). Given that
// precondition the Sscanf/index calls below cannot fail, so — unlike the
// lexer's other decoders — these deliberately have no error return: an
// error-returning version would carry a permanently-dead "malformed"
// branch that no input could ever reach (regex guarantees the format),
// which is worse for coverage and for readability than asserting the
// precondition in this comment once, here.

func parseDateValue(s string) DateValue {
	var y, mo, d int
	_, _ = fmt.Sscanf(s, "%4d-%2d-%2d", &y, &mo, &d)
	return DateValue{Year: y, Month: mo, Day: d}
}

func parseTimeValue(s string) TimeValue {
	rest := s
	var hh, mm int
	_, _ = fmt.Sscanf(rest, "%2d:%2d", &hh, &mm)
	rest = rest[5:]
	tv := TimeValue{Hour: hh, Minute: mm}
	if strings.HasPrefix(rest, ":") {
		var ss int
		_, _ = fmt.Sscanf(rest[1:], "%2d", &ss)
		tv.Second = ss
		rest = rest[3:]
		if strings.HasPrefix(rest, ".") {
			end := 1
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			tv.Nanosecond = fracToNanos(rest[1:end])
			rest = rest[end:]
		}
	}
	if len(rest) == 6 && (rest[0] == '+' || rest[0] == '-') {
		var oh, om int
		_, _ = fmt.Sscanf(rest[1:], "%2d:%2d", &oh, &om)
		sign := 1
		if rest[0] == '-' {
			sign = -1
		}
		tv.HasOffset = true
		tv.OffsetSeconds = sign * (oh*3600 + om*60)
	}
	return tv
}

func parseDateTimeValue(s string) DateTimeValue {
	idx := strings.IndexByte(s, 'T')
	return DateTimeValue{Date: parseDateValue(s[:idx]), Time: parseTimeValue(s[idx+1:])}
}

// fracToNanos converts a fractional-second digit string (1-6 digits, as
// matched by the grammar) to nanoseconds, right-padding to 9 digits.
func fracToNanos(digits string) int {
	padded := (digits + "000000000")[:9]
	n, _ := strconv.Atoi(padded)
	return n
}

// --- string scanning ---

func (l *lexer) scanString() (token, *ParseError) {
	if l.peekRune() == '\'' {
		return l.scanRawString()
	}
	// '"' next: distinguish multiline ("""...""") from single dquote.
	if l.peekRune() == '"' && l.peekRuneAt(1) == '"' && l.peekRuneAt(2) == '"' {
		return l.scanMultilineString()
	}
	return l.scanDQuoteString()
}

func (l *lexer) errAt(line, col int, code Code, msg string) *ParseError {
	return &ParseError{Line: line, Col: col, Path: fmt.Sprintf("%d:%d", line, col), Code: code, Message: msg}
}

func (l *lexer) scanRawString() (token, *ParseError) {
	startLine, startCol := l.line, l.col
	l.advance() // opening '
	var b strings.Builder
	for {
		if l.atEOF() {
			return token{}, l.errAt(startLine, startCol, CodeParseUnterminatedString, "unterminated raw string")
		}
		r := l.advance()
		if r == '\'' {
			return token{kind: tokString, strVal: b.String()}, nil
		}
		b.WriteRune(r)
	}
}

func (l *lexer) scanDQuoteString() (token, *ParseError) {
	startLine, startCol := l.line, l.col
	l.advance() // opening "
	var b strings.Builder
	for {
		if l.atEOF() {
			return token{}, l.errAt(startLine, startCol, CodeParseUnterminatedString, "unterminated string")
		}
		cLine, cCol := l.line, l.col
		r := l.advance()
		switch {
		case r == '"':
			return token{kind: tokString, strVal: b.String()}, nil
		case r == '\\':
			if l.atEOF() {
				return token{}, l.errAt(startLine, startCol, CodeParseUnterminatedString, "unterminated string")
			}
			if err := l.scanEscape(&b, cLine, cCol); err != nil {
				return token{}, err
			}
		case r < 0x20:
			return token{}, l.errAt(cLine, cCol, CodeParseControlCharacter, "control character in string")
		default:
			b.WriteRune(r)
		}
	}
}

func (l *lexer) scanEscape(b *strings.Builder, escLine, escCol int) *ParseError {
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
				return l.errAt(escLine, escCol, CodeParseUnpairedSurrogate, "unpaired surrogate escape")
			}
			l.advance() // backslash
			l.advance() // u
			low, err := l.scanHex4(escLine, escCol)
			if err != nil {
				return err
			}
			if low < 0xDC00 || low > 0xDFFF {
				return l.errAt(escLine, escCol, CodeParseUnpairedSurrogate, "unpaired surrogate escape")
			}
			combined := 0x10000 + (cp-0xD800)*0x400 + (low - 0xDC00)
			b.WriteRune(rune(combined))
		case cp >= 0xDC00 && cp <= 0xDFFF:
			// Lone low surrogate.
			return l.errAt(escLine, escCol, CodeParseUnpairedSurrogate, "unpaired surrogate escape")
		default:
			b.WriteRune(rune(cp))
		}
	default:
		return l.errAt(escLine, escCol, CodeParseInvalidEscape, fmt.Sprintf("invalid escape \\%c", e))
	}
	return nil
}

func (l *lexer) scanHex4(errLine, errCol int) (int, *ParseError) {
	if l.pos+4 > len(l.src) {
		return 0, l.errAt(errLine, errCol, CodeParseUnterminatedString, "unterminated \\u escape")
	}
	digits := string(l.src[l.pos : l.pos+4])
	v, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return 0, l.errAt(errLine, errCol, CodeParseInvalidEscape, "invalid \\u escape")
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
func (l *lexer) scanMultilineString() (token, *ParseError) {
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
			return token{}, l.errAt(startLine, startCol, CodeParseUnterminatedString, "unterminated multiline string")
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
		return token{}, l.errAt(cLine, cCol, CodeParseControlCharacter, "control character in multiline string")
	}
}
