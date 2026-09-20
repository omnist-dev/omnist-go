package omnist_test

// Spec §2.5's byte-order-mark rules (D-15, D-21), pinned across all six read
// surfaces and every writer. The mark is only ever written here as an escape
// (bom below): a raw U+FEFF in source is invisible to a reader and to grep,
// which is how an open-coded second strip hides, and
// TestNoRawByteOrderMarkInTrackedFiles fails the build if one appears.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
	"github.com/omnist-dev/omnist-go/osd"
)

const bom = "\uFEFF"

func TestStripLeadingBOM(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"none", "a: 1", "a: 1", false},
		{"empty", "", "", false},
		{"single", bom + "a: 1", "a: 1", false},
		{"only a mark", bom, "", false},
		{"double", bom + bom + "a: 1", "", true},
		{"triple", bom + bom + bom + "a: 1", "", true},
		{"double and nothing else", bom + bom, "", true},
		{"interior is content", "a" + bom + "b", "a" + bom + "b", false},
		{"after whitespace is content", " " + bom + "a", " " + bom + "a", false},
		{"one leading and one interior", bom + "a" + bom, "a" + bom, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := omnist.StripLeadingBOM(tc.in, omnist.CodeParseCodecSyntax)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("StripLeadingBOM(%q) = %q, want an error", tc.in, got)
				}
				if err.Path != "1:1" || err.Line != 1 || err.Col != 1 || err.Code != omnist.CodeParseCodecSyntax {
					t.Errorf("error = %#v, want 1:1 parse.codec-syntax", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("StripLeadingBOM(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
		})
	}
}

// surface is one read surface, reduced to "text in, Document (or error) out".
type surface struct {
	name     string
	body     string
	doubleOK omnist.Code // code D-21 requires for a second leading mark
	read     func(string) (omnist.Document, error)
}

func surfaces() []surface {
	l := omnist.DefaultLimits()
	return []surface{
		{"oml", "a: 1\n", omnist.CodeParseUnexpectedToken, func(s string) (omnist.Document, error) { return oml.Read(s, l) }},
		{"json", `{"a":1}`, omnist.CodeParseCodecSyntax, func(s string) (omnist.Document, error) { return json.Read(s, l) }},
		{"yaml", "a: 1\n", omnist.CodeParseCodecSyntax, func(s string) (omnist.Document, error) { return yaml.Read(s, l) }},
		{"toml", "a = 1\n", omnist.CodeParseCodecSyntax, func(s string) (omnist.Document, error) { return toml.Read(s, l) }},
		{"xml", "<root><a>1</a></root>", omnist.CodeParseCodecSyntax, func(s string) (omnist.Document, error) {
			d, _, err := xml.Read(s, l)
			return d, err
		}},
	}
}

func TestEveryDocumentSurfaceStripsOneLeadingBOMAndRejectsASecond(t *testing.T) {
	for _, s := range surfaces() {
		t.Run(s.name, func(t *testing.T) {
			plain, err := s.read(s.body)
			if err != nil {
				t.Fatalf("baseline read failed: %v", err)
			}
			withBOM, err := s.read(bom + s.body)
			if err != nil {
				t.Fatalf("single leading mark must be stripped, got %v", err)
			}
			if !omnist.DocumentsEqual(plain, withBOM) {
				t.Errorf("a leading mark changed the Document")
			}
			for _, in := range []string{bom + bom + s.body, bom + bom + bom + s.body} {
				_, err := s.read(in)
				pe, ok := err.(*omnist.ParseError)
				if !ok || pe.Code != s.doubleOK || pe.Path != "1:1" {
					t.Errorf("%d marks: got %#v, want %s at 1:1", strings.Count(in, bom), err, s.doubleOK)
				}
			}
		})
	}
}

func TestOSDStripsOneLeadingBOMAndRejectsASecond(t *testing.T) {
	const body = "record R {\n    \"a\": string,\n}\nroot R\n"
	plain, err := osd.Read(body)
	if err != nil {
		t.Fatal(err)
	}
	withBOM, err := osd.Read(bom + body)
	if err != nil {
		t.Fatalf("single leading mark must be stripped, got %v", err)
	}
	if !omnist.SchemasEqual(plain, withBOM, omnist.ModeExact) {
		t.Errorf("a leading mark changed the Schema")
	}
	_, err = osd.Read(bom + bom + body)
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseUnexpectedToken || pe.Path != "1:1" {
		t.Errorf("got %#v, want parse.unexpected-token at 1:1", err)
	}
}

// D-21: only the mark at offset zero is special. Anywhere else it is an
// ordinary codepoint that must survive, byte for byte, in a label and a value.
func TestInteriorBOMIsPreservedOnEverySurface(t *testing.T) {
	l := omnist.DefaultLimits()
	label, value := "a"+bom+"b", "x"+bom+"y"
	cases := map[string]func() (omnist.Document, error){
		"oml":  func() (omnist.Document, error) { return oml.Read(`"`+label+`": "`+value+`"`+"\n", l) },
		"json": func() (omnist.Document, error) { return json.Read(`{"`+label+`": "`+value+`"}`, l) },
		"yaml": func() (omnist.Document, error) { return yaml.Read(label+": "+value+"\n", l) },
		"toml": func() (omnist.Document, error) { return toml.Read(`"`+label+`" = "`+value+`"`+"\n", l) },
		"xml": func() (omnist.Document, error) {
			// U+FEFF is not an XML name character, so only the text survives there.
			d, _, err := xml.Read("<root><a>"+value+"</a></root>", l)
			return d, err
		},
	}
	for name, read := range cases {
		t.Run(name, func(t *testing.T) {
			d, err := read()
			if err != nil {
				t.Fatal(err)
			}
			got, _ := oml.Write(d, true)
			if !strings.Contains(got, value) {
				t.Errorf("value lost its interior mark: %q", got)
			}
			if name != "xml" && !strings.Contains(got, label) {
				t.Errorf("label lost its interior mark: %q", got)
			}
		})
	}
}

// A writer MUST NOT emit a byte-order mark (D-15), whatever the Document
// holds -- including a first label and first value that themselves begin with
// U+FEFF, the case where a careless writer would emit one as the first
// character of its output. YAML is the one where an unquoted key would.
func TestWritersNeverEmitABOM(t *testing.T) {
	str := func(s string) omnist.Value { return omnist.ScalarValue(omnist.NewStringScalar(s)) }
	docs := map[string]omnist.Document{
		"plain":            omnist.NodeDocument(omnist.NewNode().AddValue("a", str("x"))),
		"label leads BOM":  omnist.NodeDocument(omnist.NewNode().AddValue(bom+"a", str("x"))),
		"value leads BOM":  omnist.NodeDocument(omnist.NewNode().AddValue("a", str(bom+"x"))),
		"nested leads BOM": omnist.NodeDocument(omnist.NewNode().AddNode("r", omnist.NewNode().AddValue(bom+"a", str(bom+"x")))),
	}
	writers := map[string]func(omnist.Document) (string, error){
		"oml":         func(d omnist.Document) (string, error) { s, _ := oml.Write(d, false); return s, nil },
		"oml compact": func(d omnist.Document) (string, error) { s, _ := oml.WriteCompact(d); return s, nil },
		"json":        func(d omnist.Document) (string, error) { s, _, err := json.Write(d); return s, err },
		"json strict": func(d omnist.Document) (string, error) { s, _, err := json.WriteStrict(d); return s, err },
		"yaml":        func(d omnist.Document) (string, error) { s, _, err := yaml.Write(d); return s, err },
		"toml":        func(d omnist.Document) (string, error) { s, _, err := toml.Write(d); return s, err },
		"xml":         func(d omnist.Document) (string, error) { s, _, err := xml.Write(d); return s, err },
	}
	for wn, w := range writers {
		for dn, d := range docs {
			t.Run(wn+"/"+dn, func(t *testing.T) {
				out, err := w(d)
				if err != nil {
					return // a refusal emits nothing, which is also fine
				}
				if strings.HasPrefix(out, bom) {
					t.Errorf("output begins with a byte-order mark: %q", out)
				}
			})
		}
	}
	schema, err := osd.Read("record R {\n    \"a\": string,\n}\nroot R\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		if out := osd.Write(schema, compact); strings.HasPrefix(out, bom) {
			t.Errorf("osd.Write(compact=%v) begins with a byte-order mark: %q", compact, out)
		}
	}
}

// TestNoRawByteOrderMarkInTrackedFiles fails if any file in the repository
// contains a raw U+FEFF (the bytes EF BB BF). A raw mark in source is
// invisible in an editor, in a diff and to grep for "BOM", which is how an
// open-coded strip escaped notice in two other ports' scanners; the mark is
// always written as the escape sequence instead. This file scans itself too:
// it spells the byte sequence with byte escapes, never the character.
//
// The vendor/ directory is the omnist-spec submodule, whose test vectors
// legitimately carry the character; it is not this repository's source.
func TestNoRawByteOrderMarkInTrackedFiles(t *testing.T) {
	raw := []byte("\xEF\xBB\xBF")
	scanned := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || path == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		if strings.Contains(string(data), string(raw)) {
			t.Errorf("%s contains a raw U+FEFF; write it as an escape (\\uFEFF) instead", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 50 {
		t.Fatalf("scanned only %d files; the walk is not seeing the repository", scanned)
	}
}

// U+FEFF is not whitespace to any scanner in this repository: after the one
// leading mark D-15 strips, a mark that is not at offset zero is content, so a
// scanner that skipped it as trivia would silently swallow it (as
// TypeScript's OSD scanner did). Each reader must therefore either reject a
// mark standing where only whitespace or punctuation may be, or carry it into
// a string.
func TestBOMIsNeverTreatedAsWhitespace(t *testing.T) {
	l := omnist.DefaultLimits()
	if _, err := oml.Read(" "+bom+"a: 1\n", l); err == nil {
		t.Error("oml: a mark after whitespace was swallowed")
	}
	if _, err := oml.Read(bom+" "+bom+"a: 1\n", l); err == nil {
		t.Error("oml: a mark after a stripped mark and whitespace was swallowed")
	}
	if _, err := osd.Read(" " + bom + "record R { } root R"); err == nil {
		t.Error("osd: a mark after whitespace was swallowed")
	}
	if _, err := osd.Read("record R { }" + bom + " root R"); err == nil {
		t.Error("osd: a mark between tokens was swallowed")
	}
	if _, err := json.Read(" "+bom+`{"a":1}`, l); err == nil {
		t.Error("json: a mark after whitespace was swallowed")
	}
	if _, err := toml.Read(" "+bom+"a = 1\n", l); err == nil {
		t.Error("toml: a mark after whitespace was swallowed")
	}
	if _, _, err := xml.Read(" "+bom+"<r/>", l); err == nil {
		t.Error("xml: a mark after whitespace was swallowed")
	}
	// YAML has no error to raise: a mark after a newline begins a plain key.
	d, err := yaml.Read("\n"+bom+"a: 1\n", l)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := oml.Write(d, true); !strings.Contains(got, bom+"a") {
		t.Errorf("yaml: the mark was dropped from the key: %q", got)
	}
}
