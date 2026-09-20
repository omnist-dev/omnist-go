package oml

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// Spec §2.5: one leading U+FEFF is stripped (D-15), a second is rejected at
// 1:1 with parse.unexpected-token (D-21). The mark is only ever written as an escape.
const bomMark = "\uFEFF"

func TestReadLeadingBOMIsStrippedAndDoubledIsRejected(t *testing.T) {
	const body = "a: 1\n"
	plain, err := Read(body, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	withBOM, err := Read(bomMark+body, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("a single leading mark must be stripped: %v", err)
	}
	if !docEqual(plain, withBOM) {
		t.Errorf("a leading mark changed the Document")
	}
	for _, in := range []string{bomMark + bomMark + body, bomMark + bomMark + bomMark + body} {
		_, err := Read(in, omnist.DefaultLimits())
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeParseUnexpectedToken || pe.Path != "1:1" {
			t.Errorf("got %#v, want parse.unexpected-token at 1:1", err)
		}
	}
}

// E-23: a string-body error reports the string's opening quote -- including a
// control character inside a multiline string, which used to report the
// character's own position.
func TestMultilineStringControlCharacterReportsTheOpeningQuote(t *testing.T) {
	_, err := Read("k: 1\na: \"\"\"\nfirst\nsec\x01ond\n\"\"\"\n", omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseControlCharacter || pe.Path != "2:4" {
		t.Errorf("got %#v, want parse.control-character at 2:4 (the opening \"\"\")", err)
	}
}

// OML-25 / E-24: any scalar followed by leftover content at document level is
// parse.trailing-content at the first leftover significant token; a trailing
// comment is not leftover.
func TestScalarThenLeftoverContentIsTrailingContent(t *testing.T) {
	cases := []struct{ in, path string }{
		{"nan: 1", "1:4"},
		{"inf: 1", "1:4"},
		{"5: 1", "1:2"},
		{"true: 1", "1:5"},
		{"null: 1", "1:5"},
		{"1\n2", "2:1"},
	}
	for _, tc := range cases {
		_, err := Read(tc.in, omnist.DefaultLimits())
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeParseTrailingContent || pe.Path != tc.path {
			t.Errorf("%q: got %#v, want parse.trailing-content at %s", tc.in, err, tc.path)
		}
	}
	if _, err := Read("5 # just a comment\n", omnist.DefaultLimits()); err != nil {
		t.Errorf("a trailing comment is not leftover content: %v", err)
	}
}
