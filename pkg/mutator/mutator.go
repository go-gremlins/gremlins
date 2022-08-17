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

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/pkg/coverage"
	"github.com/go-gremlins/gremlins/pkg/mutant"
	"github.com/go-gremlins/gremlins/pkg/mutator/internal"
	"github.com/go-gremlins/gremlins/pkg/mutator/internal/workerpool"
	"github.com/go-gremlins/gremlins/pkg/report"
)

// Mutator is the "engine" that performs the mutation testing.
//
// It traverses the AST of the project, finds which TokenMutant can be applied and
// performs the actual mutation testing.
type Mutator struct {
	fs           fs.FS
	jDealer      ExecutorDealer
	covProfile   coverage.Profile
	mutantStream chan mutant.Mutant
	module       gomodule.GoModule
}

// Option for the Mutator initialization.
type Option func(m Mutator) Mutator

// New instantiates a Mutator.
//
// It gets a fs.FS on which to perform the analysis, a coverage.Profile to
// check if the mutants are covered and a sets of Option.
func New(mod gomodule.GoModule, r coverage.Result, jDealer ExecutorDealer, opts ...Option) Mutator {
	dirFS := os.DirFS(filepath.Join(mod.Root, mod.CallingDir))
	mut := Mutator{
		module:     mod,
		jDealer:    jDealer,
		covProfile: r.Profile,
		fs:         dirFS,
	}
	for _, opt := range opts {
		mut = opt(mut)
	}

	return mut
}

// WithDirFs overrides the fs.FS of the module (mainly used for testing purposes).
func WithDirFs(dirFS fs.FS) Option {
	return func(m Mutator) Mutator {
		m.fs = dirFS

		return m
	}
}

// Run executes the mutation testing.
//
// It walks the fs.FS provided and checks every .go file which is not a test.
// For each file it will scan for tokenMutations and gather all the mutants found.
func (mu *Mutator) Run(ctx context.Context) report.Results {
	mu.mutantStream = make(chan mutant.Mutant)
	go func() {
		defer close(mu.mutantStream)
		_ = fs.WalkDir(mu.fs, ".", func(path string, d fs.DirEntry, err error) error {
			if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
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

func (mu *Mutator) runOnFile(fileName string) {
	src, _ := mu.fs.Open(fileName)
	set := token.NewFileSet()
	file, _ := parser.ParseFile(set, fileName, src, parser.ParseComments)
	_ = src.Close()

	ast.Inspect(file, func(node ast.Node) bool {
		n, ok := internal.NewTokenNode(node)
		if !ok {
			return true
		}
		mu.findMutations(fileName, set, file, n)

		return true
	})
}

func (mu *Mutator) findMutations(fileName string, set *token.FileSet, file *ast.File, node *internal.NodeToken) {
	mutantTypes, ok := internal.TokenMutantType[node.Tok()]
	if !ok {
		return
	}

	pkg := mu.pkgName(fileName, file.Name.Name)
	for _, mt := range mutantTypes {
		if !configuration.Get[bool](configuration.MutantTypeEnabledKey(mt)) {
			return
		}
		mutantType := mt
		tm := internal.NewTokenMutant(pkg, set, file, node)
		tm.SetType(mutantType)
		tm.SetStatus(mu.mutationStatus(set.Position(node.TokPos)))

		mu.mutantStream <- tm
	}
}

func (mu *Mutator) pkgName(fileName, fPkg string) string {
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

func (mu *Mutator) mutationStatus(pos token.Position) mutant.Status {
	var status mutant.Status
	if mu.covProfile.IsCovered(pos) {
		status = mutant.Runnable
	}

	return status
}

func (mu *Mutator) executeTests(ctx context.Context) report.Results {
	// TODO: add config for CPU
	// - if integration mode, use half cpu
	// - if cpu not set, use numCPU
	// - make timeout coefficient configurable
	// - make test cpu configurable
	// - set sensible defaults
	pool := workerpool.Initialize("mutator")
	pool.Start()

	var mutants []mutant.Mutant
	outCh := make(chan mutant.Mutant)
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

func results(m []mutant.Mutant) report.Results {
	return report.Results{Mutants: m}
}
