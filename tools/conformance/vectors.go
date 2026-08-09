// Package conformance is the Track 2 (JSON-vector) conformance harness for
// omnist-go, per issue #31 and vendor/omnist-spec/docs/08-conformance-and-
// errors.md §8.5. It walks vendor/omnist-spec/test-suite/*.json, dispatches
// each vector by its "operation" field, calls the corresponding real
// omnist-go function, and compares the result against the vector's
// "expect" using the referee (github.com/omnist-dev/omnist-go's
// DocumentsEqual/SchemasEqual).
package conformance

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	omnist "github.com/omnist-dev/omnist-go"
)

// Vector is one JSON-vector test case, per spec §8.5.1's common envelope.
type Vector struct {
	Name      string          `json:"name"`
	Spec      string          `json:"spec"`
	Operation string          `json:"operation"`
	Purpose   string          `json:"purpose"`
	Input     json.RawMessage `json:"input"`
	Expect    json.RawMessage `json:"expect"`
	Comment   string          `json:"comment,omitempty"`
}

// vectorFile is the top-level shape of a test-suite/*.json file: a single
// "vectors" array.
type vectorFile struct {
	Vectors []Vector `json:"vectors"`
}

// LoadVectorFile parses one test-suite/*.json file's vectors.
func LoadVectorFile(data []byte) ([]Vector, error) {
	var f vectorFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode vector file: %w", err)
	}
	return f.Vectors, nil
}

// --- §8.5.4 canonical Document JSON encoding -> omnist.Document ---
//
// This is deliberately independent of formats/json/json_reader.go's json.Read: json.Read
// parses actual JSON-*format* documents (JSON's own map/array shape,
// requiring the array-becomes-repeated-edges desugaring JSON's format page
// describes). The vector suite's canonical encoding is a different,
// explicit thing per §8.5.4 precisely so a vector file does not depend on
// the reader's JSON library to decide whether "1" is an integer or a
// number, or on desugaring rules a vector might be testing in the first
// place (e.g. formats-json/json.json's own vectors test json.Read; a
// materialize vector's "document" input must not silently re-exercise
// json.Read's array-desugaring on the way in).

// canonNode is the raw shape of a canonical-encoding node or scalar.
type canonNode struct {
	Edges  [][2]json.RawMessage `json:"edges"`
	Scalar *canonScalar         `json:"scalar"`
}

type canonScalar struct {
	Kind  *string         `json:"kind"`
	Value json.RawMessage `json:"value"`
}

// DecodeCanonicalDocument decodes one §8.5.4-encoded Document (a bare
// canonNode at the top: either {"edges": [...]} or {"scalar": {...}}).
func DecodeCanonicalDocument(raw json.RawMessage) (omnist.Document, error) {
	var cn canonNode
	if err := json.Unmarshal(raw, &cn); err != nil {
		return omnist.Document{}, fmt.Errorf("decode canonical document: %w", err)
	}
	if cn.Scalar != nil {
		v, err := decodeCanonicalValue(*cn.Scalar)
		if err != nil {
			return omnist.Document{}, err
		}
		return omnist.ValueDocument(v), nil
	}
	n, err := decodeCanonicalEdges(cn.Edges)
	if err != nil {
		return omnist.Document{}, err
	}
	return omnist.NodeDocument(n), nil
}

func decodeCanonicalTarget(raw json.RawMessage) (omnist.Target, error) {
	var cn canonNode
	if err := json.Unmarshal(raw, &cn); err != nil {
		return omnist.Target{}, fmt.Errorf("decode canonical target: %w", err)
	}
	if cn.Scalar != nil {
		v, err := decodeCanonicalValue(*cn.Scalar)
		if err != nil {
			return omnist.Target{}, err
		}
		return omnist.ValueTarget(v), nil
	}
	n, err := decodeCanonicalEdges(cn.Edges)
	if err != nil {
		return omnist.Target{}, err
	}
	return omnist.NodeTarget(n), nil
}

func decodeCanonicalEdges(edges [][2]json.RawMessage) (*omnist.Node, error) {
	n := omnist.NewNode()
	for _, pair := range edges {
		var label string
		if err := json.Unmarshal(pair[0], &label); err != nil {
			return nil, fmt.Errorf("decode edge label: %w", err)
		}
		target, err := decodeCanonicalTarget(pair[1])
		if err != nil {
			return nil, err
		}
		if node, ok := target.Node(); ok {
			n.AddNode(label, node)
		} else {
			v, _ := target.Value()
			n.AddValue(label, v)
		}
	}
	return n, nil
}

// decodeCanonicalValue decodes a {"kind": K, "value": V} pair. Per §8.5.4,
// null is {"scalar": {"kind": null, "value": null}} -- Kind is a JSON null
// literal, not the four-character string "null", which is why Kind is
// *string here rather than string.
func decodeCanonicalValue(cs canonScalar) (omnist.Value, error) {
	if cs.Kind == nil {
		return omnist.NullValue(), nil
	}
	switch *cs.Kind {
	case "string":
		var s string
		if err := json.Unmarshal(cs.Value, &s); err != nil {
			return omnist.Value{}, fmt.Errorf("decode string scalar: %w", err)
		}
		return omnist.ScalarValue(omnist.NewStringScalar(s)), nil
	case "integer":
		i, err := decodeCanonicalInteger(cs.Value)
		if err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewIntegerScalar(i)), nil
	case "number":
		f, err := decodeCanonicalNumber(cs.Value)
		if err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewNumberScalar(f)), nil
	case "boolean":
		var b bool
		if err := json.Unmarshal(cs.Value, &b); err != nil {
			return omnist.Value{}, fmt.Errorf("decode boolean scalar: %w", err)
		}
		return omnist.ScalarValue(omnist.NewBooleanScalar(b)), nil
	case "date":
		var s string
		if err := json.Unmarshal(cs.Value, &s); err != nil {
			return omnist.Value{}, fmt.Errorf("decode date scalar: %w", err)
		}
		d, err := parseISODate(s)
		if err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewDateScalar(d)), nil
	case "time":
		var s string
		if err := json.Unmarshal(cs.Value, &s); err != nil {
			return omnist.Value{}, fmt.Errorf("decode time scalar: %w", err)
		}
		tm, err := parseISOTime(s)
		if err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewTimeScalar(tm)), nil
	case "datetime":
		var s string
		if err := json.Unmarshal(cs.Value, &s); err != nil {
			return omnist.Value{}, fmt.Errorf("decode datetime scalar: %w", err)
		}
		dt, err := parseISODateTime(s)
		if err != nil {
			return omnist.Value{}, err
		}
		return omnist.ScalarValue(omnist.NewDateTimeScalar(dt)), nil
	default:
		return omnist.Value{}, fmt.Errorf("unknown canonical scalar kind %q", *cs.Kind)
	}
}

// decodeCanonicalInteger handles §8.5.4's "integer values are JSON numbers
// when they fit exactly and decimal strings otherwise."
func decodeCanonicalInteger(raw json.RawMessage) (*big.Int, error) {
	s := strings.TrimSpace(string(raw))
	s = strings.Trim(s, `"`)
	i, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("decode integer scalar: %q is not a valid integer literal", s)
	}
	return i, nil
}

func decodeCanonicalNumber(raw json.RawMessage) (float64, error) {
	s := strings.TrimSpace(string(raw))
	s = strings.Trim(s, `"`)
	switch s {
	case "nan", "NaN":
		return math.NaN(), nil
	case "inf", "Infinity", "+inf", "+Infinity":
		return math.Inf(1), nil
	case "-inf", "-Infinity":
		return math.Inf(-1), nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("decode number scalar: %w", err)
	}
	return f, nil
}

var (
	reISODate     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	reISOTime     = regexp.MustCompile(`^(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,9}))?)?(Z|[+-]\d{2}:\d{2})?$`)
	reISODateTime = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})T(.+)$`)
)

func parseISODate(s string) (omnist.DateValue, error) {
	m := reISODate.FindStringSubmatch(s)
	if m == nil {
		return omnist.DateValue{}, fmt.Errorf("decode date scalar: %q is not an ISO-8601 date", s)
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	return omnist.DateValue{Year: y, Month: mo, Day: d}, nil
}

func parseISOTime(s string) (omnist.TimeValue, error) {
	m := reISOTime.FindStringSubmatch(s)
	if m == nil {
		return omnist.TimeValue{}, fmt.Errorf("decode time scalar: %q is not an ISO-8601 time", s)
	}
	hh, _ := strconv.Atoi(m[1])
	mm, _ := strconv.Atoi(m[2])
	tv := omnist.TimeValue{Hour: hh, Minute: mm}
	if m[3] != "" {
		ss, _ := strconv.Atoi(m[3])
		tv.Second = ss
	}
	if m[4] != "" {
		padded := (m[4] + "000000000")[:9]
		n, _ := strconv.Atoi(padded)
		tv.Nanosecond = n
	}
	if m[5] != "" {
		if m[5] == "Z" {
			tv.HasOffset = true
			tv.OffsetSeconds = 0
		} else {
			sign := 1
			if m[5][0] == '-' {
				sign = -1
			}
			oh, _ := strconv.Atoi(m[5][1:3])
			om, _ := strconv.Atoi(m[5][4:6])
			tv.HasOffset = true
			tv.OffsetSeconds = sign * (oh*3600 + om*60)
		}
	}
	return tv, nil
}

func parseISODateTime(s string) (omnist.DateTimeValue, error) {
	m := reISODateTime.FindStringSubmatch(s)
	if m == nil {
		return omnist.DateTimeValue{}, fmt.Errorf("decode datetime scalar: %q is not an ISO-8601 datetime", s)
	}
	d, err := parseISODate(m[1])
	if err != nil {
		return omnist.DateTimeValue{}, err
	}
	t, err := parseISOTime(m[2])
	if err != nil {
		return omnist.DateTimeValue{}, err
	}
	return omnist.DateTimeValue{Date: d, Time: t}, nil
}
