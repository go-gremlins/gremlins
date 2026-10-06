// Package calc holds the sites the schemata parity run compares: a KILLED
// and a LIVED mutant of every mutator, a suppressed site, an uncovered
// function, a mutant that outlives the test timeout, two operators on one
// line, constant sites (consts.go) and a line only another package's test
// executes (Twice).
package calc

import "time"

// Add is pinned by TestAdd: its ARITHMETIC_BASE mutant is killed.
func Add(a, b int) int { return a + b }

// Scale is checked only at b == 1, where * and / agree: its mutant lives.
func Scale(a, b int) int { return a * b }

// Below is checked on both sides of the boundary: both its mutants are killed.
func Below(a, b int) bool { return a < b }

// Pause sleeps when d is at least 1. The test passes 0, so the boundary
// mutant (d > 1) lives and the negated one (d < 1) outlives the timeout.
func Pause(d int) {
	if d >= 1 {
		time.Sleep(time.Hour)
	}
}

// Inc is pinned by TestInc.
func Inc(x int) int {
	x++

	return x
}

// Neg is pinned by TestNeg.
func Neg(x int) int { return -x }

// Diff is pinned by TestDiff.
func Diff(a, b int) int { return a - b }

// Unused is called by no test: its mutant is not covered.
func Unused(a, b int) int { return a * b }

// Mix holds two operators on one line. TestMix kills the '+' mutant and lets
// the '*' one live (c == 1, where * and / agree), so a schema id that switched
// the other operator would change both verdicts.
func Mix(a, b, c int) int { return a + b*c }

// Clamp is checked only at 0, where neither of its mutants changes the
// result: both live.
func Clamp(x int) int {
	if x < 0 {
		return 0
	}

	return x
}

// Bump is checked at 5: the decrement lives, the negated != is killed.
func Bump(x int) bool {
	x++

	return x != 0
}

// Flip is checked only at 0, where -x and +x agree: its mutants live.
func Flip(x int) int { return -x }

// Acc is pinned by TestAcc: both of its assignment mutants are killed.
func Acc(a, b int) int {
	a += b

	return a
}

// Grow is checked only at (1, 1), where *=, /= and = agree: its mutants live.
func Grow(a, b int) int {
	a *= b

	return a
}

// Twice is executed by no test of calc, only by parity/other's TestQuad,
// which kills its mutant when --cross-package runs it.
func Twice(x int) int { return x * 2 }
