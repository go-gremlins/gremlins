package other

import "testing"

func TestQuad(t *testing.T) {
	if Quad(1) != 4 {
		t.Error("Quad(1) != 4")
	}
}
