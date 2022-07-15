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

package mutator_test

import (
	"github.com/k3rn31/gremlins/mutator"
	"go/ast"
	"go/token"
	"testing"
)

func TestNewTokenNode(t *testing.T) {
	testCases := []struct {
		name      string
		node      ast.Node
		wantTok   token.Token
		wantPos   token.Pos
		supported bool
	}{
		{
			name: "BinaryExpr",
			node: &ast.BinaryExpr{
				X:     nil,
				OpPos: 123,
				Op:    token.ADD,
				Y:     nil,
			},
			wantTok:   token.ADD,
			wantPos:   123,
			supported: true,
		},
		{
			name: "UnaryExpr",
			node: &ast.UnaryExpr{
				X:     nil,
				OpPos: 123,
				Op:    token.ADD,
			},
			wantTok:   token.ADD,
			wantPos:   123,
			supported: true,
		},
		{
			name: "IncDecStmt",
			node: &ast.IncDecStmt{
				X:      nil,
				TokPos: 123,
				Tok:    token.INC,
			},
			wantTok:   token.INC,
			wantPos:   123,
			supported: true,
		},
		{
			name: "not supported",
			node: &ast.BasicLit{
				ValuePos: 123,
			},
			supported: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tn, ok := mutator.NewTokenNode(tc.node)
			if ok != tc.supported {
				t.Fatal("expected to receive a token")
			}
			if !tc.supported {
				return
			}

			gotTok := tn.Tok()
			gotPos := tn.TokPos

			if gotTok != tc.wantTok {
				t.Errorf("want %s, got %s", gotTok, tc.wantTok)
			}
			if gotPos != tc.wantPos {
				t.Errorf("want %d, got %d", gotPos, tc.wantPos)
			}
		})
	}

}
