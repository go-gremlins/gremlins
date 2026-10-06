package stmt

import (
	"fmt"
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
	show("AddAssign", func() any { return AddAssign(7, 3) })
	show("SubAssign", func() any { return SubAssign(7, 3) })
	show("MulAssign", func() any { return MulAssign(7, 3) })
	show("QuoAssign", func() any { return QuoAssign(7, 3) })
	show("RemAssign", func() any { return RemAssign(7, 3) })
	show("QuoAssignFloat", func() any { return QuoAssignFloat(7, 4) })
	show("AndAssignTwoMutants", func() any { return AndAssignTwoMutants(12, 10) })
	show("OrAssign", func() any { return OrAssign(12, 10) })
	show("XorAssign", func() any { return XorAssign(12, 10) })
	show("AndNotAssign", func() any { return AndNotAssign(12, 10) })
	show("ShlAssignSameType", func() any { return ShlAssignSameType(12, 2) })
	show("ShrAssignSameType", func() any { return ShrAssignSameType(12, 2) })
	show("ShlAssignMixedType", func() any { return ShlAssignMixedType(12, 2) })
	show("AddAssignString", func() any { return AddAssignString("a", "b") })
	show("AddAssignSideEffectLHS", func() any { a, n := AddAssignSideEffectLHS(); return []any{a, n} })
	show("AddAssignSideEffectRHS", func() any { x, n := AddAssignSideEffectRHS(); return []any{x, n} })
	show("InCase", func() any { return []int{InCase(1, 5), InCase(2, 5)} })
	show("LabelledAssign", func() any { return []int{LabelledAssign(true), LabelledAssign(false)} })
	show("NestedSiteInLHS", func() any { return NestedSiteInLHS(1) })
	show("MultiLine", func() any { return MultiLine(7, 3) })
	show("MapAssignNamedMap", func() any { m, n := MapAssignNamedMap(); return []any{m, n} })
	show("ForPostAddAssign", func() any { return ForPostAddAssign() })
	show("IfInitAssign", func() any { return []int{IfInitAssign(1), IfInitAssign(5)} })
	show("SimpleInt", func() any { v, m := SimpleInt(13, 5); return []any{v, m} })
	show("SimpleShift", func() any { v, m := SimpleShift(12, 2, 1); return []any{v, m} })
	show("SimpleShiftConst", func() any { v, m, b, c := SimpleShiftConst(12, 3); return []any{v, m, b, c} })
	show("SimpleFloat", func() any { x, m := SimpleFloat(7, 4); return []any{x, m} })
	show("SimpleString", func() any { s, m := SimpleString("a", "b"); return []any{s, m} })
}
