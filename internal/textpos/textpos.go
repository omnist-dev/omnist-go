// Package textpos converts the byte offsets and byte columns that Go's codec
// libraries report into the text positions spec E-28 and E-31 require: lines
// split on LF only (E-29), columns counted in Unicode code points, both
// 1-based and always inside the input.
package textpos

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// FromOffset converts a 0-based byte offset into text to a 1-based line and a
// 1-based code-point column. An offset outside [0, len(text)] is clamped, and
// one that falls inside a multi-byte character is moved back to that
// character's first byte.
func FromOffset(text string, offset int) (line, col int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	for offset > 0 && offset < len(text) && !utf8.RuneStart(text[offset]) {
		offset--
	}
	prefix := text[:offset]
	line = strings.Count(prefix, "\n") + 1
	lineStart := strings.LastIndexByte(prefix, '\n') + 1
	return line, utf8.RuneCountInString(prefix[lineStart:]) + 1
}

// FromLineByteCol converts a library's 1-based line and 1-based BYTE column
// into a 1-based line and a 1-based code-point column. A line past the last is
// clamped to the last line, and a column past the end of its line to the
// position just after the line's last character.
func FromLineByteCol(text string, line, byteCol int) (int, int) {
	if line < 1 {
		line = 1
	}
	if byteCol < 1 {
		byteCol = 1
	}
	lineStart := 0
	for n := 1; n < line; n++ {
		i := strings.IndexByte(text[lineStart:], '\n')
		if i < 0 {
			break
		}
		lineStart += i + 1
	}
	lineEnd := len(text)
	if i := strings.IndexByte(text[lineStart:], '\n'); i >= 0 {
		lineEnd = lineStart + i
	}
	offset := lineStart + byteCol - 1
	if offset > lineEnd {
		offset = lineEnd
	}
	return FromOffset(text, offset)
}

// Index answers FromLineByteCol queries for one text in amortised constant
// time per query, where the package-level function rescans the text from its
// start each time. A reader that positions every key or element it sees (the
// TOML and XML readers do, because a node's Path is its line:col) would
// otherwise be quadratic in the input size.
//
// Line starts are found once, in a single pass, when the Index is built. The
// code-point column within a line is counted from the previous query's offset
// when the new query is on the same line and not before it, so queries that
// advance through the text (the readers' access pattern) cost the distance
// moved, not the length of the line; out-of-order queries stay correct and
// merely recount from the start of the line. Every result is identical to
// FromLineByteCol's.
//
// An Index is not safe for concurrent use.
type Index struct {
	text       string
	lineStarts []int // byte offset of each line's first byte; lineStarts[0] == 0

	// Cache of the last column count: runes in text[lineStarts[cLine]:cOff].
	cLine, cOff, cRunes int
}

// NewIndex builds the line index for text.
func NewIndex(text string) *Index {
	starts := []int{0}
	for i := 0; i < len(text); {
		j := strings.IndexByte(text[i:], '\n')
		if j < 0 {
			break
		}
		i += j + 1
		starts = append(starts, i)
	}
	return &Index{text: text, lineStarts: starts}
}

// FromLineByteCol has FromLineByteCol's contract and results.
func (x *Index) FromLineByteCol(line, byteCol int) (int, int) {
	if line < 1 {
		line = 1
	}
	if byteCol < 1 {
		byteCol = 1
	}
	if line > len(x.lineStarts) {
		line = len(x.lineStarts)
	}
	lineStart := x.lineStarts[line-1]
	lineEnd := len(x.text)
	if line < len(x.lineStarts) {
		lineEnd = x.lineStarts[line] - 1
	}
	offset := lineStart + byteCol - 1
	if offset > lineEnd {
		offset = lineEnd
	}
	// Move an offset inside a multi-byte character back to its first byte.
	// lineStart follows a newline (or is 0), so it is always a rune start and
	// the loop cannot cross it.
	for offset > lineStart && offset < len(x.text) && !utf8.RuneStart(x.text[offset]) {
		offset--
	}
	from, runes := lineStart, 0
	if x.cLine == line && x.cOff >= lineStart && x.cOff <= offset {
		from, runes = x.cOff, x.cRunes
	}
	runes += utf8.RuneCountInString(x.text[from:offset])
	x.cLine, x.cOff, x.cRunes = line, offset, runes
	return line, runes + 1
}

// FromOffset has FromOffset's contract and results, using the index: the line
// is found by binary search and the column by FromLineByteCol.
func (x *Index) FromOffset(offset int) (line, col int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(x.text) {
		offset = len(x.text)
	}
	// The line is the last one whose start is at or before offset.
	line = sort.SearchInts(x.lineStarts, offset+1)
	return x.FromLineByteCol(line, offset-x.lineStarts[line-1]+1)
}
