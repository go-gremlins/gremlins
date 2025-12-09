/*
 * Copyright 2024 The Gremlins Authors
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
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

func TestExprMutatorApplyAndRollback(t *testing.T) {
	testCases := []struct {
		name         string
		original     string
		mutated      string
		mutationType mutator.Type
	}{
		{
			name: "invert logical not in simple expression",
			original: `package main

func main() {
	a := !true
}
`,
			mutated: `package main

func main() {
	a := !!true
}
`,
			mutationType: mutator.InvertLogicalNot,
		},
		{
			name: "invert logical not in if condition",
			original: `package main

func main() {
	if !condition {
		return
	}
}
`,
			mutated: `package main

func main() {
	if !!condition {
		return
	}
}
`,
			mutationType: mutator.InvertLogicalNot,
		},
		{
			name: "invert logical not in complex expression",
			original: `package main

func main() {
	result := !someFunc()
}
`,
			mutated: `package main

func main() {
	result := !!someFunc()
}
`,
			mutationType: mutator.InvertLogicalNot,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workdir := t.TempDir()
			filePath := "sourceFile.go"
			fileFullPath := filepath.Join(workdir, filePath)

			err := os.WriteFile(fileFullPath, []byte(tc.original), 0600)
			if err != nil {
				t.Fatal(err)
			}

			set := token.NewFileSet()
			f, err := parser.ParseFile(set, filePath, tc.original, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			// Find the first UnaryExpr with NOT operator
			var foundNode *ast.UnaryExpr
			var parentNode ast.Node
			var replaceFunc func(newExpr ast.Expr) error

			ast.Inspect(f, func(n ast.Node) bool {
				if foundNode != nil {
					return false
				}
				if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
					foundNode = unary
					// Find parent and create replacer
					parentNode, replaceFunc = findParentAndReplacerForTest(f, unary)

					return false
				}

				return true
			})

			if foundNode == nil {
				t.Fatal("no UnaryExpr with NOT found")
			}

			exprNode, ok := engine.NewExprNode(foundNode)
			if !ok {
				t.Fatal("new expr node should be created")
			}

			mut := engine.NewExprMutant("example.com/test", set, f, exprNode, parentNode, replaceFunc)
			mut.SetType(tc.mutationType)
			mut.SetStatus(mutator.Runnable)
			mut.SetWorkdir(workdir)

			// Test Apply
			err = mut.Apply()
			if err != nil {
				t.Fatalf("Apply failed: %v", err)
			}

			//nolint:gosec // test code reading test file
			got, err := os.ReadFile(fileFullPath)
			if err != nil {
				t.Fatal(err)
			}
			if !cmp.Equal(string(got), tc.mutated) {
				t.Errorf("After Apply:\n%s", cmp.Diff(tc.mutated, string(got)))
			}

			// Test Rollback
			err = mut.Rollback()
			if err != nil {
				t.Fatalf("Rollback failed: %v", err)
			}

			//nolint:gosec // test code reading test file
			got, err = os.ReadFile(fileFullPath)
			if err != nil {
				t.Fatal(err)
			}
			if !cmp.Equal(string(got), tc.original) {
				t.Errorf("After Rollback:\n%s", cmp.Diff(tc.original, string(got)))
			}
		})
	}
}

func TestExprMutatorTypeAndStatus(t *testing.T) {
	workdir := t.TempDir()
	filePath := "test.go"
	code := "package main\nfunc f() { _ = !true }"

	err := os.WriteFile(filepath.Join(workdir, filePath), []byte(code), 0600)
	if err != nil {
		t.Fatal(err)
	}

	set := token.NewFileSet()
	f, err := parser.ParseFile(set, filePath, code, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	var foundNode *ast.UnaryExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if u, ok := n.(*ast.UnaryExpr); ok && u.Op == token.NOT {
			foundNode = u

			return false
		}

		return true
	})

	exprNode, _ := engine.NewExprNode(foundNode)
	parent, replacer := findParentAndReplacerForTest(f, foundNode)
	mut := engine.NewExprMutant("example.com/test", set, f, exprNode, parent, replacer)

	// Test SetType/Type
	mut.SetType(mutator.InvertLogicalNot)
	if got := mut.Type(); got != mutator.InvertLogicalNot {
		t.Errorf("Type() = %v, want %v", got, mutator.InvertLogicalNot)
	}

	// Test SetStatus/Status
	mut.SetStatus(mutator.Killed)
	if got := mut.Status(); got != mutator.Killed {
		t.Errorf("Status() = %v, want %v", got, mutator.Killed)
	}

	// Test Pkg
	if got := mut.Pkg(); got != "example.com/test" {
		t.Errorf("Pkg() = %q, want %q", got, "example.com/test")
	}

	// Test SetWorkdir/Workdir
	mut.SetWorkdir(workdir)
	if got := mut.Workdir(); got != workdir {
		t.Errorf("Workdir() = %q, want %q", got, workdir)
	}

	// Test Position
	pos := mut.Position()
	if pos.Filename != filePath {
		t.Errorf("Position().Filename = %q, want %q", pos.Filename, filePath)
	}
}

func TestExprMutatorInvalidMutationType(t *testing.T) {
	workdir := t.TempDir()
	filePath := "test.go"
	code := "package main\nfunc f() { _ = !true }"

	err := os.WriteFile(filepath.Join(workdir, filePath), []byte(code), 0600)
	if err != nil {
		t.Fatal(err)
	}

	set := token.NewFileSet()
	f, err := parser.ParseFile(set, filePath, code, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	var foundNode *ast.UnaryExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if u, ok := n.(*ast.UnaryExpr); ok && u.Op == token.NOT {
			foundNode = u

			return false
		}

		return true
	})

	exprNode, _ := engine.NewExprNode(foundNode)
	parent, replacer := findParentAndReplacerForTest(f, foundNode)
	mut := engine.NewExprMutant("example.com/test", set, f, exprNode, parent, replacer)
	mut.SetType(mutator.ArithmeticBase) // Invalid type for expression mutator
	mut.SetWorkdir(workdir)

	err = mut.Apply()
	if err == nil {
		t.Error("Apply with invalid mutation type should fail")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("Expected 'not yet implemented' error, got: %v", err)
	}
}

// Helper function to find parent and create replacer for testing.
func findParentAndReplacerForTest(file *ast.File, target ast.Expr) (ast.Node, func(ast.Expr) error) {
	var parent ast.Node
	var replacer func(ast.Expr) error

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range node.Rhs {
				if rhs == target {
					parent = node
					replacer = func(newExpr ast.Expr) error {
						node.Rhs[i] = newExpr

						return nil
					}

					return false
				}
			}
		case *ast.IfStmt:
			if node.Cond == target {
				parent = node
				replacer = func(newExpr ast.Expr) error {
					node.Cond = newExpr

					return nil
				}

				return false
			}
		case *ast.ReturnStmt:
			for i, result := range node.Results {
				if result == target {
					parent = node
					replacer = func(newExpr ast.Expr) error {
						node.Results[i] = newExpr

						return nil
					}

					return false
				}
			}
		}

		return true
	})

	return parent, replacer
}
