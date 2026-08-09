package main

import "testing"

func TestLookupReaderKnown(t *testing.T) {
	for _, name := range knownFormatNames() {
		if _, err := lookupReader(name); err != nil {
			t.Errorf("lookupReader(%q): %v", name, err)
		}
		if _, err := lookupWriter(name); err != nil {
			t.Errorf("lookupWriter(%q): %v", name, err)
		}
	}
}

func TestLookupReaderCaseInsensitive(t *testing.T) {
	if _, err := lookupReader("JSON"); err != nil {
		t.Errorf("lookupReader(%q): %v", "JSON", err)
	}
}

func TestLookupReaderUnknown(t *testing.T) {
	if _, err := lookupReader("csv"); err == nil {
		t.Error("lookupReader(csv): expected error, got nil")
	}
}

func TestLookupWriterUnknown(t *testing.T) {
	if _, err := lookupWriter("csv"); err == nil {
		t.Error("lookupWriter(csv): expected error, got nil")
	}
}

func TestKnownFormatNamesCount(t *testing.T) {
	names := knownFormatNames()
	if len(names) != 5 {
		t.Errorf("knownFormatNames() = %v, want 5 entries", names)
	}
}
