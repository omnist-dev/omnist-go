package xml

import (
	"fmt"
	"strings"
	"testing"
	"time"

	omnist "github.com/omnist-dev/omnist-go"
)

// scaleBound is deliberately generous: the linear reader needs tens of
// milliseconds for these inputs, while the pre-fix quadratic reader (issue
// #132) needed many seconds, so the bound separates the two by two orders of
// magnitude and cannot flake on a slow CI machine.
const scaleBound = 3 * time.Second

func flatElements(n int, sep string) string {
	var b strings.Builder
	b.WriteString("<r>" + sep)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "<k%d>%d</k%d>%s", i, i, i, sep)
	}
	b.WriteString("</r>\n")
	return b.String()
}

func TestReadScalesLinearly(t *testing.T) {
	const n = 40000
	for name, src := range map[string]string{
		"one element per line": flatElements(n, "\n"),
		"single line":          flatElements(n, ""),
	} {
		start := time.Now()
		if _, _, err := Read(src, omnist.DefaultLimits()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d := time.Since(start); d > scaleBound {
			t.Errorf("%s: reading %d elements took %v, want under %v (quadratic position computation, issue #132)", name, n, d, scaleBound)
		}
	}
}

func BenchmarkReadFlatElements(b *testing.B) {
	src := flatElements(50000, "\n")
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Read(src, omnist.DefaultLimits()); err != nil {
			b.Fatal(err)
		}
	}
}

// The character pre-scan (C-16) is one linear pass even when openers are never
// closed: each of these used to rescan to the end of the input per match.
func TestCharacterScanScalesLinearly(t *testing.T) {
	const n = 40000
	for name, src := range map[string]string{
		"unterminated comment then PIs": "<r><!--" + strings.Repeat("<?&#65;", n) + "</r>",
		"unterminated openers repeated": strings.Repeat("<!--&#xD800;<?&#<![CDATA[", n),
		"unterminated CDATA then refs":  "<r><![CDATA[" + strings.Repeat("&#65;<?", n) + "</r>",
		"many valid references":         "<r>" + strings.Repeat("&#65;", n*4) + "</r>",
	} {
		start := time.Now()
		_, _, _ = Read(src, omnist.DefaultLimits())
		if d := time.Since(start); d > scaleBound {
			t.Errorf("%s: took %v, want under %v (quadratic pre-scan)", name, d, scaleBound)
		}
	}
}
