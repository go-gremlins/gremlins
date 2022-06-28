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
	"github.com/google/go-cmp/cmp"
	"github.com/k3rn31/gremlins/mutator"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

func TestMutantApplyAndRollback(t *testing.T) {
	t.Parallel()
	want := "package main\n\nfunc main() {\n	a := 1 - 2\n}\n"
	rollbackWant := "package main\n\nfunc main() {\n	a := 1 + 2\n}\n"

	workdir := t.TempDir()
	filePath := "sourceFile.go"
	fileFullPath := workdir + "/" + filePath

	err := os.WriteFile(fileFullPath, []byte(rollbackWant), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	set := token.NewFileSet()
	f, err := parser.ParseFile(set, filePath, rollbackWant, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var node *ast.BinaryExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if n, ok := n.(*ast.BinaryExpr); ok {
			node = n
		}
		return true
	})

	mut := mutator.NewMutant(set, f, node)
	mut.SetWorkdir(workdir)
	mut.TokPos = token.Pos(39)
	mut.Type = mutator.ArithmeticBase
	mut.Status = mutator.Runnable
	mut.ApplyF = func() { node.Op = token.SUB }
	mut.RollbackF = func() { node.Op = token.ADD }

	err = mut.Apply()
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(fileFullPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(string(got), want) {
		t.Fatalf(cmp.Diff(want, string(got)))
	}

	err = mut.Rollback()
	if err != nil {
		t.Fatal(err)
	}

	got, err = os.ReadFile(fileFullPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(string(got), rollbackWant) {
		t.Fatalf(cmp.Diff(rollbackWant, string(got)))
	}
}

func TestMutationStatusString(t *testing.T) {
	testCases := []struct {
		name           string
		mutationStatus mutator.MutantStatus
		expected       string
	}{
		{
			"NotCovered",
			mutator.NotCovered,
			"NOT COVERED",
		},
		{
			"Runnable",
			mutator.Runnable,
			"RUNNABLE",
		},
		{
			"Lived",
			mutator.Lived,
			"LIVED",
		},
		{
			"Killed",
			mutator.Killed,
			"KILLED",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutationStatus.String() != tc.expected {
				t.Errorf(cmp.Diff(tc.mutationStatus.String(), tc.expected))
			}
		})
	}
}
