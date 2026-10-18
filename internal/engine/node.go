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
	"go/ast"
	"go/token"
)

// NodeToken is the reference to the actualToken that will be mutated during
// the mutation testing.
type NodeToken struct {
	tok    *token.Token
	TokPos token.Pos
	unary  bool
}

// NewTokenNode checks if the ast.Node implementation is supported by
// Gremlins and gets its Tok/Op and relative position.
// It returns false as second parameter if the implementation is not
// supported.
func NewTokenNode(n ast.Node) (*NodeToken, bool) {
	var tok *token.Token
	var pos token.Pos
	var unary bool
	switch n := n.(type) {
	case *ast.AssignStmt:
		tok = &n.Tok
		pos = n.TokPos
	case *ast.BinaryExpr:
		tok = &n.Op
		pos = n.OpPos
	case *ast.BranchStmt:
		tok = &n.Tok
		pos = n.TokPos
	case *ast.IncDecStmt:
		tok = &n.Tok
		pos = n.TokPos
	case *ast.UnaryExpr:
		tok = &n.Op
		pos = n.OpPos
		unary = true
	default:
		return &NodeToken{}, false
	}

	return &NodeToken{
		tok:    tok,
		TokPos: pos,
		unary:  unary,
	}, true
}

// IsUnary reports whether the token was read as the prefix operator of an
// *ast.UnaryExpr rather than as an infix operator or a statement token. Go
// spells four operators the same in both positions -- `+`, `-`, `&` and `^` --
// and means something different by each, so the position a token was read in
// decides which mutations may be applied to it. See unaryMutableOperators.
func (n *NodeToken) IsUnary() bool {
	return n.unary
}

// Tok returns the reference to the token.Token.
func (n *NodeToken) Tok() token.Token {
	return *n.tok
}

// SetTok sets the token.Token of the tokenNode.
func (n *NodeToken) SetTok(t token.Token) {
	*n.tok = t
}
