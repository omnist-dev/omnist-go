package omnist

import "strconv"

// SubMicrosecond reports whether t carries a fractional second that needs
// more than six digits. Every reader in this module accepts a fraction of at
// most six digits (OML's `1*6DIGIT`, and the same bound in the YAML and TOML
// readers), so a time with a finer fraction cannot be written as a native
// temporal literal and read back.
func SubMicrosecond(t TimeValue) bool { return t.Nanosecond%1000 != 0 }

// StringifySubMicrosecond is the writer-side half of that rule (fail, don't
// invent): a writer whose native temporal literal allows at most six fraction
// digits (OML, YAML, TOML) calls it first. Every time or datetime leaf with a
// fraction finer than a microsecond is replaced by its exact ISO-8601 text as
// a string leaf, with one format.temporal-stringified warning at the leaf's
// Document path; nothing is rounded or truncated. A Document with no such
// leaf is returned as it is (no copy), and the input is never modified.
func StringifySubMicrosecond(d Document) (Document, []Diagnostic) {
	if !d.IsNode {
		v, diags := stringifyValue(d.Value, "$")
		return Document{Value: v}, diags
	}
	if !nodeHasSubMicrosecond(d.Node) {
		return d, nil
	}
	var diags []Diagnostic
	return Document{IsNode: true, Node: stringifyNode(d.Node, "$", &diags)}, diags
}

func valueHasSubMicrosecond(v *Value) bool {
	if v.IsNull {
		return false
	}
	switch v.Scalar.Kind {
	case KindTime:
		return SubMicrosecond(v.Scalar.Time)
	case KindDateTime:
		return SubMicrosecond(v.Scalar.DateTime.Time)
	}
	return false
}

func nodeHasSubMicrosecond(n *Node) bool {
	for i := range n.Edges {
		e := &n.Edges[i]
		if e.Target.isNode {
			if nodeHasSubMicrosecond(e.Target.node) {
				return true
			}
		} else if valueHasSubMicrosecond(&e.Target.value) {
			return true
		}
	}
	return false
}

func stringifyValue(v Value, path string) (Value, []Diagnostic) {
	if !valueHasSubMicrosecond(&v) {
		return v, nil
	}
	var text, kind string
	if v.Scalar.Kind == KindTime {
		text, kind = FormatISOTime(v.Scalar.Time), "time"
	} else {
		text, kind = FormatISODate(v.Scalar.DateTime.Date)+"T"+FormatISOTime(v.Scalar.DateTime.Time), "datetime"
	}
	return ScalarValue(NewStringScalar(text)), []Diagnostic{{
		Path:     path,
		Code:     CodeFormatTemporalStringified,
		Message:  "a " + kind + " leaf with a fraction finer than six digits has no native spelling, so it is written as its exact ISO-8601 text and reads back as a string",
		Severity: SeverityWarning,
	}}
}

func stringifyNode(n *Node, path string, diags *[]Diagnostic) *Node {
	counts := make(map[string]int, len(n.Edges))
	for _, e := range n.Edges {
		counts[e.Label]++
	}
	seen := make(map[string]int, len(counts))
	out := &Node{Edges: make([]Edge, len(n.Edges))}
	for i, e := range n.Edges {
		child := path + PathLabelSegment(e.Label)
		if counts[e.Label] > 1 {
			child += "[" + strconv.Itoa(seen[e.Label]) + "]"
		}
		seen[e.Label]++
		if e.Target.isNode {
			out.Edges[i] = Edge{Label: e.Label, Target: NodeTarget(stringifyNode(e.Target.node, child, diags))}
			continue
		}
		v, ds := stringifyValue(e.Target.value, child)
		*diags = append(*diags, ds...)
		out.Edges[i] = Edge{Label: e.Label, Target: ValueTarget(v)}
	}
	return out
}
