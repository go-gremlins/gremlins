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

package engine

import (
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// documentedMutations is the mutation table as docs/docs/usage/mutations/
// publishes it, one entry per row of each type's table. It is written out by
// hand on purpose: a test that derived its expectation from tokenMutations
// could only ever agree with it.
var documentedMutations = map[mutator.Type]map[token.Token]token.Token{
	mutator.ArithmeticBase: {
		token.ADD: token.SUB,
		token.SUB: token.ADD,
		token.MUL: token.QUO,
		token.QUO: token.MUL,
		token.REM: token.MUL,
	},
	mutator.ConditionalsBoundary: {
		token.GTR: token.GEQ,
		token.GEQ: token.GTR,
		token.LSS: token.LEQ,
		token.LEQ: token.LSS,
	},
	mutator.ConditionalsNegation: {
		token.EQL: token.NEQ,
		token.NEQ: token.EQL,
		token.GTR: token.LEQ,
		token.LEQ: token.GTR,
		token.LSS: token.GEQ,
		token.GEQ: token.LSS,
	},
	mutator.IncrementDecrement: {
		token.INC: token.DEC,
		token.DEC: token.INC,
	},
	mutator.InvertAssignments: {
		token.ADD_ASSIGN: token.SUB_ASSIGN,
		token.SUB_ASSIGN: token.ADD_ASSIGN,
		token.MUL_ASSIGN: token.QUO_ASSIGN,
		token.QUO_ASSIGN: token.MUL_ASSIGN,
		token.REM_ASSIGN: token.MUL_ASSIGN,
	},
	mutator.InvertBitwise: {
		token.AND:     token.OR,
		token.OR:      token.AND,
		token.XOR:     token.AND,
		token.AND_NOT: token.AND,
		token.SHR:     token.SHL,
		token.SHL:     token.SHR,
	},
	mutator.InvertBitwiseAssignments: {
		token.AND_ASSIGN:     token.OR_ASSIGN,
		token.OR_ASSIGN:      token.AND_ASSIGN,
		token.XOR_ASSIGN:     token.AND_ASSIGN,
		token.AND_NOT_ASSIGN: token.AND_ASSIGN,
		token.SHR_ASSIGN:     token.SHL_ASSIGN,
		token.SHL_ASSIGN:     token.SHR_ASSIGN,
	},
	mutator.InvertLogical: {
		token.LAND: token.LOR,
		token.LOR:  token.LAND,
	},
	mutator.InvertLoopCtrl: {
		token.CONTINUE: token.BREAK,
		token.BREAK:    token.CONTINUE,
	},
	mutator.InvertNegatives: {
		token.SUB: token.ADD,
	},
	mutator.RemoveSelfAssignments: {
		token.ADD_ASSIGN:     token.ASSIGN,
		token.SUB_ASSIGN:     token.ASSIGN,
		token.MUL_ASSIGN:     token.ASSIGN,
		token.QUO_ASSIGN:     token.ASSIGN,
		token.REM_ASSIGN:     token.ASSIGN,
		token.AND_ASSIGN:     token.ASSIGN,
		token.OR_ASSIGN:      token.ASSIGN,
		token.XOR_ASSIGN:     token.ASSIGN,
		token.SHL_ASSIGN:     token.ASSIGN,
		token.SHR_ASSIGN:     token.ASSIGN,
		token.AND_NOT_ASSIGN: token.ASSIGN,
	},
}

// TestTokenMutationsMatchTheDocumentedTable pins what each token becomes. The
// tests that drive the engine over fixtures only check that a mutation type is
// discovered at a token, never what the token is rewritten to, so before this
// a table entry could map to anything -- REM_ASSIGN once mapped to itself,
// producing an INVERT_ASSIGNMENTS mutant identical to the original source.
func TestTokenMutationsMatchTheDocumentedTable(t *testing.T) {
	t.Parallel()

	if diff := cmp.Diff(documentedMutations, tokenMutations); diff != "" {
		t.Errorf("tokenMutations differs from the documented table (-documented +actual):\n%s", diff)
	}
}

// TestNoTokenMutationIsTheIdentity guards the failure that matters most on its
// own: a mutant equal to its original can never be killed, so every one of them
// reads as a surviving mutant the test suite is blamed for.
func TestNoTokenMutationIsTheIdentity(t *testing.T) {
	t.Parallel()

	for mt, table := range tokenMutations {
		for from, to := range table {
			if from == to {
				t.Errorf("%s maps %s to itself", mt, from)
			}
		}
	}
}

// TestTokenMutantTypeAndTokenMutationsAgree checks that the discovery table and
// the rewrite table describe the same set of (token, type) pairs. A pair only
// in TokenMutantType is a mutant that is discovered and then rewritten to the
// zero token; a pair only in tokenMutations is a rewrite nothing reaches.
func TestTokenMutantTypeAndTokenMutationsAgree(t *testing.T) {
	t.Parallel()

	for tok, types := range TokenMutantType {
		for _, mt := range types {
			if _, ok := tokenMutations[mt][tok]; !ok {
				t.Errorf("TokenMutantType gives %s to %s, but tokenMutations has no rewrite for it", mt, tok)
			}
		}
	}
	for mt, table := range tokenMutations {
		for tok := range table {
			found := false
			for _, got := range TokenMutantType[tok] {
				if got == mt {
					found = true
				}
			}
			if !found {
				t.Errorf("tokenMutations rewrites %s under %s, but TokenMutantType never discovers it", tok, mt)
			}
		}
	}
}
