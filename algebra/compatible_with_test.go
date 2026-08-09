package algebra

import (
	"testing"

	"github.com/omnist-dev/omnist-go"
)

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
		a, b omnist.Type
		want bool
	}{
		{"same kind true", omnist.ScalarType(omnist.KindString, false), omnist.ScalarType(omnist.KindString, false), true},
		{"integer sub number true", omnist.ScalarType(omnist.KindInteger, false), omnist.ScalarType(omnist.KindNumber, false), true},
		{"number not sub integer false", omnist.ScalarType(omnist.KindNumber, false), omnist.ScalarType(omnist.KindInteger, false), false},
		{"unrelated kinds false", omnist.ScalarType(omnist.KindString, false), omnist.ScalarType(omnist.KindInteger, false), false},
		{"nullable narrowing false", omnist.ScalarType(omnist.KindString, true), omnist.ScalarType(omnist.KindString, false), false},
		{"nullable widening true", omnist.ScalarType(omnist.KindString, false), omnist.ScalarType(omnist.KindString, true), true},
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
