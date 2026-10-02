package algebra

import (
	"math/rand"
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
	"github.com/omnist-dev/omnist-go/osd"
)

func TestSanitizeIdentifierASCIIOnly(t *testing.T) {
	cases := map[string]string{
		"":         "Rec",
		"éclair":   "_clair",
		"日本":       "__",
		"123":      "_123",
		"a b":      "A_b",
		"snake_ok": "Snake_ok",
		"Already":  "Already",
		"_x":       "_x",
		"a\xffb":   "A_b",
		"x-1":      "X_1",
	}
	for in, want := range cases {
		if got := sanitizeIdentifier(in); got != want {
			t.Errorf("sanitizeIdentifier(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueNameFromCollisions(t *testing.T) {
	used := map[string]bool{}
	var got []string
	for _, l := range []string{"日本", "中国", "a b", "A b", "", ""} {
		n := uniqueNameFrom(l, used)
		used[n] = true
		got = append(got, n)
	}
	if strings.Join(got, ",") != "__,__2,A_b,A_b2,Rec,Rec2" {
		t.Errorf("got %v", got)
	}
}

// Property: whatever the (legal) keys, an inferred schema passes Validate and
// round-trips through osd.Write and osd.Read. Keys that are not legal labels
// (empty, brackets, invalid UTF-8) are the label rules' business, not the
// record namer's.
func TestInferredSchemaAlwaysValidAndRoundTrips(t *testing.T) {
	alphabet := []string{"a", "Z", "_", "0", "9", " ", "-", ".", "é", "日", "😀", "x"}
	r := rand.New(rand.NewSource(28))
	for i := 0; i < 1500; i++ {
		var edges []omnist.Edge
		seen := map[string]bool{}
		for j := 0; j < 1+r.Intn(4); j++ {
			label := "k"
			for k := 0; k < r.Intn(4); k++ {
				label += alphabet[r.Intn(len(alphabet))]
			}
			if r.Intn(3) == 0 {
				label = label[1:]
			}
			if label == "" || seen[label] {
				continue
			}
			seen[label] = true
			edges = append(edges, nodeEdge(label, omnist.NewNode().AddValue("v", omnist.ScalarValue(omnist.NewStringScalar("x")))))
		}
		s, err := Infer([]omnist.Document{strDoc(edges...)}, "", false)
		if err != nil {
			t.Fatalf("infer: %v", err)
		}
		if err := s.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		text, err := osd.Write(s, i%2 == 0)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		if _, err := osd.Read(text); err != nil {
			t.Fatalf("Read(Write(s)): %v\n%s", err, text)
		}
	}
}
