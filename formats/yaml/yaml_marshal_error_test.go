package yaml

import (
	"errors"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// A Marshal failure other than invalid UTF-8 (which CheckEncodable refuses with
// the coded C-9 failure first) is returned, not swallowed into empty output.
func TestWriteReturnsAMarshalError(t *testing.T) {
	boom := errors.New("boom")
	old := marshalYAML
	marshalYAML = func(any) ([]byte, error) { return nil, boom }
	defer func() { marshalYAML = old }()
	d := omnist.NodeDocument(omnist.NewNode().AddValue("a", omnist.ScalarValue(omnist.NewStringScalar("x"))))
	out, _, err := Write(d)
	if !errors.Is(err, boom) || out != "" {
		t.Errorf("got %q, %v; want the Marshal error and no text", out, err)
	}
}
