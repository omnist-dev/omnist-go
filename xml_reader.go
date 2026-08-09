package omnist

import (
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// ReadXML parses XML source text into a Document (spec §7.1,
// docs/formats/xml.md, "stage 1 only, no schema"). limits configures the
// safety limits enforced while reading, via the same LimitChecker every
// other codec in this package uses (limits.go).
//
// # Library choice
//
// Go's stdlib encoding/xml is used purely as a token-stream tokenizer:
// xml.Decoder.Token(), never the struct-tag-driven Unmarshal path (which
// would impose its own shape assumptions this reader does not want — it
// needs to build a Document tree directly from the raw element structure,
// not decode into a predetermined Go type). No third-party dependency is
// needed: XML's stage-1 typing story has no number/temporal ambiguity to
// resolve (docs/formats/xml.md: "every leaf arrives as a string... XML
// carries no type information"), so, unlike ReadJSON/ReadYAML/ReadTOML,
// there is no literal-shape distinction this reader needs from a
// lower-level API than the stdlib token stream already gives it.
//
// # Interleaving preservation — the central correctness property here
//
// docs/formats/xml.md: "`<m/><x/><m/>` reads as `[(m,...),(x,...),(m,...)]`
// in that order." Every other codec's reader in this package (ReadJSON's
// readObjectBody, ReadYAML's mapping reader, ReadTOML's navigateOrCreate)
// still preserves source order of DISTINCT keys, but nothing in JSON/YAML/
// TOML's own grammar lets the SAME key occur twice non-adjacently at one
// level in the first place (a JSON/YAML object key is unique by
// construction; a repeated TOML key is a redefinition error) — so those
// readers have never had an interleaving case to get wrong. XML does allow
// exactly that (`<m/><x/><m/>`), which is why docs/formats/xml.md calls
// this out as the property "the whole reason the Document is an ordered
// edge list rather than a map."
//
// This reader achieves it with no special-case logic at all: readElement
// below appends one Edge per child StartElement to the parent *Node's
// Edges slice, in the exact order xml.Decoder.Token() emits them — the
// same "just append to Edges in encounter order" mechanism every reader in
// this package already uses for ordinary sibling order. Never grouping by
// label (the way groupJSONEdges/groupYAMLEdges/groupTOMLEdges do, purely
// for those write-side formats' own grouped-key syntax) is what keeps
// same-label edges from ever being merged or reordered relative to a
// different label between them. See TestReadXMLPreservesInterleaving.
//
// # Attributes and namespace prefixes: silently dropped
//
// docs/formats/xml.md / spec §9.4 D-3: "Attributes and namespace prefixes
// are dropped" — silently, with "no adjustment reported... implementations
// MUST behave identically here" until a future conformance vector defines
// reporting codes for it. This reader honors that literally: readElement
// never inspects StartElement.Attr at all (attributes are not merely
// discarded after being read — they are never read in the first place),
// and every element name is taken from xml.Name.Local only, never
// xml.Name.Space, which is what discards a namespace prefix/binding (Go's
// xml.Decoder resolves a declared prefix to a namespace URI in Name.Space,
// or leaves an undeclared prefix's literal text there — either way, using
// only .Local drops it uniformly, without needing to special-case which of
// those two cases occurred). No Diagnostic, no return value, nothing is
// built to report this — see errors_test.go/xml_reader_test.go for the
// test asserting a clean nil-error, no-diagnostic result.
//
// # Single document element, enforced on read too
//
// docs/formats/xml.md describes the single-top-level-element rule as a
// write-side constraint ("A Document with several top-level edges cannot
// be written as XML"), but says nothing explicit about a read-side input
// with multiple top-level elements — genuinely not well-formed XML (the
// XML 1.0 grammar itself requires exactly one root element), but Go's
// encoding/xml.Decoder is a lenient, general-purpose token streamer that
// does not itself enforce that rule; asked to decode `<a/><b/>` it happily
// emits StartElement a, EndElement a, StartElement b, EndElement b, then
// EOF, with no error at any point. This is the plainly-correct reading of
// a narrow, cosmetic gap: this reader enforces well-formedness itself by
// treating any token after the root element's matching EndElement — other
// than trailing whitespace, comments, or processing instructions — as
// CodeParseTrailingContent, mirroring ReadJSON's identical trailing-content
// check (json_reader.go) for the same "more than one top-level value"
// situation.
func ReadXML(text string, limits Limits) (Document, error) {
	dec := xml.NewDecoder(strings.NewReader(text))
	r := &xmlReader{dec: dec, checker: NewLimitChecker(limits)}

	label, node, isLeaf, leafText, err := r.readRoot()
	if err != nil {
		return Document{}, err
	}
	if err := r.checkTrailing(); err != nil {
		return Document{}, err
	}

	root := NewNode()
	if isLeaf {
		root.AddValue(label, ScalarValue(NewStringScalar(leafText)))
	} else {
		root.AddNode(label, node)
	}
	return NodeDocument(root), nil
}

// xmlReader holds the streaming-decode state for one ReadXML call.
type xmlReader struct {
	dec     *xml.Decoder
	checker *LimitChecker
}

// readRoot scans past any leading prolog content (an XML declaration,
// comments, processing instructions, or insignificant whitespace) to find
// the document's one root StartElement, then reads it via readElement.
//
// The switch below has no default case for a reason confirmed empirically:
// xml.Token's only concrete types are StartElement, EndElement, CharData,
// Comment, ProcInst, and Directive, and every one but EndElement is handled
// by name here — an EndElement can never legally be the first
// non-prolog/non-whitespace token in a well-formed document (there is no
// open element yet for it to close), and Go's own xml.Decoder enforces
// that itself before Token() ever returns one: a probe against a stray
// `</a>` with nothing open confirms Token() reports "unexpected end
// element" as its error return rather than handing back an EndElement
// value for this loop to see. So this switch is exhaustive over every
// value this loop can actually receive, with no unreachable default branch
// to carry, mirroring this package's established no-dead-branch convention
// (see e.g. json_reader.go's readObjectBody comment on its key-type
// assertion for the same reasoning applied to a different guaranteed
// precondition).
func (r *xmlReader) readRoot() (label string, node *Node, isLeaf bool, leafText string, err error) {
	for {
		tok, err := r.next()
		if err != nil {
			return "", nil, false, "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			label = t.Name.Local
			node, isLeaf, leafText, err = r.readElementBody()
			return label, node, isLeaf, leafText, err
		case xml.CharData:
			if len(strings.TrimSpace(string(t))) != 0 {
				return "", nil, false, "", r.errHere(CodeParseUnexpectedToken, "unexpected text outside the document element")
			}
			// insignificant whitespace before the root element: skip
		default: // xml.ProcInst, xml.Comment, xml.Directive
			// prolog content: skip
		}
	}
}

// checkTrailing consumes tokens after the root element's matching
// EndElement, allowing only insignificant whitespace, comments, and
// processing instructions before EOF — see ReadXML's doc comment ("Single
// document element, enforced on read too").
func (r *xmlReader) checkTrailing() error {
	for {
		tok, err := r.dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return r.wrapDecodeErr(err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			if len(strings.TrimSpace(string(t))) != 0 {
				return r.errHere(CodeParseTrailingContent, "content remains after the document element")
			}
		case xml.ProcInst, xml.Comment, xml.Directive:
			// trailing prolog-like content: skip
		default:
			return r.errHere(CodeParseTrailingContent, "content remains after the document element")
		}
	}
}

// next reads the next token, translating any decode error into a
// *ParseError.
func (r *xmlReader) next() (xml.Token, error) {
	tok, err := r.dec.Token()
	if err != nil {
		return nil, r.wrapDecodeErr(err)
	}
	return tok, nil
}

// wrapDecodeErr converts an error from the underlying xml.Decoder into a
// *ParseError, mirroring ReadJSON's wrapDecodeErr (json_reader.go): the
// stdlib decoder does not expose an error taxonomy this package's Code
// values can select between meaningfully, so every low-level decode
// failure reports CodeParseUnexpectedToken with the decoder's own message,
// positioned by its InputOffset.
func (r *xmlReader) wrapDecodeErr(err error) error {
	if errors.Is(err, io.EOF) {
		return r.errHere(CodeParseUnexpectedToken, "unexpected end of input")
	}
	return r.errHere(CodeParseUnexpectedToken, err.Error())
}

// errHere builds a *ParseError positioned at the decoder's current byte
// offset, translated to a 1-based line:col pair — reusing offsetToLineCol
// (json_reader.go), which is format-agnostic (it walks raw text bytes, not
// anything JSON-specific).
func (r *xmlReader) errHere(code Code, msg string) error {
	return &ParseError{Path: r.pathHere(), Code: code, Message: msg}
}

func (r *xmlReader) pathHere() string {
	// xml.Decoder has no InputOffset method analogous to json.Decoder's;
	// it does expose InputPos (line, column) directly, which is exactly
	// the pair every other reader's Path field renders as "line:col", with
	// no byte-offset-to-line/col conversion needed at all.
	line, col := r.dec.InputPos()
	return strconv.Itoa(line) + ":" + strconv.Itoa(col)
}

// readElementBody reads one element's children (the StartElement itself
// already consumed by the caller — either by readRoot for the document
// element, or by readChild below for every other element), up to and
// including the matching EndElement. Per docs/formats/xml.md's
// model-mapping table:
//
//   - An element with one or more child elements becomes a Node: one Edge
//     per child StartElement, appended to Edges in exact source order —
//     this is the entire interleaving-preservation mechanism (see ReadXML's
//     doc comment).
//   - An element with no child elements is a leaf: its text content (all
//     CharData tokens directly inside it, concatenated) becomes a plain
//     string Value, always — "every leaf arrives as a string... zero
//     auto-typing" (docs/formats/xml.md). A self-closing element (`<b/>`)
//     is the empty-string leaf, since xml.Decoder emits a StartElement
//     immediately followed by the matching EndElement for it, with no
//     CharData token in between at all.
//
// Mixed content (an element with BOTH child elements and non-whitespace
// text alongside them) has no entry in docs/formats/xml.md's model-mapping
// table at all — the Document model has no construct for "a node that is
// also partially text". This is a narrow, cosmetic gap: the plainly-correct
// reading taken here is that once an element is known to have at least one
// child element, it is a Node, full stop, and any CharData encountered
// alongside those children (typically pure formatting whitespace between
// tags, but not exclusively) is discarded the same way it already is
// between any two sibling elements — not appended anywhere, since there is
// no Document-model slot to put it in.
//
// # Depth/node-count enforcement
//
// Unlike ReadJSON/ReadYAML/ReadTOML (whose value grammars distinguish a
// container from a scalar before this reader ever has to choose whether to
// recurse — a JSON '{' vs a bare literal, say), an XML element gives no
// such signal up front: `<a>...</a>` might turn out to be a leaf or a Node,
// and that is only known after walking its full body. Bounding MaxDepth
// therefore cannot wait until an element is known to have children (by
// then the recursive call that needed bounding has already happened) — see
// readChild, which enforces the limit on every child element BEFORE
// recursing into it, regardless of what that element turns out to contain,
// rather than only on elements later found to have children of their own.
// This is a deliberate, slightly more conservative choice than
// readNestedObject's (json_reader.go counts only realized objects): it
// treats every element below the (uncounted, per every other reader's own
// top-level convention) document root as one unit of depth/node budget,
// which is the safe reading given XML's leaf-or-node ambiguity, and is
// consistent with the fact that every element genuinely is a candidate
// Node until its body says otherwise.
func (r *xmlReader) readElementBody() (node *Node, isLeaf bool, leafText string, err error) {
	var text strings.Builder
	var children *Node

	for {
		tok, err := r.next()
		if err != nil {
			return nil, false, "", err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if children == nil {
				return nil, true, text.String(), nil
			}
			return children, false, "", nil
		case xml.CharData:
			text.Write(t)
		case xml.StartElement:
			if children == nil {
				children = NewNode()
			}
			label := t.Name.Local
			childNode, childIsLeaf, childText, err := r.readChild()
			if err != nil {
				return nil, false, "", err
			}
			if childIsLeaf {
				children.AddValue(label, ScalarValue(NewStringScalar(childText)))
			} else {
				children.AddNode(label, childNode)
			}
		case xml.ProcInst, xml.Comment, xml.Directive:
			// ignored wherever it appears
		}
	}
}

// readChild reads one non-root element (its StartElement already
// consumed), enforcing MaxDepth/MaxNodes via the shared LimitChecker around
// the read — see readElementBody's doc comment for why every element, not
// only ones later found to have children, is wrapped this way.
func (r *xmlReader) readChild() (node *Node, isLeaf bool, leafText string, err error) {
	path := r.pathHere()
	if diag := r.checker.EnterNode(path); diag != nil {
		return nil, false, "", &ParseError{Path: diag.Path, Code: diag.Code, Message: diag.Message}
	}
	defer r.checker.LeaveNode()
	return r.readElementBody()
}
