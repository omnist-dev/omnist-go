package osd

// OSD-14 (a writer refuses a schema OSD text cannot represent) and OSD-15
// (canonical escaping), spec §5.9. No conformance vector can pin OSD-14
// (DIV-5: a vector gives a schema as OSD text, and such a schema has none), so
// these are the only tests that hold it.

import (
	"math/rand"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

func oneFieldSchema(record, label string) omnist.Schema {
	return omnist.Schema{
		Root: record,
		Env: map[string]*omnist.Record{
			record: {Name: record, Fields: []omnist.Field{
				{Label: label, Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
			}},
		},
		EnvOrder: []string{record},
	}
}

// Every C0 code point, alone and embedded, in both layouts.
func TestWriteRefusesAnyC0ControlInAFieldLabel(t *testing.T) {
	for c := 0; c < 0x20; c++ {
		for _, label := range []string{string(rune(c)), "a" + string(rune(c)) + "b", "ok" + string(rune(c))} {
			for _, compact := range []bool{false, true} {
				text, err := Write(oneFieldSchema("R", label), compact)
				d, ok := err.(omnist.Diagnostic)
				if !ok {
					t.Fatalf("U+%04X label %q compact=%v: err = %#v, want an omnist.Diagnostic", c, label, compact, err)
				}
				if d.Code != omnist.CodeWriteUnsupportedValue || d.Path != "R" || d.Severity != omnist.SeverityError {
					t.Errorf("U+%04X: got %+v, want write.unsupported-value at path R, severity error", c, d)
				}
				if text != "" {
					t.Errorf("U+%04X: a refused write must return no text, got %q", c, text)
				}
			}
		}
	}
}

// The path is the RECORD, never R.<label>, and it names the first offending
// record in declaration order even when a later record is also bad.
func TestWriteRefusalPathIsTheRecordHoldingTheField(t *testing.T) {
	bad := func(name string) *omnist.Record {
		return &omnist.Record{Name: name, Fields: []omnist.Field{
			{Label: "fine", Type: omnist.AnyType(), Cardinality: omnist.DefaultCardinality()},
			{Label: "tab\there", Type: omnist.AnyType(), Cardinality: omnist.DefaultCardinality()},
		}}
	}
	s := omnist.Schema{
		Root: "Root",
		Env: map[string]*omnist.Record{
			"Root":  {Name: "Root", Fields: []omnist.Field{{Label: "ok", Type: omnist.RefType("Inner"), Cardinality: omnist.DefaultCardinality()}}},
			"Inner": bad("Inner"),
			"Later": bad("Later"),
		},
		EnvOrder: []string{"Root", "Inner", "Later"},
	}
	_, err := Write(s, false)
	d, ok := err.(omnist.Diagnostic)
	if !ok || d.Path != "Inner" || d.Code != omnist.CodeWriteUnsupportedValue {
		t.Fatalf("got %#v, want write.unsupported-value at Inner", err)
	}
	if strings.Contains(d.Path, "tab") || strings.Contains(d.Error(), "\t") {
		t.Errorf("the offending label must not appear in the path or message: %v", d)
	}
}

// Not C0, so not refused: DEL, C1 controls, the interior BOM, non-ASCII, and
// ordinary punctuation all have a spelling and must round-trip.
func TestWriteAcceptsEverythingThatIsNotC0(t *testing.T) {
	for _, label := range []string{"\x7f", "\u0085", "a\uFEFFb", "é€😀", "sp ace", "a,b:c{d}", " ", "\u2028"} {
		text, err := Write(oneFieldSchema("R", label), false)
		if err != nil {
			t.Errorf("label %q refused: %v", label, err)
			continue
		}
		got, rerr := Read(text)
		if rerr != nil || got.Env["R"].Fields[0].Label != label {
			t.Errorf("label %q did not round-trip: %v %v", label, got, rerr)
		}
	}
}

// OSD-15: a backslash is \\, a quote is \", nothing else is escaped, and the
// four vectors' shapes (including a label ending in a backslash) come out exactly.
func TestWriteEscapesExactlyBackslashAndQuote(t *testing.T) {
	cases := []struct{ label, field string }{
		{`a\b`, `"a\\b"`},
		{`a"b`, `"a\"b"`},
		{`a\"b`, `"a\\\"b"`},
		{`a\`, `"a\\"`},
		{`\`, `"\\"`},
		{`"`, `"\""`},
		{"a'b/cé", "\"a'b/cé\""},
		{"tab-free", `"tab-free"`},
	}
	for _, tc := range cases {
		want := "record R {\n    " + tc.field + ": string,\n}\nroot R\n"
		if got := mustWrite(t, oneFieldSchema("R", tc.label), false); got != want {
			t.Errorf("label %q: got %q, want %q", tc.label, got, want)
		}
		got, err := Read(want)
		if err != nil || got.Env["R"].Fields[0].Label != tc.label {
			t.Errorf("label %q: re-read = %v, %v", tc.label, got, err)
		}
	}
}

// Invalid UTF-8 in a programmatically built label is written byte for byte:
// the writer neither decodes nor repairs it (a rune loop would turn it into
// U+FFFD).
func TestWritePassesLabelBytesThroughUnrepaired(t *testing.T) {
	got := mustWrite(t, oneFieldSchema("R", "a\xffb"), true)
	if !strings.Contains(got, "\"a\xffb\"") {
		t.Errorf("label bytes were altered: %q", got)
	}
}

// Property: Read(Write(s)) equals s for arbitrary non-empty labels without
// brackets or C0 controls, in both layouts. The alphabet is weighted toward
// the characters that break a naive writer.
func TestWriteReadRoundTripsArbitraryLabels(t *testing.T) {
	alphabet := []rune{'a', 'Z', '0', ' ', '\\', '"', '\\', '"', '\'', ',', ':', '{', '}', '#', '/', '\x7f', '\u0085', '\uFEFF', '\u2028', 'é', '€', '😀', '中', '-', '_', '.'}
	rng := rand.New(rand.NewSource(20260922))
	for i := 0; i < 5000; i++ {
		n := 1 + rng.Intn(12)
		var b strings.Builder
		for j := 0; j < n; j++ {
			b.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		label := b.String()
		want := oneFieldSchema("R", label)
		for _, compact := range []bool{false, true} {
			text, err := Write(want, compact)
			if err != nil {
				t.Fatalf("label %q refused: %v", label, err)
			}
			got, err := Read(text)
			if err != nil {
				t.Fatalf("label %q compact=%v: Read(Write(s)) failed: %v\n%s", label, compact, err, text)
			}
			if !schemaEqual(want, got) {
				t.Fatalf("label %q compact=%v: round trip changed the schema\n%s", label, compact, text)
			}
		}
	}
}

// Every refusal has the same shape the writers of the four codecs use, so a
// caller can treat it uniformly: it is an error whose text carries the path and code.
func TestWriteRefusalErrorText(t *testing.T) {
	_, err := Write(oneFieldSchema("R", "a\nb"), false)
	want := "R: write.unsupported-value: "
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("error text = %v, want prefix %q", err, want)
	}
}
