package bad

import "testing"

func TestF(t *testing.T) {
	if F(1, 2, 3) != 7 || !G(2) {
		t.Error("wrong")
	}
}
