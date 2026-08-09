package omnist

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

// --- round-trip property: the most valuable test category per the issue ---

func TestOMLRoundTripProperty(t *testing.T) {
	bigDigits := strings.Repeat("9", 50)
	bigInt := new(big.Int)
	bigInt.SetString(bigDigits, 10)

	cases := []struct {
		name string
		doc  Document
	}{
		{"empty node", NodeDocument(NewNode())},
		{"bare string scalar", ValueDocument(ScalarValue(NewStringScalar("hi")))},
		{"bare null", ValueDocument(NullValue())},
		{"bare integer", ValueDocument(ScalarValue(NewIntegerScalar(big.NewInt(-42))))},
		{"bare boolean true", ValueDocument(ScalarValue(NewBooleanScalar(true)))},
		{
			"all seven scalar kinds",
			NodeDocument(NewNode().
				AddValue("str", ScalarValue(NewStringScalar("hello"))).
				AddValue("int", ScalarValue(NewIntegerScalar(bigInt))).
				AddValue("num", ScalarValue(NewNumberScalar(3.5))).
				AddValue("boolT", ScalarValue(NewBooleanScalar(true))).
				AddValue("boolF", ScalarValue(NewBooleanScalar(false))).
				AddValue("date", ScalarValue(NewDateScalar(DateValue{Year: 2024, Month: 1, Day: 2}))).
				AddValue("time", ScalarValue(NewTimeScalar(TimeValue{Hour: 10, Minute: 30}))).
				AddValue("datetime", ScalarValue(NewDateTimeScalar(DateTimeValue{
					Date: DateValue{Year: 2024, Month: 1, Day: 2},
					Time: TimeValue{Hour: 10, Minute: 30, Second: 5, Nanosecond: 123000000, HasOffset: true, OffsetSeconds: -3600},
				}))).
				AddValue("null", NullValue()),
			),
		},
		{
			"nested node two levels",
			NodeDocument(NewNode().AddNode("address", NewNode().
				AddValue("city", ScalarValue(NewStringScalar("Zurich"))).
				AddNode("geo", NewNode().
					AddValue("lat", ScalarValue(NewNumberScalar(47.37))).
					AddValue("lon", ScalarValue(NewNumberScalar(8.55)))))),
		},
		{
			"repeated labels",
			NodeDocument(NewNode().
				AddValue("tag", ScalarValue(NewStringScalar("x"))).
				AddValue("tag", ScalarValue(NewStringScalar("y"))).
				AddValue("tag", ScalarValue(NewStringScalar("z")))),
		},
		{
			"empty child node",
			NodeDocument(NewNode().AddNode("empty", NewNode())),
		},
		{
			"label needing quoting: hyphen, space, reserved words, nan/inf",
			NodeDocument(NewNode().
				AddValue("has space", ScalarValue(NewStringScalar("v1"))).
				AddValue("null", ScalarValue(NewStringScalar("v2"))).
				AddValue("true", ScalarValue(NewStringScalar("v3"))).
				AddValue("false", ScalarValue(NewStringScalar("v4"))).
				AddValue("nan", ScalarValue(NewStringScalar("v5"))).
				AddValue("inf", ScalarValue(NewStringScalar("v6"))).
				AddValue("hyphen-ok", ScalarValue(NewStringScalar("v7")))),
		},
		{
			"string escaping: quote, backslash, control, unicode",
			NodeDocument(NewNode().
				AddValue("s", ScalarValue(NewStringScalar("a\"b\\c\nd\re\tf\x01g héllo 世界")))),
		},
		{
			"number kinds: integer-valued float, exponent, nan, inf, -inf",
			NodeDocument(NewNode().
				AddValue("whole", ScalarValue(NewNumberScalar(5))).
				AddValue("frac", ScalarValue(NewNumberScalar(3.25))).
				AddValue("big", ScalarValue(NewNumberScalar(1e30))).
				AddValue("nan", ScalarValue(NewNumberScalar(math.NaN()))).
				AddValue("posinf", ScalarValue(NewNumberScalar(math.Inf(1)))).
				AddValue("neginf", ScalarValue(NewNumberScalar(math.Inf(-1))))),
		},
		{
			"time without seconds vs with seconds vs with fraction vs with offset",
			NodeDocument(NewNode().
				AddValue("t1", ScalarValue(NewTimeScalar(TimeValue{Hour: 1, Minute: 2}))).
				AddValue("t2", ScalarValue(NewTimeScalar(TimeValue{Hour: 1, Minute: 2, Second: 3}))).
				AddValue("t3", ScalarValue(NewTimeScalar(TimeValue{Hour: 1, Minute: 2, Second: 3, Nanosecond: 400000000}))).
				AddValue("t4", ScalarValue(NewTimeScalar(TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: 0}))).
				AddValue("t5", ScalarValue(NewTimeScalar(TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: -1800})))),
		},
	}

	for _, tc := range cases {
		for _, compact := range []bool{false, true} {
			t.Run(tc.name, func(t *testing.T) {
				text := WriteOML(tc.doc, compact)
				got, err := ReadOML(text, DefaultLimits())
				if err != nil {
					t.Fatalf("compact=%v ReadOML(WriteOML(doc)) failed: %v\ntext:\n%s", compact, err, text)
				}
				if !docEqual(tc.doc, got) {
					t.Fatalf("compact=%v round-trip mismatch\ntext:\n%s\ngot:  %#v\nwant: %#v", compact, text, got, tc.doc)
				}
			})
		}
	}
}

func TestWriteOMLCompactWrapper(t *testing.T) {
	doc := NodeDocument(NewNode().AddValue("a", ScalarValue(NewStringScalar("b"))))
	if got, want := WriteOMLCompact(doc), WriteOML(doc, true); got != want {
		t.Errorf("WriteOMLCompact = %q, want %q", got, want)
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
		doc := NodeDocument(NewNode().AddValue(tc.label, ScalarValue(NewStringScalar("v"))))
		text := WriteOML(doc, true)
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
		reparsed, err := ReadOML(text, DefaultLimits())
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
	doc := ValueDocument(ScalarValue(NewStringScalar(s)))
	text := WriteOML(doc, true)

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

	got, err := ReadOML(text, DefaultLimits())
	if err != nil {
		t.Fatalf("reparse failed: %v (text: %s)", err, text)
	}
	if !docEqual(doc, got) {
		t.Errorf("round-trip mismatch: got %#v want %#v", got, doc)
	}
}

// --- §4.1 shape: compact vs pretty layout, worked example ---

func TestOMLPrettyLayoutMatchesSpecExample(t *testing.T) {
	doc := NodeDocument(NewNode().
		AddValue("name", ScalarValue(NewStringScalar("Ann"))).
		AddNode("address", NewNode().
			AddValue("city", ScalarValue(NewStringScalar("Zurich"))).
			AddValue("postcode", ScalarValue(NewStringScalar("8001")))).
		AddValue("tag", ScalarValue(NewStringScalar("x"))).
		AddValue("tag", ScalarValue(NewStringScalar("y"))))

	want := "name: \"Ann\"\naddress: {\n  city: \"Zurich\"\n  postcode: \"8001\"\n}\ntag: \"x\"\ntag: \"y\"\n"
	if got := WriteOML(doc, false); got != want {
		t.Errorf("pretty form mismatch:\ngot:  %q\nwant: %q", got, want)
	}

	wantCompact := `name: "Ann"; address: { city: "Zurich"; postcode: "8001" }; tag: "x"; tag: "y"`
	if got := WriteOML(doc, true); got != wantCompact {
		t.Errorf("compact form mismatch:\ngot:  %q\nwant: %q", got, wantCompact)
	}
}

// --- issue #33: the canonical writer always emits seconds for a time
// value, even when they are zero, since TimeValue has no way to record
// "seconds were explicitly ':00' in the source" separately from "seconds
// defaulted to zero" ---

func TestOMLTimeAlwaysEmitsSeconds(t *testing.T) {
	doc := NodeDocument(NewNode().AddValue("t", ScalarValue(NewTimeScalar(TimeValue{Hour: 12, Minute: 0}))))
	want := "t: 12:00:00\n"
	if got := WriteOML(doc, false); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- issue #33: the pretty-printed writer ends with a trailing newline ---

func TestOMLPrettyOutputEndsWithNewline(t *testing.T) {
	doc := NodeDocument(NewNode().AddValue("a", ScalarValue(NewIntegerScalar(big.NewInt(1)))))
	got := WriteOML(doc, false)
	if len(got) == 0 || got[len(got)-1] != '\n' {
		t.Errorf("got %q, want a trailing newline", got)
	}
}

func TestOMLEmptyDocumentRoundTrips(t *testing.T) {
	doc := NodeDocument(NewNode())
	if got := WriteOML(doc, false); got != "" {
		t.Errorf("empty document should render as empty string, got %q", got)
	}
	reparsed, err := ReadOML("", DefaultLimits())
	if err != nil {
		t.Fatalf("ReadOML(\"\") failed: %v", err)
	}
	if !docEqual(doc, reparsed) {
		t.Errorf("empty round-trip mismatch")
	}
}
