package algebra

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/omnist-dev/omnist-go"
)

// This file implements §6.10's infer(samples) (port-order step 10): drafting
// a Schema from sample Documents. Unlike every other operation in chapter 6,
// infer is explicitly NOT part of the decidability story -- it is a
// convenience for getting started, and its output is expected to be
// hand-edited afterward. It depends on Document/Node/Edge/Target/Scalar
// (issue #1) as its input type and Schema/Record/Field/Type/Cardinality/
// EnvOrder (issue #5) as its output type. It does NOT depend on any of the
// schema-to-schema algebra functions (Prune, CompatibleWith, Normalize,
// Extract, Lint) -- infer is a separate, independent traversal over
// Documents.

// AnyFallback records one field infer_with_report opened to `any` under
// allow_any=true: a "RecordName.label" Location and a Reason drawn from one
// of the two exact phrasings §6.10 specifies ("mixes objects and values", or
// "values of more than one scalar kind (k1, k2, ...)" with kind names sorted
// and comma-joined). allow_any=false never produces any AnyFallback values;
// the corresponding condition is a hard failure instead.
type AnyFallback struct {
	Location string
	Reason   string
}

// InferWithReport implements §6.10's infer_with_report(samples, root_name,
// allow_any) exactly: drafts a Schema from samples, returning both the
// Schema and the list of AnyFallback openings (empty, never nil, on
// success when allow_any opened nothing).
//
// rootName is the pseudocode's defaultable root_name argument; passing ""
// selects the spec's default, "Root".
//
// The returned Schema is NOT normalized and MAY contain structurally
// duplicate records -- §6.10 requires this, since infer's whole value is a
// one-to-one, hand-editable correspondence between sample labels and
// generated record names. A caller wanting canonical form calls Normalize
// explicitly.
//
// Errors: zero samples is CodeAlgebraInferNoSamples; a scalar-rooted sample
// is CodeAlgebraInferScalarRoot; a label disagreeing on scalar kind (other
// than the sanctioned integer/number collapse) with allow_any=false is
// CodeAlgebraInferConflictingScalars; a label mixing nodes and scalars
// with allow_any=false is CodeAlgebraInferMixedShape (see errors.go for why
// that fourth code exists despite not appearing in the spec's taxonomy
// table).
func InferWithReport(samples []omnist.Document, rootName string, allowAny bool) (omnist.Schema, []AnyFallback, error) {
	if len(samples) == 0 {
		return omnist.Schema{}, nil, omnist.Diagnostic{
			// $ is the whole-schema fallback per spec §8.4: infer-no-samples
			// fails before any schema exists, so there is no more specific
			// Document/Schema path to name.
			Path:     "$",
			Code:     omnist.CodeAlgebraInferNoSamples,
			Message:  "cannot infer a schema from zero samples",
			Severity: omnist.SeverityError,
		}
	}
	if rootName == "" {
		rootName = "Root"
	}

	nodes := make([]*omnist.Node, len(samples))
	for i, s := range samples {
		if !s.IsNode {
			return omnist.Schema{}, nil, omnist.Diagnostic{
				// $ is the whole-schema fallback per spec §8.4: like
				// infer-no-samples above, this fails before any schema
				// exists (it's about the shape of the input samples, not a
				// schema), so "samples[N]" (a non-Document/Schema-shaped
				// path) is wrong regardless of which sample failed.
				Path:     "$",
				Code:     omnist.CodeAlgebraInferScalarRoot,
				Message:  "infer expects object (record) samples at the root",
				Severity: omnist.SeverityError,
			}
		}
		nodes[i] = s.Node
	}

	env := map[string]*omnist.Record{}
	envOrder := []string{}
	used := map[string]bool{}
	fallbacks := []AnyFallback{}

	if err := inferRecord(nodes, rootName, env, &envOrder, used, allowAny, &fallbacks); err != nil {
		return omnist.Schema{}, nil, err
	}

	return omnist.Schema{Root: rootName, Env: env, EnvOrder: envOrder}, fallbacks, nil
}

// Infer implements §6.10's infer(samples, root_name, allow_any): the plain
// convenience wrapper around InferWithReport that discards the fallback
// list. A caller who needs to know what allow_any opened MUST call
// InferWithReport directly.
func Infer(samples []omnist.Document, rootName string, allowAny bool) (omnist.Schema, error) {
	s, _, err := InferWithReport(samples, rootName, allowAny)
	return s, err
}

// inferRecord implements §6.10's infer_record(nodes, name, env, used,
// allow_any, fallbacks). It reserves name in used immediately (mirroring
// the pseudocode's "add name to used" as its first line), then runs the
// normative two-pass label collection: pass 1 walks every sample's edges to
// collect the set of labels seen anywhere, in first-seen order across
// samples; pass 2 walks every sample again, counting each pass-1 label's
// occurrences in that sample and defaulting to 0 if the label is absent.
// Two passes -- not one -- so a label missing from sample 1 but present in
// sample 5 comes out with the same [0,1] cardinality regardless of which
// sample the walk happens to reach first; a single combined pass would make
// the result depend on iteration order.
func inferRecord(
	nodes []*omnist.Node,
	name string,
	env map[string]*omnist.Record,
	envOrder *[]string,
	used map[string]bool,
	allowAny bool,
	fallbacks *[]AnyFallback,
) error {
	used[name] = true

	// Pass 1: every label seen in any sample, first-seen order.
	order := []string{}
	seenLabel := map[string]bool{}
	for _, node := range nodes {
		for _, e := range node.Edges {
			if !seenLabel[e.Label] {
				seenLabel[e.Label] = true
				order = append(order, e.Label)
			}
		}
	}

	children := make(map[string][]omnist.Target, len(order))
	perSampleCounts := make(map[string][]int, len(order))
	for _, label := range order {
		children[label] = []omnist.Target{}
		perSampleCounts[label] = []int{}
	}

	// Pass 2: per-sample counts, defaulting to 0.
	for _, node := range nodes {
		countsHere := map[string]int{}
		for _, e := range node.Edges {
			children[e.Label] = append(children[e.Label], e.Target)
			countsHere[e.Label]++
		}
		for _, label := range order {
			perSampleCounts[label] = append(perSampleCounts[label], countsHere[label])
		}
	}

	fields := make([]omnist.Field, 0, len(order))
	for _, label := range order {
		counts := perSampleCounts[label]
		maxCount := counts[0]
		minCount := counts[0]
		for _, c := range counts[1:] {
			if c > maxCount {
				maxCount = c
			}
			if c < minCount {
				minCount = c
			}
		}

		var card omnist.Cardinality
		if maxCount > 1 {
			// Array: permissive by design -- min is always 0, never the
			// observed minimum count. See the rule this file's package
			// comment quotes from §6.10.
			card = omnist.Cardinality{Min: 0, Unbounded: true}
		} else {
			card = omnist.Cardinality{Min: uint64(minCount), Max: 1}
		}

		typ, err := inferType(children[label], label, name, env, envOrder, used, allowAny, fallbacks)
		if err != nil {
			return err
		}
		fields = append(fields, omnist.Field{Label: label, Type: typ, Cardinality: card})
	}

	env[name] = &omnist.Record{Name: name, Fields: fields}
	*envOrder = append(*envOrder, name)
	return nil
}

// inferType implements §6.10's infer_type(child_values, label, record_name,
// env, used, allow_any, fallbacks). It branches on how many of the
// label's occurrences (across all samples) are nodes: all of them recurse
// into a nested record; none of them resolve a single scalar kind (with
// null tracked separately as nullability, and the one integer/number
// subtype collapse applied); some but not all is the mixed-shape case.
func inferType(
	targets []omnist.Target,
	label, recordName string,
	env map[string]*omnist.Record,
	envOrder *[]string,
	used map[string]bool,
	allowAny bool,
	fallbacks *[]AnyFallback,
) (omnist.Type, error) {
	nodeCount := 0
	for _, t := range targets {
		if t.IsNode() {
			nodeCount++
		}
	}

	switch {
	case nodeCount == len(targets):
		// All nodes: recurse into a nested named record.
		recName := uniqueNameFrom(label, used)
		childNodes := make([]*omnist.Node, len(targets))
		for i, t := range targets {
			n, _ := t.Node()
			childNodes[i] = n
		}
		if err := inferRecord(childNodes, recName, env, envOrder, used, allowAny, fallbacks); err != nil {
			return omnist.Type{}, err
		}
		return omnist.RefType(recName), nil

	case nodeCount > 0:
		// Some but not all nodes: mixes objects and values.
		if allowAny {
			*fallbacks = append(*fallbacks, AnyFallback{
				Location: recordName + "." + label,
				Reason:   "mixes objects and values",
			})
			return omnist.AnyType(), nil
		}
		return omnist.Type{}, omnist.Diagnostic{
			Path:     recordName + "." + label,
			Code:     omnist.CodeAlgebraInferMixedShape,
			Message:  fmt.Sprintf("label %q mixes objects and values; cannot infer one type", label),
			Severity: omnist.SeverityError,
		}

	default:
		// No nodes: every occurrence is a value (scalar or null).
		kindsSeen := map[omnist.ScalarKind]bool{}
		sawNull := false
		for _, t := range targets {
			v, _ := t.Value()
			if v.IsNull {
				sawNull = true
				continue
			}
			kindsSeen[v.Scalar.Kind] = true
		}
		// The one subtyping relation (§6.3): integer mixed with number
		// collapses to number.
		if kindsSeen[omnist.KindNumber] {
			delete(kindsSeen, omnist.KindInteger)
		}

		if len(kindsSeen) == 0 {
			// No non-null sample: default to a nullable string, per the
			// pseudocode's fallback.
			return omnist.ScalarType(omnist.KindString, sawNull), nil
		}

		if len(kindsSeen) > 1 {
			names := make([]string, 0, len(kindsSeen))
			for k := range kindsSeen {
				names = append(names, k.String())
			}
			sort.Strings(names)
			if allowAny {
				reason := fmt.Sprintf("values of more than one scalar kind (%s)", strings.Join(names, ", "))
				*fallbacks = append(*fallbacks, AnyFallback{
					Location: recordName + "." + label,
					Reason:   reason,
				})
				return omnist.AnyType(), nil
			}
			return omnist.Type{}, omnist.Diagnostic{
				Path:     recordName + "." + label,
				Code:     omnist.CodeAlgebraInferConflictingScalars,
				Message:  fmt.Sprintf("label %q has values of more than one scalar kind", label),
				Severity: omnist.SeverityError,
			}
		}

		var only omnist.ScalarKind
		for k := range kindsSeen {
			only = k
		}
		return omnist.ScalarType(only, sawNull), nil
	}
}

// uniqueNameFrom implements §6.10's unique_name_from(label, used): derives a
// generated record name from a label and disambiguates it against the
// already-used set. The spec leaves the exact naming/disambiguation scheme
// to the implementation, requiring only that it be deterministic given the
// same input samples (no randomness, no map-iteration-order dependence).
//
// This implementation: sanitize the label into an exported-style Go
// identifier (letters/digits/underscore only, non-identifier runes replaced
// with '_', a leading digit prefixed with '_', first letter uppercased,
// empty label falls back to "Field"), then, if that name is already used,
// append the smallest integer suffix (2, 3, ...) that is not. Both steps
// are pure functions of (label, used) with no map-iteration-order
// dependence: used is consulted only via direct key lookups, in a fixed
// increasing-suffix order.
func uniqueNameFrom(label string, used map[string]bool) string {
	base := sanitizeIdentifier(label)
	if base == "" {
		base = "omnist.Field"
	}
	if !used[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s%d", base, n)
		if !used[candidate] {
			return candidate
		}
	}
}

// sanitizeIdentifier turns an arbitrary label into a Go-identifier-shaped,
// capitalized name: non letter/digit/underscore runes become '_', a
// leading digit is prefixed with '_', and the first rune is upper-cased.
// An empty or all-non-identifier label produces "".
func sanitizeIdentifier(label string) string {
	var b strings.Builder
	for i, r := range label {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			if i == 0 && unicode.IsDigit(r) {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
