package omnist

import "testing"

// TestParseISOTimeNegativeOffset exercises ParseISOTime's negative-offset
// branch (rest[0] == '-') directly. Before issue #43 moved oml_writer_test.go
// into the separate oml package, this branch was covered incidentally by
// TestOMLRoundTripProperty's "time ... with offset" case (t5,
// OffsetSeconds: -1800) round-tripping through the OML writer and lexer —
// which still exercises this same root-package ParseISOTime function via
// oml.Read, but Go's per-package coverage only attributes execution to the
// package whose own test binary is running, so that exercise no longer
// counts toward this package's own coverage number now that it runs inside
// package oml's test binary instead of this one's. This direct test
// restores this package's self-sufficient 100% coverage of its own
// function, independent of which downstream package happens to call it.
func TestParseISOTimeNegativeOffset(t *testing.T) {
	got := ParseISOTime("10:30:00-05:30")
	want := TimeValue{Hour: 10, Minute: 30, Second: 0, HasOffset: true, OffsetSeconds: -(5*3600 + 30*60)}
	if got != want {
		t.Errorf("ParseISOTime(%q) = %+v, want %+v", "10:30:00-05:30", got, want)
	}
}

// TestFormatISOTimeNegativeOffset exercises FormatISOTime's negative-
// offset branch (off < 0) directly, for the same reason
// TestParseISOTimeNegativeOffset above exists: before issue #45 moved
// json_writer_test.go into the separate formats/json package, this
// branch was covered incidentally by that file's "with negative offset"
// case (OffsetSeconds: -1800), the only writer test in the repo that
// exercised a negative UTC offset (yaml_writer_test.go and
// toml_writer_test.go's own offset cases are both positive). Once
// json_writer_test.go's coverage stopped counting toward this package's
// own coverage number (same per-package attribution rule the comment
// above explains), this branch dropped below 100% -- caught by the
// per-function coverage check issue #45 called for, the same class of
// regression issue #43 found on the parse side.
func TestFormatISOTimeNegativeOffset(t *testing.T) {
	got := FormatISOTime(TimeValue{Hour: 1, Minute: 2, HasOffset: true, OffsetSeconds: -1800})
	want := "01:02-00:30"
	if got != want {
		t.Errorf("FormatISOTime(...) = %q, want %q", got, want)
	}
}
