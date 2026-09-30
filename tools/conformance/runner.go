package conformance

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	omnist "github.com/omnist-dev/omnist-go"
)

// Result is one vector's outcome.
type Result struct {
	Vector Vector
	Status Status
	// Reason is populated for Skip (a cited reason, per §8.5.5) and Fail
	// (a short description of what mismatched) results.
	Reason string
}

// Status is a vector's pass/fail/skip outcome, per §8.5.5.
type Status int

const (
	StatusPass Status = iota
	StatusFail
	StatusSkip
)

func (s Status) String() string {
	switch s {
	case StatusPass:
		return "pass"
	case StatusFail:
		return "fail"
	case StatusSkip:
		return "skip"
	default:
		return "unknown"
	}
}

// diagPair is the (path, code) pair §8.5.2 rule 2 compares diagnostics as
// a set of.
type diagPair struct {
	Path string
	Code string
}

// diagExpect is one entry of an "expect.diagnostics" list in a vector.
type diagExpect struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

// findingExpect is one entry of lint's "expect.findings" list. Unlike
// diagnostics, §8.5.3's operation-driver table documents findings as
// {code, severity, location} -- severity participates in lint's
// comparison (it is not a diagnostic in the §8.5.2 sense, message text
// still never compared per rule 1, hence no "message" field here).
type findingExpect struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Location string `json:"location"`
}

// RunVector dispatches one vector by its operation field (§8.5.3's table)
// and compares the result against its "expect" per §8.5.2's rules. It
// never panics on a vector it cannot execute -- a driver bug produces a
// Result, not a crash, per the issue's "not yet implemented gets an
// honest skip, not a crash or a forced pass" requirement. parse_schema_oml
// and write_schema_oml (the OSD-OML extension) are the first operations
// to actually land in that not-yet-implemented bucket, as cited skips --
// every other operation below has a real driver.
func RunVector(v Vector) Result {
	switch v.Operation {
	case "parse":
		return runParse(v)
	case "parse_schema":
		return runParseSchema(v)
	case "validate":
		return runValidate(v)
	case "materialize":
		return runMaterialize(v)
	case "write":
		return runWrite(v)
	case "compatible_with":
		return runCompatibleWith(v)
	case "equivalent":
		return runEquivalent(v)
	case "normalize":
		return runNormalize(v)
	case "prune":
		return runPrune(v)
	case "is_empty":
		return runIsEmpty(v)
	case "extract":
		return runExtract(v)
	case "infer":
		return runInfer(v)
	case "infer_with_report":
		return runInferWithReport(v)
	case "lint":
		return runLint(v)
	case "parse_schema_oml", "write_schema_oml":
		// OSD-OML extension (omnist-spec PR #53/#55): a schema expressed
		// as an OML document, read/written via parse_schema_oml/
		// write_schema_oml. Not yet implemented in this port -- an
		// honest, cited skip per conformance-harness.md §9.5 ("skips are
		// permitted and MUST be reported; they are how partial
		// implementations are tracked honestly"), not a driver failure,
		// since the spec itself already tracks this as unimplemented
		// across every port (divergence ledger §9.6). See omnist-go
		// issue #111 for this port's own implementation.
		return Result{Vector: v, Status: StatusSkip, Reason: "OSD-OML extension not yet implemented in this port, see omnist-go issue #111"}
	default:
		return Result{Vector: v, Status: StatusFail, Reason: fmt.Sprintf("unknown operation %q", v.Operation)}
	}
}

// --- helpers shared by drivers ---

func fail(v Vector, format string, args ...any) Result {
	return Result{Vector: v, Status: StatusFail, Reason: fmt.Sprintf(format, args...)}
}

func pass(v Vector) Result { return Result{Vector: v, Status: StatusPass} }

// expectOK reports whether expect.ok is true (defaulting to true when the
// field is absent, since not every success shape in §8.5.3's table
// actually carries an "ok" key -- e.g. normalize/prune/is_empty/
// compatible_with/equivalent never do).
func expectOK(expect map[string]json.RawMessage) bool {
	raw, present := expect["ok"]
	if !present {
		return true
	}
	var ok bool
	_ = json.Unmarshal(raw, &ok)
	return ok
}

func decodeExpect(v Vector) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(v.Expect, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func decodeExpectDiagnostics(expect map[string]json.RawMessage) ([]diagExpect, error) {
	raw, ok := expect["diagnostics"]
	if !ok {
		return nil, nil
	}
	var d []diagExpect
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return d, nil
}

// diagnosticSetsEqual implements §8.5.2's matching rules: compared as a
// set of (path, code), never severity, message text never compared
// (rule 1), no partial matching (rule 3) -- every expected diagnostic
// present, no unexpected one.
func diagnosticSetsEqual(actual, expected []diagPair) bool {
	if len(actual) != len(expected) {
		return false
	}
	toKey := func(p diagPair) string { return p.Path + "\x00" + p.Code }
	got := map[string]int{}
	for _, p := range actual {
		got[toKey(p)]++
	}
	for _, p := range expected {
		k := toKey(p)
		if got[k] == 0 {
			return false
		}
		got[k]--
	}
	for _, n := range got {
		if n != 0 {
			return false
		}
	}
	return true
}

// pathPlaceholder is E-32's literal expected path: "compare the code, and that
// the path is a well-formed text position inside the input, nothing closer".
const pathPlaceholder = "line:col"

// wellFormedTextPosition is E-32b's pattern: two decimal numbers of at least 1,
// no sign, no leading zero, no whitespace, joined by a colon.
var wellFormedTextPosition = regexp.MustCompile(`^[1-9][0-9]*:[1-9][0-9]*$`)

// placeholderApplies reports whether expected is exactly the one entry E-32a
// allows the placeholder on: a lone parse.codec-syntax diagnostic of a JSON,
// YAML, TOML or XML parse vector. Anywhere else the string "line:col" is an
// ordinary path, compared byte for byte, and so can never match.
func placeholderApplies(expected []diagPair, format string) bool {
	if len(expected) != 1 || expected[0].Path != pathPlaceholder || expected[0].Code != "parse.codec-syntax" {
		return false
	}
	switch format {
	case "json", "yaml", "toml", "xml":
		return true
	}
	return false
}

// positionInsideInput reports whether path is a well-formed text position that
// lies inside input per E-31/E-32b: line at most one more than the number of
// LF characters (E-29), col at most one more than the code points on that line
// (E-28).
func positionInsideInput(input, path string) bool {
	if !wellFormedTextPosition.MatchString(path) {
		return false
	}
	lineStr, colStr, _ := strings.Cut(path, ":")
	line, errL := strconv.Atoi(lineStr)
	col, errC := strconv.Atoi(colStr)
	lines := strings.Split(input, "\n")
	if errL != nil || errC != nil || line > len(lines) {
		return false
	}
	return col <= utf8.RuneCountInString(lines[line-1])+1
}

// parseDiagnosticsEqual compares a parse vector's reported diagnostics with its
// expected ones: diagnosticSetsEqual, except that E-32's placeholder entry is
// satisfied by one reported diagnostic with the same code and a well-formed
// position inside input. Every other path stays byte for byte.
func parseDiagnosticsEqual(actual, expected []diagPair, format, input string) bool {
	if !placeholderApplies(expected, format) {
		return diagnosticSetsEqual(actual, expected)
	}
	return len(actual) == 1 && actual[0].Code == expected[0].Code && positionInsideInput(input, actual[0].Path)
}

func diagStrings(pairs []diagPair) []string {
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = fmt.Sprintf("%s:%s", p.Path, p.Code)
	}
	sort.Strings(out)
	return out
}

// singleErrDiag converts one error returned by a reader/parser/operation
// (either *omnist.ParseError, a bare omnist.Diagnostic used as an error
// per schemaError's convention, or some other error) into a diagPair. It
// never returns a zero-value silently -- an error whose shape it does not
// recognize still yields a diagPair (empty code), which will correctly
// fail a diagnosticSetsEqual comparison rather than being swallowed.
func singleErrDiag(err error) diagPair {
	switch e := err.(type) {
	case *omnist.ParseError:
		return diagPair{Path: e.Path, Code: string(e.Code)}
	case omnist.ParseError:
		return diagPair{Path: e.Path, Code: string(e.Code)}
	case omnist.Diagnostic:
		return diagPair{Path: e.Path, Code: string(e.Code)}
	default:
		return diagPair{Path: "", Code: fmt.Sprintf("<unrecognized error type: %T: %v>", err, err)}
	}
}

func diagsToPairs(diags []omnist.Diagnostic) []diagPair {
	out := make([]diagPair, len(diags))
	for i, d := range diags {
		out[i] = diagPair{Path: d.Path, Code: string(d.Code)}
	}
	return out
}
