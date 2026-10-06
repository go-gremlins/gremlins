package calc

import (
	"os"
	"testing"
	"time"
)

// TestAlone is the isolation probe of the schema executor's tests: under a
// mutant run it writes the mutant's id into a marker file in its working
// directory, waits, and reads it back. A worker sharing the directory writes
// its own id meanwhile, and the test fails. It is first so that it runs
// before the tests that hang or exit. Outside a mutant run it does nothing.
func TestAlone(t *testing.T) {
	id := os.Getenv("GREMLINS_MUTANT")
	if id == "" {
		return
	}
	const marker = "alone.marker"
	if err := os.WriteFile(marker, []byte(id), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(marker)
	time.Sleep(300 * time.Millisecond)
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != id {
		t.Errorf("marker for mutant %s reads %q, %v: another run shares this directory", id, got, err)
	}
}

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Error("Add(2, 3) != 5")
	}
}

func TestScale(t *testing.T) {
	if Scale(2, 1) != 2 {
		t.Error("Scale(2, 1) != 2")
	}
}

func TestPause(t *testing.T) {
	Pause(0)
}

func TestInc(t *testing.T) {
	if Inc(1) != 2 {
		t.Error("Inc(1) != 2")
	}
}

func TestNeg(t *testing.T) {
	if Neg(1) != -1 {
		t.Error("Neg(1) != -1")
	}
}

func TestQuit(t *testing.T) {
	if Quit(7, 2) != 1 {
		os.Exit(0)
	}
}
