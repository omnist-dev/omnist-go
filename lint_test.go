package omnist

import "testing"

// --- reachablePlain directly (for 100% per-function coverage of the
// "not in env" skip branch, which a well-formed schema never triggers
// through Lint alone) ---
//
// This is the one lint_test.go case needing unexported access
// (reachablePlain itself); it doesn't even need mustParseOSD, so it's the
// only case that stayed in the internal "omnist" package. Every other
// lint.go test moved to lint_public_test.go (external "omnist_test"
// package) since it only needed mustParseOSD + exported API -- see
// referee_test.go's comment for why that split exists.

func TestReachablePlainSkipsUnresolvedRef(t *testing.T) {
	s := Schema{
		Root: "Root",
		Env: map[string]*Record{
			"Root": {Name: "Root", Fields: []Field{
				{Label: "dangling", Type: RefType("Ghost"), Cardinality: DefaultCardinality()},
			}},
		},
		EnvOrder: []string{"Root"},
	}
	reach := reachablePlain(s)
	if !reach["Root"] {
		t.Fatalf("Root should be reached")
	}
	if reach["Ghost"] {
		t.Fatalf("Ghost does not resolve in env and must not appear in reach")
	}
}
