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
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/pkg/mutant"
	"github.com/go-gremlins/gremlins/pkg/mutator"
	"github.com/go-gremlins/gremlins/pkg/mutator/internal/workerpool"
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
		mjd := mutator.NewExecutorDealer(mod, wdDealer, expectedTimeout, mutator.WithExecContext(fakeExecCommandSuccess))
		mut := &mutantStub{
			status:  mutant.Runnable,
			mutType: mutant.ConditionalsBoundary,
			pkg:     "example.com",
		}
		outCh := make(chan mutant.Mutant)
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
		mjd := mutator.NewExecutorDealer(mod, wdDealer, expectedTimeout, mutator.WithExecContext(fakeExecCommandSuccess))
		mut := &mutantStub{
			status:        mutant.Runnable,
			mutType:       mutant.ConditionalsBoundary,
			pkg:           "example.com",
			hasApplyError: true,
		}
		outCh := make(chan mutant.Mutant)
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
		mutantStatus  mutant.Status
		wantMutStatus mutant.Status
	}{
		{
			name:          "it skips NOT_COVERED",
			testResult:    fakeExecCommandSuccess,
			mutantStatus:  mutant.NotCovered,
			wantMutStatus: mutant.NotCovered,
		},
		{
			name:          "if tests pass then mutation is LIVED",
			testResult:    fakeExecCommandSuccess,
			mutantStatus:  mutant.Runnable,
			wantMutStatus: mutant.Lived,
		},
		{
			name:          "if tests fails then mutation is KILLED",
			testResult:    fakeExecCommandTestsFailure,
			mutantStatus:  mutant.Runnable,
			wantMutStatus: mutant.Killed,
		},
		{
			name:          "if build fails then mutation is BUILD FAILED",
			testResult:    fakeExecCommandBuildFailure,
			mutantStatus:  mutant.Runnable,
			wantMutStatus: mutant.NotViable,
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
			mjd := mutator.NewExecutorDealer(mod, wdDealer, expectedTimeout, mutator.WithExecContext(tc.testResult))
			mut := &mutantStub{
				status:  tc.mutantStatus,
				mutType: mutant.ConditionalsBoundary,
				pkg:     "example.com",
			}
			outCh := make(chan mutant.Mutant)
			wg := sync.WaitGroup{}
			wg.Add(1)
			executor := mjd.NewExecutor(mut, outCh, &wg)
			w := &workerpool.Worker{
				Name: "test",
				ID:   1,
			}

			var got mutant.Mutant
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

const expectedTimeout = 10 * time.Second

type commandHolder struct {
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
			if tc.timeoutCoefficient != 0 {
				settings[configuration.UnleashTimeoutCoefficientKey] = tc.timeoutCoefficient
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
			mjd := mutator.NewExecutorDealer(mod, wdDealer, expectedTimeout,
				mutator.WithExecContext(fakeExecCommandSuccessWithHolder(holder)))
			mut := &mutantStub{
				status:  mutant.Runnable,
				mutType: mutant.ConditionalsBoundary,
				pkg:     tc.pkg,
			}
			outCh := make(chan mutant.Mutant)
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

			wantTimeout := 2*time.Second + expectedTimeout*mutator.DefaultTimeoutCoefficient
			if tc.timeoutCoefficient != 0 {
				wantTimeout = 2*time.Second + expectedTimeout*time.Duration(tc.timeoutCoefficient)
			}
			want := fmt.Sprintf("go test -tags %s -timeout %s -failfast %s", tc.tags, wantTimeout, tc.wantPath)
			got := fmt.Sprintf("go %v", strings.Join(holder.args, " "))

			if !cmp.Equal(got, want) {
				t.Errorf(fmt.Sprintf("\n+ %s\n- %s\n", got, want))
			}

			timeoutDifference := absTimeDiff(holder.timeout, expectedTimeout*2)
			diffThreshold := 100 * time.Second
			if timeoutDifference > diffThreshold {
				t.Errorf("expected timeout to be within %s from the set timeout, got %s", diffThreshold, timeoutDifference)
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
			mjd := mutator.NewExecutorDealer(mod, wdDealer, expectedTimeout,
				mutator.WithExecContext(fakeExecCommandSuccessWithHolder(holder)))
			mut := &mutantStub{
				status:  mutant.Runnable,
				mutType: mutant.ConditionalsBoundary,
				pkg:     "test",
			}
			outCh := make(chan mutant.Mutant)
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
	os.Exit(0)
}

func TestProcessTestsFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

func TestProcessBuildFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(2)
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
		got.m.Lock()
		defer got.m.Unlock()
		if got != nil {
			got.command = command
			got.args = args
			got.timeout = time.Until(dl)
		}
		cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
		cs = append(cs, args...)

		return getCmd(ctx, cs)
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
