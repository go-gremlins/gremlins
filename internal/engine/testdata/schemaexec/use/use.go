// Package use depends on calc.
package use

import "schemaexec/calc"

// Double doubles x through calc.Scale.
func Double(x int) int { return calc.Scale(x, 2) }
