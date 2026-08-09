package main

// Exit code convention for this CLI. There is no spec mandate for CLI
// exit codes (omnist-spec documents Python's CLI shape as one worked
// example, not a binding contract -- see issue #37), so this convention
// is this port's own deliberate choice, not a copy of Python's.
//
//	0  success -- the requested operation ran and found nothing to
//	   report.
//	1  usage/tool error -- the CLI itself could not carry out the
//	   request: bad or missing flags, an unrecognized subcommand, an
//	   unrecognized --from/--to format name, an input/output file that
//	   could not be opened, or a --keep set naming a label the schema
//	   does not have. The underlying library operation never ran.
//	2  operation problem -- the operation ran to completion but its
//	   result reports something worth the caller's attention: a parse
//	   error against malformed input, validate/materialize diagnostics,
//	   an infer failure (e.g. conflicting scalar kinds across samples),
//	   or lint findings at error/warning severity. This is distinct
//	   from a Go panic or an unhandled error escaping to main: it is
//	   the CLI faithfully reporting a real finding about the input
//	   data, the same way a linter or type-checker's nonzero exit
//	   means "ran fine, found problems" rather than "crashed."
//
// The three boolean-result schema commands (schema compatible-with,
// schema equivalent, schema is-empty) are a deliberate exception: they
// always exit 0 and print "true" or "false" to stdout. Python's CLI
// instead folds the boolean into the exit code itself (0/1). This port
// chooses not to: reserving exit codes for the usage-vs-problem
// distinction above, and letting a boolean result live on stdout where a
// caller greps for it, was judged less surprising than overloading exit
// 1 with two unrelated meanings ("you invoked me wrong" and "the
// schemas are not compatible"). A caller who wants the boolean as an
// exit code can wrap the command: omnist schema is-empty S.osd | grep
// -q true.
const (
	ExitOK      = 0
	ExitUsage   = 1
	ExitProblem = 2
)
