package textpos

import "testing"

func TestFromOffset(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		offset   int
		wantLine int
		wantCol  int
	}{
		{"start", "ab\ncd", 0, 1, 1},
		{"end of first line", "ab\ncd", 2, 1, 3},
		{"start of second line", "ab\ncd", 3, 2, 1},
		{"end of input", "ab\ncd", 5, 2, 3},
		{"negative clamps to start", "ab", -4, 1, 1},
		{"past the end clamps", "ab", 99, 1, 3},
		{"empty text", "", 0, 1, 1},
		{"code points not bytes", "\u00e9\u20ac\U0001F600x", 9, 1, 4},
		{"after a multibyte line", "\u00e9\n\u20ac", 3, 2, 1},
		{"inside two-byte character", "\u00e9x", 1, 1, 1},
		{"inside four-byte character", "a\U0001F600", 3, 1, 2},
		{"CR is an ordinary column", "a\r\nb", 2, 1, 3},
	}
	for _, tc := range cases {
		line, col := FromOffset(tc.text, tc.offset)
		if line != tc.wantLine || col != tc.wantCol {
			t.Errorf("%s: FromOffset(%q, %d) = %d:%d, want %d:%d", tc.name, tc.text, tc.offset, line, col, tc.wantLine, tc.wantCol)
		}
	}
}

func TestFromLineByteCol(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		line     int
		byteCol  int
		wantLine int
		wantCol  int
	}{
		{"first line ASCII", "abc", 1, 2, 1, 2},
		{"second line", "ab\ncd", 2, 2, 2, 2},
		{"byte column over multibyte", "\u00e9\u20ac\U0001F600x", 1, 10, 1, 4},
		{"multibyte on an earlier line", "\u00e9\n\u20acx", 2, 4, 2, 2},
		{"line below 1 clamps", "ab", 0, 1, 1, 1},
		{"column below 1 clamps", "ab", 1, 0, 1, 1},
		{"column past end of line clamps to just after it", "ab\ncd", 1, 99, 1, 3},
		{"line past the last clamps to the last", "ab", 5, 2, 1, 2},
		{"line past the last, final line empty", "ab\n", 9, 1, 2, 1},
		{"empty text", "", 1, 1, 1, 1},
	}
	for _, tc := range cases {
		line, col := FromLineByteCol(tc.text, tc.line, tc.byteCol)
		if line != tc.wantLine || col != tc.wantCol {
			t.Errorf("%s: FromLineByteCol(%q, %d, %d) = %d:%d, want %d:%d", tc.name, tc.text, tc.line, tc.byteCol, line, col, tc.wantLine, tc.wantCol)
		}
	}
}

// TestIndexMatchesFromLineByteCol checks the Index against the scanning
// function for every (line, column) of several texts, queried in ascending,
// descending and interleaved order, including positions past the end of a
// line and past the last line.
func TestIndexMatchesFromLineByteCol(t *testing.T) {
	texts := []string{
		"", "\n", "abc", "ab\ncd", "ab\n", "\n\n", "é€\U0001F600x", "é\n€x\n\U0001F600",
		"a\r\nb\r\n", "x\nyéz\n\n€",
	}
	for _, text := range texts {
		type q struct{ line, col int }
		var qs []q
		for line := -1; line <= 7; line++ {
			for col := -1; col <= len(text)+3; col++ {
				qs = append(qs, q{line, col})
			}
		}
		orders := map[string][]q{"ascending": qs}
		desc := make([]q, len(qs))
		mixed := make([]q, 0, len(qs))
		for i := range qs {
			desc[len(qs)-1-i] = qs[i]
			mixed = append(mixed, qs[(i*7)%len(qs)])
		}
		orders["descending"] = desc
		orders["interleaved"] = mixed
		for name, order := range orders {
			idx := NewIndex(text)
			for _, c := range order {
				wl, wc := FromLineByteCol(text, c.line, c.col)
				gl, gc := idx.FromLineByteCol(c.line, c.col)
				if gl != wl || gc != wc {
					t.Fatalf("%s %q (%d,%d): Index = %d:%d, FromLineByteCol = %d:%d", name, text, c.line, c.col, gl, gc, wl, wc)
				}
			}
		}
	}
}

// TestIndexFromOffsetMatchesFromOffset checks Index.FromOffset against the
// scanning FromOffset for every offset, including out-of-range ones and ones
// inside a multi-byte character.
func TestIndexFromOffsetMatchesFromOffset(t *testing.T) {
	texts := []string{
		"", "\n", "abc", "ab\ncd", "ab\n", "\n\n", "é€\U0001F600x", "é\n€x\n\U0001F600",
		"a\r\nb\r\n", "x\nyéz\n\n€",
	}
	for _, text := range texts {
		idx := NewIndex(text)
		for offset := -2; offset <= len(text)+3; offset++ {
			wl, wc := FromOffset(text, offset)
			gl, gc := idx.FromOffset(offset)
			if gl != wl || gc != wc {
				t.Fatalf("%q offset %d: Index = %d:%d, FromOffset = %d:%d", text, offset, gl, gc, wl, wc)
			}
		}
	}
}
