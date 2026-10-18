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
	"go/parser"
	"go/token"
	"testing"

	"github.com/go-gremlins/gremlins/internal/coverage"
)

// closureGapSource has a condition inside a closure that is in no block of
// the profile below, while the `go` statement that creates the closure is.
const closureGapSource = `package p

func F(x int) {
	go func() {
		if x > 0 {
		}
	}()
}
`

// TestTheGapWalkStopsAtTheEnclosingFunction pins that a gap inside a closure is
// never credited with the statement that creates the closure: creating it
// does not run it. The profile is synthetic, recording the `go` statement's
// line and nothing inside the closure, which is the one shape that makes the
// walk reach the closure boundary before any instrumented statement.
func TestTheGapWalkStopsAtTheEnclosingFunction(t *testing.T) {
	set := token.NewFileSet()
	f, err := parser.ParseFile(set, "p.go", closureGapSource, 0)
	if err != nil {
		t.Fatal(err)
	}
	const goStmtLine = 4
	prof := coverage.Profile{"p.go": {{StartLine: goStmtLine, StartCol: 2, EndLine: goStmtLine, EndCol: 12}}}
	mu := &Engine{codeData: CodeData{Cov: prof}}

	var target token.Pos
	var path []ast.Node
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]

			return true
		}
		stack = append(stack, n)
		if b, ok := n.(*ast.BinaryExpr); ok && b.Op == token.GTR {
			target = b.OpPos
			path = append([]ast.Node(nil), stack...)
		}

		return true
	})
	if target == token.NoPos {
		t.Fatal("the condition was not found")
	}
	if mu.reached(set, target, path) {
		t.Fatal("a condition inside a closure was credited with the statement that creates the closure")
	}
	// The control: the enclosing `go` statement is covered, so without the
	// stop at the closure the walk would have returned true.
	var goStmt ast.Node
	for _, n := range path {
		if _, ok := n.(*ast.GoStmt); ok {
			goStmt = n
		}
	}
	if goStmt == nil || !prof.IsCovered(set.Position(goStmt.Pos())) {
		t.Fatal("the control does not hold: the go statement is not covered")
	}
}
