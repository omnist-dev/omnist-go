package omnist

import "strconv"

// pathSegment is one label step in a Document path, per spec §8.4.
type pathSegment struct {
	label    string
	index    int
	hasIndex bool
}

// Path is a Document or Schema path, per spec §8.4: it starts at "$" and
// descends by label, disambiguating a repeated label with a zero-based
// occurrence index in brackets. The index is present if and only if the
// label occurs more than once in the node it appears in — Path never
// decides that on its own; callers supply it (typically via
// PathIndexInNode) because only the caller walking a Node knows how many
// edges in it share a label.
//
// The zero Path is the root path "$".
type Path struct {
	segments []pathSegment
}

// RootPath returns the path to the whole Document or schema: "$".
func RootPath() Path {
	return Path{}
}

// Child returns a new Path formed by descending one edge labeled label.
// If repeated is true, index is rendered as a bracketed occurrence index
// (e.g. "$.item[2]"); if repeated is false, index is ignored and no
// bracket is rendered (e.g. "$.name"). Per spec §8.4 the index MUST be
// present exactly when the label occurs more than once in that node, so
// callers must determine repeated (e.g. via PathIndexInNode) rather than
// always passing true.
func (p Path) Child(label string, index int, repeated bool) Path {
	next := make([]pathSegment, len(p.segments), len(p.segments)+1)
	copy(next, p.segments)
	next = append(next, pathSegment{label: label, index: index, hasIndex: repeated})
	return Path{segments: next}
}

// String renders the path per spec §8.4: "$" for the root, "$.label" for
// a single-occurrence edge, "$.label[N]" for a repeated label.
func (p Path) String() string {
	out := "$"
	for _, seg := range p.segments {
		out += "." + seg.label
		if seg.hasIndex {
			out += "[" + strconv.Itoa(seg.index) + "]"
		}
	}
	return out
}

// PathIndexInNode reports the zero-based occurrence index of the edge at
// edgeIndex within node.Edges, counting only edges sharing that edge's
// label, and whether that label occurs more than once in node overall.
// It is the helper a reader walking a Node uses to build the (index,
// repeated) arguments Path.Child needs, so the "index present iff label
// repeats" rule (spec §8.4) is computed in one place rather than
// re-derived at every call site.
//
// Panics if edgeIndex is out of range, since that indicates a caller bug
// rather than a reportable condition.
func PathIndexInNode(node *Node, edgeIndex int) (occurrence int, repeated bool) {
	label := node.Edges[edgeIndex].Label
	count := 0
	occurrenceOfTarget := -1
	for i, e := range node.Edges {
		if e.Label != label {
			continue
		}
		if i == edgeIndex {
			occurrenceOfTarget = count
		}
		count++
	}
	return occurrenceOfTarget, count > 1
}
