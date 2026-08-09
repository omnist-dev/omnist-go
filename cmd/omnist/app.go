// Package main implements omnist, a thin CLI wrapper over this repo's
// exported library functions: the stage-1 readers/writers, OSD
// read/write, Validate, Materialize, and the schema algebra (Normalize,
// Prune, Extract, CompatibleWith, Equivalent, IsEmpty, Infer, Lint).
//
// There is no spec-mandated CLI contract (omnist-spec documents Python's
// CLI shape as one worked example of a binding, not a mandate -- see
// issue #37's discussion). This CLI's shape, flag names, and exit-code
// convention are this port's own deliberate design, described in
// exit.go and in each subcommand's --help text.
//
// CLI framework: this package uses only the stdlib flag package, one
// flag.FlagSet per subcommand. A third-party framework (e.g. cobra) was
// considered and rejected: this CLI has a shallow, one-level-deep
// subcommand tree (schema is the only subcommand with sub-subcommands),
// no need for shell-completion generation, and flag.FlagSet's
// ContinueOnError mode plus a small hand-written dispatcher in run()
// below covers per-subcommand flags cleanly without adding an external
// dependency for a CLI this size.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the CLI's argument-parsing-and-dispatch entry point, kept
// separate from main() so tests can invoke it directly (with fake
// stdin/stdout/stderr) without spawning a subprocess for every case. A
// smaller number of real subprocess tests (subprocess_test.go) cover the
// actual compiled binary end to end.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printTopLevelUsage(stderr)
		return ExitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		printTopLevelUsage(stdout)
		return ExitOK
	case "parse":
		return cmdParse(args[1:], stdin, stdout, stderr)
	case "validate":
		return cmdValidate(args[1:], stdin, stdout, stderr)
	case "materialize":
		return cmdMaterialize(args[1:], stdin, stdout, stderr)
	case "schema":
		return cmdSchema(args[1:], stdin, stdout, stderr)
	case "infer":
		return cmdInfer(args[1:], stdin, stdout, stderr)
	case "lint":
		return cmdLint(args[1:], stdin, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "omnist: unknown subcommand %q\n\n", args[0])
		printTopLevelUsage(stderr)
		return ExitUsage
	}
}

func printTopLevelUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `omnist -- read, write, validate, and materialize Omnist documents and
schemas (JSON/YAML/TOML/XML/OML documents, OSD schemas).

Usage:
  omnist parse --from FORMAT [--to FORMAT] [-o FILE] INPUT
  omnist validate --from FORMAT --schema SCHEMA INPUT
  omnist materialize --from FORMAT --schema SCHEMA [--to FORMAT] [-o FILE] INPUT
  omnist schema normalize [-o FILE] SCHEMA
  omnist schema prune [-o FILE] SCHEMA
  omnist schema extract --keep label1,label2,... [-o FILE] SCHEMA
  omnist schema compatible-with A B
  omnist schema equivalent A B
  omnist schema is-empty SCHEMA
  omnist infer --from FORMAT [--allow-any] [-o FILE] FILE [FILE...]
  omnist lint SCHEMA

NOTE: flags must come before the positional INPUT/SCHEMA/FILE argument(s)
-- a stdlib flag.FlagSet limitation (it stops parsing flags at the first
non-flag argument, unlike getopt-style permutation). This was accepted
as a reasonable tradeoff for staying dependency-free; see the package
doc comment above for the framework choice.

INPUT, SCHEMA, and FILE accept "-" (or, for INPUT, an omitted argument in
some commands) to mean stdin; -o omitted means stdout.

Formats: json, yaml, toml, xml, oml.

Exit codes: 0 success, 1 usage/tool error, 2 operation reported a
problem (parse error, validation/materialize diagnostics, infer
failure, lint findings). See exit.go's doc comment for the full
convention. The three boolean schema commands (compatible-with,
equivalent, is-empty) always exit 0 and print "true"/"false".

Run "omnist SUBCOMMAND -h" for a subcommand's own flags.
`)
}
