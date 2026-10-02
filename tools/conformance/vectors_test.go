package conformance

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// This file unit-tests the §8.5.4 canonical Document decoder directly,
// independent of running it against the real test-suite (which
// cmd/conformance's report is this package's primary verification, per
// issue #31's note that the runner's coverage can reasonably come from
// actually running it against the real vectors). These cases pin the
// decoder's handling of null and the two trickiest kinds (integer as a
// decimal string, and a temporal ISO-8601 string), which the real vector
// suite exercises but not always in isolation.

func TestDecodeCanonicalDocumentScalar(t *testing.T) {
	doc, err := DecodeCanonicalDocument([]byte(`{"scalar": {"kind": "string", "value": "hi"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.IsNode {
		t.Fatal("want a bare-value Document")
	}
	if doc.Value.IsNull || doc.Value.Scalar.Kind != omnist.KindString || doc.Value.Scalar.Str != "hi" {
		t.Fatalf("got %+v", doc.Value)
	}
}

func TestDecodeCanonicalDocumentNull(t *testing.T) {
	doc, err := DecodeCanonicalDocument([]byte(`{"scalar": {"kind": null, "value": null}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !doc.Value.IsNull {
		t.Fatal("want IsNull true")
	}
}

func TestDecodeCanonicalDocumentIntegerAsDecimalString(t *testing.T) {
	doc, err := DecodeCanonicalDocument([]byte(`{"scalar": {"kind": "integer", "value": "123456789012345678901234567890"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Value.Scalar.Kind != omnist.KindInteger {
		t.Fatalf("want KindInteger, got %v", doc.Value.Scalar.Kind)
	}
	if doc.Value.Scalar.Int.String() != "123456789012345678901234567890" {
		t.Fatalf("got %s", doc.Value.Scalar.Int.String())
	}
}

func TestDecodeCanonicalDocumentEdgesPreserveOrderAndRepetition(t *testing.T) {
	doc, err := DecodeCanonicalDocument([]byte(`{"edges": [["tag", {"scalar": {"kind": "string", "value": "x"}}], ["tag", {"scalar": {"kind": "string", "value": "y"}}]]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !doc.IsNode || len(doc.Node.Edges) != 2 {
		t.Fatalf("got %+v", doc)
	}
	if doc.Node.Edges[0].Label != "tag" || doc.Node.Edges[1].Label != "tag" {
		t.Fatalf("want two repeated 'tag' edges, got %+v", doc.Node.Edges)
	}
	v0, _ := doc.Node.Edges[0].Target.Value()
	v1, _ := doc.Node.Edges[1].Target.Value()
	if v0.Scalar.Str != "x" || v1.Scalar.Str != "y" {
		t.Fatalf("got %q, %q", v0.Scalar.Str, v1.Scalar.Str)
	}
}

func TestDecodeCanonicalDocumentTemporalKinds(t *testing.T) {
	cases := []struct {
		json string
		kind omnist.ScalarKind
	}{
		{`{"scalar": {"kind": "date", "value": "2024-01-01"}}`, omnist.KindDate},
		{`{"scalar": {"kind": "time", "value": "12:00:00"}}`, omnist.KindTime},
		{`{"scalar": {"kind": "datetime", "value": "2024-01-01T12:30:00"}}`, omnist.KindDateTime},
	}
	for _, c := range cases {
		doc, err := DecodeCanonicalDocument([]byte(c.json))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.json, err)
		}
		if doc.Value.Scalar.Kind != c.kind {
			t.Fatalf("%s: got kind %v want %v", c.json, doc.Value.Scalar.Kind, c.kind)
		}
	}
	dt, err := DecodeCanonicalDocument([]byte(`{"scalar": {"kind": "datetime", "value": "2024-01-01T12:30:00"}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.DateTimeValue{
		Date: omnist.DateValue{Year: 2024, Month: 1, Day: 1},
		Time: omnist.TimeValue{Hour: 12, Minute: 30, Second: 0},
	}
	if dt.Value.Scalar.DateTime != want {
		t.Fatalf("got %+v want %+v", dt.Value.Scalar.DateTime, want)
	}
}

// TestRunVectorMaterializeBadInputFails confirms materialize is wired up
// as a real driver (issue #35): a vector with no input.schema/document is
// a driver-level decode failure, not the "not yet implemented" skip this
// operation used to report before issue #35 implemented Materialize.
func TestRunVectorMaterializeBadInputFails(t *testing.T) {
	v := Vector{Name: "x", Operation: "materialize", Input: []byte(`{}`)}
	res := RunVector(v)
	if res.Status != StatusFail {
		t.Fatalf("want fail, got %v (%s)", res.Status, res.Reason)
	}
	if res.Reason == "" {
		t.Fatal("want a reason describing the decode failure")
	}
}

func TestRunVectorUnknownOperationFails(t *testing.T) {
	v := Vector{Name: "x", Operation: "not-a-real-operation"}
	res := RunVector(v)
	if res.Status != StatusFail {
		t.Fatalf("want fail, got %v", res.Status)
	}
}

// A vector carrying declared_max_alias_expansion (test-suite/README.md's
// fourth declared-limit key) is RUN, with that value passed to the reader as
// Limits.MaxAliasExpansion and with no other vector affected. The chain below
// has E(c) = 21/5 = 4.20: declared 5 accepts it, declared 4 rejects it with
// document.limit.alias-expansion at "$", and the same input with no key runs
// under the default of 50 and is accepted.
func TestAliasExpansionLimitKeyIsRunNotSkipped(t *testing.T) {
	const text = `"a: &a leaf\nb: &b {p: *a, q: *a, r: *a, s: *a}\nc: &c {p: *b, q: *b, r: *b, s: *b}\n"`
	input := func(limit string) []byte {
		if limit == "" {
			return []byte(`{"format": "yaml", "text": ` + text + `}`)
		}
		return []byte(`{"format": "yaml", "declared_max_alias_expansion": ` + limit + `, "text": ` + text + `}`)
	}
	// Every case expects the rejection, so a run that accepts is a FAIL and a
	// run that rejects is a PASS: the status pins which way the reader went.
	reject := []byte(`{"ok": false, "diagnostics": [{"path": "$", "code": "document.limit.alias-expansion"}]}`)
	for _, tc := range []struct {
		limit string
		want  Status
	}{{"4", StatusPass}, {"5", StatusFail}, {"", StatusFail}} {
		r := RunVector(Vector{Name: "formats-yaml/alias-expansion/x", Operation: "parse", Input: input(tc.limit), Expect: reject})
		if r.Status != tc.want {
			t.Errorf("limit %q: status = %v (%s), want %v", tc.limit, r.Status, r.Reason, tc.want)
		}
	}
}

// A TOML write vector with strict:true is not skippable: TOML's failures are
// unconditional, so the strict flag changes nothing and the vector runs.
func TestTOMLStrictWriteVectorRuns(t *testing.T) {
	v := Vector{
		Name:      "x",
		Operation: "write",
		Input:     []byte(`{"format": "toml", "strict": true, "document": {"edges": [["n", {"scalar": {"kind": null, "value": null}}]]}}`),
		Expect:    []byte(`{"ok": false, "diagnostics": [{"path": "$.n", "code": "write.unsupported-value"}]}`),
	}
	if r := RunVector(v); r.Status != StatusPass {
		t.Errorf("status = %v (%s), want pass", r.Status, r.Reason)
	}
}

// A vector carrying declared_max_expanded_slots (D-22) is RUN, with that value
// passed to the reader as Limits.MaxExpandedSlots and for no other vector. The
// text expands to 22 slots: declared 21 rejects it with
// document.limit.expanded-size at "$", declared 22 accepts it, and the same
// input with no key runs under the default of 1,000,000 and is accepted.
func TestExpandedSlotsLimitKeyIsRunNotSkipped(t *testing.T) {
	const text = `"base: &base {k1: 1, k2: 2, k3: 3}\nt: {a: *base, b: *base, c: *base, d: *base}\n"`
	input := func(limit string) []byte {
		if limit == "" {
			return []byte(`{"format": "yaml", "text": ` + text + `}`)
		}
		return []byte(`{"format": "yaml", "declared_max_expanded_slots": ` + limit + `, "text": ` + text + `}`)
	}
	reject := []byte(`{"ok": false, "diagnostics": [{"path": "$", "code": "document.limit.expanded-size"}]}`)
	for _, tc := range []struct {
		limit string
		want  Status
	}{{"21", StatusPass}, {"22", StatusFail}, {"", StatusFail}} {
		r := RunVector(Vector{Name: "formats-yaml/alias-expansion/x", Operation: "parse", Input: input(tc.limit), Expect: reject})
		if r.Status != tc.want {
			t.Errorf("limit %q: status = %v (%s), want %v", tc.limit, r.Status, r.Reason, tc.want)
		}
	}
}
