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

// Record is a closed set of fields (spec §3.3, §3.1): only the labels
// listed are allowed, and nothing else — there is no wildcard.
type Record struct {
	Name   string
	Fields []Field
}

// Schema is a graph of named records plus a distinguished root record name
// (spec §3.3: `Schema = (root: Ref, env: Name -> Record)`).
type Schema struct {
	Root string
	Env  map[string]*Record
}
