package constgroup

import (
	"fmt"
	"testing"
)

func TestPrint(t *testing.T) {
	fmt.Println("Room:", Room("abc"))
	fmt.Println("Paren:", Paren(5))
	fmt.Println("NegSum:", NegSum(7))
	fmt.Println("Compare:", Compare(true), Compare(false))
	fmt.Println("FloatOperand:", FloatOperand(3))
	fmt.Println("FloatGroup:", FloatGroup(2))
	fmt.Println("FloatNamed:", FloatNamed(2))
	fmt.Println("Converted:", Converted(5))
	fmt.Println("Wide:", Wide(1))
	fmt.Println("LenArrayLength:", LenArrayLength(1))
	fmt.Println("LenArrayLengthGroup:", LenArrayLengthGroup(1))
	fmt.Println("LenElements:", LenElements(5, 1))
}
