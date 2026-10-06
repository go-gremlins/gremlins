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
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
)

// cacheHarness runs the map builder repeatedly against one cache directory,
// recording which tests each run actually executed. That record is the point:
// a run served from the cache and a run that rebuilt produce the same map, and
// only the commands issued tell them apart.
type cacheHarness struct {
	t        *testing.T
	cacheDir string
	pkgRoot  string
	logPath  string
	// goEnv is what the fake go reports for the build environment, beyond its
	// defaults, as goEnvEnv carries it.
	goEnv string
	// rootAtFixture makes the fixture directory the module root, so that its
	// packages are inside the module rather than reached from outside it — the
	// shape of an ordinary checkout, as against a replace target or a go.work
	// member.
	rootAtFixture bool
	// notMain is a package the dependency listing reports as outside every
	// main module, as notMainEnv carries it.
	notMain string
	// extraProfile is appended to every profile, as extraProfileEnv carries
	// it.
	extraProfile string
	// env is added to every fake command's environment: the delays the
	// helper sleeps for, as compileDelayEnv and its siblings carry them.
	env []string
	// scope is the map scope a --diff run sets, as WithMapScope takes it;
	// nil sets none.
	scope []string
}

func newCacheHarness(t *testing.T) *cacheHarness {
	t.Helper()

	log.Init(&bytes.Buffer{}, &bytes.Buffer{})
	t.Cleanup(log.Reset)

	return &cacheHarness{
		t:        t,
		cacheDir: t.TempDir(),
		pkgRoot:  fixtureRoot(t),
		logPath:  filepath.Join(t.TempDir(), "invocations.log"),
	}
}

// build maps the module once. buildIDs decides what `go tool buildid` reports,
// as "pkg=id" pairs; an unlisted package keeps a stable default.
func (h *cacheHarness) build(helper, buildIDs string) *coverage.TestMap {
	h.t.Helper()

	return h.buildScoped(helper, buildIDs, "")
}

// buildScoped maps only one package, the way a run scoped to the package under
// edit does. An empty package maps the whole module.
func (h *cacheHarness) buildScoped(helper, buildIDs, pkg string) *coverage.TestMap {
	h.t.Helper()

	tm, err := h.coverage(helper, buildIDs, pkg).BuildTestMap()
	if err != nil {
		h.t.Fatalf("BuildTestMap() error: %v", err)
	}

	return tm
}

// coverage is the Coverage a build uses, for a test that goes on to ask it
// for more than the map. It clears the invocation log, so what the log holds
// afterwards is what this Coverage issued.
func (h *cacheHarness) coverage(helper, buildIDs, pkg string) *coverage.Coverage {
	h.t.Helper()

	_ = os.Remove(h.logPath)
	// A real run is scoped by the directory gremlins was pointed at, and that
	// directory is what used to divide the cache. Deriving it from the package
	// under test is what makes these cases scoped runs rather than whole-module
	// runs with a narrowed `go list`.
	callingDir := "."
	if suffix, ok := strings.CutPrefix(pkg, "example.com/"); ok {
		callingDir = suffix
	}
	mod := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: callingDir}
	if h.rootAtFixture {
		// The builder changes into the module root, and the suite's working
		// directory has to survive that.
		h.t.Chdir(h.pkgRoot)
		mod.Root = h.pkgRoot
	}
	fake := fakeGoCommandWith(helper, h.pkgRoot, buildIDs, h.logPath, pkg, h.goEnv, h.notMain, h.extraProfile)
	withEnv := func(command string, args ...string) *exec.Cmd {
		cmd := fake(command, args...)
		cmd.Env = append(cmd.Env, h.env...)

		return cmd
	}

	opts := []coverage.Option{coverage.WithTestMapCacheDir(h.cacheDir), coverage.WithStubTypes()}
	if h.scope != nil {
		opts = append(opts, coverage.WithMapScope(h.scope))
	}

	return coverage.NewWithCmd(withEnv, h.t.TempDir(), mod, opts...)
}

// testsRun returns the tests the last build actually executed, sorted.
func (h *cacheHarness) testsRun() []string {
	h.t.Helper()

	data, err := os.ReadFile(h.logPath)
	if err != nil {
		return nil
	}
	var run []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if name, ok := strings.CutPrefix(line, "run "); ok {
			run = append(run, name)
		}
	}
	sort.Strings(run)

	return run
}

// cacheFile is the file holding one package's mapping. It is found by reading
// the files rather than by recomputing the name, so that the test does not
// restate the layout it is checking.
func (h *cacheHarness) cacheFile(importPath string) string {
	h.t.Helper()

	for _, path := range h.cacheFiles() {
		var entry struct {
			ImportPath string `json:"import_path"`
		}
		// #nosec G304 - the path comes from a directory this test created
		data, err := os.ReadFile(path)
		if err == nil && json.Unmarshal(data, &entry) == nil && entry.ImportPath == importPath {
			return path
		}
	}
	h.t.Fatalf("no cache file was written for %s", importPath)

	return ""
}

// cacheFiles lists the mapping files on disk, which is one per mapped package.
func (h *cacheHarness) cacheFiles() []string {
	h.t.Helper()

	var found []string
	_ = filepath.Walk(h.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".json") {
			found = append(found, path)
		}

		return nil
	})
	sort.Strings(found)

	return found
}

// blockOrder makes a union comparable. Merge keeps every distinct block but not
// a fixed order, because it reads profiles out of a map.
func blockOrder() cmp.Option {
	return cmpopts.SortSlices(func(a, b coverage.Block) bool {
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}

		return a.StartCol < b.StartCol
	})
}

func allTests() []string {
	return []string{
		"TestDouble", "TestRangeAscending", "TestRangeDescending",
		"TestSizeAscending", "TestTriple",
	}
}

func TestMapCacheReusesUnchangedPackages(t *testing.T) {
	h := newCacheHarness(t)

	first := h.build("TestTestMapHelperProcess", "")
	if diff := cmp.Diff(allTests(), h.testsRun()); diff != "" {
		t.Fatalf("the first build should run every test (-want +got):\n%s", diff)
	}

	second := h.build("TestTestMapHelperProcess", "")

	// Nothing changed, so nothing needed running — and the map is the same map.
	if got := h.testsRun(); len(got) != 0 {
		t.Errorf("want no test executed on the second build, got %v", got)
	}
	if diff := cmp.Diff(first.Union(), second.Union(), blockOrder()); diff != "" {
		t.Errorf("the cached map covers different code (-want +got):\n%s", diff)
	}
	if first.Len() != second.Len() {
		t.Errorf("want %d tests from the cache, got %d", first.Len(), second.Len())
	}
	if !second.Mapped("example.com") || !second.Mapped("example.com/vm") {
		t.Error("a package served from the cache must still report as mapped")
	}
}

// The build ID is computed over the package's own source AND every dependency's,
// so a change anywhere beneath a package invalidates exactly the binaries that
// link it — which is what this stands for.
func TestMapCacheRemapsOnlyThePackageWhoseBuildIDChanged(t *testing.T) {
	h := newCacheHarness(t)

	h.build("TestTestMapHelperProcess", "")
	// A change vm's own mapping cannot narrow across — an init runs for every
	// test — so that what is re-mapped is decided by the build IDs alone. The
	// packages built on vm keep theirs here, standing for packages the change
	// did not reach.
	h.edit("vm/vm.go", vmSource+"\nfunc init() {}\n")
	h.build("TestTestMapHelperProcess", "example.com/vm=changed-by-an-edit")

	want := []string{"TestSizeAscending"}
	if diff := cmp.Diff(want, h.testsRun()); diff != "" {
		t.Errorf("only the changed package should be re-mapped (-want +got):\n%s", diff)
	}
}

func TestMapCacheDoesNotKeepAPackageItCouldNotMapWhole(t *testing.T) {
	h := newCacheHarness(t)

	first := h.build("TestTestMapHelperProcessFailingTest", "")
	if first.Mapped("example.com") {
		t.Fatal("the package with an unrunnable test should not be mapped")
	}

	// Caching a half-mapped package would make the gap permanent, so the next
	// run has to try it again.
	h.build("TestTestMapHelperProcessFailingTest", "")

	if got := h.testsRun(); !contains(got, "TestRangeDescending") {
		t.Errorf("want the unmappable package retried, got %v", got)
	}
}

func TestMapCacheRebuildsWhenTheFileIsUnusable(t *testing.T) {
	testCases := map[string]func(path string) error{
		"corrupt json": func(path string) error {
			return os.WriteFile(path, []byte("{not json"), 0o600)
		},
		"a cache written by another version": func(path string) error {
			return os.WriteFile(path, []byte(`{"version":999,"import_path":"example.com",`+
				`"build_id":"x","tests":{}}`), 0o600)
		},
		"a cache file naming another package": func(path string) error {
			return os.WriteFile(path, []byte(`{"version":`+strconv.Itoa(coverage.CacheVersion)+
				`,"import_path":"example.com/elsewhere","build_id":"x","tests":{}}`), 0o600)
		},
		"no cache file at all": os.Remove,
	}

	for name, corrupt := range testCases {
		t.Run(name, func(t *testing.T) {
			h := newCacheHarness(t)
			h.build("TestTestMapHelperProcess", "")

			if err := corrupt(h.cacheFile("example.com")); err != nil {
				t.Fatalf("cannot set up the case: %v", err)
			}

			tm := h.build("TestTestMapHelperProcess", "")

			// A file that cannot be read costs a re-map of its package, never a
			// wrong map and never a failed run. The other package still hits.
			want := []string{"TestRangeAscending", "TestRangeDescending"}
			if diff := cmp.Diff(want, h.testsRun()); diff != "" {
				t.Errorf("want the unusable package re-mapped (-want +got):\n%s", diff)
			}
			if tm.Len() != 5 {
				t.Errorf("want the full map after the rebuild, got %d tests", tm.Len())
			}
		})
	}
}

func TestMapCacheSurvivesTheRoundTripExactly(t *testing.T) {
	h := newCacheHarness(t)

	first := h.build("TestTestMapHelperProcess", "")
	second := h.build("TestTestMapHelperProcess", "")

	pos := token.Position{Filename: "vm/vm.go", Line: vmOwnLine, Column: 3}
	if len(first.TestsFor(pos)) == 0 {
		t.Fatal("want a line some test executes, or the comparison below is of nothing")
	}
	if diff := cmp.Diff(first.TestsFor(pos), second.TestsFor(pos)); diff != "" {
		t.Errorf("the cached map answers differently (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(first.Union(), second.Union(), blockOrder()); diff != "" {
		t.Errorf("the cached union differs (-want +got):\n%s", diff)
	}
}

// The defect this layout exists to remove: a run scoped to one package used to
// rewrite the module's single cache file from what it had seen, discarding
// every package it never looked at.
func TestAScopedRunDoesNotEvictTheRestOfTheModule(t *testing.T) {
	h := newCacheHarness(t)

	h.build("TestTestMapHelperProcess", "")
	if got := h.cacheFiles(); len(got) != 3 {
		t.Fatalf("want one file per package with tests, got %d", len(got))
	}

	scoped := h.buildScoped("TestTestMapHelperProcess", "", "example.com/vm")
	if got := scoped.Len(); got != 1 {
		t.Errorf("want the scoped run to map its one package, got %d tests", got)
	}
	if got := h.cacheFiles(); len(got) != 3 {
		t.Errorf("want the other packages' files left alone, got %d files", len(got))
	}

	// The whole-module run that follows is the one that pays for eviction, so
	// it is the one that proves there was none.
	h.build("TestTestMapHelperProcess", "")
	if got := h.testsRun(); len(got) != 0 {
		t.Errorf("want every package still served from the cache, got %v re-run", got)
	}
}

// The case CI is built on: main maps the module, a pull request mutates one
// package, and the mapping it needs is already there.
//
// The scan path used to be part of the cache key, so a scoped run looked in a
// directory the whole-module run had never written to and re-mapped from
// nothing. That failure only ever cost time, which is why it needed a test: a
// cache that silently never hits looks exactly like one that does.
func TestAScopedRunReadsTheWholeModuleMap(t *testing.T) {
	h := newCacheHarness(t)

	h.build("TestTestMapHelperProcess", "")

	scoped := h.buildScoped("TestTestMapHelperProcess", "", "example.com/vm")
	if got := h.testsRun(); len(got) != 0 {
		t.Errorf("want the scoped run served from the whole-module map, got %v re-run", got)
	}
	if got := scoped.Len(); got != 1 {
		t.Errorf("want the scoped package's tests, got %d", got)
	}
	// A second copy under another key is the shape of the defect: the mappings
	// would be right, and paid for twice.
	if got := h.cacheFiles(); len(got) != 3 {
		t.Errorf("want no second copy of the map, got %d files", len(got))
	}

	// Reading the file is not the same as being able to use it, and the
	// difference is invisible from everything above: a map that answers "no test
	// reaches this line" for every line still loads, still reports its tests, and
	// still runs nothing — the selector simply falls back to whole suites, which
	// is the cost the cache existed to avoid. A scoped run's mutants are located
	// relative to the directory it was pointed at, so this is the name it asks
	// about.
	pos := token.Position{Filename: "vm.go", Line: vmOwnLine, Column: 3}
	if got := scoped.TestsFor(pos); len(got) == 0 {
		t.Error("the restored map answers about no test at the scoped run's own paths")
	}
}

// Two runs scoped to different packages of one module used to share a file, so
// the second save won and the first run's work was lost.
func TestTwoScopedRunsDoNotClobberEachOther(t *testing.T) {
	h := newCacheHarness(t)

	h.buildScoped("TestTestMapHelperProcess", "", "example.com")
	h.buildScoped("TestTestMapHelperProcess", "", "example.com/vm")
	h.buildScoped("TestTestMapHelperProcess", "", "example.com/calc")

	h.build("TestTestMapHelperProcess", "")
	if got := h.testsRun(); len(got) != 0 {
		t.Errorf("want both scoped runs' work kept, got %v re-run", got)
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}

	return false
}

// A profile that cannot be read is a test that could not be mapped, and so a
// package that runs its whole suite rather than one selected from half a map.
func TestAnUnreadableProfileLeavesThePackageUnmapped(t *testing.T) {
	h := newCacheHarness(t)
	h.extraProfile = "not a profile line\n"

	tm := h.buildCalc("")
	if tm.Mapped("example.com/calc") {
		t.Error("want the package unmapped when its profiles cannot be read")
	}
}
