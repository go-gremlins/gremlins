package b

import (
	"path/filepath"
	"testing"
)

func TestSub(t *testing.T) {
	if Sub(5, 3) != 2 {
		t.Fatal("Sub(5, 3) != 2")
	}
}

// TestReadsOriginalSource passes only in the package's original directory:
// the schema copy also holds the generated helper file.
func TestReadsOriginalSource(t *testing.T) {
	m, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("package directory holds %d .go files, want 2: %v", len(m), m)
	}
}
