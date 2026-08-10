// Command check_doc_examples is the CI gate for issue #62: every fenced
// code block in docs/*.md must carry an HTML-comment marker, directly
// above or below the block, declaring how it's verified:
//
//   - <!-- verified-by: path/to/test.go::TestOrExampleName --> -- a real
//     Go test backs this exact block's literal content/output.
//   - <!-- doc-illustrative --> -- the block is conceptual or non-runnable
//     (a CLI transcript, a design-decision snapshot, a bare fragment) and
//     deliberately has no backing test.
//
// The gate only requires a marker on code blocks that are new or changed
// relative to the PR's base branch (default origin/main, see -base) --
// pre-existing unmarked blocks at the time this gate ships are not
// retroactively flagged. It never gates on blocks that are unchanged.
//
// Known trap (see docs/workflow-playbook.md Sec7): running this against an
// uncommitted working tree gives a false "passed", because the diff is
// against the base branch's committed history, not the working tree.
// Commit before trusting a local run.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// codeBlock is one fenced ```...``` block found in a Markdown file, along
// with the marker (if any) found on the line immediately before the
// opening fence or immediately after the closing fence.
type codeBlock struct {
	file string
	// fenceStart/fenceEnd are 1-indexed line numbers of the opening and
	// closing ``` lines, inclusive.
	fenceStart, fenceEnd int
	// markerLine is the 1-indexed line number the marker comment was found
	// on (0 if no marker was found at all).
	markerLine int
	marker     string // "verified-by:<ref>", "doc-illustrative", or "" if absent/invalid
}

// spanLines returns every line number this block "occupies" for the
// purpose of matching against a diff: the fence lines themselves, plus the
// marker line if present, plus (if absent) the line directly above the
// fence -- since that's where a marker would need to be added, and a
// docs-diff that only touches the fence body should still be caught even
// when the marker line itself wasn't touched.
func (b codeBlock) spanLines() []int {
	lines := []int{b.fenceStart, b.fenceEnd}
	if b.markerLine != 0 {
		lines = append(lines, b.markerLine)
	} else if b.fenceStart > 1 {
		lines = append(lines, b.fenceStart-1)
	}
	return lines
}

var markerRe = regexp.MustCompile(`^\s*<!--\s*(verified-by:\s*\S+|doc-illustrative)\s*-->\s*$`)

// parseMarkdown scans a Markdown file's lines for fenced code blocks and
// the marker comment adjacent to each (checked above the opening fence
// first, then below the closing fence).
func parseMarkdown(path string, lines []string) []codeBlock {
	var blocks []codeBlock
	inFence := false
	fenceStart := 0
	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if !inFence && strings.HasPrefix(trimmed, "```") {
			inFence = true
			fenceStart = lineNo
			continue
		}
		if inFence && trimmed == "```" {
			inFence = false
			b := codeBlock{file: path, fenceStart: fenceStart, fenceEnd: lineNo}

			// Marker directly above the opening fence.
			if fenceStart-2 >= 0 && fenceStart-2 < len(lines) {
				above := lines[fenceStart-2]
				if m := markerRe.FindStringSubmatch(above); m != nil {
					b.markerLine = fenceStart - 1
					b.marker = normalizeMarker(m[1])
				}
			}
			// Marker directly below the closing fence, if none found above.
			if b.marker == "" && lineNo < len(lines) {
				below := lines[lineNo]
				if m := markerRe.FindStringSubmatch(below); m != nil {
					b.markerLine = lineNo + 1
					b.marker = normalizeMarker(m[1])
				}
			}
			blocks = append(blocks, b)
			continue
		}
	}
	return blocks
}

func normalizeMarker(s string) string {
	return strings.TrimSpace(s)
}

// changedLines runs `git diff <base>...HEAD -- <path>` and returns the set
// of 1-indexed line numbers in the new (HEAD-side) version of the file
// that were added or are part of a changed hunk.
func changedLines(repoRoot, base, path string) (map[int]bool, error) {
	cmd := exec.Command("git", "diff", "--unified=0", base+"...HEAD", "--", path)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git diff failed: %v\n%s", err, ee.Stderr)
		}
		return nil, err
	}
	lines := map[int]bool{}
	hunkRe := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		m := hunkRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		start := atoi(m[1])
		count := 1
		if m[2] != "" {
			count = atoi(m[2])
		}
		if count == 0 {
			// A pure deletion hunk (count 0) touches no new-side lines; the
			// reported start is the line *after* which the deletion
			// happened. Nothing to mark on the new side.
			continue
		}
		for l := start; l < start+count; l++ {
			lines[l] = true
		}
	}
	return lines, sc.Err()
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	// Keep a trailing empty element out of the slice for a final newline.
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func main() {
	base := flag.String("base", "origin/main", "base ref/branch to diff against (git ... syntax, e.g. origin/main)")
	docsDir := flag.String("docs", "docs", "directory containing the *.md files to check")
	flag.Parse()

	repoRoot, err := gitRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "check_doc_examples:", err)
		os.Exit(2)
	}

	pattern := filepath.Join(repoRoot, *docsDir, "*.md")
	files, err := filepath.Glob(pattern)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check_doc_examples:", err)
		os.Exit(2)
	}
	sort.Strings(files)

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "check_doc_examples: no files matched %s\n", pattern)
		os.Exit(2)
	}

	var failures []string
	totalBlocks, markedBlocks := 0, 0

	for _, f := range files {
		rel, err := filepath.Rel(repoRoot, f)
		if err != nil {
			rel = f
		}
		rel = filepath.ToSlash(rel)

		lines, err := readLines(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "check_doc_examples:", err)
			os.Exit(2)
		}
		blocks := parseMarkdown(rel, lines)

		changed, err := changedLines(repoRoot, *base, rel)
		if err != nil {
			fmt.Fprintln(os.Stderr, "check_doc_examples:", err)
			os.Exit(2)
		}

		for _, b := range blocks {
			totalBlocks++
			if b.marker != "" {
				markedBlocks++
				continue
			}
			// Unmarked. Only a gate failure if this block is new/changed.
			touched := false
			for _, ln := range b.spanLines() {
				if changed[ln] {
					touched = true
					break
				}
			}
			if touched {
				failures = append(failures, fmt.Sprintf(
					"%s:%d: code block (lines %d-%d) added or changed but has no <!-- verified-by: ... --> or <!-- doc-illustrative --> marker",
					b.file, b.fenceStart, b.fenceStart, b.fenceEnd))
			}
		}
	}

	fmt.Printf("check_doc_examples: %d code block(s) scanned across %d file(s), %d marked, %d unmarked\n",
		totalBlocks, len(files), markedBlocks, totalBlocks-markedBlocks)

	if len(failures) > 0 {
		fmt.Println("\nFAIL: the following added/changed code blocks are missing a marker:")
		for _, f := range failures {
			fmt.Println(" -", f)
		}
		os.Exit(1)
	}

	fmt.Println("PASS: every added/changed code block has a verified-by or doc-illustrative marker.")
}

func gitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
