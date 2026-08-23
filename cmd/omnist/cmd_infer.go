package main

import (
	"flag"
	"fmt"
	"io"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
	"github.com/omnist-dev/omnist-go/osd"
)

// cmdInfer implements `omnist infer`: read one or more sample documents
// in --from format and draft an OSD schema from them via
// InferWithReport. With --allow-any, fields that mix shapes or scalar
// kinds are opened to `any` instead of failing; any such fallbacks are
// reported to stderr as informational (exit 0 still, since --allow-any
// is the caller opting into that lenience).
func cmdInfer(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	const name = "omnist infer"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "input format: json, yaml, toml, xml, oml (required)")
	allowAny := fs.Bool("allow-any", false, "open mixed-shape/mixed-scalar-kind fields to `any` instead of failing")
	out := fs.String("o", "", "output file (default: stdout)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "usage: %s --from FORMAT [--allow-any] [-o FILE] FILE [FILE...]\n", name)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if fs.NArg() == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: expected at least one FILE argument\n", name)
		fs.Usage()
		return ExitUsage
	}
	if *from == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --from is required\n", name)
		return ExitUsage
	}

	reader, err := lookupReader(*from)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}

	samples := make([]omnist.Document, 0, fs.NArg())
	for _, path := range fs.Args() {
		text, rerr := readInput(path, stdin)
		if rerr != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, rerr)
			return ExitUsage
		}
		doc, readDiags, perr := reader(text, omnist.DefaultLimits())
		if perr != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %s: %v\n", name, path, perr)
			return ExitProblem
		}
		// A reader can succeed while still reporting a non-fatal
		// adjustment (D-3, spec §8.3.8) -- surfaced the same
		// informational way as --allow-any's fallbacks below, since
		// infer's contract is best-effort schema drafting, not
		// round-trip fidelity of the samples themselves.
		for _, d := range readDiags {
			_, _ = fmt.Fprintf(stderr, "%s: %s: %s: %s: %s\n", name, path, d.Path, d.Code, d.Message)
		}
		samples = append(samples, doc)
	}

	schema, fallbacks, err := algebra.InferWithReport(samples, "", *allowAny)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitProblem
	}
	for _, fb := range fallbacks {
		_, _ = fmt.Fprintf(stderr, "%s: opened to any: %s\n", fb.Location, fb.Reason)
	}
	if err := writeOutput(*out, osd.Write(schema, false), stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return ExitUsage
	}
	return ExitOK
}
