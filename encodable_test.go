package omnist

import (
	"math/big"
	"strings"
	"testing"
)

func encStr(s string) Value { return ScalarValue(NewStringScalar(s)) }

const badUTF8 = "\xff"

func TestCheckEncodable(t *testing.T) {
	tests := []struct {
		name      string
		doc       Document
		wantPath  string // "" means no error
		wantLabel bool
	}{
		{"empty document", NodeDocument(NewNode()), "", false},
		{"valid strings of every shape", NodeDocument(NewNode().
			AddValue("é", encStr("😀")).AddValue("n", NullValue()).
			AddValue("i", ScalarValue(NewIntegerScalar(big.NewInt(7)))).
			AddNode("g", NewNode().AddValue("�", encStr("�")))), "", false},
		{"bare scalar root, valid", ValueDocument(encStr("ok")), "", false},
		{"bare scalar root, bad", ValueDocument(encStr(badUTF8)), "$", false},
		{"bare null root", ValueDocument(NullValue()), "", false},
		{"bare non-string root", ValueDocument(ScalarValue(NewBooleanScalar(true))), "", false},
		{"top-level leaf", NodeDocument(NewNode().AddValue("a", encStr(badUTF8))), "$.a", false},
		{"top-level label", NodeDocument(NewNode().AddValue(badUTF8, encStr("ok"))), "$", true},
		{"nested leaf", NodeDocument(NewNode().AddNode("r", NewNode().AddValue("a", encStr(badUTF8)))), "$.r.a", false},
		{"nested label is at the holder", NodeDocument(NewNode().AddNode("r", NewNode().AddValue(badUTF8, encStr("ok")))), "$.r", true},
		{"a string under a bad label is at the holder", NodeDocument(NewNode().
			AddNode(badUTF8, NewNode().AddValue("x", encStr(badUTF8)))), "$", true},
		{"repeated leaf, first", NodeDocument(NewNode().AddValue("a", encStr(badUTF8)).AddValue("a", encStr("ok"))), "$.a[0]", false},
		{"repeated leaf, second", NodeDocument(NewNode().AddValue("a", encStr("ok")).AddValue("a", encStr(badUTF8))), "$.a[1]", false},
		{"single occurrence has no index", NodeDocument(NewNode().AddValue("a", encStr("ok")).AddValue("b", encStr(badUTF8))), "$.b", false},
		{"first bad one wins, in edge order", NodeDocument(NewNode().AddValue("a", encStr(badUTF8)).AddValue(badUTF8, encStr("ok"))), "$.a", false},
		{"a clean sibling subtree is skipped", NodeDocument(NewNode().
			AddNode("g", NewNode().AddValue("a", encStr("ok"))).
			AddNode("g", NewNode().AddValue("a", encStr(badUTF8)))), "$.g[1].a", false},
		{"repeated holder is indexed", NodeDocument(NewNode().
			AddNode("g", NewNode().AddValue("a", encStr("ok"))).
			AddNode("g", NewNode().AddValue(badUTF8, encStr("ok")))), "$.g[1]", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckEncodable(tc.doc)
			if tc.wantPath == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			d, ok := err.(Diagnostic)
			if !ok {
				t.Fatalf("got %v (%T), want a Diagnostic", err, err)
			}
			if d.Code != CodeWriteUnsupportedValue || d.Path != tc.wantPath || d.Severity != SeverityError {
				t.Errorf("got %s at %q, want %s at %q", d.Code, d.Path, CodeWriteUnsupportedValue, tc.wantPath)
			}
			if got := strings.Contains(d.Message, "edge label"); got != tc.wantLabel {
				t.Errorf("message %q: names a label = %v, want %v", d.Message, got, tc.wantLabel)
			}
		})
	}
}
