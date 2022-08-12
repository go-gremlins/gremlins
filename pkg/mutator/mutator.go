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
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/pkg/coverage"
	"github.com/go-gremlins/gremlins/pkg/log"
	"github.com/go-gremlins/gremlins/pkg/mutant"
	"github.com/go-gremlins/gremlins/pkg/mutator/internal"
	"github.com/go-gremlins/gremlins/pkg/mutator/workdir"
	"github.com/go-gremlins/gremlins/pkg/report"
)

// Mutator is the "engine" that performs the mutation testing.
//
// It traverses the AST of the project, finds which TokenMutant can be applied and
// performs the actual mutation testing.
type Mutator struct {
	module            gomodule.GoModule
	fs                fs.FS
	wdManager         workdir.Dealer
	covProfile        coverage.Profile
	execContext       execContext
	apply             func(m mutant.Mutant) error
	rollback          func(m mutant.Mutant) error
	mutantStream      chan mutant.Mutant
	buildTags         string
	testExecutionTime time.Duration
	dryRun            bool
	integrationMode   bool
}

const timeoutCoefficient = 2

type execContext = func(ctx context.Context, name string, args ...string) *exec.Cmd

// Option for the Mutator initialization.
type Option func(m Mutator) Mutator

// New instantiates a Mutator.
//
// It gets a fs.FS on which to perform the analysis, a coverage.Profile to
// check if the mutants are covered and a sets of Option.
//
// By default, it sets uses exec.Command to perform the tests on the source
// code. This can be overridden, for example in tests.
//
// The apply and rollback functions are wrappers around the TokenMutant apply and
// rollback. These can be overridden with nop functions in tests. Not an
// ideal setup. In the future we can think of a better way to handle this.
func New(mod gomodule.GoModule, r coverage.Result, manager workdir.Dealer, opts ...Option) Mutator {
	dirFS := os.DirFS(filepath.Join(mod.Root, mod.CallingDir))
	buildTags := configuration.Get[string](configuration.UnleashTagsKey)
	dryRun := configuration.Get[bool](configuration.UnleashDryRunKey)
	integrationMode := configuration.Get[bool](configuration.UnleashIntegrationMode)

	mut := Mutator{
		module:            mod,
		wdManager:         manager,
		covProfile:        r.Profile,
		testExecutionTime: r.Elapsed * timeoutCoefficient,
		fs:                dirFS,
		execContext:       exec.CommandContext,
		apply: func(m mutant.Mutant) error {
			return m.Apply()
		},
		rollback: func(m mutant.Mutant) error {
			return m.Rollback()
		},

		buildTags:       buildTags,
		dryRun:          dryRun,
		integrationMode: integrationMode,
	}
	for _, opt := range opts {
		mut = opt(mut)
	}

	return mut
}

// WithExecContext overrides the default exec.Command with a custom executor.
func WithExecContext(c execContext) Option {
	return func(m Mutator) Mutator {
		m.execContext = c

		return m
	}
}

// WithApplyAndRollback overrides the apply and rollback functions.
func WithApplyAndRollback(a, r func(m mutant.Mutant) error) Option {
	return func(m Mutator) Mutator {
		m.apply = a
		m.rollback = r

		return m
	}
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
// For each TokenMutant found, if it is RUNNABLE, and it is not in dry-run mode,
// it will apply the mutation, run the tests and mark the TokenMutant as either
// KILLED or LIVED depending on the result. If the tests pass, it means the
// TokenMutant survived, so it will be LIVED, if the tests fail, the TokenMutant will
// be KILLED.
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
	var mutants []mutant.Mutant
	if mu.dryRun {
		log.Infoln("Running in 'dry-run' mode...")
	} else {
		log.Infoln("Executing mutation testing on covered mutants...")
	}
	currDir, _ := os.Getwd()
	rootDir, cl, err := mu.wdManager.Get()
	if err != nil {
		panic("error, this is temporary")
	}
	defer func(d string) {
		_ = os.Chdir(d)
		cl()
	}(currDir)
	wrkDir := filepath.Join(rootDir, mu.module.CallingDir)

	_ = os.Chdir(rootDir)

	for mut := range mu.mutantStream {
		ok := checkDone(ctx)
		if !ok {
			return results(mutants)
		}
		mut.SetWorkdir(wrkDir)
		if mut.Status() == mutant.NotCovered || mu.dryRun {
			mutants = append(mutants, mut)
			report.Mutant(mut)

			continue
		}

		if err := mu.apply(mut); err != nil {
			log.Errorf("failed to apply mutation at %s - %s\n\t%v", mut.Position(), mut.Status(), err)

			continue
		}

		mut.SetStatus(mu.runTests(mut.Pkg()))

		if err := mu.rollback(mut); err != nil {
			// What should we do now?
			log.Errorf("failed to restore mutation at %s - %s\n\t%v", mut.Position(), mut.Status(), err)
		}

		report.Mutant(mut)
		mutants = append(mutants, mut)
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

func (mu *Mutator) runTests(pkg string) mutant.Status {
	ctx, cancel := context.WithTimeout(context.Background(), mu.testExecutionTime)
	defer cancel()
	cmd := mu.execContext(ctx, "go", mu.getTestArgs(pkg)...)

	rel, err := run(cmd)
	defer rel()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return mutant.TimedOut
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return getTestFailedStatus(exitErr.ExitCode())
	}

	return mutant.Lived
}

func run(cmd *exec.Cmd) (func(), error) {
	if err := cmd.Run(); err != nil {

		return func() {}, err
	}

	return func() {
		err := cmd.Process.Release()
		if err != nil {
			_ = cmd.Process.Kill()
		}
	}, nil
}

func (mu *Mutator) getTestArgs(pkg string) []string {
	args := []string{"test"}
	if mu.buildTags != "" {
		args = append(args, "-tags", mu.buildTags)
	}
	args = append(args, "-timeout", (1*time.Second + mu.testExecutionTime).String())
	args = append(args, "-failfast")

	path := pkg
	if mu.integrationMode {
		path = "./..."
		if mu.module.CallingDir != "." {
			path = fmt.Sprintf("./%s/...", mu.module.CallingDir)
		}
	}
	args = append(args, path)

	return args
}

func getTestFailedStatus(exitCode int) mutant.Status {
	switch exitCode {
	case 1:
		return mutant.Killed
	case 2:
		return mutant.NotViable
	default:
		return mutant.Lived
	}
}
