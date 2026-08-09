package osd

import (
	"fmt"

	omnist "github.com/omnist-dev/omnist-go"
)

// Read parses OSD source text into a Schema (spec ch.5, "text to
// Schema"). Unlike ReadOML, there is no Limits parameter: an OSD schema is
// authored, not attacker-controlled arbitrary data (spec §2.4/§4.7's
// safety limits exist for Documents built from untrusted input), and
// nothing in chapter 5 calls for a resource-cap concern of its own — a
// schema's size is bounded by the same source text a human wrote and
// re-reads, not by nesting depth of adversarial data.
//
// On failure the returned error is either:
//   - a *omnist.ParseError with a text-position ("line:col") Path, for a genuine
//     syntax failure (an unexpected token, an unterminated string, a
//     control character) — these happen before enough structure exists to
//     name a record or field; or
//   - an omnist.Diagnostic with a Schema-shaped Path ("RecordName", "RecordName.label",
//     or the whole-schema fallback "$"), for a schema.* well-formedness
//     failure (spec §3.3 S-1..S-7) — these happen once at least a partial
//     Schema exists to describe the location within.
//
// This split follows spec §8.4 exactly: "A parse.* diagnostic's path MUST
// be a text-position path. A ... schema.* ... diagnostic's path MUST be a
// Document or Schema path ... never a text-position path."
func Read(text string) (omnist.Schema, error) {
	p := &osdParser{lex: newOSDLexer(text)}
	if err := p.advance(); err != nil {
		return omnist.Schema{}, err
	}
	return p.parseSchema()
}

// schemaError constructs the omnist.Diagnostic (used as an error) a schema.*
// well-formedness failure reports, per spec §8.4's omnist.Schema path forms.
func schemaError(path string, code omnist.Code, msg string) error {
	return omnist.Diagnostic{Path: path, Code: code, Message: msg, Severity: omnist.SeverityError}
}

type osdParser struct {
	lex *osdLexer
	cur osdToken
}

func (p *osdParser) advance() *omnist.ParseError {
	tok, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
}

func (p *osdParser) errAt(t osdToken, code omnist.Code, msg string) *omnist.ParseError {
	return &omnist.ParseError{Line: t.line, Col: t.col, Path: fmt.Sprintf("%d:%d", t.line, t.col), Code: code, Message: msg}
}

// parseSchema implements `schema = *( record-def / root-def )` (spec
// §5.4/§5.8), then enforces the well-formedness constraints that sit above
// the grammar (S-1, S-4, S-6) once the whole schema is assembled: exactly
// one root, unique record names (checked incrementally as each record-def
// is parsed, so the duplicate is reported as soon as it is seen), and every
// reference — including the root — resolving to a defined record.
func (p *osdParser) parseSchema() (omnist.Schema, error) {
	schema := omnist.Schema{Env: map[string]*omnist.Record{}}
	var recordOrder []string
	rootSet := false

	for p.cur.kind != osdTokEOF {
		if p.cur.kind != osdTokName {
			return omnist.Schema{}, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected 'record' or 'root'")
		}
		switch p.cur.text {
		case "record":
			rec, err := p.parseRecordDef()
			if err != nil {
				return omnist.Schema{}, err
			}
			if _, exists := schema.Env[rec.Name]; exists {
				return omnist.Schema{}, schemaError(rec.Name, omnist.CodeSchemaDuplicateRecord, fmt.Sprintf("record %q is already defined", rec.Name))
			}
			schema.Env[rec.Name] = rec
			recordOrder = append(recordOrder, rec.Name)
		case "root":
			name, err := p.parseRootDef()
			if err != nil {
				return omnist.Schema{}, err
			}
			// spec §5.8 / chapter 9 divergence-ledger D-2: a second `root`
			// declaration's behavior is an explicitly acknowledged open
			// item (the spec does not yet mandate erroring, though a future
			// spec-level change may make it one). This implementation
			// picks "first root wins" — the earliest `root` declaration in
			// source order is authoritative and later ones are parsed (so
			// their own syntax is still validated) but otherwise ignored.
			// This is one of the two defensible choices the issue names;
			// it is not a blocking ambiguity, per the issue's own
			// instruction not to treat D-2 as one.
			if !rootSet {
				schema.Root = name
				rootSet = true
			}
		default:
			return omnist.Schema{}, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected 'record' or 'root'")
		}
	}

	if !rootSet {
		return omnist.Schema{}, schemaError("$", omnist.CodeSchemaNoRoot, "a schema must declare a root")
	}
	if _, ok := schema.Env[schema.Root]; !ok {
		return omnist.Schema{}, schemaError("$", omnist.CodeSchemaUnknownType, fmt.Sprintf("root %q does not resolve to a defined record", schema.Root))
	}

	// S-6: every Ref type (forward references and mutual recursion both
	// legal, so this is checked only after every record-def has been
	// parsed, not while parsing each one) must resolve in env. Iterating
	// recordOrder rather than schema.Env directly keeps the first-error
	// reported deterministic across runs, since Go map iteration order is
	// randomized.
	for _, name := range recordOrder {
		rec := schema.Env[name]
		for _, f := range rec.Fields {
			if f.Type.Kind != omnist.TypeRefKind {
				continue
			}
			if _, ok := schema.Env[f.Type.RefName]; !ok {
				return omnist.Schema{}, schemaError(rec.Name+"."+f.Label, omnist.CodeSchemaUnknownType, fmt.Sprintf("unknown type %q", f.Type.RefName))
			}
		}
	}

	schema.EnvOrder = recordOrder
	return schema, nil
}

// scalarKeyword reports whether name is one of the seven scalar-name
// keywords (spec §5.6) and, if so, which omnist.ScalarKind it names.
func scalarKeyword(name string) (omnist.ScalarKind, bool) {
	switch name {
	case "string":
		return omnist.KindString, true
	case "integer":
		return omnist.KindInteger, true
	case "number":
		return omnist.KindNumber, true
	case "boolean":
		return omnist.KindBoolean, true
	case "date":
		return omnist.KindDate, true
	case "time":
		return omnist.KindTime, true
	case "datetime":
		return omnist.KindDateTime, true
	}
	return 0, false
}

// parseRecordDef implements `record-def = "record" name "{" [ field
// *( "," field ) [ "," ] ] "}"` (spec §5.4), enforcing S-3 (reserved
// names) as soon as the name is read and S-5 (unique field labels) as
// each field is added.
func (p *osdParser) parseRecordDef() (*omnist.Record, error) {
	if err := p.advance(); err != nil { // consume 'record'
		return nil, err
	}
	if p.cur.kind != osdTokName {
		return nil, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected a record name")
	}
	name := p.cur.text
	if err := p.advance(); err != nil {
		return nil, err
	}

	if _, isScalar := scalarKeyword(name); isScalar {
		return nil, schemaError(name, omnist.CodeSchemaReservedName, fmt.Sprintf("record name %q is a reserved scalar keyword", name))
	}
	if name == "any" {
		return nil, schemaError(name, omnist.CodeSchemaReservedName, "record name \"any\" is reserved")
	}

	if p.cur.kind != osdTokLBrace {
		return nil, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected '{' after record name")
	}
	if err := p.advance(); err != nil { // consume '{'
		return nil, err
	}

	rec := &omnist.Record{Name: name}
	seenLabels := map[string]bool{}
	for p.cur.kind != osdTokRBrace {
		if p.cur.kind == osdTokEOF {
			return nil, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "unexpected end of input, expected '}'")
		}
		field, err := p.parseField(name)
		if err != nil {
			return nil, err
		}
		if seenLabels[field.Label] {
			// Path is the record itself (RecordName), not
			// RecordName.field, per §8.4's omnist.Schema-path form: this is a
			// record-level diagnostic (the record has two fields with the
			// same label), not a field-level one — there's no single field
			// it names, since it's the duplication itself that's wrong.
			return nil, schemaError(name, omnist.CodeSchemaDuplicateField, fmt.Sprintf("field %q is already defined in record %q", field.Label, name))
		}
		seenLabels[field.Label] = true
		rec.Fields = append(rec.Fields, field)

		switch p.cur.kind {
		case osdTokComma:
			if err := p.advance(); err != nil {
				return nil, err
			}
		case osdTokRBrace:
			// loop exits on the next condition check
		default:
			return nil, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected ',' or '}' after field")
		}
	}
	if err := p.advance(); err != nil { // consume '}'
		return nil, err
	}
	return rec, nil
}

// parseRootDef implements `root-def = "root" name` (spec §5.8).
func (p *osdParser) parseRootDef() (string, error) {
	if err := p.advance(); err != nil { // consume 'root'
		return "", err
	}
	if p.cur.kind != osdTokName {
		return "", p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected a record name after 'root'")
	}
	name := p.cur.text
	if err := p.advance(); err != nil {
		return "", err
	}
	return name, nil
}

// parseField implements `field = string [ cardinality ] ":" type` (spec
// §5.4). recordName is the enclosing record's name, used to build the
// omnist.Schema-shaped path of any diagnostic raised while parsing this field.
//
// Per the quoting rule (§5.2), the label MUST be a quoted string; an
// unquoted name here is specifically schema.unquoted-label (spec §5.10's
// `record R{a:string}` worked example), not a generic syntax error.
func (p *osdParser) parseField(recordName string) (omnist.Field, error) {
	if p.cur.kind != osdTokString {
		if p.cur.kind == osdTokName {
			return omnist.Field{}, schemaError(recordName, omnist.CodeSchemaUnquotedLabel, "expected a quoted field name")
		}
		return omnist.Field{}, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected a quoted field name")
	}
	label := p.cur.strVal
	if err := p.advance(); err != nil {
		return omnist.Field{}, err
	}

	card := omnist.DefaultCardinality()
	if p.cur.kind == osdTokLBracket {
		c, err := p.parseCardinality(recordName, label)
		if err != nil {
			return omnist.Field{}, err
		}
		card = c
	}

	if p.cur.kind != osdTokColon {
		return omnist.Field{}, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected ':' after field label/cardinality")
	}
	if err := p.advance(); err != nil {
		return omnist.Field{}, err
	}

	typ, err := p.parseType(recordName, label)
	if err != nil {
		return omnist.Field{}, err
	}
	return omnist.Field{Label: label, Type: typ, Cardinality: card}, nil
}

// parseCardinality implements `cardinality = "[" ( int [ "," [ int ] ] /
// "," [ int ] ) "]"` (spec §5.5) plus the three checks that sit above the
// grammar: whole-number bounds (via parseCardinalityBound), non-negative
// minimum, and max >= min. path names the field this cardinality belongs
// to, for any schema.* diagnostic raised here.
func (p *osdParser) parseCardinality(recordName, label string) (omnist.Cardinality, error) {
	path := recordName + "." + label

	if err := p.advance(); err != nil { // consume '['
		return omnist.Cardinality{}, err
	}

	if p.cur.kind == osdTokRBracket {
		if err := p.advance(); err != nil {
			return omnist.Cardinality{}, err
		}
		return omnist.Cardinality{}, schemaError(path, omnist.CodeSchemaEmptyCardinality, "cardinality must not be empty")
	}

	if p.cur.kind == osdTokComma {
		// "," [ int ] "]"  ->  min = 0
		if err := p.advance(); err != nil {
			return omnist.Cardinality{}, err
		}
		if p.cur.kind == osdTokRBracket {
			if err := p.advance(); err != nil {
				return omnist.Cardinality{}, err
			}
			return omnist.Cardinality{Min: 0, Unbounded: true}, nil
		}
		maxV, err := p.parseCardinalityBound(path)
		if err != nil {
			return omnist.Cardinality{}, err
		}
		if err := p.expectRBracket(); err != nil {
			return omnist.Cardinality{}, err
		}
		if maxV < 0 {
			return omnist.Cardinality{}, schemaError(path, omnist.CodeSchemaInvalidCardinality, "cardinality bound must be non-negative")
		}
		return omnist.Cardinality{Min: 0, Max: uint64(maxV)}, nil
	}

	// int [ "," [ int ] ] "]"
	firstV, err := p.parseCardinalityBound(path)
	if err != nil {
		return omnist.Cardinality{}, err
	}

	switch p.cur.kind {
	case osdTokRBracket:
		if err := p.advance(); err != nil {
			return omnist.Cardinality{}, err
		}
		if firstV < 0 {
			return omnist.Cardinality{}, schemaError(path, omnist.CodeSchemaInvalidCardinality, "cardinality bound must be non-negative")
		}
		return omnist.Cardinality{Min: uint64(firstV), Max: uint64(firstV)}, nil
	case osdTokComma:
		if err := p.advance(); err != nil {
			return omnist.Cardinality{}, err
		}
		if p.cur.kind == osdTokRBracket {
			if err := p.advance(); err != nil {
				return omnist.Cardinality{}, err
			}
			if firstV < 0 {
				return omnist.Cardinality{}, schemaError(path, omnist.CodeSchemaInvalidCardinality, "cardinality bound must be non-negative")
			}
			return omnist.Cardinality{Min: uint64(firstV), Unbounded: true}, nil
		}
		secondV, err := p.parseCardinalityBound(path)
		if err != nil {
			return omnist.Cardinality{}, err
		}
		if err := p.expectRBracket(); err != nil {
			return omnist.Cardinality{}, err
		}
		if firstV < 0 || secondV < 0 || secondV < firstV {
			return omnist.Cardinality{}, schemaError(path, omnist.CodeSchemaInvalidCardinality, "invalid cardinality range")
		}
		return omnist.Cardinality{Min: uint64(firstV), Max: uint64(secondV)}, nil
	default:
		return omnist.Cardinality{}, p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected ',' or ']' in cardinality")
	}
}

func (p *osdParser) expectRBracket() *omnist.ParseError {
	if p.cur.kind != osdTokRBracket {
		return p.errAt(p.cur, omnist.CodeParseUnexpectedToken, "expected ']' after cardinality")
	}
	return p.advance()
}

// parseCardinalityBound reads one cardinality bound token: either a whole
// number (accepted, possibly negative — spec §5.5's note that a negative
// bound is caught here at construction time, not at the token boundary),
// or a non-integer bound like "1.5", which is rejected right here with
// schema.non-integer-cardinality rather than the generic syntax error.
func (p *osdParser) parseCardinalityBound(path string) (int64, error) {
	t := p.cur
	switch t.kind {
	case osdTokInt:
		if err := p.advance(); err != nil {
			return 0, err
		}
		return t.intVal, nil
	case osdTokNumber:
		if err := p.advance(); err != nil {
			return 0, err
		}
		return 0, schemaError(path, omnist.CodeSchemaNonIntegerCardinality, "cardinality bound must be a whole number")
	default:
		return 0, p.errAt(t, omnist.CodeParseUnexpectedToken, "expected a cardinality bound")
	}
}

// parseType implements `type = scalar-type / any-type / ref-type` (spec
// §5.6). path names the field this type belongs to, for any schema.*
// diagnostic raised here (nullable-any, nullable-ref). Reference
// resolution (S-6) is deferred to parseSchema, once every record-def has
// been seen, since forward references and mutual recursion are legal.
func (p *osdParser) parseType(recordName, label string) (omnist.Type, error) {
	t := p.cur
	if t.kind != osdTokName {
		// Per the quoting rule (§5.2), a quoted string in type position is
		// an error, symmetric with the reverse case (schema.unquoted-label
		// for an unquoted field name). schema.quoted-type (added upstream
		// via omnist-spec#35) is the dedicated code for it; any other
		// non-name token in type position is still a generic syntax error.
		if t.kind == osdTokString {
			return omnist.Type{}, schemaError(recordName, omnist.CodeSchemaQuotedType, "expected a bare type name, not a quoted string")
		}
		return omnist.Type{}, p.errAt(t, omnist.CodeParseUnexpectedToken, "expected a type name")
	}
	name := t.text
	if err := p.advance(); err != nil {
		return omnist.Type{}, err
	}
	path := recordName + "." + label

	if kind, ok := scalarKeyword(name); ok {
		nullable := false
		if p.cur.kind == osdTokQuestion {
			nullable = true
			if err := p.advance(); err != nil {
				return omnist.Type{}, err
			}
		}
		return omnist.ScalarType(kind, nullable), nil
	}

	if name == "any" {
		if p.cur.kind == osdTokQuestion {
			return omnist.Type{}, schemaError(path, omnist.CodeSchemaNullableAny, "`any` already includes null; `any?` is not valid")
		}
		return omnist.AnyType(), nil
	}

	// Reference: any other name (spec §5.6), including capitalized "Any",
	// which is deliberately not the any-type — only the exact lowercase
	// spelling is (spec §5.6's own worked note).
	if p.cur.kind == osdTokQuestion {
		return omnist.Type{}, schemaError(path, omnist.CodeSchemaNullableRef, "'?' cannot apply to a reference; use cardinality [0,1] instead")
	}
	return omnist.RefType(name), nil
}
