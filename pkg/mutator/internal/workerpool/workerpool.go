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

import "sync"

type Job interface {
	Start(worker *Worker)
}

type Worker struct {
	Name   string
	Id     int
	stopCh chan struct{}
}

func NewWorker(id int, name string) *Worker {
	return &Worker{
		Name: name,
		Id:   id,
	}
}

func (w *Worker) Start(jobQueue <-chan Job) {
	w.stopCh = make(chan struct{})
	go func() {
		for {
			job, ok := <-jobQueue
			if !ok {
				w.stopCh <- struct{}{}
				break
			}
			job.Start(w)
		}
	}()
}

func (w *Worker) stop() {
	<-w.stopCh
}

type Pool struct {
	queue   chan Job
	name    string
	workers []*Worker
	size    int
}

func Initialise(name string, size int) *Pool {
	p := &Pool{
		size: size,
		name: name,
	}
	p.workers = []*Worker{}
	for i := 0; i < p.size; i++ {
		w := NewWorker(i, p.name)
		p.workers = append(p.workers, w)
	}
	p.queue = make(chan Job, 1)

	return p
}

func (p *Pool) AppendJob(job Job) {
	p.queue <- job
}

func (p *Pool) Start() {
	for _, w := range p.workers {
		w.Start(p.queue)
	}
}

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
