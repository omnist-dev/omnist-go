package toml

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

func flatKeys(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "k%d = %d\n", i, i)
	}
	return b.String()
}

func oneLineInlineTable(n int) string {
	var b strings.Builder
	b.WriteString("t = {")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "k%d = %d", i, i)
	}
	b.WriteString("}\n")
	return b.String()
}

func tableHeaders(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "[t%d]\nk = 1\n", i)
	}
	return b.String()
}

func TestReadScalesLinearly(t *testing.T) {
	const n = 20000
	for name, src := range map[string]string{
		"flat keys":             flatKeys(n),
		"one-line inline table": oneLineInlineTable(n),
	} {
		start := time.Now()
		if _, err := Read(src, omnist.DefaultLimits()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d := time.Since(start); d > scaleBound {
			t.Errorf("%s: reading %d keys or tables took %v, want under %v (quadratic position computation, issue #132)", name, n, d, scaleBound)
		}
	}
}

func BenchmarkReadTableHeaders(b *testing.B) {
	src := tableHeaders(50000)
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Read(src, omnist.DefaultLimits()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadFlatKeys(b *testing.B) {
	src := flatKeys(50000)
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Read(src, omnist.DefaultLimits()); err != nil {
			b.Fatal(err)
		}
	}
}
