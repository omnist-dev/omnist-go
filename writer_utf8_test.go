package omnist_test

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
)

// C-9 (spec §7.3): every writer MUST fail, unconditionally, with
// write.unsupported-value on a string value or an edge label that has no UTF-8
// encoding (in Go: a byte sequence that is not well-formed UTF-8). The path is
// the Document path of the node holding the string; for a label, the node
// that holds the edge. No writer may substitute U+FFFD or emit an escape.

type utf8Writer struct {
	name  string
	write func(omnist.Document) (string, error)
	// bareScalar reports whether the format can write a bare scalar root.
	bareScalar bool
}

func utf8Writers() []utf8Writer {
	return []utf8Writer{
		{"json", func(d omnist.Document) (string, error) { s, _, err := json.Write(d); return s, err }, true},
		{"json strict", func(d omnist.Document) (string, error) { s, _, err := json.WriteStrict(d); return s, err }, true},
		{"yaml", func(d omnist.Document) (string, error) { s, _, err := yaml.Write(d); return s, err }, true},
		{"toml", func(d omnist.Document) (string, error) { s, _, err := toml.Write(d); return s, err }, false},
		{"xml", func(d omnist.Document) (string, error) { s, _, err := xml.Write(d); return s, err }, false},
		{"oml", func(d omnist.Document) (string, error) { s, _, err := oml.Write(d, false); return s, err }, true},
		{"oml compact", func(d omnist.Document) (string, error) { s, _, err := oml.WriteCompact(d); return s, err }, true},
	}
}

func str(s string) omnist.Value { return omnist.ScalarValue(omnist.NewStringScalar(s)) }

// under wraps inner as the one child of a root edge "r", the shape every
// format (XML's single document element included) can write.
func under(inner *omnist.Node) omnist.Document {
	return omnist.NodeDocument(omnist.NewNode().AddNode("r", inner))
}

func TestEveryWriterRefusesAStringWithNoUTF8Encoding(t *testing.T) {
	// Each of these is not well-formed UTF-8 and has no UTF-8 encoding.
	bad := map[string]string{
		"lone 0xff":                "\xff",
		"truncated 3-byte":         "ok\xe2\x82",
		"overlong NUL":             "\xc0\x80",
		"encoded surrogate U+D800": "\xed\xa0\x80",
		"stray continuation":       "\x80x",
	}
	type docCase struct {
		name string
		doc  func(b string) omnist.Document
		path string
	}
	docs := []docCase{
		{"leaf value", func(b string) omnist.Document {
			return under(omnist.NewNode().AddValue("a", str(b)))
		}, "$.r.a"},
		{"leaf value after a valid sibling", func(b string) omnist.Document {
			return under(omnist.NewNode().AddValue("x", str("fine")).AddValue("a", str(b)))
		}, "$.r.a"},
		{"second of a repeated label is indexed", func(b string) omnist.Document {
			return under(omnist.NewNode().AddValue("item", str("ok")).AddValue("item", str(b)))
		}, "$.r.item[1]"},
		{"first of a repeated label is indexed", func(b string) omnist.Document {
			return under(omnist.NewNode().AddValue("item", str(b)).AddValue("item", str("ok")))
		}, "$.r.item[0]"},
		{"nested below a repeated parent", func(b string) omnist.Document {
			return under(omnist.NewNode().
				AddNode("g", omnist.NewNode().AddValue("a", str("ok"))).
				AddNode("g", omnist.NewNode().AddValue("a", str(b))))
		}, "$.r.g[1].a"},
		{"label is reported at the node holding the edge", func(b string) omnist.Document {
			return under(omnist.NewNode().AddValue(b, str("ok")))
		}, "$.r"},
		{"label of a later edge, valid sibling first", func(b string) omnist.Document {
			return under(omnist.NewNode().AddValue("a", str("ok")).AddValue(b, str("ok")))
		}, "$.r"},
		{"a string beneath an unencodable label is reported at the holder", func(b string) omnist.Document {
			return under(omnist.NewNode().AddNode(b, omnist.NewNode().AddValue("x", str(b))))
		}, "$.r"},
		{"top-level label is reported at the root", func(b string) omnist.Document {
			return omnist.NodeDocument(omnist.NewNode().AddNode(b, omnist.NewNode().AddValue("x", str("ok"))))
		}, "$"},
		{"repeated holder is indexed", func(b string) omnist.Document {
			return under(omnist.NewNode().
				AddNode("g", omnist.NewNode().AddValue("a", str("ok"))).
				AddNode("g", omnist.NewNode().AddValue(b, str("ok"))))
		}, "$.r.g[1]"},
	}
	for _, w := range utf8Writers() {
		for bname, b := range bad {
			for _, dc := range docs {
				t.Run(w.name+"/"+bname+"/"+dc.name, func(t *testing.T) {
					out, err := w.write(dc.doc(b))
					d, ok := err.(omnist.Diagnostic)
					if !ok {
						t.Fatalf("got err %v (%T), want a write.unsupported-value Diagnostic; output %q", err, err, out)
					}
					if d.Code != omnist.CodeWriteUnsupportedValue || d.Path != dc.path || d.Severity != omnist.SeverityError {
						t.Errorf("got %s at %q (%v), want %s at %q", d.Code, d.Path, d.Severity, omnist.CodeWriteUnsupportedValue, dc.path)
					}
					if out != "" {
						t.Errorf("a failed write returned text %q", out)
					}
				})
			}
		}
	}
}

func TestBareScalarRootWithNoUTF8EncodingIsRefused(t *testing.T) {
	for _, w := range utf8Writers() {
		if !w.bareScalar {
			continue
		}
		t.Run(w.name, func(t *testing.T) {
			out, err := w.write(omnist.ValueDocument(str("\xff")))
			d, ok := err.(omnist.Diagnostic)
			if !ok || d.Code != omnist.CodeWriteUnsupportedValue || d.Path != "$" || out != "" {
				t.Errorf("got %q, %v; want write.unsupported-value at $", out, err)
			}
		})
	}
}

// No repair and no escape: valid text, including a literal U+FFFD that is
// itself well-formed, writes through unchanged.
func TestWritersAcceptEveryWellFormedString(t *testing.T) {
	good := []string{"", "plain", "é", "☃", "�", "😀", "a\u0000b"[:1], "tab\there"}
	for _, w := range utf8Writers() {
		for _, g := range good {
			t.Run(w.name+"/"+g, func(t *testing.T) {
				out, err := w.write(under(omnist.NewNode().AddValue("a", str(g))))
				if err != nil {
					t.Fatalf("a well-formed string was refused: %v", err)
				}
				if g == "�" && !strings.Contains(out, "�") {
					t.Errorf("a literal U+FFFD was altered: %q", out)
				}
			})
		}
	}
}
