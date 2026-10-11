package yaml

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

func parseErr(t *testing.T, text string, l omnist.Limits) *omnist.ParseError {
	t.Helper()
	_, err := Read(text, l)
	pe, ok := err.(*omnist.ParseError)
	if !ok {
		t.Fatalf("Read(%q) error = %v (%T), want *ParseError", text, err, err)
	}
	return pe
}

// C-12: a repeated key is parse.codec-syntax, whatever the spelling or shape.
func TestDuplicateKeysAreRejected(t *testing.T) {
	for _, in := range []string{
		"a: 1\nb: 2\na: 3\n",
		"{a: 1, a: 2}\n",
		"o:\n  k: 1\n  k: 2\n",
		"m:\n  - {a: 1, a: 2}\n",
		"a: 1\n\"a\": 2\n",
		"a: 1\na: [2, 3]\n",
	} {
		pe := parseErr(t, in, omnist.DefaultLimits())
		if pe.Code != omnist.CodeParseCodecSyntax || pe.Line < 1 {
			t.Errorf("Read(%q) = %+v, want parse.codec-syntax", in, pe)
		}
	}
	for _, in := range []string{"x: {a: 1}\ny: {a: 2}\n", "a: 1\nA: 2\n"} {
		if _, err := Read(in, omnist.DefaultLimits()); err != nil {
			t.Errorf("Read(%q): %v", in, err)
		}
	}
}

// C-13: !!omap, !!pairs and !!set are refused with format.tag-forbidden at $.
func TestForbiddenTags(t *testing.T) {
	for _, in := range []string{
		"t: !!omap [{k: 1}]\n",
		"t: !!pairs [{k: 1}]\n",
		"t: !!set {x, y}\n",
		"t: !!set\n  ? x\n",
		"!!omap [{k: 1}]\n",
		"t: !<tag:yaml.org,2002:set> {x}\n",
		"a:\n  b: !!omap []\n",
	} {
		pe := parseErr(t, in, omnist.DefaultLimits())
		if pe.Code != omnist.CodeFormatTagForbidden || pe.Path != "$" {
			t.Errorf("Read(%q) = %+v, want format.tag-forbidden at $", in, pe)
		}
	}
	if _, err := Read("t: !!map {k: 1}\ns: !!seq [1]\n", omnist.DefaultLimits()); err != nil {
		t.Errorf("!!map and !!seq stay accepted: %v", err)
	}
}

// D-27: the tag on a << decides whether it is a merge key.
func TestMergeKeyFollowsItsTag(t *testing.T) {
	d, err := Read("p: &p {x: 1}\nq: {!!str <<: *p}\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	q, _ := d.Node.Edges[1].Target.Node()
	if len(q.Edges) != 1 || q.Edges[0].Label != "<<" {
		t.Errorf("!!str << should be an ordinary key, got %+v", q.Edges)
	}
	d, err = Read("p: &p {x: 1}\nq: {!!merge <<: *p, y: 2}\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	q, _ = d.Node.Edges[1].Target.Node()
	if len(q.Edges) != 2 || q.Edges[0].Label != "x" {
		t.Errorf("!!merge << should merge, got %+v", q.Edges)
	}
}

// E-4a / D-28: the digit limit is reported at the literal's Document path and
// counts the value's decimal digits.
func TestDigitLimitPathAndValueCount(t *testing.T) {
	l := omnist.DefaultLimits()
	l.MaxIntDigits = 3
	for in, path := range map[string]string{
		"a: 1000\n":               "$.a",
		"a: 0x3E8\n":              "$.a",
		"a: 1_000\n":              "$.a",
		"o:\n  b: 1000\n":         "$.o.b",
		"l: [1, 1000]\n":          "$.l[1]",
		"l:\n  - {k: 1000}\n":     "$.l[0].k",
		"1000\n":                  "$",
		"o:\n  l: [1, 2, 1000]\n": "$.o.l[2]",
	} {
		pe := parseErr(t, in, l)
		if pe.Code != omnist.CodeDocumentLimitIntDigits || pe.Path != path {
			t.Errorf("Read(%q) = %+v, want int-digits at %s", in, pe, path)
		}
	}
	// 0x3E7 = 999: three digits by value, though five by spelling.
	if _, err := Read("a: 0x3E7\n", l); err != nil {
		t.Errorf("0x3E7: %v", err)
	}
	// A key resolving to an over-long integer is reported at the enclosing node.
	if pe := parseErr(t, "1000: x\n", l); pe.Path != "$" || pe.Code != omnist.CodeDocumentLimitIntDigits {
		t.Errorf("key: %+v", pe)
	}
	// A 3,600-digit hex literal is 4,335 decimal digits: refused by value.
	pe := parseErr(t, "a: 0x"+strings.Repeat("F", 3600)+"\n", omnist.DefaultLimits())
	if pe.Code != omnist.CodeDocumentLimitIntDigits || pe.Path != "$.a" {
		t.Errorf("hex: %+v", pe)
	}
}

func TestRootCountsAgainstLimits(t *testing.T) {
	l := omnist.DefaultLimits()
	l.MaxDepth = 3
	if pe := parseErr(t, "a:\n  b:\n    c:\n      d: 1\n", l); pe.Code != omnist.CodeDocumentLimitDepth || pe.Path != "$" {
		t.Errorf("depth: %+v", pe)
	}
	if _, err := Read("a:\n  b:\n    c: 1\n", l); err != nil {
		t.Errorf("at limit: %v", err)
	}
	l = omnist.DefaultLimits()
	l.MaxNodes = 1
	if pe := parseErr(t, "a:\n  b: 1\n", l); pe.Code != omnist.CodeDocumentLimitNodes {
		t.Errorf("nodes: %+v", pe)
	}
}

func TestOverflowAndUnderflowFloats(t *testing.T) {
	d, err := Read("a: -1.0e+999\nb: 1.0e-999\n", omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := d.Node.Edges[0].Target.Value()
	b, _ := d.Node.Edges[1].Target.Value()
	if a.Scalar.Kind != omnist.KindNumber || a.Scalar.Num > -1e308 || b.Scalar.Num != 0 {
		t.Errorf("a=%+v b=%+v", a.Scalar, b.Scalar)
	}
}

// Writer: labels are bare when safe, quoted when bare would not be a string.
func TestWriterLabelQuoting(t *testing.T) {
	n := omnist.NewNode()
	for _, l := range []string{"name", "on", "yes", "null", "1a", "has space", "x-y", "true"} {
		n.AddValue(l, omnist.ScalarValue(omnist.NewBooleanScalar(true)))
	}
	out, _, err := Write(omnist.NodeDocument(n))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\nname: true", "\"on\": true", "\"yes\": true", "\"null\": true", "\"1a\": true", "\"has space\": true", "x-y: true", "\"true\": true"} {
		if !strings.Contains("\n"+out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil || len(back.Node.Edges) != 8 {
		t.Errorf("round trip: %v %+v", err, back)
	}
}

// E-34: NEL is written \N and reported once per string, labels included.
func TestWriterNEL(t *testing.T) {
	n := omnist.NewNode().
		AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("x\u0085y"))).
		AddValue("b", omnist.ScalarValue(omnist.NewStringScalar("ok"))).
		AddValue("l\u0085", omnist.ScalarValue(omnist.NewStringScalar("v")))
	out, diags, err := Write(omnist.NodeDocument(n))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `a: "x\Ny"`) || strings.ContainsRune(out, '\u0085') {
		t.Errorf("output = %q", out)
	}
	if len(diags) != 2 || diags[0].Path != "$.a" || diags[1].Path != "$" {
		t.Fatalf("diags = %+v", diags)
	}
	for _, d := range diags {
		if d.Code != omnist.CodeFormatStringLineBreakChar || d.Severity != omnist.SeverityWarning {
			t.Errorf("diag = %+v", d)
		}
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := back.Node.Edges[0].Target.Value()
	if v.Scalar.Str != "x\u0085y" {
		t.Errorf("round trip = %q", v.Scalar.Str)
	}
}
