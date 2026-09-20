package osd

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// Spec §2.5: one leading U+FEFF is stripped (D-15), a second is rejected at
// 1:1 with parse.unexpected-token (D-21). The mark is only ever written as an
// escape.
const bomMark = "\uFEFF"

func TestReadLeadingBOMIsStrippedAndDoubledIsRejected(t *testing.T) {
	const body = "record R {\n    \"a\": string,\n}\nroot R\n"
	plain, err := Read(body)
	if err != nil {
		t.Fatal(err)
	}
	withBOM, err := Read(bomMark + body)
	if err != nil {
		t.Fatalf("a single leading mark must be stripped: %v", err)
	}
	if !schemaEqual(plain, withBOM) {
		t.Errorf("a leading mark changed the Schema")
	}
	for _, in := range []string{bomMark + bomMark + body, bomMark + bomMark + bomMark + body} {
		_, err := Read(in)
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeParseUnexpectedToken || pe.Path != "1:1" {
			t.Errorf("got %#v, want parse.unexpected-token at 1:1", err)
		}
	}
}

// E-23: a string-body error reports the string's opening quote, for a raw
// control character and for one immediately after a backslash alike (§5.3.1:
// the ban covers escape context too).
func TestStringErrorsReportTheOpeningQuote(t *testing.T) {
	cases := map[string]string{
		"raw control character":          "record R {\n    \"a\x01b\": string,\n}\nroot R\n",
		"escaped control character":      "record R {\n    \"a\\\x01b\": string,\n}\nroot R\n",
		"escaped newline":                "record R {\n    \"a\\\nb\": string,\n}\nroot R\n",
		"control char after other chars": "record R {\n    \"abcdef\x1fg\": string,\n}\nroot R\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Read(text)
			pe, ok := err.(*omnist.ParseError)
			if !ok || pe.Code != omnist.CodeParseControlCharacter || pe.Path != "2:5" {
				t.Errorf("got %#v, want parse.control-character at 2:5", err)
			}
		})
	}
	// An unterminated string is also reported at its opening quote.
	_, err := Read("record R {\n    \"abc")
	if pe, ok := err.(*omnist.ParseError); !ok || pe.Code != omnist.CodeParseUnterminatedString || pe.Path != "2:5" {
		t.Errorf("got %#v, want parse.unterminated-string at 2:5", err)
	}
}
