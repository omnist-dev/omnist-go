package omnist

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

func TestWriteTOMLGroupingAndCountOne(t *testing.T) {
	d := NodeDocument(NewNode().
		AddValue("a", ScalarValue(NewIntegerScalar(big.NewInt(1)))).
		AddValue("m", ScalarValue(NewStringScalar("A"))).
		AddValue("m", ScalarValue(NewStringScalar("B"))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q, read back %+v, want %+v", out, back, d)
	}
	if !strings.Contains(out, "[") {
		t.Errorf("expected an inline array for the repeated label m, got %q", out)
	}
}

func TestWriteTOMLSingleLabelIsBareValueNotList(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("a", ScalarValue(NewIntegerScalar(big.NewInt(1)))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "[") {
		t.Errorf("a single-child group must not be rendered as a list: got %q", out)
	}
}

// --- worked example round-trip ---

func TestWriteTOMLRoundTripsWorkedExample(t *testing.T) {
	address := NewNode().AddValue("street", ScalarValue(NewStringScalar("1 Main"))).
		AddValue("city", ScalarValue(NewStringScalar("London")))
	item1 := NewNode().AddValue("sku", ScalarValue(NewStringScalar("W"))).
		AddValue("qty", ScalarValue(NewIntegerScalar(big.NewInt(3)))).
		AddValue("price", ScalarValue(NewNumberScalar(9.99)))
	item2 := NewNode().AddValue("sku", ScalarValue(NewStringScalar("G"))).
		AddValue("qty", ScalarValue(NewIntegerScalar(big.NewInt(1)))).
		AddValue("price", ScalarValue(NewNumberScalar(9.99)))
	order := NewNode().
		AddValue("id", ScalarValue(NewStringScalar("A1"))).
		AddValue("status", ScalarValue(NewStringScalar("shipped"))).
		AddValue("total", ScalarValue(NewNumberScalar(29.97))).
		AddNode("address", address).
		AddNode("items", item1).
		AddNode("items", item2)
	d := NodeDocument(NewNode().AddNode("order", order))

	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- native temporal round-trip: date/time/datetime all survive bare ---

func TestWriteTOMLTemporalRoundTripAllThreeKinds(t *testing.T) {
	d := NodeDocument(NewNode().
		AddValue("d", ScalarValue(NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 1}))).
		AddValue("t", ScalarValue(NewTimeScalar(TimeValue{Hour: 12, Minute: 30, Second: 45}))).
		AddValue("dt", ScalarValue(NewDateTimeScalar(DateTimeValue{
			Date: DateValue{Year: 2024, Month: 1, Day: 1},
			Time: TimeValue{Hour: 12},
		}))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
	// Confirm none of the three kinds degraded to a string — unlike
	// YAML's writer, which has to stringify (quote) KindTime because
	// YAML's own core schema has no bare spelling for it. TOML does.
	for _, e := range back.Node.Edges {
		v, _ := e.Target.Value()
		if v.Scalar.Kind == KindString {
			t.Errorf("label %s degraded to a string on round-trip: %q", e.Label, out)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, " = ", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected line %q in %q", line, out)
		}
		if strings.Contains(parts[1], `"`) {
			t.Errorf("expected the value in %q to be written bare (unquoted)", line)
		}
	}
}

func TestWriteTOMLOffsetDateTimeRoundTrips(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("dt", ScalarValue(NewDateTimeScalar(DateTimeValue{
		Date: DateValue{Year: 2024, Month: 1, Day: 1},
		Time: TimeValue{Hour: 12, Minute: 0, Second: 0, HasOffset: true, OffsetSeconds: 5*3600 + 30*60},
	}))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- null: write-time adjustment, reported not invented ---

func TestWriteTOMLNullReportsAdjustment(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("coupon", NullValue()))
	_, err := WriteTOML(d)
	if err == nil {
		t.Fatal("expected an error: TOML has no null spelling")
	}
	diag, ok := err.(Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want Diagnostic", err)
	}
	if diag.Code != CodeFormatNullUnrepresentable {
		t.Errorf("Diagnostic.Code = %s, want %s", diag.Code, CodeFormatNullUnrepresentable)
	}
	if diag.Path != "$.coupon" {
		t.Errorf("Diagnostic.Path = %s, want $.coupon", diag.Path)
	}
	if diag.Error() == "" {
		t.Error("Diagnostic.Error() returned empty string")
	}
}

func TestWriteTOMLNullInsideNestedNodeReportsAdjustment(t *testing.T) {
	d := NodeDocument(NewNode().AddNode("order", NewNode().AddValue("coupon", NullValue())))
	_, err := WriteTOML(d)
	diag, ok := err.(Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want Diagnostic", err)
	}
	if diag.Path != "$.order.coupon" {
		t.Errorf("Diagnostic.Path = %s, want $.order.coupon", diag.Path)
	}
}

func TestWriteTOMLNullInsideListReportsAdjustment(t *testing.T) {
	d := NodeDocument(NewNode().
		AddValue("m", ScalarValue(NewStringScalar("A"))).
		AddValue("m", NullValue()))
	_, err := WriteTOML(d)
	diag, ok := err.(Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want Diagnostic", err)
	}
	if diag.Code != CodeFormatNullUnrepresentable {
		t.Errorf("Diagnostic.Code = %s, want %s", diag.Code, CodeFormatNullUnrepresentable)
	}
	if diag.Path != "$.m[1]" {
		t.Errorf("Diagnostic.Path = %s, want $.m[1]", diag.Path)
	}
}

// --- bare-scalar-root rejection ---

func TestWriteTOMLBareScalarRootFails(t *testing.T) {
	d := ValueDocument(ScalarValue(NewStringScalar("x")))
	_, err := WriteTOML(d)
	if err == nil {
		t.Fatal("expected an error: TOML has no bare-scalar-root spelling")
	}
	diag, ok := err.(Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want Diagnostic", err)
	}
	if diag.Code != CodeWriteUnsupportedValue {
		t.Errorf("Diagnostic.Code = %s, want %s", diag.Code, CodeWriteUnsupportedValue)
	}
}

// --- integer/float distinction preserved ---

func TestWriteTOMLNumberAlwaysDistinctFromInteger(t *testing.T) {
	d := NodeDocument(NewNode().
		AddValue("i", ScalarValue(NewIntegerScalar(big.NewInt(2)))).
		AddValue("f", ScalarValue(NewNumberScalar(2.0))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

func TestWriteTOMLNaNInfinityNativeSpellings(t *testing.T) {
	d := NodeDocument(NewNode().
		AddValue("a", ScalarValue(NewNumberScalar(math.NaN()))).
		AddValue("b", ScalarValue(NewNumberScalar(math.Inf(1)))).
		AddValue("c", ScalarValue(NewNumberScalar(math.Inf(-1)))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch (NaN-aware): wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- KindTime with an (unusual) offset set: dropped, not malformed ---

func TestWriteTOMLBareTimeWithOffsetDropsOffset(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("t", ScalarValue(NewTimeScalar(TimeValue{
		Hour: 12, Minute: 0, Second: 0, HasOffset: true, OffsetSeconds: 3600,
	}))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	v, _ := back.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != KindTime || v.Scalar.Time.HasOffset {
		t.Errorf("got %+v, want a bare local time with no offset", v)
	}
}

// --- string escaping ---

func TestWriteTOMLStringEscaping(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("s", ScalarValue(NewStringScalar("a\nb\tc\"d\\e\bf\rg\fh\x01i"))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- keys always quoted, including ones that would need it ---

func TestWriteTOMLQuotesEveryKey(t *testing.T) {
	d := NodeDocument(NewNode().AddValue("has space", ScalarValue(NewIntegerScalar(big.NewInt(1)))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- booleans ---

func TestWriteTOMLBooleans(t *testing.T) {
	d := NodeDocument(NewNode().
		AddValue("t", ScalarValue(NewBooleanScalar(true))).
		AddValue("f", ScalarValue(NewBooleanScalar(false))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- empty node ---

func TestWriteTOMLEmptyNode(t *testing.T) {
	d := NodeDocument(NewNode())
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("got %q, want empty output for an empty node", out)
	}
}

// --- nested node (inline table) ---

func TestWriteTOMLNestedNode(t *testing.T) {
	d := NodeDocument(NewNode().AddNode("a", NewNode().AddValue("b", ScalarValue(NewIntegerScalar(big.NewInt(1))))))
	out, err := WriteTOML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadTOML(out, DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}
