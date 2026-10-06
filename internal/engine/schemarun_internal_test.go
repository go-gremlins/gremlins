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

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/engine/workdir"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/report"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

// lineMutant is a mutator.Mutator at a line of a package. Only what test
// selection and an executor that does not run it touch is implemented.
type lineMutant struct {
	mutator.Mutator
	pkg    string
	line   int
	status mutator.Status
	tests  []string
}

func (m *lineMutant) Pkg() string                { return m.pkg }
func (m *lineMutant) Position() token.Position   { return token.Position{Filename: "f.go", Line: m.line} }
func (m *lineMutant) Status() mutator.Status     { return m.status }
func (m *lineMutant) SetStatus(s mutator.Status) { m.status = s }
func (m *lineMutant) SetTestsRun(tests []string) { m.tests = tests }
func (m *lineMutant) TestsRun() []string         { return m.tests }
func (*lineMutant) SetWorkdir(string)            {}
func (*lineMutant) Type() mutator.Type           { return mutator.ArithmeticBase }
func (*lineMutant) Workdir() string              { return "" }

// lineSelector is a test map answering by line.
type lineSelector struct {
	mapped map[string]bool
	tests  map[int][]coverage.TestID
}

func (s lineSelector) Mapped(pkg string) bool                        { return s.mapped[pkg] }
func (s lineSelector) TestsFor(pos token.Position) []coverage.TestID { return s.tests[pos.Line] }

type dependents map[string][]string

func (d dependents) Dependents(pkg string) []string { return d[pkg] }

// TestSchemaTargetsAreTheExecutorSelection holds the packages Prepare is told
// to build and null-check for each mutated package, and the tests each
// package's null run runs, to what the schema executors of those mutants
// select when they run.
func TestSchemaTargetsAreTheExecutorSelection(t *testing.T) {
	t.Parallel()
	const a, b, c, d = "m/a", "m/b", "m/c", "m/d"
	deps := dependents{a: {c, d}, b: {d}}
	testCases := map[string]struct {
		dealer    MutantExecutorDealer
		muts      []*lineMutant
		wantPkgs  map[string][]string
		wantTests map[string][]string
	}{
		"whole_suites": {
			muts:      []*lineMutant{{pkg: a, line: 1}, {pkg: a, line: 2}, {pkg: b, line: 1}},
			wantPkgs:  map[string][]string{a: {a}, b: {b}},
			wantTests: map[string][]string{a: nil, b: nil},
		},
		"cross_package": {
			dealer:    MutantExecutorDealer{testSelection: testSelection{crossPackage: true, dependents: deps}},
			muts:      []*lineMutant{{pkg: a, line: 1}, {pkg: b, line: 1}},
			wantPkgs:  map[string][]string{a: {a, c, d}, b: {b, d}},
			wantTests: map[string][]string{a: nil, b: nil, c: nil, d: nil},
		},
		"test_selection_narrows": {
			dealer: MutantExecutorDealer{testSelection: testSelection{crossPackage: true, dependents: deps, testMap: lineSelector{
				mapped: map[string]bool{a: true},
				tests: map[int][]coverage.TestID{
					1: {{Pkg: c, Name: "TestC"}},
					2: {{Pkg: a, Name: "TestA"}, {Pkg: c, Name: "TestC2"}},
				},
			}}},
			muts:      []*lineMutant{{pkg: a, line: 1}, {pkg: a, line: 2}},
			wantPkgs:  map[string][]string{a: {a, c}},
			wantTests: map[string][]string{a: {"TestA", "TestC2"}, c: {"TestC", "TestA", "TestC2"}},
		},
		"one_whole_suite_widens_the_package": {
			dealer: MutantExecutorDealer{testSelection: testSelection{crossPackage: true, dependents: deps, testMap: lineSelector{
				mapped: map[string]bool{a: true},
				tests:  map[int][]coverage.TestID{1: {{Pkg: c, Name: "TestC"}}},
			}}},
			// Line 3 has no tests in the map: its mutant runs whole suites.
			muts:      []*lineMutant{{pkg: a, line: 1}, {pkg: a, line: 3}},
			wantPkgs:  map[string][]string{a: {a, c, d}},
			wantTests: map[string][]string{a: nil, c: nil, d: nil},
		},
		"integration_mode_runs_whole_suites": {
			dealer: MutantExecutorDealer{testSelection: testSelection{integrationMode: true, testMap: lineSelector{
				mapped: map[string]bool{a: true},
				tests:  map[int][]coverage.TestID{1: {{Pkg: a, Name: "TestA"}}},
			}}},
			muts:      []*lineMutant{{pkg: a, line: 1}},
			wantPkgs:  map[string][]string{a: {a}},
			wantTests: map[string][]string{a: nil},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tc.dealer.mod = gomodule.GoModule{Name: "m", Root: "/r"}
			var runnable []mutator.Mutator
			for _, m := range tc.muts {
				runnable = append(runnable, m)
			}
			targets := tc.dealer.schemaTargets(runnable)

			// What the executors select, mutant by mutant.
			selected := map[string][]string{}
			for _, m := range tc.muts {
				ex, _ := tc.dealer.NewSchemaExecutor(m, 1, nil, nil, nil).(*schemaExecutor)
				sel := ex.legacy.selectTests(m.Pkg())
				selected[m.pkg] = append(selected[m.pkg], sel.pkgs...)
			}
			for pkg, pkgs := range selected {
				slices.Sort(pkgs)
				pkgs = slices.Compact(pkgs)
				got := targets.testPkgs(pkg)
				if !slices.Equal(got, pkgs) {
					t.Errorf("testPkgs(%s) = %v, the executors select %v", pkg, got, pkgs)
				}
				if !slices.Equal(got, tc.wantPkgs[pkg]) {
					t.Errorf("testPkgs(%s) = %v, want %v", pkg, got, tc.wantPkgs[pkg])
				}
			}
			for pkg, want := range tc.wantTests {
				slices.Sort(want)
				dir := filepath.Join("/r", strings.TrimPrefix(pkg, "m/"))
				if got := targets.testsAt(dir); !slices.Equal(got, want) {
					t.Errorf("tests at %s = %v, want %v", dir, got, want)
				}
			}
		})
	}
}

// skipWithoutShell skips a test whose stand-in test binary is a /bin/sh
// script: Windows, which the CI matrix includes, has none.
func skipWithoutShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
}

// writeScript writes an executable shell script standing in for a test binary.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	skipWithoutShell(t)
	path := filepath.Join(t.TempDir(), "pkg.test")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil { //nolint:gosec // G306: the script must be executable
		t.Fatal(err)
	}

	return path
}

// TestSchemaNullRun runs a stand-in test binary the way Prepare's null run
// does and checks the command and the error.
//
// It is not parallel: it sets GREMLINS_MUTANT in the environment.
func TestSchemaNullRun(t *testing.T) {
	skipWithoutShell(t)
	t.Setenv("GREMLINS_MUTANT", "7")
	t.Setenv("GREMLINS_REACHED", "/nowhere")
	dealer := MutantExecutorDealer{execContext: exec.CommandContext, testExecutionTime: 3 * time.Second, wdDealer: wdStub{dir: t.TempDir()}}
	cpuDealer := dealer
	cpuDealer.testCPU = 2

	t.Run("invocation", func(t *testing.T) { testNullRunInvocation(t, cpuDealer) })

	t.Run("whole_suite", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out")
		bin := writeScript(t, `echo "$@" > `+out)
		if err := dealer.nullRun(context.Background(), bin, t.TempDir(), "", nil); err != nil {
			t.Fatalf("nullRun: %v", err)
		}
		raw, _ := os.ReadFile(out) //nolint:gosec // G304: test code reading its script's output
		if got, want := strings.TrimSpace(string(raw)), "-test.count=1 -test.timeout 3s -test.failfast -test.paniconexit0"; got != want {
			t.Errorf("args = %q, want %q", got, want)
		}
	})

	t.Run("failure", func(t *testing.T) {
		bin := writeScript(t, "echo '=== RUN   TestA'\necho '--- FAIL: TestA (0.00s)'\necho FAIL\nexit 1")
		err := dealer.nullRun(context.Background(), bin, t.TempDir(), "", nil)
		if err == nil {
			t.Fatal("nullRun of a failing binary returned nil")
		}
		first, _, _ := strings.Cut(err.Error(), "\n")
		if !strings.Contains(first, "exit status 1") || !strings.Contains(first, "--- FAIL: TestA") {
			t.Errorf("first line %q does not say how and where it failed", first)
		}
	})

	t.Run("overlay", func(t *testing.T) {
		// The null run, in a copy of the module, gets the overlay a mutant's
		// run gets in its worker copy: a test running the go command builds
		// the schema source in both. It runs in the copy, never in the
		// user's tree, so a test writing into its directory changes nothing
		// there.
		out := filepath.Join(t.TempDir(), "out")
		bin := writeScript(t, `{ pwd; env; } > `+out+`; : > written.txt`)
		root, work := t.TempDir(), t.TempDir()
		d := dealer
		d.mod = gomodule.GoModule{Name: "m", Root: root}
		wdd := workdir.NewCachedDealer(work, root)
		d.wdDealer = wdd
		build := &schemata.Build{
			Dir: filepath.Join(work, "schemata"), Src: filepath.Join(work, "schema"),
			Rewritten: []string{"a.go", filepath.Join("p", "b.go")},
		}
		pkgDir := filepath.Join(root, "p")
		if err := os.Mkdir(pkgDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := d.schemaNullRun(context.Background(), work, schemaTargets{})(build, bin, pkgDir); err != nil {
			t.Fatalf("nullRun: %v", err)
		}
		raw, err := os.ReadFile(out) //nolint:gosec // G304: test code reading its script's output
		if err != nil {
			t.Fatal(err)
		}
		copyRoot, err := wdd.Get(nullCopyID)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(raw), "\n")
		if want := filepath.Join(copyRoot, "p"); lines[0] != want {
			t.Errorf("null run dir = %s, want %s in the copy", lines[0], want)
		}
		if _, err := os.Stat(filepath.Join(pkgDir, "written.txt")); !os.IsNotExist(err) {
			t.Errorf("the null run wrote into the module tree: %v", err)
		}
		var overlay string
		for _, l := range lines {
			if flags, ok := strings.CutPrefix(l, "GOFLAGS="); ok {
				for _, f := range strings.Fields(flags) {
					if p, ok := strings.CutPrefix(f, "-overlay="); ok {
						overlay = p
					}
				}
			}
		}
		if overlay == "" {
			t.Fatalf("no -overlay in the null run's GOFLAGS:\n%s", raw)
		}
		data, err := os.ReadFile(overlay) //nolint:gosec // G304: test code reading the null run's overlay
		if err != nil {
			t.Fatal(err)
		}
		var o struct{ Replace map[string]string }
		if err := json.Unmarshal(data, &o); err != nil {
			t.Fatal(err)
		}
		want := map[string]string{
			filepath.Join(copyRoot, "a.go"):      filepath.Join(build.Src, "a.go"),
			filepath.Join(copyRoot, "p", "b.go"): filepath.Join(build.Src, "p", "b.go"),
			// The build copy, which runtime.Caller names in the binary.
			filepath.Join(build.Dir, "a.go"):      filepath.Join(build.Src, "a.go"),
			filepath.Join(build.Dir, "p", "b.go"): filepath.Join(build.Src, "p", "b.go"),
		}
		if !maps.Equal(o.Replace, want) {
			t.Errorf("overlay = %v, want %v", o.Replace, want)
		}
	})

	t.Run("hang", func(t *testing.T) {
		d := dealer
		d.testExecutionTime = 100 * time.Millisecond
		bin := writeScript(t, "exec sleep 60")
		start := time.Now()
		err := d.nullRun(context.Background(), bin, t.TempDir(), "", nil)
		if err == nil || !strings.Contains(err.Error(), "no verdict within") {
			t.Errorf("err = %v, want a bound error", err)
		}
		if el := time.Since(start); el > d.testExecutionTime+schemaBackstopGrace+5*time.Second {
			t.Errorf("the null run took %s", el)
		}
	})
}

// TestSchemaNullRunLeavesTheModuleTree prepares the nullcopy fixture, whose
// test writes out.txt into its working directory, and requires the fixture
// to hold no out.txt afterwards: the null run executes in a copy.
//
// It is not parallel: it changes the working directory, as the CLI's
// relative target does.
func TestSchemaNullRunLeavesTheModuleTree(t *testing.T) {
	t.Chdir(filepath.Join("testdata", "nullcopy"))
	mod, err := gomodule.Init("./w")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(mod.Root) {
		t.Fatalf("gomodule.Init(./w) gave the absolute root %s: the test proves nothing", mod.Root)
	}
	out := filepath.Join("w", "out.txt")
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("the fixture already holds %s: %v", out, err)
	}
	wdd := workdir.NewCachedDealer(t.TempDir(), mod.Root)
	defer wdd.Clean()
	d := NewExecutorDealer(mod, wdd, time.Second)
	abs, err := absModule(mod)
	if err != nil {
		t.Fatal(err)
	}
	d.mod = abs

	src, err := os.ReadFile(filepath.Join("w", "w.go"))
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	// Named relative to the calling directory, as the engine's walk does.
	file, err := parser.ParseFile(set, "w.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var runnable []mutator.Mutator
	ast.Inspect(file, func(n ast.Node) bool {
		if node, ok := NewTokenNode(n); ok && node.Tok() == token.ADD {
			m := NewTokenMutant("nullcopy/w", set, file, node)
			m.SetType(mutator.ArithmeticBase)
			m.SetStatus(mutator.Runnable)
			runnable = append(runnable, m)
		}

		return true
	})
	if len(runnable) != 1 {
		t.Fatalf("%d mutants, want Add's", len(runnable))
	}

	eng := New(mod, CodeData{}, d)
	plan, err := eng.prepareSchemata(context.Background(), d, abs, runnable)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Placed) != 1 {
		t.Fatalf("plan %+v, want Add's mutant placed (its null run passed)", plan)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		_ = os.Remove(out)
		t.Errorf("the null run wrote %s into the module tree: %v", out, err)
	}
}

// testNullRunInvocation checks the null run's flags, directory and
// environment.
func testNullRunInvocation(t *testing.T, d MutantExecutorDealer) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out")
	bin := writeScript(t, `{ echo "$@"; pwd; env; } > `+out)
	dir := t.TempDir()
	work := t.TempDir()
	d.wdDealer = wdStub{dir: work}
	if err := d.nullRun(context.Background(), bin, dir, "", []string{"TestA", "TestB"}); err != nil {
		t.Fatalf("nullRun: %v", err)
	}
	raw, err := os.ReadFile(out) //nolint:gosec // G304: test code reading its script's output
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	if want := "-test.count=1 -test.timeout 3s -test.failfast -test.paniconexit0 -test.run ^(TestA|TestB)$ -test.cpu 2"; lines[0] != want {
		t.Errorf("args = %q, want %q", lines[0], want)
	}
	if lines[1] != dir {
		t.Errorf("dir = %s, want %s", lines[1], dir)
	}
	gotmp := ""
	for _, l := range lines[2:] {
		if strings.HasPrefix(l, "GREMLINS_MUTANT=") || strings.HasPrefix(l, "GREMLINS_REACHED=") {
			t.Errorf("the null run's environment has %s", l)
		}
		if v, ok := strings.CutPrefix(l, "GOTMPDIR="); ok {
			gotmp = v
		}
	}
	if gotmp != work {
		t.Errorf("GOTMPDIR = %q, want the work directory %s, as a mutant's run", gotmp, work)
	}
}

// wdStub is a workdir.Dealer over one directory.
type wdStub struct{ dir string }

func (w wdStub) Get(string) (string, error) { return w.dir, nil }
func (wdStub) Clean()                       {}
func (w wdStub) WorkDir() string            { return w.dir }

// TestSchemaFallbacksAreCountedAndLogged checks that a mutant the schema
// executor hands to the legacy path is counted and named in the log.
//
// It is not parallel: it captures the global log.
func TestSchemaFallbacksAreCountedAndLogged(t *testing.T) {
	testCases := map[string]struct {
		build       *schemata.Build
		integration bool
		wantReason  string
	}{
		"nil_build":   {wantReason: "no schema build"},
		"integration": {build: &schemata.Build{}, integration: true, wantReason: "integration mode"},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			errOut := &bytes.Buffer{}
			log.Init(&bytes.Buffer{}, errOut)
			defer log.Reset()
			d := MutantExecutorDealer{wdDealer: wdStub{t.TempDir()}, testSelection: testSelection{integrationMode: tc.integration}, schemaCounts: &schemaCounts{}}
			// A skipped mutant: the legacy executor only reports it.
			m := &lineMutant{pkg: "m/a", line: 4, status: mutator.Skipped}
			outCh := make(chan mutator.Mutator, 1)
			var wg sync.WaitGroup
			wg.Add(1)
			d.NewSchemaExecutor(m, 1, tc.build, outCh, &wg).Start(workerpool.NewWorker(1, "w"))
			wg.Wait()
			if got := <-outCh; got != m {
				t.Fatalf("executor sent %v", got)
			}
			if n := d.schemaCounts.fallbacks.Load(); n != 1 {
				t.Errorf("fallbacks = %d, want 1", n)
			}
			if n := d.schemaCounts.judged.Load(); n != 0 {
				t.Errorf("judged = %d, want 0", n)
			}
			if !strings.Contains(errOut.String(), "f.go:4") || !strings.Contains(errOut.String(), tc.wantReason) {
				t.Errorf("log %q does not name the mutant and %q", errOut.String(), tc.wantReason)
			}
		})
	}
}

// TestNettedByReason groups netted mutants by the first line of their reason,
// most frequent first.
func TestNettedByReason(t *testing.T) {
	t.Parallel()
	netted := []schemata.NetEntry{
		{Reason: "build failed\nlong output"}, {Reason: "unsupported"},
		{Reason: "build failed\nother output"}, {Reason: "a reason"}, {Reason: "unsupported"}, {Reason: "build failed"},
	}
	want := []string{"3: build failed", "2: unsupported", "1: a reason"}
	if got := nettedByReason(netted); !slices.Equal(got, want) {
		t.Errorf("nettedByReason = %q, want %q", got, want)
	}
}

// TestExecuteSchemataStopsCollectingWhenCancelled cancels a --schemata run
// while mutant discovery is still producing, and requires executeSchemata to
// return promptly, as the legacy loop stops taking mutants once the run is
// cancelled, rather than wait for the stream to end.
func TestExecuteSchemataStopsCollectingWhenCancelled(t *testing.T) {
	t.Parallel()
	d := &MutantExecutorDealer{execContext: exec.CommandContext, testExecutionTime: time.Second, wdDealer: wdStub{dir: t.TempDir()}}
	stream := make(chan mutator.Mutator)
	mu := &Engine{mutantStream: stream, jDealer: d}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Discovery yields one mutant, then never ends.
	go func() { stream <- &lineMutant{pkg: "p", line: 1, status: mutator.Runnable} }()
	time.AfterFunc(100*time.Millisecond, cancel)

	done := make(chan report.Results, 1)
	go func() { done <- mu.executeSchemata(ctx) }()
	select {
	case res := <-done:
		if len(res.Mutants) != 0 {
			t.Errorf("a cancelled run reported %d mutants it never ran", len(res.Mutants))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("executeSchemata still collecting mutants 2s after the run was cancelled")
	}
}
