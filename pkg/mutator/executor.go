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
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/pkg/log"
	"github.com/go-gremlins/gremlins/pkg/mutant"
	"github.com/go-gremlins/gremlins/pkg/mutator/internal/workerpool"
	"github.com/go-gremlins/gremlins/pkg/mutator/workdir"
	"github.com/go-gremlins/gremlins/pkg/report"
)

// DefaultTimeoutCoefficient is the default multiplier for the timeout length
// of each test run.
const DefaultTimeoutCoefficient = 3

// ExecutorDealer is the initializer for new workerpool.Executor.
type ExecutorDealer interface {
	NewExecutor(mut mutant.Mutant, outCh chan<- mutant.Mutant, wg *sync.WaitGroup) workerpool.Executor
}

// MutantExecutorDealer is a ExecutorDealer for the initialisation of a mutantExecutor.
//
// By default, it sets uses exec.Command to perform the tests on the source
// code. This can be overridden, for example in tests.
//
// The apply and rollback functions are wrappers around the TokenMutant apply and
// rollback. These can be overridden with nop functions in tests. Not an
// ideal setup. In the future we can think of a better way to handle this.
type MutantExecutorDealer struct {
	wdDealer          workdir.Dealer
	execContext       execContext
	apply             func(m mutant.Mutant) error
	rollback          func(m mutant.Mutant) error
	mod               gomodule.GoModule
	buildTags         string
	testExecutionTime time.Duration
	dryRun            bool
	integrationMode   bool
	testCpu           int
}

// ExecutorDealerOption is the defining option for the initialisation of a ExecutorDealer.
type ExecutorDealerOption func(j MutantExecutorDealer) MutantExecutorDealer

// WithExecContext overrides the default exec.Command with a custom executor.
func WithExecContext(c execContext) ExecutorDealerOption {
	return func(m MutantExecutorDealer) MutantExecutorDealer {
		m.execContext = c

		return m
	}
}

// NewExecutorDealer initialises a MutantExecutorDealer.
func NewExecutorDealer(mod gomodule.GoModule, wdd workdir.Dealer, elapsed time.Duration, opts ...ExecutorDealerOption) *MutantExecutorDealer {
	buildTags := configuration.Get[string](configuration.UnleashTagsKey)
	dryRun := configuration.Get[bool](configuration.UnleashDryRunKey)
	integrationMode := configuration.Get[bool](configuration.UnleashIntegrationMode)
	testCpu := configuration.Get[int](configuration.UnleashTestCpuKey)
	tCoefficient := configuration.Get[int](configuration.UnleashTimeoutCoefficientKey)

	coefficient := DefaultTimeoutCoefficient
	if tCoefficient != 0 {
		coefficient = tCoefficient
	}

	if testCpu != 0 && integrationMode {
		testCpu = testCpu / 2
	}

	jd := MutantExecutorDealer{
		mod:               mod,
		wdDealer:          wdd,
		buildTags:         buildTags,
		dryRun:            dryRun,
		integrationMode:   integrationMode,
		testCpu:           testCpu,
		testExecutionTime: elapsed * time.Duration(coefficient),
		execContext:       exec.CommandContext,
		apply: func(m mutant.Mutant) error {
			return m.Apply()
		},
		rollback: func(m mutant.Mutant) error {
			return m.Rollback()
		},
	}

	for _, opt := range opts {
		jd = opt(jd)
	}

	return &jd
}

// NewExecutor returns a new workerpool.Executor for the given mutant.Mutant.
// It gets an output channel of mutant.Mutant and a sync.WaitGroup. The channel
// will stream the results of the executor, and the wait group will be done when the
// executor is complete.
func (m MutantExecutorDealer) NewExecutor(mut mutant.Mutant, outCh chan<- mutant.Mutant, wg *sync.WaitGroup) workerpool.Executor {
	mj := mutantExecutor{
		mutant:            mut,
		outCh:             outCh,
		wg:                wg,
		wdDealer:          m.wdDealer,
		module:            m.mod,
		dryRun:            m.dryRun,
		integrationMode:   m.integrationMode,
		buildTags:         m.buildTags,
		execContext:       m.execContext,
		apply:             m.apply,
		rollback:          m.rollback,
		testCpu:           m.testCpu,
		testExecutionTime: m.testExecutionTime,
	}

	return &mj
}

type execContext = func(ctx context.Context, name string, args ...string) *exec.Cmd

type mutantExecutor struct {
	mutant            mutant.Mutant
	wdDealer          workdir.Dealer
	outCh             chan<- mutant.Mutant
	wg                *sync.WaitGroup
	execContext       execContext
	apply             func(m mutant.Mutant) error
	rollback          func(m mutant.Mutant) error
	module            gomodule.GoModule
	buildTags         string
	testExecutionTime time.Duration
	dryRun            bool
	integrationMode   bool
	testCpu           int
}

// Start is the implementation of the workerpool.Executor definition and is the
// method responsible for performing the actual mutation testing.
// The executor runs on its mutant.Mutant.
// If it is RUNNABLE, and it is not in dry-run mode, it will apply the mutation,
// run the tests and mark the TokenMutant as either KILLED or LIVED depending
// on the result. If the tests pass, it means the TokenMutant survived, so it
// will be LIVED, if the tests fail, the TokenMutant will be KILLED.
// The timeout of the test is managed outside the run of the test, using
// a context with timeout. This is done because the Go test command doesn't
// make it easy to distinguish failures from timeouts.
func (m *mutantExecutor) Start(w *workerpool.Worker) {
	defer m.wg.Done()
	workerName := fmt.Sprintf("%s-%d", w.Name, w.ID)
	currDir, _ := os.Getwd()
	rootDir, err := m.wdDealer.Get(workerName)
	if err != nil {
		panic("error, this is temporary")
	}
	defer func(d string) {
		_ = os.Chdir(d)
	}(currDir)
	_ = os.Chdir(rootDir)

	workingDir := filepath.Join(rootDir, m.module.CallingDir)
	m.mutant.SetWorkdir(workingDir)

	if m.mutant.Status() == mutant.NotCovered || m.dryRun {
		m.outCh <- m.mutant
		report.Mutant(m.mutant)

		return
	}

	if err := m.apply(m.mutant); err != nil {
		log.Errorf("failed to apply mutation at %s - %s\n\t%v", m.mutant.Position(), m.mutant.Status(), err)

		return
	}

	m.mutant.SetStatus(m.runTests(m.mutant.Pkg()))

	if err := m.rollback(m.mutant); err != nil {
		// What should we do now?
		log.Errorf("failed to restore mutation at %s - %s\n\t%v", m.mutant.Position(), m.mutant.Status(), err)
	}

	m.outCh <- m.mutant
	report.Mutant(m.mutant)
}

func (m *mutantExecutor) runTests(pkg string) mutant.Status {
	ctx, cancel := context.WithTimeout(context.Background(), m.testExecutionTime)
	defer cancel()
	cmd := m.execContext(ctx, "go", m.getTestArgs(pkg)...)

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

func (m *mutantExecutor) getTestArgs(pkg string) []string {
	args := []string{"test"}
	if m.buildTags != "" {
		args = append(args, "-tags", m.buildTags)
	}
	// Here we add some seconds to the timeout to be sure it's gremlins that catches the test
	// timeout and not the test itself. The timeout on the test prevents the test.* processes
	// from hanging forever.
	args = append(args, "-timeout", (2*time.Second + m.testExecutionTime).String())
	args = append(args, "-failfast")

	if m.testCpu != 0 {
		args = append(args, fmt.Sprintf("-cpu %d", m.testCpu))
	}

	path := pkg
	if m.integrationMode {
		path = "./..."
		if m.module.CallingDir != "." {
			path = fmt.Sprintf("./%s/...", m.module.CallingDir)
		}
	}
	args = append(args, path)

	return args
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
