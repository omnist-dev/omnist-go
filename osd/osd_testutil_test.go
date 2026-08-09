package osd

import (
	omnist "github.com/omnist-dev/omnist-go"
)

// This file holds the structural-equality helpers osd_writer_test.go's
// round-trip property test needs. Neither Schema nor Record exposes a
// public Equal method, so "Read(Write(schema)) == schema" needs its own
// deep-equality walk. Originally these lived in the root package's
// writer_testutil_test.go (shared with oml_writer_test.go's Document
// helpers); moving osd_writer_test.go into this package (issue #41)
// brought schemaEqual/typeEqual/fieldEqual/recordEqual along with it,
// since they were only ever used by this file.

func typeEqual(a, b omnist.Type) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case omnist.TypeScalarKind:
		return a.ScalarKind == b.ScalarKind && a.Nullable == b.Nullable
	case omnist.TypeRefKind:
		return a.RefName == b.RefName
	default: // omnist.TypeAnyKind
		return true
	}
}

func fieldEqual(a, b omnist.Field) bool {
	return a.Label == b.Label && typeEqual(a.Type, b.Type) && a.Cardinality == b.Cardinality
}

func recordEqual(a, b *omnist.Record) bool {
	if a.Name != b.Name || len(a.Fields) != len(b.Fields) {
		return false
	}
	for i := range a.Fields {
		if !fieldEqual(a.Fields[i], b.Fields[i]) {
			return false
		}
	}
	return true
}

func schemaEqual(a, b omnist.Schema) bool {
	if a.Root != b.Root {
		return false
	}
	if len(a.EnvOrder) != len(b.EnvOrder) {
		return false
	}
	for i := range a.EnvOrder {
		if a.EnvOrder[i] != b.EnvOrder[i] {
			return false
		}
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for name, rec := range a.Env {
		other, ok := b.Env[name]
		if !ok || !recordEqual(rec, other) {
			return false
		}
	}
	return true
}
