package oml

import (
	"strings"
	"testing"
	"time"

	omnist "github.com/omnist-dev/omnist-go"
)

// A long run of double quotes used to be counted in full (and each count was
// itself quadratic) every time the multiline-string scanner met a run, so
// 100,000 quotes ran for minutes. The bound is generous: the linear scanner
// needs milliseconds.
func TestReadQuoteRunsScaleLinearly(t *testing.T) {
	const n = 100000
	for name, src := range map[string]string{
		"bare quotes":       strings.Repeat(`"`, n),
		"quote pairs":       strings.Repeat(`""`, n),
		"quotes after edge": "a: " + strings.Repeat(`"`, n),
		"unterminated":      `a: """` + strings.Repeat(`"`, n) + "x",
	} {
		start := time.Now()
		_, err := Read(src, omnist.DefaultLimits())
		if err == nil {
			t.Errorf("%s: read succeeded", name)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Errorf("%s: took %v, want under 3s (quadratic quote-run count)", name, d)
		}
	}
}
