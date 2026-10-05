package main

import (
	"strconv"
	"strings"
	"testing"
)

// D-23: every command that reads an input takes --max-input-bytes N, refuses an
// input of more than N bytes before parsing it (and says how to raise N), and
// accepts one of exactly N bytes.

const maxInputDoc = `{"name":"x","age":1}`

// maxInputCommands returns, for each command that reads input, its argument
// list given the temp-file paths and the flag value to pass.
func maxInputCommands(doc, schema string) map[string]func(flag ...string) []string {
	with := func(prefix []string, rest []string) func(flag ...string) []string {
		return func(flag ...string) []string {
			args := append([]string{}, prefix...)
			args = append(args, flag...)
			return append(args, rest...)
		}
	}
	return map[string]func(flag ...string) []string{
		"parse":                  with([]string{"parse", "--from", "json"}, []string{doc}),
		"validate":               with([]string{"validate", "--from", "json", "--schema", schema}, []string{doc}),
		"materialize":            with([]string{"materialize", "--from", "json", "--schema", schema}, []string{doc}),
		"infer":                  with([]string{"infer", "--from", "json"}, []string{doc}),
		"schema normalize":       with([]string{"schema", "normalize"}, []string{schema}),
		"schema prune":           with([]string{"schema", "prune"}, []string{schema}),
		"schema extract":         with([]string{"schema", "extract", "--keep", "name,age,nickname"}, []string{schema}),
		"schema compatible-with": with([]string{"schema", "compatible-with"}, []string{schema, schema}),
		"schema equivalent":      with([]string{"schema", "equivalent"}, []string{schema, schema}),
		"schema is-empty":        with([]string{"schema", "is-empty"}, []string{schema}),
		"lint":                   with([]string{"lint"}, []string{schema}),
	}
}

func TestEveryReadingCommandHonoursMaxInputBytes(t *testing.T) {
	doc := writeTemp(t, "doc.json", maxInputDoc)
	schema := writeTemp(t, "schema.osd", testSchema)
	for name, build := range maxInputCommands(doc, schema) {
		t.Run(name, func(t *testing.T) {
			// A maximum of 3 bytes is below every input here: refused with the
			// code and the way to raise it, before any parse.
			code, _, stderr := runCLI(t, build("--max-input-bytes", "3"), "")
			if code != ExitUsage {
				t.Errorf("exit = %d, want %d; stderr: %s", code, ExitUsage, stderr)
			}
			for _, want := range []string{"document.limit.input-size", "--max-input-bytes", "3 bytes"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q does not mention %q", stderr, want)
				}
			}
			// The two-dash and one-dash spellings are the same flag, and a
			// maximum above every input accepts it.
			for _, spelling := range []string{"--max-input-bytes", "-max-input-bytes"} {
				code, _, stderr = runCLI(t, build(spelling, "1048576"), "")
				if code == ExitUsage {
					t.Errorf("%s with a large maximum: exit = %d, stderr: %s", spelling, code, stderr)
				}
			}
			code, _, stderr = runCLI(t, build(), "")
			if code == ExitUsage {
				t.Errorf("default maximum: exit = %d, stderr: %s", code, stderr)
			}
		})
	}
}

func TestMaxInputBytesBoundaryThroughTheCLI(t *testing.T) {
	doc := writeTemp(t, "doc.json", maxInputDoc)
	n := len(maxInputDoc)
	if code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "--max-input-bytes", strconv.Itoa(n), doc}, ""); code != ExitOK {
		t.Errorf("an input of exactly the maximum: exit = %d, stderr: %s", code, stderr)
	}
	code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "--max-input-bytes", strconv.Itoa(n - 1), doc}, "")
	if code != ExitUsage || !strings.Contains(stderr, "--max-input-bytes") {
		t.Errorf("one byte over: exit = %d, stderr: %s", code, stderr)
	}
	// stdin takes the same path.
	code, _, stderr = runCLI(t, []string{"parse", "--from", "json", "--max-input-bytes", strconv.Itoa(n - 1), "-"}, maxInputDoc)
	if code != ExitUsage || !strings.Contains(stderr, "stdin") {
		t.Errorf("stdin one byte over: exit = %d, stderr: %s", code, stderr)
	}
	// A BOM is counted: three bytes more than the same text without one.
	bom := "\xEF\xBB\xBF" + maxInputDoc
	if code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "--max-input-bytes", strconv.Itoa(n + 3), "-"}, bom); code != ExitOK {
		t.Errorf("BOM at the maximum: exit = %d, stderr: %s", code, stderr)
	}
	if code, _, _ := runCLI(t, []string{"parse", "--from", "json", "--max-input-bytes", strconv.Itoa(n + 2), "-"}, bom); code != ExitUsage {
		t.Errorf("BOM one over: exit = %d, want %d", code, ExitUsage)
	}
}

func TestMaxInputBytesFlagRejectsNonsense(t *testing.T) {
	doc := writeTemp(t, "doc.json", maxInputDoc)
	for _, tc := range []struct{ v, why string }{
		{"0", "must be a positive"}, {"-1", "must be a positive"}, {"abc", "must be a positive"},
		{"", "must be a positive"}, {"1.5", "must be a positive"}, {"1073741825", "exceeds recommended safety ceiling"},
	} {
		code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "--max-input-bytes", tc.v, doc}, "")
		if code != ExitUsage || !strings.Contains(stderr, "max-input-bytes") || !strings.Contains(stderr, tc.why) {
			t.Errorf("--max-input-bytes %q: exit = %d, stderr: %s; want a usage error saying %q", tc.v, code, stderr, tc.why)
		}
	}
	// The ceiling itself is allowed.
	if code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "--max-input-bytes", "1073741824", doc}, ""); code != ExitOK {
		t.Errorf("--max-input-bytes at the ceiling: exit = %d, stderr: %s", code, stderr)
	}
}

func TestMaxInputBytesAppearsInHelp(t *testing.T) {
	_, _, stderr := runCLI(t, []string{"parse", "-h"}, "")
	if !strings.Contains(stderr, "-max-input-bytes") || !strings.Contains(stderr, "67108864") {
		t.Errorf("help does not document the flag and its default: %s", stderr)
	}
}
