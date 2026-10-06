package calc

// Limit is a package-level constant: its mutant is NOT COVERED, decided
// before any test runs.
const Limit = 4 + 4

// Offset adds a constant expression at run time: 2*3 takes a constant form.
func Offset(x int) int { return x + 2*3 }

// Scaled multiplies by a function-local constant: its declaration is a
// compile-time site, placed by duplicating Scaled.
func Scaled(x int) int {
	const k = 3 - 1

	return x * k
}

// Slots counts the elements of an array whose length is a function-local
// constant: placed by duplicating Slots.
func Slots() int {
	const n = 2 + 1
	var a [n]bool

	return len(a)
}

// Width uses a function-local constant both as an array length and at run
// time: placed by duplicating Width. TestWidth sees w = 0 (2/2 - 1) but not
// w = 5 (2*2 + 1), so one of its mutants is killed and the other lives.
func Width(x int) int {
	const w = 2*2 - 1
	var a [w]int

	return x*w + len(a)
}
