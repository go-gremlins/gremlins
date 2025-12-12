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
	"errors"
	"fmt"
	"go/token"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

var viperMutex sync.RWMutex

func init() {
	viperMutex.Lock()
	viperReset()
}

func viperSet(set map[string]any) {
	viperMutex.Lock()
	for k, v := range set {
		configuration.Set(k, v)
	}
}

func viperReset() {
	configuration.Reset()
	for _, mt := range mutator.Types {
		configuration.Set(configuration.MutantTypeEnabledKey(mt), true)
	}
	viperMutex.Unlock()
}

func loadFixture(fixture, fromPackage string) (fstest.MapFS, gomodule.GoModule, func()) {
	//nolint:gosec // test code reading test fixtures
	f, err := os.Open(fixture)
	if err != nil {
		panic(fmt.Sprintf("failed to open test fixture %s: %v", fixture, err))
	}
	src, err := io.ReadAll(f)
	if err != nil {
		_ = f.Close()
		panic(fmt.Sprintf("failed to read test fixture %s: %v", fixture, err))
	}
	filename := filenameFromFixture(fixture)
	mapFS := fstest.MapFS{
		filename: {Data: src},
	}

	return mapFS, gomodule.GoModule{
			Name:       "example.com",
			Root:       ".",
			CallingDir: fromPackage,
		}, func() {
			_ = f.Close()
		}
}

func filenameFromFixture(fix string) string {
	return strings.ReplaceAll(fix, "_go", ".go")
}

type dealerStub struct {
	t     *testing.T
	fnGet func(idf string) (string, error)
}

func newWdDealerStub(t *testing.T) *dealerStub {
	t.Helper()

	return &dealerStub{t: t, fnGet: func(_ string) (string, error) {
		return t.TempDir(), nil
	}}
}

func (d dealerStub) Get(idf string) (string, error) {
	return d.fnGet(idf)
}

func (dealerStub) Clean() {}

func (dealerStub) WorkDir() string { return "/tmp" }

type executorDealerStub struct {
	gotMutants []mutator.Mutator
}

func newJobDealerStub(t *testing.T) *executorDealerStub {
	t.Helper()

	return &executorDealerStub{}
}

func (j *executorDealerStub) NewExecutor(mut mutator.Mutator, outCh chan<- mutator.Mutator, wg *sync.WaitGroup) workerpool.Executor {
	j.gotMutants = append(j.gotMutants, mut)

	return &executorStub{
		mut:   mut,
		outCh: outCh,
		wg:    wg,
	}
}

type executorStub struct {
	mut   mutator.Mutator
	outCh chan<- mutator.Mutator
	wg    *sync.WaitGroup
}

func (j *executorStub) Start(_ *workerpool.Worker) {
	j.outCh <- j.mut
	j.wg.Done()
}

type mutantStub struct {
	worDir         string
	pkg            string
	position       token.Position
	status         mutator.Status
	mutType        mutator.Type
	applyCalled    bool
	rollbackCalled bool

	hasApplyError bool
}

func (m *mutantStub) Type() mutator.Type {
	return m.mutType
}

func (m *mutantStub) SetType(mt mutator.Type) {
	m.mutType = mt
}

func (m *mutantStub) Status() mutator.Status {
	return m.status
}

func (m *mutantStub) SetStatus(s mutator.Status) {
	m.status = s
}

func (m *mutantStub) Position() token.Position {
	return m.position
}

func (*mutantStub) Pos() token.Pos {
	panic("not used in test")
}

func (m *mutantStub) Pkg() string {
	return m.pkg
}

func (m *mutantStub) SetWorkdir(w string) {
	m.worDir = w
}

func (m *mutantStub) Workdir() string {
	return m.worDir
}

func (m *mutantStub) Apply() error {
	m.applyCalled = true
	if m.hasApplyError {
		return errors.New("test error")
	}

	return nil
}

func (m *mutantStub) Rollback() error {
	m.rollbackCalled = true

	return nil
}
