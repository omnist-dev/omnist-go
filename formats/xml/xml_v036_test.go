package xml

import (
	"math"
	"strings"
	"testing"
	"time"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/osd"
)

func pretypeSchemaV036(t *testing.T) omnist.Schema {
	t.Helper()
	schema, err := osd.Read(`
record R { "i" [0,]: integer, "x" [0,]: number }
record Root { "r": R }
root Root
`)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

// leafV036 reads <r><LABEL>text</LABEL></r> with the schema and returns the
// leaf's scalar.
func leafV036(t *testing.T, label, text string, limits omnist.Limits) (omnist.Scalar, error) {
	t.Helper()
	schema := pretypeSchemaV036(t)
	doc, _, err := ReadWithSchema("<r><"+label+">"+text+"</"+label+"></r>", &schema, limits)
	if err != nil {
		return omnist.Scalar{}, err
	}
	r, _ := doc.Node.Edges[0].Target.Node()
	v, _ := r.Edges[0].Target.Value()
	return v.Scalar, nil
}

// C-18: an integer leaf is read only from -?(0|[1-9][0-9]*).
func TestPretypeIntegerGrammar(t *testing.T) {
	limits := omnist.DefaultLimits()
	for _, text := range []string{"0", "-0", "7", "-12", "1234567890123456789012345678901234567890"} {
		s, err := leafV036(t, "i", text, limits)
		if err != nil || s.Kind != omnist.KindInteger {
			t.Errorf("%q: kind %v err %v, want integer", text, s.Kind, err)
		}
	}
	for _, text := range []string{"+5", "007", "00", "-007", "0x1F", "1_000", "5.0", "1e3", "NaN", "INF", "", " 1", "-", "+"} {
		s, err := leafV036(t, "i", text, limits)
		if err != nil || s.Kind != omnist.KindString || s.Str != text {
			t.Errorf("%q: kind %v str %q err %v, want the string unchanged", text, s.Kind, s.Str, err)
		}
	}
}

// C-18: a number leaf is read only from a JSON number.
func TestPretypeNumberGrammar(t *testing.T) {
	limits := omnist.DefaultLimits()
	good := map[string]float64{"0": 0, "-1": -1, "1.5": 1.5, "-0.25": -0.25, "1e3": 1000, "1E+3": 1000, "2.5e-1": 0.25, "10": 10}
	for text, want := range good {
		s, err := leafV036(t, "x", text, limits)
		if err != nil || s.Kind != omnist.KindNumber || s.Num != want {
			t.Errorf("%q: kind %v num %v err %v, want number %v", text, s.Kind, s.Num, err, want)
		}
	}
	for _, text := range []string{"1.", ".5", "+1.5", "007", "1_0.5", "0x10", "1e", "1e+", "-", "-.5", "NaN", "nan", "INF", "Infinity", "-Infinity", "1.5.2", "00.5"} {
		s, err := leafV036(t, "x", text, limits)
		if err != nil || s.Kind != omnist.KindString || s.Str != text {
			t.Errorf("%q: kind %v str %q err %v, want the string unchanged", text, s.Kind, s.Str, err)
		}
	}
}

// D-29: a number past binary64 reads as an infinity, underflow as zero, -0 as
// negative zero; never a refusal, whatever the length.
func TestPretypeNumberOverflowIsInfinity(t *testing.T) {
	limits := omnist.DefaultLimits()
	for text, sign := range map[string]int{"1e999": 1, "-1e999": -1, "1" + strings.Repeat("0", 400): 1, "-" + strings.Repeat("9", 5000) + ".5": -1} {
		s, err := leafV036(t, "x", text, limits)
		if err != nil || s.Kind != omnist.KindNumber || !math.IsInf(s.Num, sign) {
			t.Errorf("%.20q: kind %v num %v err %v, want %+d infinity", text, s.Kind, s.Num, err, sign)
		}
	}
	s, err := leafV036(t, "x", "1e-999", limits)
	if err != nil || s.Num != 0 || math.Signbit(s.Num) {
		t.Errorf("underflow: %v %v", s.Num, err)
	}
	s, err = leafV036(t, "x", "-1e-999", limits)
	if err != nil || s.Num != 0 || !math.Signbit(s.Num) {
		t.Errorf("negative underflow: %v %v", s.Num, err)
	}
	s, err = leafV036(t, "x", "-0", limits)
	if err != nil || s.Num != 0 || !math.Signbit(s.Num) {
		t.Errorf("-0: %v %v", s.Num, err)
	}
}

// D-28: only an integer leaf is limited; the sign and leading zeros never
// count, a number is exempt however long, and non-integer spellings stay
// strings.
func TestPretypeDigitLimitScope(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 5
	long := strings.Repeat("9", 6)
	for _, text := range []string{long, "-" + long} {
		_, err := leafV036(t, "i", text, limits)
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeDocumentLimitIntDigits || pe.Path != "$.r.i" {
			t.Errorf("%q: %+v, want document.limit.int-digits at $.r.i", text, err)
		}
	}
	for _, text := range []string{"+" + long, "0" + long, long + ".0", long + "e0"} {
		if s, err := leafV036(t, "i", text, limits); err != nil || s.Kind != omnist.KindString {
			t.Errorf("integer %q: kind %v err %v, want a string", text, s.Kind, err)
		}
	}
	for _, text := range []string{long, "-" + long, long + ".5", strings.Repeat("9", 5000)} {
		if s, err := leafV036(t, "x", text, limits); err != nil || s.Kind != omnist.KindNumber {
			t.Errorf("number %.12q: kind %v err %v, want exempt number", text, s.Kind, err)
		}
	}
}

// The grammar scans are linear: a very long leaf reads quickly.
func TestPretypeScansAreLinear(t *testing.T) {
	limits := omnist.DefaultLimits()
	limits.MaxIntDigits = 5_000_000
	text := strings.Repeat("9", 200_000)
	start := time.Now()
	for _, label := range []string{"i", "x"} {
		if _, err := leafV036(t, label, text, limits); err != nil {
			t.Fatal(err)
		}
		if _, err := leafV036(t, label, text+"x", limits); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("long leaves took %v", d)
	}
}

// Non-canonical text is rejected by the grammar scan before any big-number
// parse: a megabyte of digits ending in junk must not cost quadratic time.
func TestPretypeJunkAfterDigitsIsLinear(t *testing.T) {
	limits := omnist.DefaultLimits()
	nines := strings.Repeat("9", 8_000_000)
	start := time.Now()
	for _, label := range []string{"i", "x"} {
		for _, text := range []string{nines + "x", "-" + nines + "x", nines + ".", nines + "e", nines + "e+", "+" + nines} {
			s, err := leafV036(t, label, text, limits)
			if err != nil || s.Kind != omnist.KindString {
				t.Fatalf("%s %.8q: kind %v err %v, want a string", label, text, s.Kind, err)
			}
		}
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("junk-tailed leaves took %v", d)
	}
}
