package yaml

import (
	"math"
	"math/bits"

	yamllib "gopkg.in/yaml.v3"

	omnist "github.com/omnist-dev/omnist-go"
)

// This file enforces spec D-18, D-19 and D-20 (§2.4.1): the alias expansion
// limit. It runs on the yaml.Node graph BEFORE the reader builds any
// omnist.Document, so an over-limit input is refused without ever paying for
// the expansion it describes (D-19).
//
// For every anchored node a the check computes, from the reference graph
// alone:
//
//	W(a) value slots materialized when a is expanded
//	S(a) value slots written in a's own definition
//	E(a) = W(a) / S(a)
//
// A container (mapping or sequence) and a scalar each count one slot; mapping
// keys are not value slots. Inside a definition:
//
//   - a plain alias `*b` counts ONE slot in S and contributes W(b) to W;
//   - a merge-key entry `<<: *b` counts ONE slot in S however many aliases it
//     holds, and each alias contributes W(b)-1 to W (b's container is
//     flattened into the referring mapping, not reproduced);
//   - a merge value that is a literal sequence `<<: [*p, *q]` is a syntactic
//     carrier: it adds no slot of its own to W or S;
//   - an INLINE mapping merged in place (`<<: {k: 1}`), which the spec text
//     does not cover, is treated like a merged alias of an anonymous anchor:
//     its container is flattened away (W contribution w-1) and the slots it
//     writes count in S (s-1, its container excluded). This keeps the bound
//     conservative-toward-1 rather than inventing amplification.
//
// W is the structural count of the spec, deliberately blind to key collisions
// (D-19): it may exceed what is finally materialized, never fall below it.
//
// The walk is an explicit-stack depth-first traversal (no recursion, so a
// deeply nested definition cannot exhaust the goroutine stack), each anchored
// node's (W, S) is memoized when its frame completes, and E is checked at that
// moment, so the first offending anchor stops the walk and no W held in
// memory exceeds max*S of the anchor in hand. All arithmetic saturates at
// math.MaxUint64 regardless, so an overflowing count can never wrap under the
// limit (D-19).
//
// A reference to an anchor whose definition has not finished (the alias sits
// inside the very definition it names, directly or through other anchors, merge
// keys included) is a cycle: W is unbounded and the input is rejected under
// the same code (D-20) without computing any E.

// aliasSlots is one anchored node's memoized W and S. done is false while the
// node's own definition is still being walked (a reference to it is a cycle).
type aliasSlots struct {
	w, s uint64
	done bool
}

// foldKind says how a completed child's (w, s) is added to its parent.
type foldKind int

const (
	// foldIgnore: a mapping key; keys are not value slots.
	foldIgnore foldKind = iota
	// foldPlain: a value slot; w and s are added in full.
	foldPlain
	// foldMerge: a merged mapping (alias or inline): w-1 is added to W, and
	// s-1 to S unless the child is an alias (an alias counts in S through the
	// single `<<` entry).
	foldMerge
	// foldCarrier: a literal merge sequence, whose w and s are already net of
	// its members' flattening and are added as they stand.
	foldCarrier
)

// aliasFrame is one node being walked.
type aliasFrame struct {
	n       *yamllib.Node
	cur     int // next index in n.Content to visit
	w, s    uint64
	carrier bool     // a literal sequence directly under a merge key
	fold    foldKind // how this node folds into its parent
}

func satAdd(a, b uint64) uint64 {
	if s := a + b; s >= a {
		return s
	}
	return math.MaxUint64
}

func satMul(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	if hi != 0 {
		return math.MaxUint64
	}
	return lo
}

// sub1 is x-1 for x >= 1 and 0 for 0 (a W is always at least 1 once computed;
// the guard keeps the helper total).
func sub1(x uint64) uint64 {
	if x == 0 {
		return 0
	}
	return x - 1
}

// aliasErr builds the D-18/D-20 rejection: code document.limit.alias-expansion
// at path "$" (a document.* code), positioned at n.
func aliasErr(n *yamllib.Node, msg string) *omnist.ParseError {
	return &omnist.ParseError{Line: n.Line, Col: n.Column, Path: "$", Code: omnist.CodeDocumentLimitAliasExpansion, Message: msg}
}

// checkAliasExpansion runs the D-18/D-19/D-20 check over the node graph rooted
// at root with the given maximum expansion factor maxE (already resolved to a
// positive value). It returns nil when every anchored definition is within the
// limit and no anchor refers to itself.
func checkAliasExpansion(root *yamllib.Node, maxE int) *omnist.ParseError {
	_, err := analyzeAliases(root, maxE)
	return err
}

// analyzeAliases is checkAliasExpansion plus the per-anchor (W, S) table it
// computed (complete only when the returned error is nil), which the tests use
// to pin the arithmetic.
func analyzeAliases(root *yamllib.Node, maxE int) (map[*yamllib.Node]aliasSlots, *omnist.ParseError) {
	memo := map[*yamllib.Node]*aliasSlots{}
	limit := uint64(maxE)
	var stack []aliasFrame

	push := func(n *yamllib.Node, fold foldKind, carrier bool) {
		f := aliasFrame{n: n, w: 1, s: 1, carrier: carrier, fold: fold}
		if carrier {
			f.w, f.s = 0, 0
		}
		if n.Anchor != "" {
			memo[n] = &aliasSlots{}
		}
		stack = append(stack, f)
	}
	// finish records a completed anchored node and checks its factor.
	finish := func(n *yamllib.Node, w, s uint64) *omnist.ParseError {
		if n.Anchor == "" {
			return nil
		}
		memo[n] = &aliasSlots{w: w, s: s, done: true}
		// A saturated W (true count at or past 2^64) is over the limit whatever
		// the limit is: max*S saturates too, and equal saturated values must
		// not compare as "within".
		if w == math.MaxUint64 || w > satMul(limit, s) {
			return aliasErr(n, "an anchored definition's alias expansion factor exceeds the configured maximum")
		}
		return nil
	}
	foldInto := func(p *aliasFrame, kind foldKind, w, s uint64, isAlias bool) {
		switch kind {
		case foldPlain:
			p.w, p.s = satAdd(p.w, w), satAdd(p.s, s)
		case foldMerge:
			p.w = satAdd(p.w, sub1(w))
			if !isAlias {
				p.s = satAdd(p.s, sub1(s))
			}
		case foldCarrier:
			p.w, p.s = satAdd(p.w, w), satAdd(p.s, s)
		}
	}

	push(root, foldPlain, false)
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.cur < len(f.n.Content) {
			i := f.cur
			f.cur++
			child := f.n.Content[i]
			kind := foldPlain
			carrier := false
			switch {
			case f.n.Kind == yamllib.SequenceNode:
				if f.carrier {
					kind = foldMerge
				}
			case i%2 == 0:
				kind = foldIgnore
				if isMergeKey(child) {
					// The `<<` entry is one written slot, whatever it holds.
					f.s = satAdd(f.s, 1)
				}
			case isMergeKey(f.n.Content[i-1]):
				kind = foldMerge
				if child.Kind == yamllib.SequenceNode && child.Anchor == "" {
					kind, carrier = foldCarrier, true
				}
			}
			switch child.Kind {
			case yamllib.AliasNode:
				t := memo[child.Alias]
				if t == nil || !t.done {
					return nil, aliasErr(child, "an anchor refers to itself, directly or through other anchors (unbounded expansion)")
				}
				foldInto(f, kind, t.w, 1, true)
			case yamllib.ScalarNode:
				if child.Anchor != "" {
					// W = S = 1, so E = 1 never exceeds a positive limit.
					memo[child] = &aliasSlots{w: 1, s: 1, done: true}
				}
				foldInto(f, kind, 1, 1, false)
			default:
				push(child, kind, carrier)
			}
			continue
		}
		done := *f
		stack = stack[:len(stack)-1]
		if err := finish(done.n, done.w, done.s); err != nil {
			return nil, err
		}
		if len(stack) > 0 {
			foldInto(&stack[len(stack)-1], done.fold, done.w, done.s, false)
		}
	}

	out := make(map[*yamllib.Node]aliasSlots, len(memo))
	for n, v := range memo {
		out[n] = *v
	}
	return out, nil
}
