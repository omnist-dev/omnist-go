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
