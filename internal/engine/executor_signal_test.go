//go:build !windows

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
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

// A test command terminated by a signal never reached a verdict: the mutant was
// neither killed by the tests nor did it survive them. Reporting it as LIVED
// sends the reader after a missing test that does not exist, and reporting it as
// KILLED would pass the gate on a run that measured nothing.
func TestSignalTerminatedRunIsErrored(t *testing.T) {
	viperSet(map[string]any{configuration.UnleashDryRunKey: false})
	defer viperReset()

	wdDealer := newWdDealerStub(t)
	mod := gomodule.GoModule{
		Name:       "example.com",
		Root:       ".",
		CallingDir: ".",
	}
	mjd := engine.NewExecutorDealer(mod, wdDealer, expectedTimeout,
		engine.WithExecContext(fakeExecCommandSignalKilled))
	mut := &mutantStub{
		status:  mutator.Runnable,
		mutType: mutator.ConditionalsBoundary,
		pkg:     "example.com",
	}

	outCh := make(chan mutator.Mutator)
	wg := sync.WaitGroup{}
	wg.Add(1)
	executor := mjd.NewExecutor(mut, outCh, &wg)
	w := &workerpool.Worker{Name: "test", ID: 1}
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

	if got.Status() != mutator.Errored {
		t.Errorf("want %s, got %s", mutator.Errored, got.Status())
	}
}

func fakeExecCommandSignalKilled(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessSignalKilled", "--", command}
	cs = append(cs, args...)

	return getCmd(ctx, cs)
}

func TestProcessSignalKilled(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
}
