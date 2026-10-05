package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

func TestReadInputStdin(t *testing.T) {
	for _, path := range []string{"", "-"} {
		got, err := readInput(path, strings.NewReader("hello"), omnist.DefaultMaxInputBytes)
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
	got, err := readInput(p, nil, omnist.DefaultMaxInputBytes)
	if err != nil {
		t.Fatalf("readInput: %v", err)
	}
	if got != "file content" {
		t.Errorf("readInput() = %q, want %q", got, "file content")
	}
}

func TestReadInputMissingFile(t *testing.T) {
	_, err := readInput(filepath.Join(t.TempDir(), "does-not-exist"), nil, omnist.DefaultMaxInputBytes)
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
	_, err := readInput("-", failingReader{}, omnist.DefaultMaxInputBytes)
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

// D-23: an input of exactly the maximum is read, one byte more is refused with
// a message naming the library code and the flag that raises the maximum, and
// a stream is abandoned as soon as max+1 bytes have been seen.
func TestReadInputMaximumIsInclusive(t *testing.T) {
	p := writeTemp(t, "in.txt", "12345")
	for _, tc := range []struct {
		name string
		read func(max int) (string, error)
	}{
		{"stdin", func(max int) (string, error) { return readInput("-", strings.NewReader("12345"), max) }},
		{"file", func(max int) (string, error) { return readInput(p, nil, max) }},
	} {
		if got, err := tc.read(5); err != nil || got != "12345" {
			t.Errorf("%s: at the maximum: got %q, %v; want the input", tc.name, got, err)
		}
		_, err := tc.read(4)
		if err == nil {
			t.Fatalf("%s: one byte over the maximum was accepted", tc.name)
		}
		for _, want := range []string{"document.limit.input-size", "--max-input-bytes", "4 bytes"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: refusal %q does not mention %q", tc.name, err, want)
			}
		}
	}
}

func TestReadInputStopsStreamingAtMaxPlusOne(t *testing.T) {
	oversized := &repeatingReader{remaining: 1 << 40} // far more than a test should ever buffer
	_, err := readInput("-", oversized, 1000)
	if err == nil || !strings.Contains(err.Error(), "--max-input-bytes") {
		t.Fatalf("expected a size refusal, got %v", err)
	}
	if read := (1 << 40) - oversized.remaining; read != 1001 {
		t.Errorf("read %d bytes of the stream, want exactly max+1 = 1001", read)
	}
}

func TestReadInputDirectoryError(t *testing.T) {
	_, err := readInput(t.TempDir(), nil, omnist.DefaultMaxInputBytes)
	if err == nil {
		t.Error("expected error when reading directory as file, got nil")
	}
}
