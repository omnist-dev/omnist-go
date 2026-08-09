package omnist

import "testing"

// --- scalar_sub (§6.3) ---
//
// This is the one compatible_with_test.go case needing unexported access
// (scalarSub itself), so it stays in the internal "omnist" package; every
// other case moved to compatible_with_public_test.go (external
// "omnist_test" package) since it only needed mustParseOSD + exported API
// -- see referee_test.go's comment for why that split exists.

func TestScalarSub(t *testing.T) {
	cases := []struct {
		name string
		a, b Type
		want bool
	}{
		{"same kind true", ScalarType(KindString, false), ScalarType(KindString, false), true},
		{"integer sub number true", ScalarType(KindInteger, false), ScalarType(KindNumber, false), true},
		{"number not sub integer false", ScalarType(KindNumber, false), ScalarType(KindInteger, false), false},
		{"unrelated kinds false", ScalarType(KindString, false), ScalarType(KindInteger, false), false},
		{"nullable narrowing false", ScalarType(KindString, true), ScalarType(KindString, false), false},
		{"nullable widening true", ScalarType(KindString, false), ScalarType(KindString, true), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scalarSub(c.a, c.b)
			if got != c.want {
				t.Errorf("scalarSub(%+v, %+v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}
