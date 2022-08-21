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

package internal

import (
	"go/token"

	"github.com/go-gremlins/gremlins/pkg/mutant"
)

// TokenMutantType is the mapping from each token.Token and all the
// mutant.Type that can be applied to it.
var TokenMutantType = map[token.Token][]mutant.Type{
	token.SUB:  {mutant.InvertNegatives, mutant.ArithmeticBase},
	token.ADD:  {mutant.ArithmeticBase},
	token.MUL:  {mutant.ArithmeticBase},
	token.QUO:  {mutant.ArithmeticBase},
	token.REM:  {mutant.ArithmeticBase},
	token.EQL:  {mutant.ConditionalsNegation},
	token.NEQ:  {mutant.ConditionalsNegation},
	token.GTR:  {mutant.ConditionalsBoundary, mutant.ConditionalsNegation},
	token.LSS:  {mutant.ConditionalsBoundary, mutant.ConditionalsNegation},
	token.GEQ:  {mutant.ConditionalsBoundary, mutant.ConditionalsNegation},
	token.LEQ:  {mutant.ConditionalsBoundary, mutant.ConditionalsNegation},
	token.INC:  {mutant.IncrementDecrement},
	token.DEC:  {mutant.IncrementDecrement},
	token.LAND: {mutant.InvertLogical},
	token.LOR:  {mutant.InvertLogical},
}

var tokenMutations = map[mutant.Type]map[token.Token]token.Token{
	mutant.ArithmeticBase: {
		token.ADD: token.SUB,
		token.SUB: token.ADD,
		token.MUL: token.QUO,
		token.QUO: token.MUL,
		token.REM: token.MUL,
	},
	mutant.ConditionalsBoundary: {
		token.GTR: token.GEQ,
		token.LSS: token.LEQ,
		token.GEQ: token.GTR,
		token.LEQ: token.LSS,
	},
	mutant.ConditionalsNegation: {
		token.EQL: token.NEQ,
		token.NEQ: token.EQL,
		token.LEQ: token.GTR,
		token.GTR: token.LEQ,
		token.LSS: token.GEQ,
		token.GEQ: token.LSS,
	},
	mutant.IncrementDecrement: {
		token.INC: token.DEC,
		token.DEC: token.INC,
	},
	mutant.InvertLogical: {
		token.LAND: token.LOR,
		token.LOR:  token.LAND,
	},
	mutant.InvertNegatives: {
		token.SUB: token.ADD,
	},
}
