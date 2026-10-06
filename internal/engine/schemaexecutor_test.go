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
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/token"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/engine/workdir"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

const (
	calcPkg   = "schemaexec/calc"
	usePkg    = "schemaexec/use"
	notestPkg = "schemaexec/notest"
	gorunPkg  = "schemaexec/gorun"
)

// schemaMutators are the mutators the schemata engine rewrites: all of them.
var schemaMutators = mutator.Types

// fixtureMutant is one mutant of the fixture: its schema ID and position.
type fixtureMutant struct {
	id  int
	pos token.Position
}

// schemaFixture is the testdata/schemaexec module built into schema test
// binaries, with its mutants keyed "Func/MUTATOR".
type schemaFixture struct {
	mod   gomodule.GoModule
	build schemata.Build
	muts  map[string]fixtureMutant
}

// buildSchemaFixture discovers the mutants of the calc package, rewrites it
// and builds the test binaries of calc, use and gorun from the schema copy;
// notest, which has no test files, is built too and has none.
func buildSchemaFixture(t *testing.T) schemaFixture {
	t.Helper()
	modRoot, err := filepath.Abs("testdata/schemaexec")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedSyntax,
		Dir: modRoot,
	}
	pkgs, err := packages.Load(cfg, "./calc")
	if err != nil || len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		t.Fatalf("load calc: %v %v", err, pkgs)
	}
	pkg := pkgs[0]

	muts := map[string]fixtureMutant{}
	var sites []schemata.Site
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				tn, ok := engine.NewTokenNode(n)
				if !ok {
					return true
				}
				mts, _ := engine.MutantTypesFor(tn)
				site := schemata.Site{Node: n, Tok: tn.Tok()}
				for _, mt := range mts {
					if !slices.Contains(schemaMutators, mt) {
						continue
					}
					id := len(muts) + 1
					muts[fn.Name.Name+"/"+mt.String()] = fixtureMutant{id: id, pos: pkg.Fset.Position(tn.TokPos)}
					site.Muts = append(site.Muts, schemata.Mutant{ID: id, Type: mt})
				}
				if len(site.Muts) > 0 {
					sites = append(sites, site)
				}

				return true
			})
		}
	}

	files, _, dropped := schemata.RewritePackage(context.Background(), pkg, sites, "")
	if len(dropped) > 0 {
		t.Fatalf("sites dropped: %v", dropped)
	}
	b, errs := schemata.BuildAll(context.Background(), modRoot, t.TempDir(), "",
		map[string]map[string][]byte{calcPkg: files}, []string{calcPkg, usePkg, notestPkg, gorunPkg}, 5*time.Minute)
	for p, err := range errs {
		if err != nil {
			t.Fatalf("build %s: %v", p, err)
		}
	}
	if !b.NoTests[notestPkg] {
		t.Fatalf("notest not recorded as having no tests: %v", b.NoTests)
	}

	return schemaFixture{
		mod:   gomodule.GoModule{Name: "schemaexec", Root: modRoot, CallingDir: "."},
		build: b,
		muts:  muts,
	}
}

// newSchemaDealer returns a dealer over fresh worker copies of the fixture
// whose test timeout is one second, with extra configuration set.
func newSchemaDealer(t *testing.T, fx schemaFixture, set map[string]any, opts ...engine.ExecutorDealerOption) (*engine.MutantExecutorDealer, workdir.Dealer) {
	t.Helper()
	wdd := workdir.NewCachedDealer(t.TempDir(), fx.mod.Root)
	t.Cleanup(wdd.Clean)
	cfg := map[string]any{configuration.UnleashTimeoutMaxKey: "1s"}
	maps.Copy(cfg, set)
	viperSet(cfg)
	d := engine.NewExecutorDealer(fx.mod, wdd, 0, opts...)
	viperReset()

	return d, wdd
}

// runSchemaMutant executes the fixture mutant key on worker w with build b.
func runSchemaMutant(t *testing.T, d *engine.MutantExecutorDealer, fx schemaFixture, key string, b *schemata.Build, w *workerpool.Worker) *mutantStub {
	t.Helper()
	fm, ok := fx.muts[key]
	if !ok {
		t.Fatalf("no fixture mutant %s in %v", key, slices.Sorted(maps.Keys(fx.muts)))
	}
	stub := &mutantStub{pkg: calcPkg, position: fm.pos, status: mutator.Runnable}
	outCh := make(chan mutator.Mutator, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	d.NewSchemaExecutor(stub, fm.id, b, outCh, &wg).Start(w)
	wg.Wait()
	if got := <-outCh; got != stub {
		t.Fatalf("executor sent %v, want the mutant it was given", got)
	}

	return stub
}

// cmdRecorder records every command an executor creates.
type cmdRecorder struct {
	mu   sync.Mutex
	cmds []*exec.Cmd
}

func (r *cmdRecorder) exec(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: test code running the executor's own command
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, cmd)

	return cmd
}

func (r *cmdRecorder) all() []*exec.Cmd {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.cmds)
}

func envValue(env []string, key string) (string, bool) {
	var val string
	var found bool
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			val, found = v, true // the last one wins, as in os/exec
		}
	}

	return val, found
}

// TestSchemaExecutor runs fixture mutants against prebuilt schema binaries.
func TestSchemaExecutor(t *testing.T) {
	t.Parallel()
	fx := buildSchemaFixture(t)
	t.Run("verdicts", func(t *testing.T) { testSchemaVerdicts(t, fx) })
	t.Run("invocation", func(t *testing.T) { testSchemaInvocation(t, fx) })
	t.Run("missing_binary_falls_back", func(t *testing.T) { testSchemaMissingBinaryFallsBack(t, fx) })
	t.Run("run_cancelled", func(t *testing.T) { testSchemaRunCancelled(t, fx) })
	t.Run("cancelled_mid_run", func(t *testing.T) { testSchemaCancelledMidRun(t, fx) })
	t.Run("cancel_bounds_output_drain", func(t *testing.T) { testSchemaCancelBoundsOutputDrain(t, fx) })
	t.Run("concurrent_workers_isolated", func(t *testing.T) { testSchemaConcurrentWorkersIsolated(t, fx) })
	t.Run("combine", func(t *testing.T) { testSchemaCombine(t, fx) })
	t.Run("shared_directory_detected", func(t *testing.T) { testSchemaSharedDirectoryIsDetected(t, fx) })
}

// substExec records every command and runs, in place of each binary scripts
// names, a shell script with the given body.
type substExec struct {
	cmdRecorder
	scripts map[string]string
}

func (s *substExec) exec(ctx context.Context, name string, args ...string) *exec.Cmd {
	if p, ok := s.scripts[name]; ok {
		name = p
	}

	return s.cmdRecorder.exec(ctx, name, args...)
}

// newSubstExec writes each body of bodies, keyed by the binary it stands in
// for, to a script.
func newSubstExec(t *testing.T, bodies map[string]string) *substExec {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	s := &substExec{scripts: map[string]string{}}
	dir := t.TempDir()
	for bin, body := range bodies {
		p := filepath.Join(dir, filepath.Base(bin)+".sh")
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil { //nolint:gosec // G306: an executable test script
			t.Fatal(err)
		}
		s.scripts[bin] = p
	}

	return s
}

// testSchemaCombine runs a mutant whose selection is calc then use, with one
// of the two binaries replaced by a script, and checks that every package
// runs and that the verdicts combine as legacy's one go test over both
// packages reads them.
func testSchemaCombine(t *testing.T, fx schemaFixture) {
	t.Parallel()
	calcBin, useBin := fx.build.Binaries[calcPkg], fx.build.Binaries[usePkg]
	const (
		fail      = "exit 1"
		pass      = "exit 0"
		reachPass = `: > "$GREMLINS_REACHED"; exit 0` //nolint:gosec // G101: a shell script, not a credential
		timeout   = "echo 'panic: test timed out after 1s'; exit 2"
		signalled = "kill -KILL $$"
	)
	testCases := map[string]struct {
		key     string
		scripts map[string]string
		want    mutator.Status
	}{
		// calc fails without reaching Scale's site; use's real suite kills it.
		"unreached_failure_then_kill": {key: "Scale/ARITHMETIC_BASE", scripts: map[string]string{calcBin: fail}, want: mutator.Killed},
		// calc's real suite kills Add's mutant; use then overruns its timeout.
		"kill_then_timeout": {key: "Add/ARITHMETIC_BASE", scripts: map[string]string{useBin: timeout}, want: mutator.RunTimedOut},
		// calc kills; use passes without reach. Both run.
		"kill_then_pass": {key: "Add/ARITHMETIC_BASE", want: mutator.Killed},
		// calc's real suite reaches Scale's site and passes; use passes
		// without reach.
		"reach_aggregation": {key: "Scale/ARITHMETIC_BASE", scripts: map[string]string{useBin: pass}, want: mutator.Lived},
		"reach_aggregation_last": {
			key: "Scale/ARITHMETIC_BASE", scripts: map[string]string{calcBin: pass, useBin: reachPass}, want: mutator.Lived,
		},
		// Legacy checks for a signalled test binary before the exit status,
		// so one package with no verdict outweighs another's kill.
		"kill_then_signalled": {key: "Add/ARITHMETIC_BASE", scripts: map[string]string{useBin: signalled}, want: mutator.Errored},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sub := newSubstExec(t, tc.scripts)
			d, _ := newSchemaDealer(t, fx, map[string]any{configuration.UnleashCrossPackageKey: true},
				engine.WithExecContext(sub.exec),
				engine.WithDependents(dependentsStub{calcPkg: {usePkg}}))
			got := runSchemaMutant(t, d, fx, tc.key, &fx.build, workerpool.NewWorker(1, "w"))
			if got.Status() != tc.want {
				t.Errorf("status = %s, want %s", got.Status(), tc.want)
			}
			if n := len(sub.all()); n != 2 {
				t.Errorf("%d package runs, want both packages run", n)
			}
		})
	}
}

// testSchemaVerdicts holds each verdict the fixture can reach to the one its
// mutant must get.
func testSchemaVerdicts(t *testing.T, fx schemaFixture) {
	t.Parallel()
	testCases := map[string]struct {
		key  string
		set  map[string]any
		opts []engine.ExecutorDealerOption
		want mutator.Status
	}{
		"killed":         {key: "Add/ARITHMETIC_BASE", want: mutator.Killed},
		"lived":          {key: "Scale/ARITHMETIC_BASE", want: mutator.Lived},
		"not_covered":    {key: "Unused/ARITHMETIC_BASE", want: mutator.NotCovered},
		"run_timed_out":  {key: "Pause/CONDITIONALS_BOUNDARY", want: mutator.RunTimedOut},
		"increment":      {key: "Inc/INCREMENT_DECREMENT", want: mutator.Killed},
		"negative":       {key: "Neg/INVERT_NEGATIVES", want: mutator.Killed},
		"exit_0_in_test": {key: "Quit/ARITHMETIC_BASE", want: mutator.Killed},
		"killed_in_dependent": {
			key:  "Scale/ARITHMETIC_BASE",
			set:  map[string]any{configuration.UnleashCrossPackageKey: true},
			opts: []engine.ExecutorDealerOption{engine.WithDependents(dependentsStub{calcPkg: {usePkg}})},
			want: mutator.Killed,
		},
		// A selected package with no test files runs nothing, as legacy's
		// "? pkg [no test files]": calc's verdict stands.
		"killed_with_no_test_files_dependent": {
			key:  "Add/ARITHMETIC_BASE",
			set:  map[string]any{configuration.UnleashCrossPackageKey: true},
			opts: []engine.ExecutorDealerOption{engine.WithDependents(dependentsStub{calcPkg: {notestPkg}})},
			want: mutator.Killed,
		},
		"lived_with_no_test_files_dependent": {
			key:  "Scale/ARITHMETIC_BASE",
			set:  map[string]any{configuration.UnleashCrossPackageKey: true},
			opts: []engine.ExecutorDealerOption{engine.WithDependents(dependentsStub{calcPkg: {notestPkg}})},
			want: mutator.Lived,
		},
		// gorun's test runs go run from the directory runtime.Caller names
		// in the binary: the build copy, which the worker's overlay must
		// cover too, or the program is built without the mutant.
		"killed_through_go_run_in_caller_dir": {
			key: "Unused/ARITHMETIC_BASE",
			set: map[string]any{
				configuration.UnleashCrossPackageKey: true,
				configuration.UnleashTimeoutMaxKey:   "2m",
			},
			opts: []engine.ExecutorDealerOption{engine.WithDependents(dependentsStub{calcPkg: {gorunPkg}})},
			want: mutator.Killed,
		},
		"selected_tests_do_not_reach": {
			key: "Scale/ARITHMETIC_BASE",
			opts: []engine.ExecutorDealerOption{engine.WithTestSelection(selectorStub{
				mapped: map[string]bool{calcPkg: true},
				tests:  []coverage.TestID{{Pkg: calcPkg, Name: "TestAdd"}},
			})},
			want: mutator.NotCovered,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, _ := newSchemaDealer(t, fx, tc.set, tc.opts...)
			got := runSchemaMutant(t, d, fx, tc.key, &fx.build, workerpool.NewWorker(1, "w"))
			if got.Status() != tc.want {
				t.Errorf("status = %s, want %s", got.Status(), tc.want)
			}
			if got.applyCalled {
				t.Error("the schema executor applied the mutation to the worker copy")
			}
		})
	}
}

// testSchemaInvocation checks the command the executor runs: flags, directory,
// environment, overlay, a stale reach file cleared, the worker copy untouched.
func testSchemaInvocation(t *testing.T, fx schemaFixture) {
	t.Parallel()
	rec := &cmdRecorder{}
	d, wdd := newSchemaDealer(t, fx, map[string]any{configuration.UnleashTestCPUKey: 2},
		engine.WithExecContext(rec.exec),
		engine.WithTestSelection(selectorStub{
			mapped: map[string]bool{calcPkg: true},
			tests:  []coverage.TestID{{Pkg: calcPkg, Name: "TestAdd"}, {Pkg: calcPkg, Name: "TestScale"}},
		}))
	w := workerpool.NewWorker(3, "w")
	root, err := wdd.Get("w-3")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "calc", "calc.go")) //nolint:gosec // G304: test code reading its worker copy
	if err != nil {
		t.Fatal(err)
	}
	fm := fx.muts["Unused/ARITHMETIC_BASE"]
	// The reach file sits outside every module copy, named for the worker
	// and the mutant, so no test walking its tree can see it.
	reach := filepath.Join(wdd.WorkDir(), "schemata-reached-w-3-"+strconv.Itoa(fm.id))
	if err := os.WriteFile(reach, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got := runSchemaMutant(t, d, fx, "Unused/ARITHMETIC_BASE", &fx.build, w)
	if got.Status() != mutator.NotCovered {
		t.Errorf("status with a stale reach file = %s, want NOT COVERED", got.Status())
	}
	runSchemaMutant(t, d, fx, "Add/ARITHMETIC_BASE", &fx.build, w)
	if _, err := os.Stat(reach); !os.IsNotExist(err) {
		t.Errorf("reach file %s left after the run: %v", reach, err)
	}
	addReach := filepath.Join(wdd.WorkDir(), "schemata-reached-w-3-"+strconv.Itoa(fx.muts["Add/ARITHMETIC_BASE"].id))
	if _, err := os.Stat(addReach); !os.IsNotExist(err) {
		t.Errorf("reach file %s of a reached run left after it: %v", addReach, err)
	}
	_ = filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err == nil && strings.Contains(filepath.Base(path), "reached") {
			t.Errorf("reach file %s in the worker copy", path)
		}

		return err
	})

	cmds := rec.all()
	if len(cmds) != 2 {
		t.Fatalf("%d commands, want 2", len(cmds))
	}
	cmd := cmds[0]
	wantArgs := []string{
		fx.build.Binaries[calcPkg], "-test.count=1", "-test.timeout", "1s", "-test.failfast",
		"-test.paniconexit0", "-test.run", "^(TestAdd|TestScale)$", "-test.cpu", "2",
	}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Errorf("args = %q, want %q", cmd.Args, wantArgs)
	}
	if want := filepath.Join(root, "calc"); cmd.Dir != want {
		t.Errorf("dir = %s, want %s", cmd.Dir, want)
	}
	if v, _ := envValue(cmd.Env, "GREMLINS_MUTANT"); v != strconv.Itoa(fm.id) {
		t.Errorf("GREMLINS_MUTANT = %q, want %d", v, fm.id)
	}
	if v, _ := envValue(cmd.Env, "GREMLINS_REACHED"); v != reach {
		t.Errorf("GREMLINS_REACHED = %q, want %s", v, reach)
	}
	// As the legacy executor's go test: a test running the go command
	// keeps its work directories in the run's work directory.
	if v, _ := envValue(cmd.Env, "GOTMPDIR"); v != wdd.WorkDir() {
		t.Errorf("GOTMPDIR = %q, want the work directory %s", v, wdd.WorkDir())
	}
	overlay := overlayPath(t, cmd.Env)
	if other := overlayPath(t, cmds[1].Env); other != overlay {
		t.Errorf("second mutant on the worker used overlay %s, want the worker's one %s", other, overlay)
	}
	raw, err := os.ReadFile(overlay) //nolint:gosec // G304: test code reading the executor's overlay
	if err != nil {
		t.Fatal(err)
	}
	var o struct{ Replace map[string]string }
	if err := json.Unmarshal(raw, &o); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, rel := range fx.build.Rewritten {
		want[filepath.Join(root, rel)] = filepath.Join(fx.build.Src, rel)
		want[filepath.Join(fx.build.Dir, rel)] = filepath.Join(fx.build.Src, rel)
	}
	if len(want) == 0 || !maps.Equal(o.Replace, want) {
		t.Errorf("overlay = %v, want %v", o.Replace, want)
	}
	after, err := os.ReadFile(filepath.Join(root, "calc", "calc.go")) //nolint:gosec // G304: test code reading its worker copy
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("the worker copy's source changed")
	}
}

// testSchemaMissingBinaryFallsBack checks that a selected package without a
// binary sends the mutant through the legacy executor.
func testSchemaMissingBinaryFallsBack(t *testing.T, fx schemaFixture) {
	t.Parallel()
	rec := &cmdRecorder{}
	d, _ := newSchemaDealer(t, fx, nil, engine.WithExecContext(rec.exec))
	partial := fx.build
	partial.Binaries = map[string]string{usePkg: fx.build.Binaries[usePkg]}

	got := runSchemaMutant(t, d, fx, "Add/ARITHMETIC_BASE", &partial, workerpool.NewWorker(1, "w"))

	if !got.applyCalled || !got.rollbackCalled {
		t.Error("the mutant did not go through the legacy executor")
	}
	cmds := rec.all()
	if len(cmds) != 1 || len(cmds[0].Args) < 2 || cmds[0].Args[0] != "go" || cmds[0].Args[1] != "test" {
		t.Errorf("commands %v, want one go test", cmds)
	}
	// The stub's Apply changes nothing, so go test runs the original suite.
	if got.Status() != mutator.Lived {
		t.Errorf("status = %s, want the legacy verdict LIVED", got.Status())
	}
}

// holdShutdownStatus sets --on-shutdown-status to value until the test ends.
// The cancellation tests use a value other than the default, which is also
// what a mutant that never ran gets, so that they show the setting is read.
// The configuration is process-wide: the lock viperSet takes serialises these
// tests with every other that sets it, and a leaf test must call this after
// newSchemaDealer, which takes the same lock.
func holdShutdownStatus(t *testing.T, value string) {
	t.Helper()
	viperSet(map[string]any{configuration.UnleashOnShutdownStatusKey: value})
	t.Cleanup(viperReset)
}

// testSchemaRunCancelled checks that a run cancelled before the mutant starts
// takes the shutdown status and starts nothing.
func testSchemaRunCancelled(t *testing.T, fx schemaFixture) {
	t.Parallel()
	rec := &cmdRecorder{}
	d, _ := newSchemaDealer(t, fx, nil, engine.WithExecContext(rec.exec))
	holdShutdownStatus(t, "timed-out")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.SetRunCtx(ctx)

	got := runSchemaMutant(t, d, fx, "Add/ARITHMETIC_BASE", &fx.build, workerpool.NewWorker(1, "w"))

	// Not the default: NOT COVERED is also what a mutant that never ran gets.
	if got.Status() != mutator.TimedOut {
		t.Errorf("status = %s, want the configured shutdown status TIMED OUT", got.Status())
	}
	if n := len(rec.all()); n != 0 {
		t.Errorf("%d commands started after the run was cancelled", n)
	}
}

// A run cancelled after the site was reached takes the shutdown status,
// not the LIVED the reach would otherwise make it.
func testSchemaCancelledMidRun(t *testing.T, fx schemaFixture) {
	t.Parallel()
	d, _ := newSchemaDealer(t, fx, nil)
	holdShutdownStatus(t, "lived")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.SetRunCtx(ctx)
	time.AfterFunc(300*time.Millisecond, cancel)

	got := runSchemaMutant(t, d, fx, "Pause/CONDITIONALS_BOUNDARY", &fx.build, workerpool.NewWorker(1, "w"))

	// Neither the default nor what the run's own timeout (RUNNER TIMED OUT)
	// or its backstop (TIMED OUT) would give.
	if got.Status() != mutator.Lived {
		t.Errorf("status = %s, want the configured shutdown status LIVED", got.Status())
	}
}

// testSchemaCancelBoundsOutputDrain cancels the run while the test binary
// sleeps, once with a child that escaped the binary's process group still
// holding its output open, and requires the mutant's run to end within
// outputDrainGrace (two seconds) of the cancel.
func testSchemaCancelBoundsOutputDrain(t *testing.T, fx schemaFixture) {
	t.Parallel()
	testCases := map[string]struct{ body string }{
		"sleeping_binary":          {body: "sleep 60"},
		"child_holding_the_output": {body: "setsid sleep 60 & sleep 60"},
		"binary_ignoring_sigterm":  {body: "trap '' TERM; sleep 60"},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sub := newSubstExec(t, map[string]string{fx.build.Binaries[calcPkg]: tc.body})
			d, _ := newSchemaDealer(t, fx, nil, engine.WithExecContext(sub.exec))
			holdShutdownStatus(t, "timed-out")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d.SetRunCtx(ctx)
			const cancelAfter = 300 * time.Millisecond
			time.AfterFunc(cancelAfter, cancel)
			start := time.Now()

			got := runSchemaMutant(t, d, fx, "Add/ARITHMETIC_BASE", &fx.build, workerpool.NewWorker(1, "w"))

			if el := time.Since(start); el > cancelAfter+2*time.Second+500*time.Millisecond {
				t.Errorf("the run ended %s after the cancel, want within outputDrainGrace", el-cancelAfter)
			}
			if got.Status() != mutator.TimedOut {
				t.Errorf("status = %s, want the configured shutdown status TIMED OUT", got.Status())
			}
		})
	}
}

// testSchemaConcurrentWorkersIsolated runs eight mutants on four workers and
// expects the statuses a serial run gives.
func testSchemaConcurrentWorkersIsolated(t *testing.T, fx schemaFixture) {
	t.Parallel()
	keys := []string{
		"Add/ARITHMETIC_BASE", "Scale/ARITHMETIC_BASE", "Unused/ARITHMETIC_BASE", "Unused/INVERT_NEGATIVES",
		"Pause/CONDITIONALS_BOUNDARY", "Pause/CONDITIONALS_NEGATION", "Inc/INCREMENT_DECREMENT", "Neg/INVERT_NEGATIVES",
	}
	want := map[string]mutator.Status{
		"Add/ARITHMETIC_BASE": mutator.Killed, "Scale/ARITHMETIC_BASE": mutator.Lived,
		"Unused/ARITHMETIC_BASE": mutator.NotCovered, "Unused/INVERT_NEGATIVES": mutator.NotCovered,
		"Pause/CONDITIONALS_BOUNDARY": mutator.RunTimedOut, "Pause/CONDITIONALS_NEGATION": mutator.RunTimedOut,
		"Inc/INCREMENT_DECREMENT": mutator.Killed, "Neg/INVERT_NEGATIVES": mutator.Killed,
	}

	serialDealer, _ := newSchemaDealer(t, fx, nil)
	serial := map[string]mutator.Status{}
	w := workerpool.NewWorker(1, "serial")
	for _, k := range keys {
		serial[k] = runSchemaMutant(t, serialDealer, fx, k, &fx.build, w).Status()
	}
	if !maps.Equal(serial, want) {
		t.Fatalf("serial statuses %v, want %v", serial, want)
	}

	d, _ := newSchemaDealer(t, fx, nil)
	if concurrent := runConcurrently(t, d, fx, keys, 4); !maps.Equal(concurrent, serial) {
		t.Errorf("concurrent statuses %v, want the serial ones %v", concurrent, serial)
	}
}

// runConcurrently runs the fixture mutants keys at once on a pool of workers
// workers and returns each one's status.
func runConcurrently(t *testing.T, d *engine.MutantExecutorDealer, fx schemaFixture, keys []string, workers int) map[string]mutator.Status {
	t.Helper()
	viperSet(map[string]any{configuration.UnleashWorkersKey: workers})
	pool := workerpool.Initialize("schema")
	viperReset()
	outCh := make(chan mutator.Mutator, len(keys))
	var wg sync.WaitGroup
	stubs := map[mutator.Mutator]string{}
	pool.Start()
	for _, k := range keys {
		fm := fx.muts[k]
		stub := &mutantStub{pkg: calcPkg, position: fm.pos, status: mutator.Runnable}
		stubs[stub] = k
		wg.Add(1)
		pool.AppendExecutor(d.NewSchemaExecutor(stub, fm.id, &fx.build, outCh, &wg))
	}
	wg.Wait()
	pool.Stop()
	close(outCh)
	got := map[string]mutator.Status{}
	for m := range outCh {
		got[stubs[m]] = m.Status()
	}

	return got
}

// sharedDealer deals one working directory to every worker, as a broken
// isolation would.
type sharedDealer struct {
	workdir.Dealer
	dir string
}

func (s sharedDealer) Get(string) (string, error) { return s.dir, nil }

// testSchemaSharedDirectoryIsDetected holds the isolation test to its claim:
// with two workers dealt one directory, the fixture's TestAlone probe sees
// the other's marker, and a mutant whose verdict is otherwise NOT COVERED
// fails instead.
func testSchemaSharedDirectoryIsDetected(t *testing.T, fx schemaFixture) {
	t.Parallel()
	wdd := workdir.NewCachedDealer(t.TempDir(), fx.mod.Root)
	t.Cleanup(wdd.Clean)
	viperSet(map[string]any{configuration.UnleashTimeoutMaxKey: "1s"})
	dir, err := wdd.Get("shared") // made before the workers ask, which would each make a copy
	if err != nil {
		t.Fatal(err)
	}
	d := engine.NewExecutorDealer(fx.mod, sharedDealer{wdd, dir}, 0)
	viperReset()
	keys := []string{"Unused/ARITHMETIC_BASE", "Unused/INVERT_NEGATIVES"}

	got := runConcurrently(t, d, fx, keys, 2)

	if len(got) != len(keys) {
		t.Fatalf("statuses %v for %v", got, keys)
	}
	if got[keys[0]] == mutator.NotCovered && got[keys[1]] == mutator.NotCovered {
		t.Errorf("two workers sharing a directory both got NOT COVERED: the probe sees no sharing")
	}
}

// overlayPath returns the -overlay file named in env's GOFLAGS.
func overlayPath(t *testing.T, env []string) string {
	t.Helper()
	flags, _ := envValue(env, "GOFLAGS")
	for _, f := range strings.Fields(flags) {
		if p, ok := strings.CutPrefix(f, "-overlay="); ok {
			return p
		}
	}
	t.Fatalf("no -overlay in GOFLAGS %q", flags)

	return ""
}

// TestSchemaErroredRunIsExplained runs a mutant whose package runs are
// scripts standing in for the test binaries of calc and use, and checks that
// a mutant booked ERRORED logs, for each run that made it so, the package,
// its exit code, its failing tests and panics, and a bounded head of its
// output, and that a mutant with any other verdict logs none of it.
//
// It is not parallel: the logger is a process-wide singleton.
func TestSchemaErroredRunIsExplained(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	modRoot, err := filepath.Abs("testdata/schemaexec")
	if err != nil {
		t.Fatal(err)
	}
	// No binary is built: every run is a script, so the build only names
	// the binaries the scripts stand in for.
	calcBin, useBin := filepath.Join(t.TempDir(), "calc.test"), filepath.Join(t.TempDir(), "use.test")
	fx := schemaFixture{
		mod:   gomodule.GoModule{Name: "schemaexec", Root: modRoot, CallingDir: "."},
		build: schemata.Build{Binaries: map[string]string{calcPkg: calcBin, usePkg: useBin}},
		muts:  map[string]fixtureMutant{"m": {id: 1, pos: token.Position{Filename: "calc.go", Line: 7, Column: 9}}},
	}
	const (
		failing = `echo '=== RUN   TestAlone'; echo '--- PASS: TestAlone (0.00s)'; ` +
			`echo '=== RUN   TestAdd'; echo '    calc_test.go:30: Add(2, 3) != 5'; echo '--- FAIL: TestAdd (0.00s)'; ` +
			`echo 'stderr line' >&2; echo FAIL`
		reach = `: > "$GREMLINS_REACHED"; `
		// 200 numbered lines, then a failure past both bounds.
		long = `i=1; while [ $i -le 200 ]; do printf 'line %03d\n' $i; i=$((i+1)); done; ` +
			`echo '    --- FAIL: TestLate/sub (0.00s)'; echo '--- FAIL: TestLate (0.00s)'`
		// 40 lines of 300 bytes: past 8 KiB within 60 lines, in line 28.
		wide = `i=1; while [ $i -le 40 ]; do printf 'wide %03d %0290d\n' $i 0; i=$((i+1)); done`
	)
	testCases := map[string]struct {
		scripts map[string]string
		want    mutator.Status
		logged  []string // substrings the log must hold
		absent  []string // substrings it must not
	}{
		"failed_without_reach": {
			scripts: map[string]string{calcBin: failing + "; exit 1", useBin: "exit 0"},
			want:    mutator.Errored,
			logged: []string{
				"calc.go:7:9", calcPkg + " exited 1",
				"    failing: --- FAIL: TestAdd (0.00s)\n",
				"        === RUN   TestAlone\n", "        calc_test.go:30: Add(2, 3) != 5\n", "        stderr line\n",
			},
			// use passed: it made nothing ERRORED.
			absent: []string{usePkg + " exited", "failing: --- PASS"},
		},
		"panicked_without_reach": {
			scripts: map[string]string{calcBin: `echo 'panic: runtime error: index out of range [3]'; echo 'goroutine 7 [running]:'; exit 2`, useBin: "exit 0"},
			want:    mutator.Errored,
			logged:  []string{calcPkg + " exited 2", "    failing: panic: runtime error: index out of range [3]\n", "        goroutine 7 [running]:\n"},
		},
		"signalled": {
			scripts: map[string]string{calcBin: `echo 'panic: boom'; kill -KILL $$`, useBin: "exit 0"},
			want:    mutator.Errored,
			logged:  []string{calcPkg + " exited -1", "    failing: panic: boom\n"},
		},
		"failed_without_reach_in_each": {
			scripts: map[string]string{calcBin: failing + "; exit 1", useBin: `echo '--- FAIL: TestDouble (0.00s)'; exit 1`},
			want:    mutator.Errored,
			logged:  []string{calcPkg + " exited 1", usePkg + " exited 1", "    failing: --- FAIL: TestDouble (0.00s)\n"},
		},
		"head_bounded_by_lines": {
			scripts: map[string]string{calcBin: long + "; exit 1", useBin: "exit 0"},
			want:    mutator.Errored,
			logged: []string{
				"        line 001\n", "        line 060\n", "        [output truncated]\n",
				// Past the head's bounds, the failures are still named.
				"    failing: --- FAIL: TestLate/sub (0.00s)\n", "    failing: --- FAIL: TestLate (0.00s)\n",
			},
			absent: []string{"line 061", "line 200"},
		},
		"head_bounded_by_bytes": {
			scripts: map[string]string{calcBin: wide + "; exit 1", useBin: "exit 0"},
			want:    mutator.Errored,
			logged:  []string{"        wide 001 ", "        [output truncated]\n"},
			absent:  []string{"wide 029 ", "wide 040 "},
		},
		"killed": {
			scripts: map[string]string{calcBin: reach + failing + "; exit 1", useBin: "exit 0"},
			want:    mutator.Killed,
			absent:  []string{"failing:", "TestAdd", "=== RUN", "exited"},
		},
		// calc fails without reach, use kills: the mutant is KILLED, and
		// calc's failure, which is logged as ever, is not explained.
		"unreached_failure_then_kill": {
			scripts: map[string]string{calcBin: failing + "; exit 1", useBin: reach + "exit 1"},
			want:    mutator.Killed,
			logged:  []string{"failed without reaching the mutant (exit 1)"},
			absent:  []string{"failing:", "=== RUN", "exited"},
		},
		"lived": {
			scripts: map[string]string{calcBin: reach + failing + "; exit 0", useBin: "exit 0"},
			want:    mutator.Lived,
			absent:  []string{"failing:", "=== RUN", "exited"},
		},
		"not_covered": {
			scripts: map[string]string{calcBin: failing + "; exit 0", useBin: "exit 0"},
			want:    mutator.NotCovered,
			absent:  []string{"failing:", "=== RUN", "exited"},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			sub := newSubstExec(t, tc.scripts)
			d, _ := newSchemaDealer(t, fx, map[string]any{configuration.UnleashCrossPackageKey: true},
				engine.WithExecContext(sub.exec),
				engine.WithDependents(dependentsStub{calcPkg: {usePkg}}))
			var buf bytes.Buffer
			log.Reset()
			log.Init(&buf, &buf)
			defer log.Reset()

			got := runSchemaMutant(t, d, fx, "m", &fx.build, workerpool.NewWorker(1, "w"))

			if got.Status() != tc.want {
				t.Fatalf("status = %s, want %s", got.Status(), tc.want)
			}
			out := buf.String()
			for _, s := range tc.logged {
				if !strings.Contains(out, s) {
					t.Errorf("log lacks %q:\n%s", s, out)
				}
			}
			for _, s := range tc.absent {
				if strings.Contains(out, s) {
					t.Errorf("log holds %q:\n%s", s, out)
				}
			}
		})
	}
}
