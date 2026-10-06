package calc

import "testing"

func TestBits(t *testing.T) {
	if And(6, 3) != 2 || Or(0, 0) != 0 || Mask(6, 3) != 2 || Set(0, 0) != 0 {
		t.Error("a bitwise operator is wrong")
	}
	if Both(true, false) || !Either(true, true) {
		t.Error("a logical operator is wrong")
	}
}
