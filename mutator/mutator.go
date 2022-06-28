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

package mutator

import (
	"fmt"
	"github.com/k3rn31/gremlins/coverage"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
)

type Mutator struct {
	covProfile  coverage.Profile
	fs          fs.FS
	dryRun      bool
	execContext execContext
	apply       func(m Mutant) error
	rollback    func(m Mutant) error
}

type execContext = func(name string, args ...string) *exec.Cmd

type Option func(m Mutator) Mutator

func New(fs fs.FS, p coverage.Profile, opts ...Option) Mutator {
	mut := Mutator{covProfile: p,
		fs:          fs,
		execContext: exec.Command,
		apply: func(m Mutant) error {
			return m.Apply()
		},
		rollback: func(m Mutant) error {
			return m.Rollback()
		},
	}
	for _, opt := range opts {
		mut = opt(mut)
	}
	return mut
}

func WithDryRun(d bool) Option {
	return func(m Mutator) Mutator {
		m.dryRun = d
		return m
	}
}

func WithExecContext(c execContext) Option {
	return func(m Mutator) Mutator {
		m.execContext = c
		return m
	}
}

func WithApplyAndRollback(a func(m Mutant) error, r func(m Mutant) error) Option {
	return func(m Mutator) Mutator {
		m.apply = a
		m.rollback = r
		return m
	}
}

func (mu Mutator) Run() []Mutant {
	mutantStream := make(chan Mutant)
	go func() {
		_ = fs.WalkDir(mu.fs, ".", func(path string, d fs.DirEntry, err error) error {
			if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
				src, _ := mu.fs.Open(path)
				mu.runOnFile(path, src, mutantStream)
			}
			return nil
		})
		close(mutantStream)
	}()

	return mu.executeTests(mutantStream)
}

func (mu Mutator) runOnFile(fileName string, src io.Reader, ch chan<- Mutant) {
	set := token.NewFileSet()
	file, _ := parser.ParseFile(set, fileName, src, parser.ParseComments)
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.UnaryExpr:
			tok := n.Op
			r, ok := mu.mutants(set, file, node, tok, n.OpPos)
			if !ok {
				return true
			}
			for _, m := range r {
				m.ApplyF = func() { n.Op = mutations[m.Type][tok] }
				m.RollbackF = func() { n.Op = tok }
				ch <- m
			}
		case *ast.BinaryExpr:
			tok := n.Op
			r, ok := mu.mutants(set, file, node, n.Op, n.OpPos)
			if !ok {
				return true
			}
			for _, m := range r {
				m.ApplyF = func() { n.Op = mutations[m.Type][tok] }
				m.RollbackF = func() { n.Op = tok }
				ch <- m
			}
		case *ast.IncDecStmt:
			tok := n.Tok
			r, ok := mu.mutants(set, file, node, n.Tok, n.TokPos)
			if !ok {
				return true
			}
			for _, m := range r {
				m.ApplyF = func() { n.Tok = mutations[m.Type][tok] }
				m.RollbackF = func() { n.Tok = tok }
				ch <- m
			}
		}
		return true
	})
}

func (mu Mutator) mutants(set *token.FileSet, file *ast.File, node ast.Node, tok token.Token, tokPos token.Pos) ([]Mutant, bool) {
	var result []Mutant
	mutantTypes, ok := tokenMutantType[tok]
	if !ok {
		return nil, false
	}
	for _, mt := range mutantTypes {
		mutant := NewMutant(set, file, node)
		mutant.Type = mt
		mutant.TokPos = tokPos
		mutant.Status = mu.mutationStatus(set.Position(tokPos))

		result = append(result, mutant)
	}

	return result, true
}

func (mu Mutator) mutationStatus(pos token.Position) MutantStatus {
	var status MutantStatus
	if mu.covProfile.IsCovered(pos) {
		status = Runnable
	}

	return status
}

func (mu Mutator) executeTests(ch <-chan Mutant) []Mutant {
	var results []Mutant
	for m := range ch {
		if m.Status == NotCovered || mu.dryRun {
			results = append(results, m)
			fmt.Printf("%s at %s - %s\n", m.Type, m.Pos(), m.Status)
			continue
		}
		if err := mu.apply(m); err != nil {
			fmt.Printf("failed to apply mutation at %s - %s\n\t%v", m.Pos(), m.Status, err)
			continue
		}
		m.Status = Lived
		cmd := mu.execContext("go", "test", "-timeout", "5s", "./...")
		if err := cmd.Run(); err != nil {
			m.Status = Killed
		}
		if err := mu.rollback(m); err != nil {
			fmt.Printf("failed to restore mutation at %s - %s\n\t%v", m.Pos(), m.Status, err)
			// What should we do now?
		}
		fmt.Printf("%s at %s - %s\n", m.Type, m.Pos(), m.Status)
		results = append(results, m)
	}
	return results
}
