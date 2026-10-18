package xml

import (
	encxml "encoding/xml"
	"errors"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/internal/textpos"
)

// Read parses XML source text into an omnist.Document without a schema.
//
// Every leaf arrives as a string (spec §7.1, docs/formats/xml.md). Callers
// with an OSD schema should use ReadWithSchema to pre-type leaves into numeric,
// boolean, and temporal scalars per omnist-spec#44.
//
// The signature returns diagnostics alongside the omnist.Document and error,
// matching every writer in this repository (issue #49) and, as of D-3
// (spec §8.3.8), this reader too: every dropped attribute
// (format.attribute-dropped) and every dropped namespace prefix
// (format.namespace-dropped) is now reported this way rather than
// silently -- see ReadWithSchema's "Attribute and namespace-prefix
// dropping" section below.
func Read(src string, limits omnist.Limits) (omnist.Document, []omnist.Diagnostic, error) {
	return ReadWithSchema(src, nil, limits)
}

// ReadWithSchema parses XML source text into an omnist.Document.
//
// If schema is non-nil, text leaves are pre-typed into boolean, integer, number,
// or temporal scalars when the schema's declared field type calls for it (spec §7.1,
// omnist-spec#44). Leaves that do not parse value-exactly remain strings and fall
// through to normal stage-2 diagnostics. An integer leaf is read only from
// -?(0|[1-9][0-9]*) and a number leaf only from a JSON number (C-18), so +5,
// 007, 1., .5 and 1_000 stay strings. A number past binary64 reads as an
// infinity (underflow as zero, -0 as negative zero), with no length limit
// (D-29); the integer digit limit applies to integer leaves only (D-28).
//
// limits configures the safety limits enforced while reading, via the same
// omnist.LimitChecker every other codec in this package uses (limits.go).
//
// # Library choice
//
// Go's stdlib encoding/xml is used purely as a token-stream tokenizer:
// encxml.Decoder.Token(), never the struct-tag-driven Unmarshal path (which
// would impose its own shape assumptions this reader does not want ? it
// needs to build an omnist.Document tree directly from the raw element structure,
// not decode into a predetermined Go type).
//
// # Interleaving preservation ? the central correctness property here
//
// docs/formats/xml.md: "`<m/><x/><m/>` reads as `[(m,...),(x,...),(m,...)]`
// in that order." Every other codec's reader in this package (ReadJSON's
// readObjectBody, ReadYAML's mapping reader, ReadTOML's navigateOrCreate)
// still preserves source order of DISTINCT keys, but nothing in JSON/YAML/
// TOML's own grammar lets the SAME key occur twice non-adjacently at one
// level in the first place (a JSON/YAML object key is unique by
// construction; a repeated TOML key is a redefinition error) ? so those
// readers have never had an interleaving case to get wrong. XML does allow
// exactly that (`<m/><x/><m/>`), which is why docs/formats/xml.md calls
// this out as the property "the whole reason the omnist.Document is an ordered
// edge list rather than a map."
//
// This reader achieves it with no special-case logic at all: readElementBody
// appends every child StartElement to children.Edges in the exact order
// Decoder.Token emits it.
//
// # Single document element, enforced on read too
//
// XML grammar forbids multiple document elements (`<a/><b/>`) and forbids
// text outside the document element (`stray text<a/>` or `<a/>stray`).
// The decoder handles this via two checks:
//
//   - readRoot loops past any leading CharData/Comment/ProcInst, takes the
//     first StartElement as the document element, and rejects non-whitespace
//     CharData before it.
//   - checkTrailing runs after the document element closes and consumes
//     tokens until EOF, rejecting any further StartElement or non-whitespace
//     CharData.
//
// # The data-XML profile, and why its refusals run last
//
// docs/formats/xml.md refuses three well-formed constructs: any DOCTYPE
// declaration (format.dtd-forbidden), an entity reference other than the five
// predefined ones (format.entity-forbidden), and mixed content
// (format.mixed-content), each at path "$". They are refusals, not syntax
// errors, and a document that is not well-formed is a parse.codec-syntax
// failure even if it also contains one of the three. So this reader records the
// first profile violation it meets and keeps reading; a syntax error anywhere in
// the document wins, and the recorded refusal is returned only once the whole
// document has been read successfully.
//
// Go's decoder never resolves a DOCTYPE-defined entity, so an undeclared entity
// reference would be a fatal decode error that stops the read before the rest of
// the document could be checked. To keep reading past one, every non-predefined
// name that appears as &name; in the source is registered in Decoder.Entity with
// a sentinel replacement: a private-use rune chosen because it does not occur in
// the source, so it can only have come from an expansion. A sentinel found in
// text or an attribute value is an entity reference actually used; one inside a
// comment, CDATA section or processing instruction is never expanded and so
// never flagged.
//
// # A leading byte-order mark
//
// Spec §2.5 applies before anything else (omnist.PrepareInput): D-14 rejects
// invalid UTF-8 as parse.invalid-encoding at 1:1, then D-15 strips one leading
// U+FEFF and D-21 makes a second a parse.codec-syntax failure at 1:1. encoding/xml would otherwise treat the first as
// stray text before the document element.
//
// # Attribute and namespace-prefix dropping
//
// Per docs/formats/xml.md ("Attributes and namespace prefixes are
// dropped"), attributes leave no trace in the resulting omnist.Document, and any
// StartElement/EndElement Name.Space prefix is discarded, keeping only
// Name.Local as the edge label. Per D-3 (spec §8.3.8), both drops are now
// reported rather than silent: an element with one or more attributes
// emits one omnist.CodeFormatAttributeDropped warning at that element's own
// Document path (e.g. "$.a" for `<a x="1"><b>hi</b></a>`), and an element
// whose tag carried a namespace prefix emits one
// omnist.CodeFormatNamespaceDropped warning, also at that element's own
// Document path (e.g. "$.a.b" for `<a><ns:b>hi</ns:b></a>`). Both checks
// live in readStart, run once per StartElement token (root and every
// child alike) as soon as that element's own Document path is known.
func ReadWithSchema(src string, schema *omnist.Schema, limits omnist.Limits) (omnist.Document, []omnist.Diagnostic, error) {
	src, berr := omnist.PrepareDocumentInput(src, omnist.CodeParseCodecSyntax, limits)
	if berr != nil {
		return omnist.Document{}, nil, berr
	}
	if len(src) == 0 {
		return omnist.Document{}, nil, &omnist.ParseError{
			Line:    1,
			Col:     1,
			Path:    "1:1",
			Code:    omnist.CodeParseCodecSyntax,
			Message: "XML: unexpected end of input",
		}
	}
	pos := textpos.NewIndex(src)
	named, scanErr := scanXMLCharacters(src, pos)
	if scanErr != nil {
		return omnist.Document{}, nil, scanErr
	}
	dec := encxml.NewDecoder(strings.NewReader(src))
	dec.CharsetReader = anyCharset
	sentinel := entitySentinel(src, named)
	dec.Entity = entityMap(src, sentinel)
	r := &xmlReader{
		dec:      dec,
		src:      src,
		pos:      pos,
		checker:  omnist.NewLimitChecker(limits),
		schema:   schema,
		sentinel: sentinel,
	}
	// The Document root is the first node (spec §2.4: depth is counted from
	// the root, so the root is level 1) and counts against both limits, like
	// the OML reader's root. Only an element that has child elements is a
	// node; a leaf element is a value and is counted by neither limit.
	if diag := r.checker.EnterNode("$"); diag != nil {
		return omnist.Document{}, nil, &omnist.ParseError{Line: 1, Col: 1, Path: "$", Code: diag.Code, Message: diag.Message}
	}
	label, node, isLeaf, leafText, err := r.readRoot()
	if err != nil {
		return omnist.Document{}, nil, err
	}
	if err := r.checkTrailing(); err != nil {
		return omnist.Document{}, nil, err
	}
	// The whole document is well-formed; only now may a recorded profile
	// refusal be reported (see "The data-XML profile" above).
	if r.profile != nil {
		return omnist.Document{}, nil, r.profile
	}
	root := omnist.NewNode()
	diags := r.finishDiags()
	if isLeaf {
		sc := omnist.NewStringScalar(leafText)
		if r.schema != nil {
			if rootRec := r.schema.Env[r.schema.Root]; rootRec != nil {
				if f := findField(rootRec, label); f != nil && f.Type.Kind == omnist.TypeScalarKind {
					if typed, ok := r.pretype(leafText, f.Type.ScalarKind, &elemInfo{label: label}); ok {
						sc = typed
					}
				}
			}
		}
		root.AddValue(label, omnist.ScalarValue(sc))
	} else {
		root.AddNode(label, node)
	}
	// E-4a: an integer leaf over the digit limit is refused at the leaf's
	// Document path, indexed per E-10 now that every sibling is known.
	if r.digitAt != nil {
		return omnist.Document{}, nil, &omnist.ParseError{Path: r.digitAt.path(), Code: r.digitDiag.Code, Message: r.digitDiag.Message}
	}
	return omnist.NodeDocument(root), diags, nil
}

type xmlReader struct {
	dec     *encxml.Decoder
	src     string         // the input after PrepareInput, for code-point columns (E-28)
	pos     *textpos.Index // line index over src, built once; keeps position lookups linear overall
	checker *omnist.LimitChecker
	schema  *omnist.Schema
	// pending holds the D-3 dropped-attribute and dropped-namespace reports
	// in emission order. Their paths are resolved only when the whole
	// document has been read (finishDiags): E-10 indexes every edge of a
	// label that occurs more than once in its node, the first included, and
	// whether the first occurrence repeats is not known until its later
	// siblings have been read.
	pending []pendingDiag
	// profile is the first data-XML profile refusal seen so far, or nil.
	profile *omnist.ParseError
	// sentinel is the rune every non-predefined entity reference expands to.
	sentinel rune
	// digitAt and digitDiag hold the first schema-typed integer leaf over the
	// digit limit (E-4a), reported once the whole document has been read.
	digitAt   *elemInfo
	digitDiag *omnist.Diagnostic
}

// entityRef matches a named entity reference. Numeric character references
// (&#65; &#x41;) start with '#' and never match.
var entityRef = regexp.MustCompile(`&([\p{L}_:][\p{L}\p{N}_:.\-]*);`)

// predefinedEntity reports whether name is one of XML's five predefined
// entities, which the decoder resolves itself.
func predefinedEntity(name string) bool {
	switch name {
	case "lt", "gt", "amp", "quot", "apos":
		return true
	}
	return false
}

// entitySentinel returns a private-use rune that does not occur in src, so any
// occurrence of it in decoded text can only be the expansion of an entity.
func entitySentinel(src string, named map[rune]bool) rune {
	r := rune(0xE000)
	for strings.ContainsRune(src, r) || named[r] {
		r++
	}
	return r
}

// entityMap registers every non-predefined named entity referenced in src with
// the sentinel as its replacement text, so the decoder keeps reading instead of
// failing at the first one.
func entityMap(src string, sentinel rune) map[string]string {
	m := map[string]string{}
	for _, sub := range entityRef.FindAllStringSubmatch(src, -1) {
		if !predefinedEntity(sub[1]) {
			m[sub[1]] = string(sentinel)
		}
	}
	return m
}

// refuse records the first data-XML profile refusal. Later ones are dropped:
// a read reports one diagnostic, and the earliest in document order is the
// most useful.
func (r *xmlReader) refuse(code omnist.Code, msg string) {
	if r.profile == nil {
		r.profile = &omnist.ParseError{Path: "$", Code: code, Message: msg}
	}
}

// checkText flags a use of a non-predefined entity in decoded text.
func (r *xmlReader) checkText(s string) {
	if strings.ContainsRune(s, r.sentinel) {
		r.refuse(omnist.CodeFormatEntityForbidden, "an entity reference other than the five predefined entities is outside the data-XML profile")
	}
}

// readRoot finds the single document element, consumes it via
// readElementBody, and returns its local name and body. Leading
// whitespace, comments, and processing instructions are skipped. Any
// non-whitespace text before the root element is rejected with
// omnist.CodeParseCodecSyntax.
func (r *xmlReader) readRoot() (label string, node *omnist.Node, isLeaf bool, leafText string, err error) {
	for {
		tok, err := r.next()
		if err != nil {
			return "", nil, false, "", err
		}
		switch t := tok.(type) {
		case encxml.StartElement:
			label = t.Name.Local
			info := &elemInfo{label: label}
			r.readStart(t, info)
			var currentRec *omnist.Record
			if r.schema != nil {
				if rootRec := r.schema.Env[r.schema.Root]; rootRec != nil {
					if f := findField(rootRec, label); f != nil {
						if f.Type.Kind == omnist.TypeRefKind {
							currentRec = r.schema.Env[f.Type.RefName]
						}
					} else if rootRec.Name == label || r.schema.Root == label {
						currentRec = rootRec
					} else {
						currentRec = r.schema.Env[label]
					}
				} else {
					currentRec = r.schema.Env[label]
				}
			}
			node, isLeaf, leafText, err = r.readElementBody(currentRec, info)
			return label, node, isLeaf, leafText, err
		case encxml.CharData:
			if len(strings.TrimSpace(string(t))) != 0 {
				return "", nil, false, "", r.errHere(omnist.CodeParseCodecSyntax, "XML: unexpected text outside the document element")
			}
			// insignificant whitespace before the root element: skip
		case encxml.Directive:
			// The prolog is the only place a DOCTYPE is well-formed. It is
			// refused on sight (not when an entity it defines is used); any
			// other <!...> construct is simply not XML.
			if !strings.HasPrefix(string(t), "DOCTYPE") {
				return "", nil, false, "", r.errHere(omnist.CodeParseCodecSyntax, "XML: unrecognized <!...> declaration")
			}
			r.refuse(omnist.CodeFormatDTDForbidden, "a DOCTYPE declaration is outside the data-XML profile")
		default: // encxml.ProcInst, encxml.Comment
			// prolog content: skip
		}
	}
}

// checkTrailing consumes tokens after the root element's matching
// EndElement, allowing only insignificant whitespace, comments, and
// processing instructions before EOF ? see Read's doc comment ("Single
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
		case encxml.CharData:
			if len(strings.TrimSpace(string(t))) != 0 {
				return r.errHere(omnist.CodeParseCodecSyntax, "XML: content remains after the document element")
			}
		case encxml.ProcInst, encxml.Comment:
			// trailing prolog-like content: skip
		default: // a second element, or a <!...> declaration after the root
			return r.errHere(omnist.CodeParseCodecSyntax, "XML: content remains after the document element")
		}
	}
}

// next reads the next token, translating any decode error into a
// *omnist.ParseError.
func (r *xmlReader) next() (encxml.Token, error) {
	tok, err := r.dec.Token()
	if err != nil {
		return nil, r.wrapDecodeErr(err)
	}
	return tok, nil
}

// wrapDecodeErr converts an error from the underlying encxml.Decoder into a
// *omnist.ParseError, mirroring ReadJSON's wrapDecodeErr (json_reader.go): the
// stdlib decoder does not expose an error taxonomy this package's omnist.Code
// values can select between meaningfully, so every low-level decode
// failure reports omnist.CodeParseCodecSyntax (spec §8.3.1) with the decoder's
// own message, positioned by its InputPos.
func (r *xmlReader) wrapDecodeErr(err error) error {
	if errors.Is(err, io.EOF) {
		return r.errHere(omnist.CodeParseCodecSyntax, "XML: unexpected end of input")
	}
	return r.errHere(omnist.CodeParseCodecSyntax, "XML: "+err.Error())
}

// errHere builds a *omnist.ParseError positioned at the decoder's current byte
// offset, translated to a 1-based line:col pair ? reusing offsetToLineCol
// (json_reader.go), which is format-agnostic (it walks raw text bytes, not
// anything JSON-specific).
func (r *xmlReader) errHere(code omnist.Code, msg string) error {
	line, col := r.posHere()
	return &omnist.ParseError{Line: line, Col: col, Path: strconv.Itoa(line) + ":" + strconv.Itoa(col), Code: code, Message: msg}
}

// index returns the reader's line index, building it on first use (tests
// construct an xmlReader directly, without going through ReadWithSchema).
func (r *xmlReader) index() *textpos.Index {
	if r.pos == nil {
		r.pos = textpos.NewIndex(r.src)
	}
	return r.pos
}

// posHere is the decoder's current position: its line (LF-counted, like E-29)
// and its 1-based BYTE column converted to code points (E-28), clamped to lie
// inside the input (E-31).
func (r *xmlReader) posHere() (line, col int) {
	dl, dc := r.dec.InputPos()
	return r.index().FromLineByteCol(dl, dc)
}

// readElementBody reads one element's children up to and including the matching
// EndElement.
func (r *xmlReader) readElementBody(currentRec *omnist.Record, info *elemInfo) (node *omnist.Node, isLeaf bool, leafText string, err error) {
	var text strings.Builder
	var children *omnist.Node
	// entered is true once this element is known to be a node (it has a child
	// element), which is when it is counted against the depth and node
	// limits (E-4a): document.limit.depth/document.limit.nodes at "$".
	entered := false
	defer func() {
		if entered {
			r.checker.LeaveNode()
		}
	}()

	for {
		tok, err := r.next()
		if err != nil {
			return nil, false, "", err
		}
		switch t := tok.(type) {
		case encxml.EndElement:
			if children == nil {
				return nil, true, text.String(), nil
			}
			if strings.TrimSpace(text.String()) != "" {
				r.refuse(omnist.CodeFormatMixedContent, "text alongside child elements (mixed content) is outside the data-XML profile")
			}
			return children, false, "", nil
		case encxml.CharData:
			r.checkText(string(t))
			text.Write(t)
		case encxml.StartElement:
			if children == nil {
				if diag := r.checker.EnterNode("$"); diag != nil {
					line, col := r.posHere()
					return nil, false, "", &omnist.ParseError{Line: line, Col: col, Path: "$", Code: diag.Code, Message: diag.Message}
				}
				entered = true
				children = omnist.NewNode()
			}
			label := t.Name.Local
			f := findField(currentRec, label)
			var childRec *omnist.Record
			if f != nil && f.Type.Kind == omnist.TypeRefKind && r.schema != nil {
				childRec = r.schema.Env[f.Type.RefName]
			}
			childInfo := info.child(label)
			childNode, childIsLeaf, childText, err := r.readChild(childRec, t, childInfo)
			if err != nil {
				return nil, false, "", err
			}
			if childIsLeaf {
				if f != nil && f.Type.Kind == omnist.TypeScalarKind {
					if sc, ok := r.pretype(childText, f.Type.ScalarKind, childInfo); ok {
						children.AddValue(label, omnist.ScalarValue(sc))
						continue
					}
				}
				children.AddValue(label, omnist.ScalarValue(omnist.NewStringScalar(childText)))
			} else {
				children.AddNode(label, childNode)
			}
		case encxml.Directive:
			return nil, false, "", r.errHere(omnist.CodeParseCodecSyntax, "XML: a <!...> declaration is not allowed inside an element")
		case encxml.ProcInst, encxml.Comment:
			// inert wherever they appear
		}
	}
}

// readChild reads one non-root element. The depth and node limits are
// enforced in readElementBody, where an element first turns out to be a node.
func (r *xmlReader) readChild(childRec *omnist.Record, start encxml.StartElement, info *elemInfo) (node *omnist.Node, isLeaf bool, leafText string, err error) {
	r.readStart(start, info)
	return r.readElementBody(childRec, info)
}

// elemInfo is one element's place in the Document being read: its label, its
// zero-based occurrence among its parent's same-label children, and how many
// times each label occurs among its own children so far. A parent's final
// counts decide whether a child's path carries an index (E-10).
type elemInfo struct {
	parent *elemInfo
	label  string
	occ    int
	counts map[string]int
}

// child registers a new child element labeled label and returns its info.
func (e *elemInfo) child(label string) *elemInfo {
	if e.counts == nil {
		e.counts = map[string]int{}
	}
	c := &elemInfo{parent: e, label: label, occ: e.counts[label]}
	e.counts[label]++
	return c
}

// path renders e's Document path (spec §8.4): every edge of a label that
// occurs more than once in its node carries its occurrence index, the first
// included. It is only meaningful once e's parent has been read completely.
func (e *elemInfo) path() string {
	seg := omnist.PathLabelSegment(e.label)
	if e.parent != nil && e.parent.counts[e.label] > 1 {
		seg += "[" + strconv.Itoa(e.occ) + "]"
	}
	if e.parent == nil {
		return "$" + seg
	}
	return e.parent.path() + seg
}

// pendingDiag is a dropped-attribute or dropped-namespace report whose path
// awaits finishDiags.
type pendingDiag struct {
	at   *elemInfo
	code omnist.Code
	msg  string
}

// finishDiags resolves every pending report's path now that every parent's
// children are all known.
func (r *xmlReader) finishDiags() []omnist.Diagnostic {
	var out []omnist.Diagnostic
	for _, p := range r.pending {
		out = append(out, omnist.Diagnostic{
			Path:     p.at.path(),
			Code:     p.code,
			Message:  p.msg,
			Severity: omnist.SeverityWarning,
		})
	}
	return out
}

// readStart records D-3's two report-not-silently-drop diagnostics (spec
// §8.3.8) for one StartElement, at that element's own Document path
// (resolved by finishDiags once the path's indexes are known): one omnist.CodeFormatAttributeDropped warning if the element
// carries one or more attributes, and one omnist.CodeFormatNamespaceDropped
// warning if the element's tag itself had a namespace prefix
// (start.Name.Space != ""). Called once per StartElement -- root
// (readRoot) and every child (readChild) alike -- as soon as that
// element's docPath is known, before its body is read.
func (r *xmlReader) readStart(start encxml.StartElement, info *elemInfo) {
	for _, a := range start.Attr {
		r.checkText(a.Value)
	}
	if len(start.Attr) > 0 {
		r.pending = append(r.pending, pendingDiag{info, omnist.CodeFormatAttributeDropped, "an XML attribute was discarded on read"})
	}
	if start.Name.Space != "" {
		r.pending = append(r.pending, pendingDiag{info, omnist.CodeFormatNamespaceDropped, "an XML namespace prefix was discarded on read"})
	}
}

func findField(rec *omnist.Record, label string) *omnist.Field {
	if rec == nil {
		return nil
	}
	for i := range rec.Fields {
		if rec.Fields[i].Label == label {
			return &rec.Fields[i]
		}
	}
	return nil
}

// isJSONInteger reports whether text matches -?(0|[1-9][0-9]*), the only
// spelling an integer-typed XML leaf is read from (C-18).
func isJSONInteger(text string) bool {
	i := 0
	if i < len(text) && text[i] == '-' {
		i++
	}
	return scanJSONDigits(text, &i) && i == len(text)
}

// isJSONNumber reports whether text matches the JSON number grammar
// -?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?, the only spelling a
// number-typed XML leaf is read from (C-18).
func isJSONNumber(text string) bool {
	i := 0
	if i < len(text) && text[i] == '-' {
		i++
	}
	if !scanJSONDigits(text, &i) {
		return false
	}
	if i < len(text) && text[i] == '.' {
		i++
		if !scanDigitRun(text, &i) {
			return false
		}
	}
	if i < len(text) && (text[i] == 'e' || text[i] == 'E') {
		i++
		if i < len(text) && (text[i] == '+' || text[i] == '-') {
			i++
		}
		if !scanDigitRun(text, &i) {
			return false
		}
	}
	return i == len(text)
}

// scanJSONDigits consumes 0 or [1-9][0-9]* at *i.
func scanJSONDigits(text string, i *int) bool {
	if *i >= len(text) {
		return false
	}
	if text[*i] == '0' {
		*i++
		return true
	}
	if text[*i] < '1' || text[*i] > '9' {
		return false
	}
	return scanDigitRun(text, i)
}

// scanDigitRun consumes one or more decimal digits at *i.
func scanDigitRun(text string, i *int) bool {
	start := *i
	for *i < len(text) && text[*i] >= '0' && text[*i] <= '9' {
		*i++
	}
	return *i > start
}

// integerDigits is the number of digits in the value of text when it is an
// integer leaf spelling (C-18), else 0: the sign does not count (D-28).
func integerDigits(text string) int {
	if !isJSONInteger(text) {
		return 0
	}
	return len(strings.TrimPrefix(text, "-"))
}

// pretype is pretypeScalar for the leaf at, which first applies D-9's integer
// digit limit (E-4a): a schema-typed integer leaf with too many digits is
// recorded to be refused, the first in document order, and left a string
// meanwhile.
func (r *xmlReader) pretype(text string, kind omnist.ScalarKind, at *elemInfo) (omnist.Scalar, bool) {
	if kind == omnist.KindInteger {
		if diag := r.checker.CheckIntDigits("", integerDigits(text)); diag != nil {
			if r.digitAt == nil {
				r.digitAt, r.digitDiag = at, diag
			}
			return omnist.Scalar{}, false
		}
	}
	return pretypeScalar(text, kind)
}

func pretypeScalar(text string, kind omnist.ScalarKind) (omnist.Scalar, bool) {
	switch kind {
	case omnist.KindBoolean:
		if text == "true" {
			return omnist.NewBooleanScalar(true), true
		}
		if text == "false" {
			return omnist.NewBooleanScalar(false), true
		}
		return omnist.Scalar{}, false
	case omnist.KindInteger:
		if !isJSONInteger(text) {
			return omnist.Scalar{}, false
		}
		bi, ok := new(big.Int).SetString(text, 10)
		if !ok {
			return omnist.Scalar{}, false
		}
		return omnist.NewIntegerScalar(bi), true
	case omnist.KindNumber:
		if !isJSONNumber(text) {
			return omnist.Scalar{}, false
		}
		// D-29: a number past binary64 reads as an infinity (ParseFloat
		// returns +/-Inf with ErrRange); underflow reads as +/-0.0.
		f, _ := strconv.ParseFloat(text, 64)
		return omnist.NewNumberScalar(f), true
	case omnist.KindDate:
		if !omnist.MatchesISOKind(text, omnist.TemporalDate) {
			return omnist.Scalar{}, false
		}
		return omnist.NewDateScalar(omnist.ParseISODate(text)), true
	case omnist.KindTime:
		if !omnist.MatchesISOKind(text, omnist.TemporalTime) {
			return omnist.Scalar{}, false
		}
		return omnist.NewTimeScalar(omnist.ParseISOTime(text)), true
	case omnist.KindDateTime:
		if !omnist.MatchesISOKind(text, omnist.TemporalDateTime) {
			return omnist.Scalar{}, false
		}
		return omnist.NewDateTimeScalar(omnist.ParseISODateTime(text)), true
	case omnist.KindString:
		return omnist.NewStringScalar(text), true
	default:
		return omnist.Scalar{}, false
	}
}
