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
	"go/token"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

// wantTimeout is the -timeout go is given: the mutant's own bound plus the two
// seconds that keep go's timeout from firing before Gremlins' own.
var wantTimeout = (2*time.Second + expectedTimeout*engine.DefaultTimeoutCoefficient).String()

type selectorStub struct {
	tests  []coverage.TestID
	mapped map[string]bool
}

func (s selectorStub) Mapped(pkg string) bool { return s.mapped[pkg] }

func (s selectorStub) TestsFor(token.Position) []coverage.TestID { return s.tests }

// commandsHolder records every command the executor issued, not only the last:
// selection can turn one mutant into several runs, one per package.
type commandsHolder struct {
	m    sync.Mutex
	args [][]string
}

func (h *commandsHolder) record(args []string) {
	h.m.Lock()
	defer h.m.Unlock()
	h.args = append(h.args, args)
}

func (h *commandsHolder) commands() []string {
	h.m.Lock()
	defer h.m.Unlock()
	cmds := make([]string, 0, len(h.args))
	for _, a := range h.args {
		cmds = append(cmds, "go "+strings.Join(a, " "))
	}

	return cmds
}

func recordingExec(h *commandsHolder) func(ctx context.Context, command string, args ...string) *exec.Cmd {
	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		h.record(args)

		return fakeExecCommandSuccess(ctx, command, args...)
	}
}

func runWithSelector(t *testing.T, sel engine.TestSelector, intMode bool) (*commandsHolder, mutator.Mutator) {
	t.Helper()

	return runWith(t, sel, intMode, false)
}

func runCrossPackage(t *testing.T, sel engine.TestSelector) (*commandsHolder, mutator.Mutator) {
	t.Helper()

	return runWith(t, sel, false, true)
}

// dependentsStub stands in for the import graph.
type dependentsStub map[string][]string

func (d dependentsStub) Dependents(pkg string) []string { return d[pkg] }

func runWith(t *testing.T, sel engine.TestSelector, intMode, crossPkg bool) (*commandsHolder, mutator.Mutator) {
	t.Helper()

	viperSet(map[string]any{
		configuration.UnleashDryRunKey:       false,
		configuration.UnleashIntegrationMode: intMode,
		configuration.UnleashCrossPackageKey: crossPkg,
	})
	t.Cleanup(viperReset)

	holder := &commandsHolder{}
	mod := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}
	opts := []engine.ExecutorDealerOption{engine.WithExecContext(recordingExec(holder))}
	if sel != nil {
		opts = append(opts, engine.WithTestSelection(sel))
	}
	if crossPkg {
		opts = append(opts, engine.WithDependents(dependentsStub{
			"example.com/vm": {"example.com"},
		}))
	}
	mjd := engine.NewExecutorDealer(mod, newWdDealerStub(t), expectedTimeout, opts...)
	mut := &mutantStub{
		status:  mutator.Runnable,
		mutType: mutator.ConditionalsBoundary,
		pkg:     "example.com/vm",
	}

	outCh := make(chan mutator.Mutator)
	wg := sync.WaitGroup{}
	wg.Add(1)
	executor := mjd.NewExecutor(mut, outCh, &wg)
	go func() {
		<-outCh
		close(outCh)
	}()
	executor.Start(&workerpool.Worker{Name: "test", ID: 1})
	wg.Wait()

	return holder, mut
}

func TestSelectionRunsOnlyTheTestsThatExecuteTheMutatedLine(t *testing.T) {
	sel := selectorStub{
		mapped: map[string]bool{"example.com/vm": true},
		tests: []coverage.TestID{
			{Pkg: "example.com/vm", Name: "TestSize"},
			{Pkg: "example.com/vm", Name: "TestClamp"},
		},
	}

	holder, _ := runWithSelector(t, sel, false)

	want := []string{"go test -timeout " + wantTimeout + " -failfast -run ^(TestSize|TestClamp)$ example.com/vm"}
	if diff := cmp.Diff(want, holder.commands()); diff != "" {
		t.Errorf("commands mismatch (-want +got):\n%s", diff)
	}
}

// The half of the feature package scoping cannot do: a test that lives
// elsewhere but executes the mutated line is run, and a mutant it kills is no
// longer reported as surviving.
func TestSelectionRunsCoveringTestsFromOtherPackages(t *testing.T) {
	sel := selectorStub{
		mapped: map[string]bool{"example.com/vm": true, "example.com": true},
		tests: []coverage.TestID{
			{Pkg: "example.com", Name: "TestRangeDescending"},
			{Pkg: "example.com/vm", Name: "TestSize"},
		},
	}

	holder, mut := runCrossPackage(t, sel)

	// One invocation listing both packages, not one per package: the build is
	// most of what a mutant costs, and `go test` builds them together.
	want := []string{
		"go test -timeout " + wantTimeout +
			" -failfast -run ^(TestRangeDescending|TestSize)$ example.com example.com/vm",
	}
	if diff := cmp.Diff(want, holder.commands()); diff != "" {
		t.Errorf("commands mismatch (-want +got):\n%s", diff)
	}

	// A surviving mutant has to say what it survived, or the report repeats the
	// mistake package scoping made: a verdict with no account of what produced it.
	wantTests := []string{"example.com.TestRangeDescending", "example.com/vm.TestSize"}
	if diff := cmp.Diff(wantTests, mut.TestsRun()); diff != "" {
		t.Errorf("TestsRun() mismatch (-want +got):\n%s", diff)
	}
}

// A name shared by two packages appears once in the pattern. -run applies to
// every package listed, so it runs in both regardless; repeating it would only
// make the command longer.
func TestSelectionNamesEachTestOnceAcrossPackages(t *testing.T) {
	sel := selectorStub{
		mapped: map[string]bool{"example.com/vm": true, "example.com": true},
		tests: []coverage.TestID{
			{Pkg: "example.com", Name: "TestSize"},
			{Pkg: "example.com/vm", Name: "TestSize"},
		},
	}

	holder, mut := runCrossPackage(t, sel)

	want := []string{
		"go test -timeout " + wantTimeout + " -failfast -run ^(TestSize)$ example.com example.com/vm",
	}
	if diff := cmp.Diff(want, holder.commands()); diff != "" {
		t.Errorf("commands mismatch (-want +got):\n%s", diff)
	}
	// The report still names both, because both were run against the mutant.
	wantTests := []string{"example.com.TestSize", "example.com/vm.TestSize"}
	if diff := cmp.Diff(wantTests, mut.TestsRun()); diff != "" {
		t.Errorf("TestsRun() mismatch (-want +got):\n%s", diff)
	}
}

// By default a mutant is judged by its own package's covering tests only. Those
// tests were going to be built and their fixture paid for anyway, so running a
// subset of them cannot cost more than running all of them — which is what
// makes selection safe to leave on.
func TestSelectionKeepsToTheMutatedPackageByDefault(t *testing.T) {
	sel := selectorStub{
		mapped: map[string]bool{"example.com/vm": true, "example.com": true},
		tests: []coverage.TestID{
			{Pkg: "example.com", Name: "TestRangeDescending"},
			{Pkg: "example.com/vm", Name: "TestSize"},
		},
	}

	holder, mut := runWithSelector(t, sel, false)

	want := []string{
		"go test -timeout " + wantTimeout + " -failfast -run ^(TestSize)$ example.com/vm",
	}
	if diff := cmp.Diff(want, holder.commands()); diff != "" {
		t.Errorf("commands mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"example.com/vm.TestSize"}, mut.TestsRun()); diff != "" {
		t.Errorf("TestsRun() mismatch (-want +got):\n%s", diff)
	}
}

// With no covering test in the mutated package there is nothing to narrow to,
// and reaching into other packages is not this mode's job.
func TestSelectionFallsBackWhenOnlyOtherPackagesCover(t *testing.T) {
	sel := selectorStub{
		mapped: map[string]bool{"example.com/vm": true, "example.com": true},
		tests:  []coverage.TestID{{Pkg: "example.com", Name: "TestRangeDescending"}},
	}

	holder, mut := runWithSelector(t, sel, false)

	want := []string{"go test -timeout " + wantTimeout + " -failfast example.com/vm"}
	if diff := cmp.Diff(want, holder.commands()); diff != "" {
		t.Errorf("commands mismatch (-want +got):\n%s", diff)
	}
	if got := mut.TestsRun(); len(got) != 0 {
		t.Errorf("want no recorded tests when the whole suite ran, got %v", got)
	}
}

// --cross-package on its own needs no coverage map at all: which packages a
// mutation could break is a question about imports, so it runs their whole
// suites and asks nothing about which tests reach the line.
func TestCrossPackageWithoutSelectionRunsWholeSuitesOfTheDependents(t *testing.T) {
	holder, mut := runCrossPackage(t, nil)

	want := []string{
		"go test -timeout " + wantTimeout + " -failfast example.com/vm example.com",
	}
	if diff := cmp.Diff(want, holder.commands()); diff != "" {
		t.Errorf("commands mismatch (-want +got):\n%s", diff)
	}
	if got := mut.TestsRun(); len(got) != 0 {
		t.Errorf("want no recorded tests when whole suites ran, got %v", got)
	}
}

func TestSelectionFallsBackToTheWholeSuite(t *testing.T) {
	wholeSuite := []string{"go test -timeout " + wantTimeout + " -failfast example.com/vm"}

	testCases := map[string]struct {
		sel     engine.TestSelector
		intMode bool
	}{
		"without a selector at all": {
			sel: nil,
		},
		// What is missing from the map is exactly what selection would skip, so
		// an unmapped package must run everything it has.
		"when the mutated package could not be mapped": {
			sel: selectorStub{
				mapped: map[string]bool{"example.com": true},
				tests:  []coverage.TestID{{Pkg: "example.com", Name: "TestRange"}},
			},
		},
		// A covered mutant with no covering test means the map is incomplete,
		// not that no test exercises the line.
		"when the map names no test for a covered mutant": {
			sel: selectorStub{mapped: map[string]bool{"example.com/vm": true}},
		},
		"in integration mode, where the run is the whole module anyway": {
			sel: selectorStub{
				mapped: map[string]bool{"example.com/vm": true},
				tests:  []coverage.TestID{{Pkg: "example.com/vm", Name: "TestSize"}},
			},
			intMode: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			holder, mut := runWithSelector(t, tc.sel, tc.intMode)

			want := wholeSuite
			if tc.intMode {
				want = []string{"go test -timeout " + wantTimeout + " -failfast ./..."}
			}
			if diff := cmp.Diff(want, holder.commands()); diff != "" {
				t.Errorf("commands mismatch (-want +got):\n%s", diff)
			}
			if got := mut.TestsRun(); len(got) != 0 {
				t.Errorf("want no recorded tests when the whole suite ran, got %v", got)
			}
		})
	}
}
