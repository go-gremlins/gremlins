// Package other depends on calc: its test is the only one that executes, and
// so kills, calc.Twice's mutant, when --cross-package selects it.
package other

import "parity/calc"

// Quad doubles x twice.
func Quad(x int) int { return calc.Twice(calc.Twice(x)) }
