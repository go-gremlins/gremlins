package calc

import "testing"

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

func TestBelow(t *testing.T) {
	if !Below(1, 2) || Below(2, 2) {
		t.Error("Below is wrong at the boundary")
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

func TestDiff(t *testing.T) {
	if Diff(3, 1) != 2 {
		t.Error("Diff(3, 1) != 2")
	}
}

func TestMix(t *testing.T) {
	if Mix(1, 2, 1) != 3 {
		t.Error("Mix(1, 2, 1) != 3")
	}
}

func TestClamp(t *testing.T) {
	if Clamp(0) != 0 {
		t.Error("Clamp(0) != 0")
	}
}

func TestBump(t *testing.T) {
	if !Bump(5) {
		t.Error("Bump(5) is false")
	}
}

func TestFlip(t *testing.T) {
	if Flip(0) != 0 {
		t.Error("Flip(0) != 0")
	}
}

func TestAcc(t *testing.T) {
	if Acc(1, 2) != 3 {
		t.Error("Acc(1, 2) != 3")
	}
}

func TestGrow(t *testing.T) {
	if Grow(1, 1) != 1 {
		t.Error("Grow(1, 1) != 1")
	}
}
