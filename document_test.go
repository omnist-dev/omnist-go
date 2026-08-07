package omnist

import (
	"math/big"
	"testing"
)

func TestScalarKindString(t *testing.T) {
	cases := []struct {
		k    ScalarKind
		want string
	}{
		{KindString, "string"},
		{KindInteger, "integer"},
		{KindNumber, "number"},
		{KindBoolean, "boolean"},
		{KindDate, "date"},
		{KindTime, "time"},
		{KindDateTime, "datetime"},
		{ScalarKind(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("ScalarKind(%d).String() = %q, want %q", c.k, got, c.want)
		}
	}
}

func TestScalarConstructors(t *testing.T) {
	if s := NewStringScalar("hi"); s.Kind != KindString || s.Str != "hi" {
		t.Errorf("NewStringScalar: got %+v", s)
	}
	bi := big.NewInt(42)
	s := NewIntegerScalar(bi)
	if s.Kind != KindInteger || s.Int.Cmp(bi) != 0 {
		t.Errorf("NewIntegerScalar: got %+v", s)
	}
	// Mutating the caller's big.Int afterward must not affect the scalar
	// (constructor copies).
	bi.SetInt64(0)
	if s.Int.Cmp(big.NewInt(42)) != 0 {
		t.Errorf("NewIntegerScalar did not copy: got %v", s.Int)
	}
	if s := NewNumberScalar(1.5); s.Kind != KindNumber || s.Num != 1.5 {
		t.Errorf("NewNumberScalar: got %+v", s)
	}
	if s := NewBooleanScalar(true); s.Kind != KindBoolean || !s.Bool {
		t.Errorf("NewBooleanScalar: got %+v", s)
	}
	d := DateValue{Year: 2026, Month: 8, Day: 7}
	if s := NewDateScalar(d); s.Kind != KindDate || s.Date != d {
		t.Errorf("NewDateScalar: got %+v", s)
	}
	tm := TimeValue{Hour: 1, Minute: 2, Second: 3}
	if s := NewTimeScalar(tm); s.Kind != KindTime || s.Time != tm {
		t.Errorf("NewTimeScalar: got %+v", s)
	}
	dt := DateTimeValue{Date: d, Time: tm}
	if s := NewDateTimeScalar(dt); s.Kind != KindDateTime || s.DateTime != dt {
		t.Errorf("NewDateTimeScalar: got %+v", s)
	}
}

func TestScalarEqual(t *testing.T) {
	one := NewIntegerScalar(big.NewInt(1))
	oneAgain := NewIntegerScalar(big.NewInt(1))
	two := NewIntegerScalar(big.NewInt(2))
	oneAsNumber := NewNumberScalar(1.0)

	if !one.Equal(oneAgain) {
		t.Error("equal integers reported unequal")
	}
	if one.Equal(two) {
		t.Error("unequal integers reported equal")
	}
	// D-5: integer 1 and number 1.0 are NOT equal, despite same magnitude.
	if one.Equal(oneAsNumber) {
		t.Error("D-5 violated: integer 1 equal to number 1.0")
	}
	if oneAsNumber.Equal(one) {
		t.Error("D-5 violated (reversed): number 1.0 equal to integer 1")
	}

	str1 := NewStringScalar("a")
	str2 := NewStringScalar("a")
	str3 := NewStringScalar("b")
	if !str1.Equal(str2) || str1.Equal(str3) {
		t.Error("string equality broken")
	}

	num1 := NewNumberScalar(2.5)
	num2 := NewNumberScalar(2.5)
	num3 := NewNumberScalar(3.5)
	if !num1.Equal(num2) || num1.Equal(num3) {
		t.Error("number equality broken")
	}

	b1 := NewBooleanScalar(true)
	b2 := NewBooleanScalar(true)
	b3 := NewBooleanScalar(false)
	if !b1.Equal(b2) || b1.Equal(b3) {
		t.Error("boolean equality broken")
	}

	d1 := NewDateScalar(DateValue{2026, 1, 1})
	d2 := NewDateScalar(DateValue{2026, 1, 1})
	d3 := NewDateScalar(DateValue{2026, 1, 2})
	if !d1.Equal(d2) || d1.Equal(d3) {
		t.Error("date equality broken")
	}

	t1 := NewTimeScalar(TimeValue{Hour: 1})
	t2 := NewTimeScalar(TimeValue{Hour: 1})
	t3 := NewTimeScalar(TimeValue{Hour: 2})
	if !t1.Equal(t2) || t1.Equal(t3) {
		t.Error("time equality broken")
	}

	dt1 := NewDateTimeScalar(DateTimeValue{Date: DateValue{2026, 1, 1}})
	dt2 := NewDateTimeScalar(DateTimeValue{Date: DateValue{2026, 1, 1}})
	dt3 := NewDateTimeScalar(DateTimeValue{Date: DateValue{2026, 1, 2}})
	if !dt1.Equal(dt2) || dt1.Equal(dt3) {
		t.Error("datetime equality broken")
	}

	// Nil *big.Int handling (zero-value Scalar with Kind integer).
	zeroInt := Scalar{Kind: KindInteger}
	if !zeroInt.Equal(Scalar{Kind: KindInteger}) {
		t.Error("two nil-Int integer scalars should be equal")
	}
	if zeroInt.Equal(one) {
		t.Error("nil-Int scalar should not equal a real integer scalar")
	}

	// Unknown kind on both sides is never equal (default branch).
	unknown1 := Scalar{Kind: ScalarKind(99)}
	unknown2 := Scalar{Kind: ScalarKind(99)}
	if unknown1.Equal(unknown2) {
		t.Error("unknown kind should never report equal")
	}
}

func TestValueAndNullValue(t *testing.T) {
	n := NullValue()
	if !n.IsNull {
		t.Error("NullValue should have IsNull true")
	}
	v := ScalarValue(NewStringScalar("x"))
	if v.IsNull || v.Scalar.Str != "x" {
		t.Errorf("ScalarValue: got %+v", v)
	}
}

func TestTargetValueAndNode(t *testing.T) {
	v := ScalarValue(NewStringScalar("leaf"))
	vt := ValueTarget(v)
	if vt.IsNode() {
		t.Error("value target reported IsNode true")
	}
	if _, ok := vt.Node(); ok {
		t.Error("value target's Node() should report ok=false")
	}
	gotV, ok := vt.Value()
	if !ok || !gotV.Scalar.Equal(v.Scalar) {
		t.Errorf("value target's Value() = %+v, %v", gotV, ok)
	}

	child := NewNode()
	nt := NodeTarget(child)
	if !nt.IsNode() {
		t.Error("node target reported IsNode false")
	}
	if _, ok := nt.Value(); ok {
		t.Error("node target's Value() should report ok=false")
	}
	gotN, ok := nt.Node()
	if !ok || gotN != child {
		t.Errorf("node target's Node() = %v, %v", gotN, ok)
	}
}

func TestNodeTargetNilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NodeTarget(nil) should panic")
		}
	}()
	NodeTarget(nil)
}

func TestNodeBuildingPreservesOrderAndRepeats(t *testing.T) {
	// D-1/D-2: order preserved, repeated labels stay as separate edges.
	n := NewNode().
		AddValue("item", ScalarValue(NewStringScalar("pen"))).
		AddValue("note", ScalarValue(NewStringScalar("rush"))).
		AddValue("item", ScalarValue(NewStringScalar("pad")))

	if len(n.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(n.Edges))
	}
	wantLabels := []string{"item", "note", "item"}
	for i, want := range wantLabels {
		if n.Edges[i].Label != want {
			t.Errorf("edge %d label = %q, want %q", i, n.Edges[i].Label, want)
		}
	}
	v0, _ := n.Edges[0].Target.Value()
	v2, _ := n.Edges[2].Target.Value()
	if v0.Scalar.Str != "pen" || v2.Scalar.Str != "pad" {
		t.Error("repeated-label edges were merged or reordered")
	}

	child := NewNode()
	n2 := NewNode().AddNode("child", child)
	gotChild, ok := n2.Edges[0].Target.Node()
	if !ok || gotChild != child {
		t.Error("AddNode did not wire up the node target correctly")
	}
}

func TestDocumentConstructors(t *testing.T) {
	n := NewNode()
	d := NodeDocument(n)
	if !d.IsNode || d.Node != n {
		t.Errorf("NodeDocument: got %+v", d)
	}

	v := ScalarValue(NewBooleanScalar(true))
	d2 := ValueDocument(v)
	if d2.IsNode || !d2.Value.Scalar.Equal(v.Scalar) {
		t.Errorf("ValueDocument: got %+v", d2)
	}
}
