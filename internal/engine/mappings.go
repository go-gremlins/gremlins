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
	token.ADD:        {mutator.ArithmeticBase},
	token.ADD_ASSIGN: {mutator.InvertAssignments},
	token.AND:        {mutator.InvertBitwise},
	token.AND_NOT:    {mutator.InvertBitwise},
	token.BREAK:      {mutator.InvertLoopCtrl},
	token.CONTINUE:   {mutator.InvertLoopCtrl},
	token.DEC:        {mutator.IncrementDecrement},
	token.EQL:        {mutator.ConditionalsNegation},
	token.GEQ:        {mutator.ConditionalsBoundary, mutator.ConditionalsNegation},
	token.GTR:        {mutator.ConditionalsBoundary, mutator.ConditionalsNegation},
	token.INC:        {mutator.IncrementDecrement},
	token.LAND:       {mutator.InvertLogical},
	token.LEQ:        {mutator.ConditionalsBoundary, mutator.ConditionalsNegation},
	token.LOR:        {mutator.InvertLogical},
	token.LSS:        {mutator.ConditionalsBoundary, mutator.ConditionalsNegation},
	token.MUL:        {mutator.ArithmeticBase},
	token.MUL_ASSIGN: {mutator.InvertAssignments},
	token.NEQ:        {mutator.ConditionalsNegation},
	token.OR:         {mutator.InvertBitwise},
	token.QUO:        {mutator.ArithmeticBase},
	token.QUO_ASSIGN: {mutator.InvertAssignments},
	token.REM:        {mutator.ArithmeticBase},
	token.REM_ASSIGN: {mutator.InvertAssignments},
	token.SHL:        {mutator.InvertBitwise},
	token.SHR:        {mutator.InvertBitwise},
	token.SUB:        {mutator.InvertNegatives, mutator.ArithmeticBase},
	token.SUB_ASSIGN: {mutator.InvertAssignments},
	token.XOR:        {mutator.InvertBitwise},
}

var tokenMutations = map[mutator.Type]map[token.Token]token.Token{
	mutator.ArithmeticBase: {
		token.ADD: token.SUB,
		token.MUL: token.QUO,
		token.QUO: token.MUL,
		token.REM: token.MUL,
		token.SUB: token.ADD,
	},
	mutator.ConditionalsBoundary: {
		token.GEQ: token.GTR,
		token.GTR: token.GEQ,
		token.LEQ: token.LSS,
		token.LSS: token.LEQ,
	},
	mutator.ConditionalsNegation: {
		token.EQL: token.NEQ,
		token.GEQ: token.LSS,
		token.GTR: token.LEQ,
		token.LEQ: token.GTR,
		token.LSS: token.GEQ,
		token.NEQ: token.EQL,
	},
	mutator.IncrementDecrement: {
		token.DEC: token.INC,
		token.INC: token.DEC,
	},
	mutator.InvertAssignments: {
		token.ADD_ASSIGN: token.SUB_ASSIGN,
		token.MUL_ASSIGN: token.QUO_ASSIGN,
		token.QUO_ASSIGN: token.MUL_ASSIGN,
		token.REM_ASSIGN: token.REM_ASSIGN,
		token.SUB_ASSIGN: token.ADD_ASSIGN,
	},
	mutator.InvertLogical: {
		token.LAND: token.LOR,
		token.LOR:  token.LAND,
	},
	mutator.InvertLoopCtrl: {
		token.BREAK:    token.CONTINUE,
		token.CONTINUE: token.BREAK,
	},
	mutator.InvertNegatives: {
		token.SUB: token.ADD,
	},
	mutator.InvertBitwise: {
		token.AND:     token.OR,
		token.OR:      token.AND,
		token.XOR:     token.AND,
		token.AND_NOT: token.AND,
		token.SHL:     token.SHR,
		token.SHR:     token.SHL,
	},
}
