package conformance

import (
	"strings"
	"testing"
)

// A vector carrying declared_max_input_bytes (D-23) is RUN, with that value
// passed to the reader as Limits.MaxInputBytes, and for no other vector: the
// JSON text below is 20 bytes. Declared 20 accepts it, declared 19 refuses it
// with document.limit.input-size at "$", and the same input with no key runs
// under the 64 MiB default and is accepted.
func TestInputBytesLimitKeyIsRunNotSkipped(t *testing.T) {
	const text = `"{\"a\":\"xxxxxxxxxxxx\"}"`
	input := func(limit string) []byte {
		if limit == "" {
			return []byte(`{"format": "json", "text": ` + text + `}`)
		}
		return []byte(`{"format": "json", "declared_max_input_bytes": ` + limit + `, "text": ` + text + `}`)
	}
	reject := []byte(`{"ok": false, "diagnostics": [{"path": "$", "code": "document.limit.input-size"}]}`)
	for _, tc := range []struct {
		limit string
		want  Status
	}{{"19", StatusPass}, {"20", StatusFail}, {"", StatusFail}} {
		r := RunVector(Vector{Name: "document-model/input-size/x", Operation: "parse", Input: input(tc.limit), Expect: reject})
		if r.Status != tc.want {
			t.Errorf("limit %q: status = %v (%s), want %v", tc.limit, r.Status, r.Reason, tc.want)
		}
	}
}

// E-20a: a runner MUST NOT run a vector carrying a declared_* key it does not
// understand against its own default; it fails it or reports an E-20 skip.
// This runner skips, citing E-20a, so the boundary is never silently passed.
func TestUnknownDeclaredKeyIsSkippedNotRun(t *testing.T) {
	// The input is valid JSON, so a run that ignored the key would PASS.
	input := []byte(`{"format": "json", "declared_max_string_length": 3, "text": "{\"a\":\"xxxxxxxx\"}"}`)
	expect := []byte(`{"ok": true, "document": {"edges": [["a", {"scalar": {"kind": "string", "value": "xxxxxxxx"}}]]}}`)
	r := RunVector(Vector{Name: "x", Operation: "parse", Input: input, Expect: expect})
	if r.Status != StatusSkip {
		t.Fatalf("status = %v (%s), want skip", r.Status, r.Reason)
	}
	if !strings.Contains(r.Reason, "declared_max_string_length") || !strings.Contains(r.Reason, "E-20a") {
		t.Errorf("reason %q should name the key and E-20a", r.Reason)
	}
	// The same vector without the key still runs and passes.
	clean := []byte(`{"format": "json", "text": "{\"a\":\"xxxxxxxx\"}"}`)
	if r := RunVector(Vector{Name: "x", Operation: "parse", Input: clean, Expect: expect}); r.Status != StatusPass {
		t.Errorf("without the key: status = %v (%s), want pass", r.Status, r.Reason)
	}
}

// Any key under the declared_ prefix counts, not only declared_max_*.
func TestUnknownDeclaredKeyWithoutMaxInTheNameIsSkipped(t *testing.T) {
	input := []byte(`{"format": "json", "declared_string_cap": 3, "text": "1"}`)
	r := RunVector(Vector{Name: "x", Operation: "parse", Input: input, Expect: []byte(`{"ok": true}`)})
	if r.Status != StatusSkip || !strings.Contains(r.Reason, "declared_string_cap") {
		t.Errorf("status = %v (%s), want an E-20a skip naming the key", r.Status, r.Reason)
	}
}

// A declared_* key this runner understands, on an operation whose driver does
// not honour it, is an E-20 skip too: only parse takes the six limit keys.
func TestKnownDeclaredKeyOnAnOperationThatIgnoresItIsSkipped(t *testing.T) {
	for _, op := range []string{"parse_schema", "validate", "materialize", "write", "normalize", "lint"} {
		input := []byte(`{"declared_max_depth": 3}`)
		r := RunVector(Vector{Name: "x", Operation: op, Input: input})
		if r.Status != StatusSkip || !strings.Contains(r.Reason, "declared_max_depth") || !strings.Contains(r.Reason, op) {
			t.Errorf("%s: status = %v (%s), want an E-20a skip naming the key and the operation", op, r.Status, r.Reason)
		}
	}
}

// Keys that merely start like a declared key but are not (no declared_ prefix)
// and inputs that are not JSON objects are not E-20a's concern.
func TestDeclaredKeyScanIgnoresOtherShapes(t *testing.T) {
	if reason := unhonouredDeclaredKey("parse", []byte(`{"format": "json", "undeclared_max": 1, "text": "1"}`)); reason != "" {
		t.Errorf("a key without the declared_ prefix gave %q", reason)
	}
	if reason := unhonouredDeclaredKey("parse", []byte(`[1, 2]`)); reason != "" {
		t.Errorf("a non-object input gave %q", reason)
	}
	if reason := unhonouredDeclaredKey("parse", nil); reason != "" {
		t.Errorf("no input gave %q", reason)
	}
	for _, k := range []string{"declared_max_depth", "declared_max_nodes", "declared_max_int_digits", "declared_max_alias_expansion", "declared_max_expanded_slots", "declared_max_input_bytes"} {
		if reason := unhonouredDeclaredKey("parse", []byte(`{"`+k+`": 1}`)); reason != "" {
			t.Errorf("parse honours %s but gave %q", k, reason)
		}
	}
}
