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
	"testing"

	"github.com/go-gremlins/gremlins/pkg/mutant"
	"github.com/go-gremlins/gremlins/pkg/mutator/internal/workerpool"
)

type JobMock struct {
	mutant mutant.Mutant
	outCh  chan<- mutant.Mutant
}

func (tj *JobMock) Start(w *workerpool.Worker) {
	fm := fakeMutant{
		name: w.Name,
		id:   w.Id,
	}
	tj.outCh <- fm
}

func TestWorker(t *testing.T) {
	jobQueue := make(chan workerpool.Job)
	outCh := make(chan mutant.Mutant)

	worker := workerpool.NewWorker(1, "test")
	worker.Start(jobQueue)

	tj := &JobMock{
		mutant: &fakeMutant{},
		outCh:  outCh,
	}

	jobQueue <- tj
	close(jobQueue)

	m := <-outCh
	got := m.(fakeMutant)

	if got.name != "test" {
		t.Errorf("want %q, got %q", "test", got.name)
	}
	if got.id != 1 {
		t.Errorf("want %d, got %d", 1, got.id)
	}
}

func TestPool(t *testing.T) {
	outCh := make(chan mutant.Mutant)

	pool := workerpool.Initialise("test", 1)
	pool.Start()
	defer pool.Stop()

	tj := &JobMock{
		mutant: &fakeMutant{},
		outCh:  outCh,
	}

	pool.AppendJob(tj)

	m := <-outCh
	got := m.(fakeMutant)

	if got.name != "test" {
		t.Errorf("want %q, got %q", "test", got.name)
	}
	if got.id != 0 {
		t.Errorf("want %d, got %d", 0, got.id)
	}
}

type fakeMutant struct {
	name string
	id   int
}

func (f fakeMutant) Type() mutant.Type {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) SetType(mt mutant.Type) {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) Status() mutant.Status {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) SetStatus(s mutant.Status) {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) Position() token.Position {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) Pos() token.Pos {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) Pkg() string {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) SetWorkdir(p string) {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) Apply() error {
	//TODO implement me
	panic("implement me")
}

func (f fakeMutant) Rollback() error {
	//TODO implement me
	panic("implement me")
}
