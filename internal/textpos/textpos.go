// Package textpos converts the byte offsets and byte columns that Go's codec
// libraries report into the text positions spec E-28 and E-31 require: lines
// split on LF only (E-29), columns counted in Unicode code points, both
// 1-based and always inside the input.
package textpos

import (
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
