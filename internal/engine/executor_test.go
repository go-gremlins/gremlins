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
		mjd := engine.NewExecutorDealer(mod, wdDealer, engine.WithExecContext(fakeExecCommandSuccess))
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
		mjd := engine.NewExecutorDealer(mod, wdDealer, engine.WithExecContext(fakeExecCommandSuccess))
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
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			viperSet(map[string]any{configuration.UnleashDryRunKey: false})
			defer viperReset()
			wdDealer := newWdDealerStub(t)
			mod := gomodule.GoModule{
				Name:       "example.com",
				Root:       ".",
				CallingDir: ".",
			}
			mjd := engine.NewExecutorDealer(mod, wdDealer, engine.WithExecContext(tc.testResult))
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
		})
	}
}

type commandHolder struct {
	events []struct {
		cmd     *exec.Cmd
		command string
		args    []string
		timeout time.Duration
	}
}

func TestMutatorRun(t *testing.T) {
	testCases := []struct {
		name               string
		pkg                string
		callDir            string
		tags               string
		wantPath           string
		timeoutCoefficient int
		intMode            bool
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
			wantPath: "./test/dir/...",
		},
		{
			name:               "it can override timeout coefficient",
			timeoutCoefficient: 4,
			pkg:                "example.com/my/package",
			callDir:            "test/dir",
			tags:               "tag1,t1g2",
			wantPath:           "example.com/my/package",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			settings := map[string]any{
				configuration.UnleashIntegrationMode: tc.intMode,
				configuration.UnleashTagsKey:         tc.tags,
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
			mjd := engine.NewExecutorDealer(mod, wdDealer, engine.WithExecContext(fakeExecCommandSuccessWithHolder(holder)))
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

			gotArgs := holder.events[1].args
			if gotArgs[0] != "test" {
				t.Errorf("want %s, got %s", "test", gotArgs[0])
			}
			if gotArgs[1] != "-tags" {
				t.Errorf("want %s, got %s", "-tags", gotArgs[1])
			}
			if gotArgs[2] != tc.tags {
				t.Errorf("want %s, got %s", tc.tags, gotArgs[2])
			}
			if gotArgs[3] != "-timeout" {
				t.Errorf("want %s, got %s", "-timeout", gotArgs[3])
			}
			if gotArgs[4] == "" {
				t.Errorf("want timeout not to be empty")
			}
			if gotArgs[5] != "-count=1" {
				t.Errorf("want %s, got %s", "-failfast", gotArgs[5])
			}
			if gotArgs[6] != "-failfast" {
				t.Errorf("want %s, got %s", "-failfast", gotArgs[5])
			}
			if gotArgs[7] != tc.wantPath {
				t.Errorf("want %s, got %s", tc.wantPath, gotArgs[6])
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
			mjd := engine.NewExecutorDealer(mod, wdDealer,
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

			holderEvent := holder.events[1]
			for _, arg := range holderEvent.args {
				if !tc.cpuPresent && strings.Contains(arg, "-cpu") {
					t.Fatalf("didn't expect to have -cpu flag")
				}
				if !tc.cpuPresent {
					return
				}
				got := fmt.Sprintf("go %v", strings.Join(holderEvent.args, " "))
				cpuFlag := fmt.Sprintf("-cpu %d", tc.wantTestCPU)
				if strings.Contains(got, cpuFlag) {
					// PASS
					return
				}
				t.Fatalf("want flag %q, got args %s", cpuFlag, holderEvent.args)
			}

		})
	}
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

func TestProcessBuildFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(2) // skipcq: RVV-A0003
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
		mjd := engine.NewExecutorDealer(mod, wdDealer, engine.WithExecContext(fakeExecCommandSuccessWithHolder(holder)))
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

		cmd := holder.events[0].cmd

		if mut.Workdir() != cmd.Dir {
			t.Errorf("expected working dir to be %s, got %s", cmd.Dir, mut.Workdir())
		}
	})
}

func fakeExecCommandSuccess(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
	cs = append(cs, args...)
	// #nosec G204 - We are in tests, we don't care
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}

	return cmd
}

func fakeExecCommandSuccessWithHolder(got *commandHolder) execContext {
	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		dl, _ := ctx.Deadline()

		cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
		cs = append(cs, args...)
		cmd := getCmd(ctx, cs)

		got.events = append(got.events, struct {
			cmd     *exec.Cmd
			command string
			args    []string
			timeout time.Duration
		}{cmd: cmd, command: command, args: args, timeout: time.Until(dl)})

		return cmd
	}
}

func fakeExecCommandTestsFailure(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessTestsFailure", "--", command}
	cs = append(cs, args...)

	return getCmd(ctx, cs)
}

func fakeExecCommandBuildFailure(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessBuildFailure", "--", command}
	cs = append(cs, args...)

	return getCmd(ctx, cs)
}

func getCmd(ctx context.Context, cs []string) *exec.Cmd {
	// #nosec G204 - We are in tests, we don't care
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}

	return cmd
}
