package omnist

// This file defines the Schema model (spec ch.3, formally §3.3): Schema,
// Record, Field, Type, and Cardinality. This is a separate type family from
// the Document model in document.go (spec ch.2 vs ch.3) — a Schema
// describes the *shape* Documents may take; it is never itself a Document,
// and the two are not conflated here.

// TypeKind identifies which of the three Type alternatives (spec §3.3's
// `Type = Scalar | Ref | Any`) a Type holds.
type TypeKind int

const (
	// TypeScalarKind is one of the seven scalar kinds, optionally nullable.
	TypeScalarKind TypeKind = iota
	// TypeRefKind is a reference to a named Record in the schema's env.
	TypeRefKind
	// TypeAnyKind is the singleton `any` type (spec §3.7).
	TypeAnyKind
)

// Type is a field's type: exactly one scalar kind (optionally nullable), a
// reference to another record (by name), or the `any` type — never a
// choice between candidates (spec §3.1, §3.3). Only the fields relevant to
// Kind are meaningful; the others are zero.
//
// Like Target in document.go, this is a closed struct rather than an
// interface, for the same reason: an interface satisfied by two concrete
// types is always extensible by a third from outside the package, which
// would let a shape outside the spec's closed `Scalar | Ref | Any` union
// leak into a well-formed Schema.
type Type struct {
	Kind TypeKind

	// ScalarKind and Nullable are meaningful only when Kind == TypeScalarKind.
	ScalarKind ScalarKind
	Nullable   bool

	// RefName is meaningful only when Kind == TypeRefKind. It names a
	// record in the schema's env; resolution happens by lookup (spec §3.3
	// S-6), not eagerly at Type-construction time, since forward references
	// and mutual recursion are both legal.
	RefName string
}

// ScalarType constructs a scalar Type. nullable corresponds to a trailing
// `?` in OSD source (spec §5.6).
func ScalarType(kind ScalarKind, nullable bool) Type {
	return Type{Kind: TypeScalarKind, ScalarKind: kind, Nullable: nullable}
}

// RefType constructs a reference Type naming another record.
func RefType(name string) Type {
	return Type{Kind: TypeRefKind, RefName: name}
}

// AnyType returns the singleton `any` type.
func AnyType() Type {
	return Type{Kind: TypeAnyKind}
}

// Cardinality is a closed integer range [Min, Max] bounding the count of
// edges carrying a field's label in a node (spec §3.3, §3.4). Max is
// meaningful only when Unbounded is false.
//
// Per the issue's design-continuity note: cardinality bounds are plain
// non-negative integers or "unbounded", not arbitrary-precision — unlike
// §2.4's integer *literal digit* limit, nothing in the spec calls for
// arbitrary precision here, so a uint64 with an explicit Unbounded
// sentinel is used instead of *big.Int.
type Cardinality struct {
	Min       uint64
	Max       uint64
	Unbounded bool
}

// DefaultCardinality returns the OSD default cardinality [1,1] used when a
// field declares none (spec §5.5).
func DefaultCardinality() Cardinality {
	return Cardinality{Min: 1, Max: 1}
}

// Field is one label a Record allows, per spec §3.3: `Field = (label,
// type, cardinality)`.
type Field struct {
	Label       string
	Type        Type
	Cardinality Cardinality
}

// FieldIndex maps field labels to *Field for O(1) lookups on a Record.
type FieldIndex map[string]*Field

// Index returns a FieldIndex mapping each declared field label to its *Field.
// If r is nil, returns nil.
func (r *Record) Index() FieldIndex {
	if r == nil {
		return nil
	}
	idx := make(FieldIndex, len(r.Fields))
	for i := range r.Fields {
		idx[r.Fields[i].Label] = &r.Fields[i]
	}
	return idx
}

// Field returns the *Field with the given label from the indexed view, or nil if not present.
func (idx FieldIndex) Field(label string) *Field {
	if idx == nil {
		return nil
	}
	return idx[label]
}

// Record is a closed set of fields (spec §3.3, §3.1): only the labels
// listed are allowed, and nothing else — there is no wildcard.
type Record struct {
	Name   string
	Fields []Field
}

// Schema is a graph of named records plus a distinguished root record name
// (spec §3.3: `Schema = (root: Ref, env: Name -> Record)`).
//
// EnvOrder holds the declaration order of Env's keys. The schema algebra
// (spec ch.6, e.g. §6.4's satisfiable_set and §6.5's prune) requires
// deterministic, declaration-order iteration over env wherever output
// ordering is observable — Go's map iteration is deliberately randomized,
// so Env alone cannot satisfy that on its own. Any code that builds a new
// Schema (the OSD parser, and later prune/normalize/extract) MUST keep
// EnvOrder consistent with Env's keys — same set, declaration order.
type Schema struct {
	Root     string
	Env      map[string]*Record
	EnvOrder []string
}
