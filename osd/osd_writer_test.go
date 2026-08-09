package osd

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// --- round-trip property: the most valuable test category per the issue ---

func TestOSDRoundTripProperty(t *testing.T) {
	cases := []struct {
		name   string
		schema omnist.Schema
	}{
		{
			"single record, default cardinality, all scalar kinds plus any",
			omnist.Schema{
				Root: "R",
				Env: map[string]*omnist.Record{
					"R": {Name: "R", Fields: []omnist.Field{
						{Label: "a", Type: omnist.ScalarType(omnist.KindString, false)},
						{Label: "b", Type: omnist.ScalarType(omnist.KindInteger, true)},
						{Label: "c", Type: omnist.ScalarType(omnist.KindNumber, false)},
						{Label: "d", Type: omnist.ScalarType(omnist.KindBoolean, false)},
						{Label: "e", Type: omnist.ScalarType(omnist.KindDate, false)},
						{Label: "f", Type: omnist.ScalarType(omnist.KindTime, false)},
						{Label: "g", Type: omnist.ScalarType(omnist.KindDateTime, false)},
						{Label: "h", Type: omnist.AnyType()},
					}},
				},
				EnvOrder: []string{"R"},
			},
		},
		{
			"empty record",
			omnist.Schema{
				Root:     "R",
				Env:      map[string]*omnist.Record{"R": {Name: "R"}},
				EnvOrder: []string{"R"},
			},
		},
		{
			"nested references, mutual recursion",
			omnist.Schema{
				Root: "A",
				Env: map[string]*omnist.Record{
					"A": {Name: "A", Fields: []omnist.Field{
						{Label: "b", Type: omnist.RefType("B"), Cardinality: omnist.Cardinality{Min: 0, Unbounded: true}},
					}},
					"B": {Name: "B", Fields: []omnist.Field{
						{Label: "a", Type: omnist.RefType("A"), Cardinality: omnist.Cardinality{Min: 0, Max: 1}},
					}},
				},
				EnvOrder: []string{"A", "B"},
			},
		},
		{
			"every cardinality shape",
			omnist.Schema{
				Root: "R",
				Env: map[string]*omnist.Record{
					"R": {Name: "R", Fields: []omnist.Field{
						{Label: "default", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
						{Label: "exact", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 3, Max: 3}},
						{Label: "range", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 1, Max: 5}},
						{Label: "atleast", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 5, Unbounded: true}},
						{Label: "atmost", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 0, Max: 5}},
						{Label: "any_count", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.Cardinality{Min: 0, Unbounded: true}},
					}},
				},
				EnvOrder: []string{"R"},
			},
		},
		{
			"label needing escaping: quote, backslash, control char",
			omnist.Schema{
				Root: "R",
				Env: map[string]*omnist.Record{
					"R": {Name: "R", Fields: []omnist.Field{
						{Label: `a"b\c` + "\n" + "d", Type: omnist.ScalarType(omnist.KindString, false)},
					}},
				},
				EnvOrder: []string{"R"},
			},
		},
	}

	for _, tc := range cases {
		for _, compact := range []bool{false, true} {
			t.Run(tc.name, func(t *testing.T) {
				text := Write(tc.schema, compact)
				got, err := Read(text)
				if err != nil {
					t.Fatalf("compact=%v Read(Write(schema)) failed: %v\ntext:\n%s", compact, err, text)
				}
				if !schemaEqual(tc.schema, got) {
					t.Fatalf("compact=%v round-trip mismatch\ntext:\n%s\ngot:  %#v\nwant: %#v", compact, text, got, tc.schema)
				}
			})
		}
	}
}

// --- §5.9 canonical form: the worked example, byte-for-byte ---

func TestOSDCanonicalFormMatchesWorkedExample(t *testing.T) {
	schema := omnist.Schema{
		Root: "R",
		Env: map[string]*omnist.Record{
			"R": {Name: "R", Fields: []omnist.Field{
				{Label: "a", Type: omnist.ScalarType(omnist.KindString, true), Cardinality: omnist.Cardinality{Min: 0, Max: 3}},
			}},
		},
		EnvOrder: []string{"R"},
	}
	want := "record R {\n    \"a\" [0,3]: string?,\n}\nroot R\n"
	if got := Write(schema, false); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestOSDCardinalityOneOneOmitted(t *testing.T) {
	schema := omnist.Schema{
		Root: "R",
		Env: map[string]*omnist.Record{
			"R": {Name: "R", Fields: []omnist.Field{
				{Label: "a", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
			}},
		},
		EnvOrder: []string{"R"},
	}
	text := Write(schema, false)
	if strings.Contains(text, "[") {
		t.Errorf("default [1,1] cardinality should be omitted entirely, got %q", text)
	}
	want := "record R {\n    \"a\": string,\n}\nroot R\n"
	if text != want {
		t.Errorf("got %q want %q", text, want)
	}
}

func TestOSDRootAlwaysLast(t *testing.T) {
	// EnvOrder deliberately does not put the root record first, to check
	// that "root" is emitted last regardless of EnvOrder.
	schema := omnist.Schema{
		Root: "Second",
		Env: map[string]*omnist.Record{
			"First":  {Name: "First"},
			"Second": {Name: "Second"},
		},
		EnvOrder: []string{"First", "Second"},
	}
	text := Write(schema, false)
	rootIdx := strings.Index(text, "root Second")
	if rootIdx == -1 {
		t.Fatalf("root declaration not found: %q", text)
	}
	firstIdx := strings.Index(text, "record First")
	secondIdx := strings.Index(text, "record Second")
	if rootIdx < firstIdx || rootIdx < secondIdx {
		t.Errorf("root must be emitted last: %q", text)
	}
}

// --- §5.9 compact mode ---

func TestOSDCompactModeMatchesSpecExample(t *testing.T) {
	schema := omnist.Schema{
		Root: "R",
		Env: map[string]*omnist.Record{
			"R": {Name: "R", Fields: []omnist.Field{
				{Label: "a", Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()},
			}},
		},
		EnvOrder: []string{"R"},
	}
	want := `record R { "a": string } root R`
	if got := Write(schema, true); got != want {
		t.Errorf("got %q want %q", got, want)
	}
	got, err := Read(want)
	if err != nil {
		t.Fatalf("Read(compact example) failed: %v", err)
	}
	if !schemaEqual(schema, got) {
		t.Errorf("compact example did not round-trip: got %#v want %#v", got, schema)
	}
}

func TestOSDCompactEmptyRecord(t *testing.T) {
	schema := omnist.Schema{
		Root:     "R",
		Env:      map[string]*omnist.Record{"R": {Name: "R"}},
		EnvOrder: []string{"R"},
	}
	want := `record R {} root R`
	if got := Write(schema, true); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// --- §5.3.1 weak-unescape-in-reverse trap ---

func TestOSDLabelEscapingBackslashAndQuote(t *testing.T) {
	label := `contains \ backslash and " quote`
	schema := omnist.Schema{
		Root: "R",
		Env: map[string]*omnist.Record{
			"R": {Name: "R", Fields: []omnist.Field{{Label: label, Type: omnist.ScalarType(omnist.KindString, false)}}},
		},
		EnvOrder: []string{"R"},
	}
	text := Write(schema, false)
	got, err := Read(text)
	if err != nil {
		t.Fatalf("ReadOSD failed: %v\ntext:\n%s", err, text)
	}
	if got.Env["R"].Fields[0].Label != label {
		t.Errorf("got label %q, want %q (text: %s)", got.Env["R"].Fields[0].Label, label, text)
	}
}

// TestOSDLabelEscapingControlCharacterTrap directly exercises the trap the
// issue calls out: escaping a literal newline as the two-character
// sequence \n would (per §5.3.1's weak, non-named-escape unescaping rule)
// read back as the literal letter 'n', not a newline. The correct
// approach — what escapeOSDLabel actually does — is to escape a raw
// control character as a backslash immediately followed by that same
// literal control byte, relying on the reader's "whatever follows a
// backslash is written verbatim" behavior.
//
// The OSD STRING token's ABNF does permit a literal newline after a
// backslash (any escaped code point is accepted per §5.3.1), so this
// case does apply and is not a "doesn't exist in this grammar" edge case
// to merely note.
func TestOSDLabelEscapingControlCharacterTrap(t *testing.T) {
	label := "line one\nline two"
	escaped := escapeOSDLabel(label)

	if strings.Contains(escaped, `\n`) {
		t.Fatalf("escapeOSDLabel used the named-escape spelling \\n, which the OSD reader "+
			"decodes as the literal letter 'n' per §5.3.1, not a newline: %q", escaped)
	}
	if !strings.Contains(escaped, "\\\n") {
		t.Fatalf("escapeOSDLabel should emit a backslash immediately followed by the literal "+
			"newline byte, got %q", escaped)
	}

	schema := omnist.Schema{
		Root: "R",
		Env: map[string]*omnist.Record{
			"R": {Name: "R", Fields: []omnist.Field{{Label: label, Type: omnist.ScalarType(omnist.KindString, false)}}},
		},
		EnvOrder: []string{"R"},
	}
	text := Write(schema, false)
	got, err := Read(text)
	if err != nil {
		t.Fatalf("ReadOSD failed: %v\ntext:\n%s", err, text)
	}
	if got.Env["R"].Fields[0].Label != label {
		t.Errorf("got label %q, want %q (text: %s)", got.Env["R"].Fields[0].Label, label, text)
	}
}

// --- §5.10 worked example: label containing \n unescapes to "anb" ---

func TestOSDReaderWeakUnescapeWorkedExample(t *testing.T) {
	s, err := Read(`record R { "a\nb": string } root R`)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Env["R"].Fields[0].Label; got != "anb" {
		t.Errorf("got %q, want %q", got, "anb")
	}
}
