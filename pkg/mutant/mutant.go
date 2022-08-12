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

package mutant

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
type Status int

// Currently supported MutantStatus.
const (
	NotCovered Status = iota
	Runnable
	Lived
	Killed
	NotViable
	TimedOut
)

func (ms Status) String() string {
	switch ms {
	case NotCovered:
		return "NOT COVERED"
	case Runnable:
		return "RUNNABLE"
	case Lived:
		return "LIVED"
	case Killed:
		return "KILLED"
	case NotViable:
		return "NOT VIABLE"
	case TimedOut:
		return "TIMED OUT"
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
	InvertNegatives
)

// MutantTypes allows to iterate over Type.
var MutantTypes = []Type{
	ArithmeticBase,
	ConditionalsBoundary,
	ConditionalsNegation,
	IncrementDecrement,
	InvertNegatives,
}

func (mt Type) String() string {
	switch mt {
	case ConditionalsBoundary:
		return "CONDITIONALS_BOUNDARY"
	case ConditionalsNegation:
		return "CONDITIONALS_NEGATION"
	case IncrementDecrement:
		return "INCREMENT_DECREMENT"
	case InvertNegatives:
		return "INVERT_NEGATIVES"
	case ArithmeticBase:
		return "ARITHMETIC_BASE"
	default:
		panic("this should not happen")
	}
}

// Mutant represents a possible mutation of the source code.
type Mutant interface {
	// Type returns the Type of the Mutant.
	Type() Type

	// SetType sets the Type of the Mutant.
	SetType(mt Type)

	// Status returns the Status of the Mutant.
	Status() Status

	// SetStatus sets the Status of the Mutant.
	SetStatus(s Status)

	// Position returns the token.Position for the Mutant.
	// token.Position consumes more space than token.Pos, and in the future
	// we can consider a refactoring to remove its use and only use Mutant.Pos.
	Position() token.Position

	// Pos returns the token.Pos of the Mutant.
	Pos() token.Pos

	// Pkg returns the package where the Mutant is fount.
	Pkg() string

	// SetWorkdir sets the working directory which contains the source code on
	// which the Mutant will apply its mutations.
	SetWorkdir(p string)

	// Apply applies the mutation on the actual source code.
	Apply() error

	// Rollback removes the mutation from the source code and sets it back to
	// its original status.
	Rollback() error
}
