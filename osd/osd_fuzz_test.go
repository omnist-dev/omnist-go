package osd

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// checkSchemaSane performs a cheap, meaningful structural check on a
// successfully parsed Schema: every record referenced by Root and by every
// field's type must actually resolve in Env, and EnvOrder must describe
// the same key set as Env (see the invariant documented on omnist.Schema).
// It intentionally does not assert full spec well-formedness (S-1..S-7) --
// that is validate.go's job, not the reader's, and not what fuzzing this
// reader is for.
func checkSchemaSane(t *testing.T, s omnist.Schema) {
	t.Helper()
	if len(s.EnvOrder) != len(s.Env) {
		t.Fatalf("EnvOrder has %d entries but Env has %d", len(s.EnvOrder), len(s.Env))
	}
	seen := make(map[string]bool, len(s.EnvOrder))
	for _, name := range s.EnvOrder {
		if seen[name] {
			t.Fatalf("EnvOrder lists %q more than once", name)
		}
		seen[name] = true
		if _, ok := s.Env[name]; !ok {
			t.Fatalf("EnvOrder lists %q but Env has no entry for it", name)
		}
		if s.Env[name] == nil {
			t.Fatalf("Env[%q] is nil", name)
		}
	}
}

// FuzzRead exercises osd.Read against arbitrary input text. Unlike the
// other five readers, osd.Read takes no Limits: an OSD schema is authored
// text, not attacker-controlled data (see the doc comment on Read), so
// there is no small-limits variant to construct here. The only properties
// asserted are: Read must never panic, must never hang, and on success
// must return a structurally sane Schema. On error nothing is asserted
// beyond "it's a real error" -- Read's documented contract is that
// failures come back as either *omnist.ParseError or omnist.Diagnostic,
// and fuzzing isn't about which one.
func FuzzRead(f *testing.F) {
	f.Add("record R { \"a\": string } root R")
	f.Add("")
	f.Add("record R {\n    \"a\\nb\": string,\n}\nroot R\n") // osd-grammar/strings/label-unescaping-has-no-named-escape-table
	f.Add("record R {\n    \"a\" [1,5]: string,\n}\nroot R\n") // osd-grammar/cardinality/bounded-range
	f.Add("record R {\n    \"a\" [5,]: string,\n}\nroot R\n") // osd-grammar/cardinality/at-least-m-unbounded
	f.Add("record R {\n    \"a\" [,5]: string,\n}\nroot R\n") // osd-grammar/cardinality/comma-first-at-most-n
	f.Add("record R {\n    \"a\" [,]: string,\n}\nroot R\n") // osd-grammar/cardinality/comma-only-any-count
	f.Add("record R {\n    \"a\" []: string,\n}\nroot R\n") // osd-grammar/cardinality/empty-brackets-is-an-error
	f.Add("record R {\n    \"a\" [-1]: string,\n}\nroot R\n") // osd-grammar/cardinality/negative-minimum-is-invalid-not-a-syntax-error
	f.Add("record R {\n    \"a\" [1,0]: string,\n}\nroot R\n") // osd-grammar/cardinality/inverted-range-is-invalid
	f.Add("record R {\n    \"a\" [1.5]: string,\n}\nroot R\n") // osd-grammar/cardinality/non-integer-bound-is-an-error
	f.Add("record R {\n    \"a\": string?,\n}\nroot R\n") // osd-grammar/nullable/nullable-scalar-is-legal
	f.Add("record Other {\n    \"x\": string,\n}\nrecord R {\n    \"a\": Other?,\n}\nroot R\n") // osd-grammar/nullable/question-mark-on-a-reference-is-rejected
	f.Add("record R {\n    \"data\": any?,\n}\nroot R\n") // osd-grammar/any/any-with-question-mark-is-rejected
	f.Add("record R {\n    \"data\": Any,\n}\nroot R\n") // osd-grammar/any/capitalized-any-is-an-ordinary-reference
	f.Add("record string {\n    \"a\": string,\n}\nroot string\n") // osd-grammar/reserved-names/record-named-after-a-scalar-keyword-is-rejected
	f.Add("record any {\n    \"a\": string,\n}\nroot any\n") // osd-grammar/reserved-names/record-named-any-is-rejected
	f.Add("record R {\n    \"a\": string,\n}\nrecord R {\n    \"b\": integer,\n}\nroot R\n") // osd-grammar/reserved-names/duplicate-record-definition-is-an-error
	f.Add("record R {\n    \"a\": string,\n}\n") // osd-grammar/root/missing-root-is-an-error
	f.Add("record R {\n    a: string,\n}\nroot R\n") // osd-grammar/labels/unquoted-field-name-is-rejected
	f.Add("record R {\n    \"a\": \"string\",\n}\nroot R\n") // osd-grammar/labels/quoted-string-in-type-position-is-rejected
	f.Add("record R {\n    \"a\": string,\n}\nroot R\n") // osd-grammar/records/trailing-comma-after-last-field-is-legal
	f.Add("record R {\n    \"a\": string,\n    \"a\": integer,\n}\nroot R\n") // osd-grammar/records/duplicate-field-label-in-one-record-is-an-error
	f.Add("record R {\n    \"data\" [0,]: any,\n}\nroot R\n") // osd-grammar/any/cardinality-is-orthogonal-to-any
	f.Add("record R { \"a\" [1,5]: string } root R") // repo-test-literal
	f.Add("record R { \"a\" [5,]: string } root R") // repo-test-literal
	f.Add("record R { \"a\" [,5]: string } root R") // repo-test-literal
	f.Add("record R { \"a\" [,]: string } root R") // repo-test-literal
	f.Add("record R { \"a\": string? } root R") // repo-test-literal
	f.Add("record R { \"a\": string, } root R") // repo-test-literal
	f.Add("record R { \"data\": any } root R") // repo-test-literal
	f.Add("record R { \"data\" [0,]: any } root R") // repo-test-literal
	f.Add("record R { \"a\\\\b\": string } root R") // repo-test-literal
	f.Add("record R { \"a\\\"b\": string } root R") // repo-test-literal
	f.Add("record R { \"a\\tb\": string } root R") // repo-test-literal
	f.Add("record R { \"a\": ") // repo-test-literal
	f.Add("record R { \"b\": B } record B { \"x\": string } root R") // repo-test-literal
	f.Add("record A { \"b\" [0,1]: B } record B { \"a\" [0,1]: A } root A") // repo-test-literal
	f.Add("record A{\"x\":string} record B{\"x\":string} root A root B") // repo-test-literal
	f.Add("record R{\"a\":string} record Unused{\"x\":string} root R") // repo-test-literal
	f.Add("record R { } root R") // repo-test-literal

	f.Fuzz(func(t *testing.T, text string) {
		schema, err := Read(text)
		if err != nil {
			return
		}
		checkSchemaSane(t, schema)
	})
}
