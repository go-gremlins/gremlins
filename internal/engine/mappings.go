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

package engine

import (
	"go/token"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// TokenMutantType is the mapping from each token.Token and all the
// mutator.Type that can be applied to it.
var TokenMutantType = map[token.Token][]mutator.Type{
	token.SUB:      {mutator.InvertNegatives, mutator.ArithmeticBase},
	token.ADD:      {mutator.ArithmeticBase},
	token.MUL:      {mutator.ArithmeticBase},
	token.QUO:      {mutator.ArithmeticBase},
	token.REM:      {mutator.ArithmeticBase},
	token.EQL:      {mutator.ConditionalsNegation},
	token.NEQ:      {mutator.ConditionalsNegation},
	token.GTR:      {mutator.ConditionalsBoundary, mutator.ConditionalsNegation},
	token.LSS:      {mutator.ConditionalsBoundary, mutator.ConditionalsNegation},
	token.GEQ:      {mutator.ConditionalsBoundary, mutator.ConditionalsNegation},
	token.LEQ:      {mutator.ConditionalsBoundary, mutator.ConditionalsNegation},
	token.INC:      {mutator.IncrementDecrement},
	token.DEC:      {mutator.IncrementDecrement},
	token.LAND:     {mutator.InvertLogical},
	token.LOR:      {mutator.InvertLogical},
	token.BREAK:    {mutator.InvertLoopCtrl},
	token.CONTINUE: {mutator.InvertLoopCtrl},
}

var tokenMutations = map[mutator.Type]map[token.Token]token.Token{
	mutator.ArithmeticBase: {
		token.ADD: token.SUB,
		token.SUB: token.ADD,
		token.MUL: token.QUO,
		token.QUO: token.MUL,
		token.REM: token.MUL,
	},
	mutator.ConditionalsBoundary: {
		token.GTR: token.GEQ,
		token.LSS: token.LEQ,
		token.GEQ: token.GTR,
		token.LEQ: token.LSS,
	},
	mutator.ConditionalsNegation: {
		token.EQL: token.NEQ,
		token.NEQ: token.EQL,
		token.LEQ: token.GTR,
		token.GTR: token.LEQ,
		token.LSS: token.GEQ,
		token.GEQ: token.LSS,
	},
	mutator.IncrementDecrement: {
		token.INC: token.DEC,
		token.DEC: token.INC,
	},
	mutator.InvertLogical: {
		token.LAND: token.LOR,
		token.LOR:  token.LAND,
	},
	mutator.InvertNegatives: {
		token.SUB: token.ADD,
	},
	mutator.InvertLoopCtrl: {
		token.BREAK:    token.CONTINUE,
		token.CONTINUE: token.BREAK,
	},
}
