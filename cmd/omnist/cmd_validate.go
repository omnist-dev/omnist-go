package main

import (
	"flag"
	"fmt"
	"io"

	omnist "github.com/omnist-dev/omnist-go"
)

// cmdValidate implements `omnist validate`: stage-1 read INPUT, read the
// OSD --schema, and run Validate, printing each Diagnostic one per line
// as "path: code: message". Prints nothing when validation finds
// nothing to report.
func cmdValidate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("omnist validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "input format: json, yaml, toml, xml, oml (required)")
	schemaPath := fs.String("schema", "", "OSD schema file (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: omnist validate --from FORMAT --schema SCHEMA INPUT")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "omnist validate: expected exactly one INPUT argument")
		fs.Usage()
		return ExitUsage
	}
	input := fs.Arg(0)
	if *from == "" {
		_, _ = fmt.Fprintln(stderr, "omnist validate: --from is required")
		return ExitUsage
	}
	if *schemaPath == "" {
		_, _ = fmt.Fprintln(stderr, "omnist validate: --schema is required")
		return ExitUsage
	}

	reader, err := lookupReader(*from)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist validate: %v\n", err)
		return ExitUsage
	}
	text, err := readInput(input, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist validate: %v\n", err)
		return ExitUsage
	}
	schemaText, err := readInput(*schemaPath, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist validate: %v\n", err)
		return ExitUsage
	}

	doc, err := reader(text, omnist.DefaultLimits())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist validate: %v\n", err)
		return ExitProblem
	}
	schema, err := omnist.ReadOSD(schemaText)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "omnist validate: %v\n", err)
		return ExitProblem
	}

	diags := omnist.Validate(doc, schema)
	for _, d := range diags {
		_, _ = fmt.Fprintf(stdout, "%s: %s: %s\n", d.Path, d.Code, d.Message)
	}
	if len(diags) > 0 {
		return ExitProblem
	}
	return ExitOK
}
