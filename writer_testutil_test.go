package omnist

import "math"

// This file holds structural-equality helpers shared by oml_writer_test.go
// and osd_writer_test.go. Neither Document/Node nor Schema/Record exposes
// a public Equal method (only Scalar does), so the round-trip property
// tests — "ReadOML(WriteOML(d)) == d" — need their own deep-equality walk.
// reflect.DeepEqual is deliberately not used: it would treat two
// KindNumber NaN scalars as unequal (NaN != NaN under ==, which is what
// DeepEqual falls back to for float64), which is exactly the case the nan
// round-trip test below needs to treat as equal.

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

func typeEqual(a, b Type) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case TypeScalarKind:
		return a.ScalarKind == b.ScalarKind && a.Nullable == b.Nullable
	case TypeRefKind:
		return a.RefName == b.RefName
	default: // TypeAnyKind
		return true
	}
}

func fieldEqual(a, b Field) bool {
	return a.Label == b.Label && typeEqual(a.Type, b.Type) && a.Cardinality == b.Cardinality
}

func recordEqual(a, b *Record) bool {
	if a.Name != b.Name || len(a.Fields) != len(b.Fields) {
		return false
	}
	for i := range a.Fields {
		if !fieldEqual(a.Fields[i], b.Fields[i]) {
			return false
		}
	}
	return true
}

func schemaEqual(a, b Schema) bool {
	if a.Root != b.Root {
		return false
	}
	if len(a.EnvOrder) != len(b.EnvOrder) {
		return false
	}
	for i := range a.EnvOrder {
		if a.EnvOrder[i] != b.EnvOrder[i] {
			return false
		}
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for name, rec := range a.Env {
		other, ok := b.Env[name]
		if !ok || !recordEqual(rec, other) {
			return false
		}
	}
	return true
}
