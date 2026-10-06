package c

import "testing"

func TestLess(t *testing.T) {
	if !Less(1, 2) {
		t.Fatal("Less(1, 2) is false")
	}
}
