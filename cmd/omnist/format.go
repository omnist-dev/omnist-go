package main

import (
	"fmt"
	"sort"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// formatReaderFunc and formatWriterFunc give every stage-1 reader/writer
// pair in the library a single shared shape, so the --from/--to dispatch
// table below can be built once and shared by every subcommand that
// needs format dispatch (parse, validate, materialize, infer), instead
// of six separate copies of the same format-name switch statement.
type formatReaderFunc func(text string, limits omnist.Limits) (omnist.Document, error)
type formatWriterFunc func(d omnist.Document) (string, error)

var formatReaders = map[string]formatReaderFunc{
	"json": omnist.ReadJSON,
	"yaml": omnist.ReadYAML,
	"toml": omnist.ReadTOML,
	"xml":  omnist.ReadXML,
	"oml":  omnist.ReadOML,
}

var formatWriters = map[string]formatWriterFunc{
	"json": omnist.WriteJSON,
	"yaml": omnist.WriteYAML,
	"toml": omnist.WriteTOML,
	"xml":  omnist.WriteXML,
	// WriteOML never returns an error (compact-vs-pretty is the only
	// knob, and both always succeed), but it's wrapped here so every
	// entry in this table shares one signature.
	"oml": func(d omnist.Document) (string, error) { return omnist.WriteOML(d, false), nil },
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
