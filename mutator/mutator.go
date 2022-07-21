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
	"github.com/k3rn31/gremlins/coverage"
	"github.com/k3rn31/gremlins/log"
	"github.com/k3rn31/gremlins/mutant"
	"github.com/k3rn31/gremlins/mutator/internal"
	"github.com/k3rn31/gremlins/mutator/workdir"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Mutator is the "engine" that performs the mutation testing.
//
// It traverses the AST of the project, finds which TokenMutant can be applied and
// performs the actual mutation testing.
type Mutator struct {
	covProfile   coverage.Profile
	fs           fs.FS
	execContext  execContext
	wdManager    workdir.Dealer
	apply        func(m mutant.Mutant) error
	rollback     func(m mutant.Mutant) error
	mutantStream chan mutant.Mutant

	dryRun    bool
	buildTags string
}

type execContext = func(name string, args ...string) *exec.Cmd

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
func New(fs fs.FS, p coverage.Profile, manager workdir.Dealer, opts ...Option) Mutator {
	mut := Mutator{
		wdManager:   manager,
		covProfile:  p,
		fs:          fs,
		execContext: exec.Command,
		apply: func(m mutant.Mutant) error {
			return m.Apply()
		},
		rollback: func(m mutant.Mutant) error {
			return m.Rollback()
		},
	}
	for _, opt := range opts {
		mut = opt(mut)
	}
	return mut
}

// WithDryRun sets the dry-run flag. If true, it will not perform the actual
// mutant testing, only discovery will be executed.
func WithDryRun(d bool) Option {
	return func(m Mutator) Mutator {
		m.dryRun = d
		return m
	}
}

// WithBuildTags sets the build tags for the go test command.
func WithBuildTags(t string) Option {
	return func(m Mutator) Mutator {
		m.buildTags = t
		return m
	}
}

// WithExecContext overrides the default exec.Command with a custom executor.
func WithExecContext(c execContext) Option {
	return func(m Mutator) Mutator {
		m.execContext = c
		return m
	}
}

// WithApplyAndRollback overrides the apply and rollback functions.
func WithApplyAndRollback(a func(m mutant.Mutant) error, r func(m mutant.Mutant) error) Option {
	return func(m Mutator) Mutator {
		m.apply = a
		m.rollback = r
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
func (mu Mutator) Run() []mutant.Mutant {
	log.Infoln("Looking for mutants...")
	mu.mutantStream = make(chan mutant.Mutant)
	go func() {
		_ = fs.WalkDir(mu.fs, ".", func(path string, d fs.DirEntry, err error) error {
			if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
				src, _ := mu.fs.Open(path)
				mu.runOnFile(path, src)
			}
			return nil
		})
		close(mu.mutantStream)
	}()

	return mu.executeTests()
}

func (mu Mutator) runOnFile(fileName string, src io.Reader) {
	set := token.NewFileSet()
	file, _ := parser.ParseFile(set, fileName, src, parser.ParseComments)
	ast.Inspect(file, func(node ast.Node) bool {
		n, ok := internal.NewTokenNode(node)
		if !ok {
			return true
		}
		mu.findMutations(set, file, n)
		return true
	})
}

func (mu Mutator) findMutations(set *token.FileSet, file *ast.File, node *internal.NodeToken) {
	mutantTypes, ok := internal.TokenMutantType[node.Tok()]
	if !ok {
		return
	}
	for _, mt := range mutantTypes {
		mutantType := mt
		tm := internal.NewTokenMutant(set, file, node)
		tm.SetType(mutantType)
		tm.SetStatus(mu.mutationStatus(set.Position(node.TokPos)))

		mu.mutantStream <- tm
	}
}

func (mu Mutator) mutationStatus(pos token.Position) mutant.Status {
	var status mutant.Status
	if mu.covProfile.IsCovered(pos) {
		status = mutant.Runnable
	}

	return status
}

func (mu Mutator) executeTests() []mutant.Mutant {
	if mu.dryRun {
		log.Infoln("Running in 'dry-run' mode.")
	} else {
		log.Infoln("Executing mutation testing on covered mutants...")
	}
	wd, cl, err := mu.wdManager.Get()
	if err != nil {
		panic("error, this is temporary")
	}
	defer cl()
	_ = os.Chdir(wd)

	var results []mutant.Mutant
	for m := range mu.mutantStream {
		m.SetWorkdir(wd)
		if m.Status() == mutant.NotCovered || mu.dryRun {
			results = append(results, m)
			log.Mutant(m)
			continue
		}
		if err := mu.apply(m); err != nil {
			log.Errorf("failed to apply mutation at %s - %s\n\t%v", m.Position(), m.Status(), err)
			continue
		}
		m.SetStatus(mutant.Lived)
		args := []string{"test", "-timeout", "5s"}
		if mu.buildTags != "" {
			args = append(args, "-tags", mu.buildTags)
		}
		args = append(args, "./...")
		cmd := mu.execContext("go", args...)
		if err := cmd.Run(); err != nil {
			m.SetStatus(mutant.Killed)
		}
		if err := mu.rollback(m); err != nil {
			log.Errorf("failed to restore mutation at %s - %s\n\t%v", m.Position(), m.Status(), err)
			// What should we do now?
		}
		log.Mutant(m)
		results = append(results, m)
	}
	return results
}
