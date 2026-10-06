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
	"encoding/json"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/coverage"
)

// edit rewrites one of the fixture's source files, standing for the change that
// put the package in scope in the first place.
func (h *cacheHarness) edit(rel, content string) {
	h.t.Helper()

	if err := os.WriteFile(filepath.Join(h.pkgRoot, rel), []byte(content), 0o600); err != nil {
		h.t.Fatalf("cannot edit the fixture source: %v", err)
	}
}

// changedCalc reports the build ID pair that says the calc package's test binary
// was rebuilt, which is what any edit to it does.
const changedCalc = "example.com/calc=changed-by-an-edit"

// calcPos is a mutant's position as a run scoped to the calc package reports it:
// relative to the directory Gremlins was pointed at, which is that package. The
// profiles it is matched against are relative to the module root, and
// coverage.ProfilePosition is what bridges the two.
func calcPos(line int) token.Position {
	return token.Position{Filename: "calc.go", Line: line, Column: 1}
}

// buildCalc maps the calc package alone, which is the shape a run scoped to the
// package under edit has — the one this whole mechanism exists for, and the one
// CONTRIBUTING tells a developer to use.
func (h *cacheHarness) buildCalc(buildIDs string) *coverage.TestMap {
	h.t.Helper()

	return h.buildScoped("TestTestMapHelperProcess", buildIDs, "example.com/calc")
}

// The case the whole thing exists for. The package under mutation is the
// package that was changed, so its build ID is invalid by construction — but
// most of its tests never executed the lines that moved.
func TestAChangeReMapsOnlyTheTestsThatExecutedIt(t *testing.T) {
	h := newCacheHarness(t)

	first := h.buildCalc("")
	if got := first.TestsFor(calcPos(tripleEndLine)); len(got) != 0 {
		t.Fatalf("want the line below Triple uncovered before the edit, got %v", got)
	}

	h.edit("calc/calc.go", calcSourceDoubleGrown)
	second := h.buildCalc(changedCalc)

	if diff := cmp.Diff([]string{"TestDouble"}, h.testsRun()); diff != "" {
		t.Errorf("want only the test that executed the change re-mapped (-want +got):\n%s", diff)
	}
	if second.Len() != first.Len() {
		t.Errorf("want the map still complete at %d tests, got %d", first.Len(), second.Len())
	}

	// The kept mapping has to answer about where its code is now, not where it
	// was: Triple did not change, but it moved down a line when Double grew.
	want := []coverage.TestID{{Pkg: "example.com/calc", Name: "TestTriple"}}
	if diff := cmp.Diff(want, second.TestsFor(calcPos(tripleEndLine))); diff != "" {
		t.Errorf("the kept mapping was not moved with its code (-want +got):\n%s", diff)
	}
}

// A change nothing executes is the best case, and it is not rare: most of a
// package's files are touched by a small fraction of its suite.
func TestAChangeNoTestExecutedReMapsNothing(t *testing.T) {
	h := newCacheHarness(t)

	h.buildScoped("TestTestMapHelperProcess", "", "example.com/vm")
	h.edit("vm/vm.go", vmSource+`
func Unused(v []int) int {
	return len(v) + 1
}
`)
	tm := h.buildScoped("TestTestMapHelperProcess", "example.com/vm=changed-by-an-edit", "example.com/vm")

	if got := h.testsRun(); len(got) != 0 {
		t.Errorf("want nothing re-mapped for a function nothing calls, got %v", got)
	}
	if tm.Len() != 1 {
		t.Errorf("want the map still complete, got %d tests", tm.Len())
	}
}

// A test's own body is not in any profile — coverage does not instrument test
// files — so it is attributed by name instead of by line.
func TestAChangedTestReMapsOnlyItself(t *testing.T) {
	h := newCacheHarness(t)

	h.buildCalc("")
	h.edit("calc/triple_test.go", `package calc

import "testing"

func TestTriple(t *testing.T) {
	if Triple(3) != 9 {
		t.Fail()
	}
}
`)
	h.buildCalc(changedCalc)

	if diff := cmp.Diff([]string{"TestTriple"}, h.testsRun()); diff != "" {
		t.Errorf("want only the changed test re-mapped (-want +got):\n%s", diff)
	}
}

// vmClampChanged changes the body of Clamp, which TestDouble executes through
// its dependency on vm, without moving a line.
var vmClampChanged = strings.Replace(vmSource, "x := n\n", "x := n + 0\n", 1)

// vmSizeChanged changes the body of Size, which no test of calc executes.
var vmSizeChanged = strings.Replace(vmSource, "return len(v)\n", "return len(v) + 0\n", 1)

// A dependency's lines are recorded too, as the functions each test executed
// there, so a changed body in a dependency is attributed exactly as one in the
// package is: only the tests that executed it can behave differently.
func TestADependencyBodyChangeReMapsOnlyTheTestsThatExecutedIt(t *testing.T) {
	testCases := map[string]struct {
		vm   string
		want []string
	}{
		"a function one test executed": {vmClampChanged, []string{"TestDouble"}},
		"a function no test executed":  {vmSizeChanged, nil},
		// Reaching it would take a call, and the call is a change of its own.
		"an added function": {vmSource + "\nfunc Extra(v []int) int {\n\treturn len(v) + 1\n}\n", nil},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			h := newCacheHarness(t)
			h.buildCalc("")

			h.edit("vm/vm.go", tc.vm)
			tm := h.buildCalc(changedCalc)

			if diff := cmp.Diff(tc.want, h.testsRun()); diff != "" {
				t.Errorf("re-mapped the wrong tests (-want +got):\n%s", diff)
			}
			if tm.Len() != 2 {
				t.Errorf("want the map still complete, got %d tests", tm.Len())
			}
		})
	}
}

// A change in the package and one beneath it at once narrow independently:
// each dirties the tests that executed it, and nothing else.
func TestAChangeUnderneathAChangedPackageNarrowsAcrossBoth(t *testing.T) {
	h := newCacheHarness(t)

	h.buildCalc("")
	h.edit("calc/calc.go", calcSourceDoubleGrown)
	h.edit("vm/vm.go", vmSizeChanged)
	second := h.buildCalc(changedCalc)

	if diff := cmp.Diff([]string{"TestDouble"}, h.testsRun()); diff != "" {
		t.Errorf("want only the test that executed a change re-mapped (-want +got):\n%s", diff)
	}
	want := []coverage.TestID{{Pkg: "example.com/calc", Name: "TestTriple"}}
	if diff := cmp.Diff(want, second.TestsFor(calcPos(tripleEndLine))); diff != "" {
		t.Errorf("the kept mapping was not moved with its code (-want +got):\n%s", diff)
	}
}

// A dependency outside every main module — a replace target, say — is not
// instrumented, so no profile says which of its functions a test executed, and
// any change there still re-maps the package through Inputs.
func TestAChangeToAnUninstrumentedDependencyReMapsThePackage(t *testing.T) {
	h := newCacheHarness(t)
	h.notMain = "example.com/vm"

	h.buildCalc("")
	h.edit("vm/vm.go", vmSizeChanged)
	h.buildCalc(changedCalc)

	want := []string{"TestDouble", "TestTriple"}
	if diff := cmp.Diff(want, h.testsRun()); diff != "" {
		t.Errorf("want the whole package re-mapped (-want +got):\n%s", diff)
	}
}

// What a test executed in a dependency is for narrowing alone. Merged into the
// profile, a line of vm that only another package's tests execute would read as
// covered — making mutants runnable that are NOT COVERED without the map, and
// selecting tests for them that package scoping would never run.
func TestDependencyCoverageIsNotCoverage(t *testing.T) {
	h := newCacheHarness(t)

	tm := h.build("TestTestMapHelperProcess", "")

	// vm.go:7 is executed by root's TestRangeDescending, through vm, and by no
	// test of vm itself.
	clamped := token.Position{Filename: "vm/vm.go", Line: clampedLine, Column: 3}
	if got := tm.TestsFor(clamped); len(got) != 0 {
		t.Errorf("want no test for a line only another package executes, got %v", got)
	}
	if tm.Union().IsCovered(clamped) {
		t.Error("want a line only another package executes left out of the union")
	}

	// vm.go:5 is executed by vm's own test, and by tests of root and calc
	// through vm; only the first is an answer about vm.
	own := token.Position{Filename: "vm/vm.go", Line: vmOwnLine, Column: 3}
	want := []coverage.TestID{{Pkg: "example.com/vm", Name: "TestSizeAscending"}}
	if diff := cmp.Diff(want, tm.TestsFor(own)); diff != "" {
		t.Errorf("want only vm's own test (-want +got):\n%s", diff)
	}

	// The profiles on disk hold the package's own lines; a dependency is
	// stored as the functions a test executed there, keys and not blocks.
	data, err := os.ReadFile(h.cacheFile("example.com/calc"))
	if err != nil {
		t.Fatalf("cannot read the cache file: %v", err)
	}
	var entry struct {
		Tests map[string]map[string]json.RawMessage `json:"tests"`
		Deps  map[string][]string                   `json:"deps"`
	}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("cannot decode the cache file: %v", err)
	}
	for name, profile := range entry.Tests {
		for file := range profile {
			if file != "calc/calc.go" {
				t.Errorf("%s: want only calc's own files in the profile, got %s", name, file)
			}
		}
	}
	if diff := cmp.Diff(map[string][]string{"TestDouble": {"vm/vm.go:Clamp"}}, entry.Deps); diff != "" {
		t.Errorf("want each test's dependency functions by key (-want +got):\n%s", diff)
	}
}

// A dependency's embedded files and test data are part of what it is built
// from, below its own directory. A change there alone leaves the package's
// fingerprint and every dependency's Go files as they were, which is exactly
// the case that would otherwise reuse the whole map.
func TestAChangeBelowADependencyReMapsAllOfIt(t *testing.T) {
	h := newCacheHarness(t)
	asset := filepath.Join(h.pkgRoot, "vm", "static", "limits.json")
	if err := os.MkdirAll(filepath.Dir(asset), 0o750); err != nil {
		t.Fatalf("cannot create the fixture directory: %v", err)
	}
	writeFixture(t, asset, `{"max": 10}`)

	h.buildCalc("")
	writeFixture(t, asset, `{"max": 20}`)
	h.buildCalc(changedCalc)

	want := []string{"TestDouble", "TestTriple"}
	if diff := cmp.Diff(want, h.testsRun()); diff != "" {
		t.Errorf("want the whole package re-mapped (-want +got):\n%s", diff)
	}
}

// With the package, its dependencies and the build environment all as they
// were, a moved build ID is left with nothing to say: Go folds the checkout
// path into it, and the path decides nothing a test executes.
func TestAMovedBuildIDAloneReMapsNothing(t *testing.T) {
	h := newCacheHarness(t)

	first := h.buildCalc("")
	second := h.buildCalc(changedCalc)

	if got := h.testsRun(); len(got) != 0 {
		t.Errorf("want nothing re-mapped, got %v", got)
	}
	if diff := cmp.Diff(first.Union(), second.Union(), blockOrder()); diff != "" {
		t.Errorf("the kept map covers different code (-want +got):\n%s", diff)
	}
}

// moveTo copies the fixture to another directory and maps from there from now
// on, which is what a CI runner restoring another runner's cache sees.
func (h *cacheHarness) moveTo() {
	h.t.Helper()

	dst := filepath.Join(h.t.TempDir(), "elsewhere")
	if err := os.CopyFS(dst, os.DirFS(h.pkgRoot)); err != nil {
		h.t.Fatalf("cannot copy the fixture: %v", err)
	}
	h.pkgRoot = dst
}

// A fleet of CI runners each checks the module out under its own directory, and
// the cache is only worth restoring if a map made in one of them is good in
// another. The checkout path reaches the build ID, and it used to reach the
// cache directory's name and every dependency's entry in Inputs as well.
func TestAMapIsReusedFromAnotherCheckout(t *testing.T) {
	testCases := map[string]struct {
		rootAtFixture bool
		workspace     bool
	}{
		// Its dependencies are named by their path inside the module.
		"an ordinary checkout": {rootAtFixture: true},
		// Its dependencies are outside the module root, like a replace target
		// or a go.work member, and are named by import path.
		"dependencies outside the module root": {},
		// GOWORK is an absolute path, different in every checkout.
		"a checkout with a workspace": {rootAtFixture: true, workspace: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			h := newCacheHarness(t)
			h.rootAtFixture = tc.rootAtFixture
			if tc.workspace {
				h.workFile()
			}

			first := h.buildCalc("")
			h.moveTo()
			if tc.workspace {
				h.workFile()
			}
			second := h.buildCalc("example.com/calc=built-in-another-directory")

			if got := h.testsRun(); len(got) != 0 {
				t.Errorf("want nothing re-mapped from another checkout, got %v", got)
			}
			if diff := cmp.Diff(first.Union(), second.Union(), blockOrder()); diff != "" {
				t.Errorf("the reused map covers different code (-want +got):\n%s", diff)
			}
		})
	}
}

// buildEnvironmentChanges are the changes that decide what a package's test
// binary is compiled to without a byte of source changing. Each must stop a
// map from being reused on its own; reused across one, an amd64 map would
// answer for an arm64 build, or a map made without a build tag for a build with
// it.
func buildEnvironmentChanges(t *testing.T) map[string]func(h *cacheHarness) {
	t.Helper()

	changes := map[string]func(h *cacheHarness){}
	for _, name := range []string{
		"GOOS", "GOARCH", "GOAMD64", "GOARM", "GOARM64", "GO386", "GOPPC64",
		"GORISCV64", "GOWASM", "GOMIPS", "GOMIPS64", "CGO_ENABLED",
		"GOEXPERIMENT", "GOFLAGS", "GOWORK", "GOFIPS140",
		"CC", "CXX", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_FFLAGS", "CGO_LDFLAGS",
	} {
		// Appended, so that GOWORK stays as workFile set it unless it is the
		// name being changed: the helper takes the last value it is given.
		changes[name] = func(h *cacheHarness) { h.goEnv += "\n" + name + "=changed-by-the-test" }
	}
	// A workspace file changes which module versions are built against.
	// Its path is the same on both runs; what it says is not.
	for _, file := range []string{"go.work", "go.work.sum"} {
		changes[file] = func(h *cacheHarness) {
			path := filepath.Join(filepath.Dir(h.workFile()), file)
			if err := os.WriteFile(path, []byte("changed by the test\n"), 0o600); err != nil {
				h.t.Fatalf("cannot write %s: %v", file, err)
			}
		}
	}

	return changes
}

// workFile puts a workspace file and its checksums beside the fixture and
// points GOWORK at it, the same on every run.
func (h *cacheHarness) workFile() string {
	h.t.Helper()

	path := filepath.Join(h.pkgRoot, "go.work")
	for name, content := range map[string]string{
		"go.work":     "go 1.25\n\nuse ./calc\n",
		"go.work.sum": "example.com/thing v1.0.0 h1:abc=\n",
	} {
		if err := os.WriteFile(filepath.Join(h.pkgRoot, name), []byte(content), 0o600); err != nil {
			h.t.Fatalf("cannot write %s: %v", name, err)
		}
	}
	h.goEnv = "GOWORK=" + path

	return path
}

func TestABuildEnvironmentChangeReMapsThePackage(t *testing.T) {
	for name, change := range buildEnvironmentChanges(t) {
		t.Run(name, func(t *testing.T) {
			// Alone, nothing else would stop the whole map being reused. With a
			// change to Double that would narrow to TestDouble on its own, the
			// environment is what says TestTriple is stale too.
			for _, withEdit := range []bool{false, true} {
				h := newCacheHarness(t)
				h.workFile()
				h.buildCalc("")

				change(h)
				if withEdit {
					h.edit("calc/calc.go", calcSourceDoubleGrown)
				}
				h.buildCalc(changedCalc)

				want := []string{"TestDouble", "TestTriple"}
				if diff := cmp.Diff(want, h.testsRun()); diff != "" {
					t.Errorf("edit %v: want the whole package re-mapped (-want +got):\n%s", withEdit, diff)
				}
			}
		})
	}
}

// A method added to a type is a global change, but changing one is not: it
// reaches only the tests that executed its lines.
func TestAChangedMethodReMapsOnlyTheTestsThatExecutedIt(t *testing.T) {
	h := newCacheHarness(t)

	withMethod := calcSource + `
type counter int

func (c counter) String() string {
	return "counter"
}
`
	h.edit("calc/calc.go", withMethod)
	h.buildCalc("")

	h.edit("calc/calc.go", calcSource+`
type counter int

func (c counter) String() string {
	return "a counter"
}
`)
	h.buildCalc(changedCalc)

	// The method sits below both functions, so no test's profile reaches it.
	if got := h.testsRun(); len(got) != 0 {
		t.Errorf("want nothing re-mapped for a method no test executed, got %v", got)
	}
}

// A block no package claims means some test's record of what it executed is
// incomplete. The mappings are still right and still used, but the next change
// cannot be narrowed from them, so it re-maps the package.
func TestAnUnattributableBlockReMapsTheNextChangeWhole(t *testing.T) {
	h := newCacheHarness(t)
	h.extraProfile = "example.com/calc/../gen/parser.go:1.1,2.2 1 1\n"

	first := h.buildCalc("")
	if first.Len() != 2 {
		t.Fatalf("want the package mapped all the same, got %d tests", first.Len())
	}
	// The block alone would stop its own mapping being shifted; the entry
	// saying nothing is what stops any mapping being narrowed from it.
	data, err := os.ReadFile(h.cacheFile("example.com/calc"))
	if err != nil {
		t.Fatalf("cannot read the cache file: %v", err)
	}
	var entry struct {
		Fingerprint struct {
			Whole string `json:"whole"`
		} `json:"fingerprint"`
	}
	if err := json.Unmarshal(data, &entry); err != nil || entry.Fingerprint.Whole != "" {
		t.Errorf("want the entry written without a fingerprint, got %q (%v)", entry.Fingerprint.Whole, err)
	}
	h.edit("calc/calc.go", calcSourceDoubleGrown)
	h.buildCalc(changedCalc)

	want := []string{"TestDouble", "TestTriple"}
	if diff := cmp.Diff(want, h.testsRun()); diff != "" {
		t.Errorf("want the whole package re-mapped (-want +got):\n%s", diff)
	}
}
