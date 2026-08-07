package omnist

import "testing"

func TestRootPathString(t *testing.T) {
	if got := RootPath().String(); got != "$" {
		t.Errorf("RootPath().String() = %q, want %q", got, "$")
	}
}

func TestPathChildSingleOccurrence(t *testing.T) {
	p := RootPath().Child("name", 0, false)
	if got := p.String(); got != "$.name" {
		t.Errorf("got %q, want %q", got, "$.name")
	}
}

func TestPathChildRepeatedOccurrence(t *testing.T) {
	// Spec §8.4 worked examples.
	p0 := RootPath().Child("item", 0, true)
	if got := p0.String(); got != "$.item[0]" {
		t.Errorf("got %q, want %q", got, "$.item[0]")
	}

	p2sku := RootPath().Child("item", 2, true).Child("sku", 0, false)
	if got := p2sku.String(); got != "$.item[2].sku" {
		t.Errorf("got %q, want %q", got, "$.item[2].sku")
	}
}

func TestPathChildDoesNotMutateParent(t *testing.T) {
	root := RootPath()
	child := root.Child("a", 0, false)
	if root.String() != "$" {
		t.Errorf("parent path mutated: %q", root.String())
	}
	if child.String() != "$.a" {
		t.Errorf("child path = %q, want %q", child.String(), "$.a")
	}
	// Branching from the same parent must not share backing storage.
	sibling := root.Child("b", 0, false)
	if child.String() != "$.a" || sibling.String() != "$.b" {
		t.Errorf("sibling branches interfered: child=%q sibling=%q", child.String(), sibling.String())
	}
}

func TestPathIndexInNode(t *testing.T) {
	// [(item,"pen"), (note,"rush"), (item,"pad")] from spec §2.1's example.
	n := NewNode().
		AddValue("item", ScalarValue(NewStringScalar("pen"))).
		AddValue("note", ScalarValue(NewStringScalar("rush"))).
		AddValue("item", ScalarValue(NewStringScalar("pad")))

	occ, repeated := PathIndexInNode(n, 0)
	if occ != 0 || !repeated {
		t.Errorf("edge 0 (item): occ=%d repeated=%v, want 0,true", occ, repeated)
	}
	occ, repeated = PathIndexInNode(n, 1)
	if occ != 0 || repeated {
		t.Errorf("edge 1 (note): occ=%d repeated=%v, want 0,false", occ, repeated)
	}
	occ, repeated = PathIndexInNode(n, 2)
	if occ != 1 || !repeated {
		t.Errorf("edge 2 (item): occ=%d repeated=%v, want 1,true", occ, repeated)
	}
}

func TestPathIndexInNodeThreeItems(t *testing.T) {
	// Spec §8.4: "$.item[2].sku" implies a node with at least 3 `item`
	// edges, where the third (index 2) has a `sku` child.
	sku := NewNode().AddValue("sku", ScalarValue(NewStringScalar("abc")))
	n := NewNode().
		AddValue("item", ScalarValue(NewStringScalar("a"))).
		AddValue("item", ScalarValue(NewStringScalar("b"))).
		AddNode("item", sku)

	occ, repeated := PathIndexInNode(n, 2)
	if occ != 2 || !repeated {
		t.Errorf("third item: occ=%d repeated=%v, want 2,true", occ, repeated)
	}
	p := RootPath().Child("item", occ, repeated).Child("sku", 0, false)
	if got := p.String(); got != "$.item[2].sku" {
		t.Errorf("got %q, want %q", got, "$.item[2].sku")
	}
}

func TestPathIndexInNodePanicsOutOfRange(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected a panic for out-of-range edgeIndex")
		}
	}()
	n := NewNode()
	PathIndexInNode(n, 0)
}
