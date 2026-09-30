package conformance

import (
	"fmt"
	"testing"
)

// E-32: the "line:col" placeholder. Code must match exactly, the path must be a
// well-formed position inside the input, and every other path stays byte for
// byte. These pin the runner against weakening in either direction.

func placeholderVec(format, text, path, code string) Vector {
	input := fmt.Sprintf(`{"format": %q, "text": %q}`, format, text)
	expect := fmt.Sprintf(`{"ok": false, "diagnostics": [{"path": %q, "code": %q}]}`, path, code)
	return parseVec(input, expect)
}

func TestPlaceholderAcceptsAnyWellFormedPositionInsideInput(t *testing.T) {
	cases := []struct{ format, text string }{
		{"json", `{"a": }`},
		{"yaml", "a: b: c"},
		{"toml", "a = "},
		{"xml", "<a><b></a>"},
	}
	for _, tc := range cases {
		v := placeholderVec(tc.format, tc.text, "line:col", "parse.codec-syntax")
		if r := RunVector(v); r.Status != StatusPass {
			t.Errorf("%s: status = %v (%s), want pass", tc.format, r.Status, r.Reason)
		}
	}
}

func TestPlaceholderStillComparesTheCode(t *testing.T) {
	// The reader reports parse.codec-syntax; expecting another code under the
	// placeholder must fail. (The placeholder only stands on codec-syntax, so
	// with another code it is an ordinary path no real position equals.)
	v := placeholderVec("json", `{"a": }`, "line:col", "parse.unexpected-token")
	if r := RunVector(v); r.Status != StatusFail {
		t.Fatalf("status = %v, want fail", r.Status)
	}
}

func TestPlaceholderOnlyOnCodecFormats(t *testing.T) {
	// OML input with the placeholder: not E-32a-allowed, compared literally,
	// so it fails even though the code matches.
	v := placeholderVec("oml", "a: [1, 2", "line:col", "parse.codec-syntax")
	if r := RunVector(v); r.Status != StatusFail {
		t.Fatalf("status = %v, want fail", r.Status)
	}
}

func TestPlaceholderNeedsExactlyOneDiagnostic(t *testing.T) {
	v := parseVec(`{"format": "json", "text": "{\"a\": }"}`,
		`{"ok": false, "diagnostics": [{"path": "line:col", "code": "parse.codec-syntax"}, {"path": "1:1", "code": "parse.codec-syntax"}]}`)
	if r := RunVector(v); r.Status != StatusFail {
		t.Fatalf("status = %v, want fail", r.Status)
	}
}

func TestNonPlaceholderPathsStayByteForByte(t *testing.T) {
	// A real position that is not the reader's still fails: the relaxation is
	// the literal placeholder only. The JSON reader blames 1:7 here.
	v := placeholderVec("json", `{"a": }`, "1:1", "parse.codec-syntax")
	if r := RunVector(v); r.Status != StatusFail {
		t.Fatalf("status = %v, want fail", r.Status)
	}
	v = placeholderVec("json", `{"a": }`, "1:7", "parse.codec-syntax")
	if r := RunVector(v); r.Status != StatusPass {
		t.Fatalf("exact path: status = %v (%s), want pass", r.Status, r.Reason)
	}
}

func TestParseDiagnosticsEqualPlaceholderNearMisses(t *testing.T) {
	want := []diagPair{{Path: "line:col", Code: "parse.codec-syntax"}}
	cases := []struct {
		name   string
		actual []diagPair
		input  string
		ok     bool
	}{
		{"1:1 is valid for every input", []diagPair{{"1:1", "parse.codec-syntax"}}, "abc", true},
		{"end of input is inside", []diagPair{{"1:4", "parse.codec-syntax"}}, "abc", true},
		{"second line after LF", []diagPair{{"2:1", "parse.codec-syntax"}}, "abc\n", true},
		{"0:0", []diagPair{{"0:0", "parse.codec-syntax"}}, "abc", false},
		{"1:0", []diagPair{{"1:0", "parse.codec-syntax"}}, "abc", false},
		{"0:1", []diagPair{{"0:1", "parse.codec-syntax"}}, "abc", false},
		{"empty path", []diagPair{{"", "parse.codec-syntax"}}, "abc", false},
		{"leading zero", []diagPair{{"01:1", "parse.codec-syntax"}}, "abc", false},
		{"sign", []diagPair{{"+1:1", "parse.codec-syntax"}}, "abc", false},
		{"whitespace", []diagPair{{"1: 1", "parse.codec-syntax"}}, "abc", false},
		{"document path", []diagPair{{"$", "parse.codec-syntax"}}, "abc", false},
		{"literal placeholder echoed back", []diagPair{{"line:col", "parse.codec-syntax"}}, "abc", false},
		{"line past the last", []diagPair{{"2:1", "parse.codec-syntax"}}, "abc", false},
		{"column past end of line", []diagPair{{"1:5", "parse.codec-syntax"}}, "abc", false},
		{"column counts code points", []diagPair{{"1:5", "parse.codec-syntax"}}, "\u00e9\u20ac\U0001F600", false},
		{"column in code points is inside", []diagPair{{"1:3", "parse.codec-syntax"}}, "\u00e9\u20ac\U0001F600", true},
		{"overflowing number", []diagPair{{"99999999999999999999:1", "parse.codec-syntax"}}, "abc", false},
		{"wrong code", []diagPair{{"1:1", "parse.unexpected-token"}}, "abc", false},
		{"no diagnostic", nil, "abc", false},
		{"extra diagnostic", []diagPair{{"1:1", "parse.codec-syntax"}, {"1:1", "parse.codec-syntax"}}, "abc", false},
	}
	for _, tc := range cases {
		if got := parseDiagnosticsEqual(tc.actual, want, "json", tc.input); got != tc.ok {
			t.Errorf("%s: parseDiagnosticsEqual = %v, want %v", tc.name, got, tc.ok)
		}
	}
}

func TestParseDiagnosticsEqualWithoutPlaceholderIsByteExact(t *testing.T) {
	want := []diagPair{{Path: "1:7", Code: "parse.codec-syntax"}}
	if !parseDiagnosticsEqual([]diagPair{{"1:7", "parse.codec-syntax"}}, want, "json", "x") {
		t.Error("exact match should be equal")
	}
	if parseDiagnosticsEqual([]diagPair{{"1:8", "parse.codec-syntax"}}, want, "json", "x") {
		t.Error("1:8 must not match 1:7")
	}
}
