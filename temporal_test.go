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

// TestFormatISOTimeFraction exercises FormatISOTime's fractional-second
// rendering branch (t.Nanosecond != 0), which also is FormatISOFraction's
// only caller in this package. Before issue #45 moved yaml_writer_test.go
// (and, earlier, json_writer_test.go) into their own formats/* packages,
// this branch was covered incidentally by their own "with-fraction"/
// "with fraction" writer test cases -- toml_writer_test.go and
// xml_writer_test.go's own time fixtures never happen to use a nonzero
// Nanosecond. Once yaml_writer_test.go's coverage stopped counting toward
// this package's own number (same per-package attribution rule
// TestParseISOTimeNegativeOffset's comment explains), FormatISOTime's
// fraction branch and all of FormatISOFraction dropped to 0% -- the same
// class of regression again, caught by issue #45's per-function coverage
// check.
func TestFormatISOTimeFraction(t *testing.T) {
	got := FormatISOTime(TimeValue{Hour: 1, Minute: 2, Second: 3, Nanosecond: 500000000})
	want := "01:02:03.5"
	if got != want {
		t.Errorf("FormatISOTime(...) = %q, want %q", got, want)
	}
}

// TestParseISOTimeFraction exercises ParseISOTimes fractional-second
// branch (rest[end] digits after the dot), which is also FracToNanoss
// only caller in this package. Before issue #45 moved toml_reader_test.go
// into the separate formats/toml package, this branch was covered
// incidentally by that files own fractional-second datetime fixtures
// (via parseTOMLDateTime -> ParseISOTime); no other root-package test
// parses a fractional-second ISO time directly. Same class of regression
// as TestParseISOTimeNegativeOffset/TestFormatISOTimeFraction above,
// caught by issue #45s per-function coverage check.
func TestParseISOTimeFraction(t *testing.T) {
	got := ParseISOTime("10:30:00.5")
	want := TimeValue{Hour: 10, Minute: 30, Second: 0, Nanosecond: 500000000}
	if got != want {
		t.Errorf("ParseISOTime(%q) = %+v, want %+v", "10:30:00.5", got, want)
	}
}

// TestFormatISODate exercises FormatISODate directly. Before issue #45
// moved every one of json/yaml/toml/xml_writer_test.go into their own
// formats/* packages, FormatISODate was covered incidentally by each of
// their own date-rendering test cases; once all four writer test files
// moved out in the same issue, none of this packages own tests called
// FormatISODate anymore -- the same class of regression as this files
// other Test<Something>ISO* tests above, just delayed until the fourth
// (and last) codec moved rather than caught by an earlier one.
func TestFormatISODate(t *testing.T) {
	got := FormatISODate(DateValue{Year: 2024, Month: 1, Day: 2})
	want := "2024-01-02"
	if got != want {
		t.Errorf("FormatISODate(...) = %q, want %q", got, want)
	}
}

// TestMatchesISOKindEmptyString exercises MatchesISOKind's empty-string
// guard directly, for the same cross-package coverage-attribution reason
// as the tests above: formats/toml's fuzz-found regression tests (issue
// #57) call this guard by way of parseTOMLDateTime, but that only
// instruments formats/toml's own coverage, not this package's. Found via
// fuzzing: regexp.FindString("") == "" whether or not the regex actually
// matched, so without this guard an empty string would spuriously
// "match" every kind.
// --- ValidDate/ValidTime/ValidOffsetText (spec section 4.2.4, issue #102) ---
//
// Direct tests restoring this package's own coverage of these functions,
// independent of which downstream package (oml) happens to call them --
// see TestParseISOTimeNegativeOffset's doc comment above for why that
// matters given Go's per-package coverage attribution.

func TestValidDate(t *testing.T) {
	cases := []struct {
		name string
		d    DateValue
		want bool
	}{
		{"ordinary date", DateValue{Year: 2024, Month: 6, Day: 15}, true},
		{"month zero", DateValue{Year: 2024, Month: 0, Day: 1}, false},
		{"month 13", DateValue{Year: 2024, Month: 13, Day: 1}, false},
		{"april 31 (30-day month)", DateValue{Year: 2024, Month: 4, Day: 31}, false},
		{"december 31", DateValue{Year: 2024, Month: 12, Day: 31}, true},
		{"leap day in leap year", DateValue{Year: 2000, Month: 2, Day: 29}, true},
		{"leap day in non-leap century year", DateValue{Year: 1900, Month: 2, Day: 29}, false},
		{"leap day in ordinary non-leap year", DateValue{Year: 2023, Month: 2, Day: 29}, false},
		{"feb 28 in non-leap year", DateValue{Year: 2023, Month: 2, Day: 28}, true},
		{"day zero", DateValue{Year: 2024, Month: 1, Day: 0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidDate(tc.d); got != tc.want {
				t.Errorf("ValidDate(%+v) = %v, want %v", tc.d, got, tc.want)
			}
		})
	}
}

func TestValidTime(t *testing.T) {
	cases := []struct {
		name string
		t    TimeValue
		want bool
	}{
		{"ordinary time", TimeValue{Hour: 10, Minute: 30, Second: 15}, true},
		{"hour 23 is valid", TimeValue{Hour: 23, Minute: 0, Second: 0}, true},
		{"hour 24 is invalid", TimeValue{Hour: 24, Minute: 0, Second: 0}, false},
		{"minute 59 is valid", TimeValue{Hour: 0, Minute: 59, Second: 0}, true},
		{"minute 60 is invalid", TimeValue{Hour: 0, Minute: 60, Second: 0}, false},
		{"second 59 is valid", TimeValue{Hour: 0, Minute: 0, Second: 59}, true},
		{"leap second 60 is invalid", TimeValue{Hour: 23, Minute: 59, Second: 60}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidTime(tc.t); got != tc.want {
				t.Errorf("ValidTime(%+v) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}

func TestValidOffsetText(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{"no offset", "10:30:00", true},
		{"offset within range", "10:30:00+05:30", true},
		{"negative offset within range", "10:30:00-05:30", true},
		{"offset hour out of range", "10:30+24:00", false},
		{"offset minute out of range (the real bug)", "10:30+00:60", false},
		{"datetime with offset minute out of range", "2024-01-01T10:30+00:60", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidOffsetText(tc.s); got != tc.want {
				t.Errorf("ValidOffsetText(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

func TestMatchesISOKindEmptyString(t *testing.T) {
	for _, kind := range []TemporalKind{TemporalDate, TemporalTime, TemporalDateTime} {
		if MatchesISOKind("", kind) {
			t.Errorf("MatchesISOKind(\"\", %v) = true, want false", kind)
		}
	}
}
