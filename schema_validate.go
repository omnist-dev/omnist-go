package omnist

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file enforces the schema well-formedness rules of spec §3.3 (S-1
// through S-7) and the field-label rules of §5.4 on a Schema however it was
// built (§3.3: "These constraints govern a Schema however it was built").
//
// Schema, Record and Field are plain public structs, so a hand-written
// composite literal is NOT checked: it is unchecked until Validate is called,
// or until it is passed to an operation that validates (osd.Write does). The
// checked routes are NewRecord, NewSchema and Schema.Validate. A Schema
// returned by osd.Read, or by an algebra operation, is already well formed.
//
// Every failure is an omnist.Diagnostic (used as the error) carrying the
// schema.* code and the Schema path the spec fixes for it (§8.4.1, E-13 and
// E-30), exactly the error osd.Read reports for the same violation in text.
// Only the first violation, in the deterministic order documented on
// Validate, is returned (the spec leaves which violation is reported
// implementation-defined provided at least one is).
//
// Programmatic-only rules (spec v0.28.0-beta, no conformance vector, DIV-5):
// S-8 schema.invalid-name at "$" (the name appears in the message only, never
// in the path), S-22 schema.invalid-label at the record path (a label that is
// not valid UTF-8; the label is never put in a path) and S-23
// schema.unknown-record at "$" (an EnvOrder entry naming no record in Env).

func schemaDiag(path string, code Code, msg string) error {
	return Diagnostic{Path: path, Code: code, Message: msg, Severity: SeverityError}
}

// isReservedRecordName reports S-3: the seven scalar kind keywords and `any`,
// matched exactly and case-sensitively.
func isReservedRecordName(name string) bool {
	switch name {
	case "string", "integer", "number", "boolean", "date", "time", "datetime", "any":
		return true
	}
	return false
}

// isValidName reports S-8: [A-Za-z_][A-Za-z0-9_]*, ASCII only.
func isValidName(n string) bool {
	if n == "" {
		return false
	}
	for i := 0; i < len(n); i++ {
		c := n[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// invalidNameDiag is S-8's programmatic diagnostic: path "$", the (possibly
// malformed) name in the message only, quoted so non-printables are escaped.
func invalidNameDiag(what, name string) error {
	return schemaDiag("$", CodeSchemaInvalidName, fmt.Sprintf("%s %q does not match [A-Za-z_][A-Za-z0-9_]*", what, name))
}

// validateField checks one field of record rec: its label (§5.4: non-empty,
// no '[' or ']'; S-22: valid UTF-8; record path), then its type's
// nullability (S-7), the name a reference targets (S-8, path "$") and its
// cardinality (S-2) at the field path. Label rules run first, so a label that
// is not valid UTF-8 is never put in a path.
func validateField(rec string, f Field) error {
	if f.Label == "" {
		return schemaDiag(rec, CodeSchemaEmptyLabel, "field label must not be empty")
	}
	if strings.ContainsAny(f.Label, "[]") {
		return schemaDiag(rec, CodeSchemaBracketInLabel, "field label must not contain '[' or ']'")
	}
	if !utf8.ValidString(f.Label) {
		return schemaDiag(rec, CodeSchemaInvalidLabel, fmt.Sprintf("field label %q is not valid UTF-8", f.Label))
	}
	path := rec + "." + f.Label
	if !f.Cardinality.Unbounded && f.Cardinality.Max < f.Cardinality.Min {
		return schemaDiag(path, CodeSchemaInvalidCardinality, "invalid cardinality range")
	}
	switch f.Type.Kind {
	case TypeRefKind:
		if !isValidName(f.Type.RefName) {
			return invalidNameDiag("reference target", f.Type.RefName)
		}
		if f.Type.Nullable {
			return schemaDiag(path, CodeSchemaNullableRef, "'?' cannot apply to a reference; use cardinality [0,1] instead")
		}
	case TypeAnyKind:
		if f.Type.Nullable {
			return schemaDiag(path, CodeSchemaNullableAny, "`any` already includes null; `any?` is not valid")
		}
	}
	return nil
}

// validateRecord checks everything about one record that does not need the
// rest of the schema: S-8 (name), S-3 (reserved name), per-field rules, and
// S-5 (unique labels, reported at the record path).
func validateRecord(r *Record) error {
	if !isValidName(r.Name) {
		return invalidNameDiag("record name", r.Name)
	}
	if isReservedRecordName(r.Name) {
		return schemaDiag(r.Name, CodeSchemaReservedName, fmt.Sprintf("record name %q is reserved", r.Name))
	}
	seen := make(map[string]bool, len(r.Fields))
	for _, f := range r.Fields {
		if err := validateField(r.Name, f); err != nil {
			return err
		}
		if seen[f.Label] {
			return schemaDiag(r.Name, CodeSchemaDuplicateField, fmt.Sprintf("field %q is already defined in record %q", f.Label, r.Name))
		}
		seen[f.Label] = true
	}
	return nil
}

// Validate reports whether s is well formed per spec §3.3 and §5.4, returning
// nil or the first violation as an omnist.Diagnostic with the schema.* code
// and Schema path the spec assigns. Checks run in this order: for each entry
// of EnvOrder, S-23 (schema.unknown-record at "$" when Env holds no record
// under that name), S-8 (schema.invalid-name at "$"), S-4
// (schema.duplicate-record), S-3 (schema.reserved-name), then each field in
// declaration order for an empty label, a bracket in the label, a label that
// is not valid UTF-8 (S-22, schema.invalid-label at the record path), a
// reference target that is not a valid name (S-8), an invalid cardinality
// (S-2), a nullable reference or `any` (S-7) and a duplicate label (S-5);
// then S-1 (schema.no-root for an empty Root, schema.unknown-type at "$" for
// a root naming no record); then S-6 (a reference naming no record,
// schema.unknown-type at "R.a").
//
// Cardinality [0,0] is accepted: the model represents it (§3.4, S-15), and
// osd.Write refuses it (OSD-16).
func (s Schema) Validate() error {
	seen := make(map[string]bool, len(s.EnvOrder))
	for _, name := range s.EnvOrder {
		r := s.Env[name]
		if r == nil {
			return schemaDiag("$", CodeSchemaUnknownRecord, fmt.Sprintf("record ordering names %q, which is not a record in the environment", name))
		}
		if !isValidName(name) {
			return invalidNameDiag("record name", name)
		}
		if seen[name] {
			return schemaDiag(name, CodeSchemaDuplicateRecord, fmt.Sprintf("record %q is already defined", name))
		}
		seen[name] = true
		if err := validateRecord(r); err != nil {
			return err
		}
	}
	if s.Root == "" {
		return schemaDiag("$", CodeSchemaNoRoot, "a schema must declare a root")
	}
	if s.Env[s.Root] == nil {
		return schemaDiag("$", CodeSchemaUnknownType, fmt.Sprintf("root %q does not resolve to a defined record", s.Root))
	}
	for _, name := range s.EnvOrder {
		r := s.Env[name]
		for _, f := range r.Fields {
			if f.Type.Kind == TypeRefKind && s.Env[f.Type.RefName] == nil {
				return schemaDiag(r.Name+"."+f.Label, CodeSchemaUnknownType, fmt.Sprintf("unknown type %q", f.Type.RefName))
			}
		}
	}
	return nil
}

// NewRecord builds a Record and checks the rules that concern the record
// alone: S-3 (reserved name), the label rules, S-2, S-7 and S-5. On failure it
// returns a nil Record and the omnist.Diagnostic Validate would report. Record
// literals remain legal and unchecked; see the file comment.
func NewRecord(name string, fields ...Field) (*Record, error) {
	r := &Record{Name: name, Fields: fields}
	if err := validateRecord(r); err != nil {
		return nil, err
	}
	return r, nil
}

// NewSchema builds a Schema with the given root record name and records, in
// that declaration order (EnvOrder), then checks the whole schema with
// Validate: S-1 (root present and defined), S-4 (unique names) and S-6
// (references resolve) as well as every record-level rule. On failure it
// returns the zero Schema and the first Diagnostic. A nil record is
// ignored.
func NewSchema(root string, records ...*Record) (Schema, error) {
	s := Schema{Root: root, Env: make(map[string]*Record, len(records))}
	for _, r := range records {
		if r == nil {
			continue
		}
		if _, dup := s.Env[r.Name]; !dup {
			s.Env[r.Name] = r
		}
		s.EnvOrder = append(s.EnvOrder, r.Name)
	}
	if err := s.Validate(); err != nil {
		return Schema{}, err
	}
	return s, nil
}
