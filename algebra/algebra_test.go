package algebra

import "testing"

// --- le (§6.2) ---
//
// This is the one algebra_test.go case that needs unexported access (le
// itself), so it stays in the internal "omnist" package; every other case
// moved to algebra_public_test.go (external "omnist_test" package) since
// it only needed mustParseOSD + exported API -- see referee_test.go's
// comment for why that split exists (osd imports omnist, so an internal
// omnist test file cannot import osd without an import cycle).

func TestLe(t *testing.T) {
	cases := []struct {
		name       string
		x          uint64
		xUnbounded bool
		y          uint64
		yUnbounded bool
		want       bool
	}{
		{"y unbounded always true", 100, false, 0, true, true},
		{"both unbounded true", 5, true, 5, true, true},
		{"x unbounded y bounded false", 5, true, 100, false, false},
		{"both bounded x<=y", 3, false, 5, false, true},
		{"both bounded x==y", 5, false, 5, false, true},
		{"both bounded x>y", 6, false, 5, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := le(c.x, c.xUnbounded, c.y, c.yUnbounded)
			if got != c.want {
				t.Errorf("le(%d,%v,%d,%v) = %v, want %v", c.x, c.xUnbounded, c.y, c.yUnbounded, got, c.want)
			}
		})
	}
}
