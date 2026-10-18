package yaml

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// A timestamp fraction longer than six digits cannot be held exactly, so the
// reader refuses it with a coded error at the scalar instead of cutting it.
func TestReadTimestampFractionOverSixDigitsIsRefused(t *testing.T) {
	for _, digits := range []int{7, 9, 12} {
		_, err := Read("t: 2024-01-02T12:00:00."+strings.Repeat("1", digits)+"+01:00\n", omnist.DefaultLimits())
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeParseCodecSyntax || pe.Line != 1 || pe.Col != 4 {
			t.Errorf("%d digits: err = %+v", digits, err)
		}
	}
	for _, src := range []string{
		"t: 2024-01-02T12:00:00.123456+01:00\n",
		"t: \"2024-01-02T12:00:00.1234567+01:00\"\n",
		"t: '2024-01-02T12:00:00.1234567'\n",
	} {
		if _, err := Read(src, omnist.DefaultLimits()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
