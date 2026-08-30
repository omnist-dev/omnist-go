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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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

	out, _, err := Write(d)
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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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

// --- null: unconditional write failure, not a reported adjustment ---

// Per spec section 8.3.8/8.3.9 (updated 2026-08-24, issue #97): a null
// leaf with no TOML spelling now fails the write unconditionally
// (omnist.CodeWriteUnsupportedValue) instead of being silently dropped
// with a warning -- dropping the edge erased its existence entirely,
// with zero trace on read-back.
func TestWriteTOMLNullFailsUnconditionally(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddValue("coupon", omnist.NullValue()))
	out, diags, err := Write(d)
	if err == nil {
		t.Fatalf("Write: want error, got ok write out=%q diags=%v", out, diags)
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("err = %T, want omnist.Diagnostic", err)
	}
	if diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("omnist.Diagnostic.Code = %s, want %s", diag.Code, omnist.CodeWriteUnsupportedValue)
	}
	if diag.Path != "$.coupon" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.coupon", diag.Path)
	}
	if diag.Error() == "" {
		t.Error("omnist.Diagnostic.Error() returned empty string")
	}
}

func TestWriteTOMLNullInsideNestedNodeFailsUnconditionally(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().AddNode("order", omnist.NewNode().AddValue("coupon", omnist.NullValue())))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("Write: want error for a null leaf nested inside a table, got ok write")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("err = %T, want omnist.Diagnostic", err)
	}
	if diag.Path != "$.order.coupon" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.order.coupon", diag.Path)
	}
}

func TestWriteTOMLNullInsideListFailsUnconditionally(t *testing.T) {
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("m", omnist.NullValue()))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("Write: want error for a null leaf inside a repeated-label array, got ok write")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok {
		t.Fatalf("err = %T, want omnist.Diagnostic", err)
	}
	if diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("omnist.Diagnostic.Code = %s, want %s", diag.Code, omnist.CodeWriteUnsupportedValue)
	}
	if diag.Path != "$.m[1]" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.m[1]", diag.Path)
	}
}

func TestWriteTOMLNullFailsAfterEarlierTopLevelKeySucceeded(t *testing.T) {
	// Exercises writeTOMLTopLevel's error return on a later group after
	// an earlier one already wrote successfully -- not just the
	// single-group case.
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("ok"))).
		AddValue("b", omnist.NullValue()))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("Write: want error, got ok write")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("error = %#v, want write.unsupported-value omnist.Diagnostic", err)
	}
	if diag.Path != "$.b" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.b", diag.Path)
	}
}

func TestWriteTOMLNullFailsAfterEarlierInlineTableKeySucceeded(t *testing.T) {
	// Exercises writeTOMLInlineTable's error return on a later group
	// after an earlier one already wrote successfully.
	d := omnist.NodeDocument(omnist.NewNode().AddNode("order", omnist.NewNode().
		AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("ok"))).
		AddValue("b", omnist.NullValue())))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("Write: want error, got ok write")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("error = %#v, want write.unsupported-value omnist.Diagnostic", err)
	}
	if diag.Path != "$.order.b" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.order.b", diag.Path)
	}
}

func TestWriteTOMLNullInsideListFailsAfterEarlierElementSucceeded(t *testing.T) {
	// Exercises writeTOMLGroupValue's multi-child array branch: the
	// error return after an earlier element already wrote successfully
	// AND the eventual bool return path when nothing errors.
	d := omnist.NodeDocument(omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B"))).
		AddValue("m", omnist.NullValue()))
	_, _, err := Write(d)
	if err == nil {
		t.Fatal("Write: want error, got ok write")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("error = %#v, want write.unsupported-value omnist.Diagnostic", err)
	}
	if diag.Path != "$.m[2]" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.m[2]", diag.Path)
	}
}

// --- bare-scalar-root rejection ---

func TestWriteTOMLBareScalarRootFails(t *testing.T) {
	d := omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("x")))
	_, _, err := Write(d)
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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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
	out, _, err := Write(d)
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

// --- cross-label interleaving loss (D-3, spec §8.3.8) ---

func TestWriteTOMLInterleavingLostReportsDiagnostic(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("x", omnist.ScalarValue(omnist.NewStringScalar("X"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B"))))
	_, diags, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteTOML failed: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("diags = %+v, want exactly 1", diags)
	}
	if diags[0].Path != "$" || diags[0].Code != omnist.CodeFormatInterleavingLost || diags[0].Severity != omnist.SeverityWarning {
		t.Errorf("got diagnostic %+v, want {Path: $, Code: %s, Severity: warning}", diags[0], omnist.CodeFormatInterleavingLost)
	}
}

func TestWriteTOMLContiguousRepeatNoInterleavingDiagnostic(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B"))).
		AddValue("x", omnist.ScalarValue(omnist.NewStringScalar("X"))))
	_, diags, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteTOML failed: %v", err)
	}
	if len(diags) != 0 {
		t.Errorf("diags = %+v, want none (contiguous repeat loses nothing)", diags)
	}
}

func TestWriteTOMLInterleavingLostAtNestedPath(t *testing.T) {
	// Exercises writeTOMLInlineTable's own check, not just
	// writeTOMLTopLevel's -- the loss is reported at the nested inline
	// table's own path, not "$".
	inner := omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("x", omnist.ScalarValue(omnist.NewStringScalar("X"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B")))
	doc := omnist.NodeDocument(omnist.NewNode().AddNode("outer", inner))
	_, diags, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteTOML failed: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("diags = %+v, want exactly 1", diags)
	}
	if diags[0].Path != "$.outer" || diags[0].Code != omnist.CodeFormatInterleavingLost {
		t.Errorf("got diagnostic %+v, want {Path: $.outer, Code: %s}", diags[0], omnist.CodeFormatInterleavingLost)
	}
}
