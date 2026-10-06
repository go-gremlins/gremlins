package use

import "testing"

func TestDouble(t *testing.T) {
	if Double(3) != 6 {
		t.Error("Double(3) != 6")
	}
}
