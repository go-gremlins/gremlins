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
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

func TestTokenMutator_Apply_SetsSnippets(t *testing.T) {
	src := "package main\n\nfunc main() {\n\ta := 1 + 2\n}\n"

	testCases := map[string]struct {
		assertFunc func(t *testing.T, mut *engine.TokenMutator, err error)
	}{
		"should_set_orig_snippet_when_apply_is_called": {
			assertFunc: func(t *testing.T, mut *engine.TokenMutator, err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
				want := []byte("package main\n\nfunc main() {\n\ta := 1 + 2\n}\n")
				if !cmp.Equal(want, mut.OrigSnippet()) {
					t.Fatal(cmp.Diff(string(want), string(mut.OrigSnippet())))
				}
			},
		},
		"should_set_mutated_snippet_when_apply_is_called": {
			assertFunc: func(t *testing.T, mut *engine.TokenMutator, err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
				want := []byte("package main\n\nfunc main() {\n\ta := 1 - 2\n}\n")
				if !cmp.Equal(want, mut.MutatedSnippet()) {
					t.Fatal(cmp.Diff(string(want), string(mut.MutatedSnippet())))
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			workdir := t.TempDir()
			filePath := "sourceFile.go"
			fileFullPath := filepath.Join(workdir, filePath)

			err := os.WriteFile(fileFullPath, []byte(src), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			set := token.NewFileSet()
			f, err := parser.ParseFile(set, filePath, src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			var node *ast.BinaryExpr
			ast.Inspect(f, func(n ast.Node) bool {
				if b, ok := n.(*ast.BinaryExpr); ok && node == nil {
					node = b
				}

				return true
			})
			if node == nil {
				t.Fatal("binary expression node must be found")
			}

			n, ok := engine.NewTokenNode(node)
			if !ok {
				t.Fatal("token node must be created")
			}

			mut := engine.NewTokenMutant("example.com/test", set, f, n)
			mut.SetType(mutator.ArithmeticBase)
			mut.SetStatus(mutator.Runnable)
			mut.SetWorkdir(workdir)

			applyErr := mut.Apply()

			tc.assertFunc(t, mut, applyErr)
		})
	}
}

func TestMutantApplyAndRollback(t *testing.T) {
	want := []string{
		"package main\n\nfunc main() {\n\ta := 1 - 2\n\tb := 1 - 2\n}\n",
		"package main\n\nfunc main() {\n\ta := 1 + 2\n\tb := 1 + 2\n}\n",
	}
	rollbackWant := "package main\n\nfunc main() {\n\ta := 1 + 2\n\tb := 1 - 2\n}\n"

	workdir := t.TempDir()
	filePath := "sourceFile.go"
	fileFullPath := filepath.Join(workdir, filePath)

	err := os.WriteFile(fileFullPath, []byte(rollbackWant), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	set := token.NewFileSet()
	f, err := parser.ParseFile(set, filePath, rollbackWant, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []*ast.BinaryExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if n, ok := n.(*ast.BinaryExpr); ok {
			nodes = append(nodes, n)
		}

		return true
	})

	for i, node := range nodes {
		n, ok := engine.NewTokenNode(node)
		if !ok {
			t.Fatal("new actualToken node should be created")
		}
		mut := engine.NewTokenMutant("example.com/test", set, f, n)
		mut.SetType(mutator.ArithmeticBase)
		mut.SetStatus(mutator.Runnable)
		mut.SetWorkdir(workdir)

		err = mut.Apply()
		if err != nil {
			t.Fatal(err)
		}

		//nolint:gosec // test code reading test file
		got, err := os.ReadFile(fileFullPath)
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(string(got), want[i]) {
			t.Fatal(cmp.Diff(want[i], string(got)))
		}

		err = mut.Rollback()
		if err != nil {
			t.Fatal(err)
		}

		//nolint:gosec // test code reading test file
		got, err = os.ReadFile(fileFullPath)
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(string(got), rollbackWant) {
			t.Fatal(cmp.Diff(rollbackWant, string(got)))
		}
	}
}
