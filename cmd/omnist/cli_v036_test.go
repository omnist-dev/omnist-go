package main

import (
	"strings"
	"testing"
)

// DIV-46 / E-4a: document.limit.int-digits is positioned by a Document path on
// every read route, never a line:col.
func TestCLIIntDigitsReportsDocumentPath(t *testing.T) {
	long := strings.Repeat("9", 4301)
	cases := []struct{ from, src, path string }{
		{"json", `{"r":{"n":` + long + `}}`, "$.r.n"},
		{"yaml", "r:\n  n: " + long + "\n", "$.r.n"},
		{"toml", "[r]\nn = " + long + "\n", "$.r.n"},
		{"oml", "r: { n: " + long + " }", "$.r.n"},
	}
	for _, c := range cases {
		code, stdout, stderr := runCLI(t, []string{"parse", "--from", c.from, "-"}, c.src)
		if code != ExitProblem || stdout != "" {
			t.Errorf("%s: exit %d stdout %.60q", c.from, code, stdout)
		}
		want := "omnist parse: " + c.path + ": document.limit.int-digits:"
		if !strings.HasPrefix(stderr, want) {
			t.Errorf("%s: stderr %.120q, want prefix %q", c.from, stderr, want)
		}
	}
	// A parse.* error keeps its text position.
	_, _, stderr := runCLI(t, []string{"parse", "--from", "json", "-"}, `{"a":`)
	if !strings.Contains(stderr, "parse.") || strings.Contains(stderr, "$.") {
		t.Errorf("parse error stderr %q", stderr)
	}
}
