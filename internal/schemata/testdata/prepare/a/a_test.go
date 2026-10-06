package a

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("Add(2, 3) != 5")
	}
	if Bump(map[string]int{"a": 1}) != 2 {
		t.Fatal("Bump(a: 1) != 2")
	}
}
