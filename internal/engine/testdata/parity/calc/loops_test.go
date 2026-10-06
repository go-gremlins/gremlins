package calc

import "testing"

func TestLoops(t *testing.T) {
	if got := SumUntilNeg([]int{1, -1, 5}); got != 1 {
		t.Errorf("SumUntilNeg = %d, want 1", got)
	}
	if got := FirstNeg([]int{3, -2, 4}); got != 1 {
		t.Errorf("FirstNeg = %d, want 1", got)
	}
}
