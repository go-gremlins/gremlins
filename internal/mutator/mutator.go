/*
 * Copyright 2022 The Gremlins Authors
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

// Package mutator provides mutation types, statuses, and interfaces for mutation testing.
package mutator

import "go/token"

// Status represents the status of a given TokenMutant.
//
//   - NotCovered means that a TokenMutant has been identified, but is not covered
//     by tests.
//   - Runnable means that a TokenMutant has been identified and is covered by tests,
//     which means it can be executed.
//   - Lived means that the TokenMutant has been tested, but the tests did pass, which
//     means the test suite is not effective in catching it.
//   - Killed means that the TokenMutant has been tested and the tests failed, which
//     means they are effective in covering this regression.
//   - TimedOut means that the deadline wrapping the WHOLE `go test` child —
//     resolve, compile, link, then run — expired. Which of those phases consumed
//     it is not known, so the mutant is unadjudicated: a compile that hung
//     reaches this status identically to a run that did.
//   - RunTimedOut means that the test BINARY's own -timeout watchdog fired and
//     said so in its output. That is a positive observation about the mutant
//     rather than about the machine: a test binary existed, the suite started,
//     and it did not finish inside a bound the Go toolchain starts when the
//     binary starts, so compilation cannot have consumed it.
//
// The two timeout statuses are kept apart because they carry different
// evidence, and a consumer that scores them alike scores a slow compiler and a
// non-terminating mutant identically. gremlins takes no position on which of
// them is a detection: neither appears in test_efficacy, exactly as before. The
// distinction is recorded so that a consumer can take one.
type Status int

// Currently supported MutantStatus.
const (
	NotCovered Status = iota
	Runnable
	Skipped
	Lived
	Killed
	NotViable
	TimedOut
	RunTimedOut
)

// Statuses allows to iterate over Status. Kept next to the constants so a new
// status cannot be added without the readers that enumerate them seeing it.
var Statuses = []Status{
	NotCovered,
	Runnable,
	Skipped,
	Lived,
	Killed,
	NotViable,
	TimedOut,
	RunTimedOut,
}

// ParseShutdownStatus maps the CLI/config value for "what status to assign
// to mutants that were still in-flight when the runner cancelled the run"
// to a concrete Status. The returned bool is false for unrecognised inputs.
//
// Accepted forms (case-insensitive): "not-run" (alias "notrun"),
// "timed-out" (aliases "timedout", "timeout"), "lived".
//
// "not-run" maps to NotCovered — the closest existing semantic for "we
// never got a chance to run this mutant", and avoids inventing a new
// Status value that would break downstream JSON consumers.
//
// "timed-out" maps to TimedOut and deliberately not to RunTimedOut. A mutant
// the runner cancelled was never adjudicated by anything, which is precisely
// what TimedOut means; RunTimedOut asserts that a test binary reported the
// overrun itself, and no shutdown can produce that evidence.
func ParseShutdownStatus(s string) (Status, bool) {
	switch s {
	case "not-run", "notrun", "":
		return NotCovered, true
	case "timed-out", "timedout", "timeout":
		return TimedOut, true
	case "lived":
		return Lived, true
	}
	return 0, false
}

func (ms Status) String() string {
	switch ms {
	case NotCovered:
		return "NOT COVERED"
	case Runnable:
		return "RUNNABLE"
	case Skipped:
		return "SKIPPED"
	case Lived:
		return "LIVED"
	case Killed:
		return "KILLED"
	case NotViable:
		return "NOT VIABLE"
	case TimedOut:
		return "TIMED OUT"
	case RunTimedOut:
		return "RUN TIMED OUT"
	default:
		panic("this should not happen")
	}
}

// Type represents the category of the TokenMutant.
//
// A single token.Token can be mutated in various ways depending on the
// specific mutation being tested.
// For example `<` can be mutated to `<=` in case of ConditionalsBoundary
// or `>=` in case of ConditionalsNegation.
type Type int

// The currently supported Type in Gremlins.
const (
	ArithmeticBase Type = iota
	ConditionalsBoundary
	ConditionalsNegation
	IncrementDecrement
	InvertAssignments
	InvertBitwise
	InvertBitwiseAssignments
	InvertLogical
	InvertLoopCtrl
	InvertNegatives
	RemoveSelfAssignments
)

// Types allows to iterate over Type.
var Types = []Type{
	ArithmeticBase,
	ConditionalsBoundary,
	ConditionalsNegation,
	InvertAssignments,
	InvertBitwise,
	InvertBitwiseAssignments,
	IncrementDecrement,
	InvertLogical,
	InvertLoopCtrl,
	InvertNegatives,
	RemoveSelfAssignments,
}

func (mt Type) String() string {
	switch mt {
	case ConditionalsBoundary:
		return "CONDITIONALS_BOUNDARY"
	case ConditionalsNegation:
		return "CONDITIONALS_NEGATION"
	case IncrementDecrement:
		return "INCREMENT_DECREMENT"
	case InvertLogical:
		return "INVERT_LOGICAL"
	case InvertNegatives:
		return "INVERT_NEGATIVES"
	case ArithmeticBase:
		return "ARITHMETIC_BASE"
	case InvertLoopCtrl:
		return "INVERT_LOOPCTRL"
	case InvertAssignments:
		return "INVERT_ASSIGNMENTS"
	case InvertBitwise:
		return "INVERT_BITWISE"
	case InvertBitwiseAssignments:
		return "INVERT_BWASSIGN"
	case RemoveSelfAssignments:
		return "REMOVE_SELF_ASSIGNMENTS"

	default:
		panic("this should not happen")
	}
}

// Mutator represents a possible mutation of the source code.
type Mutator interface {
	// Type returns the Type of the Mutator.
	Type() Type

	// SetType sets the Type of the Mutator.
	SetType(mt Type)

	// Status returns the Status of the Mutator.
	Status() Status

	// SetStatus sets the Status of the Mutator.
	SetStatus(s Status)

	// Position returns the token.Position for the Mutator.
	// token.Position consumes more space than token.Pos, and in the future
	// we can consider a refactoring to remove its use and only use Mutator.Pos.
	Position() token.Position

	// Pos returns the token.Pos of the Mutator.
	Pos() token.Pos

	// Pkg returns the package where the Mutator is fount.
	Pkg() string

	// SetWorkdir sets the working directory which contains the source code on
	// which the Mutator will apply its mutations.
	SetWorkdir(p string)

	// Workdir returns the current working dir in which the Mutator will apply its mutations
	Workdir() string

	// Apply applies the mutation on the actual source code.
	Apply() error

	// Rollback removes the mutation from the source code and sets it back to
	// its original status.
	Rollback() error

	// OrigSnippet returns the original code snippet around the mutation point.
	OrigSnippet() []byte

	// MutatedSnippet returns the mutated code snippet around the mutation point.
	MutatedSnippet() []byte
}
