package conformance

import (
	encjson "encoding/json"
	"fmt"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
	"github.com/omnist-dev/omnist-go/osd"
)

// This file holds one driver function per §8.5.3 operation, each parsing
// its vector's "input" per that section's table, calling the matching
// real omnist-go function, and comparing against "expect" per §8.5.2.

// --- parse ---

type parseInput struct {
	Format               string `json:"format"`
	Text                 string `json:"text"`
	DeclaredMaxDepth     *int   `json:"declared_max_depth"`
	DeclaredMaxNodes     *int   `json:"declared_max_nodes"`
	DeclaredMaxIntDigits *int   `json:"declared_max_int_digits"`
}

func limitsFromInput(in parseInput) omnist.Limits {
	l := omnist.DefaultLimits()
	if in.DeclaredMaxDepth != nil {
		l.MaxDepth = *in.DeclaredMaxDepth
	}
	if in.DeclaredMaxNodes != nil {
		l.MaxNodes = *in.DeclaredMaxNodes
	}
	if in.DeclaredMaxIntDigits != nil {
		l.MaxIntDigits = *in.DeclaredMaxIntDigits
	}
	return l
}

func readByFormat(format, text string, limits omnist.Limits) (omnist.Document, error) {
	switch format {
	case "oml":
		return oml.Read(text, limits)
	case "json":
		return json.Read(text, limits)
	case "yaml":
		return yaml.Read(text, limits)
	case "toml":
		return toml.Read(text, limits)
	case "xml":
		return xml.Read(text, limits)
	default:
		return omnist.Document{}, fmt.Errorf("unrecognized format %q", format)
	}
}

func runParse(v Vector) Result {
	var in parseInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	doc, rerr := readByFormat(in.Format, in.Text, limitsFromInput(in))
	wantOK := expectOK(expect)
	if rerr != nil {
		if wantOK {
			return fail(v, "expected ok, got error: %v", rerr)
		}
		wantDiags, err := decodeExpectDiagnostics(expect)
		if err != nil {
			return fail(v, "decode expect.diagnostics: %v", err)
		}
		got := []diagPair{singleErrDiag(rerr)}
		want := expectDiagPairs(wantDiags)
		if !diagnosticSetsEqual(got, want) {
			return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
		}
		return pass(v)
	}
	if !wantOK {
		return fail(v, "expected error, got ok document")
	}
	wantDocRaw, ok := expect["document"]
	if !ok {
		return pass(v) // no document to compare (shouldn't happen for parse, but not this driver's call to invent one)
	}
	wantDoc, err := DecodeCanonicalDocument(wantDocRaw)
	if err != nil {
		return fail(v, "decode expect.document: %v", err)
	}
	if !omnist.DocumentsEqual(doc, wantDoc) {
		return fail(v, "document mismatch")
	}
	return pass(v)
}

// --- parse_schema ---

type parseSchemaInput struct {
	Text string `json:"text"`
}

func runParseSchema(v Vector) Result {
	var in parseSchemaInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	_, serr := osd.Read(in.Text)
	wantOK := expectOK(expect)
	if serr != nil {
		if wantOK {
			return fail(v, "expected ok, got error: %v", serr)
		}
		wantDiags, err := decodeExpectDiagnostics(expect)
		if err != nil {
			return fail(v, "decode expect.diagnostics: %v", err)
		}
		got := []diagPair{singleErrDiag(serr)}
		want := expectDiagPairs(wantDiags)
		if !diagnosticSetsEqual(got, want) {
			return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
		}
		return pass(v)
	}
	if !wantOK {
		return fail(v, "expected error, got ok schema")
	}
	return pass(v)
}

// --- validate ---

type validateInput struct {
	Schema   string             `json:"schema"`
	Document encjson.RawMessage `json:"document"`
}

func runValidate(v Vector) Result {
	var in validateInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	schema, serr := osd.Read(in.Schema)
	if serr != nil {
		return fail(v, "input schema failed to parse: %v", serr)
	}
	doc, derr := DecodeCanonicalDocument(in.Document)
	if derr != nil {
		return fail(v, "decode input.document: %v", derr)
	}
	diags := omnist.Validate(doc, schema)
	wantOK := expectOK(expect)
	got := diagsToPairs(diags)
	if wantOK {
		if len(diags) != 0 {
			return fail(v, "expected ok, got diagnostics: %v", diagStrings(got))
		}
		return pass(v)
	}
	wantDiags, err := decodeExpectDiagnostics(expect)
	if err != nil {
		return fail(v, "decode expect.diagnostics: %v", err)
	}
	want := expectDiagPairs(wantDiags)
	if !diagnosticSetsEqual(got, want) {
		return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
	}
	return pass(v)
}

// --- materialize ---

// materializeInput mirrors validateInput -- §8.5.3's table gives
// materialize the same {schema, document} input shape as validate.
type materializeInput struct {
	Schema   string             `json:"schema"`
	Document encjson.RawMessage `json:"document"`
}

func runMaterialize(v Vector) Result {
	var in materializeInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	schema, serr := osd.Read(in.Schema)
	if serr != nil {
		return fail(v, "input schema failed to parse: %v", serr)
	}
	doc, derr := DecodeCanonicalDocument(in.Document)
	if derr != nil {
		return fail(v, "decode input.document: %v", derr)
	}
	got, diags, merr := omnist.Materialize(doc, schema)
	if merr != nil {
		return fail(v, "materialize: %v", merr)
	}
	wantOK := expectOK(expect)
	gotPairs := diagsToPairs(diags)
	if wantOK {
		if len(diags) != 0 {
			return fail(v, "expected ok, got diagnostics: %v", diagStrings(gotPairs))
		}
		wantDocRaw, ok := expect["document"]
		if !ok {
			return pass(v) // no document to compare
		}
		wantDoc, err := DecodeCanonicalDocument(wantDocRaw)
		if err != nil {
			return fail(v, "decode expect.document: %v", err)
		}
		if !omnist.DocumentsEqual(got, wantDoc) {
			return fail(v, "document mismatch")
		}
		return pass(v)
	}
	wantDiags, err := decodeExpectDiagnostics(expect)
	if err != nil {
		return fail(v, "decode expect.diagnostics: %v", err)
	}
	want := expectDiagPairs(wantDiags)
	if !diagnosticSetsEqual(gotPairs, want) {
		return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(gotPairs), diagStrings(want))
	}
	return pass(v)
}

// --- write ---

type writeInput struct {
	Format   string             `json:"format"`
	Document encjson.RawMessage `json:"document"`
	Strict   bool               `json:"strict"`
}

func runWrite(v Vector) Result {
	var in writeInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	doc, derr := DecodeCanonicalDocument(in.Document)
	if derr != nil {
		return fail(v, "decode input.document: %v", derr)
	}

	var text string
	var gotDiags []omnist.Diagnostic
	var werr error
	switch in.Format {
	case "json":
		if in.Strict {
			text, gotDiags, werr = json.WriteStrict(doc)
		} else {
			text, gotDiags, werr = json.Write(doc)
		}
	case "oml":
		text, gotDiags = oml.Write(doc, false)
	case "yaml":
		text, gotDiags, werr = yaml.Write(doc)
	case "toml":
		if in.Strict {
			return Result{Vector: v, Status: StatusSkip, Reason: "not yet implemented: WriteTOML has no strict-mode parameter in this repository (toml.Write(d Document) (string, []omnist.Diagnostic, error) only) -- flagged for a follow-up issue, not fixed here per issue #31's/#49's scope"}
		}
		text, gotDiags, werr = toml.Write(doc)
	case "xml":
		text, gotDiags, werr = xml.Write(doc)
	default:
		return fail(v, "unrecognized format %q", in.Format)
	}

	wantOK := expectOK(expect)
	if werr != nil {
		if wantOK {
			return fail(v, "expected ok, got error: %v", werr)
		}
		wantDiags, err := decodeExpectDiagnostics(expect)
		if err != nil {
			return fail(v, "decode expect.diagnostics: %v", err)
		}
		got := []diagPair{singleErrDiag(werr)}
		want := expectDiagPairs(wantDiags)
		if !diagnosticSetsEqual(got, want) {
			return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
		}
		return pass(v)
	}
	if !wantOK {
		return fail(v, "expected error, got ok write")
	}
	// write is the one operation where ok:true and diagnostics can
	// coexist (§8.5.3) -- since issue #49, every Write*/WriteStrict
	// function returns (string, []omnist.Diagnostic, error) (oml.Write:
	// (string, []omnist.Diagnostic), see its own doc comment for why it
	// has no error return), so a successful write's adjustment
	// diagnostics are compared here exactly like every other operation's.
	if wantDiags, err := decodeExpectDiagnostics(expect); err != nil {
		return fail(v, "decode expect.diagnostics: %v", err)
	} else if _, hasDiagsKey := expect["diagnostics"]; hasDiagsKey {
		got := diagsToPairs(gotDiags)
		want := expectDiagPairs(wantDiags)
		if !diagnosticSetsEqual(got, want) {
			return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
		}
	}
	wantText, ok := expect["text"]
	if !ok {
		return pass(v)
	}
	var wantTextStr string
	if err := encjson.Unmarshal(wantText, &wantTextStr); err != nil {
		return fail(v, "decode expect.text: %v", err)
	}
	if text != wantTextStr {
		return fail(v, "text mismatch: got %q want %q", text, wantTextStr)
	}
	return pass(v)
}

// --- compatible_with / equivalent ---

type abInput struct {
	A string `json:"a"`
	B string `json:"b"`
}

func runCompatibleWith(v Vector) Result {
	var in abInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	a, aerr := osd.Read(in.A)
	if aerr != nil {
		return fail(v, "input.a failed to parse: %v", aerr)
	}
	b, berr := osd.Read(in.B)
	if berr != nil {
		return fail(v, "input.b failed to parse: %v", berr)
	}
	got := algebra.CompatibleWith(a, b)
	return compareBoolResult(v, got)
}

func runEquivalent(v Vector) Result {
	var in abInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	a, aerr := osd.Read(in.A)
	if aerr != nil {
		return fail(v, "input.a failed to parse: %v", aerr)
	}
	b, berr := osd.Read(in.B)
	if berr != nil {
		return fail(v, "input.b failed to parse: %v", berr)
	}
	got := algebra.Equivalent(a, b)
	return compareBoolResult(v, got)
}

func compareBoolResult(v Vector, got bool) Result {
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	raw, ok := expect["result"]
	if !ok {
		return fail(v, "expect has no \"result\" field")
	}
	var want bool
	if err := encjson.Unmarshal(raw, &want); err != nil {
		return fail(v, "decode expect.result: %v", err)
	}
	if got != want {
		return fail(v, "result mismatch: got %v want %v", got, want)
	}
	return pass(v)
}

// --- normalize / prune ---

type schemaOnlyInput struct {
	Schema string `json:"schema"`
}

func runNormalize(v Vector) Result {
	var in schemaOnlyInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	s, serr := osd.Read(in.Schema)
	if serr != nil {
		return fail(v, "input.schema failed to parse: %v", serr)
	}
	got := osd.Write(algebra.Normalize(s), false)
	return compareCanonicalSchemaText(v, got)
}

func runPrune(v Vector) Result {
	var in schemaOnlyInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	s, serr := osd.Read(in.Schema)
	if serr != nil {
		return fail(v, "input.schema failed to parse: %v", serr)
	}
	got := osd.Write(algebra.Prune(s), false)
	return compareCanonicalSchemaText(v, got)
}

// compareCanonicalSchemaText compares got against expect.schema *byte for
// byte* (spec §9.2 "Algebra results ... compared as canonical OSD text
// byte for byte"), not via the referee -- normalize/prune's whole point is
// pinning canonical output text, so a structural-equality comparison here
// would under-test the very thing the vector exists to check.
func compareCanonicalSchemaText(v Vector, got string) Result {
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	raw, ok := expect["schema"]
	if !ok {
		return fail(v, "expect has no \"schema\" field")
	}
	var want string
	if err := encjson.Unmarshal(raw, &want); err != nil {
		return fail(v, "decode expect.schema: %v", err)
	}
	if got != want {
		return fail(v, "canonical schema text mismatch: got %q want %q", got, want)
	}
	return pass(v)
}

// --- is_empty ---

func runIsEmpty(v Vector) Result {
	var in schemaOnlyInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	s, serr := osd.Read(in.Schema)
	if serr != nil {
		return fail(v, "input.schema failed to parse: %v", serr)
	}
	got := algebra.IsEmpty(s)
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	raw, ok := expect["empty"]
	if !ok {
		return fail(v, "expect has no \"empty\" field")
	}
	var want bool
	if err := encjson.Unmarshal(raw, &want); err != nil {
		return fail(v, "decode expect.empty: %v", err)
	}
	if got != want {
		return fail(v, "empty mismatch: got %v want %v", got, want)
	}
	return pass(v)
}

// --- extract ---

type extractInput struct {
	Schema string   `json:"schema"`
	Keep   []string `json:"keep"`
}

func runExtract(v Vector) Result {
	var in extractInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	s, serr := osd.Read(in.Schema)
	if serr != nil {
		return fail(v, "input.schema failed to parse: %v", serr)
	}
	keep := make(map[string]bool, len(in.Keep))
	for _, k := range in.Keep {
		keep[k] = true
	}
	result, eerr := algebra.Extract(s, keep)
	wantOK := expectOK(expect)
	if eerr != nil {
		if wantOK {
			return fail(v, "expected ok, got error: %v", eerr)
		}
		wantDiags, err := decodeExpectDiagnostics(expect)
		if err != nil {
			return fail(v, "decode expect.diagnostics: %v", err)
		}
		got := []diagPair{singleErrDiag(eerr)}
		want := expectDiagPairs(wantDiags)
		if !diagnosticSetsEqual(got, want) {
			return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
		}
		return pass(v)
	}
	if !wantOK {
		return fail(v, "expected error, got ok extract")
	}
	wantRaw, ok := expect["schema"]
	if !ok {
		return fail(v, "expect has no \"schema\" field")
	}
	var wantText string
	if err := encjson.Unmarshal(wantRaw, &wantText); err != nil {
		return fail(v, "decode expect.schema: %v", err)
	}
	wantSchema, werr := osd.Read(wantText)
	if werr != nil {
		return fail(v, "expect.schema failed to parse: %v", werr)
	}
	// extract's output naming is spec-determined (§6.9), so ModeExact --
	// same reasoning as normalize/prune, but extract returns a Schema
	// value rather than pinning canonical text directly, so the referee
	// (not a byte comparison) is the right tool here.
	if !omnist.SchemasEqual(result, wantSchema, omnist.ModeExact) {
		return fail(v, "schema mismatch (exact mode): got %q want %q", osd.Write(result, false), wantText)
	}
	return pass(v)
}

// --- infer / infer_with_report ---

type inferInput struct {
	Samples  []string `json:"samples"`
	AllowAny bool     `json:"allow_any"`
}

func inferSampleDocs(samples []string) ([]omnist.Document, error) {
	docs := make([]omnist.Document, len(samples))
	for i, s := range samples {
		d, err := oml.Read(s, omnist.DefaultLimits())
		if err != nil {
			return nil, fmt.Errorf("sample %d: %w", i, err)
		}
		docs[i] = d
	}
	return docs, nil
}

func runInfer(v Vector) Result {
	var in inferInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	docs, derr := inferSampleDocs(in.Samples)
	if derr != nil {
		return fail(v, "decode input.samples: %v", derr)
	}
	result, ierr := algebra.Infer(docs, "Root", in.AllowAny)
	wantOK := expectOK(expect)
	if ierr != nil {
		if wantOK {
			return fail(v, "expected ok, got error: %v", ierr)
		}
		wantDiags, err := decodeExpectDiagnostics(expect)
		if err != nil {
			return fail(v, "decode expect.diagnostics: %v", err)
		}
		got := []diagPair{singleErrDiag(ierr)}
		want := expectDiagPairs(wantDiags)
		if !diagnosticSetsEqual(got, want) {
			return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
		}
		return pass(v)
	}
	if !wantOK {
		return fail(v, "expected error, got ok infer")
	}
	wantRaw, ok := expect["schema"]
	if !ok {
		return fail(v, "expect has no \"schema\" field")
	}
	var wantText string
	if err := encjson.Unmarshal(wantRaw, &wantText); err != nil {
		return fail(v, "decode expect.schema: %v", err)
	}
	wantSchema, werr := osd.Read(wantText)
	if werr != nil {
		return fail(v, "expect.schema failed to parse: %v", werr)
	}
	// infer never normalizes its output (§6.10), so ModeIsomorphic --
	// this is the operation the porting guide specifically calls out
	// isomorphic mode for.
	if !omnist.SchemasEqual(result, wantSchema, omnist.ModeIsomorphic) {
		return fail(v, "schema mismatch (isomorphic mode): got %q want %q", osd.Write(result, false), wantText)
	}
	return pass(v)
}

type fallbackExpect struct {
	Location string `json:"location"`
	Reason   string `json:"reason"`
}

func runInferWithReport(v Vector) Result {
	var in inferInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	docs, derr := inferSampleDocs(in.Samples)
	if derr != nil {
		return fail(v, "decode input.samples: %v", derr)
	}
	result, fallbacks, ierr := algebra.InferWithReport(docs, "Root", in.AllowAny)
	wantOK := expectOK(expect)
	if ierr != nil {
		if wantOK {
			return fail(v, "expected ok, got error: %v", ierr)
		}
		wantDiags, err := decodeExpectDiagnostics(expect)
		if err != nil {
			return fail(v, "decode expect.diagnostics: %v", err)
		}
		got := []diagPair{singleErrDiag(ierr)}
		want := expectDiagPairs(wantDiags)
		if !diagnosticSetsEqual(got, want) {
			return fail(v, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
		}
		return pass(v)
	}
	if !wantOK {
		return fail(v, "expected error, got ok infer_with_report")
	}
	wantRaw, ok := expect["schema"]
	if !ok {
		return fail(v, "expect has no \"schema\" field")
	}
	var wantText string
	if err := encjson.Unmarshal(wantRaw, &wantText); err != nil {
		return fail(v, "decode expect.schema: %v", err)
	}
	wantSchema, werr := osd.Read(wantText)
	if werr != nil {
		return fail(v, "expect.schema failed to parse: %v", werr)
	}
	if !omnist.SchemasEqual(result, wantSchema, omnist.ModeIsomorphic) {
		return fail(v, "schema mismatch (isomorphic mode): got %q want %q", osd.Write(result, false), wantText)
	}
	// fallbacks is always present on success (spec §8.5.3), compared as a
	// set of (location, reason) -- reason is prose but the spec's own
	// operation table lists it as part of the compared shape (unlike a
	// diagnostic's "message"), so this driver compares it exactly rather
	// than treating it as message text under §8.5.2 rule 1 (that rule
	// governs *diagnostics*, and a fallback report is not a diagnostic).
	fbRaw, ok := expect["fallbacks"]
	if !ok {
		return fail(v, "expect has no \"fallbacks\" field")
	}
	var wantFallbacks []fallbackExpect
	if err := encjson.Unmarshal(fbRaw, &wantFallbacks); err != nil {
		return fail(v, "decode expect.fallbacks: %v", err)
	}
	if len(fallbacks) != len(wantFallbacks) {
		return fail(v, "fallbacks count mismatch: got %d want %d", len(fallbacks), len(wantFallbacks))
	}
	gotSet := map[string]int{}
	for _, f := range fallbacks {
		gotSet[f.Location+"\x00"+f.Reason]++
	}
	for _, f := range wantFallbacks {
		k := f.Location + "\x00" + f.Reason
		if gotSet[k] == 0 {
			return fail(v, "fallbacks mismatch: missing {location:%q reason:%q}", f.Location, f.Reason)
		}
		gotSet[k]--
	}
	return pass(v)
}

// --- lint ---

func runLint(v Vector) Result {
	var in schemaOnlyInput
	if err := encjson.Unmarshal(v.Input, &in); err != nil {
		return fail(v, "decode input: %v", err)
	}
	expect, err := decodeExpect(v)
	if err != nil {
		return fail(v, "decode expect: %v", err)
	}
	s, serr := osd.Read(in.Schema)
	if serr != nil {
		return fail(v, "input.schema failed to parse: %v", serr)
	}
	findings := algebra.Lint(s)
	// lint.json's own vectors establish: ok is false iff any finding has
	// a severity other than info (lint/any-field-is-informational-not-a-
	// warning expects ok:true despite a nonempty findings list, since its
	// one finding is info-severity; the three warning-severity examples
	// all expect ok:false).
	gotOK := true
	for _, f := range findings {
		if f.Severity != omnist.SeverityInfo {
			gotOK = false
			break
		}
	}
	wantOK := expectOK(expect)
	if gotOK != wantOK {
		return fail(v, "ok mismatch: got %v want %v (findings: %v)", gotOK, wantOK, findings)
	}
	wantRaw, ok := expect["findings"]
	if !ok {
		return fail(v, "expect has no \"findings\" field")
	}
	var wantFindings []findingExpect
	if err := encjson.Unmarshal(wantRaw, &wantFindings); err != nil {
		return fail(v, "decode expect.findings: %v", err)
	}
	if len(findings) != len(wantFindings) {
		return fail(v, "findings count mismatch: got %d want %d", len(findings), len(wantFindings))
	}
	gotSet := map[string]int{}
	for _, f := range findings {
		gotSet[string(f.Code)+"\x00"+f.Severity.String()+"\x00"+f.Location]++
	}
	for _, f := range wantFindings {
		k := f.Code + "\x00" + f.Severity + "\x00" + f.Location
		if gotSet[k] == 0 {
			return fail(v, "findings mismatch: missing {code:%q severity:%q location:%q}", f.Code, f.Severity, f.Location)
		}
		gotSet[k]--
	}
	return pass(v)
}

func expectDiagPairs(ds []diagExpect) []diagPair {
	out := make([]diagPair, len(ds))
	for i, d := range ds {
		out[i] = diagPair(d)
	}
	return out
}
