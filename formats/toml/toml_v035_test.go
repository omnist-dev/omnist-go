package toml

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// C-12 / TOML 1.0: a table, key or array of tables defined twice, or defined
// as two different things, is rejected.
func TestConflictingDefinitionsAreRejected(t *testing.T) {
	bad := []string{
		"[a]\nb = 1\n[a.b]\n",    // value then table
		"a = 1\n[a]\n",           // value then table
		"[a]\n[a]\n",             // table twice
		"[a.b]\n[a]\n[a]\n",      // implicit then explicit twice
		"a.b = 1\n[a]\n",         // dotted-defined then header
		"a = {x = 1}\n[a]\n",     // inline then header
		"a = {x = 1}\n[a.b]\n",   // inline extended
		"a = [{x = 1}]\n[[a]]\n", // static array extended
		"a = [1]\n[[a]]\n",       // value array as array of tables
		"[[a]]\n[a]\n",           // array of tables then table
		"[a]\n[[a]]\n",           // table then array of tables
		"[a.b]\n[[a]]\n",         // implicit table then array of tables
		"[x]\n[y]\n[x]\n",        // redefinition after another table
		"[a]\nb.c = 1\n[a.b]\n",  // dotted-defined inside, then header
		"[a.b]\n[a]\nb.c = 1\n",
		"[t]\na = {x = 1}\n[t.a]\n",      // inline inside a table
		"[a]\n[b]\n[a.c]\n[b]\n",         // reopen
		"a.b = 1\na = 2\n",               // key over a dotted table
		"[[a]]\n[a.b]\n[[a.b]]\n[a.b]\n", // table over array of tables
		"x = 1\n[[x.y]]\n",               // array of tables through a value
	}
	for _, text := range bad {
		_, err := Read(text, omnist.DefaultLimits())
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeParseCodecSyntax {
			t.Errorf("Read(%q) error = %v, want parse.codec-syntax", text, err)
		}
	}
}

func TestLegalTableLayoutsStillRead(t *testing.T) {
	good := []string{
		"[a.b]\n[a]\nx = 1\n",              // implicit then explicit once
		"[a]\nb.c = 1\nb.d = 2\n",          // dotted keys extend a dotted table
		"[a.b]\nx = 1\n[a.c]\ny = 2\n",     // siblings
		"[[a]]\nx = 1\n[[a]]\nx = 2\n",     // array of tables
		"[[a]]\n[a.b]\n[[a]]\n[a.b]\n",     // sub-table of each element
		"[[a.b]]\n[[a.b]]\n",               // nested array of tables
		"a.b.c = 1\n[a.b.d]\ne = 2\n",      // header below a dotted table
		"a = {b = 1}\nc = {d = {e = 2}}\n", // inline tables
		"a = {b.c = 1, b.d = 2}\n",         // dotted keys inside an inline table
		"[a]\nb = 1\n[c]\nd = 2\n",
	}
	for _, text := range good {
		if _, err := Read(text, omnist.DefaultLimits()); err != nil {
			t.Errorf("Read(%q): %v", text, err)
		}
	}
}
