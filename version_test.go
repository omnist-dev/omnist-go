package omnist

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSpecVersion(t *testing.T) {
	if SpecVersion == "" {
		t.Fatal("SpecVersion must not be empty")
	}
}

func TestSpecVersionMatchesSubmodule(t *testing.T) {
	// Guard against version/doc drift (issue #75): ensure SpecVersion
	// tracks the active release tag of the vendor/omnist-spec submodule.
	out, err := exec.Command("git", "-C", "vendor/omnist-spec", "describe", "--tags").Output()
	if err != nil {
		if _, statErr := os.Stat("vendor/omnist-spec"); statErr != nil {
			t.Skip("vendor/omnist-spec submodule not present")
		}
		return
	}
	desc := strings.TrimSpace(string(out))
	if !strings.HasPrefix(desc, SpecVersion) {
		t.Errorf("SpecVersion %q does not match vendor/omnist-spec describe tag %q", SpecVersion, desc)
	}

	shaOut, err := exec.Command("git", "-C", "vendor/omnist-spec", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return
	}
	shortSHA := strings.TrimSpace(string(shaOut))

	limitationsData, err := os.ReadFile("docs/limitations.md")
	if err == nil {
		if !strings.Contains(string(limitationsData), shortSHA) {
			t.Errorf("docs/limitations.md does not cite current submodule SHA %s", shortSHA)
		}
	}
}
