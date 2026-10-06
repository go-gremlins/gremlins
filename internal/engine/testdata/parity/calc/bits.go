package calc

// And is pinned by TestBits: its INVERT_BITWISE mutant is killed.
func And(a, b int) int { return a & b }

// Or is checked only at (0, 0), where | and & agree: its mutant lives.
func Or(a, b int) int { return a | b }

// Mask is pinned by TestBits: its &= mutants are killed.
func Mask(a, b int) int {
	a &= b

	return a
}

// Set is checked only at (0, 0), where |=, &= and = agree: its mutants live.
func Set(a, b int) int {
	a |= b

	return a
}

// Both is pinned by TestBits: its INVERT_LOGICAL mutant is killed.
func Both(a, b bool) bool { return a && b }

// Either is checked only at (true, true), where || and && agree: its mutant
// lives.
func Either(a, b bool) bool { return a || b }
