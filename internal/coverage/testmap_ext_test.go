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
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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

func fixtureRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, d := range []string{"root", "vm", "empty"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o750); err != nil {
			t.Fatalf("cannot create the fixture directory: %v", err)
		}
	}

	return root
}

func buildMap(t *testing.T, helper string) *coverage.TestMap {
	t.Helper()

	log.Init(&bytes.Buffer{}, &bytes.Buffer{})
	t.Cleanup(log.Reset)

	root := fixtureRoot(t)
	mod := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}
	// Every test gets its own cache directory: the real one belongs to whoever
	// is running the suite, and a shared one would let one test answer another.
	cov := coverage.NewWithCmd(fakeGoCommand(helper, root), t.TempDir(), mod,
		coverage.WithTestMapCacheDir(t.TempDir()))

	tm, err := cov.BuildTestMap()
	if err != nil {
		t.Fatalf("BuildTestMap() error: %v", err)
	}

	return tm
}

func TestBuildTestMap(t *testing.T) {
	tm := buildMap(t, "TestTestMapHelperProcess")

	t.Run("maps every test of every package", func(t *testing.T) {
		if got := tm.Len(); got != 3 {
			t.Errorf("want 3 tests mapped, got %d", got)
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
	if got := tm.Len(); got != 1 {
		t.Errorf("want only the mapped package's single test, got %d", got)
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
		"example.com/root.go:6.26,6.50 1 1\n" +
		"example.com/vm/vm.go:4.29,6.15 2 1\n"
	profileSizeAscending = "mode: set\n" +
		"example.com/vm/vm.go:4.29,6.15 2 1\n"
)

func fakeGoCommand(helper, pkgRoot string) func(command string, args ...string) *exec.Cmd {
	return fakeGoCommandWith(helper, pkgRoot, "", "")
}

func fakeGoCommandWith(helper, pkgRoot, buildIDs, logPath string) func(command string, args ...string) *exec.Cmd {
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

	if cmd == "go" && hasFlag(os.Args, "list") {
		fmt.Fprintf(os.Stdout, "example.com\t%s\t2\t0\n", filepath.Join(root, "root"))
		fmt.Fprintf(os.Stdout, "example.com/vm\t%s\t1\t0\n", filepath.Join(root, "vm"))
		fmt.Fprintf(os.Stdout, "example.com/empty\t%s\t0\t0\n", filepath.Join(root, "empty"))
		os.Exit(0) // skipcq: RVV-A0003
	}

	if cmd == "go" && hasFlag(os.Args, "-c") {
		out := flagValue(os.Args, "-o")
		if uncompilablePkg != "" && strings.HasSuffix(out, binaryName(uncompilablePkg)) {
			fmt.Fprintln(os.Stderr, "build failed")
			os.Exit(1) // skipcq: RVV-A0003
		}
		writeOrDie(out, "#!/bin/false\n")
		os.Exit(0) // skipcq: RVV-A0003
	}

	if hasFlag(os.Args, "-test.list") {
		if strings.HasSuffix(cmd, binaryName("example.com/vm")) {
			fmt.Fprint(os.Stdout, "TestSizeAscending\nwarning: GOCOVERDIR not set, no coverage data emitted\n")
		} else {
			fmt.Fprint(os.Stdout, "TestRangeDescending\nTestRangeAscending\n")
		}
		os.Exit(0) // skipcq: RVV-A0003
	}

	run := strings.Trim(flagValue(os.Args, "-test.run"), "^$")
	recordInvocation("run " + run)
	if run == failingTest {
		fmt.Fprintln(os.Stderr, "--- FAIL: "+run)
		os.Exit(1) // skipcq: RVV-A0003
	}
	profiles := map[string]string{
		"TestRangeDescending": profileRangeDescending,
		"TestRangeAscending":  profileRangeAscending,
		"TestSizeAscending":   profileSizeAscending,
	}
	profile, ok := profiles[run]
	if !ok {
		fmt.Fprintln(os.Stderr, "unexpected -test.run "+run)
		os.Exit(1) // skipcq: RVV-A0003
	}
	writeOrDie(flagValue(os.Args, "-test.coverprofile"), profile)
	os.Exit(0) // skipcq: RVV-A0003
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
