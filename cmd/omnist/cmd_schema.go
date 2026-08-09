package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/osd"
)

// cmdSchema dispatches `omnist schema SUBCOMMAND ...` to one of the
// seven schema-algebra sub-subcommands.
func cmdSchema(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printSchemaUsage(stderr)
		return ExitUsage
	}
	switch args[0] {
	case "-h", "--help":
		printSchemaUsage(stdout)
		return ExitOK
	case "normalize":
		return schemaTransform(args[1:], stdin, stdout, stderr, "omnist schema normalize", omnist.Normalize)
	case "prune":
		return schemaTransform(args[1:], stdin, stdout, stderr, "omnist schema prune", omnist.Prune)
	case "extract":
		return cmdSchemaExtract(args[1:], stdin, stdout, stderr)
	case "compatible-with":
		return schemaBoolean(args[1:], stdin, stdout, stderr, "omnist schema compatible-with", omnist.CompatibleWith)
	case "equivalent":
		return schemaBoolean(args[1:], stdin, stdout, stderr, "omnist schema equivalent", omnist.Equivalent)
	case "is-empty":
		return cmdSchemaIsEmpty(args[1:], stdin, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "omnist schema: unknown subcommand %q\n\n", args[0])
		printSchemaUsage(stderr)
		return ExitUsage
	}
}

func printSchemaUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, `usage:
  omnist schema normalize [-o FILE] SCHEMA
  omnist schema prune [-o FILE] SCHEMA
  omnist schema extract --keep label1,label2,... [-o FILE] SCHEMA
  omnist schema compatible-with A B
  omnist schema equivalent A B
  omnist schema is-empty SCHEMA

(flags must come before the positional SCHEMA argument)`)
}

// loadSchema reads and parses one OSD schema argument (a file path, or
// "-"/"" for stdin), matching how parse/validate/materialize distinguish
// their two failure modes: a file/stdin that couldn't be read is a usage
// error (ExitUsage, the CLI never got to run OSD's parser), while text
// that read fine but doesn't parse as OSD is an operation problem
// (ExitProblem, the same bucket a malformed document parse error falls
// into). On failure it has already written a "name: message" line to
// stderr; the caller just returns the exit code.
func loadSchema(name, path string, stdin io.Reader, stderr io.Writer) (omnist.Schema, int, bool) {
	text, err := readInput(path, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return omnist.Schema{}, ExitUsage, false
	}
	schema, err := osd.Read(text)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return omnist.Schema{}, ExitProblem, false
	}
	return schema, ExitOK, true
}

// schemaTransform implements the shared shape of `schema normalize` and
// `schema prune`: read one SCHEMA, apply a pure Schema -> Schema
// transform, write the result as OSD.
func schemaTransform(args []string, stdin io.Reader, stdout, stderr io.Writer, name string, transform func(omnist.Schema) omnist.Schema) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output file (default: stdout)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "usage: %s [-o FILE] SCHEMA\n", name)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: expected exactly one SCHEMA argument\n", name)
		fs.Usage()
		return ExitUsage
	}

	schema, code, ok := loadSchema(name, fs.Arg(0), stdin, stderr)
	if !ok {
		return code
	}
	result := transform(schema)
	if err := writeOutput(*out, osd.Write(result, false), stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	return ExitOK
}

// cmdSchemaExtract implements `omnist schema extract`.
func cmdSchemaExtract(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	const name = "omnist schema extract"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	keep := fs.String("keep", "", "comma-separated record names to keep (required)")
	out := fs.String("o", "", "output file (default: stdout)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "usage: %s --keep label1,label2,... [-o FILE] SCHEMA\n", name)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: expected exactly one SCHEMA argument\n", name)
		fs.Usage()
		return ExitUsage
	}
	if *keep == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --keep is required\n", name)
		return ExitUsage
	}
	keepMap := make(map[string]bool)
	for _, label := range strings.Split(*keep, ",") {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		keepMap[label] = true
	}
	if len(keepMap) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: --keep must name at least one record\n", name)
		return ExitUsage
	}

	schema, code, ok := loadSchema(name, fs.Arg(0), stdin, stderr)
	if !ok {
		return code
	}
	// Extract rejects a --keep set naming something the schema doesn't
	// have (or an ill-formed request in general) -- that's the caller's
	// flag input being wrong, so it's a usage error (ExitUsage), not an
	// "operation ran and reported a problem" (ExitProblem).
	result, err := omnist.Extract(schema, keepMap)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	if err := writeOutput(*out, osd.Write(result, false), stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	return ExitOK
}

// schemaBoolean implements the shared shape of `schema compatible-with`
// and `schema equivalent`: read two SCHEMA arguments, run a
// (Schema, Schema) -> bool comparison, print "true" or "false", always
// exit 0 (see exit.go for why).
func schemaBoolean(args []string, stdin io.Reader, stdout, stderr io.Writer, name string, compare func(a, b omnist.Schema) bool) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "usage: %s A B\n", name)
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: expected exactly two SCHEMA arguments\n", name)
		fs.Usage()
		return ExitUsage
	}
	a, code, ok := loadSchema(name, fs.Arg(0), stdin, stderr)
	if !ok {
		return code
	}
	b, code, ok := loadSchema(name, fs.Arg(1), stdin, stderr)
	if !ok {
		return code
	}
	_, _ = fmt.Fprintln(stdout, compare(a, b))
	return ExitOK
}

// cmdSchemaIsEmpty implements `omnist schema is-empty`.
func cmdSchemaIsEmpty(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	const name = "omnist schema is-empty"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "usage: %s SCHEMA\n", name)
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: expected exactly one SCHEMA argument\n", name)
		fs.Usage()
		return ExitUsage
	}
	schema, code, ok := loadSchema(name, fs.Arg(0), stdin, stderr)
	if !ok {
		return code
	}
	_, _ = fmt.Fprintln(stdout, omnist.IsEmpty(schema))
	return ExitOK
}
