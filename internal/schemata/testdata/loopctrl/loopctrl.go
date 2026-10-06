// Package loopctrl is the fixture for TestLoopCtrlFormsBehave: one function
// per loop-control case, each returning the trace of its iterations. Every
// plain token mutant the engine's type check admits compiles.
package loopctrl

import "iter"

func BreakInFor() []int {
	var trace []int
	for i := 0; i < 5; i++ {
		trace = append(trace, i)
		if i == 2 {
			break
		}
	}

	return trace
}

func ContinueInFor() []int {
	var trace []int
	for i := 0; i < 5; i++ {
		if i%2 == 1 {
			continue
		}
		trace = append(trace, i)
	}

	return trace
}

// BreakInSwitchInLoop: the break leaves the switch; the plain mutant, a
// continue, restarts the loop.
func BreakInSwitchInLoop() []int {
	var trace []int
	for i := 0; i < 4; i++ {
		switch i {
		case 1:
			break
		default:
			trace = append(trace, i)
		}
		trace = append(trace, i*10)
	}

	return trace
}

func BreakInSelectInLoop() []int {
	var trace []int
	for i := 0; i < 3; i++ {
		select {
		default:
			if i == 1 {
				break
			}
			trace = append(trace, i)
		}
		trace = append(trace, i*10)
	}

	return trace
}

func LabelledBreak() []int {
	var trace []int
outer:
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if j == 1 {
				break outer
			}
			trace = append(trace, i*10+j)
		}
		trace = append(trace, 100+i)
	}

	return trace
}

func LabelledContinueOuter() []int {
	var trace []int
outer:
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if j == 1 {
				continue outer
			}
			trace = append(trace, i*10+j)
		}
		trace = append(trace, 100+i)
	}

	return trace
}

// BreakOutsideLoop has a break whose plain mutant, a continue, does not
// compile: the engine generates none, so the fixture's discovery drops it.
func BreakOutsideLoop(x int) int {
	switch x {
	case 1:
		x++
		break
	}

	return x
}

// LabelledSwitchBreak is the same for a break to a switch's label.
func LabelledSwitchBreak(x int) int {
sw:
	switch x {
	case 1:
		x++
		break sw
	}

	return x
}

func seq(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func RangeOverFuncBreak() []int {
	var trace []int
	for x := range seq(5) {
		trace = append(trace, x)
		if x == 2 {
			break
		}
	}

	return trace
}

func RangeOverFuncContinue() []int {
	var trace []int
	for x := range seq(5) {
		if x%2 == 0 {
			continue
		}
		trace = append(trace, x)
	}

	return trace
}
