package omnist

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

// --- §7.3 grouping + count-1 rule ---

func TestWriteJSONGroupingAndCountOne(t *testing.T) {
	doc := NodeDocument(NewNode().
		AddValue("m", ScalarValue(NewStringScalar("A"))).
		AddValue("x", ScalarValue(NewStringScalar("X"))).
		AddValue("m", ScalarValue(NewStringScalar("B"))))
	got, err := WriteJSON(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	// Cross-label interleaving is lost: m's two edges group together
	// regardless of x sitting between them in the edge list.
	want := `{"m": ["A", "B"], "x": "X"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteJSONSingleLabelIsBareValueNotList(t *testing.T) {
	doc := NodeDocument(NewNode().AddValue("tag", ScalarValue(NewStringScalar("x"))))
	got, err := WriteJSON(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if want := `{"tag": "x"}`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- temporal leaves stringified to ISO-8601 ---

func TestWriteJSONTemporalStringified(t *testing.T) {
	doc := NodeDocument(NewNode().
		AddValue("d", ScalarValue(NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 2}))).
		AddValue("t", ScalarValue(NewTimeScalar(TimeValue{Hour: 10, Minute: 30}))).
		AddValue("dt", ScalarValue(NewDateTimeScalar(DateTimeValue{
			Date: DateValue{Year: 2024, Month: 1, Day: 2},
			Time: TimeValue{Hour: 10, Minute: 30, Second: 5, Nanosecond: 250000000},
		}))))
	got, err := WriteJSON(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	want := `{"d": "2024-01-02", "t": "10:30", "dt": "2024-01-02T10:30:05.25"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A writer MUST stringify; the result is plain KindString on read back
	// (JSON has no native temporal types), which is exactly what the "no
	// temporal auto-detection" rule in json_reader_test.go asserts too.
	reread, err := ReadJSON(got, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	for _, label := range []string{"d", "t", "dt"} {
		for _, e := range reread.Node.Edges {
			if e.Label == label {
				v, _ := e.Target.Value()
				if v.Scalar.Kind != KindString {
					t.Errorf("label %q read back as %v, want KindString", label, v.Scalar.Kind)
				}
			}
		}
	}
}

func TestWriteJSONTimeVariants(t *testing.T) {
	cases := []struct {
		name string
		tv   TimeValue
		want string
	}{
		{"no seconds", TimeValue{Hour: 1, Minute: 2}, "01:02"},
		{"with seconds", TimeValue{Hour: 1, Minute: 2, Second: 3}, "01:02:03"},
		{"with fraction", TimeValue{Hour: 1, Minute: 2, Second: 3, Nanosecond: 400000000}, "01:02:03.4"},
		{"with positive offset", TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: 3600}, "01:02+01:00"},
		{"with negative offset", TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: -1800}, "01:02-00:30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := ValueDocument(ScalarValue(NewTimeScalar(tc.tv)))
			got, err := WriteJSON(doc)
			if err != nil {
				t.Fatalf("WriteJSON failed: %v", err)
			}
			want := `"` + tc.want + `"`
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// --- NaN/Infinity: default lenient substitution ---

func TestWriteJSONNaNInfinitySubstitutesNull(t *testing.T) {
	cases := []struct {
		name string
		num  float64
	}{
		{"nan", math.NaN()},
		{"+inf", math.Inf(1)},
		{"-inf", math.Inf(-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := NodeDocument(NewNode().
				AddValue("before", ScalarValue(NewStringScalar("kept"))).
				AddValue("n", ScalarValue(NewNumberScalar(tc.num))).
				AddValue("after", ScalarValue(NewIntegerScalar(big.NewInt(7)))))
			got, err := WriteJSON(doc)
			if err != nil {
				t.Fatalf("WriteJSON failed: %v", err)
			}
			want := `{"before": "kept", "n": null, "after": 7}`
			if got != want {
				t.Errorf("got %q, want %q (rest of the document must be otherwise unaffected)", got, want)
			}
		})
	}
}

// --- NaN/Infinity: optional strict mode ---

func TestWriteJSONStrictFailsOnNaNInfinity(t *testing.T) {
	cases := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, num := range cases {
		doc := NodeDocument(NewNode().AddValue("n", ScalarValue(NewNumberScalar(num))))
		_, err := WriteJSONStrict(doc)
		if err == nil {
			t.Fatalf("WriteJSONStrict(%v) succeeded, want an error", num)
		}
		diag, ok := err.(Diagnostic)
		if !ok {
			t.Fatalf("error is %T, want Diagnostic", err)
		}
		if diag.Code != CodeWriteUnsupportedValue {
			t.Errorf("Diagnostic.Code = %s, want %s", diag.Code, CodeWriteUnsupportedValue)
		}
		if diag.Path != "$.n" {
			t.Errorf("Diagnostic.Path = %s, want $.n", diag.Path)
		}
		if diag.Error() == "" {
			t.Error("Diagnostic.Error() returned empty string")
		}
	}
}

func TestWriteJSONStrictFailsOnNaNInfinityInsideList(t *testing.T) {
	doc := NodeDocument(NewNode().
		AddValue("n", ScalarValue(NewNumberScalar(1))).
		AddValue("n", ScalarValue(NewNumberScalar(math.NaN()))))
	_, err := WriteJSONStrict(doc)
	if err == nil {
		t.Fatal("expected an error")
	}
	diag, ok := err.(Diagnostic)
	if !ok || diag.Code != CodeWriteUnsupportedValue {
		t.Errorf("error = %#v, want write.unsupported-value Diagnostic", err)
	}
	if diag.Path != "$.n[1]" {
		t.Errorf("Diagnostic.Path = %s, want $.n[1]", diag.Path)
	}
}

func TestWriteJSONStrictSucceedsOnFiniteValues(t *testing.T) {
	doc := NodeDocument(NewNode().AddValue("n", ScalarValue(NewNumberScalar(3.5))))
	got, err := WriteJSONStrict(doc)
	if err != nil {
		t.Fatalf("WriteJSONStrict failed: %v", err)
	}
	if want := `{"n": 3.5}`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteJSONStrictFailsOnBareValueDocument(t *testing.T) {
	_, err := WriteJSONStrict(ValueDocument(ScalarValue(NewNumberScalar(math.NaN()))))
	if err == nil {
		t.Fatal("expected an error")
	}
	diag, ok := err.(Diagnostic)
	if !ok || diag.Code != CodeWriteUnsupportedValue {
		t.Errorf("error = %#v, want write.unsupported-value Diagnostic", err)
	}
}

// --- number formatting preserves the integer/number distinction on write ---

func TestWriteJSONNumberAlwaysDistinctFromInteger(t *testing.T) {
	doc := ValueDocument(ScalarValue(NewNumberScalar(5)))
	got, err := WriteJSON(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if got == "5" {
		t.Fatalf("WriteJSON(number 5) = %q, which would re-read as an integer, losing the number kind", got)
	}
	reread, err := ReadJSON(got, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if reread.Value.Scalar.Kind != KindNumber {
		t.Errorf("round-tripped kind = %v, want KindNumber", reread.Value.Scalar.Kind)
	}
}

// --- string escaping ---

func TestWriteJSONStringEscaping(t *testing.T) {
	s := "\"\\\n\r\t\x00\x01\x1f\bg\fh世"
	doc := ValueDocument(ScalarValue(NewStringScalar(s)))
	text, err := WriteJSON(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	required := []string{"\\\"", "\\\\", "\\n", "\\r", "\\t", "\\u0000", "\\u0001", "\\u001f", "\\b", "\\f"}
	for _, esc := range required {
		if !strings.Contains(text, esc) {
			t.Errorf("output missing expected escape %q: %s", esc, text)
		}
	}
	if !strings.Contains(text, "世") {
		t.Errorf("non-ASCII character was escaped, want literal: %s", text)
	}
	got, err := ReadJSON(text, DefaultLimits())
	if err != nil {
		t.Fatalf("reparse failed: %v (text: %s)", err, text)
	}
	if !docEqual(doc, got) {
		t.Errorf("round-trip mismatch: got %#v want %#v", got, doc)
	}
}

// --- bare-value document writing ---

func TestWriteJSONBareValueDocument(t *testing.T) {
	cases := []struct {
		name string
		doc  Document
		want string
	}{
		{"null", ValueDocument(NullValue()), "null"},
		{"string", ValueDocument(ScalarValue(NewStringScalar("hi"))), `"hi"`},
		{"integer", ValueDocument(ScalarValue(NewIntegerScalar(big.NewInt(-7)))), "-7"},
		{"boolean", ValueDocument(ScalarValue(NewBooleanScalar(true))), "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := WriteJSON(tc.doc)
			if err != nil {
				t.Fatalf("WriteJSON failed: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWriteJSONEmptyNode(t *testing.T) {
	got, err := WriteJSON(NodeDocument(NewNode()))
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if want := "{}"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteJSONNestedNode(t *testing.T) {
	doc := NodeDocument(NewNode().AddNode("a", NewNode().AddValue("b", ScalarValue(NewIntegerScalar(big.NewInt(1))))))
	got, err := WriteJSON(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if want := `{"a": {"b": 1}}`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
