// Package notest depends on calc and has no test files.
package notest

import "schemaexec/calc"

// Sum adds a and b through calc.Add.
func Sum(a, b int) int { return calc.Add(a, b) }
