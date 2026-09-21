package main

import (
	"strings"
	"testing"
)

// The CLI is a byte-oriented entry point (spec §2.5): it reads bytes from a
// file or stdin and MUST NOT repair them. Go's string(bytes) is lossless, so
// the reader behind it does the D-14 rejection; these tests pin that the
// failure reaches the user as parse.invalid-encoding at 1:1 (exit 2, "the
// operation ran and reported a problem"), on stdin and on a file, for every
// format, rather than as a silently repaired read or a codec error.
func TestCLIRejectsInvalidUTF8OnEveryFormat(t *testing.T) {
	cases := []struct{ format, body string }{
		{"json", "{\"a\": \"x\xe2\x82y\"}"},
		{"yaml", "a: \"x\xed\xa0\x80y\"\n"},
		{"toml", "a = \"x\xf5\x80\x80\x80y\"\n"},
		{"xml", "<r><a>x\xe2\x82y</a></r>"},
		{"oml", "a: \"x\xc0\xafy\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.format+"/stdin", func(t *testing.T) {
			code, stdout, stderr := runCLI(t, []string{"parse", "--from", tc.format, "--to", "json", "-"}, tc.body)
			if code != ExitProblem || stdout != "" {
				t.Errorf("exit = %d stdout = %q, want %d and no output", code, stdout, ExitProblem)
			}
			if !strings.Contains(stderr, "1:1: parse.invalid-encoding") {
				t.Errorf("stderr = %q, want parse.invalid-encoding at 1:1", stderr)
			}
		})
		t.Run(tc.format+"/file", func(t *testing.T) {
			f := writeTemp(t, "in."+tc.format, tc.body)
			code, _, stderr := runCLI(t, []string{"parse", "--from", tc.format, "--to", "json", f}, "")
			if code != ExitProblem || !strings.Contains(stderr, "1:1: parse.invalid-encoding") {
				t.Errorf("exit = %d stderr = %q, want %d and parse.invalid-encoding at 1:1", code, stderr, ExitProblem)
			}
		})
	}
}

func TestCLIRejectsInvalidUTF8InASchemaFile(t *testing.T) {
	bad := "record R {\n    \"a\x80b\": string,\n}\nroot R\n"
	f := writeTemp(t, "s.osd", bad)
	for _, args := range [][]string{
		{"schema", "normalize", f},
		{"schema", "prune", f},
		{"schema", "is-empty", f},
		{"lint", f},
	} {
		code, _, stderr := runCLI(t, args, "")
		if code != ExitProblem || !strings.Contains(stderr, "1:1: parse.invalid-encoding") {
			t.Errorf("%v: exit = %d stderr = %q, want %d and parse.invalid-encoding at 1:1", args, code, stderr, ExitProblem)
		}
	}
}

func TestCLIAcceptsValidMultiByteInput(t *testing.T) {
	code, stdout, stderr := runCLI(t, []string{"parse", "--from", "json", "--to", "json", "-"}, "{\"k\": \"\xc3\xa9\xe2\x82\xac\xf0\x9f\x98\x80\"}")
	if code != ExitOK || !strings.Contains(stdout, "\xc3\xa9\xe2\x82\xac\xf0\x9f\x98\x80") {
		t.Errorf("exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
}

// OSD-14 through the one CLI route that can reach it: `infer` builds a schema
// from document keys, and a JSON key may carry a C0 control character. The
// write is refused (exit 2, write.unsupported-value at the record path) and
// nothing is emitted, rather than printing OSD text the reader would reject.
func TestCLIInferRefusesAKeyWithNoOSDSpelling(t *testing.T) {
	f := writeTemp(t, "a.json", "{\"a\\u0001b\": 1}")
	code, stdout, stderr := runCLI(t, []string{"infer", "--from", "json", f}, "")
	if code != ExitProblem || stdout != "" {
		t.Errorf("exit = %d stdout = %q, want %d and no output", code, stdout, ExitProblem)
	}
	if !strings.Contains(stderr, "write.unsupported-value") || !strings.Contains(stderr, "Root: ") {
		t.Errorf("stderr = %q, want write.unsupported-value at the record path", stderr)
	}
	if strings.Contains(stderr, "\x01") {
		t.Errorf("the control byte must not be echoed: %q", stderr)
	}
}
