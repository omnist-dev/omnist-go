package omnist

import (
	"strings"
	"testing"
)

// --- round-trip property: the most valuable test category per the issue ---

func TestOSDRoundTripProperty(t *testing.T) {
	cases := []struct {
		name   string
		schema Schema
	}{
		{
			"single record, default cardinality, all scalar kinds plus any",
			Schema{
				Root: "R",
				Env: map[string]*Record{
					"R": {Name: "R", Fields: []Field{
						{Label: "a", Type: ScalarType(KindString, false)},
						{Label: "b", Type: ScalarType(KindInteger, true)},
						{Label: "c", Type: ScalarType(KindNumber, false)},
						{Label: "d", Type: ScalarType(KindBoolean, false)},
						{Label: "e", Type: ScalarType(KindDate, false)},
						{Label: "f", Type: ScalarType(KindTime, false)},
						{Label: "g", Type: ScalarType(KindDateTime, false)},
						{Label: "h", Type: AnyType()},
					}},
				},
				EnvOrder: []string{"R"},
			},
		},
		{
			"empty record",
			Schema{
				Root:     "R",
				Env:      map[string]*Record{"R": {Name: "R"}},
				EnvOrder: []string{"R"},
			},
		},
		{
			"nested references, mutual recursion",
			Schema{
				Root: "A",
				Env: map[string]*Record{
					"A": {Name: "A", Fields: []Field{
						{Label: "b", Type: RefType("B"), Cardinality: Cardinality{Min: 0, Unbounded: true}},
					}},
					"B": {Name: "B", Fields: []Field{
						{Label: "a", Type: RefType("A"), Cardinality: Cardinality{Min: 0, Max: 1}},
					}},
				},
				EnvOrder: []string{"A", "B"},
			},
		},
		{
			"every cardinality shape",
			Schema{
				Root: "R",
				Env: map[string]*Record{
					"R": {Name: "R", Fields: []Field{
						{Label: "default", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
						{Label: "exact", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 3, Max: 3}},
						{Label: "range", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 1, Max: 5}},
						{Label: "atleast", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 5, Unbounded: true}},
						{Label: "atmost", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 0, Max: 5}},
						{Label: "any_count", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 0, Unbounded: true}},
					}},
				},
				EnvOrder: []string{"R"},
			},
		},
		{
			"label needing escaping: quote, backslash, control char",
			Schema{
				Root: "R",
				Env: map[string]*Record{
					"R": {Name: "R", Fields: []Field{
						{Label: `a"b\c` + "\n" + "d", Type: ScalarType(KindString, false)},
					}},
				},
				EnvOrder: []string{"R"},
			},
		},
	}

	for _, tc := range cases {
		for _, compact := range []bool{false, true} {
			t.Run(tc.name, func(t *testing.T) {
				text := WriteOSD(tc.schema, compact)
				got, err := ReadOSD(text)
				if err != nil {
					t.Fatalf("compact=%v ReadOSD(WriteOSD(schema)) failed: %v\ntext:\n%s", compact, err, text)
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
	schema := Schema{
		Root: "R",
		Env: map[string]*Record{
			"R": {Name: "R", Fields: []Field{
				{Label: "a", Type: ScalarType(KindString, true), Cardinality: Cardinality{Min: 0, Max: 3}},
			}},
		},
		EnvOrder: []string{"R"},
	}
	want := "record R {\n    \"a\" [0,3]: string?,\n}\nroot R\n"
	if got := WriteOSD(schema, false); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestOSDCardinalityOneOneOmitted(t *testing.T) {
	schema := Schema{
		Root: "R",
		Env: map[string]*Record{
			"R": {Name: "R", Fields: []Field{
				{Label: "a", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
			}},
		},
		EnvOrder: []string{"R"},
	}
	text := WriteOSD(schema, false)
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
	schema := Schema{
		Root: "Second",
		Env: map[string]*Record{
			"First":  {Name: "First"},
			"Second": {Name: "Second"},
		},
		EnvOrder: []string{"First", "Second"},
	}
	text := WriteOSD(schema, false)
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
	schema := Schema{
		Root: "R",
		Env: map[string]*Record{
			"R": {Name: "R", Fields: []Field{
				{Label: "a", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
			}},
		},
		EnvOrder: []string{"R"},
	}
	want := `record R { "a": string } root R`
	if got := WriteOSD(schema, true); got != want {
		t.Errorf("got %q want %q", got, want)
	}
	got, err := ReadOSD(want)
	if err != nil {
		t.Fatalf("ReadOSD(compact example) failed: %v", err)
	}
	if !schemaEqual(schema, got) {
		t.Errorf("compact example did not round-trip: got %#v want %#v", got, schema)
	}
}

func TestOSDCompactEmptyRecord(t *testing.T) {
	schema := Schema{
		Root:     "R",
		Env:      map[string]*Record{"R": {Name: "R"}},
		EnvOrder: []string{"R"},
	}
	want := `record R {} root R`
	if got := WriteOSD(schema, true); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// --- §5.3.1 weak-unescape-in-reverse trap ---

func TestOSDLabelEscapingBackslashAndQuote(t *testing.T) {
	label := `contains \ backslash and " quote`
	schema := Schema{
		Root: "R",
		Env: map[string]*Record{
			"R": {Name: "R", Fields: []Field{{Label: label, Type: ScalarType(KindString, false)}}},
		},
		EnvOrder: []string{"R"},
	}
	text := WriteOSD(schema, false)
	got, err := ReadOSD(text)
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

	schema := Schema{
		Root: "R",
		Env: map[string]*Record{
			"R": {Name: "R", Fields: []Field{{Label: label, Type: ScalarType(KindString, false)}}},
		},
		EnvOrder: []string{"R"},
	}
	text := WriteOSD(schema, false)
	got, err := ReadOSD(text)
	if err != nil {
		t.Fatalf("ReadOSD failed: %v\ntext:\n%s", err, text)
	}
	if got.Env["R"].Fields[0].Label != label {
		t.Errorf("got label %q, want %q (text: %s)", got.Env["R"].Fields[0].Label, label, text)
	}
}

// --- §5.10 worked example: label containing \n unescapes to "anb" ---

func TestOSDReaderWeakUnescapeWorkedExample(t *testing.T) {
	s, err := ReadOSD(`record R { "a\nb": string } root R`)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Env["R"].Fields[0].Label; got != "anb" {
		t.Errorf("got %q, want %q", got, "anb")
	}
}
