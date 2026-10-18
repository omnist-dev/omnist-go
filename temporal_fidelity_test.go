package omnist_test

import (
	"errors"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
)

// Readers must never alter a temporal value: an offset stays the offset it
// was written with (never converted to UTC) and every fraction digit is kept
// or the read fails (cross-port checks 1 and 2).

type temporalReader struct {
	name string
	line func(key, val string) string
	read func(string) (omnist.Document, error)
}

var temporalReaders = []temporalReader{
	{"yaml", func(k, v string) string { return k + ": " + v + "\n" }, func(s string) (omnist.Document, error) { return yaml.Read(s, omnist.DefaultLimits()) }},
	{"toml", func(k, v string) string { return k + " = " + v + "\n" }, func(s string) (omnist.Document, error) { return toml.Read(s, omnist.DefaultLimits()) }},
	{"oml", func(k, v string) string { return k + ": " + v + "\n" }, func(s string) (omnist.Document, error) { return oml.Read(s, omnist.DefaultLimits()) }},
}

func firstScalar(t *testing.T, d omnist.Document) omnist.Scalar {
	t.Helper()
	if !d.IsNode || len(d.Node.Edges) == 0 {
		t.Fatalf("document has no edge: %+v", d)
	}
	v, ok := d.Node.Edges[0].Target.Value()
	if !ok || v.IsNull {
		t.Fatalf("first edge is not a scalar: %+v", d.Node.Edges[0])
	}
	return v.Scalar
}

func TestReadersKeepOffsetsAndFractions(t *testing.T) {
	offsets := map[string]int{"+01:00": 3600, "-08:00": -28800, "+05:30": 19800, "+14:00": 50400, "-00:30": -1800}
	fracs := []struct {
		text  string
		nanos int
	}{{"1", 100000000}, {"123", 123000000}, {"1234", 123400000}, {"123456", 123456000}}
	for _, r := range temporalReaders {
		for off, secs := range offsets {
			for _, f := range fracs {
				frac, nanos := f.text, f.nanos
				text := "2024-01-02T12:00:00." + frac + off
				s := firstScalar(t, mustRead(t, r, r.line("t", text)))
				tv := s.DateTime.Time
				if s.Kind != omnist.KindDateTime || !tv.HasOffset || tv.OffsetSeconds != secs || tv.Nanosecond != nanos || tv.Hour != 12 {
					t.Errorf("%s %s: got %+v, want offset %d nanos %d", r.name, text, s, secs, nanos)
				}
				if got := omnist.FormatISOTime(tv); got != "12:00:00."+frac+off {
					t.Errorf("%s %s: time formats as %q", r.name, text, got)
				}
			}
		}
	}
	// OML also has a bare time with an offset.
	s := firstScalar(t, mustRead(t, temporalReaders[2], "x: 12:00:00.5+01:00\n"))
	if s.Kind != omnist.KindTime || s.Time.OffsetSeconds != 3600 || s.Time.Nanosecond != 500000000 {
		t.Errorf("oml time: %+v", s)
	}
}

func mustRead(t *testing.T, r temporalReader, src string) omnist.Document {
	t.Helper()
	d, err := r.read(src)
	if err != nil {
		t.Fatalf("%s %q: %v", r.name, src, err)
	}
	return d
}

// JSON and XML carry no native temporal: the text is kept verbatim.
func TestJSONAndXMLKeepTemporalTextVerbatim(t *testing.T) {
	const text = "2024-01-02T12:00:00.123456789+01:00"
	d, err := json.Read(`{"t":"`+text+`"}`, omnist.DefaultLimits())
	if err != nil || firstScalar(t, d).Str != text {
		t.Errorf("json: %+v %v", d, err)
	}
	d, _, err = xml.Read(`<r><t>`+text+`</t></r>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	v, _ := d.Node.Edges[0].Target.Node()
	if got := firstScalar(t, omnist.NodeDocument(v)).Str; got != text {
		t.Errorf("xml: %q", got)
	}
}

// A fraction longer than six digits cannot be held exactly: every reader that
// has a native temporal refuses it with a coded error, never truncating.
func TestReadersRefuseFractionsOverSixDigits(t *testing.T) {
	for _, r := range temporalReaders {
		for _, digits := range []int{7, 8, 9, 10} {
			text := "2024-01-02T12:00:00." + strings.Repeat("1", digits) + "+01:00"
			_, err := r.read(r.line("t", text))
			var pe *omnist.ParseError
			if !errors.As(err, &pe) || pe.Code == "" {
				t.Errorf("%s %d digits: err = %v, want a coded ParseError", r.name, digits, err)
			}
		}
	}
	// A quoted YAML scalar is a string, whatever it looks like.
	d, err := yaml.Read(`t: "2024-01-02T12:00:00.1234567+01:00"`, omnist.DefaultLimits())
	if err != nil || firstScalar(t, d).Kind != omnist.KindString {
		t.Errorf("quoted: %+v %v", d, err)
	}
}

type temporalWriter struct {
	name  string
	write func(omnist.Document) (string, []omnist.Diagnostic, error)
	read  temporalReader
}

var temporalWriters = []temporalWriter{
	{"oml", func(d omnist.Document) (string, []omnist.Diagnostic, error) { return oml.Write(d, false) }, temporalReaders[2]},
	{"oml-compact", oml.WriteCompact, temporalReaders[2]},
	{"yaml", yaml.Write, temporalReaders[0]},
	{"toml", toml.Write, temporalReaders[1]},
}

func temporalDoc(nanos int) omnist.Document {
	tv := omnist.TimeValue{Hour: 12, Second: 1, Nanosecond: nanos, HasOffset: true, OffsetSeconds: 3600}
	dt := omnist.DateTimeValue{Date: omnist.DateValue{Year: 2024, Month: 1, Day: 2}, Time: tv}
	n := omnist.NewNode()
	n.AddValue("dt", omnist.ScalarValue(omnist.NewDateTimeScalar(dt)))
	return omnist.NodeDocument(n)
}

// A writer must not emit a literal its own reader rejects: a fraction finer
// than a microsecond is written as exact text in a string, reported, and the
// output reads back.
func TestWritersStringifySubMicrosecondFractions(t *testing.T) {
	for _, w := range temporalWriters {
		for _, nanos := range []int{123456789, 123456780, 100000001, 1} {
			out, diags, err := w.write(temporalDoc(nanos))
			if err != nil {
				t.Fatalf("%s: %v", w.name, err)
			}
			if len(diags) != 1 || diags[0].Code != omnist.CodeFormatTemporalStringified || diags[0].Path != "$.dt" {
				t.Errorf("%s %d: diags = %+v", w.name, nanos, diags)
			}
			back, err := w.read.read(out)
			if err != nil {
				t.Fatalf("%s %d: output %q does not read back: %v", w.name, nanos, out, err)
			}
			s := firstScalar(t, back)
			want := "2024-01-02T12:00:01." + strings.TrimRight(nine(nanos), "0") + "+01:00"
			if s.Kind != omnist.KindString || s.Str != want {
				t.Errorf("%s %d: read back %+v, want string %q", w.name, nanos, s, want)
			}
		}
		// Exact within six digits: native literal, no diagnostic, same value.
		out, diags, err := w.write(temporalDoc(123456000))
		if err != nil || len(diags) != 0 {
			t.Fatalf("%s: %v %+v", w.name, err, diags)
		}
		s := firstScalar(t, mustRead(t, w.read, out))
		if s.Kind != omnist.KindDateTime || s.DateTime.Time.Nanosecond != 123456000 || s.DateTime.Time.OffsetSeconds != 3600 {
			t.Errorf("%s: %+v", w.name, s)
		}
	}
}

func nine(n int) string {
	d := []byte("000000000")
	for i := 8; i >= 0 && n > 0; i-- {
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d)
}

// JSON and XML stringify every temporal already; they now keep every digit.
func TestJSONAndXMLWritersKeepEveryFractionDigit(t *testing.T) {
	d := temporalDoc(123456789)
	out, _, err := json.Write(d)
	if err != nil || !strings.Contains(out, "2024-01-02T12:00:01.123456789+01:00") {
		t.Errorf("json: %q %v", out, err)
	}
	out, _, err = xml.Write(omnist.NodeDocument(omnist.NewNode().AddNode("r", d.Node)))
	if err != nil || !strings.Contains(out, "2024-01-02T12:00:01.123456789+01:00") {
		t.Errorf("xml: %q %v", out, err)
	}
}

func TestStringifySubMicrosecond(t *testing.T) {
	// Nothing to do: the same Document comes back, no diagnostics.
	d := temporalDoc(123456000)
	got, diags := omnist.StringifySubMicrosecond(d)
	if got.Node != d.Node || diags != nil {
		t.Errorf("clean document was copied or reported: %+v", diags)
	}

	// A bare root value, a time, a null, and a repeated label (indexed path).
	root := omnist.Document{Value: omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 1, Nanosecond: 7}))}
	got, diags = omnist.StringifySubMicrosecond(root)
	if got.IsNode || got.Value.Scalar.Kind != omnist.KindString || got.Value.Scalar.Str != "01:00:00.000000007" || len(diags) != 1 || diags[0].Path != "$" {
		t.Errorf("root: %+v %+v", got, diags)
	}
	clean := omnist.Document{Value: omnist.NullValue()}
	if got, diags = omnist.StringifySubMicrosecond(clean); !got.Value.IsNull || diags != nil {
		t.Errorf("null root: %+v %+v", got, diags)
	}

	sub := omnist.ScalarValue(omnist.NewTimeScalar(omnist.TimeValue{Hour: 2, Nanosecond: 1}))
	inner := omnist.NewNode().AddValue("x", omnist.ScalarValue(omnist.NewStringScalar("s"))).AddValue("t", sub).AddValue("n", omnist.NullValue())
	outer := omnist.NewNode().AddNode("a", inner).AddNode("a", inner).AddValue("a", sub)
	got, diags = omnist.StringifySubMicrosecond(omnist.NodeDocument(outer))
	if len(diags) != 3 || diags[0].Path != "$.a[0].t" || diags[1].Path != "$.a[1].t" || diags[2].Path != "$.a[2]" {
		t.Errorf("paths: %+v", diags)
	}
	// The input is never modified.
	if v, _ := inner.Edges[1].Target.Value(); v.Scalar.Kind != omnist.KindTime {
		t.Errorf("input modified: %+v", v)
	}
	if v, _ := got.Node.Edges[2].Target.Value(); v.Scalar.Kind != omnist.KindString {
		t.Errorf("copy not stringified: %+v", v)
	}
}
