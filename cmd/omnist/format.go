package main

import (
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
type formatReaderFunc func(text string, limits omnist.Limits) (omnist.Document, error)

// formatWriterFunc's diagnostics return is the ok:true+diagnostics
// channel added in issue #49: a writer can succeed (err == nil) while
// still reporting a non-fatal adjustment (a dropped null, a stringified
// temporal, a substituted NaN/Infinity) via the returned
// []omnist.Diagnostic, per spec §8.5.3.
type formatWriterFunc func(d omnist.Document) (string, []omnist.Diagnostic, error)

var formatReaders = map[string]formatReaderFunc{
	"json": json.Read,
	"yaml": yaml.Read,
	"toml": toml.Read,
	"xml":  xml.Read,
	"oml":  oml.Read,
}

var formatWriters = map[string]formatWriterFunc{
	"json": json.Write,
	"yaml": yaml.Write,
	"toml": toml.Write,
	"xml":  xml.Write,
	// oml.Write never returns an error (compact-vs-pretty is the only
	// knob, and both always succeed, and every omnist.Kind has a native
	// OML spelling so there's never an adjustment to report either), but
	// it's wrapped here so every entry in this table shares one
	// signature.
	"oml": func(d omnist.Document) (string, []omnist.Diagnostic, error) {
		text, diags := oml.Write(d, false)
		return text, diags, nil
	},
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
