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
	"sync"

	"github.com/go-gremlins/gremlins/pkg/mutant"
	"github.com/go-gremlins/gremlins/pkg/mutator/internal/workerpool"
)

type MutantJob struct {
	mutant mutant.Mutant
	outCh  chan mutant.Mutant
	apply  func(*workerpool.Worker, mutant.Mutant, chan<- mutant.Mutant, *sync.WaitGroup)
	wg     *sync.WaitGroup
}

func (m MutantJob) Start(worker *workerpool.Worker) {
	m.apply(worker, m.mutant, m.outCh, m.wg)
}
