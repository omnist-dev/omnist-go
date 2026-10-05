package oml

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// C-9 (spec §7.3): oml.Write and oml.WriteCompact refuses a string value or an edge label with no UTF-8
// encoding, with write.unsupported-value at the Document path of the node
// holding the string, and returns no text. The cross-format table lives in
// the root package (writer_utf8_test.go); this pins it for this package.
func TestWriteC9Refuses(t *testing.T) {
	str := func(s string) omnist.Value { return omnist.ScalarValue(omnist.NewStringScalar(s)) }
	tests := []struct {
		name string
		doc  omnist.Document
		path string
	}{
		{"value", omnist.NodeDocument(omnist.NewNode().AddNode("r", omnist.NewNode().AddValue("a", str("\xff")))), "$.r.a"},
		{"repeated value is indexed", omnist.NodeDocument(omnist.NewNode().AddNode("r", omnist.NewNode().AddValue("a", str("ok")).AddValue("a", str("\xff")))), "$.r.a[1]"},
		{"label is at the holder", omnist.NodeDocument(omnist.NewNode().AddNode("r", omnist.NewNode().AddValue("\xff", str("ok")))), "$.r"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := Write(tc.doc, false)
			if cout, _, cerr := WriteCompact(tc.doc); cout != "" || cerr == nil {
				t.Errorf("WriteCompact: got %q, %v", cout, cerr)
			}
			d, ok := err.(omnist.Diagnostic)
			if !ok || d.Code != omnist.CodeWriteUnsupportedValue || d.Path != tc.path || out != "" {
				t.Errorf("got %q, %v; want write.unsupported-value at %s and no text", out, err, tc.path)
			}
		})
	}
}
