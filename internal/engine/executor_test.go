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

package engine_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

func TestApplyAndRollback(t *testing.T) {
	t.Run("applies and rolls back", func(t *testing.T) {
		wdDealer := newWdDealerStub(t)
		tmpDir, _ := wdDealer.Get("")
		mod := gomodule.GoModule{
			Name:       "example.com",
			Root:       tmpDir,
			CallingDir: ".",
		}
		mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout, engine.WithExecContext(fakeExecCommandSuccess))
		mut := &mutantStub{
			status:  mutator.Runnable,
			mutType: mutator.ConditionalsBoundary,
			pkg:     "example.com",
		}
		outCh := make(chan mutator.Mutator)
		wg := sync.WaitGroup{}
		wg.Add(1)
		executor := mjd.NewExecutor(mut, outCh, &wg)
		w := &workerpool.Worker{
			Name: "test",
			ID:   1,
		}
		go func() {
			<-outCh
			close(outCh)
		}()

		executor.Start(w)

		wg.Wait()

		if !mut.applyCalled {
			t.Errorf("expected apply to be called")
		}

		if !mut.rollbackCalled {
			t.Errorf("expected rollback to be called")
		}
	})

	t.Run("does nothing if apply goes to error", func(t *testing.T) {
		wdDealer := newWdDealerStub(t)
		tmpDir, _ := wdDealer.Get("")
		mod := gomodule.GoModule{
			Name:       "example.com",
			Root:       tmpDir,
			CallingDir: ".",
		}
		mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout, engine.WithExecContext(fakeExecCommandSuccess))
		mut := &mutantStub{
			status:        mutator.Runnable,
			mutType:       mutator.ConditionalsBoundary,
			pkg:           "example.com",
			hasApplyError: true,
		}
		outCh := make(chan mutator.Mutator)
		wg := sync.WaitGroup{}
		wg.Add(1)
		executor := mjd.NewExecutor(mut, outCh, &wg)
		w := &workerpool.Worker{
			Name: "test",
			ID:   1,
		}
		go func() {
			<-outCh
			close(outCh)
		}()

		executor.Start(w)

		wg.Wait()

		if !mut.applyCalled {
			t.Errorf("expected apply to be called")
		}

		if mut.rollbackCalled {
			t.Errorf("expected rollback not to be called")
		}
	})
}

type execContext = func(ctx context.Context, name string, args ...string) *exec.Cmd

func TestMutatorTestExecution(t *testing.T) {
	testCases := []struct {
		testResult    execContext
		name          string
		mutantStatus  mutator.Status
		wantMutStatus mutator.Status
	}{
		{
			name:          "it skips NOT_COVERED",
			testResult:    fakeExecCommandSuccess,
			mutantStatus:  mutator.NotCovered,
			wantMutStatus: mutator.NotCovered,
		},
		{
			name:          "if tests pass then mutation is LIVED",
			testResult:    fakeExecCommandSuccess,
			mutantStatus:  mutator.Runnable,
			wantMutStatus: mutator.Lived,
		},
		{
			name:          "if tests fails then mutation is KILLED",
			testResult:    fakeExecCommandTestsFailure,
			mutantStatus:  mutator.Runnable,
			wantMutStatus: mutator.Killed,
		},
		{
			name:          "if build fails then mutation is BUILD FAILED",
			testResult:    fakeExecCommandBuildFailure,
			mutantStatus:  mutator.Runnable,
			wantMutStatus: mutator.NotViable,
		},
		{
			// The run-phase timeout marker is evidence only alongside a child
			// that FAILED. A child that printed it and then exited 0 adjudicated
			// the mutant, whatever it said on the way, and reading its output
			// alone would move a survivor out of the LIVED column and into a
			// timeout one — where a consumer that credits run-phase timeouts
			// would then pay the suite for it.
			name:          "if the child prints the timeout marker and still succeeds, mutation is LIVED",
			testResult:    fakeExecCommandMarkerThenSuccess,
			mutantStatus:  mutator.Runnable,
			wantMutStatus: mutator.Lived,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			viperSet(map[string]any{configuration.UnleashDryRunKey: false})
			defer viperReset()
			wdDealer := newWdDealerStub(t)
			holder := &commandHolder{}
			mod := gomodule.GoModule{
				Name:       "example.com",
				Root:       ".",
				CallingDir: ".",
			}
			mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout,
				engine.WithExecContext(fakeExecCommandWithHolder(holder, tc.testResult)),
			)
			mut := &mutantStub{
				status:  tc.mutantStatus,
				mutType: mutator.ConditionalsBoundary,
				pkg:     "example.com",
			}
			outCh := make(chan mutator.Mutator)
			wg := sync.WaitGroup{}
			wg.Add(1)
			executor := mjd.NewExecutor(mut, outCh, &wg)
			w := &workerpool.Worker{
				Name: "test",
				ID:   1,
			}

			var got mutator.Mutator
			mutex := sync.RWMutex{}
			go func() {
				mutex.Lock()
				defer mutex.Unlock()
				got = <-outCh
				close(outCh)
			}()
			executor.Start(w)
			wg.Wait()

			mutex.RLock()
			defer mutex.RUnlock()

			if got.Status() != tc.wantMutStatus {
				t.Errorf("expected mutation to be %v, but got: %v", tc.wantMutStatus, got.Status())
			}

			if tc.mutantStatus != mutator.NotCovered {
				goTmpDirEnv := fmt.Sprintf("GOTMPDIR=%s", wdDealer.WorkDir())
				actualGoTmpDir := ""
				for _, v := range holder.cmd.Env {
					if strings.HasPrefix(v, "GOTMPDIR=") {
						actualGoTmpDir = v

						break
					}
				}
				if goTmpDirEnv != actualGoTmpDir {
					t.Errorf("expected GOTMPDIR to be %s, got %s", goTmpDirEnv, actualGoTmpDir)
				}
			}
		})
	}
}

const expectedTimeout = 10 * time.Second

type commandHolder struct {
	cmd     *exec.Cmd
	command string
	args    []string
	timeout time.Duration
	m       sync.Mutex
}

func TestMutatorRun(t *testing.T) {
	testCases := []struct {
		name               string
		pkg                string
		callDir            string
		tags               string
		wantPath           string
		timeoutMax         string
		compileAllowance   string
		timeoutCoefficient int
		// wantExecutionTime, when non-zero, is the per-mutant timeout the run
		// must end up with, overriding the coefficient-derived expectation.
		wantExecutionTime time.Duration
		intMode           bool
	}{
		{
			name:     "normal mode",
			intMode:  false,
			pkg:      "example.com/my/package",
			callDir:  "test/dir",
			tags:     "tag1,t1g2",
			wantPath: "example.com/my/package",
		},
		{
			name:     "integration mode",
			intMode:  true,
			pkg:      "example.com/my/package",
			callDir:  "test/dir",
			tags:     "tag1,t1g2",
			wantPath: "./...",
		},
		{
			name:               "it can override timeout coefficient",
			timeoutCoefficient: 4,
			pkg:                "example.com/my/package",
			callDir:            "test/dir",
			tags:               "tag1,t1g2",
			wantPath:           "example.com/my/package",
		},
		{
			// The whole point of the ceiling: 4 x 10s would be 40s, and a mutant
			// that never terminates gets 40s to exhaust the machine. The cap
			// takes it to 15s regardless of how slow the package's tests are.
			name:               "it caps the timeout at timeout-max",
			timeoutCoefficient: 4,
			timeoutMax:         "15s",
			wantExecutionTime:  15 * time.Second,
			pkg:                "example.com/my/package",
			callDir:            "test/dir",
			tags:               "tag1,t1g2",
			wantPath:           "example.com/my/package",
		},
		{
			// A ceiling above the derived timeout must not pull it UP: the cap
			// bounds the worst case, it does not set the timeout.
			name:               "a timeout-max above the derived timeout leaves it alone",
			timeoutCoefficient: 4,
			timeoutMax:         "2m",
			wantExecutionTime:  40 * time.Second,
			pkg:                "example.com/my/package",
			callDir:            "test/dir",
			tags:               "tag1,t1g2",
			wantPath:           "example.com/my/package",
		},
		{
			// An unparseable ceiling falls back to the derived timeout. It is
			// reported on stderr rather than swallowed, but it must not abort a
			// run or silently become zero — a zero timeout would kill every
			// mutant instantly and report the package as perfectly tested.
			name:              "a malformed timeout-max is ignored",
			timeoutMax:        "fifteen seconds",
			wantExecutionTime: expectedTimeout * engine.DefaultTimeoutCoefficient,
			pkg:               "example.com/my/package",
			callDir:           "test/dir",
			tags:              "tag1,t1g2",
			wantPath:          "example.com/my/package",
		},
		{
			name:              "a non-positive timeout-max is ignored",
			timeoutMax:        "0s",
			wantExecutionTime: expectedTimeout * engine.DefaultTimeoutCoefficient,
			pkg:               "example.com/my/package",
			callDir:           "test/dir",
			tags:              "tag1,t1g2",
			wantPath:          "example.com/my/package",
		},
		{
			// The compile allowance moves the context deadline and must leave
			// the run bound alone: it buys time for compiling, not for running.
			name:             "compile-allowance extends the deadline but not the run bound",
			compileAllowance: "3m",
			pkg:              "example.com/my/package",
			callDir:          "test/dir",
			tags:             "tag1,t1g2",
			wantPath:         "example.com/my/package",
		},
		{
			name:             "a malformed compile-allowance falls back to the default",
			compileAllowance: "three minutes",
			pkg:              "example.com/my/package",
			callDir:          "test/dir",
			tags:             "tag1,t1g2",
			wantPath:         "example.com/my/package",
		},
		{
			name:             "a non-positive compile-allowance falls back to the default",
			compileAllowance: "0s",
			pkg:              "example.com/my/package",
			callDir:          "test/dir",
			tags:             "tag1,t1g2",
			wantPath:         "example.com/my/package",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			settings := map[string]any{
				configuration.UnleashIntegrationMode: tc.intMode,
				configuration.UnleashTagsKey:         tc.tags,
			}
			if tc.timeoutCoefficient != 0 {
				settings[configuration.UnleashTimeoutCoefficientKey] = tc.timeoutCoefficient
			}
			if tc.timeoutMax != "" {
				settings[configuration.UnleashTimeoutMaxKey] = tc.timeoutMax
			}
			if tc.compileAllowance != "" {
				settings[configuration.UnleashCompileAllowanceKey] = tc.compileAllowance
			}
			viperSet(settings)
			defer viperReset()

			mod := gomodule.GoModule{
				Name:       "example.com",
				Root:       ".",
				CallingDir: tc.callDir,
			}
			wdDealer := newWdDealerStub(t)
			holder := &commandHolder{}
			mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout,
				engine.WithExecContext(fakeExecCommandSuccessWithHolder(holder)))
			mut := &mutantStub{
				status:  mutator.Runnable,
				mutType: mutator.ConditionalsBoundary,
				pkg:     tc.pkg,
			}
			outCh := make(chan mutator.Mutator)
			wg := sync.WaitGroup{}
			wg.Add(1)
			executor := mjd.NewExecutor(mut, outCh, &wg)
			w := &workerpool.Worker{
				Name: "test",
				ID:   1,
			}
			go func() {
				<-outCh
				close(outCh)
			}()
			executor.Start(w)
			wg.Wait()

			// -timeout carries the run bound verbatim. Nothing is added to it:
			// the whole point is that the leash Go enforces on the test RUN is
			// the tighter of the two bounds, so a slow compile cannot eat it.
			wantRunBound := expectedTimeout * engine.DefaultTimeoutCoefficient
			if tc.timeoutCoefficient != 0 {
				wantRunBound = expectedTimeout * time.Duration(tc.timeoutCoefficient)
			}
			if tc.wantExecutionTime != 0 {
				wantRunBound = tc.wantExecutionTime
			}
			want := fmt.Sprintf("go test -vet=off -tags %s -timeout %s -failfast %s", tc.tags, wantRunBound, tc.wantPath)
			got := fmt.Sprintf("go %v", strings.Join(holder.args, " "))

			if !cmp.Equal(got, want) {
				t.Errorf("\n+ %s\n- %s\n", got, want)
			}

			// The context deadline is the backstop: the run bound PLUS the
			// compile allowance, so compilation is charged to it and not to the
			// run bound above.
			wantAllowance := engine.DefaultCompileAllowance
			if tc.compileAllowance != "" {
				parsed, err := time.ParseDuration(tc.compileAllowance)
				if err == nil && parsed > 0 {
					wantAllowance = parsed
				}
			}
			wantDeadline := wantRunBound + wantAllowance
			// The deadline is read after the context is created, so a little of
			// it has already elapsed; only scheduling slack is tolerated.
			const deadlineSlack = 2 * time.Second
			if d := absTimeDiff(holder.timeout, wantDeadline); d > deadlineSlack {
				t.Errorf("expected the context deadline to be %s (run bound %s + compile allowance %s), got %s",
					wantDeadline, wantRunBound, wantAllowance, holder.timeout)
			}
		})
	}
}

func TestCPU(t *testing.T) {
	testCases := []struct {
		name        string
		testCPU     int
		wantTestCPU int
		intMode     bool
		cpuPresent  bool
	}{
		{
			name:       "default normal mode doesn't set CPU",
			cpuPresent: false,
		},
		{
			name:       "default integration mode doesn't set CPU",
			intMode:    true,
			cpuPresent: false,
		},
		{
			name:        "normal mode can override CPU",
			testCPU:     1,
			wantTestCPU: 1,
			cpuPresent:  true,
		},
		{
			name:        "integration mode overrides CPU to half",
			intMode:     true,
			testCPU:     2,
			wantTestCPU: 1,
			cpuPresent:  true,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			viperSet(map[string]any{
				configuration.UnleashIntegrationMode: tc.intMode,
				configuration.UnleashTestCPUKey:      tc.testCPU,
			})
			defer viperReset()

			mod := gomodule.GoModule{
				Name:       "example.com",
				Root:       ".",
				CallingDir: ".",
			}
			wdDealer := newWdDealerStub(t)
			holder := &commandHolder{}
			mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout,
				engine.WithExecContext(fakeExecCommandSuccessWithHolder(holder)))
			mut := &mutantStub{
				status:  mutator.Runnable,
				mutType: mutator.ConditionalsBoundary,
				pkg:     "test",
			}
			outCh := make(chan mutator.Mutator)
			wg := sync.WaitGroup{}
			wg.Add(1)
			executor := mjd.NewExecutor(mut, outCh, &wg)
			w := &workerpool.Worker{
				Name: "test",
				ID:   1,
			}
			go func() {
				<-outCh
				close(outCh)
			}()
			executor.Start(w)
			wg.Wait()

			for _, arg := range holder.args {
				if !tc.cpuPresent && strings.Contains(arg, "-cpu") {
					t.Fatalf("didn't expect to have -cpu flag")
				}
				if !tc.cpuPresent {
					return
				}
				got := fmt.Sprintf("go %v", strings.Join(holder.args, " "))
				cpuFlag := fmt.Sprintf("-cpu %d", tc.wantTestCPU)
				if strings.Contains(got, cpuFlag) {
					// PASS
					return
				}
				t.Fatalf("want flag %q, got args %s", cpuFlag, holder.args)
			}

		})
	}
}

func absTimeDiff(a, b time.Duration) time.Duration {
	if a > b {
		return a - b
	}

	return b - a
}

func TestCoverageProcessSuccess(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(0) // skipcq: RVV-A0003
}

func TestProcessTestsFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(1) // skipcq: RVV-A0003
}

// TestProcessMarkerThenSuccess is the child-process entry point that prints the
// bytes a timed-out test binary prints and then exits 0.
func TestProcessMarkerThenSuccess(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "panic: test timed out after 10m0s")
	os.Exit(0) // skipcq: RVV-A0003
}

func TestProcessBuildFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(2) // skipcq: RVV-A0003
}

// TestProcessSleep is the child-process entry point used by the
// SIGTERM-shutdown tests: it sleeps long enough that the parent ctx
// will cancel it. ExecCommandContext kills the process tree on ctx
// cancellation, so this child will exit non-zero (signal kill) and
// the parent ctx.Err() will be non-nil.
func TestProcessSleep(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
	os.Exit(0) // skipcq: RVV-A0003 — only reached if no one cancels us
}

func fakeExecCommandSleep(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessSleep", "--", command}
	cs = append(cs, args...)
	return getCmd(ctx, cs)
}

// TestShutdownMidRun pins the fix for the bug where a SIGTERM from a CI
// runner during `gremlins unleash` caused mutants that were still being
// tested to be recorded as LIVED. The expected behaviour now: when the
// engine's root ctx is cancelled mid-run, in-flight mutants are reported
// with the status configured via --on-shutdown-status (default
// not-run -> mutator.NotCovered), never LIVED.
func TestShutdownMidRun(t *testing.T) {
	cases := []struct {
		name       string
		flagValue  string
		wantStatus mutator.Status
	}{
		{name: "default not-run maps to NotCovered", flagValue: "not-run", wantStatus: mutator.NotCovered},
		{name: "timed-out maps to TimedOut", flagValue: "timed-out", wantStatus: mutator.TimedOut},
		{name: "lived preserves legacy behaviour for opt-in", flagValue: "lived", wantStatus: mutator.Lived},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			viperSet(map[string]any{
				configuration.UnleashDryRunKey:           false,
				configuration.UnleashOnShutdownStatusKey: tc.flagValue,
			})
			defer viperReset()

			wdDealer := newWdDealerStub(t)
			mod := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}
			mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout,
				engine.WithExecContext(fakeExecCommandSleep),
			)
			runCtx, cancel := context.WithCancel(context.Background())
			mjd.SetRunCtx(runCtx)

			mut := &mutantStub{
				status:  mutator.Runnable,
				mutType: mutator.ConditionalsBoundary,
				pkg:     "example.com",
			}
			outCh := make(chan mutator.Mutator, 1)
			wg := sync.WaitGroup{}
			wg.Add(1)
			executor := mjd.NewExecutor(mut, outCh, &wg)
			w := &workerpool.Worker{Name: "test", ID: 1}

			// Cancel the run a short moment after the executor starts, so
			// the test subprocess is killed mid-flight.
			go func() {
				time.Sleep(100 * time.Millisecond)
				cancel()
			}()
			executor.Start(w)
			wg.Wait()

			select {
			case got := <-outCh:
				if got.Status() != tc.wantStatus {
					t.Fatalf("shutdown-mid-run: want %v, got %v", tc.wantStatus, got.Status())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("executor never wrote to outCh")
			}
		})
	}
}

func TestMutatorRunInTheCorrectFolder(t *testing.T) {
	t.Run("mutation should run in the correct folder", func(t *testing.T) {
		callingDir := "test/dir"
		mod := gomodule.GoModule{
			Name:       "example.com",
			Root:       ".",
			CallingDir: callingDir,
		}
		wdDealer := newWdDealerStub(t)
		holder := &commandHolder{}
		mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout,
			engine.WithExecContext(fakeExecCommandSuccessWithHolder(holder)))
		mut := &mutantStub{
			status:  mutator.Runnable,
			mutType: mutator.ConditionalsBoundary,
			pkg:     "example.com/my/package",
		}
		outCh := make(chan mutator.Mutator)
		wg := sync.WaitGroup{}
		wg.Add(1)
		executor := mjd.NewExecutor(mut, outCh, &wg)
		w := &workerpool.Worker{
			Name: "test",
			ID:   1,
		}
		go func() {
			<-outCh
			close(outCh)
		}()
		executor.Start(w)
		wg.Wait()

		if mut.Workdir() != holder.cmd.Dir {
			t.Errorf("expected working dir to be %s, got %s", holder.cmd.Dir, mut.Workdir())
		}
	})
	t.Run("integration mode: mutation should run in the wd root folder", func(t *testing.T) {
		viperSet(map[string]any{
			configuration.UnleashIntegrationMode: true,
		})
		defer viperReset()
		callingDir := "test/dir"
		mod := gomodule.GoModule{
			Name:       "example.com",
			Root:       ".",
			CallingDir: callingDir,
		}
		wantRootDir := "/tmp/static"
		wdDealer := &dealerStub{
			t: nil,
			fnGet: func(_ string) (string, error) {
				return wantRootDir, nil
			},
		}

		holder := &commandHolder{}
		mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout,
			engine.WithExecContext(fakeExecCommandSuccessWithHolder(holder)))
		mut := &mutantStub{
			status:  mutator.Runnable,
			mutType: mutator.ConditionalsBoundary,
			pkg:     "example.com/my/package",
		}
		outCh := make(chan mutator.Mutator)
		wg := sync.WaitGroup{}
		wg.Add(1)
		executor := mjd.NewExecutor(mut, outCh, &wg)
		w := &workerpool.Worker{
			Name: "test",
			ID:   1,
		}
		go func() {
			<-outCh
			close(outCh)
		}()
		executor.Start(w)
		wg.Wait()

		if wantRootDir != holder.cmd.Dir {
			t.Errorf("expected working dir to be %q, got %q", wantRootDir, holder.cmd.Dir)
		}
	})
}

func fakeExecCommandSuccess(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
	cs = append(cs, args...)
	// #nosec G204 G702 - We are in tests, we don't care
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}

	return cmd
}

func fakeExecCommandSuccessWithHolder(got *commandHolder) execContext {
	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		dl, _ := ctx.Deadline()
		got.m.Lock()
		defer got.m.Unlock()

		cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
		cs = append(cs, args...)
		cmd := getCmd(ctx, cs)

		got.cmd = cmd
		got.command = command
		got.args = args
		got.timeout = time.Until(dl)

		return cmd
	}
}

func fakeExecCommandWithHolder(got *commandHolder, fakeCmd func(ctx context.Context, command string, args ...string) *exec.Cmd) execContext {
	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		dl, _ := ctx.Deadline()
		got.m.Lock()
		defer got.m.Unlock()

		cmd := fakeCmd(ctx, command, args...)
		got.cmd = cmd
		got.command = command
		got.args = args
		got.timeout = time.Until(dl)

		return cmd
	}
}

func fakeExecCommandTestsFailure(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessTestsFailure", "--", command}
	cs = append(cs, args...)

	return getCmd(ctx, cs)
}

func fakeExecCommandMarkerThenSuccess(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessMarkerThenSuccess", "--", command}
	cs = append(cs, args...)

	return getCmd(ctx, cs)
}

func fakeExecCommandBuildFailure(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessBuildFailure", "--", command}
	cs = append(cs, args...)

	return getCmd(ctx, cs)
}

func getCmd(ctx context.Context, cs []string) *exec.Cmd {
	// #nosec G204 G702 - We are in tests, we don't care
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}

	return cmd
}
