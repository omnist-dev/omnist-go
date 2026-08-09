// Command conformance runs omnist-go against every Track 2 (JSON-vector)
// test in vendor/omnist-spec/test-suite/, per spec §8.5 and issue #31. It
// prints a per-operation and overall pass/fail/skip report and exits
// nonzero iff the fail count is nonzero (§8.5.5 -- skip is not failure).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	conformance "github.com/omnist-dev/omnist-go/tools/conformance"
)

func main() {
	suiteDir := flag.String("suite", "", "path to vendor/omnist-spec/test-suite (default: relative to this repo's root)")
	verbose := flag.Bool("v", false, "print every fail/skip with its reason")
	flag.Parse()

	dir := *suiteDir
	if dir == "" {
		dir = findSuiteDir()
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "conformance: could not locate vendor/omnist-spec/test-suite; pass -suite explicitly")
		os.Exit(2)
	}

	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "conformance: walking %s: %v\n", dir, err)
		os.Exit(2)
	}
	sort.Strings(files)
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "conformance: no *.json vector files found under %s\n", dir)
		os.Exit(2)
	}

	type tally struct{ pass, fail, skip int }
	byOp := map[string]*tally{}
	overall := &tally{}
	var fails, skips []conformance.Result

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "conformance: reading %s: %v\n", path, err)
			os.Exit(2)
		}
		vectors, err := conformance.LoadVectorFile(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "conformance: parsing %s: %v\n", path, err)
			os.Exit(2)
		}
		for _, v := range vectors {
			res := conformance.RunVector(v)
			t, ok := byOp[v.Operation]
			if !ok {
				t = &tally{}
				byOp[v.Operation] = t
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
	}

	fmt.Println("omnist-go Track 2 (JSON-vector) conformance report")
	fmt.Println("mode: exact-code diagnostics matching (this repository emits the full §8.3")
	fmt.Println("taxonomy from issue #1 onward; see report for empirical verification detail)")
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
	fmt.Printf("TOTAL: pass=%d fail=%d skip=%d (of %d vectors)\n", overall.pass, overall.fail, overall.skip, overall.pass+overall.fail+overall.skip)

	if len(skips) > 0 {
		fmt.Println()
		fmt.Println("Skips (every skip cites a reason, per §8.5.5):")
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
				b, _ := json.MarshalIndent(r.Vector, "    ", "  ")
				fmt.Printf("    %s\n", b)
			}
		}
	}

	if overall.fail != 0 {
		os.Exit(1)
	}
}

// findSuiteDir walks upward from the working directory looking for
// vendor/omnist-spec/test-suite, so `go run ./tools/conformance` works
// from the repo root without requiring -suite.
func findSuiteDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := wd; ; {
		candidate := filepath.Join(dir, "vendor", "omnist-spec", "test-suite")
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
