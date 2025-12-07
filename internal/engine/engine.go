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

// Package engine orchestrates mutation testing by discovering, applying, and testing mutations.
package engine

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/diff"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/exclusion"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/report"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
)

// Engine is the "engine" that performs the mutation testing.
//
// It traverses the AST of the project, finds which TokenMutator can be applied and
// performs the actual mutation testing.
type Engine struct {
	fs           fs.FS
	jDealer      ExecutorDealer
	codeData     CodeData
	mutantStream chan mutator.Mutator
	module       gomodule.GoModule
	logger       report.MutantLogger
}

// CodeData is used to check if the mutant should be executed.
type CodeData struct {
	Cov       coverage.Profile
	Diff      diff.Diff
	Exclusion exclusion.Rules
}

// Option for the Engine initialization.
type Option func(m Engine) Engine

// New instantiates an Engine.
//
// It gets a fs.FS on which to perform the analysis, a CodeData to
// check if the mutants are executable and a sets of Option.
func New(mod gomodule.GoModule, codeData CodeData, jDealer ExecutorDealer, opts ...Option) Engine {
	dirFS := os.DirFS(filepath.Join(mod.Root, mod.CallingDir))
	mut := Engine{
		module:   mod,
		jDealer:  jDealer,
		codeData: codeData,
		fs:       dirFS,
		logger:   report.NewLogger(),
	}
	for _, opt := range opts {
		mut = opt(mut)
	}

	return mut
}

// WithDirFs overrides the fs.FS of the module (mainly used for testing purposes).
func WithDirFs(dirFS fs.FS) Option {
	return func(m Engine) Engine {
		m.fs = dirFS

		return m
	}
}

// Run executes the mutation testing.
//
// It walks the fs.FS provided and checks every .go file which is not a test.
// For each file it will scan for tokenMutations and gather all the mutants found.
func (mu *Engine) Run(ctx context.Context) report.Results {
	mu.mutantStream = make(chan mutator.Mutator)
	go func() {
		defer close(mu.mutantStream)
		_ = fs.WalkDir(mu.fs, ".", func(path string, _ fs.DirEntry, _ error) error {
			isGoCode := filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go")

			if isGoCode && !mu.codeData.Exclusion.IsFileExcluded(path) {
				mu.runOnFile(path)
			}

			return nil
		})
	}()

	start := time.Now()
	res := mu.executeTests(ctx)
	res.Elapsed = time.Since(start)
	res.Module = mu.module.Name

	return res
}

func (mu *Engine) runOnFile(fileName string) {
	src, _ := mu.fs.Open(fileName)
	set := token.NewFileSet()
	file, _ := parser.ParseFile(set, fileName, src, parser.ParseComments)
	_ = src.Close()

	ast.Inspect(file, func(node ast.Node) bool {
		// Check for token-based mutations
		if n, ok := NewTokenNode(node); ok {
			mu.findMutations(fileName, set, file, n)
		}

		// Check for expression-based mutations
		if e, ok := NewExprNode(node); ok {
			mu.findExprMutations(fileName, set, file, e, node)
		}

		return true
	})
}

func (mu *Engine) findMutations(fileName string, set *token.FileSet, file *ast.File, node *NodeToken) {
	mutantTypes := GetMutantTypesForToken(node.Tok(), node.NodeType())
	if len(mutantTypes) == 0 {
		return
	}

	pkg := mu.pkgName(fileName, file.Name.Name)
	for _, mt := range mutantTypes {
		if !configuration.Get[bool](configuration.MutantTypeEnabledKey(mt)) {
			continue
		}
		mutantType := mt
		tm := NewTokenMutant(pkg, set, file, node)
		tm.SetType(mutantType)
		tm.SetStatus(mu.mutationStatus(set.Position(node.TokPos)))

		mu.mutantStream <- tm
	}
}

func (mu *Engine) findExprMutations(fileName string, set *token.FileSet, file *ast.File, node *NodeExpr, astNode ast.Node) {
	mutantTypes := GetExprMutantTypes(node.Expr())
	if len(mutantTypes) == 0 {
		return
	}

	pkg := mu.pkgName(fileName, file.Name.Name)

	// Find parent node and create replace function
	parentNode, replaceFunc := mu.findParentAndReplacer(file, astNode)
	if parentNode == nil || replaceFunc == nil {
		// Cannot mutate if we can't find parent or create replacer
		return
	}

	for _, mt := range mutantTypes {
		if !configuration.Get[bool](configuration.MutantTypeEnabledKey(mt)) {
			continue
		}
		mutantType := mt
		em := NewExprMutant(pkg, set, file, node, parentNode, replaceFunc)
		em.SetType(mutantType)
		em.SetStatus(mu.mutationStatus(set.Position(node.Pos())))

		mu.mutantStream <- em
	}
}

func (mu *Engine) pkgName(fileName, fPkg string) string {
	var pkg string
	fn := fmt.Sprintf("%s/%s", mu.module.CallingDir, fileName)
	p := filepath.Dir(fn)
	for {
		if strings.HasSuffix(p, fPkg) {
			pkg = fmt.Sprintf("%s/%s", mu.module.Name, p)

			break
		}
		d := filepath.Dir(p)
		if d == p {
			pkg = mu.module.Name

			break
		}
		p = d
	}

	return normalisePkgPath(pkg)
}

func normalisePkgPath(pkg string) string {
	sep := fmt.Sprintf("%c", os.PathSeparator)

	return strings.ReplaceAll(pkg, sep, "/")
}

func (mu *Engine) mutationStatus(pos token.Position) mutator.Status {
	var status mutator.Status

	if mu.codeData.Cov.IsCovered(pos) {
		status = mutator.Runnable
	}

	if !mu.codeData.Diff.IsChanged(pos) {
		status = mutator.Skipped
	}

	return status
}

// findParentAndReplacer finds the parent node of target and returns a function
// to replace target with a new expression in the parent.
func (mu *Engine) findParentAndReplacer(file *ast.File, target ast.Node) (ast.Node, func(ast.Expr) error) {
	var parent ast.Node
	var replacer func(ast.Expr) error

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}

		// Check if this node contains our target as a child
		switch p := n.(type) {
		case *ast.UnaryExpr:
			if p.X == target {
				parent = p
				replacer = func(newExpr ast.Expr) error {
					p.X = newExpr

					return nil
				}

				return false
			}
		case *ast.BinaryExpr:
			if p.X == target {
				parent = p
				replacer = func(newExpr ast.Expr) error {
					p.X = newExpr

					return nil
				}

				return false
			}
			if p.Y == target {
				parent = p
				replacer = func(newExpr ast.Expr) error {
					p.Y = newExpr

					return nil
				}

				return false
			}
		case *ast.ParenExpr:
			if p.X == target {
				parent = p
				replacer = func(newExpr ast.Expr) error {
					p.X = newExpr

					return nil
				}

				return false
			}
		case *ast.CallExpr:
			for i, arg := range p.Args {
				if arg == target {
					parent = p
					idx := i // capture for closure
					replacer = func(newExpr ast.Expr) error {
						p.Args[idx] = newExpr

						return nil
					}

					return false
				}
			}
		case *ast.ReturnStmt:
			for i, result := range p.Results {
				if result == target {
					parent = p
					idx := i
					replacer = func(newExpr ast.Expr) error {
						p.Results[idx] = newExpr

						return nil
					}

					return false
				}
			}
		case *ast.AssignStmt:
			for i, expr := range p.Lhs {
				if expr == target {
					parent = p
					idx := i
					replacer = func(newExpr ast.Expr) error {
						p.Lhs[idx] = newExpr

						return nil
					}

					return false
				}
			}
			for i, expr := range p.Rhs {
				if expr == target {
					parent = p
					idx := i
					replacer = func(newExpr ast.Expr) error {
						p.Rhs[idx] = newExpr

						return nil
					}

					return false
				}
			}
		case *ast.IfStmt:
			if p.Cond == target {
				parent = p
				replacer = func(newExpr ast.Expr) error {
					p.Cond = newExpr

					return nil
				}

				return false
			}
		case *ast.ForStmt:
			if p.Cond == target {
				parent = p
				replacer = func(newExpr ast.Expr) error {
					p.Cond = newExpr

					return nil
				}

				return false
			}
		case *ast.SwitchStmt:
			if p.Tag == target {
				parent = p
				replacer = func(newExpr ast.Expr) error {
					p.Tag = newExpr

					return nil
				}

				return false
			}
		}

		return true
	})

	return parent, replacer
}

func (mu *Engine) executeTests(ctx context.Context) report.Results {
	pool := workerpool.Initialize("mutator")
	pool.Start()

	var mutants []mutator.Mutator
	outCh := make(chan mutator.Mutator)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for mut := range mu.mutantStream {
			ok := checkDone(ctx)
			if !ok {
				pool.Stop()

				break
			}
			wg.Add(1)
			pool.AppendExecutor(mu.jDealer.NewExecutor(mut, outCh, wg))
		}
	}()

	go func() {
		wg.Wait()
		close(outCh)
	}()

	for m := range outCh {
		mu.logger.Mutant(m)
		mutants = append(mutants, m)
	}

	return results(mutants)
}

func checkDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	default:
		return true
	}
}

func results(m []mutator.Mutator) report.Results {
	return report.Results{Mutants: m}
}
