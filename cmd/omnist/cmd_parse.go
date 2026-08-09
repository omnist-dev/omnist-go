package main

import (
	"flag"
	"fmt"
	"io"

	omnist "github.com/omnist-dev/omnist-go"
)

// cmdParse implements `omnist parse`: stage-1 read INPUT in --from
// format, then re-serialize (optionally to a different --to format).
// This is the CLI's format-conversion command: json/yaml/toml/xml/oml
// interchange without needing a schema.
func cmdParse(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("omnist parse", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "input format: json, yaml, toml, xml, oml (required)")
	to := fs.String("to", "", "output format (defaults to --from)")
	out := fs.String("o", "", "output file (default: stdout)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: omnist parse --from FORMAT [--to FORMAT] [-o FILE] INPUT")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "omnist parse: expected exactly one INPUT argument")
		fs.Usage()
		return ExitUsage
	}
	input := fs.Arg(0)
	if *from == "" {
		_, _ = fmt.Fprintln(stderr, "omnist parse: --from is required")
		return ExitUsage
	}
	toFormat := *to
	if toFormat == "" {
		toFormat = *from
	}

	reader, err := lookupReader(*from)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist parse: %v\n", err)
		return ExitUsage
	}
	writer, err := lookupWriter(toFormat)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist parse: %v\n", err)
		return ExitUsage
	}
	text, err := readInput(input, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist parse: %v\n", err)
		return ExitUsage
	}

	doc, err := reader(text, omnist.DefaultLimits())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist parse: %v\n", err)
		return ExitProblem
	}
	outText, diags, err := writer(doc)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist parse: %v\n", err)
		return ExitProblem
	}
	// A writer can succeed while still reporting a non-fatal adjustment
	// (issue #49, spec §8.5.3's write-only ok:true+diagnostics
	// coexistence) -- print each one to stderr, mirroring cmdMaterialize's
	// established diagnostic-printing convention, without treating it as
	// a failure: the output was still produced and is still written below.
	for _, d := range diags {
		_, _ = fmt.Fprintf(stderr, "omnist parse: %s: %s: %s\n", d.Path, d.Code, d.Message)
	}
	if err := writeOutput(*out, outText, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist parse: %v\n", err)
		return ExitUsage
	}
	if len(diags) > 0 {
		return ExitProblem
	}
	return ExitOK
}
