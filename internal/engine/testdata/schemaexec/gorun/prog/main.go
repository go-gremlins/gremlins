// Command prog prints calc.Unused(5, 3).
package main

import (
	"fmt"

	"schemaexec/calc"
)

func main() { fmt.Println(calc.Unused(5, 3)) }
