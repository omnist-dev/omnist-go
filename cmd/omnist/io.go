package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/osd"
)

// maxInputBytesFlag registers --max-input-bytes on fs and returns the
// pointer its value lands in, DefaultMaxInputBytes (64 MiB) until the flag is
// given (spec D-23/D-24, issue #76). Every command that reads an input has it,
// so a refusal always has a way to be lifted. A value that is not a positive
// integer, or is above the library's recommended ceiling
// (omnist.MaxRecommendedInputBytes), is a usage error: unlike a zero
// Limits.MaxInputBytes, which selects the default, an explicit flag value that
// means nothing is a mistake worth reporting.
func maxInputBytesFlag(fs *flag.FlagSet) *int {
	max := omnist.DefaultMaxInputBytes
	fs.Func("max-input-bytes", fmt.Sprintf("refuse an input of more than `N` bytes with document.limit.input-size (default %d, 64 MiB)", omnist.DefaultMaxInputBytes), func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return fmt.Errorf("must be a positive number of bytes, got %q", s)
		}
		if verr := limitsFor(n).Validate(); verr != nil {
			return verr
		}
		max = n
		return nil
	})
	return &max
}

// limitsFor is the Limits every document reader runs under: the library
// defaults with the input-size maximum set to maxInput.
func limitsFor(maxInput int) omnist.Limits {
	l := omnist.DefaultLimits()
	l.MaxInputBytes = maxInput
	return l
}

// readInput reads a document/schema body from path, or from stdin when
// path is "" or "-", matching common Unix CLI convention, and stops reading
// as soon as more than maxInput bytes have been seen (D-23): it never buffers
// the rest of an oversized input. The refusal names the code the library
// reports for the same condition and the flag that raises the maximum.
func readInput(path string, stdin io.Reader, maxInput int) (string, error) {
	r := stdin
	what := "stdin"
	if path != "" && path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", path, err)
		}
		defer func() { _ = f.Close() }()
		r = f
		what = path
	}
	b, err := io.ReadAll(io.LimitReader(r, int64(maxInput)+1))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", what, err)
	}
	if len(b) > maxInput {
		return "", fmt.Errorf("reading %s: input is more than the maximum of %d bytes (document.limit.input-size at $); raise the maximum with --max-input-bytes N", what, maxInput)
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
