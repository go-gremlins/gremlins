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
	viability    Viability
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
	dir := filepath.Join(mod.Root, mod.CallingDir)
	mut := Engine{
		module:   mod,
		jDealer:  jDealer,
		codeData: codeData,
		fs:       os.DirFS(dir),
		logger:   report.NewLogger(),
		viability: NewTypeViability(
			dir,
			configuration.Get[string](configuration.UnleashTagsKey),
		),
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

// WithViability overrides the Viability the Engine consults before emitting a
// mutant. A nil Viability emits every mutant the token table maps, which is
// what gremlins did before one existed; the gate in unary_mutation_test.go
// uses it to show what the default checker is holding back.
func WithViability(v Viability) Option {
	return func(m Engine) Engine {
		m.viability = v

		return m
	}
}

// Run executes the mutation testing.
//
// It walks the fs.FS provided and checks every .go file which is not a test.
// For each file it will scan for tokenMutations and gather all the mutants found.
func (mu *Engine) Run(ctx context.Context) report.Results {
	// If the dealer is the standard MutantExecutorDealer, hand it the run
	// context so that in-flight mutants can be marked correctly when the
	// runner cancels (e.g. SIGTERM at job timeout) instead of falling
	// through to the default Lived branch.
	if d, ok := mu.jDealer.(*MutantExecutorDealer); ok {
		d.SetRunCtx(ctx)
	}
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
		n, ok := NewTokenNode(node)
		if !ok {
			return true
		}
		mu.findMutations(fileName, set, file, n)

		return true
	})
}

func (mu *Engine) findMutations(fileName string, set *token.FileSet, file *ast.File, node *NodeToken) {
	mutantTypes, ok := MutantTypesFor(node)
	if !ok {
		return
	}

	pkg := mu.pkgName(fileName, file.Name.Name)
	position := set.Position(node.TokPos)
	for _, mt := range mutantTypes {
		if !configuration.Get[bool](configuration.MutantTypeEnabledKey(mt)) {
			continue
		}
		if !mu.viable(position, node.Tok(), mt) {
			continue
		}
		mutantType := mt
		tm := NewTokenMutant(pkg, set, file, node)
		tm.SetType(mutantType)
		tm.SetStatus(mu.mutationStatus(set.Position(node.TokPos)))

		mu.mutantStream <- tm
	}
}

// viable reports whether applying mt to tok at position yields legal Go. The
// token table maps a token to the mutations that are MEANINGFUL for it; only
// the source around it decides whether they are legal. See Viability.
func (mu *Engine) viable(position token.Position, tok token.Token, mt mutator.Type) bool {
	if mu.viability == nil {
		return true
	}
	mutated, ok := tokenMutations[mt][tok]
	if !ok {
		return true
	}

	return mu.viability.Viable(position, mutated)
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
