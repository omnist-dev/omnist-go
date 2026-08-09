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
