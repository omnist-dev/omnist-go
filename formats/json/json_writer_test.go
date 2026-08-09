package json

import (
	"math"
	"math/big"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- §7.3 grouping + count-1 rule ---

func TestWriteJSONGroupingAndCountOne(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("A"))).
		AddValue("x", omnist.ScalarValue(omnist.NewStringScalar("X"))).
		AddValue("m", omnist.ScalarValue(omnist.NewStringScalar("B"))))
	got, err := Write(doc)
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
	doc := omnist.NodeDocument(omnist.NewNode().AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("x"))))
	got, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if want := `{"tag": "x"}`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- temporal leaves stringified to ISO-8601 ---

func TestWriteJSONTemporalStringified(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().
		AddValue("d", omnist.ScalarValue(omnist.NewDateScalar(omnist.DateValue{Year: 2024, Month: 1, Day: 2}))).
		AddValue("t", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 10, Minute: 30}))).
		AddValue("dt", omnist.ScalarValue(omnist.NewDateTimeScalar(omnist.DateTimeValue{
			Date: omnist.DateValue{Year: 2024, Month: 1, Day: 2},
			Time: omnist.TimeValue{Hour: 10, Minute: 30, Second: 5, Nanosecond: 250000000},
		}))))
	got, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	want := `{"d": "2024-01-02", "t": "10:30", "dt": "2024-01-02T10:30:05.25"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A writer MUST stringify; the result is plain omnist.KindString on read back
	// (JSON has no native temporal types), which is exactly what the "no
	// temporal auto-detection" rule in json_reader_test.go asserts too.
	reread, err := Read(got, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	for _, label := range []string{"d", "t", "dt"} {
		for _, e := range reread.Node.Edges {
			if e.Label == label {
				v, _ := e.Target.Value()
				if v.Scalar.Kind != omnist.KindString {
					t.Errorf("label %q read back as %v, want omnist.KindString", label, v.Scalar.Kind)
				}
			}
		}
	}
}

func TestWriteJSONTimeVariants(t *testing.T) {
	cases := []struct {
		name string
		tv   omnist.TimeValue
		want string
	}{
		{"no seconds", omnist.TimeValue{Hour: 1, Minute: 2}, "01:02"},
		{"with seconds", omnist.TimeValue{Hour: 1, Minute: 2, Second: 3}, "01:02:03"},
		{"with fraction", omnist.TimeValue{Hour: 1, Minute: 2, Second: 3, Nanosecond: 400000000}, "01:02:03.4"},
		{"with positive offset", omnist.TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: 3600}, "01:02+01:00"},
		{"with negative offset", omnist.TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: -1800}, "01:02-00:30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := omnist.ValueDocument(omnist.ScalarValue(omnist.NewTimeScalar(tc.tv)))
			got, err := Write(doc)
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
			doc := omnist.NodeDocument(omnist.NewNode().
				AddValue("before", omnist.ScalarValue(omnist.NewStringScalar("kept"))).
				AddValue("n", omnist.ScalarValue(omnist.NewNumberScalar(tc.num))).
				AddValue("after", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(7)))))
			got, err := Write(doc)
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
		doc := omnist.NodeDocument(omnist.NewNode().AddValue("n", omnist.ScalarValue(omnist.NewNumberScalar(num))))
		_, err := WriteStrict(doc)
		if err == nil {
			t.Fatalf("WriteStrict(%v) succeeded, want an error", num)
		}
		diag, ok := err.(omnist.Diagnostic)
		if !ok {
			t.Fatalf("error is %T, want omnist.Diagnostic", err)
		}
		if diag.Code != omnist.CodeWriteUnsupportedValue {
			t.Errorf("omnist.Diagnostic.Code = %s, want %s", diag.Code, omnist.CodeWriteUnsupportedValue)
		}
		if diag.Path != "$.n" {
			t.Errorf("omnist.Diagnostic.Path = %s, want $.n", diag.Path)
		}
		if diag.Error() == "" {
			t.Error("omnist.Diagnostic.Error() returned empty string")
		}
	}
}

func TestWriteJSONStrictFailsOnNaNInfinityInsideList(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().
		AddValue("n", omnist.ScalarValue(omnist.NewNumberScalar(1))).
		AddValue("n", omnist.ScalarValue(omnist.NewNumberScalar(math.NaN()))))
	_, err := WriteStrict(doc)
	if err == nil {
		t.Fatal("expected an error")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("error = %#v, want write.unsupported-value omnist.Diagnostic", err)
	}
	if diag.Path != "$.n[1]" {
		t.Errorf("omnist.Diagnostic.Path = %s, want $.n[1]", diag.Path)
	}
}

func TestWriteJSONStrictSucceedsOnFiniteValues(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().AddValue("n", omnist.ScalarValue(omnist.NewNumberScalar(3.5))))
	got, err := WriteStrict(doc)
	if err != nil {
		t.Fatalf("WriteJSONStrict failed: %v", err)
	}
	if want := `{"n": 3.5}`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteJSONStrictFailsOnBareValueDocument(t *testing.T) {
	_, err := WriteStrict(omnist.ValueDocument(omnist.ScalarValue(omnist.NewNumberScalar(math.NaN()))))
	if err == nil {
		t.Fatal("expected an error")
	}
	diag, ok := err.(omnist.Diagnostic)
	if !ok || diag.Code != omnist.CodeWriteUnsupportedValue {
		t.Errorf("error = %#v, want write.unsupported-value omnist.Diagnostic", err)
	}
}

// --- number formatting preserves the integer/number distinction on write ---

func TestWriteJSONNumberAlwaysDistinctFromInteger(t *testing.T) {
	doc := omnist.ValueDocument(omnist.ScalarValue(omnist.NewNumberScalar(5)))
	got, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if got == "5" {
		t.Fatalf("Write(number 5) = %q, which would re-read as an integer, losing the number kind", got)
	}
	reread, err := Read(got, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if reread.Value.Scalar.Kind != omnist.KindNumber {
		t.Errorf("round-tripped kind = %v, want omnist.KindNumber", reread.Value.Scalar.Kind)
	}
}

// --- string escaping ---

func TestWriteJSONStringEscaping(t *testing.T) {
	s := "\"\\\n\r\t\x00\x01\x1f\bg\fh世"
	doc := omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar(s)))
	text, err := Write(doc)
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
	got, err := Read(text, omnist.DefaultLimits())
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
		doc  omnist.Document
		want string
	}{
		{"null", omnist.ValueDocument(omnist.NullValue()), "null"},
		{"string", omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("hi"))), `"hi"`},
		{"integer", omnist.ValueDocument(omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(-7)))), "-7"},
		{"boolean", omnist.ValueDocument(omnist.ScalarValue(omnist.NewBooleanScalar(true))), "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Write(tc.doc)
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
	got, err := Write(omnist.NodeDocument(omnist.NewNode()))
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if want := "{}"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteJSONNestedNode(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().AddNode("a", omnist.NewNode().AddValue("b", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1))))))
	got, err := Write(doc)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if want := `{"a": {"b": 1}}`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
