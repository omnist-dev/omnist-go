package json

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// Spec §2.5: one leading U+FEFF is stripped (D-15), a second is rejected at
// 1:1 with parse.codec-syntax (D-21). The mark is only ever written as an escape.
const bomMark = "\uFEFF"

func TestReadLeadingBOMIsStrippedAndDoubledIsRejected(t *testing.T) {
	const body = `{"a":1}`
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
		if !ok || pe.Code != omnist.CodeParseCodecSyntax || pe.Path != "1:1" {
			t.Errorf("got %#v, want parse.codec-syntax at 1:1", err)
		}
	}
}
