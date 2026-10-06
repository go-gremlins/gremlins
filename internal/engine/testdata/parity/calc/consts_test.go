package calc

import "testing"

func TestConsts(t *testing.T) {
	if Offset(1) != 7 || Scaled(2) != 4 || Slots() != 3 {
		t.Error("a constant is wrong")
	}
}

func TestWidth(t *testing.T) {
	if Width(1) <= 0 {
		t.Error("Width(1) <= 0")
	}
}
