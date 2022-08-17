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

package workerpool

import (
	"runtime"
	"sync"

	"github.com/go-gremlins/gremlins/configuration"
)

// Executor is the unit of work that executes a task.
type Executor interface {
	Start(worker *Worker)
}

// Worker takes an executor and starts the actual executor.
type Worker struct {
	stopCh chan struct{}
	Name   string
	ID     int
}

// NewWorker instantiates a new worker with an ID and name.
func NewWorker(id int, name string) *Worker {
	return &Worker{
		Name: name,
		ID:   id,
	}
}

// Start gets an executor queue and starts working on it.
func (w *Worker) Start(executorQueue <-chan Executor) {
	w.stopCh = make(chan struct{})
	go func() {
		for {
			executor, ok := <-executorQueue
			if !ok {
				w.stopCh <- struct{}{}

				break
			}
			executor.Start(w)
		}
	}()
}

func (w *Worker) stop() {
	<-w.stopCh
}

// Pool manages and limits the number of concurrent Worker.
type Pool struct {
	queue   chan Executor
	name    string
	workers []*Worker
	size    int
}

// Initialize creates a new Pool with a name and the number of parallel
// workers it will use.
func Initialize(name string) *Pool {
	wNum := configuration.Get[int](configuration.UnleashWorkersKey)
	intMode := configuration.Get[bool](configuration.UnleashIntegrationMode)

	p := &Pool{
		size: size(wNum, intMode),
		name: name,
	}
	p.workers = []*Worker{}
	for i := 0; i < p.size; i++ {
		w := NewWorker(i, p.name)
		p.workers = append(p.workers, w)
	}
	p.queue = make(chan Executor, 1)

	return p
}

func size(wNum int, intMode bool) int {
	if wNum == 0 {
		wNum = runtime.NumCPU()
	}
	if intMode && wNum > 1 {
		wNum /= 2
	}

	return wNum
}

// AppendExecutor adds a new Executor to the queue of Executor to be processed.
func (p *Pool) AppendExecutor(executor Executor) {
	p.queue <- executor
}

// Start the Pool.
func (p *Pool) Start() {
	for _, w := range p.workers {
		w.Start(p.queue)
	}
}

// Stop the Pool and wait for all the pending Worker to complete.
func (p *Pool) Stop() {
	close(p.queue)
	var wg sync.WaitGroup
	for _, worker := range p.workers {
		wg.Add(1)
		go func(w *Worker) {
			defer wg.Done()
			w.stop()
		}(worker)
	}
	wg.Wait()
}

// ActiveWorkers gives the number of active workers on the Pool.
func (p *Pool) ActiveWorkers() int {
	return len(p.workers)
}
