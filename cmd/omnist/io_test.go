package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadInputStdin(t *testing.T) {
	for _, path := range []string{"", "-"} {
		got, err := readInput(path, strings.NewReader("hello"))
		if err != nil {
			t.Fatalf("readInput(%q): %v", path, err)
		}
		if got != "hello" {
			t.Errorf("readInput(%q) = %q, want %q", path, got, "hello")
		}
	}
}

func TestReadInputFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(p, []byte("file content"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readInput(p, nil)
	if err != nil {
		t.Fatalf("readInput: %v", err)
	}
	if got != "file content" {
		t.Errorf("readInput() = %q, want %q", got, "file content")
	}
}

func TestReadInputMissingFile(t *testing.T) {
	_, err := readInput(filepath.Join(t.TempDir(), "does-not-exist"), nil)
	if err == nil {
		t.Error("readInput: expected error for missing file, got nil")
	}
}

func TestWriteOutputStdout(t *testing.T) {
	var buf bytes.Buffer
	if err := writeOutput("", "hi", &buf); err != nil {
		t.Fatalf("writeOutput: %v", err)
	}
	if buf.String() != "hi" {
		t.Errorf("stdout = %q, want %q", buf.String(), "hi")
	}
}

func TestWriteOutputFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	if err := writeOutput(p, "content", nil); err != nil {
		t.Fatalf("writeOutput: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "content" {
		t.Errorf("file content = %q, want %q", got, "content")
	}
}

func TestWriteOutputBadPath(t *testing.T) {
	err := writeOutput(filepath.Join(t.TempDir(), "nosuchdir", "out.txt"), "x", nil)
	if err == nil {
		t.Error("writeOutput: expected error for unwritable path, got nil")
	}
}

// failingReader always errors, letting TestReadInputStdinError exercise
// readInput's io.ReadAll failure branch without needing a real broken
// stdin.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

func TestReadInputStdinError(t *testing.T) {
	_, err := readInput("-", failingReader{})
	if err == nil {
		t.Error("readInput: expected error from a failing stdin reader, got nil")
	}
}

type repeatingReader struct {
	remaining int64
}

func (r *repeatingReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	for i := 0; i < n; i++ {
		p[i] = 'a'
	}
	r.remaining -= int64(n)
	return n, nil
}

func TestReadInputExceedsMaxInputBytesStdin(t *testing.T) {
	oversized := &repeatingReader{remaining: maxInputBytes + 10}
	_, err := readInput("-", oversized)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size limit") {
		t.Errorf("expected size limit error, got %v", err)
	}
}

func TestReadInputExceedsMaxInputBytesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "large.txt")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(int64(maxInputBytes + 10)); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	_, err = readInput(p, nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size limit") {
		t.Errorf("expected size limit error for oversized file, got %v", err)
	}
}

func TestReadInputDirectoryError(t *testing.T) {
	_, err := readInput(t.TempDir(), nil)
	if err == nil {
		t.Error("expected error when reading directory as file, got nil")
	}
}
