package json

import (
	"math"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// edgesOf renders a Document's top-level edges as label=value strings.
func edgesOf(t *testing.T, text string) []string {
	t.Helper()
	d, err := Read(text, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("Read(%q): %v", text, err)
	}
	var out []string
	for _, e := range d.Node.Edges {
		if v, ok := e.Target.Value(); ok && !v.IsNull {
			switch v.Scalar.Kind {
			case omnist.KindInteger:
				out = append(out, e.Label+"="+v.Scalar.Int.String())
			default:
				out = append(out, e.Label+"=?")
			}
			continue
		}
		out = append(out, e.Label+"={}")
	}
	return out
}

// C-11: the surviving edge keeps the first position and the last value.
func TestDuplicateKeysKeepFirstPositionLastValue(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"a":1,"a":2}`, "a=2"},
		{`{"a":1,"b":2,"a":3}`, "a=3 b=2"},
		{`{"b":0,"a":1,"c":2,"a":3,"b":4}`, "b=4 a=3 c=2"},
		{`{"a":1,"b":2,"a":[3,4]}`, "a=3 a=4 b=2"},
		{`{"a":[1,2],"b":3,"a":4}`, "a=4 b=3"},
		{`{"a":1,"b":2,"a":[]}`, "b=2"},
		{`{"a":{"k":1},"b":2,"a":3}`, "a=3 b=2"},
		{`{"a":1,"a":{"z":9}}`, "a={}"},
		{`{"o":{"x":1,"y":2,"x":3},"p":0}`, "o={} p=0"},
	}
	for _, tc := range cases {
		got := ""
		for i, s := range edgesOf(t, tc.in) {
			if i > 0 {
				got += " "
			}
			got += s
		}
		if got != tc.want {
			t.Errorf("Read(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDuplicateKeysCollapseInsideNestedObject(t *testing.T) {
	d, err := Read(`{"o":{"x":1,"y":2,"x":3}}`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	o, _ := d.Node.Edges[0].Target.Node()
	if len(o.Edges) != 2 || o.Edges[0].Label != "x" {
		t.Fatalf("nested edges = %+v", o.Edges)
	}
	if v, _ := o.Edges[0].Target.Value(); v.Scalar.Int.Int64() != 3 {
		t.Errorf("x = %v, want 3", v.Scalar.Int)
	}
}

// D-29: a literal beyond binary64 reads as an infinity, an underflow as 0.
func TestNumberRangeLiterals(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want float64
	}{{`{"a":1e999}`, math.Inf(1)}, {`{"a":-1e999}`, math.Inf(-1)}, {`{"a":1e-999}`, 0}} {
		d, err := Read(tc.in, omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("Read(%s): %v", tc.in, err)
		}
		v, _ := d.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != omnist.KindNumber || v.Scalar.Num != tc.want {
			t.Errorf("Read(%s) = %+v, want number %v", tc.in, v.Scalar, tc.want)
		}
	}
}

// The Document root is level 1 and the first node (spec 2.4, E-4a).
func TestRootCountsAgainstDepthAndNodeLimits(t *testing.T) {
	l := omnist.DefaultLimits()
	l.MaxDepth = 3
	if _, err := Read(`{"a":{"b":{"c":1}}}`, l); err != nil {
		t.Fatalf("depth 3 document at limit 3: %v", err)
	}
	_, err := Read(`{"a":{"b":{"c":{"d":1}}}}`, l)
	if pe, ok := err.(*omnist.ParseError); !ok || pe.Code != omnist.CodeDocumentLimitDepth || pe.Path != "$" {
		t.Fatalf("got %v, want document.limit.depth at $", err)
	}
	l = omnist.DefaultLimits()
	l.MaxNodes = 2
	if _, err := Read(`{"a":{"b":1}}`, l); err != nil {
		t.Fatalf("two nodes at limit 2: %v", err)
	}
	_, err = Read(`{"a":{"b":{"c":1}}}`, l)
	if pe, ok := err.(*omnist.ParseError); !ok || pe.Code != omnist.CodeDocumentLimitNodes {
		t.Fatalf("got %v, want document.limit.nodes", err)
	}
}

// An error inside an object that is an array element is reported, not dropped.
func TestErrorInsideArrayElementObject(t *testing.T) {
	l := omnist.DefaultLimits()
	l.MaxDepth = 1
	if _, err := Read(`{"a":[{"b":1}]}`, l); err == nil {
		t.Error("expected a depth-limit error for an object element")
	}
}

// E-4a: the digit limit is reported at the literal's Document path.
func TestDigitLimitPaths(t *testing.T) {
	l := omnist.DefaultLimits()
	l.MaxIntDigits = 3
	for in, path := range map[string]string{
		`{"a":1000}`:        "$.a",
		`{"a":[1,1000]}`:    "$.a[1]",
		`{"o":{"b":-1000}}`: "$.o.b",
		`1000`:              "$",
	} {
		_, err := Read(in, l)
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits || pe.Path != path {
			t.Errorf("Read(%s) = %v, want int-digits at %s", in, err, path)
		}
	}
}
