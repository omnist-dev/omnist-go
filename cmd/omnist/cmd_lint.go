package main

import (
	"flag"
	"fmt"
	"io"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
)

// cmdLint implements `omnist lint`: read one SCHEMA and print each
// Lint(s) Finding as "location: severity: code: message". Findings are
// never SeverityError (lint's own taxonomy only produces warning/info,
// per lint.go's doc comment), so this command's problem/no-problem line
// is drawn at warning: any warning-level finding makes this an
// operation-reported-a-problem (ExitProblem); a schema with only
// info-level findings (or none) exits 0, since info findings are
// advisory rather than something-is-wrong.
func cmdLint(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	const name = "omnist lint"
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

	findings := algebra.Lint(schema)
	problem := false
	for _, f := range findings {
		_, _ = fmt.Fprintf(stdout, "%s: %s: %s: %s\n", f.Location, f.Severity, f.Code, f.Message)
		if f.Severity <= omnist.SeverityWarning {
			problem = true
		}
	}
	if problem {
		return ExitProblem
	}
	return ExitOK
}
