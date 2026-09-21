package main

import (
	"fmt"
	"io"
	"os"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/osd"
)

// maxInputBytes is the safety ceiling on raw input bytes read from stdin
// or file (100 MiB), preventing unbounded memory allocation before any
// parser limit runs (issue #76).
const maxInputBytes = 100 * 1024 * 1024 // 100 MiB

// readInput reads a document/schema body from path, or from stdin when
// path is "" or "-", matching common Unix CLI convention, bounded by
// maxInputBytes.
func readInput(path string, stdin io.Reader) (string, error) {
	if path == "" || path == "-" {
		lr := io.LimitReader(stdin, maxInputBytes+1)
		b, err := io.ReadAll(lr)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		if len(b) > maxInputBytes {
			return "", fmt.Errorf("reading stdin: input exceeds maximum size limit of %d bytes (100 MiB)", maxInputBytes)
		}
		return string(b), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	lr := io.LimitReader(f, maxInputBytes+1)
	b, err := io.ReadAll(lr)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	if len(b) > maxInputBytes {
		return "", fmt.Errorf("reading %s: input exceeds maximum size limit of %d bytes (100 MiB)", path, maxInputBytes)
	}
	return string(b), nil
}

// writeOutput writes content to path, or to stdout when path is "",
// matching common Unix CLI convention (an omitted -o means stdout).
func writeOutput(path string, content string, stdout io.Writer) error {
	if path == "" {
		_, err := io.WriteString(stdout, content)
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// writeSchemaOutput renders s as canonical OSD and writes it to path (or
// stdout), returning the command's exit code. osd.Write refuses a schema whose
// field label has no OSD spelling (OSD-14: a C0 control character); that is an
// operation-reported problem (ExitProblem), not a usage error. Parsed schemas
// never trigger it, but `infer` builds a schema from document keys, and a JSON
// key may carry such a character.
func writeSchemaOutput(name, path string, s omnist.Schema, stdout, stderr io.Writer) int {
	text, err := osd.Write(s, false)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitProblem
	}
	if err := writeOutput(path, text, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	return ExitOK
}
