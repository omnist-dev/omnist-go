package omnist

import (
	"math/big"
	"testing"
)

// personSchema builds:
//
//	record Person {
//	    "name":               string,
//	    "age"    [0,1]:       integer,
//	    "email"  [0,]:        string,
//	    "tag"    [2,3]:       string,
//	    "coupon" [0,1]:       string?,
//	    "friend" [0,1]:       Person,
//	    "data":               any,
//	}
//	root Person
func personSchema() Schema {
	person := &Record{
		Name: "Person",
		Fields: []Field{
			{Label: "name", Type: ScalarType(KindString, false), Cardinality: DefaultCardinality()},
			{Label: "age", Type: ScalarType(KindInteger, false), Cardinality: Cardinality{Min: 0, Max: 1}},
			{Label: "email", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 0, Unbounded: true}},
			{Label: "tag", Type: ScalarType(KindString, false), Cardinality: Cardinality{Min: 2, Max: 3}},
			{Label: "coupon", Type: ScalarType(KindString, true), Cardinality: Cardinality{Min: 0, Max: 1}},
			{Label: "friend", Type: RefType("Person"), Cardinality: Cardinality{Min: 0, Max: 1}},
			{Label: "data", Type: AnyType(), Cardinality: DefaultCardinality()},
		},
	}
	return Schema{Root: "Person", Env: map[string]*Record{"Person": person}}
}

// validPerson builds a Node satisfying personSchema with the minimum
// required edges: name, tag x2, data.
func validPerson() *Node {
	n := NewNode()
	n.AddValue("name", ScalarValue(NewStringScalar("Ann")))
	n.AddValue("tag", ScalarValue(NewStringScalar("a")))
	n.AddValue("tag", ScalarValue(NewStringScalar("b")))
	n.AddValue("data", ScalarValue(NewStringScalar("anything")))
	return n
}

func hasCode(diags []Diagnostic, code Code) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func codesOf(diags []Diagnostic) []Code {
	out := make([]Code, len(diags))
	for i, d := range diags {
		out[i] = d.Code
	}
	return out
}

func TestValidateValidDocument(t *testing.T) {
	s := personSchema()
	doc := NodeDocument(validPerson())
	got := Validate(doc, s)
	if len(got) != 0 {
		t.Fatalf("Validate() = %+v, want empty (valid)", got)
	}
}

func TestValidateEmptyResultIsNonNil(t *testing.T) {
	// §3.6: "an empty list means valid" — confirm the zero-diagnostics
	// result is an actual empty slice, not a nil that happens to have len 0
	// (both satisfy "empty", but the constructor should be deliberate).
	got := Validate(NodeDocument(validPerson()), personSchema())
	if got == nil {
		t.Fatal("Validate() returned nil, want non-nil empty slice")
	}
}

func TestValidateNodeWhereScalarExpected(t *testing.T) {
	s := personSchema()
	n := validPerson()
	// "name" expects a scalar; give it a node instead.
	n.Edges[0] = Edge{Label: "name", Target: NodeTarget(NewNode())}
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Code != CodeValidateShapeMismatch || got[0].Path != "$.name" {
		t.Fatalf("Validate() = %+v, want single shape-mismatch at $.name", got)
	}
}

func TestValidateScalarWhereNodeExpected(t *testing.T) {
	s := personSchema()
	n := validPerson()
	// "friend" expects a Person node; give it a scalar instead.
	n.AddValue("friend", ScalarValue(NewStringScalar("not a node")))
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Code != CodeValidateShapeMismatch || got[0].Path != "$.friend" {
		t.Fatalf("Validate() = %+v, want single shape-mismatch at $.friend", got)
	}
}

func TestValidateRootBareValueAgainstRecordRoot(t *testing.T) {
	s := personSchema()
	got := Validate(ValueDocument(ScalarValue(NewStringScalar("x"))), s)
	if len(got) != 1 || got[0].Code != CodeValidateShapeMismatch || got[0].Path != "$" {
		t.Fatalf("Validate() = %+v, want single shape-mismatch at $", got)
	}
}

func TestValidateNullAtNonNullableScalar(t *testing.T) {
	s := personSchema()
	n := validPerson()
	n.Edges[0] = Edge{Label: "name", Target: ValueTarget(NullValue())}
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Code != CodeValidateNullNotAllowed || got[0].Path != "$.name" {
		t.Fatalf("Validate() = %+v, want single null-not-allowed at $.name", got)
	}
}

func TestValidateNullAtNullableScalar(t *testing.T) {
	s := personSchema()
	n := validPerson()
	n.AddValue("coupon", NullValue())
	got := Validate(NodeDocument(n), s)
	if len(got) != 0 {
		t.Fatalf("Validate() = %+v, want empty (null allowed on coupon)", got)
	}
}

func TestValidateWrongScalarKind(t *testing.T) {
	s := personSchema()
	n := validPerson()
	n.AddValue("age", ScalarValue(NewStringScalar("not an integer")))
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Code != CodeValidateTypeMismatch || got[0].Path != "$.age" {
		t.Fatalf("Validate() = %+v, want single type-mismatch at $.age", got)
	}
}

func TestValidateUndeclaredFieldNoDescent(t *testing.T) {
	s := personSchema()
	n := validPerson()
	// "extra" is undeclared. Its subtree is itself invalid (a "tag" edge
	// with the wrong cardinality would independently fail if checked) —
	// confirm validate reports ONLY unexpected-field, proving it never
	// descends into the undeclared field's target (§3.6.1's first "easy to
	// miss" detail).
	bogusChild := NewNode()
	bogusChild.AddValue("whatever", ScalarValue(NewIntegerScalar(big.NewInt(42))))
	n.AddNode("extra", bogusChild)
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Code != CodeValidateUnexpectedField || got[0].Path != "$.extra" {
		t.Fatalf("Validate() = %+v, want exactly one unexpected-field at $.extra", got)
	}
}

func TestValidateCardinalityTooFew(t *testing.T) {
	s := personSchema()
	n := NewNode()
	n.AddValue("name", ScalarValue(NewStringScalar("Ann")))
	n.AddValue("tag", ScalarValue(NewStringScalar("a"))) // only 1, needs [2,3]
	n.AddValue("data", ScalarValue(NewStringScalar("x")))
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Code != CodeValidateCardinality || got[0].Path != "$" {
		t.Fatalf("Validate() = %+v, want single cardinality error at $", got)
	}
}

func TestValidateCardinalityTooMany(t *testing.T) {
	s := personSchema()
	n := validPerson()
	n.AddValue("tag", ScalarValue(NewStringScalar("c")))
	n.AddValue("tag", ScalarValue(NewStringScalar("d"))) // now 4, max is 3
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Code != CodeValidateCardinality || got[0].Path != "$" {
		t.Fatalf("Validate() = %+v, want single cardinality error at $ (nonzero-but-wrong count)", got)
	}
}

func TestValidateCardinalityMissingRequiredIsZeroCount(t *testing.T) {
	s := personSchema()
	n := NewNode()
	n.AddValue("tag", ScalarValue(NewStringScalar("a")))
	n.AddValue("tag", ScalarValue(NewStringScalar("b")))
	n.AddValue("data", ScalarValue(NewStringScalar("x")))
	// "name" (required, [1,1]) entirely absent.
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Code != CodeValidateCardinality || got[0].Path != "$" {
		t.Fatalf("Validate() = %+v, want single cardinality error at $ for missing name", got)
	}
}

func TestValidateCardinalityUnbounded(t *testing.T) {
	s := personSchema()
	n := validPerson()
	for i := 0; i < 50; i++ {
		n.AddValue("email", ScalarValue(NewStringScalar("e")))
	}
	got := Validate(NodeDocument(n), s)
	if len(got) != 0 {
		t.Fatalf("Validate() = %+v, want empty (email is [0,] unbounded)", got)
	}
}

func TestValidateAnyFieldAcceptsArbitrarySubtree(t *testing.T) {
	s := personSchema()
	n := validPerson()
	// Replace "data" with an arbitrarily-shaped, otherwise-invalid subtree:
	// any label, any nesting. It must validate unchecked.
	weird := NewNode()
	weird.AddValue("anything goes", ScalarValue(NewIntegerScalar(big.NewInt(42))))
	inner := NewNode()
	inner.AddValue("x", NullValue())
	weird.AddNode("nested too", inner)
	n.Edges[3] = Edge{Label: "data", Target: NodeTarget(weird)}
	got := Validate(NodeDocument(n), s)
	if len(got) != 0 {
		t.Fatalf("Validate() = %+v, want empty (any accepts anything)", got)
	}
}

func TestValidateNullableOptionalFourCombinations(t *testing.T) {
	s := personSchema()

	// present, non-null: valid.
	n1 := validPerson()
	n1.AddValue("coupon", ScalarValue(NewStringScalar("SAVE10")))
	if got := Validate(NodeDocument(n1), s); len(got) != 0 {
		t.Errorf("present-non-null: got %+v, want empty", got)
	}

	// present, null: valid (coupon is nullable).
	n2 := validPerson()
	n2.AddValue("coupon", NullValue())
	if got := Validate(NodeDocument(n2), s); len(got) != 0 {
		t.Errorf("present-null: got %+v, want empty", got)
	}

	// absent: valid (coupon is [0,1]).
	n3 := validPerson()
	if got := Validate(NodeDocument(n3), s); len(got) != 0 {
		t.Errorf("absent: got %+v, want empty", got)
	}

	// present more than once: invalid, cardinality (max 1).
	n4 := validPerson()
	n4.AddValue("coupon", ScalarValue(NewStringScalar("A")))
	n4.AddValue("coupon", ScalarValue(NewStringScalar("B")))
	got := Validate(NodeDocument(n4), s)
	if len(got) != 1 || got[0].Code != CodeValidateCardinality {
		t.Errorf("present-twice: got %+v, want single cardinality error", got)
	}
}

func TestValidateDepthLimitExceeded(t *testing.T) {
	s := personSchema()
	limits := DefaultLimits()

	// Build a chain of self-referencing "friend" nodes one level deeper
	// than MaxDepth. Each node still satisfies Person's other required
	// fields so the ONLY failure is the depth limit, isolating it from any
	// validate.* finding.
	leaf := func() *Node {
		n := NewNode()
		n.AddValue("name", ScalarValue(NewStringScalar("Ann")))
		n.AddValue("tag", ScalarValue(NewStringScalar("a")))
		n.AddValue("tag", ScalarValue(NewStringScalar("b")))
		n.AddValue("data", ScalarValue(NewStringScalar("x")))
		return n
	}
	root := leaf()
	cur := root
	for i := 0; i < limits.MaxDepth+2; i++ {
		child := leaf()
		cur.AddNode("friend", child)
		cur = child
	}

	got := Validate(NodeDocument(root), s)
	if len(got) == 0 {
		t.Fatal("Validate() = empty, want a depth-limit diagnostic")
	}
	for _, d := range got {
		if d.Code != CodeDocumentLimitDepth {
			t.Errorf("unexpected diagnostic code %v alongside depth limit: %+v", d.Code, got)
		}
	}
	if !hasCode(got, CodeDocumentLimitDepth) {
		t.Fatalf("Validate() = %+v, want a document.limit.depth diagnostic, not a validate.* finding", got)
	}
}

func TestValidateOrderIndependence(t *testing.T) {
	s := personSchema()

	a := NewNode()
	a.AddValue("name", ScalarValue(NewStringScalar("Ann")))
	a.AddValue("tag", ScalarValue(NewStringScalar("x")))
	a.AddValue("tag", ScalarValue(NewStringScalar("y")))
	a.AddValue("data", ScalarValue(NewStringScalar("d")))

	// Same edges, permuted order (D-3's own worked example shape:
	// [(a,1),(b,2)] vs [(b,2),(a,1)]).
	b := NewNode()
	b.AddValue("data", ScalarValue(NewStringScalar("d")))
	b.AddValue("tag", ScalarValue(NewStringScalar("y")))
	b.AddValue("tag", ScalarValue(NewStringScalar("x")))
	b.AddValue("name", ScalarValue(NewStringScalar("Ann")))

	gotA := Validate(NodeDocument(a), s)
	gotB := Validate(NodeDocument(b), s)
	if len(gotA) != 0 || len(gotB) != 0 {
		t.Fatalf("Validate(a) = %+v, Validate(b) = %+v, want both empty", gotA, gotB)
	}

	// Also confirm order independence on an INVALID document: the same
	// multiset of edges, permuted, must produce the same diagnostics
	// (same codes/paths), not merely both-empty.
	c := NewNode()
	c.AddValue("name", ScalarValue(NewStringScalar("Ann")))
	c.AddValue("tag", ScalarValue(NewStringScalar("only one"))) // too few
	c.AddValue("data", ScalarValue(NewStringScalar("d")))

	d := NewNode()
	d.AddValue("data", ScalarValue(NewStringScalar("d")))
	d.AddValue("name", ScalarValue(NewStringScalar("Ann")))
	d.AddValue("tag", ScalarValue(NewStringScalar("only one")))

	gotC := Validate(NodeDocument(c), s)
	gotD := Validate(NodeDocument(d), s)
	if len(gotC) != 1 || len(gotD) != 1 || gotC[0] != gotD[0] {
		t.Fatalf("Validate(c) = %+v, Validate(d) = %+v, want identical single diagnostic", gotC, gotD)
	}
}

func TestValidateMultipleSimultaneousFailures(t *testing.T) {
	s := personSchema()
	n := NewNode()
	// Missing "name" (cardinality) AND "age" has the wrong kind AND an
	// undeclared field is present: three independent problems, all MUST be
	// reported (§3.6: "MUST report every failure, not just the first").
	n.AddValue("tag", ScalarValue(NewStringScalar("a")))
	n.AddValue("tag", ScalarValue(NewStringScalar("b")))
	n.AddValue("data", ScalarValue(NewStringScalar("x")))
	n.AddValue("age", ScalarValue(NewStringScalar("not an integer")))
	n.AddValue("bogus", ScalarValue(NewStringScalar("nope")))

	got := Validate(NodeDocument(n), s)
	if len(got) != 3 {
		t.Fatalf("Validate() = %+v (len %d), want 3 independent diagnostics", got, len(got))
	}
	if !hasCode(got, CodeValidateCardinality) || !hasCode(got, CodeValidateTypeMismatch) || !hasCode(got, CodeValidateUnexpectedField) {
		t.Fatalf("Validate() codes = %v, want cardinality+type-mismatch+unexpected-field", codesOf(got))
	}
}

func TestValidateRepeatedLabelPathIndex(t *testing.T) {
	s := personSchema()
	n := validPerson()
	n.AddValue("email", ScalarValue(NewIntegerScalar(big.NewInt(42)))) // wrong kind, first "email"
	n.AddValue("email", ScalarValue(NewStringScalar("ok")))            // wrong kind on first only
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 {
		t.Fatalf("Validate() = %+v, want single type-mismatch", got)
	}
	if got[0].Path != "$.email[0]" {
		t.Errorf("Path = %q, want $.email[0] (repeated-label occurrence index)", got[0].Path)
	}
}

func TestValidateSingleOccurrenceLabelHasNoIndex(t *testing.T) {
	s := personSchema()
	n := validPerson()
	n.Edges[0] = Edge{Label: "name", Target: NodeTarget(NewNode())}
	got := Validate(NodeDocument(n), s)
	if len(got) != 1 || got[0].Path != "$.name" {
		t.Fatalf("Validate() = %+v, want path $.name with no bracketed index", got)
	}
}

func TestCardinalityMessageBoundedAndUnbounded(t *testing.T) {
	bounded := cardinalityMessage(Field{Label: "tag", Cardinality: Cardinality{Min: 2, Max: 3}}, 1)
	want := "field tag occurs 1 time(s), expected [2,3]"
	if bounded != want {
		t.Errorf("cardinalityMessage(bounded) = %q, want %q", bounded, want)
	}
	unbounded := cardinalityMessage(Field{Label: "email", Cardinality: Cardinality{Min: 0, Unbounded: true}}, 5)
	wantU := "field email occurs 5 time(s), expected [0,unbounded]"
	if unbounded != wantU {
		t.Errorf("cardinalityMessage(unbounded) = %q, want %q", unbounded, wantU)
	}
}

// TestResolveTypeDefaultCase exercises resolveType's defensive default
// branch, which only guards against a future TypeKind constant added
// without updating resolveType — schema.go's exported Type struct makes
// this reachable from outside the package too, so it is not truly dead
// code, just never hit by any legally-constructed Type today.
func TestResolveTypeDefaultCase(t *testing.T) {
	got := resolveType(personSchema(), Type{Kind: TypeKind(99)})
	if got.kind != resolvedAny {
		t.Fatalf("resolveType(unknown kind) = %+v, want resolvedAny", got)
	}
}

// TestValidateUnresolvableRef covers a Ref type whose name is absent from
// Env — see resolved's doc comment: that's a schema.* well-formedness
// concern from issue #5, not something validate re-checks, but validate
// must still behave sanely (treat it as a closed record with no fields)
// rather than panicking. Exercises conformRecord's and findField's nil-rec
// branches.
func TestValidateUnresolvableRef(t *testing.T) {
	person := &Record{
		Name: "Person",
		Fields: []Field{
			{Label: "ghost", Type: RefType("Ghost"), Cardinality: Cardinality{Min: 0, Max: 1}},
		},
	}
	s := Schema{Root: "Person", Env: map[string]*Record{"Person": person}}

	ghostNode := NewNode()
	ghostNode.AddValue("x", ScalarValue(NewStringScalar("anything")))
	root := NewNode()
	root.AddNode("ghost", ghostNode)

	got := Validate(NodeDocument(root), s)
	if len(got) != 1 || got[0].Code != CodeValidateUnexpectedField || got[0].Path != "$.ghost.x" {
		t.Fatalf("Validate() = %+v, want single unexpected-field at $.ghost.x (unresolvable ref treated as empty closed record)", got)
	}
}
