package toml

import (
	"math"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

func tomlErr(t *testing.T, text string, l omnist.Limits) *omnist.ParseError {
	t.Helper()
	_, err := Read(text, l)
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("Read(%q) error = %v (%T), want *ParseError", text, err, err)
	}
	return pe
}

// C-12: a table that repeats a key is parse.codec-syntax.
func TestDuplicateKeysAreRejected(t *testing.T) {
	for _, in := range []string{
		"a = 1\na = 2\n",
		"[t]\nk = 1\nk = 2\n",
		"a = 1\n\"a\" = 2\n",
		"a.b = 1\na.b = 2\n",
		"t = {k = 1, k = 2}\n",
		"a = [1]\na = 2\n",
		"[a.b]\n[a]\nb = 1\n",
	} {
		pe := tomlErr(t, in, omnist.DefaultLimits())
		if pe.Code != omnist.CodeParseCodecSyntax || pe.Line < 1 || pe.Path == "" {
			t.Errorf("Read(%q) = %+v, want parse.codec-syntax with a position", in, pe)
		}
	}
	for _, in := range []string{
		"[x]\na = 1\n\n[y]\na = 2\n",
		"[[x]]\na = 1\n[[x]]\na = 2\n",
		"a.b = 1\na.c = 2\n",
		"m = [1, 2]\n",
	} {
		if _, err := Read(in, omnist.DefaultLimits()); err != nil {
			t.Errorf("Read(%q): %v", in, err)
		}
	}
}

// E-4a / D-28: the digit limit is reported at the literal's Document path.
func TestDigitLimitPaths(t *testing.T) {
	l := omnist.DefaultLimits()
	l.MaxIntDigits = 3
	for in, path := range map[string]string{
		"a = 1000\n":                       "$.a",
		"a = 0x3E8\n":                      "$.a",
		"a = 0b1111101000\n":               "$.a",
		"a = 1_000\n":                      "$.a",
		"[t]\na = 1000\n":                  "$.t.a",
		"[t.u]\na = 1000\n":                "$.t.u.a",
		"a = [1, 1000]\n":                  "$.a[1]",
		"a = [1000, 1]\n":                  "$.a[0]",
		"t = {b = 1000}\n":                 "$.t.b",
		"t = {u = {b = 1000}}\n":           "$.t.u.b",
		"t = {a.b = 1000}\n":               "$.t.a.b",
		"t = [{b = 1}, {b = 1000}]\n":      "$.t[1].b",
		"t = [{b = [1, 1000]}]\n":          "$.t[0].b[1]",
		"[[x]]\na = 1\n[[x]]\na = 1000\n":  "$.x[1].a",
		"[[x]]\n[x.y]\na = 1000\n":         "$.x.y.a",
		"x.y = 1\n[x.z]\nq = {w = 1000}\n": "$.x.z.q.w",
	} {
		pe := tomlErr(t, in, l)
		if pe.Code != omnist.CodeDocumentLimitIntDigits || pe.Path != path {
			t.Errorf("Read(%q) = %+v, want int-digits at %s", in, pe, path)
		}
	}
	if _, err := Read("a = 0x3E7\n", l); err != nil {
		t.Errorf("999 by hex: %v", err)
	}
	pe := tomlErr(t, "a = 0x"+strings.Repeat("F", 3600)+"\n", omnist.DefaultLimits())
	if pe.Code != omnist.CodeDocumentLimitIntDigits || pe.Path != "$.a" {
		t.Errorf("3,600-digit hex: %+v", pe)
	}
}

func TestRootCountsAgainstLimits(t *testing.T) {
	l := omnist.DefaultLimits()
	l.MaxDepth = 3
	if pe := tomlErr(t, "[a.b.c]\nd = 1\n", l); pe.Code != omnist.CodeDocumentLimitDepth || pe.Path != "$" {
		t.Errorf("depth: %+v", pe)
	}
	if _, err := Read("[a.b]\nd = 1\n", l); err != nil {
		t.Errorf("at limit: %v", err)
	}
	// A bare key after a deep table header keeps that table's depth.
	if pe := tomlErr(t, "[a.b]\nt = {u = 1}\n", l); pe.Code != omnist.CodeDocumentLimitDepth {
		t.Errorf("inline table below a table: %+v", pe)
	}
	l = omnist.DefaultLimits()
	l.MaxNodes = 1
	if pe := tomlErr(t, "[a]\nb = 1\n", l); pe.Code != omnist.CodeDocumentLimitNodes || pe.Path != "$" {
		t.Errorf("nodes: %+v", pe)
	}
}

func TestOverflowFloatReadsAsInfinity(t *testing.T) {
	d, err := Read("a = 1e999\nb = -1e999\nc = 1e-999\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{math.Inf(1), math.Inf(-1), 0}
	for i, e := range d.Node.Edges {
		v, _ := e.Target.Value()
		if v.Scalar.Num != want[i] {
			t.Errorf("edge %d = %v, want %v", i, v.Scalar.Num, want[i])
		}
	}
}
