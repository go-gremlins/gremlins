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

package engine_test

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

func TestGetMutantTypesForToken_SUB_UnaryExpr(t *testing.T) {
	// Create a UnaryExpr node with SUB token (represents -x)
	node := &ast.UnaryExpr{
		Op: token.SUB,
		X:  &ast.Ident{Name: "x"},
	}

	types := engine.GetMutantTypesForToken(token.SUB, node)

	// Should only get InvertNegatives for unary minus
	if len(types) != 1 {
		t.Fatalf("expected 1 mutation type, got %d", len(types))
	}

	if types[0] != mutator.InvertNegatives {
		t.Errorf("expected InvertNegatives, got %s", types[0])
	}
}

func TestGetMutantTypesForToken_SUB_BinaryExpr(t *testing.T) {
	// Create a BinaryExpr node with SUB token (represents a - b)
	node := &ast.BinaryExpr{
		X:  &ast.Ident{Name: "a"},
		Op: token.SUB,
		Y:  &ast.Ident{Name: "b"},
	}

	types := engine.GetMutantTypesForToken(token.SUB, node)

	// Should only get ArithmeticBase for binary subtraction
	if len(types) != 1 {
		t.Fatalf("expected 1 mutation type, got %d", len(types))
	}

	if types[0] != mutator.ArithmeticBase {
		t.Errorf("expected ArithmeticBase, got %s", types[0])
	}
}

func TestGetMutantTypesForToken_NonAmbiguousToken(t *testing.T) {
	// Test that non-ambiguous tokens still work correctly
	node := &ast.BinaryExpr{
		X:  &ast.Ident{Name: "a"},
		Op: token.ADD,
		Y:  &ast.Ident{Name: "b"},
	}

	types := engine.GetMutantTypesForToken(token.ADD, node)

	// ADD should only have ArithmeticBase
	if len(types) != 1 {
		t.Fatalf("expected 1 mutation type, got %d", len(types))
	}

	if types[0] != mutator.ArithmeticBase {
		t.Errorf("expected ArithmeticBase, got %s", types[0])
	}
}

func TestGetMutantTypesForToken_UnsupportedToken(t *testing.T) {
	node := &ast.BinaryExpr{
		X:  &ast.Ident{Name: "a"},
		Op: token.ILLEGAL,
		Y:  &ast.Ident{Name: "b"},
	}

	types := engine.GetMutantTypesForToken(token.ILLEGAL, node)

	// ILLEGAL token should return nil
	if types != nil {
		t.Errorf("expected nil for unsupported token, got %v", types)
	}
}
