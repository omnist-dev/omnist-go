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

func TestRunVectorSkipsMaterialize(t *testing.T) {
	v := Vector{Name: "x", Operation: "materialize"}
	res := RunVector(v)
	if res.Status != StatusSkip {
		t.Fatalf("want skip, got %v", res.Status)
	}
	if res.Reason == "" {
		t.Fatal("want a cited reason, per §8.5.5")
	}
}

func TestRunVectorUnknownOperationFails(t *testing.T) {
	v := Vector{Name: "x", Operation: "not-a-real-operation"}
	res := RunVector(v)
	if res.Status != StatusFail {
		t.Fatalf("want fail, got %v", res.Status)
	}
}
