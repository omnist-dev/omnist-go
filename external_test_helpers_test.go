package omnist_test

// This file holds the OSD- and OML-parsing test helpers shared by this
// package's black-box (external, "omnist_test") test files. It exists
// because of a genuine Go layering constraint: package osd and package oml
// both import package omnist for Schema/Document/etc., so an *internal*
// omnist test file (package omnist) cannot import osd or oml without
// creating an import cycle in the test build ("import cycle not allowed in
// test"). Every test function that only needs OSD- or OML-text fixtures
// plus omnist's exported API (not any unexported production symbol)
// therefore lives in this external "omnist_test" package instead, where
// importing osd/oml is unproblematic.
//
// Test functions that also need unexported access (e.g. scalarSub,
// computeLocalSignature, le, reachablePlain, offsetToLineCol) stay in the
// internal "omnist" package's own _test.go files and build their
// Schema/Record fixtures directly as struct literals rather than through
// OSD/OML text, since they cannot import osd or oml either.
//
// Originally named osd_external_test_helpers_test.go (issue #41, OSD-only);
// renamed here (issue #43) once mustOML started needing to import oml too,
// rather than adding a third, differently-named file for the same pattern.

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/oml"
	"github.com/omnist-dev/omnist-go/osd"
)

func mustParseOSD(t *testing.T, src string) omnist.Schema {
	t.Helper()
	s, err := osd.Read(src)
	if err != nil {
		t.Fatalf("osd.Read(%q) unexpected error: %v", src, err)
	}
	return s
}

func mustOSD(t *testing.T, text string) omnist.Schema {
	t.Helper()
	s, err := osd.Read(text)
	if err != nil {
		t.Fatalf("osd.Read failed: %v", err)
	}
	return s
}

func mustOML(t *testing.T, text string) omnist.Document {
	t.Helper()
	d, err := oml.Read(text, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("oml.Read failed: %v", err)
	}
	return d
}
