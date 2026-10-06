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

package engine

import (
	"context"
	"errors"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/engine/workdir"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/procgroup"
)

// DefaultTimeoutCoefficient is the default multiplier for the timeout length
// of each test run.
const DefaultTimeoutCoefficient = 5

// unsetDuration is the zero a duration setting reads as when it is absent or
// rejected. Every reader of it applies its own fallback rather than using it as
// a bound: a zero bound would kill every mutant instantly and report the
// package as perfectly tested.
const unsetDuration = time.Duration(0)

// DefaultCompileAllowance is how long a mutant is allowed to COMPILE, over and
// above the timeout that bounds its test run.
//
// The two bounds measure unrelated things. A package's compile time is a
// property of its size and its dependency graph; a mutant's run time is a
// property of the test that adjudicates it. Charging both to one number makes
// the slow-compiling packages time out mutants that would have reached a
// verdict in milliseconds. Two minutes is roughly four times the slowest
// single-package compile observed on the projects this fork is used against
// (~31s), which is headroom enough to be invisible in practice while still
// bounding a compile that has genuinely hung.
const DefaultCompileAllowance = 2 * time.Minute

// testTimeoutMarker is the first thing the testing package prints when the
// -timeout it was given expires. `go test` collapses a test timeout, a build
// failure and an ordinary test failure into its own exit status 1, so the exit
// code alone cannot tell them apart and the output has to.
const testTimeoutMarker = "panic: test timed out after "

// outputDrainGrace bounds how long Wait may block draining the child's output
// pipes after the child itself has exited. Without it a grandchild that
// inherited the pipe and outlived its parent would hold the executor open
// indefinitely, which is the one hang the two time bounds below cannot catch.
const outputDrainGrace = 2 * time.Second

// buildFailureMarker and setupFailureMarker are what `go test` appends to the
// FAIL line for a package whose test binary could not be built, or whose test
// setup failed before any test ran. Both mean the mutant was never adjudicated.
const (
	buildFailureMarker = " [build failed]"
	setupFailureMarker = " [setup failed]"
)

// ExecutorDealer is the initializer for new workerpool.Executor.
type ExecutorDealer interface {
	NewExecutor(mut mutator.Mutator, outCh chan<- mutator.Mutator, wg *sync.WaitGroup) workerpool.Executor
}

// MutantExecutorDealer is a ExecutorDealer for the initialisation of a mutantExecutor.
//
// By default, it sets uses exec.Command to perform the tests on the source
// code. This can be overridden, for example in tests.
//
// The apply and rollback functions are wrappers around the TokenMutator apply and
// rollback. These can be overridden with nop functions in tests. Not an
// ideal setup. In the future we can think of a better way to handle this.
type MutantExecutorDealer struct {
	wdDealer    workdir.Dealer
	execContext execContext
	// testSelection is held once, whole: the dealer's schema preparation and
	// every executor it makes decide what a mutant runs from this one value,
	// so that what Prepare builds is what the executors run, with no second
	// list of its fields to drift apart. It carries testMap, dependents,
	// crossPackage and integrationMode.
	testSelection
	// The context is held rather than passed because the dealer outlives every
	// call that would carry one: it is built once and each worker asks it for a
	// mutant's bounds later. SetRunCtx is how the engine's run context reaches
	// them, and it is what lets a cancelled run be told from a mutant that ran
	// long.
	runCtx            context.Context //nolint:containedctx
	mod               gomodule.GoModule
	buildTags         string
	testExecutionTime time.Duration
	compileAllowance  time.Duration
	// schemataBuildTimeout bounds the whole schema build; zero derives the
	// bound from compileAllowance and the number of packages built.
	schemataBuildTimeout time.Duration
	dryRun               bool
	testCPU              int
	// overlays is shared by every copy of the dealer, so each worker's
	// overlay file for a schema build is written once per run, not once per
	// mutant.
	overlays *overlayCache
	// schemaCounts is shared by every copy of the dealer, like overlays.
	schemaCounts *schemaCounts
}

// schemaCounts counts how the mutants given to schema executors were judged.
type schemaCounts struct {
	// judged were run against the schema binaries.
	judged atomic.Int64
	// fallbacks were handed to the legacy path.
	fallbacks atomic.Int64
}

// SetRunCtx wires the engine's root context into the dealer so that each
// mutantExecutor it produces can observe cancellation (e.g. SIGTERM from
// a CI runner) and mark in-flight mutants accordingly instead of falling
// through to the default mutator.Lived branch.
func (m *MutantExecutorDealer) SetRunCtx(ctx context.Context) {
	m.runCtx = ctx
}

// ExecutorDealerOption is the defining option for the initialisation of a ExecutorDealer.
type ExecutorDealerOption func(j MutantExecutorDealer) MutantExecutorDealer

// WithExecContext overrides the default exec.Command with a custom executor.
func WithExecContext(c execContext) ExecutorDealerOption {
	return func(m MutantExecutorDealer) MutantExecutorDealer {
		m.execContext = c

		return m
	}
}

// TestSelector answers, for a position in the code, which tests execute it, and
// whether a package's tests are known at all.
//
// It is the executor's whole view of the test map: an executor asks what covers
// this mutant and whether it may trust the answer for this package, and nothing
// else about how the map was built.
type TestSelector interface {
	// Mapped reports whether the tests of a package are known in full.
	Mapped(pkg string) bool

	// TestsFor returns the tests that execute the given position.
	TestsFor(pos token.Position) []coverage.TestID
}

// DependentFinder answers which packages depend on a package, through their own
// code or through their tests.
//
// It is what --cross-package needs and all it needs: which packages a mutation
// could break is a question about imports, answerable statically, with no
// coverage and no test runs.
type DependentFinder interface {
	Dependents(pkg string) []string
}

// WithDependents turns on cross-package testing: a mutant is tested against the
// packages that depend on the one it is in, not only that one.
func WithDependents(d DependentFinder) ExecutorDealerOption {
	return func(m MutantExecutorDealer) MutantExecutorDealer {
		m.dependents = d

		return m
	}
}

// WithTestSelection turns on test selection: instead of the whole suite of the
// mutated package, each mutant runs the tests the selector says execute its
// line, wherever those tests live.
func WithTestSelection(sel TestSelector) ExecutorDealerOption {
	return func(m MutantExecutorDealer) MutantExecutorDealer {
		m.testMap = sel

		return m
	}
}

// NewExecutorDealer initialises a MutantExecutorDealer.
func NewExecutorDealer(mod gomodule.GoModule, wdd workdir.Dealer, elapsed time.Duration, opts ...ExecutorDealerOption) *MutantExecutorDealer {
	buildTags := configuration.Get[string](configuration.UnleashTagsKey)
	dryRun := configuration.Get[bool](configuration.UnleashDryRunKey)
	integrationMode := configuration.Get[bool](configuration.UnleashIntegrationMode)
	crossPackage := configuration.Get[bool](configuration.UnleashCrossPackageKey)
	testCPU := configuration.Get[int](configuration.UnleashTestCPUKey)
	tCoefficient := configuration.Get[int](configuration.UnleashTimeoutCoefficientKey)

	coefficient := DefaultTimeoutCoefficient
	if tCoefficient != 0 {
		coefficient = tCoefficient
	}

	if testCPU != 0 && integrationMode {
		testCPU /= testCPU
	}

	// Use a minimum of 1 second for timeout calculation to prevent
	// unreasonably short timeouts when coverage runs very quickly
	baseTime := elapsed
	if baseTime < time.Second {
		baseTime = time.Second
	}

	jd := MutantExecutorDealer{
		mod:                  mod,
		testSelection:        testSelection{crossPackage: crossPackage, integrationMode: integrationMode},
		wdDealer:             wdd,
		buildTags:            buildTags,
		dryRun:               dryRun,
		testCPU:              testCPU,
		testExecutionTime:    cappedExecutionTime(baseTime * time.Duration(coefficient)),
		compileAllowance:     compileAllowance(),
		schemataBuildTimeout: schemataBuildTimeout(),
		execContext:          exec.CommandContext,
		overlays:             newOverlayCache(),
		schemaCounts:         &schemaCounts{},
	}

	for _, opt := range opts {
		jd = opt(jd)
	}

	return &jd
}

// cappedExecutionTime clamps the coefficient-derived bound on a mutant's test
// RUN to the absolute ceiling set by --timeout-max, if one is set.
//
// The coefficient-derived timeout is proportional to how long the package's own
// tests take, which is unrelated to how much damage a runaway mutant can do in
// that time. Mutating a loop-advance statement (i++ -> i--) inside a scanner
// loop whose body appends per iteration produces a mutant that never terminates
// and allocates until the machine is out of memory. On a CI runner the OOM
// killer then reaps the runner agent itself, so the job dies with no verdict at
// all — not a LIVED mutant, not a TIMED OUT one, just a dead runner. The
// timeout is the only defence against that, and scaling it by test duration
// hands the longest leash to the slowest-testing packages, which is backwards.
//
// A ceiling bounds the exposure independently of the baseline: past it the
// mutant is killed and recorded as TIMED OUT, which is the honest verdict for a
// mutant that did not terminate.
//
// Zero (the default) means no ceiling, so runs that do not set the flag behave
// exactly as before. A malformed or non-positive value is reported and ignored
// rather than silently treated as "no cap", because a safety ceiling that
// quietly does nothing is worse than one that was never configured.
func cappedExecutionTime(d time.Duration) time.Duration {
	ceiling, ok := positiveDuration(configuration.UnleashTimeoutMaxKey, "running without a timeout ceiling")
	if !ok {
		return d
	}

	if d > ceiling {
		return ceiling
	}

	return d
}

// compileAllowance returns the time a mutant is allowed to compile, on top of
// the bound on its test run. It is configured with --compile-allowance and
// falls back to DefaultCompileAllowance.
func compileAllowance() time.Duration {
	d, ok := positiveDuration(configuration.UnleashCompileAllowanceKey, "using the default compile allowance")
	if !ok {
		return DefaultCompileAllowance
	}

	return d
}

// schemataBuildTimeout returns the bound on the whole schema build of a run,
// configured with --schemata-build-timeout; zero, when it is unset or
// invalid, derives the bound in schemata.BuildTimeout.
func schemataBuildTimeout() time.Duration {
	d, _ := positiveDuration(configuration.UnleashSchemataBuildTimeoutKey, "deriving the schemata build timeout")

	return d
}

// positiveDuration reads a Go duration from a string configuration key. It
// reports ok=false when the key is unset, unparseable, or non-positive, so the
// caller applies its own fallback.
//
// A malformed value is reported and ignored rather than silently treated as
// zero: a zero bound would kill every mutant instantly and report the package
// as perfectly tested. onInvalid names what the caller will do instead, so the
// message says what actually happens rather than only what did not.
func positiveDuration(key, onInvalid string) (time.Duration, bool) {
	raw := configuration.Get[string](key)
	if raw == "" {
		return unsetDuration, false
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Errorf("invalid %s %q: %v; %s\n", key, raw, err, onInvalid)

		return unsetDuration, false
	}
	if d <= unsetDuration {
		log.Errorf("invalid %s %q: must be positive; %s\n", key, raw, onInvalid)

		return unsetDuration, false
	}

	return d, true
}

// NewExecutor returns a new workerpool.Executor for the given mutator.Mutator.
// It gets an output channel of mutator.Mutator and a sync.WaitGroup. The channel
// will stream the results of the executor, and the wait group will be done when the
// executor is complete.
func (m MutantExecutorDealer) NewExecutor(mut mutator.Mutator, outCh chan<- mutator.Mutator, wg *sync.WaitGroup) workerpool.Executor {
	runCtx := m.runCtx
	if runCtx == nil {
		runCtx = context.Background()
	}
	mj := mutantExecutor{
		mutant:            mut,
		outCh:             outCh,
		wg:                wg,
		wdDealer:          m.wdDealer,
		module:            m.mod,
		testSelection:     m.testSelection,
		dryRun:            m.dryRun,
		buildTags:         m.buildTags,
		execContext:       m.execContext,
		testCPU:           m.testCPU,
		testExecutionTime: m.testExecutionTime,
		compileAllowance:  m.compileAllowance,
		runCtx:            runCtx,
	}

	return &mj
}

type execContext = func(ctx context.Context, name string, args ...string) *exec.Cmd

type mutantExecutor struct {
	mutant mutator.Mutator
	// testSelection is the dealer's, see MutantExecutorDealer.
	testSelection
	wdDealer    workdir.Dealer
	outCh       chan<- mutator.Mutator
	wg          *sync.WaitGroup
	execContext execContext
	// Carried for the same reason the dealer carries it: the executor is
	// constructed before the run it belongs to reaches runTests, and telling a
	// cancelled run from a long one needs the run's own context, not the
	// per-mutant one derived from it.
	runCtx            context.Context //nolint:containedctx
	module            gomodule.GoModule
	buildTags         string
	testExecutionTime time.Duration
	compileAllowance  time.Duration
	dryRun            bool
	testCPU           int
}

// Start is the implementation of the workerpool.Executor definition and is the
// method responsible for performing the actual mutation testing.
// The executor runs on its mutator.Mutator.
// If it is RUNNABLE, and it is not in dry-run mode, it will apply the mutation,
// run the tests and mark the TokenMutator as either KILLED or LIVED depending
// on the result. If the tests pass, it means the TokenMutator survived, so it
// will be LIVED, if the tests fail, the TokenMutator will be KILLED.
// A mutant is bounded twice: `go test -timeout` bounds the test RUN, and a
// context deadline bounds compilation and the run together as a backstop. See
// runTests for why the two are separate.
func (m *mutantExecutor) Start(w *workerpool.Worker) {
	defer m.wg.Done()
	workerName := fmt.Sprintf("%s-%d", w.Name, w.ID)
	rootDir, err := m.wdDealer.Get(workerName)
	if err != nil {
		log.Errorf("failed to get working directory for worker %s: %v", workerName, err)
		panic(fmt.Sprintf("failed to get working directory for worker %s: %v", workerName, err))
	}

	workingDir := filepath.Join(rootDir, m.module.CallingDir)
	m.mutant.SetWorkdir(workingDir)

	if m.mutant.Status() == mutator.NotCovered || m.mutant.Status() == mutator.Skipped || m.dryRun {
		m.outCh <- m.mutant

		return
	}

	if err := m.mutant.Apply(); err != nil {
		log.Errorf("failed to apply mutation at %s - %s\n\t%v", m.mutant.Position(), m.mutant.Status(), err)

		return
	}

	m.mutant.SetStatus(m.runTests(rootDir, m.mutant.Pkg()))

	if err := m.mutant.Rollback(); err != nil {
		// What should we do now?
		log.Errorf("failed to restore mutation at %s - %s\n\t%v", m.mutant.Position(), m.mutant.Status(), err)
	}

	m.outCh <- m.mutant
}

// testRun is the single `go test` invocation a mutant is judged by: the
// packages to run, and which tests within them. An empty tests slice means
// whole suites, which is what runs when there is no test selection.
//
// It is one invocation and not one per package on purpose. `go test` takes many
// packages with one -run, builds them together, and most of what a mutant costs
// is that build: measured on four packages of Rulewright's backend with a -run
// matching nothing at all, four invocations took 4147ms against 2115ms for one.
// The tests were never the expense; the invocations were.
type testRun struct {
	pkgs  []string
	tests []string
}

// runTests runs the mutated package's tests and classifies the outcome.
//
// Two bounds, because there are two things to bound and they scale
// independently:
//
//   - `go test -timeout` (m.testExecutionTime) bounds the test RUN. The Go
//     toolchain starts that clock when the test binary starts, so compiling the
//     mutated package does not consume it. This is the leash that matters: it is
//     what stops a mutant that never terminates.
//   - The context deadline (run bound + compile allowance) bounds compilation
//     and the run together. No -timeout can bound a compile that hangs, because
//     no test binary exists yet to enforce it, so the context stays as the
//     backstop. It is also rooted in the engine's runCtx, so an external
//     cancellation (e.g. SIGTERM from a CI runner) propagates into the child and
//     we can tell "the runner is shutting us down" from "this mutant ran long".
//
// Classification cannot go on the exit status alone. `go test` reports a test
// timeout, a build failure and an ordinary test failure all as exit status 1 —
// only the test BINARY exits 2, and what we spawn is `go`. Taking that 1 at face
// value would record a timed-out mutant as KILLED, crediting a detection that
// never happened, and a mutant that does not compile as KILLED too. So the
// child's output is scanned for the markers that tell the three apart.
//
// The two bounds also produce two DIFFERENT timeout verdicts, and the ordering
// below is what keeps them apart. mutator.RunTimedOut is the only branch here
// backed by a positive observation: the test binary printed, in its own output,
// that the suite it was running overran the leash `go test -timeout` gave it.
// Every other branch infers from an absence — a deadline that expired, a context
// that was cancelled, an exit status that says nothing about which phase
// produced it. mutator.TimedOut is that absence, and it must stay separate
// precisely because a compile that hung reaches it identically to a run that
// did.
func (m *mutantExecutor) runTests(rootDir, pkg string) mutator.Status {
	ctx, cancel := context.WithTimeout(m.runCtx, m.testExecutionTime+m.compileAllowance)
	defer cancel()

	return m.runTestCommand(ctx, rootDir, m.selectTests(pkg))
}

// selectTests decides what to run for the mutant and records the tests it
// narrowed to on the mutant. See testSelection.forMutant.
func (m *mutantExecutor) selectTests(pkg string) testRun {
	sel, names := m.forMutant(pkg, m.mutant.Position())
	if names != nil {
		m.mutant.SetTestsRun(names)
	}

	return sel
}

// testSelection is what decides, for a mutant, which packages and tests run.
// It is one value shared by the executor, which runs the selection, and by the
// schemata preparation, which must build and null-check exactly the packages
// the executor will later run.
type testSelection struct {
	testMap         TestSelector
	dependents      DependentFinder
	crossPackage    bool
	integrationMode bool
}

// forMutant decides what to run for a mutant of pkg at pos, along two
// independent axes. It also returns the IDs of the tests it narrowed to, nil
// when it runs whole suites.
//
// Which PACKAGES: the mutated one, and with --cross-package the packages that
// depend on it, because those are the ones a mutation can break. That is the
// gap package scoping leaves — go-gremlins/gremlins#224, a LIVED verdict that
// was correct for what it measured — and answering it needs nothing but the
// import graph.
//
// Which TESTS within them: all of them, or with --test-selection only the ones
// coverage says execute the mutated line. Narrowing inside a package it was
// already going to build and pay fixtures for cannot cost more than not
// narrowing; measured on Rulewright's backend it is 24% of the test executions.
//
// The two compose: neither flag is today's behaviour, both together is the
// narrowest run that still sees the callers.
//
// Every path that cannot answer confidently widens rather than narrows, to the
// whole suites of the packages it settled on: never wrong, only slow.
func (s testSelection) forMutant(pkg string, pos token.Position) (testRun, []string) {
	pkgs := []string{pkg}
	if s.crossPackage {
		pkgs = append(pkgs, s.dependents.Dependents(pkg)...)
	}
	wholeSuites := testRun{pkgs: pkgs}
	if s.testMap == nil || s.integrationMode {
		return wholeSuites, nil
	}
	// A package the map could not see whole might hold the very test that kills
	// this mutant, and skipping it would turn a killed mutant into a LIVED one.
	if !s.testMap.Mapped(pkg) {
		return wholeSuites, nil
	}
	tests := within(s.testMap.TestsFor(pos), pkgs)
	if len(tests) == 0 {
		// An uncovered mutant never reaches here, so an empty answer means the
		// map is incomplete — coverage is not always deterministic — rather than
		// that no test exercises the line.
		return wholeSuites, nil
	}

	var sel testRun
	seenPkg := make(map[string]struct{}, len(tests))
	seenName := make(map[string]struct{}, len(tests))
	names := make([]string, 0, len(tests))
	for _, id := range tests {
		if _, ok := seenPkg[id.Pkg]; !ok {
			seenPkg[id.Pkg] = struct{}{}
			sel.pkgs = append(sel.pkgs, id.Pkg)
		}
		// The -run pattern applies to every package listed, so a name shared by
		// two packages runs in both even where only one covers the line. That
		// runs more tests than strictly needed, never fewer, so it can only cost
		// time — and it saves an invocation per package, which costs more.
		if _, ok := seenName[id.Name]; !ok {
			seenName[id.Name] = struct{}{}
			sel.tests = append(sel.tests, id.Name)
		}
		names = append(names, id.String())
	}

	return sel, names
}

// within keeps the tests that live in one of the packages being run. Tests
// outside them are not this run's business: without --cross-package that means
// the mutated package alone, and with it the packages that depend on it.
func within(tests []coverage.TestID, pkgs []string) []coverage.TestID {
	in := make(map[string]struct{}, len(pkgs))
	for _, p := range pkgs {
		in[p] = struct{}{}
	}
	kept := tests[:0:0]
	for _, id := range tests {
		if _, ok := in[id.Pkg]; ok {
			kept = append(kept, id)
		}
	}

	return kept
}

func (m *mutantExecutor) runTestCommand(ctx context.Context, rootDir string, sel testRun) mutator.Status {
	cmd := m.execContext(ctx, "go", m.getTestArgs(sel)...)
	cmd.Dir = m.mutant.Workdir()
	if m.integrationMode {
		cmd.Dir = rootDir
	}
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("GOTMPDIR=%s", m.wdDealer.WorkDir()))

	// The output is scanned, never retained: a runaway mutant can print without
	// bound, and buffering that would trade one resource exhaustion for another.
	scanner := newOutputScanner()
	cmd.Stdout = scanner
	cmd.Stderr = scanner
	cmd.WaitDelay = outputDrainGrace

	// Set up process group for killing entire process tree
	procgroup.Setup(cmd)

	err := run(ctx, cmd)

	// The run-phase watchdog is read before either deadline, because it is
	// evidence and they are the lack of it. Two things follow from that order.
	//
	// A goroutine dump from a runaway mutant can be large, and the backstop can
	// expire while it is still draining; reading the backstop first would relabel
	// the one mutant that DID report a verdict as one that did not. And a mutant
	// that printed the marker before the runner's SIGTERM arrived was adjudicated
	// by its own test binary before the shutdown, so the shutdown status would
	// discard a verdict already reached.
	//
	// `err != nil` guards the marker: a package whose tests all pass exits 0, and
	// a test that merely PRINTS this string while passing must not be read as one
	// that overran. A binary the watchdog actually fired on always exits non-zero,
	// and a deadline-killed child returns the context's error, so no real timeout
	// is lost to the guard.
	if err != nil && scanner.sawTestTimeout() {
		return mutator.RunTimedOut
	}
	// The backstop bounds compile AND run together, so when it is what fired we
	// do not know which phase spent it. Unadjudicated: it stays in the efficacy
	// denominator and credits nobody.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return mutator.TimedOut
	}
	// If the parent runCtx was cancelled (Ctrl-C or runner SIGTERM) the
	// `go test` child was killed before reaching a verdict. Reporting
	// these as LIVED misrepresents the data — they were never tested.
	// The configured shutdown status (default NotCovered) is the truthful
	// outcome.
	if m.runCtx.Err() != nil {
		return shutdownStatus()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// A mutant that does not compile never runs a test binary, so it cannot
		// print the timeout panic and cannot have been claimed by the branch
		// above; the two markers only compete for a multi-package run where one
		// package failed to build and another timed out. RUN TIMED OUT is the
		// safe verdict there, because NOT VIABLE would drop the mutant out of
		// the denominator altogether.
		if scanner.sawBuildFailure() {
			return mutator.NotViable
		}
		// And a test binary the kernel destroyed is a third thing go folds into
		// exit 1. It is checked after the two toolchain markers because those
		// are statements go makes about its own work, while this one is an
		// inference from what the child printed as it died.
		if scanner.sawSignalledTestBinary() {
			log.Errorf("test run for %s reached no verdict: the test binary was terminated by a signal\n",
				m.mutant.Position())

			return mutator.Errored
		}

		status := getTestFailedStatus(exitErr.ExitCode())
		if status == mutator.Errored {
			// The error carries the signal name ("signal: killed"), which is the only
			// thing that tells an OOM kill apart from a crash, and neither of them is
			// visible in the status alone.
			log.Errorf("test run for %s reached no verdict: %v\n", m.mutant.Position(), exitErr)
		}

		return status
	}

	return mutator.Lived
}

// outputScanner is an io.Writer that watches a stream for fixed markers and
// then throws it away. It retains only enough bytes to recognise a marker split
// across two writes, so its memory use is constant however much the child
// prints.
type outputScanner struct {
	mu        sync.Mutex
	tail      string
	seen      map[string]bool
	carry     int
	signalled bool
}

// scannedMarkers are the substrings outputScanner looks for.
var scannedMarkers = []string{testTimeoutMarker, buildFailureMarker, setupFailureMarker}

// goSignalledTestBinary matches the two lines go writes when the test binary it
// spawned was terminated by a signal: the reason, then the FAIL summary for the
// package. The pairing is what makes the match evidence — a test that prints
// "signal: something" of its own accord has not been killed. There is no exit
// code for this: go uses 1, the same code an ordinary test failure uses.
var goSignalledTestBinary = regexp.MustCompile(`(?m)^signal: [^\n]+\nFAIL\b`)

// signalledBinaryCarry is how much tail the scanner must keep for the pattern
// above to be recognised when it straddles two writes. It is a bound on the
// pattern's span, not on a marker's length, because the middle is variable:
// "signal: killed" and "signal: segmentation fault (core dumped)" are both it.
const signalledBinaryCarry = 128

func newOutputScanner() *outputScanner {
	carry := signalledBinaryCarry
	for _, marker := range scannedMarkers {
		if len(marker) > carry {
			carry = len(marker)
		}
	}

	return &outputScanner{seen: make(map[string]bool, len(scannedMarkers)), carry: carry - 1}
}

func (s *outputScanner) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	window := s.tail + string(p)
	for _, marker := range scannedMarkers {
		if strings.Contains(window, marker) {
			s.seen[marker] = true
		}
	}
	if goSignalledTestBinary.MatchString(window) {
		s.signalled = true
	}

	// Retain just enough of the tail that a marker straddling two writes is
	// still recognised on the next one.
	if len(window) > s.carry {
		window = window[len(window)-s.carry:]
	}
	s.tail = window

	return len(p), nil
}

func (s *outputScanner) saw(marker string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.seen[marker]
}

// sawTestTimeout reports whether the test binary aborted on its own -timeout.
func (s *outputScanner) sawTestTimeout() bool {
	return s.saw(testTimeoutMarker)
}

// sawBuildFailure reports whether the package failed to build, or failed to set
// up before any test ran. Either way the mutant was never adjudicated.
func (s *outputScanner) sawBuildFailure() bool {
	return s.saw(buildFailureMarker) || s.saw(setupFailureMarker)
}

// sawSignalledTestBinary reports whether go said the test binary it ran was
// terminated by a signal — an OOM kill under a memory cap, most often. That run
// reached no verdict, so it is neither a mutant the tests killed nor one they
// failed to kill.
func (s *outputScanner) sawSignalledTestBinary() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.signalled
}

// shutdownStatus returns the status to record for mutants that were
// in-flight when the run was cancelled. The choice is driven by the
// `unleash.on-shutdown-status` config key (CLI flag
// --on-shutdown-status). Unrecognised values fall back to NotCovered,
// the truthful default ("we never finished running this mutant").
func shutdownStatus() mutator.Status {
	v := configuration.Get[string](configuration.UnleashOnShutdownStatusKey)
	if s, ok := mutator.ParseShutdownStatus(v); ok {
		return s
	}

	return mutator.NotCovered
}

// vetOff disables the `go vet` subset that `go test` runs before it builds
// anything.
//
// That subset is not the compiler, and the difference matters here. Its `bools`
// analyzer reports a conjunction of equalities against distinct operands —
// `name == a && name == b` — as "suspect and", and `go test` turns that into
// `FAIL pkg [build failed]` even though `go build` accepts the file without a
// word. That shape is precisely what INVERT_LOGICAL produces from the very
// common `name == a || name == b`, so with vet on, a whole class of mutants
// could never reach a test binary.
//
// Every one of them is legal Go and a real change of behaviour: the mutated
// predicate is unsatisfiable where the original was not, and a suite that
// exercises either operand kills it. Refusing to GENERATE them would be the
// wrong fix — that removes from the corpus mutants the suite ought to be asked
// about, which is the one direction a mutation tool must never shrink in. What
// is wrong is asking a style analyzer whether a mutant may be adjudicated at
// all, so the analyzer is taken out of the loop instead. It also removes a vet
// pass from every mutant's build, on a tool whose cost is dominated by builds.
const vetOff = "-vet=off"

func (m *mutantExecutor) getTestArgs(sel testRun) []string {
	args := []string{"test", vetOff}
	if m.buildTags != "" {
		args = append(args, buildTagsFlag, m.buildTags)
	}
	// -timeout is the run-only leash, and it is deliberately the tighter of the
	// two bounds: the Go toolchain starts it when the test binary starts, so a
	// slow compile cannot eat it. The context deadline in runTests sits above it
	// and covers compilation as well.
	args = append(args, "-timeout", m.testExecutionTime.String())
	args = append(args, "-failfast")

	if m.testCPU != 0 {
		args = append(args, "-cpu", fmt.Sprintf("%d", m.testCPU))
	}

	// An empty selection is the whole suite: no -run at all, exactly the command
	// that runs without selection.
	if len(sel.tests) > 0 {
		args = append(args, "-run", "^("+strings.Join(sel.tests, "|")+")$")
	}

	if m.integrationMode {
		return append(args, "./...")
	}

	return append(args, sel.pkgs...)
}

func run(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	// Ensure cleanup happens regardless of how we exit
	defer func() {
		if cmd.Process != nil {
			// Always kill the process group to catch any child processes
			// This is safe even if the process already exited
			_ = procgroup.Kill(cmd)
			// Release OS resources
			_ = cmd.Process.Release()
		}
	}()

	// Monitor context cancellation in parallel with process execution
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Context cancelled/timed out - kill the entire process group
		// Do this BEFORE the parent process exits to catch children
		_ = procgroup.Kill(cmd)
		// Wait for the process to actually exit
		<-done

		return ctx.Err()
	case err := <-done:
		// Process completed normally
		return err
	}
}

// getTestFailedStatus maps a non-zero exit status to a verdict. It is reached
// only after runTests has ruled out the statuses that `go test` also folds into
// exit 1 — a test timeout, a build failure, and a test binary killed by a
// signal — so a 1 here really is a failing test, which is a detection. Status 2
// is what a test BINARY returns when it panics; `go test` does not surface it,
// but a direct binary invocation would, and it means the mutant was never
// adjudicated.
//
// A negative exit code is os/exec reporting a process that did not exit on its
// own but was terminated by a signal — an OOM kill, most often, since a mutant
// can turn ordinary code into an allocation bomb and a memory cap then kills the
// test process. That run produced no verdict, so it is neither a mutant the
// tests failed to kill nor one they killed.
func getTestFailedStatus(exitCode int) mutator.Status {
	switch {
	case exitCode == 1:
		return mutator.Killed
	case exitCode == 2:
		return mutator.NotViable
	case exitCode < 0:
		return mutator.Errored
	default:
		return mutator.Lived
	}
}
