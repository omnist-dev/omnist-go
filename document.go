package omnist

import "math/big"

// ScalarKind identifies which of the seven scalar kinds a Scalar holds.
//
// Spec §2.2.1 defines exactly seven scalar kinds and is explicit that
// implementations MUST NOT add or collapse kinds, since doing so changes the
// Schema Algebra's subtyping lattice and therefore changes conformance
// results. Do not add an eighth constant here.
type ScalarKind int

const (
	// KindString is a sequence of Unicode code points.
	KindString ScalarKind = iota
	// KindInteger is an arbitrary-precision signed integer, subject to the
	// §2.4 safety limit on digit count.
	KindInteger
	// KindNumber is a real number, represented as IEEE 754 binary64.
	KindNumber
	// KindBoolean is true or false.
	KindBoolean
	// KindDate is a calendar date: year, month, day.
	KindDate
	// KindTime is a time of day, with optional sub-second precision and
	// optional UTC offset.
	KindTime
	// KindDateTime is a date and a time of day, joined.
	KindDateTime
)

// String returns the taxonomy-style lowercase name of the kind (e.g.
// "integer"), matching the kind names used in spec §2.2.1's table.
func (k ScalarKind) String() string {
	switch k {
	case KindString:
		return "string"
	case KindInteger:
		return "integer"
	case KindNumber:
		return "number"
	case KindBoolean:
		return "boolean"
	case KindDate:
		return "date"
	case KindTime:
		return "time"
	case KindDateTime:
		return "datetime"
	default:
		return "unknown"
	}
}

// DateValue is a calendar date: year, month, day (spec §2.2.1 `date`).
type DateValue struct {
	Year  int
	Month int
	Day   int
}

// TimeValue is a time of day, with optional sub-second precision and
// optional UTC offset (spec §2.2.1 `time`).
type TimeValue struct {
	Hour       int
	Minute     int
	Second     int
	Nanosecond int
	// HasOffset reports whether Offset is meaningful. A time value need not
	// carry a UTC offset at all.
	HasOffset bool
	// OffsetSeconds is the UTC offset in seconds when HasOffset is true.
	OffsetSeconds int
}

// DateTimeValue is a date and a time of day, joined (spec §2.2.1 `datetime`).
type DateTimeValue struct {
	Date DateValue
	Time TimeValue
}

// Scalar is a tagged value holding exactly one of the seven scalar kinds.
// Only the field matching Kind is meaningful; the others are zero.
//
// Per the design decision recorded in docs/workflow-playbook.md §2.2,
// integer uses *big.Int (not int64) because spec §2.4 requires supporting
// integer literals up to 4,300 decimal digits.
type Scalar struct {
	Kind ScalarKind

	Str      string
	Int      *big.Int
	Num      float64
	Bool     bool
	Date     DateValue
	Time     TimeValue
	DateTime DateTimeValue
}

// NewStringScalar constructs a string-kind Scalar.
func NewStringScalar(s string) Scalar { return Scalar{Kind: KindString, Str: s} }

// NewIntegerScalar constructs an integer-kind Scalar. It copies v so the
// caller's *big.Int may be safely mutated afterward.
func NewIntegerScalar(v *big.Int) Scalar {
	return Scalar{Kind: KindInteger, Int: new(big.Int).Set(v)}
}

// NewNumberScalar constructs a number-kind Scalar.
func NewNumberScalar(n float64) Scalar { return Scalar{Kind: KindNumber, Num: n} }

// NewBooleanScalar constructs a boolean-kind Scalar.
func NewBooleanScalar(b bool) Scalar { return Scalar{Kind: KindBoolean, Bool: b} }

// NewDateScalar constructs a date-kind Scalar.
func NewDateScalar(d DateValue) Scalar { return Scalar{Kind: KindDate, Date: d} }

// NewTimeScalar constructs a time-kind Scalar.
func NewTimeScalar(t TimeValue) Scalar { return Scalar{Kind: KindTime, Time: t} }

// NewDateTimeScalar constructs a datetime-kind Scalar.
func NewDateTimeScalar(dt DateTimeValue) Scalar {
	return Scalar{Kind: KindDateTime, DateTime: dt}
}

// Equal reports whether s and other are the same scalar per spec D-5:
// two scalars are equal when their kinds AND values are equal. An integer
// and a number of the same magnitude are distinct scalars in the Document
// model even though integer is a subtype of number in the Schema model
// (§6) — so this method deliberately does not do numeric cross-kind
// comparison. Go's built-in == cannot be used for Scalar equality: the Int
// field is a *big.Int pointer, so == would compare pointer identity rather
// than value, and even a value-correct == would not by itself express the
// kind-strictness rule this method exists to enforce.
func (s Scalar) Equal(other Scalar) bool {
	if s.Kind != other.Kind {
		return false
	}
	switch s.Kind {
	case KindString:
		return s.Str == other.Str
	case KindInteger:
		if s.Int == nil || other.Int == nil {
			return s.Int == other.Int
		}
		return s.Int.Cmp(other.Int) == 0
	case KindNumber:
		return s.Num == other.Num
	case KindBoolean:
		return s.Bool == other.Bool
	case KindDate:
		return s.Date == other.Date
	case KindTime:
		return s.Time == other.Time
	case KindDateTime:
		return s.DateTime == other.DateTime
	default:
		return false
	}
}

// Value is a Document value: either a Scalar or null. Per spec §2.2's
// grammar, `value = scalar-value | null`. IsNull distinguishes the null
// value from a zero Scalar, since null carries no scalar kind of its own
// (spec §2.2.1).
type Value struct {
	IsNull bool
	Scalar Scalar
}

// NullValue returns the null value.
func NullValue() Value { return Value{IsNull: true} }

// ScalarValue wraps a Scalar as a Value.
func ScalarValue(s Scalar) Value { return Value{Scalar: s} }

// Target is what an Edge points to: exactly a Value or a Node, per spec
// D-4 ("A target is a value or a node. No third case exists."). The zero
// Target is invalid; construct one with ValueTarget or NodeTarget.
//
// Target is deliberately not an interface. An interface satisfied by two
// concrete types can always be satisfied by a third one added later by a
// caller outside this package, which would let a list-valued or otherwise
// illegal target escape into user-visible Documents — exactly what D-4
// forbids. A closed struct with an internal discriminant cannot be
// extended from outside the package.
type Target struct {
	isNode bool
	value  Value
	node   *Node
}

// ValueTarget constructs a Target holding a value.
func ValueTarget(v Value) Target { return Target{value: v} }

// NodeTarget constructs a Target holding a node. Panics if n is nil, since
// a nil node is not a legal target (it is neither a value nor a node).
func NodeTarget(n *Node) Target {
	if n == nil {
		panic("omnist: NodeTarget called with nil *Node")
	}
	return Target{isNode: true, node: n}
}

// IsNode reports whether the target is a node (as opposed to a value).
func (t Target) IsNode() bool { return t.isNode }

// Node returns the target's node and true if the target is a node,
// otherwise the zero value and false.
func (t Target) Node() (*Node, bool) {
	if !t.isNode {
		return nil, false
	}
	return t.node, true
}

// Value returns the target's value and true if the target is a value,
// otherwise the zero value and false.
func (t Target) Value() (Value, bool) {
	if t.isNode {
		return Value{}, false
	}
	return t.value, true
}

// Edge is a single (label, target) pair within a Node's ordered edge list.
type Edge struct {
	Label  string
	Target Target
}

// Node is an ordered list of labeled edges (spec §2.1/§2.2). Labels MAY
// repeat; nothing in the Document model constrains uniqueness, ordering,
// or the relationship between repeated labels (spec §2.2.2).
//
// Per the design decision recorded in docs/workflow-playbook.md §2.3, Node
// stays edge-list-native everywhere: there is no separate map-collapsed
// type. Callers append to Edges directly to build a Document; this
// preserves invariant D-1 (edge order is exactly construction order) and
// D-2 (repeated labels remain separate edges, never merged into a list) by
// construction, since there is no map-shaped alternative to accidentally
// use instead.
type Node struct {
	Edges []Edge
}

// NewNode returns an empty Node.
func NewNode() *Node { return &Node{} }

// AddValue appends an edge with the given label pointing at a value
// target, in place, and returns the node for chaining.
func (n *Node) AddValue(label string, v Value) *Node {
	n.Edges = append(n.Edges, Edge{Label: label, Target: ValueTarget(v)})
	return n
}

// AddNode appends an edge with the given label pointing at a node target,
// in place, and returns the node for chaining.
func (n *Node) AddNode(label string, child *Node) *Node {
	n.Edges = append(n.Edges, Edge{Label: label, Target: NodeTarget(child)})
	return n
}

// Document is a node or a bare value (spec §2.2: `Document = node | value`).
// Exactly one of Node or Value is meaningful, selected by IsNode.
type Document struct {
	IsNode bool
	Node   *Node
	Value  Value
}

// NodeDocument wraps a Node as a Document.
func NodeDocument(n *Node) Document { return Document{IsNode: true, Node: n} }

// ValueDocument wraps a Value as a Document (a bare-value Document).
func ValueDocument(v Value) Document { return Document{Value: v} }
