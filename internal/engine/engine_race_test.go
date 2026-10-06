/*
 * Copyright 2026 The Gremlins Authors
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
	"go/token"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

// TestRunDoesNotReadATokenAWorkerIsMutating runs the engine over one token
// with two mutants, with an executor that applies its mutant as soon as it
// gets it, as the legacy executor does. The two mutants share the token's AST
// node, so once the walk has handed out the first of them it must not read
// the node again: a worker may be rewriting it.
//
// The viability check of the second mutant holds the walk until the first
// mutant has been applied, without ordering the two goroutines before that,
// so that -race reports a read the walk makes of the node after handing out
// the first mutant. It is evidence only under -race.
func TestRunDoesNotReadATokenAWorkerIsMutating(t *testing.T) {
	t.Parallel()
	mapFS, mod, c := loadFixture(defaultFixture, ".")
	defer c()
	wd := t.TempDir()
	for name, f := range mapFS {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(wd, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(wd, name), f.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	viperSet(map[string]any{})
	defer viperReset()

	applied := make(chan struct{})
	dealer := applyingDealer{t: t, workDir: wd, applied: applied, once: &sync.Once{}}
	via := &holdSecond{t: t, applied: applied}
	eng := engine.New(mod, testCodeData, dealer, engine.WithDirFs(mapFS), engine.WithViability(via))
	res := eng.Run(context.Background())

	got := map[mutator.Type]bool{}
	for _, m := range res.Mutants {
		got[m.Type()] = true
	}
	if !got[mutator.ConditionalsBoundary] || !got[mutator.ConditionalsNegation] || len(res.Mutants) != 2 {
		t.Errorf("mutants of the > token = %v, want its boundary and negation mutants", got)
	}
}

// holdSecond is a Viability that answers its first question at once and
// holds its second until applied is closed.
type holdSecond struct {
	t       *testing.T
	applied <-chan struct{}
	calls   int
}

func (h *holdSecond) Viable(_ token.Position, _ token.Token) bool {
	h.calls++
	if h.calls == 2 {
		select {
		case <-h.applied:
		case <-time.After(10 * time.Second):
			h.t.Error("the first mutant was not applied")
		}
	}

	return true
}

// applyingDealer makes executors that apply their mutant in workDir, roll it
// back, and close applied after the first of them.
type applyingDealer struct {
	t       *testing.T
	workDir string
	applied chan struct{}
	once    *sync.Once
}

func (d applyingDealer) NewExecutor(mut mutator.Mutator, outCh chan<- mutator.Mutator, wg *sync.WaitGroup) workerpool.Executor {
	return &applyingExecutor{d: d, mut: mut, outCh: outCh, wg: wg}
}

type applyingExecutor struct {
	d     applyingDealer
	mut   mutator.Mutator
	outCh chan<- mutator.Mutator
	wg    *sync.WaitGroup
}

func (e *applyingExecutor) Start(_ *workerpool.Worker) {
	defer e.wg.Done()
	e.mut.SetWorkdir(e.d.workDir)
	if err := e.mut.Apply(); err != nil {
		e.d.t.Errorf("apply: %v", err)
	}
	if err := e.mut.Rollback(); err != nil {
		e.d.t.Errorf("rollback: %v", err)
	}
	e.d.once.Do(func() { close(e.d.applied) })
	e.outCh <- e.mut
}
