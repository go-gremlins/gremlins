/*
 * Copyright 2026 The Gremlins Authors
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

// Package app is the fixture for TestConstantFormsBehave: constant-valued
// sites, each in a context that decides its form. A line marked
// "refused <op>" holds a site of that operator that must stay
// ErrUnsupported. Every plain token mutant of every site compiles.
package app

import (
	"math"
	"time"

	"fixture/foo"
)

const k = 2 * 3 // refused *: a const declaration

func SetSum() any { return foo.Set(1 + 2) }

func SetParen() any { return foo.Set((4 - 1)) }

func Small() any {
	y := foo.Small()
	y = 200 + 50
	return y
}

func Ratio() any {
	x := foo.SetR(1.5 * 2)
	return x
}

func RatioAssign() any {
	r := foo.R()
	r = 1.5 / 4
	return r
}

func Mix() any {
	l, r := foo.Mix(3, 0.5+0.25)
	return []any{l, r}
}

func Method() any {
	var b foo.Box
	b.Put(3.0 - 0.5) // refused -: a method value witness can panic out of order, and foo's ratio has no name here
	return b.R
}

func Neg() int { return -1 }

func NegOperand(x int) int { return x * -2 }

func Quo() int { return 7 / 2 }

func Typed() any { return foo.Unit * 3 }

func Rune() any { return 'a' + 1 }

func Cmp() bool { return k < 6 }

func CmpEq() bool { return k == 6 }

func If() int {
	if 2 < 2 {
		return 1
	}
	return 0
}

func Nested() int { return 2*3 + 1 } // 2*3 is an operand of a constant: one site with the +

func MultiLine() int {
	return 10 +
		5
}

func Arr() int {
	var a [1 + 1]int // an array length: placed by duplicating Arr
	return len(a)
}

func FloatBin(f float64) float64 { return f * (1.5 + 1) } // a float operand: f is the witness

func Key() []int { return []int{1 + 1: 5} } // a composite literal key: placed by duplicating Key

func Shift(x int) int { return x << (1 + 1) } // refused +: an untyped shift count

func Variadic() any { return foo.Sum(0.5 + 1) } // refused +: a variadic float argument

func Generic() any { return foo.Pick(2.5 * 2) } // a generic callee: Site[float64]() as its argument

const lenBase = 127

// LenString's len is of a constant string, whose value -- not its type --
// gives the length: the + inside cannot be folded through string(...).
func LenString(n int) int { return len(string(rune(127+1)))*3 + n } // refused +: behind a string conversion

func LenStringConst(n int) int { return len(string(rune(lenBase+1)))*3 + n } // refused +: behind a string conversion

func SetAnd() any { return foo.Set(6 & 3) }

func SetShl() any { return foo.Set(1 << 4) }

func Land() bool {
	x := true && false
	return x
}

// Pi keeps math used outside the sites, which the forms below replace with
// their values: this harness renders with Render alone, without
// RewritePackage's repair of an import the rewrite left unused, which
// imports_test.go covers.
func Pi() float64 { return math.Pi }

type dec float64

type bounds struct {
	Lo dec
	Hi float64
}

// Literal is assembler's map[string]any{"Decimal.Min": -math.MaxFloat64}:
// a float constant as a composite literal's value, of its default type.
func Literal() any {
	return map[string]any{
		"min":  -math.MaxFloat64,
		"half": 1.5 / 3,
	}
}

func StructKV() any { return bounds{Lo: -math.MaxFloat64, Hi: 2.5 * 2} }

func StructPos() any { return bounds{1.5 + 1, -math.SmallestNonzeroFloat64} }

func SliceLit() any { return []dec{0.5 * 3, 4.5 - 1} }

func ArrayLit() any { return [...]float32{1.5 + 2} }

func ForeignLit() any { return []foo.Level{0.25 * 2} }

func Send() any {
	c := make(chan dec, 1)
	c <- 1.5 * 3
	return <-c
}

func Append() any { return append([]float64{1}, 0.5+0.25, 2.5*2) }

func Return() float64 { return 2.5 * 4 }

func Define() any {
	r := 1.5 * 2
	return r
}

// d := 200 * time.Millisecond is a backoff seed. go/types records the
// untyped 200 with the type it converts to, time.Duration, so the operand
// that carries the constant's type is time.Millisecond, not the literal.
// Nothing else gives the type under :=; spelled from the literal, d is an
// int and the call below does not compile.
func DefineTyped() any {
	d := 200 * time.Millisecond
	return waitFor(d)
}

func waitFor(d time.Duration) time.Duration { return d }

func Unexported() any { return foo.Box{R: 1.5 * 2}.R } // refused *: foo's ratio has no name here

func Shadowed() any {
	float64 := 1
	return []any{float64, 0.5 * 3} // refused *: float64 names a variable here
}

// drBase - drTiny's mutant drBase + drTiny lies within half a float64 ulp
// of a float32 rounding midpoint: rounded to float64 first, then to
// float32, it gives 1, where the plain mutant rounds it once, to
// 1.0000001.
const (
	drBase = 1.000000059604644775390625                  // 1 + 2^-24
	drTiny = 5.5511151231257827021181583404541015625e-17 // 2^-54
)

func DoubleRound() any { return []float32{drBase - drTiny} }

func DoubleRoundAssign() any {
	var x float32
	x = drBase - drTiny
	return x
}
