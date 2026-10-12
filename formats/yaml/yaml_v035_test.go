package yaml

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// C-14: the hex digits of a YAML escape are lowercase, and a backslash pair is
// not mistaken for an escape.
func TestLowerEscapeHex(t *testing.T) {
	for in, want := range map[string]string{
		`"x\x1Fy"`:       `"x\x1fy"`,
		`"x\x7Fy"`:       `"x\x7fy"`,
		`"x\uFFFFy"`:     `"x\uffffy"`,
		`"x\UABCDEF01y"`: `"x\Uabcdef01y"`,
		`"a\\x1Fb"`:      `"a\\x1Fb"`,
		`"a\nb\"c"`:      `"a\nb\"c"`,
		`plain`:          `plain`,
	} {
		if got := lowerEscapeHex(in); got != want {
			t.Errorf("lowerEscapeHex(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteEscapesAreLowercase(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("x\x1f\x7f\uffffy"))))
	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a: \"x\\x1f\\x7f\\uffffy\"\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// E-37: a datetime is written with seconds, the T separator and no tag or
// quotes; a time is quoted and stringified.
func TestWriteDatetimeIsBare(t *testing.T) {
	dt := omnist.DateTimeValue{
		Date: omnist.DateValue{Year: 2024, Month: 1, Day: 15},
		Time: omnist.TimeValue{Hour: 12, HasOffset: true, OffsetSeconds: 3600},
	}
	d := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewDateTimeScalar(dt))))
	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a: 2024-01-15T12:00:00+01:00\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	dt.Time.HasOffset = false
	d = omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewDateTimeScalar(dt))))
	if out, _, _ = Write(d); out != "a: 2024-01-15T12:00:00\n" {
		t.Errorf("out = %q", out)
	}
}

// C-17: YAML 1.1 number spellings as PyYAML's SafeLoader resolves them.
func TestNumberSpellings(t *testing.T) {
	type want struct {
		kind omnist.ScalarKind
		i    int64
		f    float64
		s    string
	}
	cases := map[string]want{
		"0o17":      {kind: omnist.KindString, s: "0o17"},
		"08":        {kind: omnist.KindString, s: "08"},
		"0:5":       {kind: omnist.KindString, s: "0:5"},
		"1e3":       {kind: omnist.KindString, s: "1e3"},
		"1.0e3":     {kind: omnist.KindString, s: "1.0e3"},
		"1.0e+3":    {kind: omnist.KindNumber, f: 1000},
		"010":       {kind: omnist.KindInteger, i: 8},
		"190:20:30": {kind: omnist.KindInteger, i: 190*3600 + 20*60 + 30},
		"1:30.5":    {kind: omnist.KindNumber, f: 90.5},
		"-1:30.5":   {kind: omnist.KindNumber, f: -90.5},
		"+1_0:30.5": {kind: omnist.KindNumber, f: 630.5},
		"5.":        {kind: omnist.KindNumber, f: 5},
		".5":        {kind: omnist.KindNumber, f: 0.5},
	}
	for text, w := range cases {
		d, err := Read("v: "+text+"\n", omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		s := v.Scalar
		switch {
		case s.Kind != w.kind,
			w.kind == omnist.KindInteger && s.Int.Int64() != w.i,
			w.kind == omnist.KindNumber && s.Num != w.f,
			w.kind == omnist.KindString && s.Str != w.s:
			t.Errorf("%q: got %+v, want %+v", text, s, w)
		}
	}
}
