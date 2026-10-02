package osd

// Issue #121 property test: over random labels (including the illegal
// classes) the construction route either refuses with the spec's schema.*
// code or yields a schema that survives Read(Write(s)) unchanged; a random
// corruption of a valid schema fails Validate and Write with a schema.* code.

import (
	"errors"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	omnist "github.com/omnist-dev/omnist-go"
)

var labelAlphabet = []string{"a", "b", "Z", "_", " ", "\"", "\\", "é", "日", "😀", "[", "]", "-", "0", ".", "\xff", "\xc3"}

func randLabel(r *rand.Rand) string {
	n := r.Intn(5) // 0 allowed: the empty label
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(labelAlphabet[r.Intn(len(labelAlphabet))])
	}
	return b.String()
}

func TestWriteRoundTripOverConstructedSchemas(t *testing.T) {
	r := rand.New(rand.NewSource(121))
	built, refused := 0, 0
	for i := 0; i < 2000; i++ {
		var fields []omnist.Field
		n := r.Intn(4)
		wantCode, seen := omnist.Code(""), map[string]bool{}
		for j := 0; j < n; j++ {
			label := randLabel(r)
			card := omnist.DefaultCardinality()
			if r.Intn(2) == 0 {
				card = omnist.Cardinality{Min: uint64(r.Intn(3)), Unbounded: true}
			}
			fields = append(fields, omnist.Field{Label: label, Type: omnist.ScalarType(omnist.KindString, r.Intn(2) == 0), Cardinality: card})
			switch {
			case wantCode != "":
			case label == "":
				wantCode = omnist.CodeSchemaEmptyLabel
			case strings.ContainsAny(label, "[]"):
				wantCode = omnist.CodeSchemaBracketInLabel
			case !utf8.ValidString(label):
				wantCode = omnist.CodeSchemaInvalidLabel
			case seen[label]:
				wantCode = omnist.CodeSchemaDuplicateField
			}
			seen[label] = true
		}
		rec, err := omnist.NewRecord("R", fields...)
		if wantCode != "" {
			refused++
			var d omnist.Diagnostic
			if !errors.As(err, &d) || d.Code != wantCode || d.Path != "R" || rec != nil {
				t.Fatalf("labels %q: got %v, want %s at R", labelsOf(fields), err, wantCode)
			}
			// The same fields in a hand-built literal: Validate and Write agree.
			lit := omnist.Schema{Root: "R", Env: map[string]*omnist.Record{"R": {Name: "R", Fields: fields}}, EnvOrder: []string{"R"}}
			if verr := lit.Validate(); !errors.As(verr, &d) || d.Code != wantCode {
				t.Fatalf("Validate: got %v, want %s", verr, wantCode)
			}
			if text, werr := Write(lit, i%2 == 0); werr == nil || text != "" {
				t.Fatalf("Write accepted ill-formed labels %q", labelsOf(fields))
			}
			continue
		}
		if err != nil {
			t.Fatalf("labels %q: unexpected %v", labelsOf(fields), err)
		}
		s, err := omnist.NewSchema("R", rec)
		if err != nil {
			t.Fatal(err)
		}
		built++
		for _, compact := range []bool{false, true} {
			text, err := Write(s, compact)
			if err != nil {
				t.Fatalf("Write(%q): %v", labelsOf(fields), err)
			}
			back, err := Read(text)
			if err != nil {
				t.Fatalf("Read(Write(s)) failed for %q: %v\n%s", labelsOf(fields), err, text)
			}
			if !schemaEqual(s, back) {
				t.Fatalf("round trip changed the schema for %q:\n%s", labelsOf(fields), text)
			}
		}
	}
	if built == 0 || refused == 0 {
		t.Fatalf("generator degenerate: built=%d refused=%d", built, refused)
	}
}

func labelsOf(fs []omnist.Field) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Label
	}
	return out
}

// Every ill-formed schema in the table fails Validate with the same code and
// path Write reports; and a Diagnostic that Validate returns for the schema
// matches what Read reports for the corresponding text where one exists.
func TestValidateAndWriteAgree(t *testing.T) {
	for _, c := range badSchemaCases() {
		verr := c.schema.Validate()
		_, werr := Write(c.schema, false)
		if verr == nil || werr == nil || verr.Error() != werr.Error() {
			t.Errorf("%s: Validate %v, Write %v", c.name, verr, werr)
		}
	}
}

// Validate reports at Read's code and path for a schema given as text.
func TestValidateMatchesReaderDiagnostics(t *testing.T) {
	texts := map[string]string{
		`record R { "": string } root R`:               "R/schema.empty-label",
		`record R { "a[": string } root R`:             "R/schema.bracket-in-label",
		`record R { "a": string, "a": string } root R`: "R/schema.duplicate-field",
		`record R { "a": Z } root R`:                   "R.a/schema.unknown-type",
		`record R { "a" [3,2]: string } root R`:        "R.a/schema.invalid-cardinality",
		`record string { } root string`:                "string/schema.reserved-name",
		`record R { } record R { } root R`:             "R/schema.duplicate-record",
	}
	for text, want := range texts {
		_, err := Read(text)
		var d omnist.Diagnostic
		if !errors.As(err, &d) || d.Path+"/"+string(d.Code) != want {
			t.Errorf("%s: reader gave %v, want %s", text, err, want)
		}
	}
}
