package omnist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
)

// ReadJSON parses JSON source text into a Document (spec
// docs/formats/json.md, "stage 1 only, no schema"). limits configures the
// safety limits enforced while reading (integer digit count, node/depth
// counts), via the same LimitChecker OML/OSD reading uses (limits.go) —
// per the issue's design-continuity note, this file does not reimplement
// that machinery.
//
// encoding/json's json.Decoder is used purely as a JSON tokenizer, read
// with Token() one token at a time via json.Decoder.UseNumber(). Two
// properties of that approach matter and are both deliberate:
//
//   - UseNumber() preserves each numeric literal's own text (as a
//     json.Number, effectively a string) instead of collapsing every
//     number to float64. Without it, `1` and `1.0` would both decode to
//     the Go float64 1.0 and the integer/number distinction spec
//     §2.2.1 requires (kind decided by the literal's shape: a decimal
//     point or exponent, or not — never by magnitude) would be lost.
//   - Token()-based streaming visits object keys in source order and
//     surfaces each one individually, unlike unmarshaling into
//     map[string]interface{} (which would both discard key order,
//     violating D-1, and collapse the integer/number distinction even
//     with UseNumber, since a map value slot has no memory of the
//     literal that filled it).
func ReadJSON(text string, limits Limits) (Document, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	r := &jsonReader{dec: dec, checker: NewLimitChecker(limits), text: text}

	tok, err := r.next()
	if err != nil {
		return Document{}, err
	}
	doc, err := r.readDocument(tok)
	if err != nil {
		return Document{}, err
	}

	// A single JSON document must not have trailing content after its one
	// top-level value. dec.Token() returns io.EOF once the stream (including
	// any trailing whitespace) is exhausted; anything else means more
	// tokens remain.
	if _, err := r.dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, r.errHere(CodeParseTrailingContent, "content remains after the document")
		}
		return Document{}, r.wrapDecodeErr(err)
	}
	return doc, nil
}

// jsonReader holds the streaming-decode state for one ReadJSON call.
type jsonReader struct {
	dec     *json.Decoder
	checker *LimitChecker
	text    string
}

// next reads the next raw JSON token, translating any decode error (bad
// syntax, unterminated string, unexpected EOF mid-value, etc.) into a
// *ParseError positioned via the decoder's byte offset.
func (r *jsonReader) next() (json.Token, error) {
	tok, err := r.dec.Token()
	if err != nil {
		return nil, r.wrapDecodeErr(err)
	}
	return tok, nil
}

// wrapDecodeErr converts an error from the underlying json.Decoder into a
// *ParseError. encoding/json does not expose its own error taxonomy in a
// way this package's Code values can select between meaningfully, so every
// low-level decode failure is reported as CodeParseUnexpectedToken with the
// decoder's own message — the position (derived from InputOffset) is the
// part worth preserving precisely.
func (r *jsonReader) wrapDecodeErr(err error) error {
	if errors.Is(err, io.EOF) {
		return r.errHere(CodeParseUnexpectedToken, "unexpected end of input")
	}
	return r.errHere(CodeParseUnexpectedToken, err.Error())
}

// errHere builds a *ParseError positioned at the decoder's current byte
// offset, translated to a 1-based line:col pair against the original text.
func (r *jsonReader) errHere(code Code, msg string) error {
	line, col := offsetToLineCol(r.text, r.dec.InputOffset())
	return &ParseError{Line: line, Col: col, Path: fmt.Sprintf("%d:%d", line, col), Code: code, Message: msg}
}

// offsetToLineCol converts a 0-based byte offset into a 1-based line:col
// pair, counting '\n' bytes. This is only used for error reporting, so
// exactness for exotic line-ending conventions is not load-bearing.
//
// offset always comes from json.Decoder.InputOffset(), which is bounded to
// [0, len(text)] by construction — there is no untrusted caller of this
// unexported helper — so, per the same no-dead-branch convention used
// elsewhere in this file, there is no defensive clamp here for a range
// this function's only caller can never produce.
func offsetToLineCol(text string, offset int64) (line, col int) {
	line, col = 1, 1
	for i := int64(0); i < offset; i++ {
		if text[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// readDocument builds the top-level Document from the already-read first
// token, per the model-mapping table: an object becomes a node document, a
// bare scalar becomes a value document. A bare top-level array is rejected
// (see readArrayElements's doc comment for why).
func (r *jsonReader) readDocument(tok json.Token) (Document, error) {
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			node, err := r.readObjectBody()
			if err != nil {
				return Document{}, err
			}
			return NodeDocument(node), nil
		case '[':
			return Document{}, r.errHere(CodeDocumentUnlabeledElement, "a top-level array has no label to attach to")
		}
	}
	v, err := r.scalarToValue(tok)
	if err != nil {
		return Document{}, err
	}
	return ValueDocument(v), nil
}

// readObjectBody reads object members up to (and including) the closing
// '}', with the opening '{' already consumed by the caller. Per the
// model-mapping table, each key becomes a label; a key whose value is a
// JSON array expands into one edge per array element sharing that label
// (the "repeated label" rule) rather than a single list-shaped edge, since
// the Document model has no list construct (issue #1's design).
//
// The caller is responsible for LimitChecker.EnterNode/LeaveNode around
// this call when the object being read is itself a nested target (a
// record inside another record, or an array element) — mirroring
// oml_parser.go's parseBracedNode convention. The top-level object is
// deliberately NOT wrapped: like OML's brace-less top level, the Document
// root does not count as one more level of nesting away from itself.
func (r *jsonReader) readObjectBody() (*Node, error) {
	node := NewNode()
	for r.dec.More() {
		keyTok, err := r.next()
		if err != nil {
			return nil, err
		}
		// JSON's own grammar requires an object member's key to be a quoted
		// string; json.Decoder enforces that itself before Token() ever
		// returns (an unquoted or otherwise malformed key, e.g. `{1:2}`,
		// fails at the Token() call above, wrapped by r.next()), so a
		// type-asserted key here is never anything but a string. Asserting
		// that directly, rather than carrying a second "not a string key"
		// error branch no input can reach, follows the same
		// no-dead-branch convention this file uses elsewhere (see
		// numberToValue's SetString comment).
		key := keyTok.(string)

		valTok, err := r.next()
		if err != nil {
			return nil, err
		}
		if err := r.readMember(node, key, valTok); err != nil {
			return nil, err
		}
	}
	// Consume the closing '}'.
	if _, err := r.next(); err != nil {
		return nil, err
	}
	return node, nil
}

// readMember appends the edge(s) for one object member (key already read,
// valTok is the value's first token) to node.
func (r *jsonReader) readMember(node *Node, key string, valTok json.Token) error {
	if delim, ok := valTok.(json.Delim); ok {
		switch delim {
		case '{':
			child, err := r.readNestedObject()
			if err != nil {
				return err
			}
			node.AddNode(key, child)
			return nil
		case '[':
			targets, err := r.readArrayElements()
			if err != nil {
				return err
			}
			for _, t := range targets {
				node.Edges = append(node.Edges, Edge{Label: key, Target: t})
			}
			return nil
		}
	}
	v, err := r.scalarToValue(valTok)
	if err != nil {
		return err
	}
	node.AddValue(key, v)
	return nil
}

// readNestedObject reads a '{'-started object that is a target somewhere
// beneath the root (an object member's value, or an array element),
// enforcing the depth/node-count limits via the shared LimitChecker
// around the read, per limits.go's EnterNode/LeaveNode contract.
func (r *jsonReader) readNestedObject() (*Node, error) {
	line, col := offsetToLineCol(r.text, r.dec.InputOffset())
	path := fmt.Sprintf("%d:%d", line, col)
	if diag := r.checker.EnterNode(path); diag != nil {
		return nil, &ParseError{Line: line, Col: col, Path: path, Code: diag.Code, Message: diag.Message}
	}
	defer r.checker.LeaveNode()
	return r.readObjectBody()
}

// readArrayElements reads array elements up to (and including) the
// closing ']', with the opening '[' already consumed by the caller, and
// returns one Target per element.
//
// A JSON array is sugar for a repeated label (spec docs/formats/json.md's
// model-mapping table): it is not itself a Document-model construct, so
// an array element that is itself an array has no label to attach to and
// is rejected — "Bare nested arrays are rejected... inner elements with no
// label and therefore no edge to occupy." The taxonomy has no
// JSON-specific code for this; CodeDocumentUnlabeledElement is the
// existing code (checked against errors.go, spec §8.3.2) whose own name
// and description ("unlabeled element") match this situation exactly, so
// it is reused here rather than minting a new one — the same reasoning
// readDocument uses for a bare top-level array, which is the same
// "array with nothing to hold it" situation one level up.
//
// An empty array ('[]') is treated the same way OML treats its identical
// array-as-repeated-label-sugar construct (oml_parser.go's parseArray):
// rejected with CodeParseEmptyArray, rather than silently producing zero
// edges for the label. This is a narrow, cosmetic reading rather than a
// load-bearing one — docs/formats/json.md's model-mapping table has no
// empty-array row to consult, but JSON's array sugar is explicitly the
// same mechanism OML's array sugar is ("the array is not a value in the
// model, it is the same label occurring more than once"), and OML's reader
// already treats zero repeats of that sugar as an error rather than a
// silent no-op; this reader follows that existing, in-repo precedent for
// the identical construct rather than inventing a second behavior for it.
func (r *jsonReader) readArrayElements() ([]Target, error) {
	if !r.dec.More() {
		// Consume the ']' before erroring so callers don't have to.
		errPos := r.errHere(CodeParseEmptyArray, "'[]' is not a valid value")
		if _, err := r.next(); err != nil {
			return nil, err
		}
		return nil, errPos
	}

	var targets []Target
	for r.dec.More() {
		tok, err := r.next()
		if err != nil {
			return nil, err
		}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{':
				child, err := r.readNestedObject()
				if err != nil {
					return nil, err
				}
				targets = append(targets, NodeTarget(child))
				continue
			case '[':
				return nil, r.errHere(CodeDocumentUnlabeledElement, "an array element must not itself be an array")
			}
		}
		v, err := r.scalarToValue(tok)
		if err != nil {
			return nil, err
		}
		targets = append(targets, ValueTarget(v))
	}
	// Consume the closing ']'.
	if _, err := r.next(); err != nil {
		return nil, err
	}
	return targets, nil
}

// scalarToValue converts one already-read non-Delim JSON token into a
// Document Value. Per the "no temporal types" rule (docs/formats/json.md),
// a date-looking string is never upgraded here — it stays KindString,
// since stage 1 never consults a schema and JSON strings carry no type tag
// of their own.
//
// Every caller has already excluded json.Delim before reaching here (the
// top-level, object-member, and array-element call sites each branch on
// json.Delim first and return/continue before falling through to this
// function), so the type switch's only remaining possibilities from
// json.Decoder.UseNumber()'s token set are nil, bool, string, and
// json.Number — an exhaustive set with no default case needed, matching
// the no-dead-branch convention oml_lexer.go's temporal decoders already
// use (see the comment above parseDateValue).
func (r *jsonReader) scalarToValue(tok json.Token) (Value, error) {
	switch v := tok.(type) {
	case nil:
		return NullValue(), nil
	case bool:
		return ScalarValue(NewBooleanScalar(v)), nil
	case string:
		return ScalarValue(NewStringScalar(v)), nil
	default:
		return r.numberToValue(v.(json.Number))
	}
}

// numberToValue implements the integer/number split by literal shape, per
// spec §2.2.1: a literal with no '.', 'e', or 'E' is an integer; any other
// numeric literal is a number. json.Number preserves the original literal
// text (that's the entire reason ReadJSON calls UseNumber()), so this
// inspects the text directly rather than the decoded magnitude.
func (r *jsonReader) numberToValue(n json.Number) (Value, error) {
	s := string(n)
	if strings.ContainsAny(s, ".eE") {
		f, err := n.Float64()
		if err != nil {
			return Value{}, r.errHere(CodeParseUnexpectedToken, "invalid number literal: "+s)
		}
		return ScalarValue(NewNumberScalar(f)), nil
	}

	digits := strings.TrimPrefix(s, "-")
	if diag := r.checker.CheckIntDigits(r.pathHere(), len(digits)); diag != nil {
		line, col := offsetToLineCol(r.text, r.dec.InputOffset())
		return Value{}, &ParseError{Line: line, Col: col, Path: diag.Path, Code: diag.Code, Message: diag.Message}
	}
	// s is a JSON-grammar-valid integer literal (the only kind
	// json.Decoder.UseNumber() ever hands back for a no-'.'/'e'/'E' token):
	// -?(0|[1-9]\d*). SetString with base 10 cannot fail on such a string,
	// so — mirroring oml_lexer.go's parseDateValue/parseTimeValue
	// convention for a regex-pinned precondition — this does not carry a
	// permanently-dead "malformed" branch for a case no input can reach.
	bi, _ := new(big.Int).SetString(s, 10)
	return ScalarValue(NewIntegerScalar(bi)), nil
}

// pathHere returns the "line:col" text-position path for the decoder's
// current byte offset, for use in Diagnostic.Path.
func (r *jsonReader) pathHere() string {
	line, col := offsetToLineCol(r.text, r.dec.InputOffset())
	return fmt.Sprintf("%d:%d", line, col)
}
