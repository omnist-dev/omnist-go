package xml

import (
	"math"
	"math/big"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- interleaving preservation on write: the highest-priority test ---

func TestWriteXMLPreservesInterleaving(t *testing.T) {
	root := omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("x", omnist.ScalarValue(omnist.NewStringScalar("X"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B")))
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", root))

	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	want := "<root><m>A</m><x>X</x><m>B</m></root>"
	if out != want {
		t.Fatalf("got %q, want %q (edges must not be grouped by label)", out, want)
	}

	// Read it back and confirm the interleaving survived the full
	// round trip, not just the writer's raw string output.
	back, _, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(back, d) {
		t.Errorf("round trip mismatch: got %+v, want %+v", back, d)
	}
}

// --- repeated elements, no wrapper, 2+ occurrences ---

func TestWriteXMLRepeatedElementsNoWrapper(t *testing.T) {
	root := omnist.NewNode().
		AddValue("items", omnist.ScalarValue(omnist.NewStringScalar("a"))).
		AddValue("items", omnist.ScalarValue(omnist.NewStringScalar("b"))).
		AddValue("items", omnist.ScalarValue(omnist.NewStringScalar("c")))
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", root))
	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	want := "<root><items>a</items><items>b</items><items>c</items></root>"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// --- single-document-element enforcement on write ---

func TestWriteXMLRejectsMultipleTopLevelEdges(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("1"))).
		AddValue("b", omnist.ScalarValue(omnist.NewStringScalar("2"))))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("expected an error for multiple top-level edges")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("expected omnist.Diagnostic, got %T: %v", err, err)
	}
	if diag.Code != omnist.CodeFormatMultipleRoots {
		t.Errorf("got code %v, want %v", diag.Code, omnist.CodeFormatMultipleRoots)
	}
}

func TestWriteXMLRejectsEmptyTopLevel(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode())
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("expected an error for a omnist.Document with no top-level edges")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Fatalf("got %v (%T), want omnist.CodeWriteUnsupportedValue", err, err)
	}
}

// --- bare-scalar-root rejection, same category as TOML precedent ---

func TestWriteXMLRejectsBareScalarRoot(t *testing.T) {
	d := omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("hi")))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("expected an error for a bare scalar root")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Fatalf("got %v (%T), want omnist.CodeWriteUnsupportedValue", err, err)
	}
}

// --- round-trip property ---

func TestWriteXMLRoundTripsWorkedExample(t *testing.T) {
	src := `<order><id>A1</id><status>shipped</status><total>29.97</total><address><street>1 Main</street><city>London</city></address><items><sku>W</sku><qty>3</qty><price>9.99</price></items><items><sku>G</sku><qty>1</qty><price>9.99</price></items></order>`
	d, _, err := Read(src, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(back, d) {
		t.Errorf("round trip mismatch:\n first read:  %+v\n second read: %+v", d, back)
	}
}

func TestWriteXMLRoundTripsSelfClosingLeaf(t *testing.T) {
	d, _, err := Read(`<a/>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(back, d) {
		t.Errorf("round trip mismatch: got %+v, want %+v", back, d)
	}
}

// --- scalar text rendering ---

func TestWriteXMLScalarKinds(t *testing.T) {
	root := omnist.NewNode().
		AddValue("str", omnist.ScalarValue(omnist.NewStringScalar("hi<&>\""))).
		AddValue("int", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(42)))).
		AddValue("num", omnist.ScalarValue(omnist.NewNumberScalar(3.5))).
		AddValue("bool", omnist.ScalarValue(omnist.NewBooleanScalar(true))).
		AddValue("date", omnist.ScalarValue(omnist.NewDateScalar(omnist.DateValue{Year: 2024, Month: 1, Day: 15})))
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", root))
	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<str>hi&lt;&amp;&gt;&#34;</str>") {
		t.Errorf("expected escaped string content, got %q", out)
	}
	if !strings.Contains(out, "<int>42</int>") {
		t.Errorf("expected <int>42</int>, got %q", out)
	}
	if !strings.Contains(out, "<num>3.5</num>") {
		t.Errorf("expected <num>3.5</num>, got %q", out)
	}
	if !strings.Contains(out, "<bool>true</bool>") {
		t.Errorf("expected <bool>true</bool>, got %q", out)
	}
	if !strings.Contains(out, "<date>2024-01-15</date>") {
		t.Errorf("expected <date>2024-01-15</date>, got %q", out)
	}
}

func TestWriteXMLBooleanFalseAndTemporalKinds(t *testing.T) {
	root := omnist.NewNode().
		AddValue("bfalse", omnist.ScalarValue(omnist.NewBooleanScalar(false))).
		AddValue("time", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 13, Minute: 30}))).
		AddValue("dt", omnist.ScalarValue(omnist.NewDateTimeScalar(omnist.DateTimeValue{
			Date: omnist.DateValue{Year: 2024, Month: 1, Day: 15},
			Time: omnist.TimeValue{Hour: 9, Minute: 0},
		})))
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", root))
	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<bfalse>false</bfalse>") {
		t.Errorf("expected <bfalse>false</bfalse>, got %q", out)
	}
	if !strings.Contains(out, "<time>13:30</time>") {
		t.Errorf("expected <time>13:30</time>, got %q", out)
	}
	if !strings.Contains(out, "<dt>2024-01-15T09:00</dt>") {
		t.Errorf("expected <dt>2024-01-15T09:00</dt>, got %q", out)
	}
}

func TestWriteXMLNaNInfinity(t *testing.T) {
	root := omnist.NewNode().
		AddValue("nan", omnist.ScalarValue(omnist.NewNumberScalar(math.NaN()))).
		AddValue("inf", omnist.ScalarValue(omnist.NewNumberScalar(math.Inf(1)))).
		AddValue("ninf", omnist.ScalarValue(omnist.NewNumberScalar(math.Inf(-1))))
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", root))
	out, _, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<nan>NaN</nan>", "<inf>Infinity</inf>", "<ninf>-Infinity</ninf>"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got %q", want, out)
		}
	}
}

// --- null leaf: unconditional write failure, not a reported adjustment ---

func TestWriteXMLNullLeafFailsUnconditionally(t *testing.T) {
	// Per spec section 8.3.8/8.3.9 (updated 2026-08-24, issue #97/#96):
	// a null leaf's only XML fallback -- an empty element -- collides
	// with a genuinely different, independently-valid input (a real
	// empty-string leaf writes identically), so the write now fails
	// unconditionally instead of substituting the ambiguous empty
	// element with a warning.
	d := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.NullValue()))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("Write: want error for a null leaf, got ok write")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("err = %T, want omnist.Diagnostic", err)
	}
	if diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Fatalf("got code %v, want omnist.CodeWriteUnsupportedValue", diag.Code)
	}
	if diag.Severity != omnist.SeverityError {
		t.Errorf("got severity %v, want omnist.SeverityError", diag.Severity)
	}
	if diag.Path != "$.a" {
		t.Errorf("got path %q, want $.a", diag.Path)
	}
}

// --- empty internal node: unconditional write failure ---

func TestWriteXMLEmptyInternalNodeFailsUnconditionally(t *testing.T) {
	// Per spec section 8.3.8/8.3.9 (updated 2026-08-24, issue #98): an
	// internal node with zero edges has no XML spelling distinct from an
	// empty-string leaf -- both would write as the self-closing form --
	// so the write now fails unconditionally. This is a genuine gap fix
	// (no diagnostic existed for this at all before), not a strictness
	// flip.
	d := omnist.NodeDocument(omnist.NewNode().AddNode("a", omnist.NewNode()))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("Write: want error for an empty internal node, got ok write")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("err = %T, want omnist.Diagnostic", err)
	}
	if diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Fatalf("got code %v, want omnist.CodeWriteUnsupportedValue", diag.Code)
	}
	if diag.Path != "$.a" {
		t.Errorf("got path %q, want $.a", diag.Path)
	}
}

// TestWriteXMLEmptyStringLeafStillWrites confirms the empty-internal-node
// failure above does NOT extend to a Value target holding an empty
// string -- that case is fine and unrelated (writeXMLElement's own doc
// comment on the self-closing form already discusses this distinction).
func TestWriteXMLEmptyStringLeafStillWrites(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewStringScalar(""))))
	out, _, err := Write(d)
	if err != nil {
		t.Fatalf("Write: unexpected error for an empty-string leaf: %v", err)
	}
	if out != "<a></a>" {
		t.Errorf("out = %q, want <a></a>", out)
	}
}

// --- invalid element names are rejected, not silently mangled ---

// --- invalid element names are rejected, not silently mangled ---

func TestWriteXMLRejectsInvalidLabel(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("1bad", omnist.ScalarValue(omnist.NewStringScalar("x"))))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("expected an error for an invalid XML element name")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Fatalf("got %v (%T), want omnist.CodeWriteUnsupportedValue", err, err)
	}
}

// TestWriteXMLRejectsSpaceInLabelMatchesVector backs conformance vector
// formats-xml/basic/label-with-illegal-xml-characters-cannot-be-written
// (issue #96): confirmed by cross-port verification that this port's
// isValidXMLName/writeXMLElement already fail closed here (hard error,
// no sanitize-and-succeed fallback), so this is a confirming test, not a
// bug fix -- see the issue's own comment thread.
func TestWriteXMLRejectsSpaceInLabelMatchesVector(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().AddValue("my label", omnist.ScalarValue(omnist.NewStringScalar("hi")))))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("expected an error for a space in a label")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want omnist.Diagnostic", err)
	}
	if diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("got code %v, want omnist.CodeWriteUnsupportedValue", diag.Code)
	}
	if diag.Path != "$.root.my label" {
		t.Errorf("got path %q, want $.root.my label", diag.Path)
	}
}

func TestWriteXMLRejectsColonInLabel(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("ns:b", omnist.ScalarValue(omnist.NewStringScalar("x"))))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("expected an error for a colon in a label")
	}
}

func TestWriteXMLAcceptsNestedInvalidLabel(t *testing.T) {
	// A nested (non-root) label must be validated too, not just the root.
	inner := omnist.NewNode().AddValue("bad label", omnist.ScalarValue(omnist.NewStringScalar("x")))
	d := omnist.NodeDocument(omnist.NewNode().AddNode("root", inner))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("expected an error for an invalid nested label")
	}
}

func TestIsValidXMLName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"a", true},
		{"a1", true},
		{"a-b.c_d", true},
		{"_leading", true},
		{"", false},
		{"1a", false},
		{"-a", false},
		{".a", false},
		{"a b", false},
		{"a:b", false},
		{"a<b", false},
	}
	for _, c := range cases {
		if got := isValidXMLName(c.name); got != c.want {
			t.Errorf("isValidXMLName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
