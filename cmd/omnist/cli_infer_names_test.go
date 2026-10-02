package main

import (
	"strings"
	"testing"

	"github.com/omnist-dev/omnist-go/osd"
)

// S-8: infer derives record names ASCII-only, so keys that used to yield a
// name Validate and osd.Write reject now infer cleanly (exit 0), in every
// input format, and the printed schema reads back.
func TestCLIInferNamesAreValidForOddKeys(t *testing.T) {
	type tc struct {
		name, from, file, in string
		want                 string // a record name expected in the output
	}
	cases := []tc{
		{"json accent", "json", "a.json", `{"éclair":{"a":1}}`, "record _clair"},
		{"json cjk", "json", "a.json", `{"日本":{"a":1}}`, "record __"},
		{"json digits", "json", "a.json", `{"123":{"a":1}}`, "record _123"},
		{"json space", "json", "a.json", `{"a b":{"a":1}}`, "record A_b"},
		{"json collision", "json", "a.json", `{"日本":{"a":1},"中国":{"a":1}}`, "record __2"},
		{"yaml accent", "yaml", "a.yaml", "éclair: {a: 1}\n", "record _clair"},
		{"yaml cjk", "yaml", "a.yaml", "日本: {a: 1}\n", "record __"},
		{"yaml digits", "yaml", "a.yaml", "\"123\": {a: 1}\n", "record _123"},
		{"yaml space", "yaml", "a.yaml", "a b: {a: 1}\n", "record A_b"},
		{"toml accent", "toml", "a.toml", "[\"éclair\"]\na = 1\n", "record _clair"},
		{"toml cjk", "toml", "a.toml", "[\"日本\"]\na = 1\n", "record __"},
		{"toml digits", "toml", "a.toml", "[\"123\"]\na = 1\n", "record _123"},
		{"toml space", "toml", "a.toml", "[\"a b\"]\na = 1\n", "record A_b"},
		{"xml accent", "xml", "a.xml", "<root><éclair><a>1</a></éclair></root>", "record _clair"},
		{"xml cjk", "xml", "a.xml", "<root><日本><a>1</a></日本></root>", "record __"},
	}
	for _, c := range cases {
		f := writeTemp(t, c.file, c.in)
		code, stdout, stderr := runCLI(t, []string{"infer", "--from", c.from, f}, "")
		if code != ExitOK || stderr != "" {
			t.Errorf("%s: exit %d stderr %q", c.name, code, stderr)
			continue
		}
		if !strings.Contains(stdout, c.want) {
			t.Errorf("%s: output lacks %q:\n%s", c.name, c.want, stdout)
		}
		if _, err := osd.Read(stdout); err != nil {
			t.Errorf("%s: output does not read back: %v\n%s", c.name, err, stdout)
		}
	}
}

// An empty key is not a legal field label (S-5/§5.4), so infer fails with
// schema.empty-label, not with a bad generated record name.
func TestCLIInferEmptyKeyFailsOnTheLabel(t *testing.T) {
	f := writeTemp(t, "a.json", `{"":{"a":1}}`)
	code, stdout, stderr := runCLI(t, []string{"infer", "--from", "json", f}, "")
	if code != ExitProblem || stdout != "" || !strings.Contains(stderr, "schema.empty-label") || strings.Contains(stderr, "invalid-name") {
		t.Errorf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}
