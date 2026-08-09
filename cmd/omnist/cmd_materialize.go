package main

import (
	"flag"
	"fmt"
	"io"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/osd"
)

// cmdMaterialize implements `omnist materialize`: stage-1 read INPUT,
// read the OSD --schema, run Materialize, print its diagnostics one per
// line, and -- only if the caller asked for output via --to and/or -o --
// also serialize the materialized document. Diagnostics are printed
// regardless of whether output was requested, since Materialize is
// best-effort: it returns a document even when diagnostics were raised.
func cmdMaterialize(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("omnist materialize", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "input format: json, yaml, toml, xml, oml (required)")
	schemaPath := fs.String("schema", "", "OSD schema file (required)")
	to := fs.String("to", "", "output format for the materialized document (implies output; defaults to --from if -o is given)")
	out := fs.String("o", "", "output file for the materialized document (default: stdout if --to is given)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: omnist materialize --from FORMAT --schema SCHEMA [--to FORMAT] [-o FILE] INPUT")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "omnist materialize: expected exactly one INPUT argument")
		fs.Usage()
		return ExitUsage
	}
	input := fs.Arg(0)
	if *from == "" {
		_, _ = fmt.Fprintln(stderr, "omnist materialize: --from is required")
		return ExitUsage
	}
	if *schemaPath == "" {
		_, _ = fmt.Fprintln(stderr, "omnist materialize: --schema is required")
		return ExitUsage
	}

	reader, err := lookupReader(*from)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", err)
		return ExitUsage
	}

	wantsOutput := *to != "" || *out != ""
	var writer formatWriterFunc
	if wantsOutput {
		toFormat := *to
		if toFormat == "" {
			toFormat = *from
		}
		writer, err = lookupWriter(toFormat)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", err)
			return ExitUsage
		}
	}

	text, err := readInput(input, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", err)
		return ExitUsage
	}
	schemaText, err := readInput(*schemaPath, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", err)
		return ExitUsage
	}

	doc, err := reader(text, omnist.DefaultLimits())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", err)
		return ExitProblem
	}
	schema, err := osd.Read(schemaText)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", err)
		return ExitProblem
	}

	result, diags, err := omnist.Materialize(doc, schema)
	for _, d := range diags {
		_, _ = fmt.Fprintf(stdout, "%s: %s: %s\n", d.Path, d.Code, d.Message)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", err)
		return ExitProblem
	}

	writeDiagCount := 0
	if wantsOutput {
		outText, writeDiags, werr := writer(result)
		if werr != nil {
			_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", werr)
			return ExitProblem
		}
		// Same non-fatal-adjustment channel as cmdParse -- a writer can
		// succeed while reporting one (issue #49, spec §8.5.3).
		for _, d := range writeDiags {
			_, _ = fmt.Fprintf(stderr, "omnist materialize: %s: %s: %s\n", d.Path, d.Code, d.Message)
		}
		writeDiagCount = len(writeDiags)
		if werr := writeOutput(*out, outText, stdout); werr != nil {
			_, _ = fmt.Fprintf(stderr, "omnist materialize: %v\n", werr)
			return ExitUsage
		}
	}

	if len(diags) > 0 || writeDiagCount > 0 {
		return ExitProblem
	}
	return ExitOK
}
