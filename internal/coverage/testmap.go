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

package coverage

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-gremlins/gremlins/internal/log"
)

// TestID identifies one top-level test function by the package that declares it
// and its name.
type TestID struct {
	Pkg  string
	Name string
}

// String returns the test in the "package.TestName" form used in reports.
func (t TestID) String() string {
	return t.Pkg + "." + t.Name
}

// TestMap records what each test in the module executed, anywhere in the module.
//
// Go's coverage profile does not attribute blocks to the test that executed
// them, so this cannot be read out of one profile: it is built by running each
// test on its own, with coverage over the whole module, and keeping the profile
// that run produced. That is why it is expensive, and why it is opt-in.
//
// The map is what makes test selection sound in both directions. Narrower: a
// test that never executes the mutated line cannot notice the mutation, so it
// does not need to run. Wider: a test in another package that does execute the
// line can notice it, and package scoping would never have run it.
type TestMap struct {
	profiles map[TestID]Profile
	mapped   map[string]struct{}
	elapsed  time.Duration
}

// Elapsed returns how long the map took to build.
func (t *TestMap) Elapsed() time.Duration {
	return t.elapsed
}

// Len returns the number of tests in the map.
func (t *TestMap) Len() int {
	return len(t.profiles)
}

// Mapped reports whether the tests of a package were mapped.
//
// A package that was not — its listing failed, or one of its tests produced no
// profile — cannot be selected from: what is missing from the map is exactly
// what would be silently skipped. Callers must fall back to running that
// package's whole suite, which is the behaviour without selection: never wrong,
// only slow.
func (t *TestMap) Mapped(pkg string) bool {
	_, ok := t.mapped[pkg]

	return ok
}

// TestsFor returns the tests that executed the given position, ordered by
// package and then name so that a report reads the same way twice.
func (t *TestMap) TestsFor(pos token.Position) []TestID {
	var found []TestID
	for id, profile := range t.profiles {
		if profile.IsCovered(pos) {
			found = append(found, id)
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Pkg != found[j].Pkg {
			return found[i].Pkg < found[j].Pkg
		}

		return found[i].Name < found[j].Name
	})

	return found
}

// Union returns a single Profile holding every block any test executed.
//
// It is a better answer to "is this line covered" than the profile from a plain
// coverage run, which attributes a line only to the package it lives in: a line
// executed solely by another package's tests reads as uncovered there, and the
// mutants on it are never tested at all.
func (t *TestMap) Union() Profile {
	profiles := make([]Profile, 0, len(t.profiles))
	for _, p := range t.profiles {
		profiles = append(profiles, p)
	}

	return Merge(profiles...)
}

// listPattern selects the test functions a plain `go test` would run: Test, and
// also Example and Fuzz, which -run executes too. Benchmarks are deliberately
// left out, because -run does not run them and so they cannot kill a mutant.
const listPattern = "^(Test|Example|Fuzz)"

var testNameRe = regexp.MustCompile(`^(?:Test|Example|Fuzz)[\p{L}\p{N}_]*$`)

// testPackage is a package of the module as the map builder sees it: where its
// source lives, and the test binary compiled for it.
type testPackage struct {
	importPath string
	dir        string
	hasTests   bool
	binary     string
}

// BuildTestMap runs every test in the module on its own and records what each
// one executed.
//
// The work is per package rather than per test: the test binary is compiled
// once with coverage over the module, then run once per test. Going through
// `go test` for each test instead costs a full go invocation every time —
// measured at ~400ms against ~8ms for a run of the compiled binary, so the
// invocation, not the test, was most of the map.
//
// A package whose tests cannot all be mapped is left out of the map rather than
// half-recorded, so that callers can tell "no test covers this" from "we did
// not look".
func (c *Coverage) BuildTestMap() (*TestMap, error) {
	start := time.Now()
	_ = os.Chdir(c.mod.Root)

	pkgs, err := c.listPackages()
	if err != nil {
		return nil, err
	}

	key := cacheKey(c.mapScope(), c.buildTags)
	path, pathErr := c.cachePath()
	cache := &mapCache{Version: cacheVersion, Key: key, Packages: map[string]cachedPackage{}}
	if pathErr == nil {
		cache = loadCache(path, key)
	}
	// The cache is rebuilt from what this run saw rather than updated in place,
	// so a package that has gone away does not keep its mapping alive forever.
	next := &mapCache{Version: cacheVersion, Key: key, Packages: map[string]cachedPackage{}}

	tm := &TestMap{
		profiles: make(map[TestID]Profile),
		mapped:   make(map[string]struct{}),
	}

	log.Infof("Mapping the tests of %d packages to the code they execute...\n", len(pkgs))

	done, reused := 0, 0
	for _, pkg := range pkgs {
		// A package with no test files is mapped by having nothing to map. That
		// is a complete answer, not a missing one, and saying so lets a mutant
		// there still be judged by covering tests in other packages.
		if !pkg.hasTests {
			tm.mapped[pkg.importPath] = struct{}{}

			continue
		}
		res := c.mapPackage(&pkg, tm, cache, next)
		done += res.tests
		if res.cached {
			reused += res.tests
		}
		if res.mapped {
			tm.mapped[pkg.importPath] = struct{}{}
		}
	}
	if pathErr == nil {
		if err := next.save(path); err != nil {
			log.Errorf("cannot write the test map cache: %v\n", err)
		}
	}
	tm.elapsed = time.Since(start)
	log.Infof("Mapped %d of %d tests in %s (%d reused from the cache)\n", tm.Len(), done, tm.elapsed, reused)

	return tm, nil
}

// mapResult says what became of one package: how many tests it had, whether
// they came from the cache, and whether the package can be selected from.
type mapResult struct {
	tests  int
	cached bool
	mapped bool
}

// mapPackage compiles a package's test binary once and runs each of its tests
// against it, unless the cache already holds a mapping made from a binary with
// the same build ID.
func (c *Coverage) mapPackage(pkg *testPackage, tm *TestMap, cache, next *mapCache) mapResult {
	binary, err := c.compileTests(pkg.importPath)
	if err != nil {
		log.Errorf("cannot compile the tests of %s, so it will run its whole suite: %v\n", pkg.importPath, err)

		return mapResult{}
	}
	pkg.binary = binary
	defer func() {
		_ = os.Remove(binary)
	}()

	// Without an identity for the binary the mapping can neither be trusted
	// from the cache nor written to it; it is still made, just not remembered.
	id, err := c.buildID(binary)
	if err != nil {
		log.Errorf("cannot identify the test binary of %s, so its mapping will not be cached: %v\n",
			pkg.importPath, err)
	}
	if id != "" {
		if entry, ok := cache.Packages[pkg.importPath]; ok && entry.BuildID == id {
			for name, profile := range entry.Tests {
				tm.profiles[TestID{Pkg: pkg.importPath, Name: name}] = profile
			}
			next.Packages[pkg.importPath] = entry

			return mapResult{tests: len(entry.Tests), cached: true, mapped: true}
		}
	}

	names, err := c.listTests(pkg)
	if err != nil {
		log.Errorf("cannot list the tests of %s, so it will run its whole suite: %v\n", pkg.importPath, err)

		return mapResult{}
	}

	complete := true
	mapped := make(map[string]Profile, len(names))
	for _, name := range names {
		profile, err := c.profileForTest(pkg, name)
		if err != nil {
			log.Errorf("cannot map %s.%s, so %s will run its whole suite: %v\n",
				pkg.importPath, name, pkg.importPath, err)
			complete = false

			continue
		}
		mapped[name] = profile
	}
	// A partial package is discarded rather than kept, so that "the map has no
	// test here" always means "no test covers this line", never "we did not
	// look". Selecting from half a package would silently skip the other half,
	// and caching half of it would make that permanent.
	if !complete {
		return mapResult{tests: len(names)}
	}
	for name, profile := range mapped {
		tm.profiles[TestID{Pkg: pkg.importPath, Name: name}] = profile
	}
	if id != "" {
		next.Packages[pkg.importPath] = cachedPackage{BuildID: id, Tests: mapped}
	}

	return mapResult{tests: len(names), mapped: true}
}

const wholeModule = "./..."

// mapScope is how much of the module the map has to cover.
//
// Without --cross-package a mutant is only ever judged by its own package's
// tests, so mapping anything outside the scanned path would be work nobody
// reads. With it, a test that kills a mutant can be anywhere, and a listing
// narrowed to the scanned path could not see it.
func (c *Coverage) mapScope() string {
	if c.crossPackage {
		return wholeModule
	}

	return c.scanPath()
}

// goListFormat asks for what the builder needs about every package: where it
// is, and whether it has tests of either kind.
const goListFormat = `{{.ImportPath}}	{{.Dir}}	{{len .TestGoFiles}}	{{len .XTestGoFiles}}`

func (c *Coverage) listPackages() ([]testPackage, error) {
	out, err := c.cmdContext("go", "list", "-f", goListFormat, c.mapScope()).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("impossible to list the packages of the module: %w\n%s", err, out)
	}

	return parsePackageList(string(out)), nil
}

// parsePackageList reads the tab-separated output of `go list -f goListFormat`.
// Anything that does not have the expected shape is skipped rather than
// guessed at: go writes build diagnostics to the same stream.
func parsePackageList(out string) []testPackage {
	var pkgs []testPackage
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			continue
		}
		internal, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		external, err := strconv.Atoi(fields[3])
		if err != nil {
			continue
		}
		pkgs = append(pkgs, testPackage{
			importPath: fields[0],
			dir:        fields[1],
			hasTests:   internal+external > 0,
		})
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].importPath < pkgs[j].importPath })

	return pkgs
}

// compileTests builds the package's test binary once, instrumented for coverage
// over the whole module. Every test of the package then runs against this one
// binary.
func (c *Coverage) compileTests(importPath string) (string, error) {
	binary := filepath.Join(c.workDir, strings.NewReplacer("/", "_", ".", "_").Replace(importPath)+".test")
	args := []string{"test", "-c", "-o", binary}
	if c.buildTags != "" {
		args = append(args, "-tags", c.buildTags)
	}
	args = append(args, "-coverpkg", c.testMapCoverPkg(importPath), importPath)

	if out, err := c.cmdContext("go", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w\n%s", err, out)
	}

	return binary, nil
}

// listTests asks the compiled binary which tests it holds, which is the same
// set `go test` would run and costs nothing extra to ask.
func (c *Coverage) listTests(pkg *testPackage) ([]string, error) {
	cmd := c.cmdContext(pkg.binary, "-test.list", listPattern)
	cmd.Dir = pkg.dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w\n%s", err, out)
	}

	return parseTestNames(string(out)), nil
}

// parseTestNames keeps the lines of `-test.list` output that are test names.
// The binary also writes diagnostics there — "warning: GOCOVERDIR not set" for
// one — and none of them can be mistaken for a name.
func parseTestNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if testNameRe.MatchString(line) {
			names = append(names, line)
		}
	}

	return names
}

// testTimeout matches the default `go test` applies, so that a test hanging
// during mapping fails the way it would have anyway rather than never
// returning.
const testTimeout = 10 * time.Minute

func (c *Coverage) profileForTest(pkg *testPackage, name string) (Profile, error) {
	// The binary runs in the package directory, as `go test` runs it, so a test
	// reading testdata still finds it. That makes the profile path have to be
	// absolute.
	file, err := filepath.Abs(filepath.Join(c.workDir, "testmap.cov"))
	if err != nil {
		return nil, err
	}

	cmd := c.cmdContext(pkg.binary,
		"-test.run", "^"+regexp.QuoteMeta(name)+"$",
		"-test.timeout", testTimeout.String(),
		"-test.coverprofile", file)
	cmd.Dir = pkg.dir

	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w\n%s", err, out)
	}

	f, err := os.Open(file) //nolint:gosec // G304: the path is Gremlins' own working directory
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	return c.parse(f)
}

// testMapCoverPkg is the coverage scope a package's tests are mapped under.
//
// With --cross-package it must be the whole module: the point is to see a test
// in one package executing a line in another. Without it, a test is only ever
// asked about its own package's code, and instrumenting the rest of the module
// would cost time to record coverage nothing will read.
func (c *Coverage) testMapCoverPkg(importPath string) string {
	if c.coverPkg != "" {
		return c.coverPkg
	}
	if c.crossPackage {
		return wholeModule
	}

	return importPath
}
