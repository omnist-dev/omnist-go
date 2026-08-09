package omnist

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ISODateTimeRegexp, ISODateRegexp, and ISOTimeRegexp are the exact,
// start-anchored regexes the OML lexer uses (spec §4.2's DATETIME/DATE/
// TIME productions) to recognize the three temporal token kinds. They are
// exported because multiple packages beyond the OML lexer itself need the
// identical spelling: a codec reader deciding whether a source string is
// "exactly" a date/time/datetime (see MatchesISOKind), or any future
// package that needs to recognize this exact literal shape without
// duplicating — and risking drifting from — the OML grammar's own regex.
//
// Each regex is anchored only at the start ('^'), not the end: that is
// correct for the OML lexer's own use (FindString on a remaining-input
// tail, consuming a prefix while scanning forward through a longer
// document) but means a caller that needs a *whole-string* match — is
// this entire string exactly a DATE, not just prefixed by one — must
// check that explicitly, e.g. via MatchesISOKind, rather than trusting a
// non-empty FindString result on its own.
var (
	ISODateTimeRegexp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?([+-]\d{2}:\d{2})?`)
	ISODateRegexp     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
	ISOTimeRegexp     = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?([+-]\d{2}:\d{2})?`)
)

// TemporalKind selects which of ISODateRegexp/ISOTimeRegexp/
// ISODateTimeRegexp MatchesISOKind checks a string against.
type TemporalKind int

const (
	TemporalDate TemporalKind = iota
	TemporalTime
	TemporalDateTime
)

// MatchesISOKind reports whether s is *exactly* (start to end) the
// spelling ISODateRegexp/ISOTimeRegexp/ISODateTimeRegexp accepts for
// kind — the "exact spelling matches_kind() accepts for that specific
// kind, not merely parseable by a looser library function" rule from
// spec §7.2's try_upgrade notes. The underlying regexes are only
// start-anchored (correct for a lexer scanning forward through a longer
// document via FindString on a remaining-input tail, which pins the
// *start* of a match but not the end), so this wraps FindString with an
// explicit "the match is the whole string" comparison rather than
// introducing new regexes with different anchoring — that would be
// exactly the kind of drift spec §7.2 warns against: reuse the same
// format-matching logic rather than a new, possibly looser or stricter
// parser.
//
// This is deliberately the OML lexer's spelling (§4's grammar), not, for
// example, a TOML/YAML reader's own wider temporal parsing (TOML and YAML
// both allow separators/offsets OML's grammar does not). Callers reading
// from an arbitrary Document (JSON, YAML, or direct construction) should
// use this canonical spelling per §7.2's rule of a single canonical
// spelling per kind — the OML lexer's spelling is that spelling, since
// OML is the format whose native literal syntax the temporal scalar kinds
// were designed around (§2.2.1/§4).
func MatchesISOKind(s string, kind TemporalKind) bool {
	// Found via fuzzing (issue #57): regexp.FindString returns "" both when
	// there is no match and when the match itself is the empty string, so
	// without this guard an empty s would spuriously "match" every kind
	// (none of ISODateRegexp/ISOTimeRegexp/ISODateTimeRegexp can ever
	// legitimately match "" — they all require at least a fixed run of
	// digits). Reject it upfront rather than relying on that ambiguous
	// equality check to happen to come out false.
	if s == "" {
		return false
	}
	var re *regexp.Regexp
	switch kind {
	case TemporalDate:
		re = ISODateRegexp
	case TemporalTime:
		re = ISOTimeRegexp
	case TemporalDateTime:
		re = ISODateTimeRegexp
	}
	return re.FindString(s) == s
}

// --- temporal value construction ---
//
// ParseISODate/ParseISOTime/ParseISODateTime are only ever meant to be
// called on text already matched by ISODateRegexp/ISOTimeRegexp/
// ISODateTimeRegexp (or a superset grammar that pins the same digit
// layout, e.g. a TOML/YAML reader's own wider temporal regex reusing
// these for the date/time portions), which fully pin the digit layout
// these functions assume (down to field widths). Given that precondition
// the Sscanf/index calls below cannot fail, so — unlike other decoders in
// this package — these deliberately have no error return: an
// error-returning version would carry a permanently-dead "malformed"
// branch that no input could ever reach (the regex guarantees the
// format), which is worse for coverage and for readability than asserting
// the precondition in this comment once, here.

// ParseISODate parses s, which must already match ISODateRegexp, into a
// DateValue.
func ParseISODate(s string) DateValue {
	var y, mo, d int
	_, _ = fmt.Sscanf(s, "%4d-%2d-%2d", &y, &mo, &d)
	return DateValue{Year: y, Month: mo, Day: d}
}

// ParseISOTime parses s, which must already match ISOTimeRegexp, into a
// TimeValue.
func ParseISOTime(s string) TimeValue {
	rest := s
	var hh, mm int
	_, _ = fmt.Sscanf(rest, "%2d:%2d", &hh, &mm)
	rest = rest[5:]
	tv := TimeValue{Hour: hh, Minute: mm}
	if strings.HasPrefix(rest, ":") {
		var ss int
		_, _ = fmt.Sscanf(rest[1:], "%2d", &ss)
		tv.Second = ss
		rest = rest[3:]
		if strings.HasPrefix(rest, ".") {
			end := 1
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			tv.Nanosecond = FracToNanos(rest[1:end])
			rest = rest[end:]
		}
	}
	if len(rest) == 6 && (rest[0] == '+' || rest[0] == '-') {
		var oh, om int
		_, _ = fmt.Sscanf(rest[1:], "%2d:%2d", &oh, &om)
		sign := 1
		if rest[0] == '-' {
			sign = -1
		}
		tv.HasOffset = true
		tv.OffsetSeconds = sign * (oh*3600 + om*60)
	}
	return tv
}

// ParseISODateTime parses s, which must already match ISODateTimeRegexp,
// into a DateTimeValue.
func ParseISODateTime(s string) DateTimeValue {
	idx := strings.IndexByte(s, 'T')
	return DateTimeValue{Date: ParseISODate(s[:idx]), Time: ParseISOTime(s[idx+1:])}
}

// FracToNanos converts a fractional-second digit string (1-6 digits, as
// matched by the grammar) to nanoseconds, right-padding to 9 digits.
// Exported (issue #45) once formats/yaml's reader turned out to need it
// directly for its own sexagesimal/fractional-time parsing (parseYAMLDateTime),
// a real cross-codec dependency this project's package-restructuring plan
// had not accounted for -- the same class of gap FormatISODate/
// FormatISOTime/FormatISOFraction above were promoted to fix.
func FracToNanos(digits string) int {
	padded := (digits + "000000000")[:9]
	n, _ := strconv.Atoi(padded)
	return n
}

// FormatISODate renders a DateValue per ISO-8601's calendar-date form
// (YYYY-MM-DD). Promoted here (issue #45) from json_writer.go, which
// originally defined it privately as formatISODate; yaml_writer.go,
// toml_writer.go, and xml_writer.go all called that same private
// function directly (a real, load-bearing cross-codec dependency this
// project's package-restructuring plan had described as not existing —
// discovered while moving JSON into its own formats/json package, since
// that move is what first made the dependency across package boundaries
// visible). oml_writer.go's formatOMLDate is a separate, OML-specific
// copy and is deliberately not merged into this one: OML's writer lives
// in its own already-moved package and duplicating one four-line
// function is preferable to adding a needless production dependency from
// oml on the root package's temporal.go beyond what it already uses.
func FormatISODate(d DateValue) string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// FormatISOTime renders a TimeValue per ISO-8601's time-of-day form.
// Seconds are emitted only when Second or Nanosecond is nonzero, and the
// fractional part only when Nanosecond is nonzero. Promoted here (issue
// #45) alongside FormatISODate, for the same cross-codec reason.
func FormatISOTime(t TimeValue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%02d:%02d", t.Hour, t.Minute)
	if t.Second != 0 || t.Nanosecond != 0 {
		fmt.Fprintf(&b, ":%02d", t.Second)
		if t.Nanosecond != 0 {
			b.WriteByte('.')
			b.WriteString(FormatISOFraction(t.Nanosecond))
		}
	}
	if t.HasOffset {
		off := t.OffsetSeconds
		sign := byte('+')
		if off < 0 {
			sign = '-'
			off = -off
		}
		fmt.Fprintf(&b, "%c%02d:%02d", sign, off/3600, (off/60)%60)
	}
	return b.String()
}

// FormatISOFraction converts a Nanosecond field to the shortest 1-6 digit
// fraction string that reproduces it. Nanosecond is always a product of
// FracToNanos above, which right-pads to 9 digits, so it is always an
// exact multiple of 1000, and trimming trailing zeros from its 6-digit
// microsecond form can never trim down to nothing given the caller's
// guard that Nanosecond != 0. Promoted here (issue #45) alongside
// FormatISODate/FormatISOTime, for the same cross-codec reason.
func FormatISOFraction(ns int) string {
	micros := ns / 1000
	s := fmt.Sprintf("%06d", micros)
	return strings.TrimRight(s, "0")
}
