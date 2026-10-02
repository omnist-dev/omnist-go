package yaml

import (
	"fmt"
	"math"
	"math/bits"

	yamllib "gopkg.in/yaml.v3"

	omnist "github.com/omnist-dev/omnist-go"
)

// This file enforces spec D-18, D-18a, D-19, D-20 and D-22 (§2.4.1): the alias
// expansion limit and the expanded-size cap. It runs on the yaml.Node graph
// BEFORE the reader builds any omnist.Document, so an over-limit input is
// refused without ever paying for the expansion it describes (D-19).
//
// Step 1, validateMergeShapes: every merge value must be a mapping or a
// sequence of mappings (after following an alias one level); an EMPTY sequence
// is a sequence of mappings vacuously, a well-formed carrier that merges
// nothing (D-18a, omnist-spec#127). Anything
// else is parse.codec-syntax, reported before anything is counted so it wins
// over every document.limit.* code (D-18a).
//
// Step 2, analyzeAliases: for every candidate node (every anchored node, and
// every mapping and sequence whether anchored or not, the document root and an
// inline merge source included; scalars are never checked, E = 1.00 trivially)
// it computes, from the reference graph alone:
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
//     holds, and an alias to a mapping contributes W(b)-1 to W (b's container
//     is flattened into the referring mapping, not reproduced);
//   - a sequence in merge-value position, `<<: [*p, *q]` or `<<: &s [*p, *q]`,
//     is a syntactic carrier whether or not it is anchored (D-18a): it adds no
//     slot of its own to W or S and is not a candidate. Each member
//     contributes as a merge source does (W-1);
//   - an EMPTY carrier, `<<: []` or `<<: &s []`, contributes 0 to W (no
//     members, no slot) and the `<<` entry is still one slot in S; `s: &s []`
//     then `<<: *s` is the same (the sum over zero members). An empty sequence
//     outside merge position is an ordinary node, W = S = 1;
//   - `<<: *s` where s is a sequence contributes the sum over s's members of
//     W(member)-1 (aliasSlots.mw), and no slot for s itself; a PLAIN alias to
//     the same sequence materializes the list, 1 + sum W(member);
//   - an INLINE mapping merged in place (`<<: {k: 1}`): the `<<` entry is the
//     one slot in the referrer's S, the inline mapping's own written values
//     add theirs (s-1, its container excluded), and its container is
//     flattened away in W (contribution w-1). The inline mapping is also a
//     candidate on its own subtree.
//
// W is the structural count of the spec, deliberately blind to key collisions
// (D-19): it may exceed what is finally materialized, never fall below it.
//
// Mapping keys are not value slots and are never materialized (the reader
// refuses a container key as document.unlabeled-element), so a container key
// is measured as a candidate on its own subtree but folds into nothing.
//
// The walk is an explicit-stack depth-first traversal (no recursion, so a
// deeply nested definition or a very long alias chain cannot exhaust the
// goroutine stack), each anchored node's slots are memoized when its frame
// completes, and E is checked at that moment for every container, so the first
// offender stops the walk and no W held in memory exceeds max*S of the anchor
// in hand. All arithmetic saturates at math.MaxUint64 regardless, so an
// overflowing count can never wrap under a limit (D-19).
//
// D-22: for an input that contains at least one alias or merge key, W(root)
// above the expanded-size maximum is rejected with document.limit.expanded-size
// at "$", after every candidate has passed D-18 (so D-18 is reported when both
// fail) and before anything is materialized. An input with neither is exempt.
//
// A reference to an anchor whose definition has not finished (the alias sits
// inside the very definition it names, directly or through other anchors, merge
// keys included) is a cycle: W is unbounded and the input is rejected under
// the alias-expansion code (D-20) without computing any E.

// aliasSlots is one anchored node's memoized slots. done is false while the
// node's own definition is still being walked (a reference to it is a cycle).
// For a sequence, w is the plain-reference value 1 + sum W(member) and mw is
// what a reference from merge-value position contributes, sum W(member)-1.
type aliasSlots struct {
	w, s  uint64
	mw    uint64
	isSeq bool
	done  bool
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
	// foldCarrier: a merge-position sequence (written in place, or an alias to
	// one), whose w and s are already net of its members' flattening and are
	// added as they stand.
	foldCarrier
)

// aliasFrame is one node being walked.
type aliasFrame struct {
	n       *yamllib.Node
	cur     int // next index in n.Content to visit
	w, s    uint64
	pw, mw  uint64   // sequences only: 1 + sum W(member), and sum W(member)-1
	carrier bool     // a sequence directly under a merge key
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

// sizeErr builds the D-22 rejection: code document.limit.expanded-size at "$".
func sizeErr(n *yamllib.Node, w, max uint64) *omnist.ParseError {
	msg := "the input expands to more value slots than the configured maximum"
	if w != math.MaxUint64 {
		msg = fmt.Sprintf("the input expands to %d value slots, over the configured maximum of %d", w, max)
	}
	return &omnist.ParseError{Line: n.Line, Col: n.Column, Path: "$", Code: omnist.CodeDocumentLimitExpandedSize, Message: msg}
}

// mergeShapeErr builds the D-18a rejection of a malformed merge: a syntax
// error positioned at the offending node as written.
func mergeShapeErr(n *yamllib.Node) *omnist.ParseError {
	return &omnist.ParseError{Line: n.Line, Col: n.Column, Path: fmt.Sprintf("%d:%d", n.Line, n.Column), Code: omnist.CodeParseCodecSyntax, Message: "YAML: a merge key's value must be a mapping or a sequence of mappings"}
}

// checkAliasExpansion runs merge-shape validation and then the D-18/D-19/D-20
// and D-22 check over the node graph rooted at root, with the given maximum
// expansion factor maxE and maximum expanded size maxSlots (both already
// resolved to positive values). It returns nil when the merges are well formed,
// every candidate is within the limit, no anchor refers to itself and the
// expanded size is within its cap.
func checkAliasExpansion(root *yamllib.Node, maxE, maxSlots int) *omnist.ParseError {
	if err := validateMergeShapes(root); err != nil {
		return err
	}
	_, err := analyzeAliases(root, maxE, maxSlots)
	return err
}

// validateMergeShapes reports the first malformed merge in document order: a
// merge value that is not a mapping or a (possibly empty) sequence of
// mappings, where an alias counts as the node it names (D-18a). A sequence
// inside a merge sequence, a scalar member, and an alias to a sequence of
// scalars are all malformed. The walk does not follow aliases (an aliased definition is
// reached where it is written), so it is linear and cannot loop on a cycle.
func validateMergeShapes(root *yamllib.Node) *omnist.ParseError {
	stack := []*yamllib.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.Kind == yamllib.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				if isMergeKey(n.Content[i]) {
					if err := mergeValueErr(n.Content[i+1]); err != nil {
						return err
					}
				}
			}
		}
		// Reverse order, so the first malformed merge in the document is the
		// one reported.
		for i := len(n.Content) - 1; i >= 0; i-- {
			stack = append(stack, n.Content[i])
		}
	}
	return nil
}

// mergeValueErr checks one merge value v, as written.
func mergeValueErr(v *yamllib.Node) *omnist.ParseError {
	t := deref(v)
	switch t.Kind {
	case yamllib.MappingNode:
		return nil
	case yamllib.SequenceNode:
		for _, m := range t.Content {
			if deref(m).Kind != yamllib.MappingNode {
				return mergeShapeErr(m)
			}
		}
		return nil
	default:
		return mergeShapeErr(v)
	}
}

// analyzeAliases is the counting pass plus the per-anchor slots table it
// computed (complete only when the returned error is nil), which the tests use
// to pin the arithmetic. The merges must already be validated.
func analyzeAliases(root *yamllib.Node, maxE, maxSlots int) (map[*yamllib.Node]aliasSlots, *omnist.ParseError) {
	memo := map[*yamllib.Node]*aliasSlots{}
	limit := uint64(maxE)
	var stack []aliasFrame
	var rootW uint64
	sawRef := false // an alias or a merge key occurs: D-22 applies

	push := func(n *yamllib.Node, fold foldKind, carrier bool) {
		f := aliasFrame{n: n, w: 1, s: 1, pw: 1, carrier: carrier, fold: fold}
		if carrier {
			f.w, f.s = 0, 0
		}
		if n.Anchor != "" {
			memo[n] = &aliasSlots{}
		}
		stack = append(stack, f)
	}
	// finish records a completed anchored node and checks the factor of every
	// completed candidate container, anchored or not.
	finish := func(f aliasFrame) *omnist.ParseError {
		n := f.n
		isSeq := n.Kind == yamllib.SequenceNode
		if n.Anchor != "" {
			sl := &aliasSlots{w: f.w, s: f.s, isSeq: isSeq, done: true}
			if isSeq {
				// A carrier's own w is net of flattening; a plain reference to
				// it still materializes the list.
				sl.w, sl.mw = f.pw, f.mw
			}
			memo[n] = sl
		}
		if f.carrier {
			// A merge-position sequence is a syntactic carrier, not a candidate.
			return nil
		}
		// A saturated W (true count at or past 2^64) is over the limit whatever
		// the limit is: max*S saturates too, and equal saturated values must
		// not compare as "within".
		if f.w == math.MaxUint64 || f.w > satMul(limit, f.s) {
			return aliasErr(n, "a mapping's or sequence's alias expansion factor exceeds the configured maximum")
		}
		return nil
	}
	foldInto := func(p *aliasFrame, kind foldKind, w, s uint64, isAlias bool) {
		if p.n.Kind == yamllib.SequenceNode {
			p.pw, p.mw = satAdd(p.pw, w), satAdd(p.mw, sub1(w))
		}
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
					sawRef = true
				}
			case isMergeKey(f.n.Content[i-1]):
				kind = foldMerge
				if child.Kind == yamllib.SequenceNode {
					kind, carrier = foldCarrier, true
				}
			}
			switch child.Kind {
			case yamllib.AliasNode:
				sawRef = true
				t := memo[child.Alias]
				if t == nil || !t.done {
					return nil, aliasErr(child, "an anchor refers to itself, directly or through other anchors (unbounded expansion)")
				}
				if kind == foldMerge && t.isSeq {
					// `<<: *s`: the members flattened, no slot for s (D-18a).
					foldInto(f, foldCarrier, t.mw, 0, true)
				} else {
					foldInto(f, kind, t.w, 1, true)
				}
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
		if err := finish(done); err != nil {
			return nil, err
		}
		if len(stack) > 0 {
			foldInto(&stack[len(stack)-1], done.fold, done.w, done.s, false)
		} else {
			rootW = done.w
		}
	}

	// D-22, after every candidate passed D-18: only an input with an alias or a
	// merge key is subject to the cap. A saturated W(root) cannot reach here
	// (the ratio check above rejects it), and would exceed every maximum.
	if sawRef && rootW > uint64(maxSlots) {
		return nil, sizeErr(root, rootW, uint64(maxSlots))
	}

	out := make(map[*yamllib.Node]aliasSlots, len(memo))
	for n, v := range memo {
		out[n] = *v
	}
	return out, nil
}
