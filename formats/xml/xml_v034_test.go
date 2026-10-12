package xml

import (
	"math/big"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

func stringifyDoc(edges ...omnist.Edge) omnist.Document {
	r := omnist.NewNode()
	r.Edges = edges
	return omnist.NodeDocument(omnist.NewNode().AddNode("r", r))
}

func leaf(label string, s omnist.Scalar) omnist.Edge {
	return omnist.Edge{Label: label, Target: omnist.ValueTarget(omnist.ScalarValue(s))}
}

func diagPaths(diags []omnist.Diagnostic) (paths, codes []string) {
	for _, d := range diags {
		paths = append(paths, d.Path)
		codes = append(codes, string(d.Code))
	}
	return
}

// E-5: value-stringified for integer/number/boolean leaves, temporal-stringified
// (never also value-stringified) for date/time/datetime, none for a string.
func TestWriteReportsStringifiedLeaves(t *testing.T) {
	d := stringifyDoc(
		leaf("i", omnist.NewIntegerScalar(big.NewInt(5))),
		leaf("n", omnist.NewNumberScalar(1.5)),
		leaf("b", omnist.NewBooleanScalar(true)),
		leaf("s", omnist.NewStringScalar("5")),
		leaf("d", omnist.NewDateScalar(omnist.DateValue{Year: 2024, Month: 1, Day: 15})),
		leaf("t", omnist.NewTimeScalar(omnist.TimeValue{Hour: 12})),
		leaf("dt", omnist.NewDateTimeScalar(omnist.DateTimeValue{Date: omnist.DateValue{Year: 2024, Month: 1, Day: 1}, Time: omnist.TimeValue{Hour: 12, Minute: 30}})),
	)
	out, diags, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if want := "<r><i>5</i><n>1.5</n><b>true</b><s>5</s><d>2024-01-15</d><t>12:00:00</t><dt>2024-01-01T12:30:00</dt></r>"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	paths, codes := diagPaths(diags)
	wantPaths := []string{"$.r.i", "$.r.n", "$.r.b", "$.r.d", "$.r.t", "$.r.dt"}
	wantCodes := []string{"format.value-stringified", "format.value-stringified", "format.value-stringified", "format.temporal-stringified", "format.temporal-stringified", "format.temporal-stringified"}
	if len(paths) != 6 {
		t.Fatalf("diags = %v %v", paths, codes)
	}
	for i := range paths {
		if paths[i] != wantPaths[i] || codes[i] != wantCodes[i] {
			t.Errorf("diag %d = %s %s, want %s %s", i, paths[i], codes[i], wantPaths[i], wantCodes[i])
		}
		if diags[i].Severity != omnist.SeverityWarning {
			t.Errorf("diag %d severity = %v", i, diags[i].Severity)
		}
	}
}

// E-10: repeated leaves are indexed, also below repeated nodes.
func TestWriteReportsIndexPaths(t *testing.T) {
	item := func(n int64) omnist.Edge {
		return omnist.Edge{Label: "item", Target: omnist.NodeTarget(omnist.NewNode().
			AddValue("n", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(n)))).
			AddValue("n", omnist.ScalarValue(omnist.NewIntegerScalar(big.NewInt(n+1)))))}
	}
	_, diags, err := Write(stringifyDoc(item(1), item(3)))
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := diagPaths(diags)
	want := []string{"$.r.item[0].n[0]", "$.r.item[0].n[1]", "$.r.item[1].n[0]", "$.r.item[1].n[1]"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v", paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("path %d = %s, want %s", i, paths[i], want[i])
		}
	}
}

func TestFormatXMLTimeKeepsZeroSeconds(t *testing.T) {
	for _, tc := range []struct {
		in   omnist.TimeValue
		want string
	}{
		{omnist.TimeValue{Hour: 12}, "12:00:00"},
		{omnist.TimeValue{Hour: 1, Minute: 2, Second: 3}, "01:02:03"},
		{omnist.TimeValue{Hour: 12, HasOffset: true, OffsetSeconds: 3600}, "12:00:00+01:00"},
		{omnist.TimeValue{Hour: 1, Nanosecond: 500_000_000}, "01:00:00.5"},
	} {
		if got := omnist.FormatISOTime(tc.in); got != tc.want {
			t.Errorf("FormatISOTime(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// E-4a: the Document root is level 1 and a node; a leaf element is no node.
func TestReadRootCountsAgainstLimits(t *testing.T) {
	l := omnist.DefaultLimits()
	l.MaxDepth = 3
	_, _, err := Read("<a><b><c><d>x</d></c></b></a>", l)
	if pe, ok := err.(*omnist.ParseError); !ok || pe.Code != omnist.CodeDocumentLimitDepth || pe.Path != "$" {
		t.Fatalf("got %v, want document.limit.depth at $", err)
	}
	if _, _, err := Read("<a><b><c>x</c></b></a>", l); err != nil {
		t.Errorf("at limit: %v", err)
	}
	l = omnist.DefaultLimits()
	l.MaxNodes = 1
	_, _, err = Read("<r><a>1</a></r>", l)
	if pe, ok := err.(*omnist.ParseError); !ok || pe.Code != omnist.CodeDocumentLimitNodes || pe.Path != "$" {
		t.Fatalf("got %v, want document.limit.nodes at $", err)
	}
	if _, _, err := Read("<r>x</r>", l); err != nil {
		t.Errorf("a leaf root is one node: %v", err)
	}
	l.MaxNodes = 0
	if _, _, err := Read("<r>x</r>", l); err == nil {
		t.Error("MaxNodes 0 refuses even the root")
	}
}
