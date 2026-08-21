package xml

import (
	encxml "encoding/xml"
	"errors"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// Read parses XML source text into an omnist.Document without a schema.
//
// Every leaf arrives as a string (spec §7.1, docs/formats/xml.md). Callers
// with an OSD schema should use ReadWithSchema to pre-type leaves into numeric,
// boolean, and temporal scalars per omnist-spec#44.
func Read(src string, limits omnist.Limits) (omnist.Document, error) {
	return ReadWithSchema(src, nil, limits)
}

// ReadWithSchema parses XML source text into an omnist.Document.
//
// If schema is non-nil, text leaves are pre-typed into boolean, integer, number,
// or temporal scalars when the schema's declared field type calls for it (spec §7.1,
// omnist-spec#44). Leaves that do not parse value-exactly remain strings and fall
// through to normal stage-2 diagnostics.
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
// # Attribute and namespace-prefix dropping
//
// Per docs/formats/xml.md ("Attributes and namespace prefixes are
// dropped"), attributes leave no trace in the resulting omnist.Document, and any
// StartElement/EndElement Name.Space prefix is discarded, keeping only
// Name.Local as the edge label.
func ReadWithSchema(src string, schema *omnist.Schema, limits omnist.Limits) (omnist.Document, error) {
	if len(src) == 0 {
		return omnist.Document{}, &omnist.ParseError{
			Path:    "1:0",
			Code:    omnist.CodeParseUnexpectedToken,
			Message: "unexpected end of input",
		}
	}
	r := &xmlReader{
		dec:     encxml.NewDecoder(strings.NewReader(src)),
		checker: omnist.NewLimitChecker(limits),
		schema:  schema,
	}
	label, node, isLeaf, leafText, err := r.readRoot()
	if err != nil {
		return omnist.Document{}, err
	}
	if err := r.checkTrailing(); err != nil {
		return omnist.Document{}, err
	}
	root := omnist.NewNode()
	if isLeaf {
		if r.schema != nil {
			if rootRec := r.schema.Env[r.schema.Root]; rootRec != nil {
				if f := findField(rootRec, label); f != nil && f.Type.Kind == omnist.TypeScalarKind {
					if sc, ok := pretypeScalar(leafText, f.Type.ScalarKind); ok {
						root.AddValue(label, omnist.ScalarValue(sc))
						return omnist.NodeDocument(root), nil
					}
				}
			}
		}
		root.AddValue(label, omnist.ScalarValue(omnist.NewStringScalar(leafText)))
	} else {
		root.AddNode(label, node)
	}
	return omnist.NodeDocument(root), nil
}

type xmlReader struct {
	dec     *encxml.Decoder
	checker *omnist.LimitChecker
	schema  *omnist.Schema
}

// readRoot finds the single document element, consumes it via
// readElementBody, and returns its local name and body. Leading
// whitespace, comments, and processing instructions are skipped. Any
// non-whitespace text before the root element is rejected with
// omnist.CodeParseUnexpectedToken.
func (r *xmlReader) readRoot() (label string, node *omnist.Node, isLeaf bool, leafText string, err error) {
	for {
		tok, err := r.next()
		if err != nil {
			return "", nil, false, "", err
		}
		switch t := tok.(type) {
		case encxml.StartElement:
			label = t.Name.Local
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
			node, isLeaf, leafText, err = r.readElementBody(currentRec)
			return label, node, isLeaf, leafText, err
		case encxml.CharData:
			if len(strings.TrimSpace(string(t))) != 0 {
				return "", nil, false, "", r.errHere(omnist.CodeParseUnexpectedToken, "unexpected text outside the document element")
			}
			// insignificant whitespace before the root element: skip
		default: // encxml.ProcInst, encxml.Comment, encxml.Directive
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
				return r.errHere(omnist.CodeParseTrailingContent, "content remains after the document element")
			}
		case encxml.ProcInst, encxml.Comment, encxml.Directive:
			// trailing prolog-like content: skip
		default:
			return r.errHere(omnist.CodeParseTrailingContent, "content remains after the document element")
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
// failure reports omnist.CodeParseUnexpectedToken with the decoder's own message,
// positioned by its InputOffset.
func (r *xmlReader) wrapDecodeErr(err error) error {
	if errors.Is(err, io.EOF) {
		return r.errHere(omnist.CodeParseUnexpectedToken, "unexpected end of input")
	}
	return r.errHere(omnist.CodeParseUnexpectedToken, err.Error())
}

// errHere builds a *omnist.ParseError positioned at the decoder's current byte
// offset, translated to a 1-based line:col pair ? reusing offsetToLineCol
// (json_reader.go), which is format-agnostic (it walks raw text bytes, not
// anything JSON-specific).
func (r *xmlReader) errHere(code omnist.Code, msg string) error {
	return &omnist.ParseError{Path: r.pathHere(), Code: code, Message: msg}
}

func (r *xmlReader) pathHere() string {
	line, col := r.dec.InputPos()
	return strconv.Itoa(line) + ":" + strconv.Itoa(col)
}

// readElementBody reads one element's children up to and including the matching
// EndElement.
func (r *xmlReader) readElementBody(currentRec *omnist.Record) (node *omnist.Node, isLeaf bool, leafText string, err error) {
	var text strings.Builder
	var children *omnist.Node

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
			return children, false, "", nil
		case encxml.CharData:
			text.Write(t)
		case encxml.StartElement:
			if children == nil {
				children = omnist.NewNode()
			}
			label := t.Name.Local
			f := findField(currentRec, label)
			var childRec *omnist.Record
			if f != nil && f.Type.Kind == omnist.TypeRefKind && r.schema != nil {
				childRec = r.schema.Env[f.Type.RefName]
			}
			childNode, childIsLeaf, childText, err := r.readChild(childRec)
			if err != nil {
				return nil, false, "", err
			}
			if childIsLeaf {
				if f != nil && f.Type.Kind == omnist.TypeScalarKind {
					if sc, ok := pretypeScalar(childText, f.Type.ScalarKind); ok {
						children.AddValue(label, omnist.ScalarValue(sc))
						continue
					}
				}
				children.AddValue(label, omnist.ScalarValue(omnist.NewStringScalar(childText)))
			} else {
				children.AddNode(label, childNode)
			}
		case encxml.ProcInst, encxml.Comment, encxml.Directive:
			// ignored wherever it appears
		}
	}
}

// readChild reads one non-root element, enforcing MaxDepth/MaxNodes via
// the shared omnist.LimitChecker.
func (r *xmlReader) readChild(childRec *omnist.Record) (node *omnist.Node, isLeaf bool, leafText string, err error) {
	path := r.pathHere()
	if diag := r.checker.EnterNode(path); diag != nil {
		return nil, false, "", &omnist.ParseError{Path: diag.Path, Code: diag.Code, Message: diag.Message}
	}
	defer r.checker.LeaveNode()
	return r.readElementBody(childRec)
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
		bi, ok := new(big.Int).SetString(text, 10)
		if !ok {
			return omnist.Scalar{}, false
		}
		return omnist.NewIntegerScalar(bi), true
	case omnist.KindNumber:
		f, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return omnist.Scalar{}, false
		}
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
