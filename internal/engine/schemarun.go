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
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/procgroup"
	"github.com/go-gremlins/gremlins/internal/report"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

// nullRunOutputLimit bounds how much of a null run's output is kept for its
// error. The first lines say what failed; a runaway suite can print without
// bound.
const nullRunOutputLimit = 64 << 10

// nullCopyID is the work-directory dealer's identifier of the module copy the
// null runs execute in.
const nullCopyID = "schemata-null"

// executeSchemata is executeTests under --schemata. It collects every mutant
// first, because the schema build needs all of them: the runnable ones go to
// Prepare, the ones it places run against the schema binaries, and every other
// mutant goes to the executor NewExecutor makes, exactly as without schemata.
func (mu *Engine) executeSchemata(ctx context.Context) report.Results {
	all, ok := collect(ctx, mu.mutantStream)
	if !ok {
		// As the legacy loop, which stops taking mutants once the run is
		// cancelled: none has run, so there is nothing to report.
		return report.Results{Schemata: &report.SchemataSummary{}}
	}
	var runnable []mutator.Mutator
	for _, m := range all {
		if m.Status() == mutator.Runnable {
			runnable = append(runnable, m)
		}
	}

	summary := &report.SchemataSummary{}
	d, ok := mu.jDealer.(*MutantExecutorDealer)
	switch {
	case !ok:
		log.Errorf("schemata: the executor dealer cannot run schema binaries; every mutant runs through go test\n")
		summary.PerMutant = len(runnable)
	case d.dryRun:
		// No mutant runs, so there is nothing to build.
		summary.NotUsed = "dry run"
	case len(runnable) == 0:
		// Nothing to build for.
	case d.integrationMode:
		log.Infoln("schemata has no effect in integration mode: every mutant runs the whole module")
		summary.PerMutant = len(runnable)
		summary.NotUsed = "integration mode"
	default:
		return mu.executePlaced(ctx, d, all, runnable)
	}

	res := mu.execute(ctx, feed(all), mu.jDealer.NewExecutor)
	res.Schemata = summary

	return res
}

// collect takes every mutant of stream until it closes, and reports false,
// without waiting for the rest, if ctx ends first: discovery does not watch
// the context, and it can take long.
func collect(ctx context.Context, stream <-chan mutator.Mutator) ([]mutator.Mutator, bool) {
	var all []mutator.Mutator
	for {
		if ctx.Err() != nil {
			return nil, false
		}
		select {
		case <-ctx.Done():
			return nil, false
		case m, ok := <-stream:
			if !ok {
				return all, true
			}
			all = append(all, m)
		}
	}
}

// executePlaced prepares the schema build of runnable and runs all: the
// placed mutants against the build, every other one as without schemata.
func (mu *Engine) executePlaced(ctx context.Context, d *MutantExecutorDealer, all, runnable []mutator.Mutator) report.Results {
	if d.schemaCounts == nil {
		d.schemaCounts = &schemaCounts{}
	}
	// GoModule.Root is relative whenever the target was (`gremlins unleash
	// ./pkg`), while go/packages and the go command report absolute paths,
	// and the test binaries run in package directories, where a relative
	// path means something else. Every schemata path is built from the
	// absolute root. The legacy executors read only the module's name and
	// calling directory, so the dealer's copy changes nothing for them.
	mod, err := absModule(mu.module)
	if err != nil {
		log.Errorf("schemata: %v; every mutant runs through go test\n", err)
		res := mu.execute(ctx, feed(all), d.NewExecutor)
		res.Schemata = &report.SchemataSummary{PerMutant: len(runnable)}

		return res
	}
	d.mod = mod
	plan, err := mu.prepareSchemata(ctx, d, mod, runnable)
	if err != nil {
		// Only the run's context ends Prepare early: the run is over.
		log.Errorf("schemata: %v\n", err)

		return report.Results{Schemata: &report.SchemataSummary{}}
	}

	ids := make(map[mutator.Mutator]int, len(plan.Placed))
	for _, p := range plan.Placed {
		ids[p.Mutator] = p.ID
	}
	build := &plan.Build
	judged, fell := d.schemaCounts.judged.Load(), d.schemaCounts.fallbacks.Load()
	res := mu.execute(ctx, feed(all), func(mut mutator.Mutator, outCh chan<- mutator.Mutator, wg *sync.WaitGroup) workerpool.Executor {
		if id, ok := ids[mut]; ok {
			return d.NewSchemaExecutor(mut, id, build, outCh, wg)
		}

		return d.NewExecutor(mut, outCh, wg)
	})
	if dir := os.Getenv(SchemataKeepEnv); dir != "" {
		if err := keepSchemata(dir, plan); err != nil {
			log.Errorf("schemata: cannot keep the build in %s: %v\n", dir, err)
		} else {
			log.Infof("schemata: build kept in %s\n", dir)
		}
	}
	// Counted, not taken from the plan: a placed mutant counts as placed only
	// once its executor has judged it against the binaries.
	res.Schemata = &report.SchemataSummary{
		Placed:    int(d.schemaCounts.judged.Load() - judged),
		PerMutant: len(plan.Netted) + int(d.schemaCounts.fallbacks.Load()-fell),
	}

	return res
}

// SchemataKeepEnv names the environment variable that keeps a --schemata
// run's build: set to a directory, the run copies into it, before its work
// directory is removed, every test binary (bin/), every rewritten or added
// source file (src/, relative to the module root) and index.json, which maps
// each placed mutant's id to its position, type and package, and each package
// to its kept binary. A kept binary runs a mutant as the run did: in its
// package's directory, with GREMLINS_MUTANT=<id>, and GREMLINS_REACHED=<file>
// to learn whether the mutant's site ran.
const SchemataKeepEnv = "GREMLINS_SCHEMATA_KEEP"

// keptIndex is the index.json of a kept schema build.
type keptIndex struct {
	Binaries map[string]string `json:"binaries"`
	Source   string            `json:"source"`
	Mutants  []keptMutant      `json:"mutants"`
}

type keptMutant struct {
	Position string `json:"position"`
	Type     string `json:"type"`
	Package  string `json:"package"`
	ID       int    `json:"id"`
}

// keepSchemata copies plan's build into dir, as SchemataKeepEnv describes.
func keepSchemata(dir string, plan schemata.Plan) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	b := plan.Build
	idx := keptIndex{Binaries: map[string]string{}, Source: filepath.Join(dir, "src")}
	for _, pkg := range slices.Sorted(maps.Keys(b.Binaries)) {
		dst := filepath.Join(dir, "bin", filepath.Base(b.Binaries[pkg]))
		if err := copyFile(b.Binaries[pkg], dst, 0o700); err != nil {
			return err
		}
		idx.Binaries[pkg] = dst
	}
	for _, rel := range b.Rewritten {
		if err := copyFile(filepath.Join(b.Src, rel), filepath.Join(idx.Source, rel), 0o600); err != nil {
			return err
		}
	}
	for _, p := range plan.Placed {
		idx.Mutants = append(idx.Mutants, keptMutant{
			ID: p.ID, Position: p.Mutator.Position().String(), Type: p.Mutator.Type().String(), Package: p.Mutator.Pkg(),
		})
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "index.json"), data, 0o600) //nolint:gosec // G703: dir is the user's own GREMLINS_SCHEMATA_KEEP
}

// copyFile copies src to dst with mode perm, making dst's directory.
func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src) //nolint:gosec // G304: a file of the run's own build
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil { //nolint:gosec // G703: under the user's own GREMLINS_SCHEMATA_KEEP
		return err
	}

	return os.WriteFile(dst, data, perm) //nolint:gosec // G703: under the user's own GREMLINS_SCHEMATA_KEEP
}

// prepareSchemata runs Prepare over runnable in a fresh directory of the
// engine's work directory, which the caller of the engine removes, with the
// test packages and null runs the executors' own test selection gives.
func (mu *Engine) prepareSchemata(ctx context.Context, d *MutantExecutorDealer, mod gomodule.GoModule, runnable []mutator.Mutator) (schemata.Plan, error) {
	targets := d.schemaTargets(runnable)
	workDir, err := os.MkdirTemp(d.wdDealer.WorkDir(), "schemata-*")
	if err == nil {
		workDir, err = filepath.Abs(workDir)
	}
	if err != nil {
		reason := fmt.Sprintf("schemata: no build directory: %v", err)
		log.Errorf("%s\n", reason)
		var plan schemata.Plan
		for _, m := range runnable {
			plan.Netted = append(plan.Netted, schemata.NetEntry{Mutator: m, Reason: reason})
		}

		return plan, nil
	}

	plan, err := mu.prepare(ctx, mod, workDir, d.buildTags, runnable, targets.testPkgs, d.compileAllowance, d.schemataBuildTimeout,
		d.schemaNullRun(ctx, workDir, targets))
	if err != nil {
		return plan, err
	}
	for _, line := range nettedByReason(plan.Netted) {
		log.Infof("schemata: netted %s\n", line)
	}

	return plan, nil
}

// absModule is mod with an absolute root.
func absModule(mod gomodule.GoModule) (gomodule.GoModule, error) {
	root, err := filepath.Abs(mod.Root)
	if err != nil {
		return mod, fmt.Errorf("module root %s: %w", mod.Root, err)
	}
	mod.Root = root

	return mod, nil
}

// feed returns a closed channel holding muts, in order.
func feed(muts []mutator.Mutator) <-chan mutator.Mutator {
	ch := make(chan mutator.Mutator, len(muts))
	for _, m := range muts {
		ch <- m
	}
	close(ch)

	return ch
}

// nettedByReason counts the netted mutants by the first line of their reason,
// as "<count>: <reason>" lines, most frequent first.
func nettedByReason(netted []schemata.NetEntry) []string {
	counts := map[string]int{}
	for _, n := range netted {
		reason, _, _ := strings.Cut(n.Reason, "\n")
		counts[reason]++
	}
	reasons := slices.Collect(maps.Keys(counts))
	slices.SortFunc(reasons, func(a, b string) int {
		if c := cmp.Compare(counts[b], counts[a]); c != 0 {
			return c
		}

		return strings.Compare(a, b)
	})
	lines := make([]string, 0, len(reasons))
	for _, r := range reasons {
		lines = append(lines, fmt.Sprintf("%d: %s", counts[r], r))
	}

	return lines
}

// schemaTargets is what the executors of a run's mutants will run, gathered
// before any of them does, so that Prepare builds and null-checks exactly it.
type schemaTargets struct {
	// pkgs maps a mutated package to the packages its mutants select.
	pkgs map[string][]string
	// tests maps a selected package to the tests the mutants that select it
	// run there; nil when one of them runs its whole suite.
	tests map[string][]string
	// dirs maps a selected package's directory under the module root to it.
	dirs map[string]string
}

// schemaTargets gathers what the executors of runnable will select, from the
// same testSelection.forMutant their selectTests calls.
func (m MutantExecutorDealer) schemaTargets(runnable []mutator.Mutator) schemaTargets {
	sel := m.testSelection
	t := schemaTargets{pkgs: map[string][]string{}, tests: map[string][]string{}, dirs: map[string]string{}}
	whole := map[string]bool{}
	for _, mut := range runnable {
		run, _ := sel.forMutant(mut.Pkg(), mut.Position())
		t.pkgs[mut.Pkg()] = append(t.pkgs[mut.Pkg()], run.pkgs...)
		for _, p := range run.pkgs {
			if len(run.tests) == 0 {
				whole[p] = true
			}
			t.tests[p] = append(t.tests[p], run.tests...)
		}
	}
	for pkg, pkgs := range t.pkgs {
		slices.Sort(pkgs)
		t.pkgs[pkg] = slices.Compact(pkgs)
	}
	for pkg, tests := range t.tests {
		if whole[pkg] {
			t.tests[pkg] = nil
		} else {
			slices.Sort(tests)
			t.tests[pkg] = slices.Compact(tests)
		}
		if dir, ok := m.mod.PackageDir(m.mod.Root, pkg); ok {
			t.dirs[filepath.Clean(dir)] = pkg
		}
	}

	return t
}

// testPkgs is the packages the mutants of pkg select: Prepare's testPkgs.
func (t schemaTargets) testPkgs(pkg string) []string {
	return t.pkgs[pkg]
}

// testsAt is the tests to run in the package at dir, nil for the whole suite.
// A package no mutant selects runs its whole suite.
func (t schemaTargets) testsAt(dir string) []string {
	pkg, ok := t.dirs[filepath.Clean(dir)]
	if !ok {
		return nil
	}

	return t.tests[pkg]
}

// schemaNullRun is Prepare's null run: each binary runs the tests targets
// selects in its package's directory of a copy of the module made for the
// null runs, never in the user's own tree, where a test writing into its
// directory would leave files behind. It gets the overlay that points the
// copy's rewritten files at the schema build's, written into tmpDir, as a
// mutant's run gets one for its worker copy. pkgDir is the package's
// directory under the module root.
func (m MutantExecutorDealer) schemaNullRun(ctx context.Context, tmpDir string, targets schemaTargets) schemata.NullRunFunc {
	overlays := m.overlays
	if overlays == nil {
		overlays = newOverlayCache()
	}

	return func(b *schemata.Build, bin, pkgDir string) error {
		dir, root, err := m.nullRunDir(pkgDir)
		if err != nil {
			return fmt.Errorf("%s in %s: %w", filepath.Base(bin), pkgDir, err)
		}
		overlay, err := overlays.get(b, root, tmpDir)
		if err != nil {
			return fmt.Errorf("%s in %s: %w", filepath.Base(bin), dir, err)
		}

		return m.nullRun(ctx, bin, dir, overlay, targets.testsAt(pkgDir))
	}
}

// nullRunDir maps pkgDir, a directory under the module root, to the same
// directory in the null runs' copy of the module, and returns it with the
// copy's absolute root.
func (m MutantExecutorDealer) nullRunDir(pkgDir string) (string, string, error) {
	rel, err := filepath.Rel(m.mod.Root, pkgDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("%s is not under the module root %s", pkgDir, m.mod.Root)
	}
	root, err := m.wdDealer.Get(nullCopyID)
	if err == nil {
		// Absolute: the binary runs in a package directory, and the
		// overlay's keys must name the files the go command sees.
		root, err = filepath.Abs(root)
	}
	if err != nil {
		return "", "", fmt.Errorf("null run copy: %w", err)
	}

	return filepath.Join(root, rel), root, nil
}

// nullRun runs the test binary bin in dir with no mutant switched on, the
// tests named in tests (all of them when empty), and the flags, bounds,
// GOTMPDIR and overlay a mutant's run gets; an empty overlay adds none. A run that does
// not pass is an error whose first line says how it ended and, when the
// output shows it, which test failed.
func (m MutantExecutorDealer) nullRun(ctx context.Context, bin, dir, overlay string, tests []string) error {
	bound := m.testExecutionTime + schemaBackstopGrace
	ctx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	cmd := m.execContext(ctx, bin, testBinaryArgs(m.testExecutionTime, m.testCPU, tests)...)
	cmd.Dir = dir
	cmd.Env = append(withoutMutant(os.Environ()), goTmpDir(m.wdDealer))
	if overlay != "" {
		cmd.Env = append(cmd.Env, overlayGOFLAGS(overlay))
	}
	out := &headWriter{limit: nullRunOutputLimit}
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.WaitDelay = outputDrainGrace
	procgroup.Setup(cmd)

	err := run(ctx, cmd)
	switch {
	case err == nil:
		return nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%s in %s: no verdict within %s", filepath.Base(bin), dir, bound)
	case cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 0:
		// exec.ErrWaitDelay: the binary passed, a child held its output.
		return nil
	}
	text := out.String()

	return fmt.Errorf("%s in %s: %w%s\n%s", filepath.Base(bin), dir, err, firstFailure(text), text)
}

// withoutMutant is env without the variables that switch a mutant on.
func withoutMutant(env []string) []string {
	return slices.DeleteFunc(env, func(kv string) bool {
		return strings.HasPrefix(kv, "GREMLINS_MUTANT=") || strings.HasPrefix(kv, "GREMLINS_REACHED=")
	})
}

// firstFailure is ": <line>" for the first line of out that names a failing
// test or a panic, or "" when there is none.
func firstFailure(out string) string {
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if isFailureLine(l) {
			return ": " + l
		}
	}

	return ""
}

// isFailureLine reports whether l, trimmed of space, names a failing test or
// a panic.
func isFailureLine(l string) bool {
	return strings.HasPrefix(l, "--- FAIL") || strings.HasPrefix(l, "panic:")
}

// headWriter keeps the first limit bytes written to it and drops the rest.
// os/exec writes Stdout and Stderr from one goroutine when they are the same
// comparable writer, so it needs no lock.
type headWriter struct {
	buf   []byte
	limit int
}

func (w *headWriter) Write(p []byte) (int, error) {
	if room := w.limit - len(w.buf); room > 0 {
		w.buf = append(w.buf, p[:min(room, len(p))]...)
	}

	return len(p), nil
}

func (w *headWriter) String() string {
	return string(w.buf)
}
