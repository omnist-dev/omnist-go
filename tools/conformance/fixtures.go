// This file is Track 1 (fixture-based) of the conformance harness, per
// issue #55 and vendor/omnist-spec/docs/conformance-harness.md. It walks
// vendor/omnist-spec/conformance/fixtures/<operation>/<name>/, one
// directory per fixture, dispatches each fixture's operation to the same
// real omnist-go functions Track 2's drivers.go already calls (reusing
// that dispatch rather than duplicating it), and compares the result
// using referee.go's DocumentsEqual/SchemasEqual -- the same referee
// Track 2 uses, per §4 ("the same idea, this track's file-per-fixture
// shape instead of one JSON object").
//
// Track 1 differs from Track 2 only in fixture *format* (files on disk,
// OML/OSD text, not one JSON vector object with a canonical-encoding
// Document), never in what each operation does -- so this file's job is
// reading each fixture's files into the same Go values drivers.go's
// Track 2 functions already operate on (omnist.Document, omnist.Schema,
// bool, []string), then calling the identical algebra/omnist functions.
package conformance

import (
	encjson "encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
	"github.com/omnist-dev/omnist-go/osd"
)

// FixtureCase is one Track 1 fixture: one directory under
// conformance/fixtures/<operation>/<name>/.
type FixtureCase struct {
	Operation string
	Name      string
	Dir       string
	Purpose   string // purpose.txt's first line (happy-path/edge-case/error-case/determinism-regression)
}

// singleInputOps and twoInputOps are §3's two fixture shapes. Used only to
// sanity-check WalkFixtures isn't silently picking up an operation this
// track doesn't know how to read yet.
var fixtureOps = map[string]bool{
	"normalize": true, "prune": true, "write": true, "is_empty": true, "lint": true,
	"validate": true, "materialize": true, "compatible_with": true, "equivalent": true, "extract": true,
	"infer": true,
}

// WalkFixtures lists every fixture directory under root, skipping
// _referee-self-test (already hand-ported as referee_test.go -- see §6 and
// the issue) and any non-directory entry (e.g. .gitkeep placeholders left
// in otherwise-empty operation directories).
func WalkFixtures(root string) ([]FixtureCase, error) {
	opEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading fixtures root %s: %w", root, err)
	}
	var cases []FixtureCase
	for _, opEntry := range opEntries {
		if !opEntry.IsDir() {
			continue
		}
		op := opEntry.Name()
		if op == "_referee-self-test" {
			continue
		}
		opDir := filepath.Join(root, op)
		nameEntries, err := os.ReadDir(opDir)
		if err != nil {
			return nil, fmt.Errorf("reading operation dir %s: %w", opDir, err)
		}
		for _, nameEntry := range nameEntries {
			if !nameEntry.IsDir() {
				continue // e.g. .gitkeep
			}
			dir := filepath.Join(opDir, nameEntry.Name())
			purpose, _ := readFileTrim(filepath.Join(dir, "purpose.txt"))
			firstLine := purpose
			if idx := strings.IndexByte(purpose, '\n'); idx >= 0 {
				firstLine = purpose[:idx]
			}
			cases = append(cases, FixtureCase{
				Operation: op,
				Name:      nameEntry.Name(),
				Dir:       dir,
				Purpose:   firstLine,
			})
		}
	}
	sort.Slice(cases, func(i, j int) bool {
		if cases[i].Operation != cases[j].Operation {
			return cases[i].Operation < cases[j].Operation
		}
		return cases[i].Name < cases[j].Name
	})
	return cases, nil
}

// RunFixture dispatches one fixture by its operation and reports
// pass/fail/skip, mirroring RunVector's contract: never panics on a
// fixture it cannot execute, an unrecognized operation is a Result, not a
// crash.
func RunFixture(fc FixtureCase) Result {
	if !fixtureOps[fc.Operation] {
		return fixSkip(fc, fmt.Sprintf("no Track 1 driver for operation %q", fc.Operation))
	}
	switch fc.Operation {
	case "normalize":
		return runFixtureNormalize(fc)
	case "prune":
		return runFixturePrune(fc)
	case "write":
		return runFixtureWrite(fc)
	case "is_empty":
		return runFixtureIsEmpty(fc)
	case "lint":
		return runFixtureLint(fc)
	case "validate":
		return runFixtureValidate(fc)
	case "materialize":
		return runFixtureMaterialize(fc)
	case "compatible_with":
		return runFixtureCompatibleWith(fc)
	case "equivalent":
		return runFixtureEquivalent(fc)
	case "extract":
		return runFixtureExtract(fc)
	case "infer":
		return runFixtureInfer(fc)
	default:
		return fixSkip(fc, fmt.Sprintf("no Track 1 driver for operation %q", fc.Operation))
	}
}

// --- shared fixture I/O helpers ---

func readFileTrim(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}

func mustReadSchema(fc FixtureCase, filename string) (omnist.Schema, error) {
	text, err := readFileTrim(filepath.Join(fc.Dir, filename))
	if err != nil {
		return omnist.Schema{}, fmt.Errorf("reading %s: %w", filename, err)
	}
	s, err := osd.Read(text)
	if err != nil {
		return omnist.Schema{}, fmt.Errorf("parsing %s: %w", filename, err)
	}
	return s, nil
}

func mustReadDoc(fc FixtureCase, filename string) (omnist.Document, error) {
	text, err := readFileTrim(filepath.Join(fc.Dir, filename))
	if err != nil {
		return omnist.Document{}, fmt.Errorf("reading %s: %w", filename, err)
	}
	d, err := oml.Read(text, omnist.DefaultLimits())
	if err != nil {
		return omnist.Document{}, fmt.Errorf("parsing %s: %w", filename, err)
	}
	return d, nil
}

func readBool(fc FixtureCase, relPath string) (bool, error) {
	text, err := readFileTrim(filepath.Join(fc.Dir, relPath))
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", relPath, err)
	}
	b, err := strconv.ParseBool(strings.TrimSpace(text))
	if err != nil {
		return false, fmt.Errorf("parsing %s as bool: %w", relPath, err)
	}
	return b, nil
}

func fixFail(fc FixtureCase, format string, args ...any) Result {
	return Result{
		Vector: Vector{Name: fc.Operation + "/" + fc.Name, Operation: fc.Operation, Purpose: fc.Purpose},
		Status: StatusFail,
		Reason: fmt.Sprintf(format, args...),
	}
}

func fixPass(fc FixtureCase) Result {
	return Result{
		Vector: Vector{Name: fc.Operation + "/" + fc.Name, Operation: fc.Operation, Purpose: fc.Purpose},
		Status: StatusPass,
	}
}

func fixSkip(fc FixtureCase, reason string) Result {
	return Result{
		Vector: Vector{Name: fc.Operation + "/" + fc.Name, Operation: fc.Operation, Purpose: fc.Purpose},
		Status: StatusSkip,
		Reason: reason,
	}
}

// fixtureDiagnostics reads expected/diagnostics.json, present only when
// expected/ok.txt is "false" (§3). Returns nil if the file does not
// exist -- some error-case fixtures in this repository's checkout assert
// only the ok:false boolean, with no diagnostics.json to further pin
// paths/codes; that is a valid (if less strict) fixture, not a harness bug.
func fixtureDiagnostics(fc FixtureCase, relDir string) ([]diagPair, bool, error) {
	path := filepath.Join(fc.Dir, relDir, "diagnostics.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var raw []diagExpect
	if err := encjson.Unmarshal(b, &raw); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", path, err)
	}
	return expectDiagPairs(raw), true, nil
}

// --- single-input operations: normalize, prune ---

func runFixtureNormalize(fc FixtureCase) Result {
	s, err := mustReadSchema(fc, "input.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	want, err := mustReadSchema(fc, "expected.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	got := algebra.Normalize(s)
	if !omnist.SchemasEqual(got, want, omnist.ModeExact) {
		return fixFail(fc, "schema mismatch (exact mode): got %q want %q", osd.Write(got, false), osd.Write(want, false))
	}
	return fixPass(fc)
}

func runFixturePrune(fc FixtureCase) Result {
	s, err := mustReadSchema(fc, "input.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	want, err := mustReadSchema(fc, "expected.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	got := algebra.Prune(s)
	if !omnist.SchemasEqual(got, want, omnist.ModeExact) {
		return fixFail(fc, "schema mismatch (exact mode): got %q want %q", osd.Write(got, false), osd.Write(want, false))
	}
	return fixPass(fc)
}

// --- write ---

// writeDoc mirrors runWrite's format switch (drivers.go) for a single,
// non-strict write -- Track 1's write fixtures have no "strict" input
// field (§3's single-input write shape is just input/expected text), so
// this always calls the non-strict writer.
func writeDoc(format string, d omnist.Document) (string, []omnist.Diagnostic, error) {
	switch format {
	case "oml":
		text, diags := oml.Write(d, false)
		return text, diags, nil
	case "json":
		return json.Write(d)
	case "yaml":
		return yaml.Write(d)
	case "toml":
		return toml.Write(d)
	case "xml":
		return xml.Write(d)
	default:
		return "", nil, fmt.Errorf("unrecognized format %q", format)
	}
}

// writeFixtureFormat finds this write fixture's target format from its
// "input.<ext>" file, per §3's note that write's extensions match the
// target format, not always .osd.
func writeFixtureFormat(fc FixtureCase) (string, error) {
	entries, err := os.ReadDir(fc.Dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "input.") {
			return strings.TrimPrefix(filepath.Ext(e.Name()), "."), nil
		}
	}
	return "", fmt.Errorf("no input.<format> file found")
}

func runFixtureWrite(fc FixtureCase) Result {
	format, err := writeFixtureFormat(fc)
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	inputText, err := readFileTrim(filepath.Join(fc.Dir, "input."+format))
	if err != nil {
		return fixFail(fc, "reading input.%s: %v", format, err)
	}
	doc, rerr := readByFormat(format, inputText, omnist.DefaultLimits())
	if rerr != nil {
		return fixFail(fc, "parsing input.%s: %v", format, rerr)
	}
	expectedText, err := readFileTrim(filepath.Join(fc.Dir, "expected."+format))
	if err != nil {
		return fixFail(fc, "reading expected.%s: %v", format, err)
	}
	wantDoc, werr := readByFormat(format, expectedText, omnist.DefaultLimits())
	if werr != nil {
		return fixFail(fc, "parsing expected.%s: %v", format, werr)
	}
	gotText, _, writeErr := writeDoc(format, doc)
	if writeErr != nil {
		return fixFail(fc, "write: %v", writeErr)
	}
	// §2's "--compact vs. pretty-printed output must compare equal" note
	// applies here too: the referee re-parses gotText rather than
	// byte-comparing it against expectedText.
	gotDoc, gerr := readByFormat(format, gotText, omnist.DefaultLimits())
	if gerr != nil {
		return fixFail(fc, "re-parsing written output: %v (output was %q)", gerr, gotText)
	}
	if !omnist.DocumentsEqual(gotDoc, wantDoc) {
		return fixFail(fc, "document mismatch: got %q want %q", gotText, expectedText)
	}
	return fixPass(fc)
}

// --- is_empty ---

func runFixtureIsEmpty(fc FixtureCase) Result {
	s, err := mustReadSchema(fc, "input.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	want, err := readBool(fc, "expected.txt")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	got := algebra.IsEmpty(s)
	if got != want {
		return fixFail(fc, "empty mismatch: got %v want %v", got, want)
	}
	return fixPass(fc)
}

// --- lint ---

type lintExpectedFile struct {
	OK       bool            `json:"ok"`
	Findings []findingExpect `json:"findings"`
}

func runFixtureLint(fc FixtureCase) Result {
	s, err := mustReadSchema(fc, "input.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	b, err := os.ReadFile(filepath.Join(fc.Dir, "expected.json"))
	if err != nil {
		return fixFail(fc, "reading expected.json: %v", err)
	}
	var want lintExpectedFile
	if err := encjson.Unmarshal(b, &want); err != nil {
		return fixFail(fc, "decode expected.json: %v", err)
	}
	findings := algebra.Lint(s)
	gotOK := true
	for _, f := range findings {
		if f.Severity != omnist.SeverityInfo {
			gotOK = false
			break
		}
	}
	if gotOK != want.OK {
		return fixFail(fc, "ok mismatch: got %v want %v (findings: %v)", gotOK, want.OK, findings)
	}
	if len(findings) != len(want.Findings) {
		return fixFail(fc, "findings count mismatch: got %d want %d", len(findings), len(want.Findings))
	}
	gotSet := map[string]int{}
	for _, f := range findings {
		gotSet[string(f.Code)+"\x00"+f.Severity.String()+"\x00"+f.Location]++
	}
	for _, f := range want.Findings {
		k := f.Code + "\x00" + f.Severity + "\x00" + f.Location
		if gotSet[k] == 0 {
			return fixFail(fc, "findings mismatch: missing {code:%q severity:%q location:%q}", f.Code, f.Severity, f.Location)
		}
		gotSet[k]--
	}
	return fixPass(fc)
}

// --- validate ---

func runFixtureValidate(fc FixtureCase) Result {
	s, err := mustReadSchema(fc, "schema.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	doc, err := mustReadDoc(fc, "input.oml")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	wantOK, err := readBool(fc, "expected/ok.txt")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	diags := omnist.Validate(doc, s)
	got := diagsToPairs(diags)
	if wantOK {
		if len(diags) != 0 {
			return fixFail(fc, "expected ok, got diagnostics: %v", diagStrings(got))
		}
		return fixPass(fc)
	}
	if len(diags) == 0 {
		return fixFail(fc, "expected diagnostics (ok:false), got none")
	}
	want, hasFile, err := fixtureDiagnostics(fc, "expected")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	if hasFile && !diagnosticSetsEqual(got, want) {
		return fixFail(fc, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
	}
	return fixPass(fc)
}

// --- materialize ---

func runFixtureMaterialize(fc FixtureCase) Result {
	s, err := mustReadSchema(fc, "schema.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	doc, err := mustReadDoc(fc, "input.oml")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	wantOK, err := readBool(fc, "expected/ok.txt")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	_, diags, merr := omnist.Materialize(doc, s)
	if merr != nil {
		return fixFail(fc, "materialize: %v", merr)
	}
	got := diagsToPairs(diags)
	if wantOK {
		if len(diags) != 0 {
			return fixFail(fc, "expected ok, got diagnostics: %v", diagStrings(got))
		}
		return fixPass(fc)
	}
	if len(diags) == 0 {
		return fixFail(fc, "expected diagnostics (ok:false), got none")
	}
	want, hasFile, err := fixtureDiagnostics(fc, "expected")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	if hasFile && !diagnosticSetsEqual(got, want) {
		return fixFail(fc, "diagnostics mismatch: got %v want %v", diagStrings(got), diagStrings(want))
	}
	return fixPass(fc)
}

// --- compatible_with / equivalent ---

func runFixtureCompatibleWith(fc FixtureCase) Result {
	a, err := mustReadSchema(fc, "a.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	b, err := mustReadSchema(fc, "b.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	want, err := readBool(fc, "expected.txt")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	got := algebra.CompatibleWith(a, b)
	if got != want {
		return fixFail(fc, "result mismatch: got %v want %v", got, want)
	}
	return fixPass(fc)
}

func runFixtureEquivalent(fc FixtureCase) Result {
	a, err := mustReadSchema(fc, "a.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	b, err := mustReadSchema(fc, "b.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	want, err := readBool(fc, "expected.txt")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	got := algebra.Equivalent(a, b)
	if got != want {
		return fixFail(fc, "result mismatch: got %v want %v", got, want)
	}
	return fixPass(fc)
}

// --- extract ---

func runFixtureExtract(fc FixtureCase) Result {
	s, err := mustReadSchema(fc, "schema.osd")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	keepText, err := readFileTrim(filepath.Join(fc.Dir, "keep.txt"))
	if err != nil {
		return fixFail(fc, "reading keep.txt: %v", err)
	}
	keep := map[string]bool{}
	for _, label := range strings.Split(keepText, ",") {
		label = strings.TrimSpace(label)
		if label != "" {
			keep[label] = true
		}
	}
	wantOK, err := readBool(fc, "expected/ok.txt")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	result, eerr := algebra.Extract(s, keep)
	if eerr != nil {
		if wantOK {
			return fixFail(fc, "expected ok, got error: %v", eerr)
		}
		return fixPass(fc)
	}
	if !wantOK {
		return fixFail(fc, "expected error, got ok extract")
	}
	wantText, err := readFileTrim(filepath.Join(fc.Dir, "expected", "output.osd"))
	if err != nil {
		return fixFail(fc, "reading expected/output.osd: %v", err)
	}
	wantSchema, werr := osd.Read(wantText)
	if werr != nil {
		return fixFail(fc, "parsing expected/output.osd: %v", werr)
	}
	if !omnist.SchemasEqual(result, wantSchema, omnist.ModeExact) {
		return fixFail(fc, "schema mismatch (exact mode): got %q want %q", osd.Write(result, false), wantText)
	}
	return fixPass(fc)
}

// --- infer ---

func runFixtureInfer(fc FixtureCase) Result {
	sampleEntries, err := os.ReadDir(filepath.Join(fc.Dir, "samples"))
	if err != nil {
		return fixFail(fc, "reading samples dir: %v", err)
	}
	var names []string
	for _, e := range sampleEntries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // 1.oml, 2.oml, ... -- lexical == numeric for this fixture set's sample counts
	var docs []omnist.Document
	for _, n := range names {
		text, err := readFileTrim(filepath.Join(fc.Dir, "samples", n))
		if err != nil {
			return fixFail(fc, "reading samples/%s: %v", n, err)
		}
		d, derr := oml.Read(text, omnist.DefaultLimits())
		if derr != nil {
			return fixFail(fc, "parsing samples/%s: %v", n, derr)
		}
		docs = append(docs, d)
	}
	allowAny := false
	if _, err := os.Stat(filepath.Join(fc.Dir, "allow_any.txt")); err == nil {
		allowAny, err = readBool(fc, "allow_any.txt")
		if err != nil {
			return fixFail(fc, "%v", err)
		}
	}
	wantOK, err := readBool(fc, "expected/ok.txt")
	if err != nil {
		return fixFail(fc, "%v", err)
	}
	result, ierr := algebra.Infer(docs, "Root", allowAny)
	if ierr != nil {
		if wantOK {
			return fixFail(fc, "expected ok, got error: %v", ierr)
		}
		return fixPass(fc)
	}
	if !wantOK {
		return fixFail(fc, "expected error, got ok infer")
	}
	wantText, err := readFileTrim(filepath.Join(fc.Dir, "expected", "output.osd"))
	if err != nil {
		return fixFail(fc, "reading expected/output.osd: %v", err)
	}
	wantSchema, werr := osd.Read(wantText)
	if werr != nil {
		return fixFail(fc, "parsing expected/output.osd: %v", werr)
	}
	// infer never normalizes its output (§6.10), so ModeIsomorphic -- same
	// reasoning as Track 2's runInfer.
	if !omnist.SchemasEqual(result, wantSchema, omnist.ModeIsomorphic) {
		return fixFail(fc, "schema mismatch (isomorphic mode): got %q want %q", osd.Write(result, false), wantText)
	}
	return fixPass(fc)
}
