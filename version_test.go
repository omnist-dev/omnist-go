package omnist

import "testing"

func TestSpecVersion(t *testing.T) {
	if SpecVersion == "" {
		t.Fatal("SpecVersion must not be empty")
	}
}
