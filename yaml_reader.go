package omnist

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ReadYAML parses YAML source text into a Document (spec
// docs/formats/yaml.md, "stage 1 only, no schema"). limits configures the
// safety limits enforced while reading, via the same LimitChecker every
// other codec uses (limits.go).
//
// # Library choice and empirical verification (issue #25)
//
// gopkg.in/yaml.v3 is used as the underlying parser, for its yaml.Node API:
// it hands back a raw parse tree (Kind/Tag/Style/Value per node) rather than
// only a fully-decoded Go value, which is exactly the "raw parse tree to
// build a custom resolution layer on top of" the issue asks for.
//
// The spec requires YAML 1.1 core-schema scalar resolution specifically —
// most notably the "Norway problem" (bare on/off/yes/no resolve to
// booleans) and the sexagesimal-integer sharp edge (bare 12:00:00 resolves
// to the integer 43200, not a time). Before writing anything below, this was
// checked empirically (not assumed from documentation) against yaml.v3's
// own resolve.go and confirmed with a standalone throwaway program that fed
// yaml.v3 a Node-tree decode of "on: true", "on" alone, "12:00:00", and a
// bare ISO date:
//
//   - A bare "on" parses to Tag="!!str" — v3 does NOT resolve it to a
//     boolean. v3's resolve.go contains this comment directly on the bool
//     lookup table: yes/no/on/off (and y/n) are simply absent from the
//     table v3 uses (only true/false/True/False/TRUE/FALSE are), and
//     resolve.go's comment on the sexagesimal path states outright: "Base
//     60 floats are a bad idea, were dropped in YAML 1.2, and are
//     purposefully unsupported here." So v3 implements YAML 1.2 resolution,
//     not 1.1, on exactly the two points this issue calls out.
//   - gopkg.in/yaml.v2's resolve.go DOES carry the full YAML 1.1 bool table
//     (y/Y/yes/Yes/YES/n/N/no/No/NO/on/On/ON/off/Off/OFF alongside
//     true/false), confirmed by reading its resolveMapList — but it has the
//     identical "purposefully unsupported" comment for sexagesimal, so v2
//     alone still doesn't clear the sexagesimal-integer requirement either.
//   - Neither published library, used via its own default decode path,
//     reaches full YAML-1.1-required behavior. Reconfiguring either one
//     to do so is not exposed as an option (the resolver tables in both
//     are unexported package internals, not something a caller can widen).
//
// Given that, this reader takes the issue's explicit fallback: parse with
// yaml.v3's Node tree (which is unaffected by resolve.go — Node.Value keeps
// the literal, un-interpreted source text of every scalar, and Node.Style
// reports whether it was quoted/block-style or bare), and layer this
// package's own YAML-1.1 core-schema resolution on top in
// resolveYAMLScalar below, rather than trusting either library's default
// interpreted value. v3's own Tag is still consulted as a hint for the
// parts of core-schema resolution where 1.1 and 1.2 do not actually
// disagree (plain decimal/hex/octal integers, ordinary floats, .inf/.nan,
// canonical true/false, null, and ISO timestamps all resolve identically
// under both — resolve.go's own "purposefully unsupported" comments name
// the Norway words and sexagesimal as the ONLY two divergences), but the
// Norway words and sexagesimal notation are always checked first, directly
// against the node's raw un-interpreted text, before v3's tag is
// consulted for anything — so v3's YAML-1.2 choices on exactly those two
// points are never allowed to leak through.
func ReadYAML(text string, limits Limits) (Document, error) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	var root yaml.Node
	if err := dec.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			return Document{}, &ParseError{Line: 1, Col: 1, Path: "1:1", Code: CodeParseUnexpectedToken, Message: "unexpected end of input"}
		}
		return Document{}, wrapYAMLDecodeErr(err)
	}

	// A YAML stream may contain multiple "---"-separated documents;
	// Decode only ever reads one. Trailing documents have nowhere to
	// attach in a single-Document result, so — mirroring ReadJSON's own
	// trailing-content check — a second Decode call succeeding means
	// there is more content this reader must refuse rather than silently
	// discard.
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, &ParseError{Line: extra.Line, Col: extra.Column, Path: fmt.Sprintf("%d:%d", extra.Line, extra.Column), Code: CodeParseTrailingContent, Message: "content remains after the document"}
		}
		return Document{}, wrapYAMLDecodeErr(err)
	}

	// root is always a DocumentNode wrapping exactly one child when Decode
	// succeeds with io.EOF not yet reached above — yaml.v3's contract for
	// decoding into a *yaml.Node.
	r := &yamlReader{checker: NewLimitChecker(limits)}
	return r.readDocument(root.Content[0])
}

// wrapYAMLDecodeErr converts a yaml.v3 decode error into a *ParseError.
// yaml.v3 does not expose a structured position for every syntax error (it
// reports "yaml: line N: ..." as plain text), so — like ReadJSON's own
// wrapDecodeErr for the same reason with encoding/json — this preserves the
// library's message and falls back to position 1:1, which is not exact for
// every syntax error but is what the library makes available.
func wrapYAMLDecodeErr(err error) error {
	return &ParseError{Line: 1, Col: 1, Path: "1:1", Code: CodeParseUnexpectedToken, Message: err.Error()}
}

// yamlReader holds the state for one ReadYAML call.
type yamlReader struct {
	checker *LimitChecker
}

// deref follows an AliasNode to the anchor it points at. Per
// docs/formats/yaml.md, "aliases resolve at parse time... shared identity
// is not preserved" — this function only locates the anchored node; it is
// the caller's job (readTarget/readScalar/readMapping, all of which build a
// brand-new Go value from what deref returns) that does the actual
// expansion into an independent copy. Nothing here keeps a shared pointer
// in the Document result: two edges built from the same alias each get
// their own freshly-allocated Node/Scalar, because the functions that
// consume deref's result never return the *yaml.Node itself as part of the
// Document — they always finish by allocating Document-model types.
func deref(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode {
		return n.Alias
	}
	return n
}

// readDocument builds the top-level Document, per the model-mapping table
// (docs/formats/yaml.md): a mapping becomes a node document, a bare scalar
// becomes a value document. A bare top-level sequence is rejected, for the
// same reason ReadJSON rejects a bare top-level array — see
// readSequenceElements's doc comment.
func (r *yamlReader) readDocument(n *yaml.Node) (Document, error) {
	n = deref(n)
	switch n.Kind {
	case yaml.MappingNode:
		node, err := r.readMappingBody(n)
		if err != nil {
			return Document{}, err
		}
		return NodeDocument(node), nil
	case yaml.SequenceNode:
		return Document{}, r.errAt(n, CodeDocumentUnlabeledElement, "a top-level sequence has no label to attach to")
	default:
		v, err := r.readScalar(n)
		if err != nil {
			return Document{}, err
		}
		return ValueDocument(v), nil
	}
}

// readMappingBody reads a MappingNode's Content (alternating key, value
// pairs) into a Node's edge list. Per the model-mapping table, a key whose
// value is a sequence expands into one edge per element sharing that label
// (the "repeated label" rule), exactly as JSON's array-valued members do —
// docs/formats/yaml.md states YAML and JSON are "the same codec" on this
// shape.
//
// The caller is responsible for LimitChecker.EnterNode/LeaveNode around
// this call when the mapping being read is itself a nested target — see
// readNestedMapping. The top level is deliberately not wrapped, matching
// ReadJSON's readObjectBody convention.
func (r *yamlReader) readMappingBody(n *yaml.Node) (*Node, error) {
	node := NewNode()
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode := deref(n.Content[i])
		label, err := r.readLabel(keyNode)
		if err != nil {
			return nil, err
		}
		if err := r.readMember(node, label, n.Content[i+1]); err != nil {
			return nil, err
		}
	}
	return node, nil
}

// readLabel resolves a mapping key node to the string label it must be.
// Per spec §2.2.2, a label MUST be a string. YAML's own core-schema
// resolution can hand a reader a key that is not textually a string at
// all — most notably the Norway problem (docs/formats/yaml.md: "a bare
// `on:` key parses to the boolean true, not the string \"on\""), but the
// same requirement applies to any other non-string resolution (a bare
// integer or null key, or a mapping/sequence used as a key) — so this
// checks the resolved kind generally, not only for booleans.
//
// Code choice: the spec taxonomy (errors.go, spec §8.3.2) has no
// dedicated "key must be a string" code. CodeDocumentUnlabeledElement is
// reused here, following the same reasoning ReadJSON's readArrayElements
// doc comment already applies to a different "no valid label" situation —
// its own name and description ("unlabeled element") match a mapping entry
// whose key did not resolve to a usable label. This is the plainly-correct
// reading of a narrow gap, not a load-bearing ambiguity: noted in the
// issue report rather than inventing a new taxonomy code without
// consultation.
func (r *yamlReader) readLabel(keyNode *yaml.Node) (string, error) {
	if keyNode.Kind != yaml.ScalarNode {
		return "", r.errAt(keyNode, CodeDocumentUnlabeledElement, "a mapping key must be a scalar string, not a nested collection")
	}
	v, err := r.readScalar(keyNode)
	if err != nil {
		return "", err
	}
	if v.IsNull {
		return "", r.errAt(keyNode, CodeDocumentUnlabeledElement, "a mapping key resolved to null, not a string")
	}
	if v.Scalar.Kind != KindString {
		return "", r.errAt(keyNode, CodeDocumentUnlabeledElement, fmt.Sprintf("a mapping key must be a string; %q resolved to %s (YAML's core-schema resolution, not a string) — quote the key to keep it a string", keyNode.Value, v.Scalar.Kind))
	}
	return v.Scalar.Str, nil
}

// readMember appends the edge(s) for one mapping entry (key already
// resolved to label) to node.
func (r *yamlReader) readMember(node *Node, label string, valNode *yaml.Node) error {
	valNode = deref(valNode)
	switch valNode.Kind {
	case yaml.MappingNode:
		child, err := r.readNestedMapping(valNode)
		if err != nil {
			return err
		}
		node.AddNode(label, child)
		return nil
	case yaml.SequenceNode:
		targets, err := r.readSequenceElements(valNode)
		if err != nil {
			return err
		}
		for _, t := range targets {
			node.Edges = append(node.Edges, Edge{Label: label, Target: t})
		}
		return nil
	default:
		v, err := r.readScalar(valNode)
		if err != nil {
			return err
		}
		node.AddValue(label, v)
		return nil
	}
}

// readNestedMapping reads a MappingNode that is a target somewhere beneath
// the root (a mapping value, or a sequence element), enforcing the
// depth/node-count limits via the shared LimitChecker, per limits.go's
// EnterNode/LeaveNode contract — mirroring ReadJSON's readNestedObject.
func (r *yamlReader) readNestedMapping(n *yaml.Node) (*Node, error) {
	path := fmt.Sprintf("%d:%d", n.Line, n.Column)
	if diag := r.checker.EnterNode(path); diag != nil {
		return nil, &ParseError{Line: n.Line, Col: n.Column, Path: path, Code: diag.Code, Message: diag.Message}
	}
	defer r.checker.LeaveNode()
	return r.readMappingBody(n)
}

// readSequenceElements reads a SequenceNode's Content into one Target per
// element. A YAML sequence is sugar for a repeated label (spec
// docs/formats/yaml.md's model-mapping table), exactly like a JSON array;
// it is not itself a Document-model construct. An element that is itself a
// sequence has no label to attach to and is rejected, for the identical
// reason ReadJSON's readArrayElements rejects a nested bare array — see
// that function's doc comment, which this mirrors rather than re-deriving.
//
// An empty sequence ('[]' or a key with no items) is rejected with
// CodeParseEmptyArray for the same reason ReadJSON and OML's parser treat
// an empty array as an error rather than silently producing zero edges —
// this is the same array-as-repeated-label-sugar mechanism in a third
// format, so it follows the existing in-repo precedent rather than
// inventing a fourth behavior for the identical construct. Narrow/cosmetic
// reading: docs/formats/yaml.md does not spell out the empty-sequence case
// explicitly.
func (r *yamlReader) readSequenceElements(n *yaml.Node) ([]Target, error) {
	if len(n.Content) == 0 {
		return nil, r.errAt(n, CodeParseEmptyArray, "an empty sequence is not a valid value")
	}
	targets := make([]Target, 0, len(n.Content))
	for _, elemNode := range n.Content {
		elemNode = deref(elemNode)
		switch elemNode.Kind {
		case yaml.MappingNode:
			child, err := r.readNestedMapping(elemNode)
			if err != nil {
				return nil, err
			}
			targets = append(targets, NodeTarget(child))
		case yaml.SequenceNode:
			return nil, r.errAt(elemNode, CodeDocumentUnlabeledElement, "a sequence element must not itself be a sequence")
		default:
			v, err := r.readScalar(elemNode)
			if err != nil {
				return nil, err
			}
			targets = append(targets, ValueTarget(v))
		}
	}
	return targets, nil
}

func (r *yamlReader) errAt(n *yaml.Node, code Code, msg string) *ParseError {
	return &ParseError{Line: n.Line, Col: n.Column, Path: fmt.Sprintf("%d:%d", n.Line, n.Column), Code: code, Message: msg}
}

// readScalar resolves one already-dereferenced ScalarNode to a Document
// Value, applying this reader's YAML-1.1 core-schema resolution
// (resolveYAMLScalar) and then the digit-count limit for any resulting
// integer. resolveYAMLScalar itself cannot fail — every plain scalar that
// doesn't resolve to a more specific kind falls back to KindString, so
// there is nothing here for it to report an error about — the only
// failure mode a scalar leaf can have is exceeding the digit limit below.
func (r *yamlReader) readScalar(n *yaml.Node) (Value, error) {
	v := resolveYAMLScalar(n)
	if !v.IsNull && v.Scalar.Kind == KindInteger {
		digits := len(strings.TrimPrefix(v.Scalar.Int.String(), "-"))
		if diag := r.checker.CheckIntDigits(fmt.Sprintf("%d:%d", n.Line, n.Column), digits); diag != nil {
			return Value{}, &ParseError{Line: n.Line, Col: n.Column, Path: diag.Path, Code: diag.Code, Message: diag.Message}
		}
	}
	return v, nil
}

// --- YAML 1.1 core-schema scalar resolution ---
//
// resolveYAMLScalar is this package's own resolution layer, built directly
// against each scalar node's raw, un-interpreted text (Node.Value) and its
// Style, per the empirical findings documented on ReadYAML: neither
// available library's default resolution matches spec-required YAML 1.1
// behavior on the Norway words or sexagesimal notation, so those two are
// always decided here, first, from raw text — never from yaml.v3's Tag.
//
// A non-plain scalar (single/double-quoted, or a block literal/folded
// style) is always a string: YAML's core schema only ever resolves *plain*
// (bare, unquoted) scalars to a non-string type — quoting is exactly the
// documented escape hatch (docs/formats/yaml.md: "Quoting the key (\"on\":)
// sidesteps the problem entirely").
var (
	// yamlBoolWords is YAML 1.1's bool type's exact word list
	// (http://yaml.org/type/bool.html), reproduced here rather than
	// sourced from either library (gopkg.in/yaml.v2 carries this same
	// table internally, but it is unexported). This is the full list,
	// including the single-letter y/n forms alongside the Norway
	// problem's on/off/yes/no and the case variants of true/false.
	yamlBoolWords = map[string]bool{
		"y": true, "Y": true, "yes": true, "Yes": true, "YES": true,
		"n": false, "N": false, "no": false, "No": false, "NO": false,
		"true": true, "True": true, "TRUE": true,
		"false": false, "False": false, "FALSE": false,
		"on": true, "On": true, "ON": true,
		"off": false, "Off": false, "OFF": false,
	}

	// yamlNullWords is YAML 1.1's null type's word list
	// (http://yaml.org/type/null.html); an empty plain scalar is also
	// null, handled separately below since a map key can't be "".
	yamlNullWords = map[string]bool{
		"~": true, "null": true, "Null": true, "NULL": true,
	}

	// reYAMLSexagesimal matches YAML 1.1's base-60 integer notation
	// (http://yaml.org/type/int.html): a leading sign, a first group of
	// one or more digits, then one or more ":"-separated groups of 1-2
	// digits. "12:00:00" is the spec's own named example (three groups:
	// 12, 0, 0 -> 12*3600 + 0*60 + 0 = 43200).
	reYAMLSexagesimal = regexp.MustCompile(`^[-+]?[0-9][0-9_]*(:[0-5]?[0-9])+$`)

	// reYAMLInt matches YAML 1.1's plain decimal/octal/hex/binary int
	// forms (http://yaml.org/type/int.html), underscores allowed as
	// digit-group separators.
	reYAMLInt = regexp.MustCompile(`^[-+]?(0b[0-1_]+|0x[0-9a-fA-F_]+|0o?[0-7_]+|0|[1-9][0-9_]*)$`)

	// reYAMLFloat matches YAML 1.1's canonical plain float form
	// (http://yaml.org/type/float.html): optional sign, digits with a
	// mandatory decimal point or an exponent (or both), underscores
	// allowed. Sexagesimal float notation exists in the 1.1 type spec
	// too, but the spec issue this reader implements only names the
	// sexagesimal *integer* case (12:00:00) as a required behavior, and
	// nothing in docs/formats/yaml.md's worked example exercises a
	// sexagesimal float — so it is deliberately not implemented here.
	// This is a narrow, cosmetic gap (an input like "1:30:00.5" would
	// fall through to reYAMLSexagesimal's non-match and then to the
	// plain-string fallback below, rather than becoming a KindNumber),
	// noted in the issue report rather than treated as load-bearing.
	reYAMLFloat = regexp.MustCompile(`^[-+]?(\.[0-9_]+|[0-9_]+(\.[0-9_]*)?)([eE][-+]?[0-9]+)?$`)

	// reYAMLDate matches a bare calendar date (docs/formats/yaml.md's
	// own worked example: "placed: 2024-01-01").
	reYAMLDate = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

	// reYAMLDateTime matches a bare ISO-8601-ish timestamp, per YAML
	// 1.1's timestamp type (http://yaml.org/type/timestamp.html):
	// 'T' or a literal space between date and time, and either a 'Z' or
	// a +HH:MM/-HH:MM offset (colon in the offset optional per the 1.1
	// grammar, though every worked example and test in this issue uses
	// the colon form).
	reYAMLDateTime = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})[Tt ]([0-9]{2}):([0-9]{2})(:([0-9]{2})(\.([0-9]+))?)?(Z|([+-][0-9]{2}):?([0-9]{2}))?$`)
)

// resolveYAMLScalar is the core-schema resolution entry point for one
// scalar node.
func resolveYAMLScalar(n *yaml.Node) Value {
	if n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return ScalarValue(NewStringScalar(n.Value))
	}

	s := n.Value

	// Empty plain scalar resolves to null (YAML 1.1's null type;
	// checked ahead of yamlNullWords since "" can't be a map key in
	// yamlNullWords the way non-empty words are).
	if s == "" {
		return NullValue()
	}
	if b, ok := yamlBoolWords[s]; ok {
		return ScalarValue(NewBooleanScalar(b))
	}
	if yamlNullWords[s] {
		return NullValue()
	}
	if reYAMLSexagesimal.MatchString(s) {
		return ScalarValue(NewIntegerScalar(parseSexagesimalInt(s)))
	}
	if reYAMLInt.MatchString(s) {
		bi, ok := parseYAMLInt(s)
		if ok {
			return ScalarValue(NewIntegerScalar(bi))
		}
		// Falls through to string on the chance SetString rejects text
		// reYAMLInt's own pattern already constrains to a valid big.Int
		// literal in every base it matches — see parseYAMLInt's doc
		// comment; TestYAMLIntLikePatternThatFailsToParseFallsBackToString
		// (yaml_reader_test.go) exercises this with "0x_".
	}
	switch s {
	case ".inf", "+.inf", ".Inf", "+.Inf", ".INF", "+.INF":
		return ScalarValue(NewNumberScalar(math.Inf(1)))
	case "-.inf", "-.Inf", "-.INF":
		return ScalarValue(NewNumberScalar(math.Inf(-1)))
	case ".nan", ".NaN", ".NAN":
		return ScalarValue(NewNumberScalar(math.NaN()))
	}
	if reYAMLFloat.MatchString(s) && strings.ContainsAny(s, ".eE") {
		f, err := strconv.ParseFloat(strings.ReplaceAll(s, "_", ""), 64)
		if err == nil {
			return ScalarValue(NewNumberScalar(f))
		}
	}
	if reYAMLDateTime.MatchString(s) {
		return ScalarValue(NewDateTimeScalar(parseYAMLDateTime(s)))
	}
	if reYAMLDate.MatchString(s) {
		return ScalarValue(NewDateScalar(parseDateValue(s)))
	}
	return ScalarValue(NewStringScalar(s))
}

// parseSexagesimalInt computes the big.Int value of a YAML base-60
// integer literal already confirmed to match reYAMLSexagesimal: a sign,
// then ":"-separated groups, most-significant group first, each
// contributing group*60^(remaining groups). "12:00:00" -> 12*3600 + 0*60 +
// 0 = 43200 (the spec's own named example). Arbitrary-precision big.Int
// arithmetic is used, matching every other integer path in this package
// (spec §2.4's digit-count limit applies to arbitrarily large integers).
func parseSexagesimalInt(s string) *big.Int {
	neg := false
	if s[0] == '+' || s[0] == '-' {
		neg = s[0] == '-'
		s = s[1:]
	}
	groups := strings.Split(s, ":")
	sixty := big.NewInt(60)
	result := new(big.Int)
	for _, g := range groups {
		g = strings.ReplaceAll(g, "_", "")
		part, _ := new(big.Int).SetString(g, 10)
		result.Mul(result, sixty)
		result.Add(result, part)
	}
	if neg {
		result.Neg(result)
	}
	return result
}

// parseYAMLInt parses a plain integer literal already confirmed to match
// reYAMLInt (decimal, or 0b/0x/0o-prefixed) into a big.Int. big.Int's
// SetString with base 0 already understands Go-style 0b/0x/0o/0 prefixes,
// which is exactly YAML 1.1's own int type's prefix set, so this defers to
// it directly rather than duplicating base detection; the leading '+' sign
// SetString does not accept is stripped first (SetString accepts '-' but
// not a redundant '+').
func parseYAMLInt(s string) (*big.Int, bool) {
	s = strings.TrimPrefix(s, "+")
	s = strings.ReplaceAll(s, "_", "")
	return new(big.Int).SetString(s, 0)
}

// parseYAMLDateTime converts a scalar's text into a DateTimeValue. Its
// only caller (resolveYAMLScalar) checks reYAMLDateTime.MatchString(s)
// first and only calls this on success, so FindStringSubmatch here is
// guaranteed non-nil — mirroring oml_lexer.go's parseDateValue/
// parseTimeValue precondition convention (see that file's comment above
// parseDateValue), this does not carry a permanently-dead "no match"
// branch for a case the precondition already excludes.
//
// Unlike OML's reDateTime (which oml_lexer.go's parseDateTimeValue
// assumes), YAML's timestamp type additionally allows a lowercase 't' or a
// literal space as the date/time separator and a bare 'Z' for UTC, so this
// does not reuse oml_lexer.go's parseDateTimeValue directly (its
// Sscanf-based implementation is pinned to the OML lexer's own narrower
// regex) and instead extracts fields from reYAMLDateTime's own capture
// groups.
func parseYAMLDateTime(s string) DateTimeValue {
	m := reYAMLDateTime.FindStringSubmatch(s)
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	hour, _ := strconv.Atoi(m[4])
	minute, _ := strconv.Atoi(m[5])

	tv := TimeValue{Hour: hour, Minute: minute}
	if m[6] != "" {
		sec, _ := strconv.Atoi(m[7])
		tv.Second = sec
		if m[8] != "" {
			tv.Nanosecond = fracToNanos(m[9])
		}
	}
	switch {
	case m[10] == "Z":
		tv.HasOffset = true
		tv.OffsetSeconds = 0
	case m[10] != "":
		oh, _ := strconv.Atoi(m[11])
		om, _ := strconv.Atoi(m[12])
		sign := 1
		if strings.HasPrefix(m[11], "-") {
			sign = -1
			oh = -oh
		}
		tv.HasOffset = true
		tv.OffsetSeconds = sign * (oh*3600 + om*60)
	}
	return DateTimeValue{
		Date: DateValue{Year: year, Month: month, Day: day},
		Time: tv,
	}
}

