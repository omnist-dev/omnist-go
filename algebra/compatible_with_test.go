package algebra

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/omnist-dev/omnist-go"
)

// --- scalar_sub (§6.3) ---
//
// This is the one compatible_with_test.go case needing unexported access
// (scalarSub itself), so it stays in the internal "omnist" package; every
// other case moved to compatible_with_public_test.go (external
// "omnist_test" package) since it only needed mustParseOSD + exported API
// -- see referee_test.go's comment for why that split exists.

func TestScalarSub(t *testing.T) {
	cases := []struct {
		name string
		a, b omnist.Type
		want bool
	}{
		{"same kind true", omnist.ScalarType(omnist.KindString, false), omnist.ScalarType(omnist.KindString, false), true},
		{"integer sub number true", omnist.ScalarType(omnist.KindInteger, false), omnist.ScalarType(omnist.KindNumber, false), true},
		{"number not sub integer false", omnist.ScalarType(omnist.KindNumber, false), omnist.ScalarType(omnist.KindInteger, false), false},
		{"unrelated kinds false", omnist.ScalarType(omnist.KindString, false), omnist.ScalarType(omnist.KindInteger, false), false},
		{"nullable narrowing false", omnist.ScalarType(omnist.KindString, true), omnist.ScalarType(omnist.KindString, false), false},
		{"nullable widening true", omnist.ScalarType(omnist.KindString, false), omnist.ScalarType(omnist.KindString, true), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scalarSub(c.a, c.b)
			if got != c.want {
				t.Errorf("scalarSub(%+v, %+v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}


func BenchmarkCompatibleWithManyFields(b *testing.B) {
	sizes := []int{50, 200, 1000}
	for _, size := range sizes {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			fieldsA := make([]omnist.Field, size)
			fieldsB := make([]omnist.Field, size)
			for i := 0; i < size; i++ {
				label := fmt.Sprintf("field_%d", i)
				fieldsA[i] = omnist.Field{Label: label, Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()}
				fieldsB[i] = omnist.Field{Label: label, Type: omnist.ScalarType(omnist.KindString, false), Cardinality: omnist.DefaultCardinality()}
			}
			recA := &omnist.Record{Name: "Rec", Fields: fieldsA}
			recB := &omnist.Record{Name: "Rec", Fields: fieldsB}
			schemaA := omnist.Schema{Root: "Rec", Env: map[string]*omnist.Record{"Rec": recA}, EnvOrder: []string{"Rec"}}
			schemaB := omnist.Schema{Root: "Rec", Env: map[string]*omnist.Record{"Rec": recB}, EnvOrder: []string{"Rec"}}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = CompatibleWith(schemaA, schemaB)
			}
		})
	}
}

func TestFieldByLabelHelper(t *testing.T) {
	rec := &omnist.Record{
		Name: "Test",
		Fields: []omnist.Field{
			{Label: "a", Type: omnist.ScalarType(omnist.KindString, false)},
		},
	}
	if f, ok := fieldByLabel(rec, "a"); !ok || f.Label != "a" {
		t.Errorf("expected field a, got %v, ok=%v", f, ok)
	}
	if _, ok := fieldByLabel(rec, "missing"); ok {
		t.Errorf("expected not ok for missing field")
	}
	if _, ok := fieldByLabel(nil, "a"); ok {
		t.Errorf("expected not ok for nil record")
	}
}
