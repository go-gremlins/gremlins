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

package schemata_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

// twoPkgsModule is the twopkgs fixture as the engine sees it.
func twoPkgsModule(t *testing.T) gomodule.GoModule {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "twopkgs"))
	if err != nil {
		t.Fatal(err)
	}

	return gomodule.GoModule{Name: "twopkgs", Root: root, CallingDir: "."}
}

// fakeBuilds is a build seam that records how often each package was built,
// and how many builds ran at once, and answers each call through answer.
type fakeBuilds struct {
	mu         sync.Mutex
	calls      map[string]int
	running    int
	maxRunning map[int]int // by call number: the most builds running at once
	answer     func(ctx context.Context, pkg string, call int) error
	realOnNil  bool
}

func (f *fakeBuilds) build(ctx context.Context, dir, goTmp, overlay, bin, tags, pkg string) error {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
		f.maxRunning = map[int]int{}
	}
	f.calls[pkg]++
	call := f.calls[pkg]
	f.running++
	f.maxRunning[call] = max(f.maxRunning[call], f.running)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.running--
		f.mu.Unlock()
	}()
	if err := f.answer(ctx, pkg, call); err != nil || !f.realOnNil {
		return err
	}

	return schemata.BuildTest(ctx, dir, goTmp, overlay, bin, tags, pkg)
}

func (f *fakeBuilds) callsOf(pkg string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls[pkg]
}

// slowFor waits d, as a build that takes d would, or returns the context's
// error the way buildTest does when the context ends first.
func slowFor(ctx context.Context, pkg string, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return fmt.Errorf("schemata: build %s: %w", pkg, ctx.Err())
	}
}

// TestPrepareSlowBuildPlacesEveryMutant builds the twopkgs fixture with each
// package's build taking far longer than the per-mutant compile allowance,
// and requires every mutant placed: the per-mutant allowance bounds a
// mutant's compile, not the one schema build of the whole run.
func TestPrepareSlowBuildPlacesEveryMutant(t *testing.T) {
	t.Parallel()
	mod := twoPkgsModule(t)
	in := streamMutants(t, mod)
	const allowance = 20 * time.Millisecond
	f := &fakeBuilds{realOnNil: true, answer: func(ctx context.Context, pkg string, _ int) error {
		return slowFor(ctx, pkg, 10*allowance)
	}}
	runs := &nullRuns{}
	plan, err := schemata.PrepareWith(context.Background(), mod, t.TempDir(), "", in, ownPackage, allowance, 0, runs.run, f.build)
	if err != nil {
		t.Fatal(err)
	}
	checkAccounting(t, in, plan)
	if len(plan.Netted) != 0 || len(plan.Placed) != len(in) {
		t.Errorf("placed %d, netted %d (%v); want all %d placed", len(plan.Placed), len(plan.Netted), byPkg(plan).reasons, len(in))
	}
}

// TestBuildAllRetriesACutOffBuild fails each package's first build the way a
// build cut off at the deadline, killed by a signal, or out of memory
// fails, and requires the package rebuilt once and its binary kept.
func TestBuildAllRetriesACutOffBuild(t *testing.T) {
	t.Parallel()
	mod := twoPkgsModule(t)
	cutOff := map[string]error{
		"deadline": fmt.Errorf("schemata: build x: %w", context.DeadlineExceeded),
		"signal":   errors.New("schemata: build x: exit status 1\ngo build x: /go/pkg/tool/compile: signal: killed"),
		"oom":      errors.New("schemata: build x: exit status 2\nfatal error: runtime: out of memory"),
	}
	for name, first := range cutOff {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			const pkg = "twopkgs/ok"
			f := &fakeBuilds{realOnNil: true, answer: func(_ context.Context, _ string, call int) error {
				if call == 1 {
					return first
				}

				return nil
			}}
			b, errs := schemata.BuildAllWith(context.Background(), mod.Root, t.TempDir(), "", nil, []string{pkg}, time.Minute, f.build)
			if errs[pkg] != nil || b.Binaries[pkg] == "" {
				t.Errorf("binary %q, error %v; want the retry to build it", b.Binaries[pkg], errs[pkg])
			}
			if c := f.callsOf(pkg); c != 2 {
				t.Errorf("built %d times, want 2", c)
			}
		})
	}
}

// TestBuildAllDoesNotRetryACompileError fails a build with the compiler's
// report of an error in the source, which a second build cannot change, and
// requires it built once and reported as not compiling.
func TestBuildAllDoesNotRetryACompileError(t *testing.T) {
	t.Parallel()
	mod := twoPkgsModule(t)
	const pkg = "twopkgs/ok"
	f := &fakeBuilds{answer: func(context.Context, string, int) error {
		return errors.New("schemata: build twopkgs/ok: exit status 1\n# twopkgs/ok\n./ok.go:4:40: undefined: x")
	}}
	_, errs := schemata.BuildAllWith(context.Background(), mod.Root, t.TempDir(), "", nil, []string{pkg}, time.Minute, f.build)
	if c := f.callsOf(pkg); c != 1 {
		t.Errorf("built %d times, want once", c)
	}
	first, _, _ := strings.Cut(fmt.Sprint(errs[pkg]), "\n")
	if !strings.Contains(first, "does not compile") {
		t.Errorf("first line %q, want it to say the package does not compile", first)
	}
	if !strings.Contains(fmt.Sprint(errs[pkg]), "undefined: x") {
		t.Errorf("error %v lost the compiler's output", errs[pkg])
	}
}

// TestPrepareNetsABuildThatTimesOutTwice gives the build of every package a
// bound it cannot finish within, and requires each built twice, its mutants
// netted, and the reason's first line to say the build timed out.
func TestPrepareNetsABuildThatTimesOutTwice(t *testing.T) {
	t.Parallel()
	mod := twoPkgsModule(t)
	in := streamMutants(t, mod)
	f := &fakeBuilds{answer: func(ctx context.Context, pkg string, _ int) error {
		return slowFor(ctx, pkg, time.Hour)
	}}
	runs := &nullRuns{}
	plan, err := schemata.PrepareWith(context.Background(), mod, t.TempDir(), "", in, ownPackage, time.Minute, 50*time.Millisecond, runs.run, f.build)
	if err != nil {
		t.Fatal(err)
	}
	checkAccounting(t, in, plan)
	if len(plan.Placed) != 0 {
		t.Errorf("placed %d, want none", len(plan.Placed))
	}
	for _, n := range plan.Netted {
		first, _, _ := strings.Cut(n.Reason, "\n")
		if !strings.Contains(first, "timed out") || !strings.Contains(first, "retry") {
			t.Errorf("reason %q, want its first line to say the build timed out after a retry", first)
		}
		if !strings.Contains(n.Reason, context.DeadlineExceeded.Error()) {
			t.Errorf("reason %q lost the build's error", n.Reason)
		}
	}
	for _, pkg := range []string{"twopkgs/ok", "twopkgs/bad"} {
		if c := f.callsOf(pkg); c != 2 {
			t.Errorf("%s built %d times, want 2", pkg, c)
		}
	}
}

// TestBuildAllNetsAKilledBuildThatFailsAgain kills a package's build both
// times and requires the error's first line to say it was killed.
func TestBuildAllNetsAKilledBuildThatFailsAgain(t *testing.T) {
	t.Parallel()
	mod := twoPkgsModule(t)
	const pkg = "twopkgs/ok"
	f := &fakeBuilds{answer: func(context.Context, string, int) error {
		return errors.New("schemata: build twopkgs/ok: exit status 1\ncompile: signal: killed")
	}}
	_, errs := schemata.BuildAllWith(context.Background(), mod.Root, t.TempDir(), "", nil, []string{pkg}, time.Minute, f.build)
	if c := f.callsOf(pkg); c != 2 {
		t.Errorf("built %d times, want 2", c)
	}
	first, _, _ := strings.Cut(fmt.Sprint(errs[pkg]), "\n")
	if !strings.Contains(first, "killed") || !strings.Contains(first, "retry") {
		t.Errorf("first line %q, want it to say the build was killed after a retry", first)
	}
}

// TestBuildAllRetriesAtHalfParallelism cuts off the first build of more
// packages than there are CPUs, and requires the retries to run at most half
// as many at once as the first builds may.
func TestBuildAllRetriesAtHalfParallelism(t *testing.T) {
	t.Parallel()
	mod := twoPkgsModule(t)
	var pkgs []string
	for i := range 2 * runtime.NumCPU() {
		pkgs = append(pkgs, fmt.Sprintf("twopkgs/p%d", i))
	}
	f := &fakeBuilds{answer: func(ctx context.Context, pkg string, call int) error {
		_ = slowFor(ctx, pkg, 20*time.Millisecond)
		if call == 1 {
			return context.DeadlineExceeded
		}

		return errors.New("exit status 1\nno such package")
	}}
	schemata.BuildAllWith(context.Background(), mod.Root, t.TempDir(), "", nil, pkgs, time.Minute, f.build)
	if got, want := f.maxRunning[2], max(1, runtime.NumCPU()/2); got > want {
		t.Errorf("%d retries ran at once, want at most %d", got, want)
	}
	for _, p := range pkgs {
		if c := f.callsOf(p); c != 2 {
			t.Errorf("%s built %d times, want 2", p, c)
		}
	}
}

// TestBuildAllDoesNotRetryWhenTheRunEnds cancels the run while the first
// builds run, and requires no retry: nothing it built could be used.
func TestBuildAllDoesNotRetryWhenTheRunEnds(t *testing.T) {
	t.Parallel()
	mod := twoPkgsModule(t)
	const pkg = "twopkgs/ok"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeBuilds{answer: func(context.Context, string, int) error {
		cancel()

		return fmt.Errorf("schemata: build %s: %w", pkg, context.Canceled)
	}}
	_, errs := schemata.BuildAllWith(ctx, mod.Root, t.TempDir(), "", nil, []string{pkg}, time.Minute, f.build)
	if c := f.callsOf(pkg); c != 1 {
		t.Errorf("built %d times, want once", c)
	}
	if !errors.Is(errs[pkg], context.Canceled) {
		t.Errorf("error %v, want the run's cancellation", errs[pkg])
	}
}

// TestBuildAllKeepsTheRunsErrorWhenItEndsDuringTheRetry cancels the run
// while a retry runs, and requires the run's error, not a timed-out build.
func TestBuildAllKeepsTheRunsErrorWhenItEndsDuringTheRetry(t *testing.T) {
	t.Parallel()
	mod := twoPkgsModule(t)
	const pkg = "twopkgs/ok"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeBuilds{answer: func(_ context.Context, _ string, call int) error {
		if call == 1 {
			return context.DeadlineExceeded
		}
		cancel()

		return fmt.Errorf("schemata: build %s: %w", pkg, context.Canceled)
	}}
	_, errs := schemata.BuildAllWith(ctx, mod.Root, t.TempDir(), "", nil, []string{pkg}, time.Minute, f.build)
	if c := f.callsOf(pkg); c != 2 {
		t.Errorf("built %d times, want 2", c)
	}
	if !errors.Is(errs[pkg], context.Canceled) || strings.Contains(fmt.Sprint(errs[pkg]), "timed out") {
		t.Errorf("error %v, want the run's cancellation as it is", errs[pkg])
	}
}

// TestBuildTimeout derives the bound on the whole schema build: the
// configured value when there is one, otherwise the per-mutant compile
// allowance for each package built, but never under ten minutes.
func TestBuildTimeout(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		configured, allowance time.Duration
		pkgs                  int
		want                  time.Duration
	}{
		{0, 2 * time.Minute, 1, 10 * time.Minute},
		{0, 2 * time.Minute, 5, 10 * time.Minute},
		{0, 2 * time.Minute, 6, 12 * time.Minute},
		{0, 2 * time.Minute, 40, 80 * time.Minute},
		{0, time.Millisecond, 3, 10 * time.Minute},
		{3 * time.Minute, 2 * time.Minute, 40, 3 * time.Minute},
		{time.Millisecond, 2 * time.Minute, 1, time.Millisecond},
	} {
		if got := schemata.BuildTimeout(tc.configured, tc.allowance, tc.pkgs); got != tc.want {
			t.Errorf("BuildTimeout(%s, %s, %d) = %s, want %s", tc.configured, tc.allowance, tc.pkgs, got, tc.want)
		}
	}
}

// TestShowsOutOfMemory holds each marker as out-of-memory output, and output
// with none of them as not.
func TestShowsOutOfMemory(t *testing.T) {
	t.Parallel()
	for _, m := range schemata.OutOfMemoryMarkers {
		if !schemata.ShowsOutOfMemory("fatal error: runtime: " + m + "\n") {
			t.Errorf("output with %q not seen as out of memory", m)
		}
	}
	if schemata.ShowsOutOfMemory("./x.go:3:1: undefined: y\n") {
		t.Error("a compile error seen as out of memory")
	}
}
