package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI is a small helper that runs the CLI's internal run() dispatcher
// (not a subprocess -- see subprocess_test.go for real binary
// invocations) with the given args and stdin text, and returns the exit
// code plus captured stdout/stderr.
func runCLI(t *testing.T, args []string, stdin string) (code int, stdout, stderr string) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	code = run(args, strings.NewReader(stdin), &outBuf, &errBuf)
	return code, outBuf.String(), errBuf.String()
}

const testSchema = `record Root {
  "name": string,
  "age": integer,
  "nickname" [0,1]: string
}
root Root
`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunNoArgs(t *testing.T) {
	code, _, stderr := runCLI(t, nil, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRunHelp(t *testing.T) {
	code, stdout, _ := runCLI(t, []string{"--help"}, "")
	if code != ExitOK {
		t.Errorf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout, "omnist parse") {
		t.Errorf("stdout missing usage: %q", stdout)
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"bogus"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "unknown subcommand") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestCmdParseConvert(t *testing.T) {
	code, stdout, stderr := runCLI(t, []string{"parse", "--from", "json", "--to", "yaml", "-"}, `{"a": 1}`)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "\"a\": 1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestCmdParseMalformed(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "-"}, `{not json`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitProblem, stderr)
	}
}

func TestCmdParseMissingFrom(t *testing.T) {
	code, _, _ := runCLI(t, []string{"parse", "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdParseUnknownFormat(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"parse", "--from", "csv", "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "unknown format") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestCmdParseMissingFile(t *testing.T) {
	code, _, _ := runCLI(t, []string{"parse", "--from", "json", filepath.Join(t.TempDir(), "nope.json")}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdParseWrongArgCount(t *testing.T) {
	code, _, _ := runCLI(t, []string{"parse", "--from", "json"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdParseWriteToFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.yaml")
	code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "-o", out, "-"}, `{"a": 1}`)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "\"a\": 1") {
		t.Errorf("file content = %q", got)
	}
}

func TestCmdValidateSuccess(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, _ := runCLI(t, []string{"validate", "--from", "json", "--schema", schemaPath, "-"}, `{"name": "Ada", "age": 12}`)
	if code != ExitOK {
		t.Errorf("exit = %d, want %d", code, ExitOK)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on success", stdout)
	}
}

func TestCmdValidateFailure(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, _ := runCLI(t, []string{"validate", "--from", "json", "--schema", schemaPath, "-"}, `{"name": "Ada"}`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
	if !strings.Contains(stdout, "validate.cardinality") {
		t.Errorf("stdout = %q, want a validate.cardinality diagnostic", stdout)
	}
}

func TestCmdValidateMissingFlags(t *testing.T) {
	code, _, _ := runCLI(t, []string{"validate", "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	code, _, _ = runCLI(t, []string{"validate", "--from", "json", "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d (missing --schema)", code, ExitUsage)
	}
}

func TestCmdValidateBadSchema(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", "not a valid schema {{{")
	code, _, stderr := runCLI(t, []string{"validate", "--from", "json", "--schema", schemaPath, "-"}, `{}`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitProblem, stderr)
	}
}

func TestCmdValidateMissingSchemaFile(t *testing.T) {
	code, _, _ := runCLI(t, []string{"validate", "--from", "json", "--schema", filepath.Join(t.TempDir(), "nope.osd"), "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdMaterializeSuccessNoOutput(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "-"}, `{"name": "Ada", "age": 12}`)
	if code != ExitOK {
		t.Errorf("exit = %d, want %d", code, ExitOK)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty (no --to/-o requested)", stdout)
	}
}

func TestCmdMaterializeWithOutput(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, stderr := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "--to", "json", "-"}, `{"name": "Ada", "age": 12}`)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Ada") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestCmdMaterializeDiagnostics(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "-"}, `{"name": "Ada"}`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
	if !strings.Contains(stdout, "validate.cardinality") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestCmdMaterializeMissingFlags(t *testing.T) {
	code, _, _ := runCLI(t, []string{"materialize", "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ = runCLI(t, []string{"materialize", "--schema", schemaPath, "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d (missing --from)", code, ExitUsage)
	}
}

func TestCmdMaterializeWrongArgCount(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdMaterializeBadToFormat(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "--to", "csv", "-"}, `{"name":"Ada","age":1}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaNormalize(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, stderr := runCLI(t, []string{"schema", "normalize", schemaPath}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "record Root") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestCmdSchemaPrune(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, stderr := runCLI(t, []string{"schema", "prune", schemaPath}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "record Root") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestCmdSchemaExtract(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, stderr := runCLI(t, []string{"schema", "extract", "--keep", "name,age", schemaPath}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "record Root") || strings.Contains(stdout, "nickname") {
		t.Errorf("stdout = %q, want Root without the dropped nickname field", stdout)
	}
}

func TestCmdSchemaExtractMissingKeep(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"schema", "extract", schemaPath}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaExtractUnknownLabel(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, stderr := runCLI(t, []string{"schema", "extract", "--keep", "DoesNotExist", schemaPath}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitUsage, stderr)
	}
}

func TestCmdSchemaCompatibleWith(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, stderr := runCLI(t, []string{"schema", "compatible-with", schemaPath, schemaPath}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "true" {
		t.Errorf("stdout = %q, want true", stdout)
	}
}

func TestCmdSchemaEquivalent(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, stderr := runCLI(t, []string{"schema", "equivalent", schemaPath, schemaPath}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "true" {
		t.Errorf("stdout = %q, want true", stdout)
	}
}

func TestCmdSchemaCompareWrongArgCount(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"schema", "compatible-with", schemaPath}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaIsEmpty(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, stderr := runCLI(t, []string{"schema", "is-empty", schemaPath}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "false" {
		t.Errorf("stdout = %q, want false", stdout)
	}
}

func TestCmdSchemaIsEmptyWrongArgCount(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema", "is-empty"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaNoArgs(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaHelp(t *testing.T) {
	code, stdout, _ := runCLI(t, []string{"schema", "--help"}, "")
	if code != ExitOK {
		t.Errorf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout, "schema normalize") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestCmdSchemaUnknownSubcommand(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"schema", "bogus"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "unknown subcommand") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestCmdSchemaBadSchemaFile(t *testing.T) {
	schemaPath := writeTemp(t, "bad.osd", "{{{ garbage")
	code, _, _ := runCLI(t, []string{"schema", "normalize", schemaPath}, "")
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
}

func TestCmdSchemaWrongArgCountNormalize(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema", "normalize"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdInfer(t *testing.T) {
	f1 := writeTemp(t, "a.json", `{"name": "Ada", "age": 12}`)
	code, stdout, stderr := runCLI(t, []string{"infer", "--from", "json", f1}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "record Root") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestCmdInferAllowAnyFallback(t *testing.T) {
	f1 := writeTemp(t, "a.json", `{"v": 1}`)
	f2 := writeTemp(t, "b.json", `{"v": "s"}`)
	code, stdout, stderr := runCLI(t, []string{"infer", "--from", "json", "--allow-any", f1, f2}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "opened to any") {
		t.Errorf("stderr = %q, want an any-fallback note", stderr)
	}
	if !strings.Contains(stdout, "any") {
		t.Errorf("stdout = %q, want any-typed field", stdout)
	}
}

func TestCmdInferConflict(t *testing.T) {
	f1 := writeTemp(t, "a.json", `{"v": 1}`)
	f2 := writeTemp(t, "b.json", `{"v": "s"}`)
	code, _, stderr := runCLI(t, []string{"infer", "--from", "json", f1, f2}, "")
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitProblem, stderr)
	}
}

func TestCmdInferNoFiles(t *testing.T) {
	code, _, _ := runCLI(t, []string{"infer", "--from", "json"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdInferMissingFrom(t *testing.T) {
	f1 := writeTemp(t, "a.json", `{"v": 1}`)
	code, _, _ := runCLI(t, []string{"infer", f1}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdInferBadFile(t *testing.T) {
	code, _, _ := runCLI(t, []string{"infer", "--from", "json", filepath.Join(t.TempDir(), "nope.json")}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdInferMalformedFile(t *testing.T) {
	f1 := writeTemp(t, "bad.json", `{not json`)
	code, _, _ := runCLI(t, []string{"infer", "--from", "json", f1}, "")
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
}

func TestCmdLintClean(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, stdout, stderr := runCLI(t, []string{"lint", schemaPath}, "")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestCmdLintFinding(t *testing.T) {
	// A record unreachable from root should trigger a lint finding.
	schema := `record Root {
  "name": string
}
record Unused {
  "x": string
}
root Root
`
	schemaPath := writeTemp(t, "s.osd", schema)
	code, stdout, _ := runCLI(t, []string{"lint", schemaPath}, "")
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d, stdout=%q", code, ExitProblem, stdout)
	}
	if stdout == "" {
		t.Error("stdout empty, want at least one finding line")
	}
}

func TestCmdLintWrongArgCount(t *testing.T) {
	code, _, _ := runCLI(t, []string{"lint"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdLintMissingFile(t *testing.T) {
	code, _, _ := runCLI(t, []string{"lint", filepath.Join(t.TempDir(), "nope.osd")}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestFlagParseErrorIsUsage(t *testing.T) {
	code, _, _ := runCLI(t, []string{"parse", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func badOutputPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "no-such-dir", "out.txt")
}

func TestCmdParseWriteError(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "-o", badOutputPath(t), "-"}, `{"a": 1}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitUsage, stderr)
	}
}

func TestCmdValidateMissingInputFile(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"validate", "--from", "json", "--schema", schemaPath, filepath.Join(t.TempDir(), "nope.json")}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdValidateBadFromFormat(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"validate", "--from", "csv", "--schema", schemaPath, "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdValidateWrongArgCount(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"validate", "--from", "json", "--schema", schemaPath}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdMaterializeMissingInputFile(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, filepath.Join(t.TempDir(), "nope.json")}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdMaterializeMissingSchemaFile(t *testing.T) {
	code, _, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", filepath.Join(t.TempDir(), "nope.osd"), "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdMaterializeBadSchema(t *testing.T) {
	schemaPath := writeTemp(t, "bad.osd", "{{{ garbage")
	code, _, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "-"}, `{}`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
}

func TestCmdMaterializeWriteError(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "--to", "json", "-o", badOutputPath(t), "-"}, `{"name":"Ada","age":1}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaTransformWriteError(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"schema", "normalize", "-o", badOutputPath(t), schemaPath}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaExtractWriteError(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"schema", "extract", "--keep", "name,age", "-o", badOutputPath(t), schemaPath}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaExtractKeepAllWhitespace(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, stderr := runCLI(t, []string{"schema", "extract", "--keep", " , ,", schemaPath}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitUsage, stderr)
	}
}

func TestCmdSchemaExtractBadSchema(t *testing.T) {
	schemaPath := writeTemp(t, "bad.osd", "{{{ garbage")
	code, _, _ := runCLI(t, []string{"schema", "extract", "--keep", "name", schemaPath}, "")
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
}

func TestCmdSchemaCompareSecondArgBad(t *testing.T) {
	good := writeTemp(t, "good.osd", testSchema)
	bad := writeTemp(t, "bad.osd", "{{{ garbage")
	code, _, _ := runCLI(t, []string{"schema", "compatible-with", good, bad}, "")
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
}

func TestCmdSchemaCompareFirstArgBad(t *testing.T) {
	bad := writeTemp(t, "bad.osd", "{{{ garbage")
	good := writeTemp(t, "good.osd", testSchema)
	code, _, _ := runCLI(t, []string{"schema", "compatible-with", bad, good}, "")
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
}

func TestCmdInferWriteError(t *testing.T) {
	f1 := writeTemp(t, "a.json", `{"name": "Ada"}`)
	code, _, _ := runCLI(t, []string{"infer", "--from", "json", "-o", badOutputPath(t), f1}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdInferBadFromFormat(t *testing.T) {
	f1 := writeTemp(t, "a.json", `{"name": "Ada"}`)
	code, _, _ := runCLI(t, []string{"infer", "--from", "csv", f1}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdLintFlagParseError(t *testing.T) {
	code, _, _ := runCLI(t, []string{"lint", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaTransformFlagParseError(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema", "normalize", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaExtractFlagParseError(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema", "extract", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaCompareFlagParseError(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema", "compatible-with", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaIsEmptyFlagParseError(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema", "is-empty", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdInferFlagParseError(t *testing.T) {
	code, _, _ := runCLI(t, []string{"infer", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdValidateFlagParseError(t *testing.T) {
	code, _, _ := runCLI(t, []string{"validate", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdMaterializeFlagParseError(t *testing.T) {
	code, _, _ := runCLI(t, []string{"materialize", "--not-a-flag"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdParseBadToFormat(t *testing.T) {
	code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "--to", "csv", "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitUsage, stderr)
	}
}

func TestCmdParseWriterError(t *testing.T) {
	// A bare scalar-rooted document has no XML spelling (XML requires a
	// single top-level element) -- WriteXML reports
	// write.unsupported-value, exercising the writer(doc) error branch
	// distinct from the reader(text) error branch above it.
	code, _, stderr := runCLI(t, []string{"parse", "--from", "json", "--to", "xml", "-"}, `42`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitProblem, stderr)
	}
}

func TestCmdMaterializeMissingSchemaFlag(t *testing.T) {
	code, _, _ := runCLI(t, []string{"materialize", "--from", "json", "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdMaterializeBadFromFormat(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"materialize", "--from", "csv", "--schema", schemaPath, "-"}, `{}`)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdMaterializeOutputFileNoToFlag(t *testing.T) {
	// -o without --to should still work, defaulting the output format
	// to --from.
	schemaPath := writeTemp(t, "s.osd", testSchema)
	dir := t.TempDir()
	out := filepath.Join(dir, "out.json")
	code, _, stderr := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "-o", out, "-"}, `{"name":"Ada","age":1}`)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Ada") {
		t.Errorf("file content = %q", got)
	}
}

func TestCmdMaterializeMalformedInput(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "-"}, `{not json`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
}

func TestCmdSchemaExtractWrongArgCount(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema", "extract", "--keep", "name"}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdSchemaIsEmptyMissingFile(t *testing.T) {
	code, _, _ := runCLI(t, []string{"schema", "is-empty", filepath.Join(t.TempDir(), "nope.osd")}, "")
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestCmdValidateMalformedInput(t *testing.T) {
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, _ := runCLI(t, []string{"validate", "--from", "json", "--schema", schemaPath, "-"}, `{not json`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d", code, ExitProblem)
	}
}

func TestCmdMaterializeWriterErrorMultipleRoots(t *testing.T) {
	// testSchema's Root record has more than one top-level field, so
	// materializing it and writing the result as XML hits
	// CodeFormatMultipleRoots -- exercising the writer(result) error
	// branch in cmdMaterialize distinct from the earlier reader/schema
	// parse-error branches.
	schemaPath := writeTemp(t, "s.osd", testSchema)
	code, _, stderr := runCLI(t, []string{"materialize", "--from", "json", "--schema", schemaPath, "--to", "xml", "-"}, `{"name":"Ada","age":1}`)
	if code != ExitProblem {
		t.Errorf("exit = %d, want %d, stderr=%q", code, ExitProblem, stderr)
	}
}

func TestCmdParseToOML(t *testing.T) {
	// Exercises the oml entry in formatWriters' shared table (format.go),
	// whose closure wrapping oml.Write otherwise never runs.
	code, stdout, stderr := runCLI(t, []string{"parse", "--from", "json", "--to", "oml", "-"}, `{"a": 1}`)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "a") {
		t.Errorf("stdout = %q", stdout)
	}
}
