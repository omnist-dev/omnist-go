package algebra_test

// External "algebra_test" package: extract_test.go only ever needed
// exported API (Extract, Diagnostic, CodeAlgebraExtractInvalidatesRoot,
// TypeRefKind) plus its own local keepSet/hasLabel helpers, so it lives
// here rather than in the internal "algebra" package (see
// algebra_external_test_helpers_test.go's comment for the full
// explanation of why the split is kept).

import (
	"errors"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/algebra"
)

// orderSchemaOSD is the Root/Order/Address/LineItem schema from spec
// §3.1's mental model, used verbatim by both §6.9 worked examples.
const orderSchemaOSD = `
record Address  { "street": string, "city": string }
record LineItem { "sku": string, "qty": integer, "price": number }

record Order {
    "id":           string,
    "status":       string,
    "total":        number,
    "address":      Address,
    "items" [1,]:   LineItem,
    "coupon" [0,1]: string,
}

record Root { "order": Order }
root Root
`

func keepSet(labels ...string) map[string]bool {
	m := make(map[string]bool, len(labels))
	for _, l := range labels {
		m[l] = true
	}
	return m
}

func hasLabel(rec *omnist.Record, label string) bool {
	for _, f := range rec.Fields {
		if f.Label == label {
			return true
		}
	}
	return false
}

// --- §6.9 worked example 1: coupon dropped, everything else survives ---

func TestExtractSuccessDropsOptionalCoupon(t *testing.T) {
	s := mustParseOSD(t, orderSchemaOSD)
	keep := keepSet("order", "id", "status", "total", "address", "street",
		"city", "items", "sku", "qty", "price")

	out, err := algebra.Extract(s, keep)
	if err != nil {
		t.Fatalf("algebra.Extract() error = %v, want success", err)
	}

	order := out.Env["Order"]
	if order == nil {
		t.Fatalf("Order missing from extracted schema: %+v", out)
	}
	if hasLabel(order, "coupon") {
		t.Errorf("coupon should be gone, got fields = %+v", order.Fields)
	}
	for _, label := range []string{"id", "status", "total", "address", "items"} {
		if !hasLabel(order, label) {
			t.Errorf("%s should survive, got fields = %+v", label, order.Fields)
		}
	}
	if out.Env["Address"] == nil || out.Env["LineItem"] == nil || out.Env["Root"] == nil {
		t.Errorf("Address/LineItem/Root should all survive, got env = %+v", out.Env)
	}
}

// --- §6.9 worked example 2: total is the first offender, not address/items ---
//
// keep leaves Address's and LineItem's own fields (street/city, sku/qty/
// price) intact, so step 1 never touches those records at all — only
// Order's total/address/items are dropped, and total is declared first
// among them.

func TestExtractFailureReportsFirstBadInDeclarationOrder(t *testing.T) {
	s := mustParseOSD(t, orderSchemaOSD)
	keep := keepSet("order", "id", "status", "street", "city", "sku", "qty", "price")

	_, err := algebra.Extract(s, keep)
	if err == nil {
		t.Fatalf("algebra.Extract() succeeded, want algebra.extract-invalidates-root error")
	}
	var d omnist.Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("error = %v (%T), want omnist.Diagnostic", err, err)
	}
	if d.Code != omnist.CodeAlgebraExtractInvalidatesRoot {
		t.Errorf("Code = %q, want %q", d.Code, omnist.CodeAlgebraExtractInvalidatesRoot)
	}
	if !strings.Contains(d.Message, `"total"`) {
		t.Errorf("Message = %q, want it to name total (the first offender in declaration order)", d.Message)
	}
	if strings.Contains(d.Message, "address") || strings.Contains(d.Message, "items") {
		t.Errorf("Message = %q, must not name address/items — total is declared first in Order", d.Message)
	}
	if d.Path != "Order" {
		t.Errorf("Path = %q, want %q", d.Path, "Order")
	}
}

// --- §6.9 worked example 2 (cross-record continuation): with keep = {"order"},
// every field of every record is unkept, so first_bad is decided across the
// *whole* env in declaration order, not scoped to Order. Address is declared
// before Order in orderSchemaOSD, so Address is invalidated first and
// first_bad is street/Address — not anything about Order's own field order.

func TestExtractFirstBadCrossRecordDeclarationOrderBeatsOrder(t *testing.T) {
	s := mustParseOSD(t, orderSchemaOSD)
	keep := keepSet("order")

	_, err := algebra.Extract(s, keep)
	if err == nil {
		t.Fatalf("algebra.Extract() succeeded, want algebra.extract-invalidates-root error")
	}
	var d omnist.Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("error = %v (%T), want omnist.Diagnostic", err, err)
	}
	if d.Code != omnist.CodeAlgebraExtractInvalidatesRoot {
		t.Errorf("Code = %q, want %q", d.Code, omnist.CodeAlgebraExtractInvalidatesRoot)
	}
	if !strings.Contains(d.Message, `"street"`) {
		t.Errorf("Message = %q, want it to name street (Address is declared before Order)", d.Message)
	}
	if d.Path != "Address" {
		t.Errorf("Path = %q, want %q (declared before Order in env)", d.Path, "Address")
	}
}

// --- deleting a mandatory field invalidates, never silently relaxes ---

func TestExtractMandatoryDeletionInvalidatesNotRelaxes(t *testing.T) {
	s := mustParseOSD(t, `
		record Leaf { "a": string, "b": string }
		record Root { "leaf" [0,1]: Leaf }
		root Root
	`)
	// Drop "a", a mandatory field of Leaf; Leaf's own field is optional in
	// Root so Root itself should survive, but Leaf must be genuinely gone
	// from the output — never present with "a" downgraded to optional.
	out, err := algebra.Extract(s, keepSet("leaf", "b"))
	if err != nil {
		t.Fatalf("algebra.Extract() error = %v, want success (Root.leaf is optional)", err)
	}
	if _, ok := out.Env["Leaf"]; ok {
		t.Errorf("Leaf should be invalidated and absent, got env = %+v", out.Env)
	}
	if hasLabel(out.Env["Root"], "leaf") {
		t.Errorf("Root.leaf (pointing at invalidated Leaf) should be dropped, got fields = %+v", out.Env["Root"].Fields)
	}
}

// --- propagation across more than one hop ---

func TestExtractPropagatesThroughMultipleHops(t *testing.T) {
	s := mustParseOSD(t, `
		record Leaf { "v": string }
		record Mid  { "leaf": Leaf }
		record Top  { "mid": Mid }
		record Root { "top": Top }
		root Root
	`)
	// Drop "v", invalidating Leaf directly; that must propagate Leaf -> Mid
	// -> Top -> Root, three hops of mandatory-ref propagation.
	_, err := algebra.Extract(s, keepSet("top", "mid", "leaf"))
	if err == nil {
		t.Fatalf("algebra.Extract() succeeded, want root invalidated via 3-hop propagation")
	}
	var d omnist.Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("error = %v (%T), want omnist.Diagnostic", err, err)
	}
	if d.Code != omnist.CodeAlgebraExtractInvalidatesRoot {
		t.Errorf("Code = %q, want %q", d.Code, omnist.CodeAlgebraExtractInvalidatesRoot)
	}
	if !strings.Contains(d.Message, `"v"`) || d.Path != "Leaf" {
		t.Errorf("Message/Path = %q/%q, want the original offender v/Leaf, not a propagated one", d.Message, d.Path)
	}
}

// --- first_bad determinism: declared-first offender wins, not alphabetical/shortest ---

func TestExtractFirstBadIsDeclarationOrderNotAlphabeticalOrShortest(t *testing.T) {
	s := mustParseOSD(t, `
		record R { "zeta": string, "beta": string, "alpha": string }
		record Root { "r": R }
		root Root
	`)
	// All three fields of R are dropped; "zeta" is declared first, so it
	// must be the reported offender even though it's neither alphabetically
	// first ("alpha") nor shortest ("beta"/"alpha" tie length before zeta).
	_, err := algebra.Extract(s, keepSet("r"))
	var d omnist.Diagnostic
	if !errors.As(err, &d) {
		t.Fatalf("error = %v (%T), want omnist.Diagnostic", err, err)
	}
	if !strings.Contains(d.Message, `"zeta"`) {
		t.Errorf("Message = %q, want zeta (declared first in R)", d.Message)
	}
	if d.Path != "R" {
		t.Errorf("Path = %q, want %q", d.Path, "R")
	}
}

// --- step 5: a surviving record's field pointing at an invalidated record is dropped ---

func TestExtractDropsSurvivingFieldPointingAtInvalidatedRecord(t *testing.T) {
	s := mustParseOSD(t, `
		record Doomed  { "must": string }
		record Sibling { "x": string }
		record Root {
			"doomed"  [0,1]: Doomed,
			"sibling":        Sibling,
		}
		root Root
	`)
	// Drop "must" (mandatory in Doomed) -> Doomed invalidated. Root.doomed
	// is optional so filtering alone doesn't invalidate Root, and Root
	// survives (via Root.sibling) but must lose the "doomed" field itself,
	// not keep it dangling at a record no longer in env.
	out, err := algebra.Extract(s, keepSet("doomed", "sibling", "x"))
	if err != nil {
		t.Fatalf("algebra.Extract() error = %v, want success", err)
	}
	if _, ok := out.Env["Doomed"]; ok {
		t.Errorf("Doomed should be invalidated and absent, got env = %+v", out.Env)
	}
	root := out.Env["Root"]
	if root == nil {
		t.Fatalf("Root missing: %+v", out)
	}
	if hasLabel(root, "doomed") {
		t.Errorf("Root.doomed (dangling at invalidated Doomed) should be dropped, got fields = %+v", root.Fields)
	}
	if !hasLabel(root, "sibling") {
		t.Errorf("Root.sibling should survive untouched, got fields = %+v", root.Fields)
	}
}

// --- success path actually runs prune+normalize, not just the raw intermediate schema ---

func TestExtractSuccessRunsPruneAndNormalize(t *testing.T) {
	s := mustParseOSD(t, `
		record Unreachable { "v": string }
		record Root        { "a": string }
		root Root
	`)
	// Unreachable is never referenced from Root; extract's intermediate
	// schema (steps 1-4) would still contain it verbatim since nothing
	// about the keep-set filtering touches it. Only prune removes it.
	out, err := algebra.Extract(s, keepSet("a"))
	if err != nil {
		t.Fatalf("algebra.Extract() error = %v, want success", err)
	}
	if _, ok := out.Env["Unreachable"]; ok {
		t.Errorf("Unreachable should have been pruned, got env = %+v", out.Env)
	}
}

func TestExtractSuccessNormalizesStructurallyIdenticalRecords(t *testing.T) {
	s := mustParseOSD(t, `
		record A { "v": string }
		record B { "v": string, "extra" [0,1]: string }
		record Root { "a": A, "b": B }
		root Root
	`)
	// Dropping "extra" from B (optional, so no invalidation) leaves A and B
	// structurally identical. Only normalize would merge them.
	out, err := algebra.Extract(s, keepSet("a", "b", "v"))
	if err != nil {
		t.Fatalf("algebra.Extract() error = %v, want success", err)
	}
	aRef := out.Env["Root"].Fields
	names := map[string]bool{}
	for _, f := range aRef {
		if f.Type.Kind == omnist.TypeRefKind {
			names[f.Type.RefName] = true
		}
	}
	if len(names) != 1 {
		t.Errorf("expected A and B merged into one record by normalize, Root's ref fields point at %v", names)
	}
}

// --- keep everything: no-op besides the canonical prune+normalize pass ---

func TestExtractKeepEverythingSucceeds(t *testing.T) {
	s := mustParseOSD(t, orderSchemaOSD)
	keep := keepSet("order", "id", "status", "total", "address", "street",
		"city", "items", "sku", "qty", "price", "coupon")
	out, err := algebra.Extract(s, keep)
	if err != nil {
		t.Fatalf("algebra.Extract() error = %v, want success", err)
	}
	if !hasLabel(out.Env["Order"], "coupon") {
		t.Errorf("coupon should survive when kept, got fields = %+v", out.Env["Order"].Fields)
	}
}
