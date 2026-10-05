package omnist_test

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
)

// D-23 to D-26 (spec §2.4.2): a finite maximum input size, in bytes, checked
// before decoding and before any parsing, refused with document.limit.input-size
// at "$", equal to the maximum accepted.

func TestMaxInputBytesLimitsAndValidate(t *testing.T) {
	if omnist.DefaultLimits().MaxInputBytes != omnist.DefaultMaxInputBytes || omnist.DefaultMaxInputBytes != 64<<20 {
		t.Errorf("default input size = %d, want 64 MiB", omnist.DefaultLimits().MaxInputBytes)
	}
	if omnist.MaxRecommendedInputBytes != 1<<30 {
		t.Errorf("recommended ceiling = %d, want 1 GiB", omnist.MaxRecommendedInputBytes)
	}
	// Positive values are used; zero and negatives select the default and
	// never mean unbounded (D-10), like every other defaulted limit.
	for _, tc := range []struct{ in, want int }{{1, 1}, {20, 20}, {1 << 30, 1 << 30}, {0, 64 << 20}, {-1, 64 << 20}, {-1 << 62, 64 << 20}} {
		if got := (omnist.Limits{MaxInputBytes: tc.in}).EffectiveMaxInputBytes(); got != tc.want {
			t.Errorf("Effective(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
	base := omnist.Limits{MaxDepth: 10, MaxNodes: 100, MaxIntDigits: 10}
	if err := base.Validate(); err != nil {
		t.Errorf("Validate with MaxInputBytes unset: %v", err)
	}
	base.MaxInputBytes = omnist.MaxRecommendedInputBytes
	if err := base.Validate(); err != nil {
		t.Errorf("Validate at the ceiling: %v", err)
	}
	base.MaxInputBytes = -1
	if err := base.Validate(); err == nil {
		t.Error("expected error for negative MaxInputBytes")
	}
	base.MaxInputBytes = omnist.MaxRecommendedInputBytes + 1
	if err := base.Validate(); err == nil {
		t.Error("expected error for MaxInputBytes > MaxRecommendedInputBytes")
	}
}

func TestCheckInputSize(t *testing.T) {
	l := omnist.Limits{MaxInputBytes: 5}
	if err := omnist.CheckInputSize("12345", l); err != nil {
		t.Errorf("an input equal to the maximum must be accepted: %v", err)
	}
	if err := omnist.CheckInputSize("", l); err != nil {
		t.Errorf("empty input: %v", err)
	}
	err := omnist.CheckInputSize("123456", l)
	if err == nil || err.Code != omnist.CodeDocumentLimitInputSize || err.Path != "$" {
		t.Fatalf("got %#v, want document.limit.input-size at $", err)
	}
	if !strings.Contains(err.Message, "MaxInputBytes") {
		t.Errorf("message %q should name the Limits field that raises the cap", err.Message)
	}
	// Bytes, not characters: two two-byte characters are four bytes.
	if err := omnist.CheckInputSize("éé", omnist.Limits{MaxInputBytes: 4}); err != nil {
		t.Errorf("4 bytes against a maximum of 4: %v", err)
	}
	if err := omnist.CheckInputSize("éé", omnist.Limits{MaxInputBytes: 3}); err == nil {
		t.Error("4 bytes against a maximum of 3 must be refused")
	}
	// An unset maximum is the default, never "no limit".
	if err := omnist.CheckInputSize("x", omnist.Limits{}); err != nil {
		t.Errorf("zero Limits: %v", err)
	}
	if err := omnist.CheckInputSize(strings.Repeat("x", omnist.DefaultMaxInputBytes+1), omnist.Limits{}); err == nil {
		t.Error("an input over the default maximum must be refused when the limit is unset")
	}
	if err := omnist.CheckInputSize(strings.Repeat("x", omnist.DefaultMaxInputBytes), omnist.Limits{MaxInputBytes: -7}); err != nil {
		t.Errorf("an input at the default maximum, negative limit: %v", err)
	}
}

// readerCase builds, for one format, a valid document of exactly n bytes of
// padding inside a fixed frame, and reads it.
type readerCase struct {
	name  string
	build func(pad string) string
	read  func(text string, l omnist.Limits) error
}

func inputSizeReaders() []readerCase {
	return []readerCase{
		{"json", func(p string) string { return `{"a":"` + p + `"}` },
			func(s string, l omnist.Limits) error { _, err := json.Read(s, l); return err }},
		{"yaml", func(p string) string { return "a: " + p + "\n" },
			func(s string, l omnist.Limits) error { _, err := yaml.Read(s, l); return err }},
		{"toml", func(p string) string { return `a = "` + p + `"` + "\n" },
			func(s string, l omnist.Limits) error { _, err := toml.Read(s, l); return err }},
		{"xml", func(p string) string { return "<a>" + p + "</a>" },
			func(s string, l omnist.Limits) error { _, _, err := xml.Read(s, l); return err }},
		{"xml-with-schema", func(p string) string { return "<a>" + p + "</a>" },
			func(s string, l omnist.Limits) error { _, _, err := xml.ReadWithSchema(s, nil, l); return err }},
		{"oml", func(p string) string { return `a: "` + p + `"` },
			func(s string, l omnist.Limits) error { _, err := oml.Read(s, l); return err }},
	}
}

// lim is DefaultLimits with the input-size maximum set to n (zero leaves it unset).
func lim(n int) omnist.Limits {
	l := omnist.DefaultLimits()
	l.MaxInputBytes = n
	return l
}

func wantInputSize(t *testing.T, what string, err error) {
	t.Helper()
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeDocumentLimitInputSize || pe.Path != "$" {
		t.Errorf("%s: got %#v, want document.limit.input-size at $", what, err)
	}
}

func TestEveryReaderEnforcesMaxInputBytes(t *testing.T) {
	for _, rc := range inputSizeReaders() {
		t.Run(rc.name, func(t *testing.T) {
			text := rc.build(strings.Repeat("x", 12))
			n := len(text)
			// Exactly at the maximum: accepted. One byte over: refused.
			if err := rc.read(text, lim(n)); err != nil {
				t.Errorf("at the maximum (%d bytes): %v", n, err)
			}
			wantInputSize(t, "one over", rc.read(text, lim(n-1)))
			wantInputSize(t, "far over", rc.read(text, lim(1)))

			// Bytes, not characters: replace two ASCII characters of padding
			// with one two-byte character each; the text is the same number
			// of bytes but fewer characters.
			multi := rc.build("é" + strings.Repeat("x", 10))
			nm := len(multi)
			if err := rc.read(multi, lim(nm)); err != nil {
				t.Errorf("multibyte at the maximum (%d bytes): %v", nm, err)
			}
			wantInputSize(t, "multibyte, characters would fit", rc.read(multi, lim(nm-1)))

			// A leading BOM is counted (taken before the strip, D-15): the
			// input is accepted at len(text)+3 and refused at len(text)+2.
			bom := string(rune(0xFEFF)) + text
			if err := rc.read(bom, lim(n+3)); err != nil {
				t.Errorf("BOM at the maximum: %v", err)
			}
			wantInputSize(t, "BOM counted", rc.read(bom, lim(n+2)))

			// The size check precedes decoding (D-23): an oversized input that
			// is also invalid UTF-8 reports input-size, not parse.invalid-encoding;
			// within the cap the encoding error is reported as before.
			bad := text + "\xff"
			wantInputSize(t, "oversized and invalid UTF-8", rc.read(bad, lim(n)))
			if pe, ok := rc.read(bad, lim(n+1)).(*omnist.ParseError); !ok || pe.Code != omnist.CodeParseInvalidEncoding {
				t.Errorf("within the cap: got %#v, want parse.invalid-encoding", pe)
			}

			// Oversized and malformed: the size refusal still wins.
			wantInputSize(t, "oversized and malformed", rc.read("{{{{{{{{{{", lim(3)))

			// An unset maximum selects the default (never "no limit"): a small
			// input is read and the zero Limits does not refuse it.
			if err := rc.read(text, lim(0)); err != nil {
				t.Errorf("unset maximum: %v", err)
			}
		})
	}
}

// The default maximum applies to a reader called with DefaultLimits, one byte
// over it is refused before any parsing, and the input need not be valid.
func TestEveryReaderRefusesAboveTheDefaultMaximum(t *testing.T) {
	big := strings.Repeat("x", omnist.DefaultMaxInputBytes+1)
	for _, rc := range inputSizeReaders() {
		t.Run(rc.name, func(t *testing.T) {
			wantInputSize(t, "default maximum", rc.read(big, omnist.DefaultLimits()))
		})
	}
}
