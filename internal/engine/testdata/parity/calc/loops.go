package calc

// SumUntilNeg sums xs up to its first negative element. TestLoops passes an
// element after the negative one, so the break mutant is killed.
func SumUntilNeg(xs []int) int {
	s := 0
	for _, x := range xs {
		if x < 0 {
			break
		}
		s += x
	}

	return s
}

// FirstNeg returns the index of the first negative element of xs, len(xs)
// when there is none. TestLoops passes one negative, where break and
// continue agree: the break mutant lives.
func FirstNeg(xs []int) int {
	at := len(xs)
	for i, x := range xs {
		if x < 0 {
			at = i

			break
		}
	}

	return at
}
