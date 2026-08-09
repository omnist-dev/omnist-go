package algebra_test

// This file holds the OSD-parsing test helper shared by this package's
// black-box (external, "algebra_test") test files. Unlike the root
// package's own external_test_helpers_test.go, there is no import-cycle
// concern here to force this split -- package osd does not import package
// algebra (algebra imports omnist, not the other way around), so an
// internal "algebra" test file could import osd without creating a cycle.
// The split is kept anyway, matching every prior stage's established
// pattern: cases needing unexported access (le, scalarSub,
// computeLocalSignature/computeRefineKey/appendEscapedLabel/appendUint,
// reachablePlain) stay in the internal "algebra" package's own _test.go
// files; everything that only needs mustParseOSD plus exported API lives
// here in the external "algebra_test" package instead.
//
// Moved from the root package's external_test_helpers_test.go (issue #47)
// along with the algebra_public_test.go / compatible_with_public_test.go /
// normalize_public_test.go / lint_public_test.go / extract_test.go files
// that use it.

import (
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
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
