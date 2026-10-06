// Package calc holds one function per verdict the schema executor must reach.
package calc

import "time"

// Add is pinned by TestAdd: its mutant is killed.
func Add(a, b int) int { return a + b }

// Scale is checked only at b == 1, where * and / agree: its mutant lives in
// this package's suite, and the use package's suite kills it.
func Scale(a, b int) int { return a * b }

// Unused is called by no test: its mutants are not covered.
func Unused(a, b int) int { return a - b }

// Pause sleeps when d is positive; the tests pass 0, so a mutant that makes
// the condition true for 0 outlives the test timeout.
func Pause(d int) {
	if d > 0 {
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

// Quit is checked by TestQuit, which exits 0 when the result is wrong: go
// test fails a binary whose test calls os.Exit(0), so its mutant is killed.
func Quit(a, b int) int { return a % b }
