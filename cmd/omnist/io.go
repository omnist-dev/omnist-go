package main

import (
	"fmt"
	"io"
	"os"
)

// readInput reads a document/schema body from path, or from stdin when
// path is "" or "-", matching common Unix CLI convention.
func readInput(path string, stdin io.Reader) (string, error) {
	if path == "" || path == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
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
