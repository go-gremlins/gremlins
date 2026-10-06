// Package constgroup is the fixture for TestConstantGroupsBehave: sites
// inside a constant-valued operand of a non-constant expression, each
// maximal constant operand placed as one constant site. Every plain token
// mutant the engine's type check admits compiles.
package constgroup

const maxIdentifier = 63

const databaseNamePrefix = "rw_test_"

// Room is the shape that netted in CI: the first two - fold to a constant,
// the last one does not.
func Room(sfx string) int {
	return maxIdentifier - len(databaseNamePrefix) - len("_") - len(sfx)
}

// Paren is a parenthesised constant operand holding one site.
func Paren(n int) int { return (4 * 3) + n }

// NegSum is a constant operand holding two sites of different mutators:
// unary - (ARITHMETIC_BASE, INVERT_NEGATIVES) over + (ARITHMETIC_BASE).
func NegSum(x int) int { return -(2 + 1) * x }

// Compare is a constant bool operand holding an arithmetic site under a
// comparison site.
func Compare(b bool) bool { return b && 2+1 > 2 }

// FloatOperand is a float constant operand of a binary expression: the
// other operand is the witness of its type.
func FloatOperand(x float64) float64 { return x * (1.5 + 0.5) }

// FloatGroup is a float constant operand holding two sites.
func FloatGroup(x float64) float64 { return x * (1.5 + 0.5*2) }

// Celsius is a named float type, to show the witness carries it.
type Celsius float64

// FloatNamed is FloatOperand in a named type, with the constant on the left.
func FloatNamed(c Celsius) Celsius { return (10.0 - 0.5) - c }

// Converted has a conversion on the path from the constant operand to
// its site.
func Converted(v int64) int64 { return int64(3*4) + v }

// LenArrayLength has a site in an array length under a lone constant
// site: the 2 * 3 is placed by duplicating the function, the *2 by its own
// constant form, which takes len's recorded value.
func LenArrayLength(n int) int { return len([2 * 3]int{})*2 + n }

// LenArrayLengthGroup is LenArrayLength under a group of two sites.
func LenArrayLengthGroup(n int) int { return len([2 * 3]int{})*2 - 1 + n }

// LenElements has a site len never evaluates: its mutants are placed in
// the group with the group's own value.
func LenElements(x, n int) int { return len([2]int{x + 1, 0})*3 + n }

// Wide has every intermediate of the constant operand beyond int64, folded
// exactly as the compiler folds them: run time would overflow.
func Wide(n int) int { return (1<<62)*4/8 + n }
