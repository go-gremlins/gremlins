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

package app

import (
	"fmt"
	"testing"
)

// show prints name and f's result with its dynamic type, or the value f
// panicked with.
func show(name string, f func() any) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %v\n", name, r)
		}
	}()
	v := f()
	fmt.Printf("%s: %T %v\n", name, v, v)
}

func TestPrint(t *testing.T) {
	show("SetSum", SetSum)
	show("SetParen", SetParen)
	show("Small", Small)
	show("Ratio", Ratio)
	show("RatioAssign", RatioAssign)
	show("Mix", Mix)
	show("Method", Method)
	show("Neg", func() any { return Neg() })
	show("NegOperand", func() any { return NegOperand(5) })
	show("Quo", func() any { return Quo() })
	show("Typed", Typed)
	show("Rune", Rune)
	show("Cmp", func() any { return Cmp() })
	show("CmpEq", func() any { return CmpEq() })
	show("If", func() any { return If() })
	show("Nested", func() any { return Nested() })
	show("MultiLine", func() any { return MultiLine() })
	show("Arr", func() any { return Arr() })
	show("FloatBin", func() any { return FloatBin(2) })
	show("Key", func() any { return Key() })
	show("Shift", func() any { return Shift(3) })
	show("Variadic", Variadic)
	show("Generic", Generic)
	show("LenString", func() any { return LenString(1) })
	show("LenStringConst", func() any { return LenStringConst(1) })
	show("K", func() any { return k })
	show("SetAnd", SetAnd)
	show("SetShl", SetShl)
	show("Land", func() any { return Land() })
	show("Literal", Literal)
	show("StructKV", StructKV)
	show("StructPos", StructPos)
	show("SliceLit", SliceLit)
	show("ArrayLit", ArrayLit)
	show("ForeignLit", ForeignLit)
	show("Send", Send)
	show("Append", Append)
	show("Return", func() any { return Return() })
	show("Define", Define)
	show("DefineTyped", DefineTyped)
	show("Unexported", Unexported)
	show("Shadowed", Shadowed)
	show("DoubleRound", DoubleRound)
	show("DoubleRoundAssign", DoubleRoundAssign)
}
