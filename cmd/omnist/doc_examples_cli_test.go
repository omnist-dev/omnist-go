package main

// This file backs the runnable CLI transcripts added to docs/cli.md by
// issue #66: real subprocess invocations of the compiled omnist binary,
// following the same buildOmnistBinary/runBinary mechanism
// subprocess_test.go already established (see its doc comment) rather
// than inventing a second one. Unlike doc_examples_test.go/
// doc_examples_reference_test.go's godoc Example functions -- which
// assert an exact "// Output:" comment -- these are plain *testing.T
// tests that assert the exact stdout/stderr/exit-code triple shown in
// docs/cli.md's fenced transcript, since a CLI example's "output" is a
// terminal transcript (command *and* what it prints), not a single
// Println value. Keep each test's asserted strings in sync,
// character-for-character, with the corresponding block in
// docs/cli.md; if you change one, change the other and re-run
// `go test ./cmd/omnist` to confirm they still match.

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFileT(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestCLIExampleParseJSONToYAML backs cli.md's "Convert JSON to YAML"
// example.
func TestCLIExampleParseJSONToYAML(t *testing.T) {
	bin := buildOmnistBinary(t)
	code, stdout, stderr := runBinary(t, bin, `{"name": "Ann", "tags": ["a", "b"]}`, "parse", "--from", "json", "--to", "yaml", "-")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "\"name\": \"Ann\"\n\"tags\":\n    - \"a\"\n    - \"b\"\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestCLIExampleValidate backs cli.md's "Validate a document against a
// schema" example.
func TestCLIExampleValidate(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := writeFileT(t, dir, "person.osd", "record Person { \"name\": string, \"age\": integer }\nroot Person\n")
	docPath := writeFileT(t, dir, "person.json", `{"name": "Ann", "age": "42"}`)

	code, stdout, stderr := runBinary(t, bin, "", "validate", "--from", "json", "--schema", schemaPath, docPath)
	if code != ExitProblem {
		t.Fatalf("exit = %d, want %d, stderr = %q", code, ExitProblem, stderr)
	}
	want := "$.age: validate.type-mismatch: value does not match declared kind\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestCLIExampleSchemaCompatibleWith backs cli.md's "Check whether a
// schema change is backward-compatible" example.
func TestCLIExampleSchemaCompatibleWith(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	newPath := writeFileT(t, dir, "new.osd", "record User { \"id\": string, \"name\": string, \"nick\" [0,1]: string }\nroot User\n")
	oldPath := writeFileT(t, dir, "old.osd", "record User { \"id\": string, \"name\": string }\nroot User\n")

	code, stdout, stderr := runBinary(t, bin, "", "schema", "compatible-with", newPath, oldPath)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "false\n" {
		t.Errorf("stdout = %q, want %q", stdout, "false\n")
	}
}

// TestCLIExampleMaterialize backs cli.md's `materialize` example: a JSON
// string that looks like a date is upgraded to TOML's native date kind.
func TestCLIExampleMaterialize(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := writeFileT(t, dir, "event.osd", "record Event { \"when\": date }\nroot Event\n")
	docPath := writeFileT(t, dir, "event.json", `{"when": "2024-01-01"}`)

	code, stdout, stderr := runBinary(t, bin, "", "materialize", "--from", "json", "--schema", schemaPath, "--to", "toml", docPath)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "\"when\" = 2024-01-01\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestCLIExampleSchemaNormalize backs cli.md's `schema normalize`
// example: two structurally-identical records (A, B) collapse to one
// shared shape.
func TestCLIExampleSchemaNormalize(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := writeFileT(t, dir, "dup.osd", "record A { \"id\": string }\nrecord B { \"id\": string }\nrecord Root { \"a\": A, \"b\": B }\nroot Root\n")

	code, stdout, stderr := runBinary(t, bin, "", "schema", "normalize", schemaPath)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "record A {\n    \"id\": string,\n}\nrecord Root {\n    \"a\": A,\n    \"b\": A,\n}\nroot Root\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestCLIExampleSchemaPrune backs cli.md's `schema prune` example: a
// never-emittable field ([0,0]) and the record it alone referenced both
// disappear.
func TestCLIExampleSchemaPrune(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := writeFileT(t, dir, "dead.osd", "record Root { \"id\": string, \"dead\" [0,0]: Orphan }\nrecord Orphan { \"note\": string }\nroot Root\n")

	code, stdout, stderr := runBinary(t, bin, "", "schema", "prune", schemaPath)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "record Root {\n    \"id\": string,\n}\nroot Root\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestCLIExampleSchemaExtract backs cli.md's `schema extract` example:
// --keep trims the schema down to only the named field.
func TestCLIExampleSchemaExtract(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := writeFileT(t, dir, "extract.osd", "record Root { \"keep\": string, \"drop\" [0,1]: string }\nroot Root\n")

	code, stdout, stderr := runBinary(t, bin, "", "schema", "extract", "--keep", "keep", schemaPath)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "record Root {\n    \"keep\": string,\n}\nroot Root\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestCLIExampleSchemaEquivalent backs cli.md's `schema equivalent`
// example: two schemas differing only in record naming are equivalent.
func TestCLIExampleSchemaEquivalent(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	personPath := writeFileT(t, dir, "person.osd", "record Person { \"id\": string, \"name\": string }\nroot Person\n")
	userPath := writeFileT(t, dir, "user.osd", "record User { \"id\": string, \"name\": string }\nroot User\n")

	code, stdout, stderr := runBinary(t, bin, "", "schema", "equivalent", personPath, userPath)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "true\n" {
		t.Errorf("stdout = %q, want %q", stdout, "true\n")
	}
}

// TestCLIExampleSchemaIsEmpty backs cli.md's `schema is-empty` example:
// a record that can only ever satisfy itself by requiring itself
// ([1,1], the OSD default) is unsatisfiable.
func TestCLIExampleSchemaIsEmpty(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := writeFileT(t, dir, "empty.osd", "record Root { \"self\": Root }\nroot Root\n")

	code, stdout, stderr := runBinary(t, bin, "", "schema", "is-empty", schemaPath)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "true\n" {
		t.Errorf("stdout = %q, want %q", stdout, "true\n")
	}
}

// TestCLIExampleInfer backs cli.md's `infer` example: two samples that
// disagree on whether "tags" is present produce an optional field.
func TestCLIExampleInfer(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	s1 := writeFileT(t, dir, "s1.json", `{"name": "Ann", "tags": ["a"]}`)
	s2 := writeFileT(t, dir, "s2.json", `{"name": "Bo"}`)

	code, stdout, stderr := runBinary(t, bin, "", "infer", "--from", "json", s1, s2)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "record Root {\n    \"name\": string,\n    \"tags\" [0,1]: string,\n}\nroot Root\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestCLIExampleInferAllowAny backs cli.md's `--allow-any` example: a
// label that mixes scalar kinds fails infer outright without the flag,
// and opens to `any` (with an informational stderr line) with it.
func TestCLIExampleInferAllowAny(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	a1 := writeFileT(t, dir, "a1.json", `{"val": "a"}`)
	a2 := writeFileT(t, dir, "a2.json", `{"val": 42}`)

	code, _, stderr := runBinary(t, bin, "", "infer", "--from", "json", a1, a2)
	if code != ExitProblem {
		t.Fatalf("without --allow-any: exit = %d, want %d", code, ExitProblem)
	}
	wantErr := "omnist infer: Root.val: algebra.infer-conflicting-scalars: label \"val\" has values of more than one scalar kind\n"
	if stderr != wantErr {
		t.Errorf("without --allow-any: stderr = %q, want %q", stderr, wantErr)
	}

	code, stdout, stderr := runBinary(t, bin, "", "infer", "--from", "json", "--allow-any", a1, a2)
	if code != ExitOK {
		t.Fatalf("with --allow-any: exit = %d, stderr = %q", code, stderr)
	}
	wantOut := "record Root {\n    \"val\": any,\n}\nroot Root\n"
	if stdout != wantOut {
		t.Errorf("with --allow-any: stdout = %q, want %q", stdout, wantOut)
	}
	wantErr = "Root.val: opened to any: values of more than one scalar kind (integer, string)\n"
	if stderr != wantErr {
		t.Errorf("with --allow-any: stderr = %q, want %q", stderr, wantErr)
	}
}

// TestCLIExampleLint backs cli.md's `lint` example: an unreachable
// record produces a warning-level finding, which is also a reported
// "problem" per lint's own doc comment.
func TestCLIExampleLint(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := writeFileT(t, dir, "orphan.osd", "record Root { \"id\": string }\nrecord Orphan { \"id\": string, \"note\": string }\nroot Root\n")

	code, stdout, stderr := runBinary(t, bin, "", "lint", schemaPath)
	if code != ExitProblem {
		t.Fatalf("exit = %d, want %d, stderr = %q", code, ExitProblem, stderr)
	}
	want := "Orphan: warning: lint.unreachable-record: record \"Orphan\" is defined but not reachable from root by any reference\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}
