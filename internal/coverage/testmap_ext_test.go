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

package coverage_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
)

// The fixture is the shape that makes cross-package selection worth having: the
// clamp on vm.go:7 is executed only by a test in the root package, and the vm
// package's own test never reaches it.
const (
	clampedLine   = 7
	uncoveredLine = 20
	// vmOwnLine is executed by the vm package's own test, which is what a run
	// scoped to that package can still answer about.
	vmOwnLine = 5
)

// pkgDirsEnv carries the fixture's package directories to the helper process,
// which has to name them in the `go list` output it fakes. The directories have
// to exist: the builder runs each test binary in its package's directory, the
// way `go test` does.
const pkgDirsEnv = "GREMLINS_TEST_PKG_ROOT"

// buildIDsEnv lets a test decide what `go tool buildid` reports for each
// package, which is how it controls whether the cache hits.
const buildIDsEnv = "GREMLINS_TEST_BUILD_IDS"

// invocationLogEnv names a file the helper appends to, so a test can see which
// commands were actually issued — the only way to tell a cache hit from a
// rebuild that produced the same answer.
const invocationLogEnv = "GREMLINS_TEST_INVOCATION_LOG"

// listOnlyEnv narrows the `go list` the helper fakes to one package, which is
// how a test stands a scoped run — the recommended workflow, and the one that
// used to evict the rest of the module's map.
const listOnlyEnv = "GREMLINS_TEST_LIST_ONLY"

// goEnvEnv overrides what the helper reports for `go env`, as "NAME=value"
// pairs separated by newlines, which is how a test changes the build
// environment without changing a byte of source.
const goEnvEnv = "GREMLINS_TEST_GO_ENV"

// notMainEnv names a package the helper's dependency listing reports as
// belonging to a module that is not a main module — a replace target, or a
// vendored or module-cache dependency that somehow sits outside both — which
// is a dependency the map builder must not instrument.
const notMainEnv = "GREMLINS_TEST_NOT_MAIN"

// extraProfileEnv carries profile lines the helper appends to every profile it
// writes, after dropping what the binary did not instrument: a block of a name
// no package claims, as a //line directive can produce.
const extraProfileEnv = "GREMLINS_TEST_EXTRA_PROFILE"

// compileDelayEnv and runDelayEnv make the fake go's test-binary compile and
// each test run take at least that long, as Go durations, so that a test can
// reason about the times the map records. A fake coverage gather pays both, per
// package it gathers: one compile, and one run of each of its tests.
const (
	compileDelayEnv = "GREMLINS_TEST_COMPILE_DELAY"
	runDelayEnv     = "GREMLINS_TEST_RUN_DELAY"
)

// The fixture has real sources as well as real directories, because the map
// cache reads them: what a package's source looked like when its map was made
// is how a run decides which of its mappings a change reached. The line numbers
// below are the ones the fake profiles name, so a change to either has to be a
// change to both.
const (
	// rootSource declares Descend over lines 5-7 and Ascend over lines 9-11.
	rootSource = `package root

import "example.com/vm"

func Descend(n int) int {
	return vm.Clamp(n, 0, 10)
}

func Ascend(n int) int {
	return vm.Clamp(n, 10, 0)
}
`
	// vmSource declares Clamp over lines 3-10, doc comment included, and Size
	// over lines 12-14. No test executes Size, which is what makes it the case
	// per-test invalidation exists for.
	vmSource = `package vm

// Clamp holds n between lo and hi.
func Clamp(n, lo, hi int) int {
	x := n
	if x > hi {
		x = hi
	}
	return x
}

func Size(v []int) int {
	return len(v)
}
`
)

// The test files declare the tests the helper process lists and runs. Nothing
// reads their bodies — the profiles are canned — but a mapping is only kept
// for a test whose declaration says what it is.
const (
	vmTestSource = `package vm

import "testing"

func TestSizeAscending(t *testing.T) {
	_ = Clamp(1, 0, 2)
}
`
	rootTestSource = `package root

import "testing"

func TestRangeDescending(t *testing.T) {
	_ = Descend(1)
}

func TestRangeAscending(t *testing.T) {
	_ = Ascend(1)
}
`
)

// The calc package is the fixture for invalidating a map per test rather than
// per package. Its profiles name only its own files, which is the shape a run
// without --cross-package produces — the test binary is built with -coverpkg
// for its own package alone — and the shape per-test invalidation is sound for.
const (
	// calcSource declares Double over lines 3-5 and Triple over lines 7-9.
	calcSource = `package calc

func Double(n int) int {
	return n * 2
}

func Triple(n int) int {
	return n * 3
}
`
	// calcSourceDoubleGrown adds a line inside Double, which pushes Triple down
	// by one without changing a byte of it.
	calcSourceDoubleGrown = `package calc

func Double(n int) int {
	m := n
	return m * 2
}

func Triple(n int) int {
	return n * 3
}
`
	doubleTestSource = `package calc

import "testing"

func TestDouble(t *testing.T) {
	if Double(2) != 4 {
		t.Fail()
	}
}
`
	tripleTestSource = `package calc

import "testing"

func TestTriple(t *testing.T) {
	if Triple(2) != 6 {
		t.Fail()
	}
}
`
)

// tripleEndLine is the last line of Triple before Double grows, and so a line
// that is covered only once a kept mapping has been moved with it.
const tripleEndLine = 10

func fixtureRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, d := range []string{"root", "vm", "empty", "calc"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o750); err != nil {
			t.Fatalf("cannot create the fixture directory: %v", err)
		}
	}
	writeFixture(t, filepath.Join(root, "root", "root.go"), rootSource)
	writeFixture(t, filepath.Join(root, "vm", "vm.go"), vmSource)
	writeFixture(t, filepath.Join(root, "calc", "calc.go"), calcSource)
	writeFixture(t, filepath.Join(root, "calc", "double_test.go"), doubleTestSource)
	writeFixture(t, filepath.Join(root, "calc", "triple_test.go"), tripleTestSource)
	writeFixture(t, filepath.Join(root, "vm", "vm_test.go"), vmTestSource)
	writeFixture(t, filepath.Join(root, "root", "root_test.go"), rootTestSource)

	return root
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("cannot write the fixture source: %v", err)
	}
}

func buildMap(t *testing.T, helper string, opts ...coverage.Option) *coverage.TestMap {
	t.Helper()

	log.Init(&bytes.Buffer{}, &bytes.Buffer{})
	t.Cleanup(log.Reset)

	// These cases are about seeing a test in one package execute a line in
	// another, which only a --cross-package map records: without it a test's
	// profile holds its own package's lines and nothing else.
	viper.Set(configuration.UnleashCrossPackageKey, true)
	t.Cleanup(func() { viper.Set(configuration.UnleashCrossPackageKey, false) })

	root := fixtureRoot(t)
	mod := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}
	// Every test gets its own cache directory: the real one belongs to whoever
	// is running the suite, and a shared one would let one test answer another.
	cov := coverage.NewWithCmd(fakeGoCommand(helper, root), t.TempDir(), mod,
		append([]coverage.Option{coverage.WithTestMapCacheDir(t.TempDir()), coverage.WithStubTypes()}, opts...)...)

	tm, err := cov.BuildTestMap()
	if err != nil {
		t.Fatalf("BuildTestMap() error: %v", err)
	}

	return tm
}

func TestBuildTestMap(t *testing.T) {
	tm := buildMap(t, "TestTestMapHelperProcess")

	t.Run("maps every test of every package", func(t *testing.T) {
		if got := tm.Len(); got != 5 {
			t.Errorf("want 5 tests mapped, got %d", got)
		}
	})

	t.Run("finds the test in another package that executes the line", func(t *testing.T) {
		pos := token.Position{Filename: "vm/vm.go", Line: clampedLine, Column: 3}

		want := []coverage.TestID{{Pkg: "example.com", Name: "TestRangeDescending"}}
		if diff := cmp.Diff(want, tm.TestsFor(pos)); diff != "" {
			t.Errorf("TestsFor() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("does not select a test that never executes the line", func(t *testing.T) {
		pos := token.Position{Filename: "vm/vm.go", Line: uncoveredLine, Column: 3}

		if got := tm.TestsFor(pos); len(got) != 0 {
			t.Errorf("want no tests, got %v", got)
		}
	})

	t.Run("the union holds what any test executed, once", func(t *testing.T) {
		union := tm.Union()

		if !union.IsCovered(token.Position{Filename: "vm/vm.go", Line: clampedLine, Column: 3}) {
			t.Error("expected the union to cover the line only one test executes")
		}
		if got := len(union["vm/vm.go"]); got != 2 {
			t.Errorf("want the two distinct blocks of vm.go, got %d", got)
		}
	})

	t.Run("a package whose tests all mapped can be selected from", func(t *testing.T) {
		if !tm.Mapped("example.com") || !tm.Mapped("example.com/vm") {
			t.Error("expected both packages to be mapped")
		}
	})

	// Nothing to map is a complete answer, not a missing one — and saying so is
	// what lets a mutant there be judged by covering tests in other packages.
	t.Run("a package with no test files is mapped by having no tests", func(t *testing.T) {
		if !tm.Mapped("example.com/empty") {
			t.Error("expected a package with no test files to report as mapped")
		}
	})

	t.Run("a package that was never listed is not mapped", func(t *testing.T) {
		if tm.Mapped("example.com/absent") {
			t.Error("expected an unknown package to report as unmapped")
		}
	})
}

// scoped is the one fixture package the scope tests keep.
const scoped = "example.com/vm"

// A --diff run has mutants only in the packages its changed files belong to,
// so mapping the rest is work no mutant reads: on a 139-package module it was
// most of a warm map's 5m34s. A package left out is unmapped, never "mapped
// with no tests", so a mutant there still falls back to its package's whole
// suite (TestSelectionFallsBackToTheWholeSuite, "when the mutated package could
// not be mapped") rather than running nothing.
func TestBuildTestMapMapsOnlyThePackagesInScope(t *testing.T) {
	tm := buildMap(t, "TestTestMapHelperProcess", coverage.WithMapScope([]string{scoped}))

	if !tm.Mapped(scoped) {
		t.Error("expected the package in scope to be mapped")
	}
	for _, pkg := range []string{"example.com", "example.com/empty"} {
		if tm.Mapped(pkg) {
			t.Errorf("expected %s, outside the scope, to be unmapped so it falls back to its whole suite", pkg)
		}
	}
	if got, all := tm.Len(), buildMap(t, "TestTestMapHelperProcess").Len(); got == 0 || got >= all {
		t.Errorf("want only the scoped package's tests mapped: got %d of the unscoped %d", got, all)
	}
}

// A scoped package the module listing does not return means the diff's paths
// and the listing disagree on what a package is. Nothing is mapped for it
// either way, so the run is unaffected, but it is said out loud rather than
// matching nothing in silence.
func TestBuildTestMapNamesAScopedPackageTheListingLacks(t *testing.T) {
	var out bytes.Buffer
	log.Init(&out, &out)
	t.Cleanup(log.Reset)
	mod := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}
	cov := coverage.NewWithCmd(fakeGoCommand("TestTestMapHelperProcess", fixtureRoot(t)), t.TempDir(), mod,
		coverage.WithTestMapCacheDir(t.TempDir()), coverage.WithStubTypes(),
		coverage.WithMapScope([]string{scoped, "example.com/nowhere"}))
	if _, err := cov.BuildTestMap(); err != nil {
		t.Fatalf("BuildTestMap() error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "example.com/nowhere") || strings.Contains(got, "lacks "+scoped) {
		t.Errorf("want the log to name only the unlisted package, got:\n%s", got)
	}
}

func TestBuildTestMapLeavesAPackageUnmappedWhenATestCannotBeRun(t *testing.T) {
	tm := buildMap(t, "TestTestMapHelperProcessFailingTest")

	// Half a package is worse than none of it: what is missing from the map is
	// exactly what selection would silently skip, so the package must fall back
	// to its whole suite.
	if tm.Mapped("example.com") {
		t.Error("expected the package holding the unrunnable test to be unmapped")
	}
	if !tm.Mapped("example.com/vm") {
		t.Error("expected the other package to still be mapped")
	}
	// Everything but the package holding the unrunnable test still maps.
	if got := tm.Len(); got != 3 {
		t.Errorf("want the other packages' tests, got %d", got)
	}
}

func TestBuildTestMapLeavesAPackageUnmappedWhenItsTestsCannotCompile(t *testing.T) {
	tm := buildMap(t, "TestTestMapHelperProcessCompileFailure")

	if tm.Mapped("example.com") {
		t.Error("expected the package whose tests do not compile to be unmapped")
	}
	if !tm.Mapped("example.com/vm") {
		t.Error("expected the other package to still be mapped")
	}
}

func TestBuildTestMapFailsWhenThePackagesCannotBeListed(t *testing.T) {
	log.Init(&bytes.Buffer{}, &bytes.Buffer{})
	defer log.Reset()

	mod := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}
	cov := coverage.NewWithCmd(
		fakeGoCommand("TestTestMapHelperProcessListFailure", fixtureRoot(t)), t.TempDir(), mod,
		coverage.WithTestMapCacheDir(t.TempDir()))

	if _, err := cov.BuildTestMap(); err == nil {
		t.Error("expected an error when the packages cannot be listed")
	}
}

const (
	// TestRangeDescending is the only test that reaches the clamp.
	profileRangeDescending = "mode: set\n" +
		"example.com/root.go:6.26,6.50 1 1\n" +
		"example.com/vm/vm.go:4.29,6.15 2 1\n" +
		"example.com/vm/vm.go:6.15,8.3 1 1\n"
	profileRangeAscending = "mode: set\n" +
		"example.com/root.go:10.26,10.50 1 1\n" +
		"example.com/vm/vm.go:4.29,6.15 2 1\n"
	profileSizeAscending = "mode: set\n" +
		"example.com/vm/vm.go:4.29,6.15 2 1\n"
	// TestDouble also executes the start of vm's Clamp, which its binary
	// records only once vm is instrumented alongside calc.
	profileDouble = "mode: set\n" +
		"example.com/calc/calc.go:3.24,5.2 1 1\n" +
		"example.com/vm/vm.go:4.29,6.15 2 1\n"
	profileTriple = "mode: set\n" +
		"example.com/calc/calc.go:7.24,9.2 1 1\n"
)

func fakeGoCommand(helper, pkgRoot string) func(command string, args ...string) *exec.Cmd {
	return fakeGoCommandWith(helper, pkgRoot, "", "", "", "", "", "")
}

func fakeGoCommandWith(helper, pkgRoot, buildIDs, logPath, listOnly, goEnv, notMain, extraProfile string) func(command string, args ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=" + helper, "--", command}
		cs = append(cs, args...)
		// #nosec G204 G702 - We are in tests, we don't care
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{
			"GO_TEST_PROCESS=1",
			// ThreadSanitizer sleeps for a second on the way out of every
			// race-instrumented process, so that a race in a thread still
			// finishing is still reported. This process is a fake `go` that
			// prints a fixed answer and exits, and the map builder starts
			// hundreds of them: at the default the package takes nine minutes
			// under -race and seven seconds without, which reads as a deadlock
			// rather than as a sleep. There is nothing here for the sleep to
			// catch.
			"GORACE=atexit_sleep_ms=0",
			pkgDirsEnv + "=" + pkgRoot,
			buildIDsEnv + "=" + buildIDs,
			invocationLogEnv + "=" + logPath,
			listOnlyEnv + "=" + listOnly,
			goEnvEnv + "=" + goEnv,
			notMainEnv + "=" + notMain,
			extraProfileEnv + "=" + extraProfile,
		}

		return cmd
	}
}

// TestTestMapHelperProcess stands in for the go command and for the test
// binaries it compiles: it answers `go list`, pretends to compile, lists the
// tests of the binary it is impersonating, and writes the profile a per-test
// coverage run would produce.
func TestTestMapHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	respondAsGo(t, "", "")
}

func TestTestMapHelperProcessFailingTest(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	respondAsGo(t, "TestRangeAscending", "")
}

func TestTestMapHelperProcessCompileFailure(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	respondAsGo(t, "", "example.com")
}

func TestTestMapHelperProcessListFailure(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	if command(os.Args) == "go" && hasFlag(os.Args, "list") {
		fmt.Fprintln(os.Stderr, "cannot load package")
		os.Exit(1) // skipcq: RVV-A0003
	}
	respondAsGo(t, "", "")
}

//nolint:cyclop // it stands in for three different commands; splitting it hides the shape
func respondAsGo(t *testing.T, failingTest, uncompilablePkg string) {
	t.Helper()

	cmd := command(os.Args)
	root := os.Getenv(pkgDirsEnv)

	if cmd == "go" && hasFlag(os.Args, "tool") && hasFlag(os.Args, "buildid") {
		fmt.Fprintln(os.Stdout, buildIDFor(os.Args[len(os.Args)-1]))
		os.Exit(0) // skipcq: RVV-A0003
	}

	// A dependency listing is a `go list` too, and it has to be answered before
	// the package listing or the package listing swallows it.
	if cmd == "go" && hasFlag(os.Args, "list") && hasFlag(os.Args, "-deps") {
		listDepsAsGo(root, os.Args[len(os.Args)-1])
		os.Exit(0) // skipcq: RVV-A0003
	}

	if cmd == "go" && hasFlag(os.Args, "env") {
		envAsGo(os.Args)
		os.Exit(0) // skipcq: RVV-A0003
	}

	if cmd == "go" && hasFlag(os.Args, "list") {
		listPackagesAsGo(root, os.Getenv(listOnlyEnv))
		os.Exit(0) // skipcq: RVV-A0003
	}

	// A coverage gather: `go test ... -cover -coverprofile <file>
	// <pkgs>`. It names the packages it gathered in the log, which is how a
	// test sees whether one was issued and what for.
	if cmd == "go" && hasFlag(os.Args, "mod") && hasFlag(os.Args, "download") {
		os.Exit(0) // skipcq: RVV-A0003
	}

	if cmd == "go" && hasFlag(os.Args, "-cover") {
		gatherAsGo(os.Args)
		os.Exit(0) // skipcq: RVV-A0003
	}

	if cmd == "go" && hasFlag(os.Args, "-c") {
		sleepFor(compileDelayEnv, 1)
		out := flagValue(os.Args, "-o")
		if uncompilablePkg != "" && strings.HasSuffix(out, binaryName(uncompilablePkg)) {
			fmt.Fprintln(os.Stderr, "build failed")
			os.Exit(1) // skipcq: RVV-A0003
		}
		// The binary records what it was instrumented for, which is what
		// decides whose lines its profiles can name.
		writeOrDie(out, "#!/bin/false\ncoverpkg="+flagValue(os.Args, "-coverpkg")+"\n")
		os.Exit(0) // skipcq: RVV-A0003
	}

	if hasFlag(os.Args, "-test.list") {
		switch {
		case strings.HasSuffix(cmd, binaryName("example.com/vm")):
			fmt.Fprint(os.Stdout, "TestSizeAscending\nwarning: GOCOVERDIR not set, no coverage data emitted\n")
		case strings.HasSuffix(cmd, binaryName("example.com/calc")):
			fmt.Fprint(os.Stdout, "TestDouble\nTestTriple\n")
		default:
			fmt.Fprint(os.Stdout, "TestRangeDescending\nTestRangeAscending\n")
		}
		os.Exit(0) // skipcq: RVV-A0003
	}

	run := strings.Trim(flagValue(os.Args, "-test.run"), "^$")
	recordInvocation("run " + run)
	sleepFor(runDelayEnv, 1)
	if run == failingTest {
		fmt.Fprintln(os.Stderr, "--- FAIL: "+run)
		os.Exit(1) // skipcq: RVV-A0003
	}
	profiles := map[string]string{
		"TestRangeDescending": profileRangeDescending,
		"TestRangeAscending":  profileRangeAscending,
		"TestSizeAscending":   profileSizeAscending,
		"TestDouble":          profileDouble,
		"TestTriple":          profileTriple,
	}
	profile, ok := profiles[run]
	if !ok {
		fmt.Fprintln(os.Stderr, "unexpected -test.run "+run)
		os.Exit(1) // skipcq: RVV-A0003
	}
	writeOrDie(flagValue(os.Args, "-test.coverprofile"), instrumentedOnly(cmd, profile))
	os.Exit(0) // skipcq: RVV-A0003
}

// gatherAsGo answers a coverage gather over the packages named after the
// profile path: it waits gatherDelayEnv once per package, and writes a profile
// holding every block their tests execute.
func gatherAsGo(args []string) {
	file := flagValue(args, "-coverprofile")
	var pkgs []string
	for i, a := range args {
		if a == "-coverprofile" {
			pkgs = args[i+2:]

			break
		}
	}
	recordInvocation("gather " + strings.Join(pkgs, " "))
	byPkg := map[string][]string{
		"example.com":      {profileRangeDescending, profileRangeAscending},
		"example.com/vm":   {profileSizeAscending},
		"example.com/calc": {profileDouble, profileTriple},
	}
	out := "mode: set\n"
	for _, p := range expandPatterns(pkgs) {
		// A gather compiles the package's tests and runs its suite once.
		sleepFor(compileDelayEnv, 1)
		sleepFor(runDelayEnv, len(byPkg[p]))
		for _, profile := range byPkg[p] {
			out += strings.TrimPrefix(profile, "mode: set\n")
		}
	}
	writeOrDie(file, out)
}

// expandPatterns turns the package patterns a gather names — "./...",
// "./calc/..." or an import path — into the fixture's import paths.
func expandPatterns(patterns []string) []string {
	var pkgs []string
	for _, p := range patterns {
		switch {
		case p == "./...":
			pkgs = append(pkgs, "example.com", "example.com/vm", "example.com/calc")
		case strings.HasPrefix(p, "./"):
			pkgs = append(pkgs, "example.com/"+strings.TrimSuffix(strings.TrimPrefix(p, "./"), "/..."))
		default:
			pkgs = append(pkgs, p)
		}
	}

	return pkgs
}

// sleepFor waits n times the duration the environment variable holds, if any.
func sleepFor(env string, n int) {
	d, err := time.ParseDuration(os.Getenv(env))
	if err != nil {
		return
	}
	time.Sleep(time.Duration(n) * d)
}

// instrumentedOnly drops the blocks of every package the binary was not
// instrumented for, as a real coverage profile never holds them: a binary built
// with -coverpkg for its own package names no line of a dependency.
func instrumentedOnly(binary, profile string) string {
	data, err := os.ReadFile(binary) //nolint:gosec // the binary this helper wrote itself
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) // skipcq: RVV-A0003
	}
	_, coverPkg, _ := strings.Cut(string(data), "coverpkg=")
	coverPkg = strings.TrimSpace(coverPkg)
	if strings.Contains(coverPkg, "...") {
		return profile
	}
	scope := map[string]bool{}
	for _, p := range strings.Split(coverPkg, ",") {
		scope[p] = true
	}
	lines := strings.Split(profile, "\n")
	kept := []string{lines[0]}
	for _, line := range lines[1:] {
		file, _, _ := strings.Cut(line, ":")
		if scope[path.Dir(file)] {
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\n") + "\n" + os.Getenv(extraProfileEnv)
}

// listPackagesAsGo writes the package lines `go list -f` would, narrowed to one
// package when the test asked for a scoped run.
func listPackagesAsGo(root, only string) {
	lines := []struct{ path, dir, tests string }{
		{"example.com", "root", "2\t0"},
		{"example.com/vm", "vm", "1\t0"},
		{"example.com/empty", "empty", "0\t0"},
		{"example.com/calc", "calc", "2\t0"},
	}
	for _, l := range lines {
		if only != "" && l.path != only {
			continue
		}
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", l.path, filepath.Join(root, l.dir), l.tests)
	}
}

// envAsGo answers `go env -json NAME...` for the names asked, the way go does:
// one JSON object, with an empty value for a name that is unset.
//
// The toolchain root and the module cache are ones the fixture is not inside,
// so nothing the fixture holds is filtered out as already pinned. Everything
// else is a plausible default that a test can override through goEnvEnv.
func envAsGo(args []string) {
	values := map[string]string{
		"GOROOT":      "/nonexistent/goroot",
		"GOMODCACHE":  "/nonexistent/modcache",
		"GOVERSION":   "go-fixture",
		"GOOS":        "linux",
		"GOARCH":      "amd64",
		"GOAMD64":     "v1",
		"CGO_ENABLED": "1",
	}
	for _, pair := range strings.Split(os.Getenv(goEnvEnv), "\n") {
		if name, value, ok := strings.Cut(pair, "="); ok {
			values[name] = value
		}
	}
	asked := map[string]string{}
	afterJSON := false
	for _, a := range args {
		switch {
		case a == "-json":
			afterJSON = true
		case afterJSON:
			asked[a] = values[a]
		}
	}
	out, err := json.Marshal(asked)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) // skipcq: RVV-A0003
	}
	// A coverage-instrumented helper also writes a GOCOVERDIR warning, to
	// stderr; a real go can write warnings there too. Neither is the answer.
	fmt.Fprintln(os.Stderr, "go: warning: something unrelated")
	fmt.Fprintln(os.Stdout, string(out))
}

// listDepsAsGo writes the packages `go list -deps -test` would, by import path
// and directory, which is what says whether a package was changed from
// underneath. Everything depends on vm, so an edit there is the case where a
// package's own fingerprint looks narrowable and its mappings are stale anyway.
//
// The package itself is listed twice, as go does: once as it is and once as
// recompiled for its own test binary, under the bracketed name go gives that.
// Each line ends with the package's name, which is what an unnamed import of
// it binds.
func listDepsAsGo(root, pkg string) {
	const vm = "example.com/vm"
	main := os.Getenv(notMainEnv) != vm
	if pkg != vm {
		fmt.Fprintf(os.Stdout, "%s [%s.test]\t%s\t%t\tvm\n", vm, pkg, filepath.Join(root, "vm"), main)
		fmt.Fprintf(os.Stdout, "%s\t%s\t%t\tvm\n", vm, filepath.Join(root, "vm"), main)
	}
	name := path.Base(dirOf(pkg))
	fmt.Fprintf(os.Stdout, "%s\t%s\ttrue\t%s\n", pkg, filepath.Join(root, dirOf(pkg)), name)
	fmt.Fprintf(os.Stdout, "%s [%s.test]\t%s\ttrue\t%s\n", pkg, pkg, filepath.Join(root, dirOf(pkg)), name)
	// A standard-library package belongs to no module at all.
	fmt.Fprintf(os.Stdout, "testing\t/nonexistent/goroot/src/testing\t\ttesting\n")
	for _, std := range []string{"embed", "errors", "fmt", "regexp"} {
		fmt.Fprintf(os.Stdout, "%s\t/nonexistent/goroot/src/%s\t\t%s\n", std, std, std)
	}
	// go writes build diagnostics to the same stream, and a synthesised test
	// package can report no directory at all.
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "go: downloading example.com/thing v1.0.0")
}

func dirOf(pkg string) string {
	switch pkg {
	case "example.com/vm":
		return "vm"
	case "example.com/calc":
		return "calc"
	default:
		return "root"
	}
}

// buildIDFor reports the build ID the test chose for the package this binary
// belongs to, defaulting to one derived from the binary itself so that a test
// that does not care still gets a stable answer.
func buildIDFor(binary string) string {
	for _, pair := range strings.Split(os.Getenv(buildIDsEnv), ",") {
		pkg, id, ok := strings.Cut(pair, "=")
		if ok && strings.HasSuffix(binary, binaryName(pkg)) {
			return id
		}
	}

	return "buildid-of-" + filepath.Base(binary)
}

// recordInvocation appends to the file a test is watching, if it asked for one.
func recordInvocation(line string) {
	path := os.Getenv(invocationLogEnv)
	if path == "" {
		return
	}
	// #nosec G304 G702 G703 - the path comes from the environment this test process was given
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	fmt.Fprintln(f, line)
}

// binaryName mirrors the name the builder gives a package's test binary.
func binaryName(importPath string) string {
	return strings.NewReplacer("/", "_", ".", "_").Replace(importPath) + ".test"
}

func writeOrDie(path, content string) {
	// #nosec G306 G703 - the path comes from the arguments this test process gave itself
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) // skipcq: RVV-A0003
	}
}

// command returns what the process under test was asked to run, which is the
// argument right after the "--" separator.
func command(args []string) string {
	for i, a := range args {
		if a == "--" && i+1 < len(args) {
			return args[i+1]
		}
	}

	return ""
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}

	return false
}

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}

	return ""
}
