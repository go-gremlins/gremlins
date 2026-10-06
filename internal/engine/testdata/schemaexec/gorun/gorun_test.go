package gorun

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestUnusedThroughGoRun finds its own directory with runtime.Caller and runs
// prog there with go run, as a test that builds its module's code does. The
// go command builds what GOFLAGS lays over that directory: a mutant of
// Unused reaches the program only if the overlay covers the path the test
// binary names.
func TestUnusedThroughGoRun(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	cmd := exec.Command("go", "run", "./prog")
	cmd.Dir = filepath.Dir(file)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run ./prog in %s: %v\n%s", cmd.Dir, err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "2" {
		t.Errorf("calc.Unused(5, 3) = %s through go run, want 2", got)
	}
}
