package forms

import (
	"fmt"
	"math"
	"testing"
)

// show prints name and f's result, or the value f panicked with.
func show(name string, f func() any) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %v\n", name, r)
		}
	}()
	fmt.Printf("%s: %v\n", name, f())
}

func TestPrint(t *testing.T) {
	nan := math.NaN()
	show("Eq", func() any { return []bool{Eq(1, 1), Eq(1, 2)} })
	show("Neq", func() any { return []bool{Neq("a", "a"), Neq("a", "b")} })
	show("TypedNil", func() any { return TypedNil() })
	show("Recovered", func() any { return Recovered() })
	show("Less", func() any { return []bool{Less(1, 2), Less(2, 2), Less(3, 2)} })
	show("LessEq", func() any { return []bool{LessEq(1, 2), LessEq(2, 2), LessEq(nan, 2), LessEq(2, nan)} })
	show("Hot", func() any { return []bool{Hot(29), Hot(30), Hot(31)} })
	show("GreaterEq", func() any { return []bool{GreaterEq("a", "b"), GreaterEq("b", "b"), GreaterEq("c", "b")} })
	show("GMax", func() any { return []any{GMax(1, 2), GMax(2, 2), GMax(2.5, 1.5), GMax("a", "b"), GMax(nan, 1)} })
	show("GSum", func() any { return []any{GSum([]int{1, 2, 3}), GSum([]float64{0.5, 0.25})} })
	show("Add", func() any { return Add(3, 4) })
	show("Sub", func() any { return []float64{Sub(3, 4), Sub(nan, 1)} })
	show("Mul", func() any { return Mul(12, 3) })
	show("Quo", func() any { return Quo(12, 3) })
	show("QuoZero", func() any { return Quo(12, 0) })
	show("Rem", func() any { return Rem(14, 4) })
	show("Neg", func() any { return Neg(5) })
	show("NegU", func() any { return NegU(3) })
	show("Pos", func() any { return Pos(-2.5) })
	show("MultiLine", func() any { return MultiLine(3, 4) })
	show("Commented", func() any { return []bool{Commented(1, 2), Commented(2, 2)} })
	show("Nested", func() any { return []bool{Nested(1, 1, 3), Nested(1, 2, 3), Nested(2, 2, 3)} })
	show("NestedEq", func() any { return []bool{NestedEq(1, 2), NestedEq(2, 2), NestedEq(0, 0)} })
	show("GAddOne", func() any { return []any{GAddOne(1), GAddOne(2.5)} })
	show("Order", func() any { r, x := Order(); return []any{r, x} })
	show("IncIdent", func() any { return IncIdent() })
	show("DecField", func() any { return DecField() })
	show("IncSlice", func() any { return IncSlice([]int{1, 2, 3}, 2) })
	show("IncSliceOOR", func() any { return IncSlice([]int{1}, 3) })
	show("IncArray", func() any { return IncArray() })
	show("IncPtrArray", func() any { return *IncPtrArray(&[2]int{4, 5}) })
	show("IncDeref", func() any { return IncDeref() })
	show("DecParen", func() any { return DecParen() })
	show("IncMap", func() any { m, n := IncMap(); return []any{m, n} })
	show("IncNilMap", func() any { IncNilMap(); return nil })
	show("GInc", func() any { return []any{GInc([]int{1, 2}), GInc([]float64{1, 2.5})} })
	show("Closures", func() any { return Closures() })
	show("BitInt", func() any { return [][]int{BitInt(12, 10), BitInt(0, 7), BitInt(-6, 3)} })
	show("BitU8", func() any { return [][]uint8{BitU8(12, 10), BitU8(255, 1)} })
	show("BitMask", func() any { return [][]mask{BitMask(12, 10), BitMask(0xff00, 0x0ff0)} })
	show("ShiftU64", func() any { return [][]uint64{ShiftU64(12, 2), ShiftU64(1<<63, 63)} })
	show("ShiftI8", func() any { return [][]int8{ShiftI8(12, 2), ShiftI8(-64, 1), ShiftI8(3, 9)} })
	show("ShlUntypedLeft", func() any { return []int64{ShlUntypedLeft(0), ShlUntypedLeft(40)} })
	show("LandShort", func() any {
		var out []any
		for _, p := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
			r, n := LandShort(p[0], p[1])
			out = append(out, r, n)
		}
		return out
	})
	show("LorShort", func() any {
		var out []any
		for _, p := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
			r, n := LorShort(p[0], p[1])
			out = append(out, r, n)
		}
		return out
	})
	show("LandNamed", func() any { return []flag{LandNamed(true, true), LandNamed(true, false), LandNamed(false, true)} })
	show("MaskTest", func() any { return []bool{MaskTest(4, 3, true), MaskTest(4, 4, true), MaskTest(4, 3, false)} })
	for _, p := range [][2]int{{1, 2}, {2, 2}, {3, 2}} {
		a, b := p[0], p[1]
		sa, sb := fmt.Sprint(a), fmt.Sprint(b)
		show("Flag", func() any {
			return []any{
				FlagConv(sa, sb), FlagConvLess(a, b), FlagReturn(float64(a), float64(b)), FlagAssign(a, b),
				FlagVar(a, b), FlagArg(a, b), FlagField(a, b), FlagSlice(a, b), FlagMap(a, b), FlagSend(a, b),
				FlagOperand(true, a, b), FlagOperand(false, a, b), FlagParam[flag](a, b), FlagLocal(sa, sb),
			}
		})
	}
}
