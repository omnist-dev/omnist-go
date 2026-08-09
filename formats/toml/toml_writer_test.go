package toml

import (
	"math"
	"math/big"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

func TestWriteTOMLGroupingAndCountOne(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("a", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B"))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
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
	d := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "[") {
		t.Errorf("a single-child group must not be rendered as a list: got %q", out)
	}
}

// --- worked example round-trip ---

func TestWriteTOMLRoundTripsWorkedExample(t *testing.T) {
	address := omnist.NewNode().AddValue("street", omnist.ScalarValue(omnist.NewStringScalar("1 Main"))).
		AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("London")))
	item1 := omnist.NewNode().AddValue("sku", omnist.ScalarValue(omnist.NewStringScalar("W"))).
		AddValue("qty", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(3)))).
		AddValue("price", omnist.ScalarValue(omnist.NewNumberScalar(9.99)))
	item2 := omnist.NewNode().AddValue("sku", omnist.ScalarValue(omnist.NewStringScalar("G"))).
		AddValue("qty", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))).
		AddValue("price", omnist.ScalarValue(omnist.NewNumberScalar(9.99)))
	order := omnist.NewNode().
		AddValue("id", omnist.ScalarValue(omnist.NewStringScalar("A1"))).
		AddValue("status", omnist.ScalarValue(omnist.NewStringScalar("shipped"))).
		AddValue("total", omnist.ScalarValue(omnist.NewNumberScalar(29.97))).
		AddNode("address", address).
		AddNode("items", item1).
		AddNode("items", item2)
	d := omnist.NodeDocument(omnist.NewNode().AddNode("order", order))

	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- native temporal round-trip: date/time/datetime all survive bare ---

func TestWriteTOMLTemporalRoundTripAllThreeKinds(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("d", omnist.ScalarValue(omnist.NewDateScalar(omnist.DateValue{Year: 2024, Month: 1, Day: 1}))).
		AddValue("t", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 12, Minute: 30, Second: 45}))).
		AddValue("dt", omnist.ScalarValue(omnist.NewDateTimeScalar(omnist.DateTimeValue{
			Date: omnist.DateValue{Year: 2024, Month: 1, Day: 1},
			Time: omnist.TimeValue{Hour: 12},
		}))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
	// Confirm none of the three kinds degraded to a string — unlike
	// YAML's writer, which has to stringify (quote) omnist.KindTime because
	// YAML's own core schema has no bare spelling for it. TOML does.
	for _, e := range back.Node.Edges {
		v, _ := e.Target.Value()
		if v.Scalar.Kind == omnist.KindString {
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
	d := omnist.NodeDocument(omnist.NewNode().AddValue("dt", omnist.ScalarValue(omnist.NewDateTimeScalar(omnist.DateTimeValue{
		Date: omnist.DateValue{Year: 2024, Month: 1, Day: 1},
		Time: omnist.TimeValue{Hour: 12, Minute: 0, Second: 0, HasOffset: true, OffsetSeconds: 5*3600 + 30*60},
	}))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- null: write-time adjustment, reported not invented ---

func TestWriteTOMLNullReportsAdjustment(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("coupon", omnist.NullValue()))
	_, err := Write(d)
	if err == nil {
		t.Fatal("expected an error: TOML has no null spelling")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want omnist.Diagnostic", err)
	}
	if diag.Code != omnist.CodeFormatNullUnrepresentable {
		t.Errorf("omnist.Diagnostic.Code = %s, want %s", diag.Code, omnist.CodeFormatNullUnrepresentable)
	}
	if diag.Path != "$.coupon" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.coupon", diag.Path)
	}
	if diag.Error() == "" {
		t.Error("omnist.Diagnostic.Error() returned empty string")
	}
}

func TestWriteTOMLNullInsideNestedNodeReportsAdjustment(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddNode("order", omnist.NewNode().AddValue("coupon", omnist.NullValue())))
	_, err := Write(d)
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want omnist.Diagnostic", err)
	}
	if diag.Path != "$.order.coupon" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.order.coupon", diag.Path)
	}
}

func TestWriteTOMLNullInsideListReportsAdjustment(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("m", omnist.NullValue()))
	_, err := Write(d)
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want omnist.Diagnostic", err)
	}
	if diag.Code != omnist.CodeFormatNullUnrepresentable {
		t.Errorf("omnist.Diagnostic.Code = %s, want %s", diag.Code, omnist.CodeFormatNullUnrepresentable)
	}
	if diag.Path != "$.m[1]" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.m[1]", diag.Path)
	}
}

// --- bare-scalar-root rejection ---

func TestWriteTOMLBareScalarRootFails(t *testing.T) {
	d := omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("x")))
	_, err := Write(d)
	if err == nil {
		t.Fatal("expected an error: TOML has no bare-scalar-root spelling")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("error is %T, want omnist.Diagnostic", err)
	}
	if diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("omnist.Diagnostic.Code = %s, want %s", diag.Code, omnist.CodeWriteUnsupportedValue)
	}
}

// --- integer/float distinction preserved ---

func TestWriteTOMLNumberAlwaysDistinctFromInteger(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("i", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(2)))).
		AddValue("f", omnist.ScalarValue(omnist.NewNumberScalar(2.0))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

func TestWriteTOMLNaNInfinityNativeSpellings(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("a", omnist.ScalarValue(omnist.NewNumberScalar(math.NaN()))).
		AddValue("b", omnist.ScalarValue(omnist.NewNumberScalar(math.Inf(1)))).
		AddValue("c", omnist.ScalarValue(omnist.NewNumberScalar(math.Inf(-1)))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch (NaN-aware): wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- omnist.KindTime with an (unusual) offset set: dropped, not malformed ---

func TestWriteTOMLBareTimeWithOffsetDropsOffset(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("t", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{
		Hour: 12, Minute: 0, Second: 0, HasOffset: true, OffsetSeconds: 3600,
	}))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	v, _ := back.Node.Edges[0].Target.Value()
	if v.Scalar.Kind != omnist.KindTime || v.Scalar.Time.HasOffset {
		t.Errorf("got %+v, want a bare local time with no offset", v)
	}
}

// --- string escaping ---

func TestWriteTOMLStringEscaping(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("s", omnist.ScalarValue(omnist.NewStringScalar("a\nb\tc\"d\\e\bf\rg\fh\x01i"))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- keys always quoted, including ones that would need it ---

func TestWriteTOMLQuotesEveryKey(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("has space", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- booleans ---

func TestWriteTOMLBooleans(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("t", omnist.ScalarValue(omnist.NewBooleanScalar(true))).
		AddValue("f", omnist.ScalarValue(omnist.NewBooleanScalar(false))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}

// --- empty node ---

func TestWriteTOMLEmptyNode(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode())
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("got %q, want empty output for an empty node", out)
	}
}

// --- nested node (inline table) ---

func TestWriteTOMLNestedNode(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddNode("a", omnist.NewNode().AddValue("b", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1))))))
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Read(out, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("round-trip parse failed on %q: %v", out, err)
	}
	if !docEqual(d, back) {
		t.Errorf("round-trip mismatch: wrote %q,\ngot  %+v,\nwant %+v", out, back, d)
	}
}
