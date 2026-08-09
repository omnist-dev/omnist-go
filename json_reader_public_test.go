package omnist_test

// External "omnist_test" package: TestJSONCrossFormatStructuralEqualityWithOML
// only ever needed OML-text fixtures plus exported API (oml.Read, ReadJSON,
// DocumentsEqual), so it moved out of json_reader_test.go to avoid the
// import cycle package oml would otherwise create (see
// osd_external_test_helpers_test.go's comment for the full explanation;
// this is the OML-side instance of the same issue #41 precedent).

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- cross-format structural equality (JSON vs OML) ---

func TestJSONCrossFormatStructuralEqualityWithOML(t *testing.T) {
	omlText := `id: "A1"; total: 29.97; count: 3; ok: true; nothing: null; address: { street: "1 Main"; city: "London" }; items: "W"; items: "G"`
	jsonText := `{"id":"A1","total":29.97,"count":3,"ok":true,"nothing":null,"address":{"street":"1 Main","city":"London"},"items":["W","G"]}`

	omlDoc := mustOML(t, omlText)
	jsonDoc, err := omnist.ReadJSON(jsonText, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if !omnist.DocumentsEqual(omlDoc, jsonDoc) {
		t.Fatalf("format-independence violated:\nOML:  %#v\nJSON: %#v", omlDoc, jsonDoc)
	}
}
