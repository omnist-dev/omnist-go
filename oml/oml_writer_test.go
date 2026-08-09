package oml

import (
	"math"
	"math/big"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- round-trip property: the most valuable test category per the issue ---

func TestOMLRoundTripProperty(t *testing.T) {
	bigDigits := strings.Repeat("9", 50)
	bigInt := new(big.Int)
	bigInt.SetString(bigDigits, 10)

	cases := []struct {
		name string
		doc  omnist.Document
	}{
		{"empty node", omnist.NodeDocument(omnist.NewNode())},
		{"bare string scalar", omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar("hi")))},
		{"bare null", omnist.ValueDocument(omnist.NullValue())},
		{"bare integer", omnist.ValueDocument(omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(-42))))},
		{"bare boolean true", omnist.ValueDocument(omnist.ScalarValue(omnist.NewBooleanScalar(true)))},
		{
			"all seven scalar kinds",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("str", omnist.ScalarValue(omnist.NewStringScalar("hello"))).
				AddValue("int", omnist.ScalarValue(omnist.NewIntegerScalar(bigInt))).
				AddValue("num", omnist.ScalarValue(omnist.NewNumberScalar(3.5))).
				AddValue("boolT", omnist.ScalarValue(omnist.NewBooleanScalar(true))).
				AddValue("boolF", omnist.ScalarValue(omnist.NewBooleanScalar(false))).
				AddValue("date", omnist.ScalarValue(omnist.NewDateScalar(omnist.DateValue{Year: 2024, Month: 1, Day: 2}))).
				AddValue("time", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 10, Minute: 30}))).
				AddValue("datetime", omnist.ScalarValue(omnist.NewDateTimeScalar(omnist.DateTimeValue{
					Date: omnist.DateValue{Year: 2024, Month: 1, Day: 2},
					Time: omnist.TimeValue{Hour: 10, Minute: 30, Second: 5, Nanosecond: 123000000, HasOffset: true, OffsetSeconds: -3600},
				}))).
				AddValue("null", omnist.NullValue()),
			),
		},
		{
			"nested node two levels",
			omnist.NodeDocument(omnist.NewNode().AddNode("address", omnist.NewNode().
				AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("Zurich"))).
				AddNode("geo", omnist.NewNode().
					AddValue("lat", omnist.ScalarValue(omnist.NewNumberScalar(47.37))).
					AddValue("lon", omnist.ScalarValue(omnist.NewNumberScalar(8.55)))))),
		},
		{
			"repeated labels",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("x"))).
				AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("y"))).
				AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("z")))),
		},
		{
			"empty child node",
			omnist.NodeDocument(omnist.NewNode().AddNode("empty", omnist.NewNode())),
		},
		{
			"label needing quoting: hyphen, space, reserved words, nan/inf",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("has space", omnist.ScalarValue(omnist.NewStringScalar("v1"))).
				AddValue("null", omnist.ScalarValue(omnist.NewStringScalar("v2"))).
				AddValue("true", omnist.ScalarValue(omnist.NewStringScalar("v3"))).
				AddValue("false", omnist.ScalarValue(omnist.NewStringScalar("v4"))).
				AddValue("nan", omnist.ScalarValue(omnist.NewStringScalar("v5"))).
				AddValue("inf", omnist.ScalarValue(omnist.NewStringScalar("v6"))).
				AddValue("hyphen-ok", omnist.ScalarValue(omnist.NewStringScalar("v7")))),
		},
		{
			"string escaping: quote, backslash, control, unicode",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("s", omnist.ScalarValue(omnist.NewStringScalar("a\"b\\c\nd\re\tf\x01g héllo 世界")))),
		},
		{
			"number kinds: integer-valued float, exponent, nan, inf, -inf",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("whole", omnist.ScalarValue(omnist.NewNumberScalar(5))).
				AddValue("frac", omnist.ScalarValue(omnist.NewNumberScalar(3.25))).
				AddValue("big", omnist.ScalarValue(omnist.NewNumberScalar(1e30))).
				AddValue("nan", omnist.ScalarValue(omnist.NewNumberScalar(math.NaN()))).
				AddValue("posinf", omnist.ScalarValue(omnist.NewNumberScalar(math.Inf(1)))).
				AddValue("neginf", omnist.ScalarValue(omnist.NewNumberScalar(math.Inf(-1))))),
		},
		{
			"time without seconds vs with seconds vs with fraction vs with offset",
			omnist.NodeDocument(omnist.NewNode().
				AddValue("t1", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 1, Minute: 2}))).
				AddValue("t2", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 1, Minute: 2, Second: 3}))).
				AddValue("t3", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 1, Minute: 2, Second: 3, Nanosecond: 400000000}))).
				AddValue("t4", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: 0}))).
				AddValue("t5", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: -1800})))),
		},
	}

	for _, tc := range cases {
		for _, compact := range []bool{false, true} {
			t.Run(tc.name, func(t *testing.T) {
				text, _ := Write(tc.doc, compact)
				got, err := Read(text, omnist.DefaultLimits())
				if err != nil {
					t.Fatalf("compact=%v Read(Write(doc)) failed: %v\ntext:\n%s", compact, err, text)
				}
				if !docEqual(tc.doc, got) {
					t.Fatalf("compact=%v round-trip mismatch\ntext:\n%s\ngot:  %#v\nwant: %#v", compact, text, got, tc.doc)
				}
			})
		}
	}
}

func TestWriteOMLCompactWrapper(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("b"))))
	got, _ := WriteCompact(doc)
	want, _ := Write(doc, true)
	if got != want {
		t.Errorf("WriteCompact = %q, want %q", got, want)
	}
}

// --- §4.4 label quoting ---

func TestOMLLabelQuoting(t *testing.T) {
	cases := []struct {
		label string
		bare  bool
	}{
		{"foo", true},
		{"foo_bar-9", true},
		{"_leading", true},
		{"null", false},
		{"true", false},
		{"false", false},
		{"nan", false},
		{"inf", false},
		{"has space", false},
		{"-inf", false}, // doesn't match IDENT at all (no leading '-')
		{"", false},
		{"9start", false}, // IDENT requires a leading letter/underscore
	}
	for _, tc := range cases {
		doc := omnist.NodeDocument(omnist.NewNode().AddValue(tc.label, omnist.ScalarValue(omnist.NewStringScalar("v"))))
		text, _ := Write(doc, true)
		wantPrefix := tc.label + ":"
		if tc.bare {
			if !strings.HasPrefix(text, wantPrefix) {
				t.Errorf("label %q: want bare prefix %q, got %q", tc.label, wantPrefix, text)
			}
		} else {
			quotedPrefix := `"` + escapeOMLString(tc.label) + `":`
			if !strings.HasPrefix(text, quotedPrefix) {
				t.Errorf("label %q: want quoted prefix %q, got %q", tc.label, quotedPrefix, text)
			}
		}
		// Whatever the spelling, it must always read back correctly.
		reparsed, err := Read(text, omnist.DefaultLimits())
		if err != nil {
			t.Fatalf("label %q: reparse failed: %v (text: %s)", tc.label, err, text)
		}
		if !docEqual(doc, reparsed) {
			t.Errorf("label %q: round-trip mismatch", tc.label)
		}
	}
}

// --- §4.5 string escaping ---

func TestOMLStringEscapingSanctionedFormsOnly(t *testing.T) {
	// Every ASCII control character plus the printable escape-relevant
	// characters, so every branch of escapeOMLString is exercised.
	s := "\"\\\n\r\t\x00\x01\x1f/\bg\fh世"
	doc := omnist.ValueDocument(omnist.ScalarValue(omnist.NewStringScalar(s)))
	text, _ := Write(doc, true)

	for _, forbidden := range []string{`\/`, `\b`, `\f`} {
		if strings.Contains(text, forbidden) {
			t.Errorf("output contains forbidden escape %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{`\"`, `\\`, `\n`, `\r`, `\t`, `\u0000`, `\u0001`, `\u001f`} {
		if !strings.Contains(text, required) {
			t.Errorf("output missing expected escape %q: %s", required, text)
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

// --- §4.1 shape: compact vs pretty layout, worked example ---

func TestOMLPrettyLayoutMatchesSpecExample(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().
		AddValue("name", omnist.ScalarValue(omnist.NewStringScalar("Ann"))).
		AddNode("address", omnist.NewNode().
			AddValue("city", omnist.ScalarValue(omnist.NewStringScalar("Zurich"))).
			AddValue("postcode", omnist.ScalarValue(omnist.NewStringScalar("8001")))).
		AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("x"))).
		AddValue("tag", omnist.ScalarValue(omnist.NewStringScalar("y"))))

	want := "name: \"Ann\"\naddress: {\n  city: \"Zurich\"\n  postcode: \"8001\"\n}\ntag: \"x\"\ntag: \"y\"\n"
	if got, _ := Write(doc, false); got != want {
		t.Errorf("pretty form mismatch:\ngot:  %q\nwant: %q", got, want)
	}

	wantCompact := `name: "Ann"; address: { city: "Zurich"; postcode: "8001" }; tag: "x"; tag: "y"`
	if got, _ := Write(doc, true); got != wantCompact {
		t.Errorf("compact form mismatch:\ngot:  %q\nwant: %q", got, wantCompact)
	}
}

// --- issue #33: the canonical writer always emits seconds for a time
// value, even when they are zero, since omnist.TimeValue has no way to record
// "seconds were explicitly ':00' in the source" separately from "seconds
// defaulted to zero" ---

func TestOMLTimeAlwaysEmitsSeconds(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().AddValue("t", omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 12, Minute: 0}))))
	want := "t: 12:00:00\n"
	if got, _ := Write(doc, false); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- issue #33: the pretty-printed writer ends with a trailing newline ---

func TestOMLPrettyOutputEndsWithNewline(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(1)))))
	got, _ := Write(doc, false)
	if len(got) == 0 || got[len(got)-1] != '\n' {
		t.Errorf("got %q, want a trailing newline", got)
	}
}

func TestOMLEmptyDocumentRoundTrips(t *testing.T) {
	doc := omnist.NodeDocument(omnist.NewNode())
	if got, _ := Write(doc, false); got != "" {
		t.Errorf("empty document should render as empty string, got %q", got)
	}
	reparsed, err := Read("", omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("Read(\"\") failed: %v", err)
	}
	if !docEqual(doc, reparsed) {
		t.Errorf("empty round-trip mismatch")
	}
}
