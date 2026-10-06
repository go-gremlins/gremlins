package loopctrl

import (
	"fmt"
	"testing"
)

func TestPrint(t *testing.T) {
	fmt.Printf("BreakInFor: %v\n", BreakInFor())
	fmt.Printf("ContinueInFor: %v\n", ContinueInFor())
	fmt.Printf("BreakInSwitchInLoop: %v\n", BreakInSwitchInLoop())
	fmt.Printf("BreakInSelectInLoop: %v\n", BreakInSelectInLoop())
	fmt.Printf("LabelledBreak: %v\n", LabelledBreak())
	fmt.Printf("LabelledContinueOuter: %v\n", LabelledContinueOuter())
	fmt.Printf("BreakOutsideLoop: %v\n", []int{BreakOutsideLoop(1), BreakOutsideLoop(2)})
	fmt.Printf("LabelledSwitchBreak: %v\n", []int{LabelledSwitchBreak(1), LabelledSwitchBreak(2)})
	fmt.Printf("RangeOverFuncBreak: %v\n", RangeOverFuncBreak())
	fmt.Printf("RangeOverFuncContinue: %v\n", RangeOverFuncContinue())
}
