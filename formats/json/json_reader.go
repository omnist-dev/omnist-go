package json

import (
	encjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// Read parses JSON source text into a omnist.Document (spec
// docs/formats/json.md, "stage 1 only, no schema"). limits configures the
// safety limits enforced while reading (integer digit count, node/depth
// counts), via the same omnist.LimitChecker OML/OSD reading uses (limits.go) —
// per the issue's design-continuity note, this file does not reimplement
// that machinery.
//
// encoding/json's encjson.Decoder is used purely as a JSON tokenizer, read
// with Token() one token at a time via encjson.Decoder.UseNumber(). Two
// properties of that approach matter and are both deliberate:
//
//   - UseNumber() preserves each numeric literal's own text (as a
//     encjson.Number, effectively a string) instead of collapsing every
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
func Read(text string, limits omnist.Limits) (omnist.Document, error) {
	dec := encjson.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	r := &jsonReader{dec: dec, checker: omnist.NewLimitChecker(limits), text: text}

	tok, err := r.next()
	if err != nil {
		return omnist.Document{}, err
	}
	doc, err := r.readDocument(tok)
	if err != nil {
		return omnist.Document{}, err
	}

	// A single JSON document must not have trailing content after its one
	// top-level value. dec.Token() returns io.EOF once the stream (including
	// any trailing whitespace) is exhausted; anything else means more
	// tokens remain.
	if _, err := r.dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return omnist.Document{}, r.errHere(omnist.CodeParseTrailingContent, "content remains after the document")
		}
		return omnist.Document{}, r.wrapDecodeErr(err)
	}
	return doc, nil
}

// jsonReader holds the streaming-decode state for one Read call.
type jsonReader struct {
	dec     *encjson.Decoder
	checker *omnist.LimitChecker
	text    string
	// path is the omnist.Document path (spec §8.4) of the value currently being
	// read — the root path "$" at the top level, descending by label as
	// readMember/readArrayElements recurse into nested objects/arrays.
	path omnist.Path
}

// next reads the next raw JSON token, translating any decode error (bad
// syntax, unterminated string, unexpected EOF mid-value, etc.) into a
// *omnist.ParseError positioned via the decoder's byte offset.
func (r *jsonReader) next() (encjson.Token, error) {
	tok, err := r.dec.Token()
	if err != nil {
		return nil, r.wrapDecodeErr(err)
	}
	return tok, nil
}

// wrapDecodeErr converts an error from the underlying encjson.Decoder into a
// *omnist.ParseError. encoding/json does not expose its own error taxonomy in a
// way this package's omnist.Code values can select between meaningfully, so every
// low-level decode failure is reported as omnist.CodeParseUnexpectedToken with the
// decoder's own message — the position (derived from InputOffset) is the
// part worth preserving precisely.
func (r *jsonReader) wrapDecodeErr(err error) error {
	if errors.Is(err, io.EOF) {
		return r.errHere(omnist.CodeParseUnexpectedToken, "unexpected end of input")
	}
	return r.errHere(omnist.CodeParseUnexpectedToken, err.Error())
}

// errHere builds a *omnist.ParseError positioned at the decoder's current byte
// offset, translated to a 1-based line:col pair against the original text.
func (r *jsonReader) errHere(code omnist.Code, msg string) error {
	line, col := offsetToLineCol(r.text, r.dec.InputOffset())
	return &omnist.ParseError{Line: line, Col: col, Path: fmt.Sprintf("%d:%d", line, col), Code: code, Message: msg}
}

// offsetToLineCol converts a 0-based byte offset into a 1-based line:col
// pair, counting '\n' bytes. This is only used for error reporting, so
// exactness for exotic line-ending conventions is not load-bearing.
//
// offset always comes from encjson.Decoder.InputOffset(), which is bounded to
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

// readDocument builds the top-level omnist.Document from the already-read first
// token, per the model-mapping table: an object becomes a node document, a
// bare scalar becomes a value document. A bare top-level array is rejected
// (see readArrayElements's doc comment for why).
func (r *jsonReader) readDocument(tok encjson.Token) (omnist.Document, error) {
	if delim, ok := tok.(encjson.Delim); ok {
		switch delim {
		case '{':
			node, err := r.readObjectBody()
			if err != nil {
				return omnist.Document{}, err
			}
			return omnist.NodeDocument(node), nil
		case '[':
			// document.unlabeled-element is a document.* code, so per spec
			// §8.4 its path MUST be a omnist.Document path; "$" is the
			// whole-document fallback since a top-level array has no
			// label of its own to descend by.
			line, col := offsetToLineCol(r.text, r.dec.InputOffset())
			return omnist.Document{}, &omnist.ParseError{Line: line, Col: col, Path: "$", Code: omnist.CodeDocumentUnlabeledElement, Message: "a top-level array has no label to attach to"}
		}
	}
	v, err := r.scalarToValue(tok, omnist.RootPath())
	if err != nil {
		return omnist.Document{}, err
	}
	return omnist.ValueDocument(v), nil
}

// readObjectBody reads object members up to (and including) the closing
// '}', with the opening '{' already consumed by the caller. Per the
// model-mapping table, each key becomes a label; a key whose value is a
// JSON array expands into one edge per array element sharing that label
// (the "repeated label" rule) rather than a single list-shaped edge, since
// the omnist.Document model has no list construct (issue #1's design).
//
// The caller is responsible for omnist.LimitChecker.EnterNode/LeaveNode around
// this call when the object being read is itself a nested target (a
// record inside another record, or an array element) — mirroring
// oml_parser.go's parseBracedNode convention. The top-level object is
// deliberately NOT wrapped: like OML's brace-less top level, the omnist.Document
// root does not count as one more level of nesting away from itself.
func (r *jsonReader) readObjectBody() (*omnist.Node, error) {
	node := omnist.NewNode()
	for r.dec.More() {
		keyTok, err := r.next()
		if err != nil {
			return nil, err
		}
		// JSON's own grammar requires an object member's key to be a quoted
		// string; encjson.Decoder enforces that itself before Token() ever
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
func (r *jsonReader) readMember(node *omnist.Node, key string, valTok encjson.Token) error {
	if delim, ok := valTok.(encjson.Delim); ok {
		switch delim {
		case '{':
			savedPath := r.path
			r.path = r.path.Child(key, 0, false)
			child, err := r.readNestedObject()
			r.path = savedPath
			if err != nil {
				return err
			}
			node.AddNode(key, child)
			return nil
		case '[':
			targets, err := r.readArrayElements(key)
			if err != nil {
				return err
			}
			for _, t := range targets {
				node.Edges = append(node.Edges, omnist.Edge{Label: key, Target: t})
			}
			return nil
		}
	}
	v, err := r.scalarToValue(valTok, r.path.Child(key, 0, false))
	if err != nil {
		return err
	}
	node.AddValue(key, v)
	return nil
}

// readNestedObject reads a '{'-started object that is a target somewhere
// beneath the root (an object member's value, or an array element),
// enforcing the depth/node-count limits via the shared omnist.LimitChecker
// around the read, per limits.go's EnterNode/LeaveNode contract.
func (r *jsonReader) readNestedObject() (*omnist.Node, error) {
	// document.limit.depth/document.limit.nodes are document.* codes, so
	// per spec §8.4 their path MUST be a omnist.Document path, never
	// text-position — "$" here for the same reason oml_parser.go's
	// parseBracedNode uses it (no general path more specific than the
	// whole document is tracked for a limit that can be tripped by any
	// descendant).
	line, col := offsetToLineCol(r.text, r.dec.InputOffset())
	if diag := r.checker.EnterNode("$"); diag != nil {
		return nil, &omnist.ParseError{Line: line, Col: col, Path: "$", Code: diag.Code, Message: diag.Message}
	}
	defer r.checker.LeaveNode()
	return r.readObjectBody()
}

// readArrayElements reads array elements up to (and including) the
// closing ']', with the opening '[' already consumed by the caller, and
// returns one omnist.Target per element.
//
// A JSON array is sugar for a repeated label (spec docs/formats/json.md's
// model-mapping table): it is not itself a omnist.Document-model construct, so
// an array element that is itself an array has no label to attach to and
// is rejected — "Bare nested arrays are rejected... inner elements with no
// label and therefore no edge to occupy." The taxonomy has no
// JSON-specific code for this; omnist.CodeDocumentUnlabeledElement is the
// existing code (checked against errors.go, spec §8.3.2) whose own name
// and description ("unlabeled element") match this situation exactly, so
// it is reused here rather than minting a new one — the same reasoning
// readDocument uses for a bare top-level array, which is the same
// "array with nothing to hold it" situation one level up.
//
// An empty array ('[]') is treated the same way OML treats its identical
// array-as-repeated-label-sugar construct (oml_parser.go's parseArray):
// rejected with omnist.CodeParseEmptyArray, rather than silently producing zero
// edges for the label. This is a narrow, cosmetic reading rather than a
// load-bearing one — docs/formats/json.md's model-mapping table has no
// empty-array row to consult, but JSON's array sugar is explicitly the
// same mechanism OML's array sugar is ("the array is not a value in the
// model, it is the same label occurring more than once"), and OML's reader
// already treats zero repeats of that sugar as an error rather than a
// silent no-op; this reader follows that existing, in-repo precedent for
// the identical construct rather than inventing a second behavior for it.
func (r *jsonReader) readArrayElements(key string) ([]omnist.Target, error) {
	if !r.dec.More() {
		// Consume the ']' before erroring so callers don't have to.
		errPos := r.errHere(omnist.CodeParseEmptyArray, "'[]' is not a valid value")
		if _, err := r.next(); err != nil {
			return nil, err
		}
		return nil, errPos
	}

	var targets []omnist.Target
	index := 0
	for r.dec.More() {
		tok, err := r.next()
		if err != nil {
			return nil, err
		}
		// elemPath is this element's omnist.Document path, per §8.4: a JSON array
		// is sugar for a repeated edge (spec docs/formats/json.md's
		// model-mapping table), so every element is treated as an
		// occurrence of the repeated label key, indexed by its position —
		// even a single-element array, since it is the array construct
		// (not the eventual edge count) that marks this as repeated-label
		// sugar.
		elemPath := r.path.Child(key, index, true)
		if delim, ok := tok.(encjson.Delim); ok {
			switch delim {
			case '{':
				savedPath := r.path
				r.path = elemPath
				child, err := r.readNestedObject()
				r.path = savedPath
				if err != nil {
					return nil, err
				}
				targets = append(targets, omnist.NodeTarget(child))
				index++
				continue
			case '[':
				line, col := offsetToLineCol(r.text, r.dec.InputOffset())
				return nil, &omnist.ParseError{Line: line, Col: col, Path: elemPath.String(), Code: omnist.CodeDocumentUnlabeledElement, Message: "an array element must not itself be an array"}
			}
		}
		v, err := r.scalarToValue(tok, elemPath)
		if err != nil {
			return nil, err
		}
		targets = append(targets, omnist.ValueTarget(v))
		index++
	}
	// Consume the closing ']'.
	if _, err := r.next(); err != nil {
		return nil, err
	}
	return targets, nil
}

// scalarToValue converts one already-read non-Delim JSON token into a
// omnist.Document omnist.Value. Per the "no temporal types" rule (docs/formats/json.md),
// a date-looking string is never upgraded here — it stays KindString,
// since stage 1 never consults a schema and JSON strings carry no type tag
// of their own.
//
// Every caller has already excluded encjson.Delim before reaching here (the
// top-level, object-member, and array-element call sites each branch on
// encjson.Delim first and return/continue before falling through to this
// function), so the type switch's only remaining possibilities from
// encjson.Decoder.UseNumber()'s token set are nil, bool, string, and
// encjson.Number — an exhaustive set with no default case needed, matching
// the no-dead-branch convention temporal.go's temporal decoders already
// use (see the comment above omnist.ParseISODate).
func (r *jsonReader) scalarToValue(tok encjson.Token, path omnist.Path) (omnist.Value, error) {
	switch v := tok.(type) {
	case nil:
		return omnist.NullValue(), nil
	case bool:
		return omnist.ScalarValue(omnist.NewBooleanScalar(v)), nil
	case string:
		return omnist.ScalarValue(omnist.NewStringScalar(v)), nil
	default:
		return r.numberToValue(v.(encjson.Number), path)
	}
}

// numberToValue implements the integer/number split by literal shape, per
// spec §2.2.1: a literal with no '.', 'e', or 'E' is an integer; any other
// numeric literal is a number. encjson.Number preserves the original literal
// text (that's the entire reason Read calls UseNumber()), so this
// inspects the text directly rather than the decoded magnitude.
//
// path is this value's own omnist.Document path (spec §8.4), supplied by the
// caller (the top-level document, an object member, or an array element
// each know their own path differently) — used only for
// document.limit.int-digits, which is a document.* code and so MUST carry
// a omnist.Document path, never the text-position path every parse.* diagnostic
// in this file uses.
func (r *jsonReader) numberToValue(n encjson.Number, path omnist.Path) (omnist.Value, error) {
	s := string(n)
	if strings.ContainsAny(s, ".eE") {
		f, err := n.Float64()
		if err != nil {
			return omnist.Value{}, r.errHere(omnist.CodeParseUnexpectedToken, "invalid number literal: "+s)
		}
		return omnist.ScalarValue(omnist.NewNumberScalar(f)), nil
	}

	digits := strings.TrimPrefix(s, "-")
	if diag := r.checker.CheckIntDigits(path.String(), len(digits)); diag != nil {
		line, col := offsetToLineCol(r.text, r.dec.InputOffset())
		return omnist.Value{}, &omnist.ParseError{Line: line, Col: col, Path: diag.Path, Code: diag.Code, Message: diag.Message}
	}
	// s is a JSON-grammar-valid integer literal (the only kind
	// encjson.Decoder.UseNumber() ever hands back for a no-'.'/'e'/'E' token):
	// -?(0|[1-9]\d*). SetString with base 10 cannot fail on such a string,
	// so — mirroring temporal.go's omnist.ParseISODate/omnist.ParseISOTime
	// convention for a regex-pinned precondition — this does not carry a
	// permanently-dead "malformed" branch for a case no input can reach.
	bi, _ := new(big.Int).SetString(s, 10)
	return omnist.ScalarValue(omnist.NewIntegerScalar(bi)), nil
}
