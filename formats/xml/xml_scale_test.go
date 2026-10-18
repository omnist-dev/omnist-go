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

// Hostile inputs give a coded result in linear time, never a crash: an
// attribute value of 1 to 8 MB (read and reported as dropped, D-3) and a
// document nested 1,000 to 100,000 deep (document.limit.depth).
func TestHostileXMLScalesLinearly(t *testing.T) {
	for _, mb := range []int{1, 2, 4, 8} {
		src := `<a x="` + strings.Repeat("v", mb<<20) + `"><b>1</b></a>`
		start := time.Now()
		_, diags, err := Read(src, omnist.DefaultLimits())
		if err != nil || len(diags) != 1 || diags[0].Code != omnist.CodeFormatAttributeDropped {
			t.Errorf("%d MB attribute: err %v diags %+v", mb, err, diags)
		}
		if d := time.Since(start); d > scaleBound {
			t.Errorf("%d MB attribute took %v, want under %v", mb, d, scaleBound)
		}
	}
	for _, depth := range []int{1000, 10000, 100000} {
		src := strings.Repeat("<a>", depth) + "x" + strings.Repeat("</a>", depth)
		start := time.Now()
		_, _, err := Read(src, omnist.DefaultLimits())
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeDocumentLimitDepth {
			t.Errorf("depth %d: err = %v, want document.limit.depth", depth, err)
		}
		if d := time.Since(start); d > scaleBound {
			t.Errorf("depth %d took %v, want under %v", depth, d, scaleBound)
		}
	}
}
