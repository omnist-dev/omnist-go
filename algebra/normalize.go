package algebra

import (
	"sort"

	"github.com/omnist-dev/omnist-go"
)

// This file implements §6.8's normalize(S) (port-order step 7), the
// paper's MinimizeSA: the canonical minimal schema equivalent to S. It
// depends on Prune/IsEmpty/SatisfiableSet (issue #9, algebra.go) and the
// Schema/Record/Field/Type/Cardinality/EnvOrder types (issue #5).

// localSig is the comparable value used as a map key for local_signature
// (§6.8 step 2): a byte-encoding of each field's (label, min, max,
// shape_of(type)) — shape_of distinguishes Any, Ref (target-blind), and
// Scalar(kind, nullable), per §6.8's formal local_signature/shape_of
// pseudocode — in declaration order. Record.Fields is already a
// slice in declaration order (schema.go); two records with the same
// fields in a different order are NOT the same local signature, matching
// the spec's field-by-field tuple description — there is no sort here.
// (Sorting by label happens one level up, in refine_key, per the
// pseudocode.) The encoding uses control-byte separators (see
// appendFieldSig/appendEscapedLabel) so no two distinct field lists can
// collide.
type localSig string

// computeLocalSignature builds the comparable local signature for rec, per
// §6.8 step 2. Returned as a string so it's directly usable as a Go map
// key (the issue's requirement: "usable as a Go map key or otherwise
// comparable").
func computeLocalSignature(rec *omnist.Record) localSig {
	var b []byte
	for _, f := range rec.Fields {
		b = appendFieldSig(b, f)
	}
	return localSig(b)
}

// appendFieldSig appends one field's (label, min, max, shape_of(type))
// encoding to b, using '\x00'/'\x01' as field/component separators that
// cannot appear in the numeric or boolean components and are escaped out
// of the label so a crafted label can't forge a delimiter collision.
func appendFieldSig(b []byte, f omnist.Field) []byte {
	b = append(b, '\x00')
	b = appendEscapedLabel(b, f.Label)
	b = append(b, '\x01')
	b = appendUint(b, f.Cardinality.Min)
	b = append(b, '\x01')
	if f.Cardinality.Unbounded {
		b = append(b, 'U')
	} else {
		b = appendUint(b, f.Cardinality.Max)
	}
	b = append(b, '\x01')
	switch f.Type.Kind {
	case omnist.TypeAnyKind:
		b = append(b, 'A')
	case omnist.TypeRefKind:
		// Target name deliberately excluded here: refine_key resolves
		// ref equivalence via the fixpoint step, not the local
		// signature (§6.8's shape_of: `if t is Ref: return ("ref",)`).
		b = append(b, 'R')
	default: // omnist.TypeScalarKind
		// §6.8's shape_of: `return ("scalar", t.kind, t.nullable)` —
		// both the scalar kind and nullability are part of the
		// signature, so distinct scalar kinds (and nullable vs
		// non-nullable) never collapse into one bucket.
		b = append(b, 'S')
		b = append(b, byte(f.Type.ScalarKind))
		b = append(b, '\x01')
		if f.Type.Nullable {
			b = append(b, 'N')
		} else {
			b = append(b, 'n')
		}
	}
	return b
}

// appendEscapedLabel appends label with '\x00' and '\x01' bytes escaped
// (prefixed with '\x02'), so an adversarial label containing those control
// bytes cannot forge a false signature collision or split.
func appendEscapedLabel(b []byte, label string) []byte {
	for i := 0; i < len(label); i++ {
		c := label[i]
		if c == '\x00' || c == '\x01' || c == '\x02' {
			b = append(b, '\x02')
		}
		b = append(b, c)
	}
	return b
}

// appendUint appends the decimal digits of v to b without allocating via
// fmt, since this runs in the hot path of grouping every field of every
// record.
func appendUint(b []byte, v uint64) []byte {
	start := len(b)
	if v == 0 {
		return append(b, '0')
	}
	for v > 0 {
		b = append(b, byte('0'+v%10))
		v /= 10
	}
	// digits were appended least-significant-first; reverse them in place.
	for i, j := start, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

// refineFieldKey is one field's contribution to refine_key (§6.8): like
// localFieldSig but with the reference target resolved to its *current
// block index* rather than ignored, so refinement can tell apart records
// whose references point at different (non-mergeable) sub-blocks.
type refineFieldKey struct {
	Label     string
	Min       uint64
	Max       uint64
	Unbounded bool
	IsRef     bool
	Block     int // meaningful only when IsRef; -1 otherwise
}

// computeRefineKey implements §6.8's refine_key(rec, block_of): the
// record's local_signature combined with, for every field sorted by
// label, (label, min, max, block_of[target] if ref else none). Returned
// as a comparable string so blocks can be grouped by it in a Go map, same
// approach as localSig.
func computeRefineKey(rec *omnist.Record, blockOf map[string]int) string {
	fields := make([]refineFieldKey, len(rec.Fields))
	for i, f := range rec.Fields {
		fields[i] = refineFieldKey{
			Label:     f.Label,
			Min:       f.Cardinality.Min,
			Max:       f.Cardinality.Max,
			Unbounded: f.Cardinality.Unbounded,
			IsRef:     f.Type.Kind == omnist.TypeRefKind,
			Block:     -1,
		}
		if f.Type.Kind == omnist.TypeRefKind {
			fields[i].Block = blockOf[f.Type.RefName]
		}
	}
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Label < fields[j].Label
	})

	b := []byte(computeLocalSignature(rec))
	b = append(b, '\x03') // separates local_signature from the sorted-field part
	for _, f := range fields {
		b = append(b, '\x00')
		b = appendEscapedLabel(b, f.Label)
		b = append(b, '\x01')
		b = appendUint(b, f.Min)
		b = append(b, '\x01')
		if f.Unbounded {
			b = append(b, 'U')
		} else {
			b = appendUint(b, f.Max)
		}
		b = append(b, '\x01')
		if f.IsRef {
			b = append(b, 'R')
			b = appendUint(b, uint64(f.Block))
		} else {
			b = append(b, 'N') // none: not a reference field
		}
	}
	return string(b)
}

// EquivalenceClasses implements §6.8's equivalence_classes(S): the
// structural partition of S.Env's record names, WITHOUT pruning first —
// callers (Normalize here, lint in a later issue) decide whether to prune
// before calling this. Blocks and the names within each block are both in
// deterministic (sorted-name) order, satisfying the spec's determinism
// requirement that two conformant implementations produce identical
// output, not merely equivalent output.
func EquivalenceClasses(s omnist.Schema) [][]string {
	names := make([]string, len(s.EnvOrder))
	copy(names, s.EnvOrder)
	sort.Strings(names)

	// Initial grouping by local signature.
	blocks := groupBy(names, func(name string) string {
		return string(computeLocalSignature(s.Env[name]))
	})
	blockOf := indexBlocks(blocks)

	// Refine to a fixpoint: repeat until the block count stops changing.
	for {
		var newBlocks [][]string
		for _, block := range blocks {
			refined := groupBy(block, func(name string) string {
				return computeRefineKey(s.Env[name], blockOf)
			})
			newBlocks = append(newBlocks, refined...)
		}
		if len(newBlocks) == len(blocks) {
			blocks = newBlocks
			break
		}
		blocks = newBlocks
		blockOf = indexBlocks(blocks)
	}
	return blocks
}

// groupBy partitions names (assumed already sorted) into blocks sharing
// the same key(name), preserving the sorted order both across blocks (by
// each block's first-seen name) and within each block.
func groupBy(names []string, key func(string) string) [][]string {
	order := make([]string, 0, len(names))
	buckets := make(map[string][]string, len(names))
	for _, n := range names {
		k := key(n)
		if _, ok := buckets[k]; !ok {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], n)
	}
	blocks := make([][]string, len(order))
	for i, k := range order {
		blocks[i] = buckets[k]
	}
	return blocks
}

// indexBlocks builds block_of: the index of the block each name currently
// belongs to.
func indexBlocks(blocks [][]string) map[string]int {
	idx := make(map[string]int)
	for i, block := range blocks {
		for _, n := range block {
			idx[n] = i
		}
	}
	return idx
}

// Normalize implements §6.8's normalize(S): prune, short-circuit if empty,
// partition into equivalence classes, pick each block's lexicographically
// smallest name as its representative, rewrite every reference (including
// the root) to representatives, and keep only representatives in the new
// env.
func Normalize(s omnist.Schema) omnist.Schema {
	s = Prune(s)
	if IsEmpty(s) {
		return s
	}

	rep := make(map[string]string, len(s.EnvOrder))
	for _, block := range EquivalenceClasses(s) {
		// block[0] is the lexicographic minimum: groupBy (called by
		// EquivalenceClasses at every refinement pass) preserves the
		// relative order of its input, and every pass starts from names
		// sorted ascending, so each block is a sorted subsequence of that
		// order and its first element is its minimum.
		keep := block[0]
		for _, n := range block {
			rep[n] = keep
		}
	}

	names := make([]string, len(s.EnvOrder))
	copy(names, s.EnvOrder)
	sort.Strings(names)

	newEnv := make(map[string]*omnist.Record, len(rep))
	newOrder := make([]string, 0, len(rep))
	for _, name := range names {
		if rep[name] != name {
			continue
		}
		newEnv[name] = remapRecord(s.Env[name], rep)
		newOrder = append(newOrder, name)
	}
	return omnist.Schema{Root: rep[s.Root], Env: newEnv, EnvOrder: newOrder}
}

// remapRecord rewrites every reference field in rec to its representative
// per rep, implementing the remap(rec, rep) used by normalize's new_env
// construction.
func remapRecord(rec *omnist.Record, rep map[string]string) *omnist.Record {
	fields := make([]omnist.Field, len(rec.Fields))
	for i, f := range rec.Fields {
		if f.Type.Kind == omnist.TypeRefKind {
			f.Type = omnist.RefType(rep[f.Type.RefName])
		}
		fields[i] = f
	}
	return &omnist.Record{Name: rec.Name, Fields: fields}
}
