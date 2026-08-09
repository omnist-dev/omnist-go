// Command conformance-fixtures runs omnist-go against every Track 1
// (fixture-based) test in vendor/omnist-spec/conformance/fixtures/, per
// spec docs/conformance-harness.md and issue #55. It prints a
// per-operation and overall pass/fail/skip report and exits nonzero iff
// the fail count is nonzero (§8.5.5's reporting discipline, reused here
// per the harness doc's §7).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	conformance "github.com/omnist-dev/omnist-go/tools/conformance"
)

func main() {
	fixturesDir := flag.String("fixtures", "", "path to vendor/omnist-spec/conformance/fixtures (default: relative to this repo's root)")
	verbose := flag.Bool("v", false, "print every fail/skip with its reason")
	flag.Parse()

	dir := *fixturesDir
	if dir == "" {
		dir = findFixturesDir()
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "conformance-fixtures: could not locate vendor/omnist-spec/conformance/fixtures; pass -fixtures explicitly")
		os.Exit(2)
	}

	cases, err := conformance.WalkFixtures(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "conformance-fixtures: %v\n", err)
		os.Exit(2)
	}
	if len(cases) == 0 {
		fmt.Fprintf(os.Stderr, "conformance-fixtures: no fixtures found under %s\n", dir)
		os.Exit(2)
	}

	type tally struct{ pass, fail, skip int }
	byOp := map[string]*tally{}
	overall := &tally{}
	var fails, skips []conformance.Result

	for _, fc := range cases {
		res := conformance.RunFixture(fc)
		t, ok := byOp[fc.Operation]
		if !ok {
			t = &tally{}
			byOp[fc.Operation] = t
		}
		switch res.Status {
		case conformance.StatusPass:
			t.pass++
			overall.pass++
		case conformance.StatusFail:
			t.fail++
			overall.fail++
			fails = append(fails, res)
		case conformance.StatusSkip:
			t.skip++
			overall.skip++
			skips = append(skips, res)
		}
	}

	fmt.Println("omnist-go Track 1 (fixture-based) conformance report")
	fmt.Println("source: vendor/omnist-spec/conformance/fixtures/ (_referee-self-test skipped --")
	fmt.Println("already hand-ported as referee_test.go)")
	fmt.Println()
	ops := make([]string, 0, len(byOp))
	for op := range byOp {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	for _, op := range ops {
		t := byOp[op]
		fmt.Printf("  %-20s pass=%-4d fail=%-4d skip=%-4d\n", op, t.pass, t.fail, t.skip)
	}
	fmt.Println()
	fmt.Printf("TOTAL: pass=%d fail=%d skip=%d (of %d fixtures)\n", overall.pass, overall.fail, overall.skip, overall.pass+overall.fail+overall.skip)

	if len(skips) > 0 {
		fmt.Println()
		fmt.Println("Skips (every skip cites a reason, per §8.5.5's discipline, reused here):")
		for _, r := range skips {
			fmt.Printf("  SKIP %-70s %s\n", r.Vector.Name, r.Reason)
		}
	}
	if len(fails) > 0 {
		fmt.Println()
		fmt.Println("Failures:")
		for _, r := range fails {
			fmt.Printf("  FAIL %-70s %s\n", r.Vector.Name, r.Reason)
			if *verbose {
				fmt.Printf("    purpose: %s\n", r.Vector.Purpose)
			}
		}
	}

	if overall.fail != 0 {
		os.Exit(1)
	}
}

// findFixturesDir walks upward from the working directory looking for
// vendor/omnist-spec/conformance/fixtures, so `go run
// ./tools/conformance/cmd/conformance-fixtures` works from the repo root
// without requiring -fixtures.
func findFixturesDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := wd; ; {
		candidate := filepath.Join(dir, "vendor", "omnist-spec", "conformance", "fixtures")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
