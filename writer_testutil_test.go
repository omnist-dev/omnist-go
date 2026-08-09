package omnist

import "math"

// This file holds structural-equality helpers for oml_writer_test.go's
// Document round-trip properties. Document/Node exposes no public Equal
// method (only Scalar does), so the round-trip property tests —
// "ReadOML(WriteOML(d)) == d" — need their own deep-equality walk.
// reflect.DeepEqual is deliberately not used: it would treat two
// KindNumber NaN scalars as unequal (NaN != NaN under ==, which is what
// DeepEqual falls back to for float64), which is exactly the case the nan
// round-trip test below needs to treat as equal.
//
// The Schema-shaped counterparts (typeEqual/fieldEqual/recordEqual/
// schemaEqual) moved to osd/osd_testutil_test.go along with
// osd_writer_test.go (issue #41) — they were only ever used there.

func valueEqual(a, b Value) bool {
	if a.IsNull != b.IsNull {
		return false
	}
	if a.IsNull {
		return true
	}
	return scalarEqual(a.Scalar, b.Scalar)
}

// scalarEqual defers to Scalar.Equal for every kind except KindNumber,
// where it additionally treats two NaNs as equal (Scalar.Equal uses plain
// ==, under which NaN != NaN) so round-trip tests covering the "nan"
// reserved spelling can assert equality in the normal way.
func scalarEqual(a, b Scalar) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind == KindNumber && math.IsNaN(a.Num) && math.IsNaN(b.Num) {
		return true
	}
	return a.Equal(b)
}

func targetEqual(a, b Target) bool {
	an, aok := a.Node()
	bn, bok := b.Node()
	if aok != bok {
		return false
	}
	if aok {
		return nodeEqual(an, bn)
	}
	av, _ := a.Value()
	bv, _ := b.Value()
	return valueEqual(av, bv)
}

func nodeEqual(a, b *Node) bool {
	if len(a.Edges) != len(b.Edges) {
		return false
	}
	for i := range a.Edges {
		if a.Edges[i].Label != b.Edges[i].Label {
			return false
		}
		if !targetEqual(a.Edges[i].Target, b.Edges[i].Target) {
			return false
		}
	}
	return true
}

func docEqual(a, b Document) bool {
	if a.IsNode != b.IsNode {
		return false
	}
	if a.IsNode {
		return nodeEqual(a.Node, b.Node)
	}
	return valueEqual(a.Value, b.Value)
}
