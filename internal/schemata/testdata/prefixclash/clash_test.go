package prefixclash

import "testing"

// _gremlinsActive takes the name the default prefix would generate: the
// package only builds if the rewrite chose another prefix. It is declared in
// a test file, which the package's non-test syntax does not include.
func _gremlinsActive() int { return 0 }

func TestClash(t *testing.T) {
	if Add(2, 3)+_gremlinsActive() != 5 {
		t.Error("Add(2, 3) != 5")
	}
	if !Less(1, 2) || Less(2, 2) {
		t.Error("Less is wrong")
	}
}
