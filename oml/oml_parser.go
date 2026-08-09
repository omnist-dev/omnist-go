package oml

import (
	"fmt"

	omnist "github.com/omnist-dev/omnist-go"
)

// Read parses OML source text into an omnist.Document (spec ch.4, "text to
// omnist.Document, stage 1"). limits configures the safety limits enforced while
// reading (integer digit count at tokenize time, nesting depth at parse
// time), per the design decision that omnist.Limits is always caller-configurable
// rather than hardcoded (docs/workflow-playbook.md §2.4, limits.go).
//
// On any parse failure the returned error is a *omnist.ParseError whose omnist.Path is a
// text-position path ("line:col", 1-based) per spec §8.4 — no omnist.Document
// path is possible, since a parse.* failure occurs before an omnist.Document
// exists.
func Read(text string, limits omnist.Limits) (omnist.Document, error) {
	checker := omnist.NewLimitChecker(limits)
	p := &parser{lex: newLexer(text, checker), checker: checker}
	if err := p.advance(); err != nil {
		return omnist.Document{}, err
	}
	doc, err := p.parseDocument()
	if err != nil {
		return omnist.Document{}, err
	}
	return doc, nil
}

type parser struct {
	lex     *lexer
	checker *omnist.LimitChecker
	cur     token
	// path is the omnist.Document path (spec §8.4) of the value about to be
	// parsed — the root path "$" at the top level, descending by label as
	// parseEdge recurses into nested `{ }` nodes. See parseEdge's
	// childPath comment for the one caveat (repeated-label indices).
	path omnist.Path
}

func (p *parser) advance() *omnist.ParseError {
	tok, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
}

func (p *parser) errAt(t token, code omnist.Code, msg string) *omnist.ParseError {
	return &omnist.ParseError{Line: t.line, Col: t.col, Path: fmt.Sprintf("%d:%d", t.line, t.col), Code: code, Message: msg}
}

// isReservedWord reports whether text is one of the three parser-level
// reserved words excluded from bare-label position (spec §4.4). nan/inf
// are excluded earlier, by the tokenizer (§4.2.2), so they never reach
// this check as an IDENT in the first place.
func isReservedWord(text string) bool {
	return text == "null" || text == "true" || text == "false"
}

// startsLabelColon reports whether the current token could begin a label
// immediately followed by ':' — i.e. is a STRING, or an IDENT that is not
// null/true/false. It does not itself check the following token; callers
// combine it with a one-token lookahead per §4.6.1.
func (p *parser) startsLabelColon() bool {
	switch p.cur.kind {
	case tokString:
		return true
	case tokIdent:
		return !isReservedWord(p.cur.text)
	default:
		return false
	}
}

// parseDocument implements §4.6/§4.6.1: [SEP] (node-edges / scalar) [SEP],
// with the top-level lookahead disambiguation. Leading/trailing SEP is
// invisible here already (the lexer never emits separator tokens; it only
// tracks sepBefore on the following real token), so this reduces to:
// decide the shape from the current token plus one token of lookahead,
// parse it, then require EOF.
func (p *parser) parseDocument() (omnist.Document, error) {
	if p.cur.kind == tokEOF {
		return omnist.NodeDocument(omnist.NewNode()), nil
	}

	if p.looksLikeEdgeStart() {
		// The implicit top-level node (no surrounding '{'/'}') is still a
		// node for depth/node-count purposes -- spec Sec2.4's depth is
		// "counted from the Document root", so the root itself is level 1,
		// not level 0. parseBracedNode's EnterNode/LeaveNode pair (for
		// every explicit '{...}') mirrors this same accounting; without an
		// EnterNode call here, the root was silently uncounted, undercounting
		// both depth and node count by exactly one relative to every nested
		// node inside it -- found via a conformance-vector correction that
		// exposed the off-by-one directly (document-model/limits/
		// depth-one-past-declared-limit-fails and node-count-one-past-
		// declared-limit-fails both expected a document.limit.* rejection
		// this repo was not producing).
		diag := p.checker.EnterNode("$")
		if diag != nil {
			return omnist.Document{}, &omnist.ParseError{Line: p.cur.line, Col: p.cur.col, Path: "$", Code: diag.Code, Message: diag.Message}
		}
		defer p.checker.LeaveNode()

		// parseNodeEdges(tokEOF) only returns successfully once p.cur is
		// tokEOF (that is its loop's own exit condition), so there is no
		// separate trailing-content check to make here.
		node, err := p.parseNodeEdges(tokEOF)
		if err != nil {
			return omnist.Document{}, err
		}
		return omnist.NodeDocument(node), nil
	}

	startTok := p.cur
	val, err := p.parseScalarValue()
	if err != nil {
		return omnist.Document{}, err
	}
	if p.cur.kind != tokEOF {
		// A leftover ':' is only "trailing content" when the scalar just
		// consumed came from a reserved-word IDENT (null/true/false) that
		// failed the §4.6.1 label lookahead — spec §4.6.1's own worked
		// example ("null: 1 ... fails on the leftover ':' as trailing
		// content") pins that case explicitly. Any other leftover ':' (for
		// instance after a bare NUMBER like "nan") was never a candidate
		// for label position at all, so it is simply an out-of-place
		// token, not a continuation of an almost-valid construct —
		// reported as parse.unexpected-token instead, per
		// oml-grammar/reserved/nan-bare-is-a-number-token-not-a-label.
		if p.cur.kind == tokColon && startTok.kind != tokIdent {
			return omnist.Document{}, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "unexpected ':' after a value")
		}
		return omnist.Document{}, p.errAt(p.cur, omnist.CodeParseTrailingContent, "content remains after the document")
	}
	return omnist.ValueDocument(val), nil
}

// looksLikeEdgeStart implements the §4.6.1 one-token lookahead: STRING or
// non-reserved IDENT, followed by ':'. It must peek one token ahead
// without disturbing p.cur for the caller that takes the scalar branch,
// so it snapshots the lexer position/state and restores it when the
// scalar branch is chosen — the lexer itself has no other mutable state
// besides position/line/col, and the omnist.LimitChecker isn't touched by
// tokenizing a single lookahead token except for CheckIntDigits, which is
// a pure validation (no counters mutated), so re-scanning the same token
// on the scalar branch is safe and side-effect free.
//
// If the lookahead token itself turns out to be malformed, this reports
// "not an edge start" rather than an error: normal parsing will retokenize
// the same position immediately afterward (on the scalar branch) and
// surface that same lex error there, which is where a caller expects a
// *omnist.ParseError from, not from a lookahead helper.
func (p *parser) looksLikeEdgeStart() bool {
	if !p.startsLabelColon() {
		return false
	}
	savedLex := *p.lex
	next, err := p.lex.next()
	*p.lex = savedLex
	if err != nil {
		return false
	}
	return next.kind == tokColon
}

// parseNodeEdges parses zero or more edges (spec node-edges = [ edge
// *( SEP edge ) ]) until closing is seen (tokRBrace or tokEOF). It does
// not consume the closing token.
func (p *parser) parseNodeEdges(closing tokenKind) (*omnist.Node, error) {
	node := omnist.NewNode()
	first := true
	for p.cur.kind != closing {
		if p.cur.kind == tokEOF && closing != tokEOF {
			return nil, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "unexpected end of input, expected '}'")
		}
		if !first && !p.cur.sepBefore {
			// At the top level (closing == tokEOF), whether this is
			// "trailing content" or "a missing separator between edges"
			// depends on whether what follows still looks like an edge
			// attempt (§4.6.1's own STRING/IDENT-plus-':' lookahead,
			// applied here the same way it disambiguates the top level):
			//   - "a: 1 b: 2" — "b" is followed by ':', so this reads as
			//     a second edge with a missing separator before it
			//     (parse.unexpected-token).
			//   - "a: 2024-01-01T99" — "T99" is not followed by ':', so
			//     it never looks like another edge attempt; per spec
			//     §4.8's worked example this is trailing content instead
			//     (parse.trailing-content), the same reading as a bare
			//     scalar document's own leftover-content check above.
			// Inside a brace-delimited node more structure is always
			// still expected before '}', so the missing-separator reading
			// applies unconditionally there.
			if closing == tokEOF && !p.looksLikeEdgeStart() {
				return nil, p.errAt(p.cur, omnist.CodeParseTrailingContent, "content remains after the document")
			}
			return nil, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected a separator (newline or ';') between edges")
		}
		edges, err := p.parseEdge()
		if err != nil {
			return nil, err
		}
		node.Edges = append(node.Edges, edges...)
		first = false
	}
	return node, nil
}

// parseEdge parses one `label ':' [SEP] value` production. Per §4.3.1,
// when value is an array, this expands to multiple edges sharing label;
// otherwise it is exactly one edge.
func (p *parser) parseEdge() ([]omnist.Edge, error) {
	label, err := p.parseLabel()
	if err != nil {
		return nil, err
	}
	if p.cur.kind != tokColon {
		return nil, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected ':' after label")
	}
	// childPath is this edge's omnist.Document path (spec §8.4), best-effort:
	// repeated-label occurrence indices aren't tracked at parse time (that
	// needs the finished omnist.Node, per path.go's PathIndexInNode), so a
	// repeated label always renders unindexed here. The one diagnostic
	// this currently feeds, document.limit.int-digits, only has a
	// conformance vector for a singly-occurring label, so this
	// simplification is not currently vector-visible.
	childPath := p.path.Child(label, 0, false)
	p.lex.valuePath = childPath.String()
	if err := p.advance(); err != nil {
		p.lex.valuePath = ""
		return nil, err
	}
	p.lex.valuePath = ""

	if p.cur.kind == tokLBracket {
		targets, err := p.parseArray()
		if err != nil {
			return nil, err
		}
		edges := make([]omnist.Edge, len(targets))
		for i, t := range targets {
			edges[i] = omnist.Edge{Label: label, Target: t}
		}
		return edges, nil
	}

	savedPath := p.path
	p.path = childPath
	target, err := p.parseValueTarget()
	p.path = savedPath
	if err != nil {
		return nil, err
	}
	return []omnist.Edge{{Label: label, Target: target}}, nil
}

// parseLabel implements spec §4.4: label = STRING / bare-label, with a
// bare label rejected when its text is null/true/false.
func (p *parser) parseLabel() (string, error) {
	switch p.cur.kind {
	case tokString:
		s := p.cur.strVal
		if err := p.advance(); err != nil {
			return "", err
		}
		return s, nil
	case tokIdent:
		if isReservedWord(p.cur.text) {
			return "", p.errAt(p.cur, omnist.CodeParseReservedWordLabel, fmt.Sprintf("%q used as a bare label", p.cur.text))
		}
		s := p.cur.text
		if err := p.advance(); err != nil {
			return "", err
		}
		return s, nil
	default:
		return "", p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected a label")
	}
}

// parseValueTarget parses spec `value` minus the array alternative. Every
// caller has already excluded tokLBracket before calling this: parseEdge
// routes '[' to parseArray directly (arrays expand to multiple edges, not
// one omnist.Target), and parseArray's own element loop checks for a nested '['
// and reports parse.nested-array before ever calling this — so there is
// no array-start case left to handle here.
func (p *parser) parseValueTarget() (omnist.Target, error) {
	if p.cur.kind == tokLBrace {
		node, err := p.parseBracedNode()
		if err != nil {
			return omnist.Target{}, err
		}
		return omnist.NodeTarget(node), nil
	}
	v, err := p.parseScalarValue()
	if err != nil {
		return omnist.Target{}, err
	}
	return omnist.ValueTarget(v), nil
}

// parseBracedNode parses `'{' [SEP] node-edges [SEP] '}'`, enforcing the
// parse-time nesting-depth limit via the shared omnist.LimitChecker (spec §4.7;
// do not duplicate limits.go's logic here).
//
// Per §4.6.1's wording ("this lookahead fires ... at the start of each
// value inside { }"): inside braces the content is unambiguously
// node-edges — the grammar's `value = scalar | "{" node-edges "}" |
// array` alternative is already selected by the '{' punctuation itself,
// so there is no scalar-vs-edge-list choice left to disambiguate here.
// We read that clause as referring to the same "does this position begin
// an edge, or is the list empty" decision node-edges always makes (via
// startsLabelColon-shaped structure, degenerately: RBrace vs edge start),
// not a second invocation of the top-level scalar/edge-list lookahead.
// This is a narrow reading of loosely worded prose, not a case affecting
// any worked example in §4.8, and is called out here per the issue's
// instruction to flag such readings in code.
func (p *parser) parseBracedNode() (*omnist.Node, error) {
	openTok := p.cur
	// document.limit.depth/document.limit.nodes are document.* codes, so
	// per spec §8.4 their path MUST be an omnist.Document path, never a
	// text-position one — "$" (the whole-document fallback) is used here
	// rather than a computed line:col, since this repo has no general
	// omnist.Document-path tracking through arbitrarily nested braces/arrays at
	// parse time to name a more specific location.
	diag := p.checker.EnterNode("$")
	if diag != nil {
		return nil, &omnist.ParseError{Line: openTok.line, Col: openTok.col, Path: "$", Code: diag.Code, Message: diag.Message}
	}
	defer p.checker.LeaveNode()

	if err := p.advance(); err != nil { // consume '{'
		return nil, err
	}
	// parseNodeEdges(tokRBrace) only returns successfully once p.cur is
	// tokRBrace (that is its loop's own exit condition; the tokEOF case
	// inside it is reported as an error there instead), so there is no
	// separate "expected '}'" check to make here.
	node, err := p.parseNodeEdges(tokRBrace)
	if err != nil {
		return nil, err
	}
	if err := p.advance(); err != nil { // consume '}'
		return nil, err
	}
	return node, nil
}

// parseScalarValue parses spec `scalar` (STRING / DATETIME / DATE / TIME /
// NUMBER / INTEGER / null / true / false). A bare IDENT that is not one
// of the three reserved words is not a scalar (§4.3: "OML has no implicit
// string-from-identifier coercion anywhere").
func (p *parser) parseScalarValue() (omnist.Value, error) {
	t := p.cur
	switch t.kind {
	case tokString:
		if err := p.advance(); err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewStringScalar(t.strVal)), nil
	case tokInteger:
		if err := p.advance(); err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewIntegerScalar(t.intVal)), nil
	case tokNumber:
		if err := p.advance(); err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewNumberScalar(t.numVal)), nil
	case tokDate:
		if err := p.advance(); err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewDateScalar(t.dateVal)), nil
	case tokTime:
		if err := p.advance(); err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewTimeScalar(t.timeVal)), nil
	case tokDateTime:
		if err := p.advance(); err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewDateTimeScalar(t.dateTimeVal)), nil
	case tokIdent:
		switch t.text {
		case "null":
			if err := p.advance(); err != nil {
				return omnist.Value{}, err
			}
			return omnist.NullValue(), nil
		case "true":
			if err := p.advance(); err != nil {
				return omnist.Value{}, err
			}
			return omnist.ScalarValue(omnist.NewBooleanScalar(true)), nil
		case "false":
			if err := p.advance(); err != nil {
				return omnist.Value{}, err
			}
			return omnist.ScalarValue(omnist.NewBooleanScalar(false)), nil
		default:
			return omnist.Value{}, p.errAt(t, omnist.CodeParseBareWord, fmt.Sprintf("bare word %q is not a valid value", t.text))
		}
	case tokLBracket:
		return omnist.Value{}, p.errAt(t, omnist.CodeParseUnexpectedToken, "array is not valid here")
	default:
		return omnist.Value{}, p.errAt(t, omnist.CodeParseUnexpectedToken, "expected a value")
	}
}

// parseArray implements spec §4.3.1. Arrays are parse-time sugar: this
// returns the element Targets to be expanded into repeated edges by the
// caller (parseEdge), since an array is not itself an omnist.Document-model
// construct.
//
// SPEC NOTE (narrow, resolved per prose): grammars/oml.abnf's `array`
// production includes `[SEP]` around elements, which — since SEP's own
// definition includes newline and ';' — would literally permit a newline
// or ';' inside `[...]`. Prose §4.3.1 is explicit and unambiguous that
// this is an error ("Comma is the only element separator. A newline or ';'
// inside [...] is an error"), and only the prose defines the
// parse.separator-in-array error code this behavior requires. Per
// grammars/oml.abnf's own header, "where [ABNF and prose] disagree, that
// is a defect to be fixed, not a choice" — this repo cannot fix
// omnist-spec, so this is implemented per prose (hspace/comments are
// still skipped silently inside arrays; a newline or ';' is rejected) and
// is flagged prominently in the issue report as a genuine ABNF/prose
// disagreement, not silently resolved.
func (p *parser) parseArray() ([]omnist.Target, error) {
	openTok := p.cur
	if err := p.advance(); err != nil { // consume '['
		return nil, err
	}
	if err := p.rejectArraySep(); err != nil {
		return nil, err
	}

	if p.cur.kind == tokRBracket {
		return nil, p.errAt(openTok, omnist.CodeParseEmptyArray, "'[]' is not a valid value")
	}

	var targets []omnist.Target
	for {
		if p.cur.kind == tokLBracket {
			return nil, p.errAt(p.cur, omnist.CodeParseNestedArray, "array element must not itself be an array")
		}
		target, err := p.parseValueTarget()
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)

		if err := p.rejectArraySep(); err != nil {
			return nil, err
		}

		switch p.cur.kind {
		case tokComma:
			if err := p.advance(); err != nil {
				return nil, err
			}
			if err := p.rejectArraySep(); err != nil {
				return nil, err
			}
			if p.cur.kind == tokRBracket {
				// Trailing comma before ']' is legal.
				if err := p.advance(); err != nil {
					return nil, err
				}
				return targets, nil
			}
		case tokRBracket:
			if err := p.advance(); err != nil {
				return nil, err
			}
			return targets, nil
		default:
			return nil, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected ',' or ']' in array")
		}
	}
}

// rejectArraySep checks whether the current token is separator-preceded
// by a newline or ';' — illegal inside '[...]' per §4.3.1 — and returns
// the parse.separator-in-array error if so.
func (p *parser) rejectArraySep() error {
	if p.cur.sepBefore {
		return &omnist.ParseError{
			Line: p.cur.sepLine, Col: p.cur.sepCol,
			Path:    fmt.Sprintf("%d:%d", p.cur.sepLine, p.cur.sepCol),
			Code:    omnist.CodeParseSeparatorInArray,
			Message: "newline or ';' is not a valid array separator",
		}
	}
	return nil
}
