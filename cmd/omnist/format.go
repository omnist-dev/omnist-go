package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/formats/json"
	"github.com/omnist-dev/omnist-go/formats/toml"
	"github.com/omnist-dev/omnist-go/formats/xml"
	"github.com/omnist-dev/omnist-go/formats/yaml"
	"github.com/omnist-dev/omnist-go/oml"
)

// formatReaderFunc and formatWriterFunc give every stage-1 reader/writer
// pair in the library a single shared shape, so the --from/--to dispatch
// table below can be built once and shared by every subcommand that
// needs format dispatch (parse, validate, materialize, infer), instead
// of six separate copies of the same format-name switch statement.
//
// formatReaderFunc's diagnostics return mirrors formatWriterFunc's below:
// a read can succeed (err == nil) while still reporting a non-fatal
// adjustment via the returned []omnist.Diagnostic. Today only xml.Read
// ever populates it (an attribute or namespace prefix dropped, D-3, spec
// §8.3.8) -- json/yaml/toml/oml have nothing to report on read, so
// they're wrapped below to return nil diagnostics, the same way oml.Write
// is wrapped in formatWriters for the identical reason.
type formatReaderFunc func(text string, limits omnist.Limits) (omnist.Document, []omnist.Diagnostic, error)

// formatWriterFunc's diagnostics return is the ok:true+diagnostics
// channel added in issue #49: a writer can succeed (err == nil) while
// still reporting a non-fatal adjustment (a dropped null, a stringified
// temporal, a substituted NaN/Infinity) via the returned
// []omnist.Diagnostic, per spec §8.5.3.
type formatWriterFunc func(d omnist.Document) (string, []omnist.Diagnostic, error)

// noReadDiagnostics adapts a reader with no diagnostics channel of its
// own (json.Read, yaml.Read, toml.Read, oml.Read) to formatReaderFunc,
// mirroring formatWriters' oml.Write wrapper below.
func noReadDiagnostics(read func(string, omnist.Limits) (omnist.Document, error)) formatReaderFunc {
	return func(text string, limits omnist.Limits) (omnist.Document, []omnist.Diagnostic, error) {
		doc, err := read(text, limits)
		return doc, nil, documentPathError(err)
	}
}

var formatReaders = map[string]formatReaderFunc{
	"json": noReadDiagnostics(json.Read),
	"yaml": noReadDiagnostics(yaml.Read),
	"toml": noReadDiagnostics(toml.Read),
	"xml":  xml.Read,
	"oml":  noReadDiagnostics(oml.Read),
}

var formatWriters = map[string]formatWriterFunc{
	"json": json.Write,
	"yaml": yaml.Write,
	"toml": toml.Write,
	"xml":  xml.Write,
	// oml.Write takes a compact flag; the CLI always writes the pretty form.
	"oml": func(d omnist.Document) (string, []omnist.Diagnostic, error) {
		return oml.Write(d, false)
	},
}

// xmlReaderWithSchema returns the XML reader pretyped with schema, so the
// CLI applies integer pretyping and the D-9 digit limit exactly as the
// library's xml.ReadWithSchema does.
func xmlReaderWithSchema(schema *omnist.Schema) formatReaderFunc {
	return func(text string, limits omnist.Limits) (omnist.Document, []omnist.Diagnostic, error) {
		doc, diags, err := xml.ReadWithSchema(text, schema, limits)
		return doc, diags, documentPathError(err)
	}
}

// documentPathError reports a read refusal that carries a Document path as the
// path-first diagnostic it is. A document.* code (document.limit.int-digits)
// is positioned by a Document path (spec §8.4, E-4a), never a line:col, so
// ParseError.Error() would print the wrong location; a parse.* error keeps its
// text position.
func documentPathError(err error) error {
	var pe *omnist.ParseError
	if errors.As(err, &pe) && pe.Path != "" && (pe.Line == 0 || strings.HasPrefix(string(pe.Code), "document.")) {
		return omnist.Diagnostic{Path: pe.Path, Code: pe.Code, Message: pe.Message}
	}
	return err
}

// knownFormatNames returns the five supported format names, sorted, for
// use in usage/error text.
func knownFormatNames() []string {
	names := make([]string, 0, len(formatReaders))
	for name := range formatReaders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func lookupReader(name string) (formatReaderFunc, error) {
	r, ok := formatReaders[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("unknown format %q (supported: %s)", name, strings.Join(knownFormatNames(), ", "))
	}
	return r, nil
}

func lookupWriter(name string) (formatWriterFunc, error) {
	w, ok := formatWriters[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("unknown format %q (supported: %s)", name, strings.Join(knownFormatNames(), ", "))
	}
	return w, nil
}
