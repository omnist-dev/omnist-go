package conformance

import (
	"encoding/json"
	"testing"
)

// E-27: read-side vectors give their input as text or bytes_hex, exactly one.
// These pin the runner's handling of the bytes form directly, so the runner
// cannot quietly regress to running an empty or repaired input.

func parseVec(input, expect string) Vector {
	return Vector{Name: "t", Operation: "parse", Input: json.RawMessage(input), Expect: json.RawMessage(expect)}
}

const invalidEncodingExpect = `{"ok": false, "diagnostics": [{"path": "1:1", "code": "parse.invalid-encoding"}]}`

func TestBytesHexInvalidUTF8IsPresentedAsRawBytesAndRejected(t *testing.T) {
	// "{"a": "<e2 82>"}" as raw bytes: the reader must see the bad bytes.
	v := parseVec(`{"format": "json", "bytes_hex": "7b2261223a2022e2827d"}`, invalidEncodingExpect)
	if r := RunVector(v); r.Status != StatusPass {
		t.Fatalf("status = %v (%s), want pass", r.Status, r.Reason)
	}
}

func TestBytesHexWrongExpectationIsAFailNotAPass(t *testing.T) {
	// The same bytes with the wrong code named must fail: the runner really
	// compares (path, code).
	v := parseVec(`{"format": "json", "bytes_hex": "7b2261223a2022e2827d"}`,
		`{"ok": false, "diagnostics": [{"path": "1:1", "code": "parse.codec-syntax"}]}`)
	if r := RunVector(v); r.Status != StatusFail {
		t.Fatalf("status = %v, want fail", r.Status)
	}
}

func TestBytesHexValidMultiByteIsAccepted(t *testing.T) {
	// {"a": "é"} with é as c3 a9.
	v := parseVec(`{"format": "json", "bytes_hex": "7b2261223a2022c3a9227d"}`,
		`{"ok": true, "document": {"edges": [["a", {"scalar": {"kind": "string", "value": "é"}}]]}}`)
	if r := RunVector(v); r.Status != StatusPass {
		t.Fatalf("status = %v (%s), want pass", r.Status, r.Reason)
	}
}

func TestReadSourceInputRejectsMalformedInputForms(t *testing.T) {
	s := func(x string) *string { return &x }
	cases := []struct {
		name        string
		text, bytes *string
		want        string
		wantErr     bool
	}{
		{"text only", s("abc"), nil, "abc", false},
		{"empty text is still text", s(""), nil, "", false},
		{"bytes only", nil, s("c3a9"), "\xc3\xa9", false},
		{"bytes of an invalid sequence stay invalid", nil, s("e282"), "\xe2\x82", false},
		{"empty bytes", nil, s(""), "", false},
		{"both", s("a"), s("61"), "", true},
		{"neither", nil, nil, "", true},
		{"odd length", nil, s("abc"), "", true},
		{"not hex", nil, s("zz"), "", true},
	}
	for _, tc := range cases {
		got, err := readSourceInput(tc.text, tc.bytes)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q, err=%v", tc.name, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestParseVectorWithNeitherInputFieldFails(t *testing.T) {
	v := parseVec(`{"format": "json"}`, `{"ok": true}`)
	if r := RunVector(v); r.Status != StatusFail {
		t.Fatalf("status = %v, want fail (a missing input is not the empty string)", r.Status)
	}
}

func TestParseSchemaTakesBytesHex(t *testing.T) {
	// record R { "a<80>b": string, } root R
	v := Vector{Name: "t", Operation: "parse_schema",
		Input:  json.RawMessage(`{"bytes_hex": "7265636f72642052207b0a2020202022618062223a20737472696e672c0a7d0a726f6f7420520a"}`),
		Expect: json.RawMessage(invalidEncodingExpect)}
	if r := RunVector(v); r.Status != StatusPass {
		t.Fatalf("status = %v (%s), want pass", r.Status, r.Reason)
	}
	v.Input = json.RawMessage(`{}`)
	if r := RunVector(v); r.Status != StatusFail {
		t.Fatalf("no input: status = %v, want fail", r.Status)
	}
}
