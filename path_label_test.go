package omnist

import "testing"

// E-36: a label that would make a path ambiguous or unsafe is bracket-quoted.
func TestPathLabelSegment(t *testing.T) {
	for label, want := range map[string]string{
		"name":     ".name",
		"a b":      ".a b",
		"$":        ".$",
		"é":        ".é",
		"0":        ".0",
		"":         `[""]`,
		"a.b":      `["a.b"]`,
		"item[0]":  `["item[0]"]`,
		"a]b":      `["a]b"]`,
		`q"x`:      `["q\"x"]`,
		`a\b`:      `["a\\b"]`,
		"l1\nl2":   `["l1\nl2"]`,
		"a\tb\rc":  `["a\tb\rc"]`,
		"a\x1fb":   `["a\u001fb"]`,
		"a\x00b":   `["a\u0000b"]`,
		"a\x7fb":   `["a\u007fb"]`,
		"a\u0085b": `["a\u0085b"]`,
		"a\u2028b": `["a\u2028b"]`,
		"a\u2029b": `["a\u2029b"]`,
		"é.ü":      `["é.ü"]`,
	} {
		if got := PathLabelSegment(label); got != want {
			t.Errorf("PathLabelSegment(%q) = %s, want %s", label, got, want)
		}
	}
	p := RootPath().Child("a.b", 1, true).Child("c", 0, false).Child("d e", 0, false)
	if got, want := p.String(), `$["a.b"][1].c.d e`; got != want {
		t.Errorf("Path.String() = %s, want %s", got, want)
	}
}
