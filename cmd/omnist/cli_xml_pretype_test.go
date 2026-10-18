package main

import (
	"strings"
	"testing"
)

const pretypeSchema = `record R { "n" [0,]: integer }
record Root { "r": R }
root Root
`

// The CLI pretypes XML with --schema (spec E-4a), so an integer leaf over the
// digit limit is refused at read time, as xml.ReadWithSchema does for library
// callers (omnist-go#140).
func TestCLIXMLPretypeDigitLimit(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", pretypeSchema)
	long := strings.Repeat("9", 4301)
	cases := []struct{ name, src, path string }{
		{"leaf", "<r><n>" + long + "</n></r>", "$.r.n"},
		{"repeated", "<r><n>1</n><n>" + long + "</n></r>", "$.r.n[1]"},
	}
	for _, cmd := range []string{"validate", "materialize"} {
		for _, c := range cases {
			code, stdout, stderr := runCLI(t, []string{cmd, "--from", "xml", "--schema", schemaPath, "-"}, c.src)
			if code != ExitProblem || stdout != "" {
				t.Errorf("%s %s: exit %d stdout %q, want %d and empty stdout", cmd, c.name, code, stdout, ExitProblem)
			}
			if !strings.Contains(stderr, "document.limit.int-digits") || !strings.Contains(stderr, c.path) {
				t.Errorf("%s %s: stderr %.200q, want document.limit.int-digits at %s", cmd, c.name, stderr, c.path)
			}
		}
	}
}

// Within the limit the XML still validates, and non-XML input does not use
// the pretyping path.
func TestCLIXMLPretypeWithinLimitAndOtherFormats(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", pretypeSchema)
	code, stdout, stderr := runCLI(t, []string{"validate", "--from", "xml", "--schema", schemaPath, "-"}, "<r><n>12</n></r>")
	if code != ExitOK || stdout != "" || stderr != "" {
		t.Errorf("xml: exit %d stdout %q stderr %q", code, stdout, stderr)
	}
	code, _, _ = runCLI(t, []string{"validate", "--from", "json", "--schema", schemaPath, "-"}, `{"r":{"n":12}}`)
	if code != ExitOK {
		t.Errorf("json: exit %d", code)
	}
}

// A syntax error keeps its text position (not a Document path).
func TestCLIXMLPretypeSyntaxErrorKeepsPosition(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", pretypeSchema)
	code, _, stderr := runCLI(t, []string{"validate", "--from", "xml", "--schema", schemaPath, "-"}, "<r><n>1</r>")
	if code != ExitProblem || !strings.Contains(stderr, "parse.") || strings.Contains(stderr, "$.") {
		t.Errorf("exit %d stderr %q", code, stderr)
	}
}

// An invalid schema is an operation problem for XML input too, as for every
// other format.
func TestCLIXMLPretypeBadSchema(t *testing.T) {
	bad := writeTemp(t, "bad.osd", "not a schema")
	code, _, stderr := runCLI(t, []string{"validate", "--from", "xml", "--schema", bad, "-"}, "<r/>")
	if code != ExitProblem || stderr == "" {
		t.Errorf("exit %d stderr %q", code, stderr)
	}
}
