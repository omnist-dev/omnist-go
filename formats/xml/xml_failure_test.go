package xml

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// The failure carried up the recursion is an error in its own right until the
// top level turns it into the Diagnostic a writer returns.
func TestXMLFailureError(t *testing.T) {
	f := &xmlFailure{diag: omnist.Diagnostic{Path: "$.a", Code: omnist.CodeWriteUnsupportedValue, Message: "boom", Severity: omnist.SeverityError}}
	if got := f.Error(); !strings.Contains(got, "boom") || !strings.Contains(got, string(omnist.CodeWriteUnsupportedValue)) {
		t.Errorf("Error() = %q", got)
	}
}
