package w

import (
	"os"
	"testing"
)

// TestAdd writes out.txt into its working directory, as a test writing a
// golden file or a cache does: a null run in the user's tree leaves it there.
func TestAdd(t *testing.T) {
	if err := os.WriteFile("out.txt", []byte("written\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Add(2, 3) != 5 {
		t.Error("Add(2, 3) != 5")
	}
}
