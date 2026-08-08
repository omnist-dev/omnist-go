package omnist

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

func TestWriteYAMLGroupingAndCountOne(t *testing.T) {
	n := NewNode()
	n.Edges = append(n.Edges,
		Edge{Label: "m", Target: ValueTarget(ScalarValue(NewStringScalar("A")))},
		Edge{Label: "x", Target: ValueTarget(ScalarValue(NewStringScalar("X")))},
		Edge{Label: "m", Target: ValueTarget(ScalarValue(NewStringScalar("B")))},
	)
	out, err := WriteYAML(NodeDocument(n))
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadYAML(%q): %v", out, err)
	}
	want := NodeDocument(NewNode().
		AddValue("m", ScalarValue(NewStringScalar("A"))).
		AddValue("m", ScalarValue(NewStringScalar("B"))).
		AddValue("x", ScalarValue(NewStringScalar("X"))))
	if !docEqual(back, want) {
		t.Errorf("got %+v, want %+v (from %q)", back, want, out)
	}
}

func TestWriteYAMLSingleLabelIsBareValueNotList(t *testing.T) {
	n := NewNode().AddValue("tag", ScalarValue(NewStringScalar("x")))
	out, err := WriteYAML(NodeDocument(n))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "-") {
		t.Errorf("single-occurrence label should not be written as a sequence: %q", out)
	}
}

// --- temporal writing (see yaml_writer.go's WriteYAML doc comment for the
// reasoning: date/datetime are written bare, time is forced to a quoted
// string). ---

// Note: the *label* ("d:", "dt:") is always double-quoted regardless of
// leaf kind (see buildYAMLNode's doc comment) — these tests check that the
// *value* itself is not quoted/stringified, not that the whole line is
// quote-free, since asserting the latter would also be sensitive to
// yaml.v3's own choice of whether to render an explicit "!!timestamp" tag
// token for disambiguation, which is an emission detail this package does
// not control and does not need to: what actually matters, and what these
// tests check, is that the kind survives the round trip as date/datetime,
// not that it gets demoted to a string.

func TestWriteYAMLDateWrittenBareAndRoundTripsAsDate(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("d", ScalarValue(NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 1}))))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"2024`) {
		t.Errorf("a date leaf's value should be written bare (unquoted): %q", out)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("date did not round-trip: wrote %q, got %+v", out, back)
	}
}

func TestWriteYAMLDateTimeWrittenBareAndRoundTripsAsDateTime(t *testing.T) {
	dt := DateTimeValue{Date: DateValue{Year: 2024, Month: 1, Day: 1}, Time: TimeValue{Hour: 12, Minute: 30}}
	d := NodeDocument(NewNode().AddValue("dt", ScalarValue(NewDateTimeScalar(dt))))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"2024`) {
		t.Errorf("a datetime leaf's value should be written bare (unquoted): %q", out)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("datetime did not round-trip: wrote %q, got %+v", out, back)
	}
}

// TestWriteYAMLTimeForcedToQuotedStringOnWrite is the single most
// important temporal-writing test in this issue: it confirms the sharp
// edge (a bare time-of-day would silently become the sexagesimal integer
// on read-back) is avoided by construction, and documents what a KindTime
// leaf actually becomes on round-trip (a string, not a time) as a direct
// consequence.
func TestWriteYAMLTimeForcedToQuotedStringOnWrite(t *testing.T) {
	tv := TimeValue{Hour: 12, Minute: 0, Second: 0}
	d := NodeDocument(NewNode().AddValue("t", ScalarValue(NewTimeScalar(tv))))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"12:00"`) {
		t.Errorf("expected the time to be written quoted as \"12:00\", got %q", out)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := back.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindString || v.Scalar.Str != "12:00" {
		t.Errorf("got %+v, want the string \"12:00\" (proving the quoting prevented sexagesimal misresolution)", v.Scalar)
	}
}

func TestWriteYAMLTimeVariants(t *testing.T) {
	cases := []struct {
		name string
		t    TimeValue
		want string
	}{
		{"hour-minute", TimeValue{Hour: 1, Minute: 2}, "01:02"},
		{"with-seconds", TimeValue{Hour: 1, Minute: 2, Second: 3}, "01:02:03"},
		{"with-fraction", TimeValue{Hour: 1, Minute: 2, Second: 3, Nanosecond: 500000000}, "01:02:03.5"},
		{"with-offset", TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: 3600}, "01:02+01:00"},
	}
	for _, c := range cases {
		d := NodeDocument(NewNode().AddValue("t", ScalarValue(NewTimeScalar(c.t))))
		out, err := WriteYAML(d)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: got %q, want it to contain %q", c.name, out, c.want)
		}
	}
}

// --- NaN/Infinity: written using YAML's own core-schema spellings,
// unlike WriteJSON (which has no native representation to fall back on). ---

func TestWriteYAMLNaNInfinityRoundTrip(t *testing.T) {
	cases := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, f := range cases {
		d := NodeDocument(NewNode().AddValue("v", ScalarValue(NewNumberScalar(f))))
		out, err := WriteYAML(d)
		if err != nil {
			t.Fatal(err)
		}
		back, err := ReadYAML(out, DefaultLimits())
		if err != nil {
			t.Fatalf("ReadYAML(%q): %v", out, err)
		}
		v, _ := back.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != KindNumber {
			t.Fatalf("got kind %v, want number", v.Scalar.Kind)
		}
		switch {
		case math.IsNaN(f):
			if !math.IsNaN(v.Scalar.Num) {
				t.Errorf("wrote %q, got %v, want NaN", out, v.Scalar.Num)
			}
		default:
			if !math.IsInf(v.Scalar.Num, int(math.Copysign(1, f))) {
				t.Errorf("wrote %q, got %v, want Inf matching sign of %v", out, v.Scalar.Num, f)
			}
		}
	}
}

func TestWriteYAMLNumberAlwaysDistinctFromInteger(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("v", ScalarValue(NewNumberScalar(5.0))))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := back.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindNumber {
		t.Errorf("wrote %q, got kind %v, want number (5.0 must not become integer 5 on round-trip)", out, v.Scalar.Kind)
	}
}

// --- strings/labels are always quoted, even when they'd collide with
// core-schema resolution if left bare. ---

func TestWriteYAMLStringThatWouldCollideWithResolutionStaysAString(t *testing.T) {
	cases := []string{"on", "yes", "no", "null", "true", "2024-01-01", "12:00:00", "5"}
	for _, s := range cases {
		d := NodeDocument(NewNode().AddValue("v", ScalarValue(NewStringScalar(s))))
		out, err := WriteYAML(d)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		back, err := ReadYAML(out, DefaultLimits())
		if err != nil {
			t.Fatalf("%q: ReadYAML(%q): %v", s, out, err)
		}
		v, _ := back.Node.Edges[0].Target.Value()
		if v.Scalar.Kind != KindString || v.Scalar.Str != s {
			t.Errorf("%q: wrote %q, got %+v, want string %q preserved", s, out, v.Scalar, s)
		}
	}
}

func TestWriteYAMLLabelThatWouldCollideWithResolutionStaysAString(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("on", ScalarValue(NewStringScalar("v"))))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadYAML(%q): %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("wrote %q, got %+v, want the \"on\" label preserved as a string label", out, back)
	}
}

// --- null, booleans, integers, nested structure, empty node, bare value ---

func TestWriteYAMLNull(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("v", NullValue()))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("wrote %q, got %+v", out, back)
	}
}

func TestWriteYAMLBoolean(t *testing.T) {
	for _, b := range []bool{true, false} {
		d := NodeDocument(NewNode().AddValue("v", ScalarValue(NewBooleanScalar(b))))
		out, err := WriteYAML(d)
		if err != nil {
			t.Fatal(err)
		}
		back, err := ReadYAML(out, DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if !docEqual(d, back) {
			t.Errorf("wrote %q, got %+v, want %v", out, back, b)
		}
	}
}

func TestWriteYAMLHugeInteger(t *testing.T) {
	huge, ok := new(big.Int).SetString(strings.Repeat("9", 50), 10)
	if !ok {
		t.Fatal("test setup failed")
	}
	d := NodeDocument(NewNode().AddValue("v", ScalarValue(NewIntegerScalar(huge))))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("huge integer did not round-trip: wrote %q", out)
	}
}

func TestWriteYAMLBareValueDocument(t *testing.T) {
	d := ValueDocument(ScalarValue(NewStringScalar("bare")))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("wrote %q, got %+v", out, back)
	}
}

func TestWriteYAMLEmptyNode(t *testing.T) {
	out, err := WriteYAML(NodeDocument(NewNode()))
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadYAML(%q): %v", out, err)
	}
	if !back.IsNode || len(back.Node.Edges) != 0 {
		t.Errorf("got %+v, want an empty node", back)
	}
}

// TestWriteYAMLInvalidUTF8StringFails exercises WriteYAML's error path.
// This is not defensive/dead code: yaml.v3's encoder genuinely refuses to
// marshal a !!str scalar whose text is not valid UTF-8 (confirmed
// empirically — see WriteYAML's doc comment on this branch), and
// Document.Value.Scalar.Str is a plain Go string, which (unlike a Go
// source-code string literal) carries no compiler-enforced UTF-8
// guarantee, so a Document built programmatically with invalid-UTF-8 bytes
// in a string leaf is a legitimate input this function must handle.
func TestWriteYAMLInvalidUTF8StringFails(t *testing.T) {
	invalid := string([]byte{0xff, 0xfe})
	d := NodeDocument(NewNode().AddValue("v", ScalarValue(NewStringScalar(invalid))))
	_, err := WriteYAML(d)
	if err == nil {
		t.Fatal("expected an error writing a string with invalid UTF-8 bytes")
	}
}

func TestWriteYAMLNestedNode(t *testing.T) {
	child := NewNode().AddValue("x", ScalarValue(NewIntegerScalar(big.NewInt(1))))
	d := NodeDocument(NewNode().AddNode("child", child))
	out, err := WriteYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadYAML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("wrote %q, got %+v", out, back)
	}
}
