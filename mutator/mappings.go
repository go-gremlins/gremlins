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

package mutator

import "go/token"

// MutantType represents the category of the Mutant.
//
// A single token.Token can be mutated in various ways depending on the
// specific mutation being tested.
// For example `<` can be mutated to `<=` in case of ConditionalsBoundary
// or `>=` in case of ConditionalsNegation.
type MutantType int

func (mt MutantType) String() string {
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

// The currently supported MutantType in Gremlins.
const (
	ConditionalsBoundary MutantType = iota
	ConditionalsNegation
	IncrementDecrement
	InvertNegatives
	ArithmeticBase
)

var mokenMutantType = map[token.Token][]MutantType{
	token.SUB: {InvertNegatives, ArithmeticBase},
	token.ADD: {ArithmeticBase},
	token.MUL: {ArithmeticBase},
	token.QUO: {ArithmeticBase},
	token.REM: {ArithmeticBase},
	token.EQL: {ConditionalsNegation},
	token.NEQ: {ConditionalsNegation},
	token.GTR: {ConditionalsBoundary, ConditionalsNegation},
	token.LSS: {ConditionalsBoundary, ConditionalsNegation},
	token.GEQ: {ConditionalsBoundary, ConditionalsNegation},
	token.LEQ: {ConditionalsBoundary, ConditionalsNegation},
	token.INC: {IncrementDecrement},
	token.DEC: {IncrementDecrement},
}

var mutations = map[MutantType]map[token.Token]token.Token{
	ArithmeticBase: {
		token.ADD: token.SUB,
		token.SUB: token.ADD,
		token.MUL: token.QUO,
		token.QUO: token.MUL,
		token.REM: token.MUL,
	},
	ConditionalsBoundary: {
		token.GTR: token.GEQ,
		token.LSS: token.LEQ,
		token.GEQ: token.GTR,
		token.LEQ: token.LSS,
	},
	ConditionalsNegation: {
		token.EQL: token.NEQ,
		token.NEQ: token.EQL,
		token.LEQ: token.GTR,
		token.GTR: token.LEQ,
		token.LSS: token.GEQ,
		token.GEQ: token.LSS,
	},
	IncrementDecrement: {
		token.INC: token.DEC,
		token.DEC: token.INC,
	},
	InvertNegatives: {
		token.SUB: token.ADD,
	},
}
