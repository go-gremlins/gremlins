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

package workerpool_test

import (
	"go/token"
	"runtime"
	"testing"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

type ExecutorMock struct {
	mutant mutator.Mutator
	outCh  chan<- mutator.Mutator
}

func (tj *ExecutorMock) Start(w *workerpool.Worker) {
	fm := fakeMutant{
		name: w.Name,
		id:   w.ID,
	}
	tj.outCh <- fm
}

func TestWorker(t *testing.T) {
	executorQueue := make(chan workerpool.Executor)
	outCh := make(chan mutator.Mutator)

	worker := workerpool.NewWorker(1, "test")
	worker.Start(executorQueue)

	tj := &ExecutorMock{
		mutant: &fakeMutant{},
		outCh:  outCh,
	}

	executorQueue <- tj
	close(executorQueue)

	m := <-outCh
	got, ok := m.(fakeMutant)
	if !ok {
		t.Fatal("it should be a fakeMutant")
	}

	if got.name != "test" {
		t.Errorf("want %q, got %q", "test", got.name)
	}
	if got.id != 1 {
		t.Errorf("want %d, got %d", 1, got.id)
	}
}

func TestPool(t *testing.T) {
	t.Run("test executes work", func(t *testing.T) {
		configuration.Set(configuration.UnleashWorkersKey, 1)
		defer configuration.Reset()

		outCh := make(chan mutator.Mutator)

		pool := workerpool.Initialize("test")
		pool.Start()
		defer pool.Stop()

		tj := &ExecutorMock{
			mutant: &fakeMutant{},
			outCh:  outCh,
		}

		pool.AppendExecutor(tj)

		m := <-outCh
		got, ok := m.(fakeMutant)
		if !ok {
			t.Fatal("it should be a fakeMutant")
		}

		if got.name != "test" {
			t.Errorf("want %q, got %q", "test", got.name)
		}
		if got.id != 0 {
			t.Errorf("want %d, got %d", 0, got.id)
		}
	})

	t.Run("default uses runtime CPUs as number of workers", func(t *testing.T) {
		configuration.Set(configuration.UnleashWorkersKey, 0)
		defer configuration.Reset()

		pool := workerpool.Initialize("test")
		pool.Start()
		defer pool.Stop()

		if pool.ActiveWorkers() != runtime.NumCPU() {
			t.Errorf("want %d, got %d", runtime.NumCPU(), pool.ActiveWorkers())
		}
	})

	t.Run("default uses half of runtime CPUs as number of workers in integration mode", func(t *testing.T) {
		configuration.Set(configuration.UnleashWorkersKey, 0)
		configuration.Set(configuration.UnleashIntegrationMode, true)
		defer configuration.Reset()

		pool := workerpool.Initialize("test")
		pool.Start()
		defer pool.Stop()

		if pool.ActiveWorkers() != runtime.NumCPU()/2 {
			t.Errorf("want %d, got %d", runtime.NumCPU()/2, pool.ActiveWorkers())
		}
	})

	t.Run("can override CPUs", func(t *testing.T) {
		configuration.Set(configuration.UnleashWorkersKey, 3)
		defer configuration.Reset()

		pool := workerpool.Initialize("test")
		pool.Start()
		defer pool.Stop()

		if pool.ActiveWorkers() != 3 {
			t.Errorf("want %d, got %d", 3, pool.ActiveWorkers())
		}
	})

	t.Run("in integration mode, overrides CPUs by half", func(t *testing.T) {
		configuration.Set(configuration.UnleashWorkersKey, 2)
		configuration.Set(configuration.UnleashIntegrationMode, true)
		defer configuration.Reset()

		pool := workerpool.Initialize("test")
		pool.Start()
		defer pool.Stop()

		if pool.ActiveWorkers() != 1 {
			t.Errorf("want %d, got %d", 1, pool.ActiveWorkers())
		}
	})
}

type fakeMutant struct {
	name string
	id   int
}

func (fakeMutant) Type() mutator.Type {
	panic("not used in test")
}

func (fakeMutant) SetType(_ mutator.Type) {
	panic("not used in test")
}

func (fakeMutant) Status() mutator.Status {
	panic("not used in test")
}

func (fakeMutant) SetStatus(_ mutator.Status) {
	panic("not used in test")
}

func (fakeMutant) Position() token.Position {
	panic("not used in test")
}

func (fakeMutant) Pos() token.Pos {
	panic("not used in test")
}

func (fakeMutant) Pkg() string {
	panic("not used in test")
}

func (fakeMutant) SetWorkdir(_ string) {
	panic("not used in test")
}

func (fakeMutant) Workdir() string {
	panic("not used in test")
}

func (fakeMutant) Apply() error {
	panic("not used in test")
}

func (fakeMutant) Rollback() error {
	panic("not used in test")
}
