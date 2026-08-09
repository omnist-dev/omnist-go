package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildOmnistBinary compiles the actual cmd/omnist binary once per test
// run into a temp dir, proving `go build` on this package succeeds and
// giving the subprocess tests below a real executable to invoke -- as
// opposed to the run_test.go tests, which call run() in-process.
func buildOmnistBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "omnist"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build cmd/omnist: %v\n%s", err, stderr.String())
	}
	return bin
}

func runBinary(t *testing.T, bin string, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return 0, outBuf.String(), errBuf.String()
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), outBuf.String(), errBuf.String()
	}
	t.Fatalf("running %s %v: %v", bin, args, err)
	return -1, "", ""
}

// TestSubprocessParseGoldenPath proves the real compiled binary converts
// JSON to YAML end to end via stdin/stdout, not just the internal run()
// function.
func TestSubprocessParseGoldenPath(t *testing.T) {
	bin := buildOmnistBinary(t)
	code, stdout, stderr := runBinary(t, bin, `{"greeting": "hi"}`, "parse", "--from", "json", "--to", "yaml", "-")
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "\"greeting\": \"hi\"") {
		t.Errorf("stdout = %q", stdout)
	}
}

// TestSubprocessValidateGoldenPath proves the real binary validates a
// document against a schema file on disk and reports real diagnostics
// with a nonzero exit code, via a real subprocess invocation.
func TestSubprocessValidateGoldenPath(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "s.osd")
	if err := os.WriteFile(schemaPath, []byte(testSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runBinary(t, bin, `{"name": "Ada"}`, "validate", "--from", "json", "--schema", schemaPath, "-")
	if code != ExitProblem {
		t.Fatalf("exit = %d, want %d", code, ExitProblem)
	}
	if !strings.Contains(stdout, "validate.cardinality") {
		t.Errorf("stdout = %q", stdout)
	}
}

// TestSubprocessSchemaNormalizeGoldenPath proves the real binary runs a
// schema-algebra subcommand (normalize) end to end and prints valid OSD.
func TestSubprocessSchemaNormalizeGoldenPath(t *testing.T) {
	bin := buildOmnistBinary(t)
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "s.osd")
	if err := os.WriteFile(schemaPath, []byte(testSchema), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runBinary(t, bin, "", "schema", "normalize", schemaPath)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "record Root") {
		t.Errorf("stdout = %q", stdout)
	}
}

// TestSubprocessMissingFileExitsCleanly proves a real invocation with a
// nonexistent input file produces the documented usage-error exit code
// and a sensible message, not a panic or Go stack trace.
func TestSubprocessMissingFileExitsCleanly(t *testing.T) {
	bin := buildOmnistBinary(t)
	code, _, stderr := runBinary(t, bin, "", "parse", "--from", "json", filepath.Join(t.TempDir(), "does-not-exist.json"))
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if strings.Contains(stderr, "panic") || strings.Contains(stderr, "goroutine") {
		t.Errorf("stderr looks like a panic, not a clean error: %q", stderr)
	}
}

// TestSubprocessMalformedInputExitsCleanly proves a real invocation with
// malformed input produces the documented operation-problem exit code
// rather than a panic.
func TestSubprocessMalformedInputExitsCleanly(t *testing.T) {
	bin := buildOmnistBinary(t)
	code, _, stderr := runBinary(t, bin, `{not valid json`, "parse", "--from", "json", "-")
	if code != ExitProblem {
		t.Fatalf("exit = %d, want %d", code, ExitProblem)
	}
	if strings.Contains(stderr, "panic") || strings.Contains(stderr, "goroutine") {
		t.Errorf("stderr looks like a panic, not a clean error: %q", stderr)
	}
}

// TestSubprocessBadFormatNameExitsCleanly proves an unknown --from
// format name is a usage error, not a panic, via a real invocation.
func TestSubprocessBadFormatNameExitsCleanly(t *testing.T) {
	bin := buildOmnistBinary(t)
	code, _, stderr := runBinary(t, bin, "{}", "parse", "--from", "yeehaw", "-")
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "unknown format") {
		t.Errorf("stderr = %q", stderr)
	}
}
