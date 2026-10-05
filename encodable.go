package omnist

import (
	"strconv"
	"unicode/utf8"
)

// CheckEncodable is C-9's writer-side check (spec §7.3): a writer MUST fail,
// unconditionally, with write.unsupported-value on a string value or an edge
// label that has no UTF-8 encoding, and a Go string can hold any bytes. Every
// writer in this repository calls it before writing anything; a writer built
// on this package should too.
//
// It returns nil if every edge label and every string value in d is valid
// UTF-8, and otherwise a write.unsupported-value Diagnostic (the error a
// writer returns) at the Document path of the node holding the first bad
// string: the leaf for a value (indexed per E-10), the node that holds the edge
// for a label, since a path cannot quote a label. A string beneath an edge
// whose own label is bad is reported at that edge's holder, because the label
// is examined before the edge is entered. The check never repairs, replaces or
// escapes anything.
//
// It has two passes so that a Document with nothing wrong in it costs one
// allocation-free walk and no path strings. The first (nodeEncodable) only
// answers yes or no. Only when it says no does the second (locateUnencodable)
// walk again, counting labels per node, to build the path.
func CheckEncodable(d Document) error {
	if !d.IsNode {
		if valueEncodable(&d.Value) {
			return nil
		}
		return unencodableDiagnostic("$", false)
	}
	if nodeEncodable(d.Node) {
		return nil
	}
	path, label, _ := locateUnencodable(d.Node, "$")
	return unencodableDiagnostic(path, label)
}

// valueEncodable reports whether v is encodable: null and every non-string
// scalar always are. It takes a pointer so the walk copies no Value.
func valueEncodable(v *Value) bool {
	return v.IsNull || v.Scalar.Kind != KindString || utf8.ValidString(v.Scalar.Str)
}

// nodeEncodable is the pathless fast pass: no path string, no map, no copy of
// an edge, target or value.
func nodeEncodable(n *Node) bool {
	for i := range n.Edges {
		e := &n.Edges[i]
		if !utf8.ValidString(e.Label) {
			return false
		}
		if e.Target.isNode {
			if !nodeEncodable(e.Target.node) {
				return false
			}
			continue
		}
		if !valueEncodable(&e.Target.value) {
			return false
		}
	}
	return true
}

// locateUnencodable walks n (whose Document path is path) in edge order and
// returns the path of the first bad string, whether it is a label, and whether
// one was found beneath n at all (a subtree with nothing wrong in it reports
// false so the walk moves on to the next edge).
func locateUnencodable(n *Node, path string) (string, bool, bool) {
	counts := make(map[string]int, len(n.Edges))
	for _, e := range n.Edges {
		counts[e.Label]++
	}
	seen := make(map[string]int, len(counts))
	for i := range n.Edges {
		e := &n.Edges[i]
		if !utf8.ValidString(e.Label) {
			return path, true, true
		}
		child := path + "." + e.Label
		if counts[e.Label] > 1 {
			child += "[" + strconv.Itoa(seen[e.Label]) + "]"
		}
		seen[e.Label]++
		if e.Target.isNode {
			if p, label, found := locateUnencodable(e.Target.node, child); found {
				return p, label, true
			}
			continue
		}
		if !valueEncodable(&e.Target.value) {
			return child, false, true
		}
	}
	return "", false, false
}

func unencodableDiagnostic(path string, label bool) Diagnostic {
	msg := "a string value that is not well-formed UTF-8 has no UTF-8 encoding and cannot be written (C-9)"
	if label {
		msg = "an edge label that is not well-formed UTF-8 has no UTF-8 encoding and cannot be written (C-9)"
	}
	return Diagnostic{
		Path:     path,
		Code:     CodeWriteUnsupportedValue,
		Message:  msg,
		Severity: SeverityError,
	}
}
