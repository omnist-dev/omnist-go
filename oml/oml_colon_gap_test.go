package oml

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// §4.2.1 and oml.abnf: `edge = label skip COLON gap value`. Horizontal space
// and comments may stand before the colon, a newline or ';' may not (a SEP
// token would stand where the colon is owed). OML-29 is the other direction:
// a gap AFTER the colon is insignificant. omnist-go issue #129.
func TestReadSeparatorBeforeColonIsRejected(t *testing.T) {
	cases := []struct {
		name, in string
		code     omnist.Code
		path     string
	}{
		// At the top level the lookahead sees no label-colon pair, so the
		// label is a bare scalar word (or a string followed by leftovers).
		{"newline", "a\n: 1", omnist.CodeParseBareWord, "1:1"},
		{"semicolon", "a;: 1", omnist.CodeParseBareWord, "1:1"},
		{"comment then newline", "a #c\n: 1", omnist.CodeParseBareWord, "1:1"},
		{"CRLF", "a\r\n: 1", omnist.CodeParseBareWord, "1:1"},
		{"blank line", "a\n\n: 1", omnist.CodeParseBareWord, "1:1"},
		{"string label", "\"a\"\n: 1", omnist.CodeParseTrailingContent, "2:1"},
		// Inside braces and for a later top-level edge the label is known to
		// be a label, so the separator where the colon is owed is the error,
		// at the end of the label where the separator run starts.
		{"nested newline", "x: {a\n: 1}", omnist.CodeParseUnexpectedToken, "1:6"},
		{"nested semicolon", "x: {a;: 1}", omnist.CodeParseUnexpectedToken, "1:6"},
		{"nested spaces semicolon spaces", "x: {a ; : 1}", omnist.CodeParseUnexpectedToken, "1:6"},
		{"nested comment newline", "x: {a #c\n: 1}", omnist.CodeParseUnexpectedToken, "1:6"},
		{"nested spaces newline", "x: {a  \n: 1}", omnist.CodeParseUnexpectedToken, "1:6"},
		{"nested CRLF", "x: {a\r\n: 1}", omnist.CodeParseUnexpectedToken, "1:6"},
		{"nested blank lines and comment", "x: {a\n\n#c\n: 1}", omnist.CodeParseUnexpectedToken, "1:6"},
		{"nested string label", "x: {\"a\"\n: 1}", omnist.CodeParseUnexpectedToken, "1:8"},
		{"second top-level edge", "x: 1\na\n: 2", omnist.CodeParseUnexpectedToken, "2:2"},
		{"second top-level edge CRLF", "a: 1\r\nb\r\n: 2", omnist.CodeParseUnexpectedToken, "2:2"},
		{"after a semicolon inside braces", "x: {a: 1;\n b\n: 2}", omnist.CodeParseUnexpectedToken, "2:3"},
		// A lone CR is not a newline (§8.4 E-29, oml.abnf `newline`) and not
		// hspace, so it is never skipped trivia: it is rejected where it
		// stands, whichever gap it appears in.
		{"lone CR before colon, nested", "x: {a\r: 1}", omnist.CodeParseUnexpectedToken, "1:6"},
		// At the top level the lookahead token fails to lex, so the label is
		// read as a scalar first and the bare word is the first error.
		{"lone CR before colon, top level", "a\r: 1", omnist.CodeParseBareWord, "1:1"},
		{"two lone CRs before colon", "a\r\r: 1", omnist.CodeParseBareWord, "1:1"},
		{"lone CR before colon, string label", "\"a\"\r: 1", omnist.CodeParseUnexpectedToken, "1:4"},
		{"lone CR between edges", "a: 1\rb: 2", omnist.CodeParseUnexpectedToken, "1:5"},
		{"lone CR after colon", "a:\r1", omnist.CodeParseUnexpectedToken, "1:3"},
		{"lone CR after a value", "a: \"x\"\r", omnist.CodeParseUnexpectedToken, "1:7"},
		{"lone CR before a closing brace", "x: {a: 1\r}", omnist.CodeParseUnexpectedToken, "1:9"},
		{"lone CR at the end", "x: {a: 1}\r", omnist.CodeParseUnexpectedToken, "1:10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Read(tc.in, omnist.DefaultLimits())
			pe, ok := err.(*omnist.ParseError)
			if !ok {
				t.Fatalf("Read(%q) = %v, want a parse error", tc.in, err)
			}
			if pe.Code != tc.code || pe.Path != tc.path {
				t.Errorf("Read(%q): got %s at %s, want %s at %s", tc.in, pe.Code, pe.Path, tc.code, tc.path)
			}
		})
	}
}

// The neighbours that must keep working: horizontal space and a comment (to
// the next LF, a CR inside it included) before the colon, every gap shape
// after it (OML-29), and CRLF as a separator between edges.
func TestReadColonGapStillAccepted(t *testing.T) {
	cases := []struct {
		in    string
		edges int
	}{
		{"a : 1", 1},
		{"a\t:\t1", 1},
		{"a:\n1", 1},
		{"a: ;1", 1},
		{"a: # c\n1", 1},
		{"a: # c\r\n1", 1},
		{"a:\n\n1", 1},
		{"x: {a:\n1}", 1},
		{"a: 1\r\nb: 2", 2},
		{"a: 1 # c\rb: 2", 1}, // the comment runs to LF, swallowing the CR and b: 2
		{"# c\r a: 1", 0},
		{"a: 1;b: 2", 2},
		{"\"a\" : 1", 1},
	}
	for _, tc := range cases {
		doc, err := Read(tc.in, omnist.DefaultLimits())
		if err != nil {
			t.Errorf("Read(%q): %v", tc.in, err)
			continue
		}
		if got := len(doc.Node.Edges); got != tc.edges {
			t.Errorf("Read(%q) has %d top-level edges, want %d", tc.in, got, tc.edges)
		}
	}
}
