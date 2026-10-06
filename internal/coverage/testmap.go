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
	"bytes"
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/tools/cover"

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

	// durations is how long each test's own mapping run took, measured when
	// the mapping was made — this run, or the run that wrote the cache entry
	// it was reused from. compiled is how long each mapped package's test
	// binary took to compile in THIS run, which every run pays. withTests is
	// every package the builder set out to map that has test files, mapped or
	// not: the packages in scope, and so the only ones a run can need coverage
	// of. Together they
	// are what SuiteBaseline times the scope from.
	durations map[TestID]time.Duration
	compiled  map[string]time.Duration
	withTests []string

	// callingDir is what a caller's positions are relative to. The profiles are
	// relative to the module root, so that a mapping means the same thing in a
	// scoped run and a whole-module one — and so that one can read the other's
	// cache.
	callingDir string
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
	pos = ProfilePosition(t.callingDir, pos)
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

	// An unusable cache directory is not a reason to stop: the map is still
	// built, just not remembered. An empty path says so to mapPackage.
	cacheDir, err := c.cacheDirPath(cacheKey(c.cacheScope(), c.buildTags))
	if err != nil {
		log.Errorf("cannot locate the test map cache, so this run will not use one: %v\n", err)
		cacheDir = ""
	}

	tm := &TestMap{
		profiles:   make(map[TestID]Profile),
		mapped:     make(map[string]struct{}),
		durations:  make(map[TestID]time.Duration),
		compiled:   make(map[string]time.Duration),
		callingDir: c.mod.CallingDir,
	}

	if c.scope != nil {
		pkgs = c.scopePackages(pkgs)
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
		tm.withTests = append(tm.withTests, pkg.importPath)
		res := c.mapPackage(&pkg, tm, cacheDir)
		done += res.tests
		reused += res.reused
		if res.mapped {
			tm.mapped[pkg.importPath] = struct{}{}
		}
	}
	tm.elapsed = time.Since(start)
	log.Infof("Mapped %d of %d tests in %s (%d reused from the cache)\n", tm.Len(), done, tm.elapsed, reused)

	return tm, nil
}

// scopePackages keeps the listed packages the map scope names. A scoped
// package the listing lacks matches nothing either way, but it means the
// diff's paths and the listing disagree on what a package is, so it is named.
func (c *Coverage) scopePackages(pkgs []testPackage) []testPackage {
	want := make(map[string]bool, len(c.scope))
	for _, p := range c.scope {
		want[p] = true
	}
	var scoped []testPackage
	for _, pkg := range pkgs {
		if want[pkg.importPath] {
			scoped = append(scoped, pkg)
			delete(want, pkg.importPath)
		}
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for p := range want {
			missing = append(missing, p)
		}
		sort.Strings(missing)
		log.Infof("The diff names packages the module listing lacks, so none of them is mapped: %s\n", strings.Join(missing, ", "))
	}
	log.Infof("Mapping %d of %d packages: the rest hold no mutant in scope\n", len(scoped), len(pkgs))

	return scoped
}

// serveCached puts a cached package's mappings in the map as they stand. It
// writes nothing back: the file is already the answer, and a run touches no
// other package's, so it cannot evict one. (mapPackage records a compile time
// an older entry lacked, in that package's own file.) compiled is the compile
// time the package reports to the suite baseline: this run's, or, when the
// compile was skipped, the one the entry recorded.
func (c *Coverage) serveCached(tm *TestMap, importPath string, cached cachedPackage, compiled time.Duration) {
	for name, profile := range cached.Tests {
		id := TestID{Pkg: importPath, Name: name}
		tm.profiles[id] = profile
		if d, ok := cached.Durations[name]; ok {
			tm.durations[id] = d
		}
	}
	tm.compiled[importPath] = compiled
}

// unchanged reports whether a package can be served from its entry without
// compiling: now is a usable fingerprint equal to the one the entry was mapped
// under, and the entry carries everything a hit reports, a compile time and a
// duration for every test. An entry written before compile times were kept, or
// under a fingerprint that said nothing, is compiled as before; so is a package
// the type-checker could not read, which reusable would re-map whole rather
// than compare (see wholeReason).
func unchanged(cached cachedPackage, now fingerprint) bool {
	if cached.Compiled <= 0 || now.Whole == "" || now.Inputs == "" || !now.Typed {
		return false
	}
	for name := range cached.Tests {
		if _, ok := cached.Durations[name]; !ok {
			return false
		}
	}
	was, err := json.Marshal(cached.Fingerprint)
	if err != nil {
		return false
	}
	is, err := json.Marshal(now)

	return err == nil && bytes.Equal(was, is)
}

// mapResult says what became of one package: how many tests it had, how many of
// their mappings came from the cache, and whether the package can be selected
// from.
type mapResult struct {
	tests  int
	reused int
	mapped bool
}

// mapPackage maps a package's tests, from its cache entry where it can. A
// package whose fingerprint equals its entry's is served without compiling
// (see unchanged). Otherwise its test binary is compiled once: an entry made
// from a binary with the same build ID is served, and any other mapping is
// kept, shifted or remade test by test (see reusable) by running each test
// against the binary.
//
// cacheDir is empty when the cache is unusable, in which case the mapping is
// still made and simply not remembered.
func (c *Coverage) mapPackage(pkg *testPackage, tm *TestMap, cacheDir string) mapResult {
	var cached cachedPackage
	hit := false
	if cacheDir != "" {
		cached, hit = loadCachedPackage(cacheDir, pkg.importPath)
	}
	// The fingerprint is taken before the compile, because an unchanged one
	// makes the compile unnecessary: with the fingerprint and the build inputs
	// both equal to the entry's, the build ID the compile would yield could
	// only say that the checkout moved (see buildInputsOf), and the entry is
	// what a compile and a fresh map would produce again, line for line.
	n := c.reusableFrom(pkg, cached, hit)
	if hit && unchanged(cached, n.fp) {
		c.serveCached(tm, pkg.importPath, cached, cached.Compiled)

		return mapResult{tests: len(cached.Tests), reused: len(cached.Tests), mapped: true}
	}

	compileStart := time.Now()
	binary, err := c.compileTests(pkg)
	compiled := time.Since(compileStart)
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
		// Without an identity the entry can neither be trusted nor replaced.
		hit, n.reuse = false, nil
	}
	if hit && cached.BuildID == id {
		c.serveCached(tm, pkg.importPath, cached, compiled)
		// An entry from before compile times were kept is otherwise compiled
		// on every run: record this run's, so the next one can skip it. It is
		// the package's own file, rewritten with what it already says.
		if cached.Compiled <= 0 && cached.Fingerprint.Whole != "" {
			cached.Compiled = compiled
			if err := cached.save(cacheDir); err != nil {
				log.Errorf("cannot write the test map cache for %s: %v\n", pkg.importPath, err)
			}
		}

		return mapResult{tests: len(cached.Tests), reused: len(cached.Tests), mapped: true}
	}

	names, err := c.listTests(pkg)
	if err != nil {
		log.Errorf("cannot list the tests of %s, so it will run its whole suite: %v\n", pkg.importPath, err)

		return mapResult{}
	}

	complete, attributable, reused := true, true, 0
	mapped := make(map[string]Profile, len(names))
	durations := make(map[string]time.Duration, len(names))
	deps := map[string][]string{}
	for _, name := range names {
		// A mapping is reused with the duration it was recorded with, since
		// this run never times the test; one recorded without a duration is
		// re-made rather than reused, so the entry written below has one for
		// every test.
		recorded, timed := cached.Durations[name]
		if profile, keep := n.reuse[name]; keep && timed {
			mapped[name] = profile
			durations[name] = recorded
			if keys := cached.Deps[name]; len(keys) > 0 {
				deps[name] = keys
			}
			reused++

			continue
		}
		runStart := time.Now()
		got, err := c.profileForTest(pkg, name, n.sources)
		ran := time.Since(runStart)
		if err != nil {
			log.Errorf("cannot map %s.%s, so %s will run its whole suite: %v\n",
				pkg.importPath, name, pkg.importPath, err)
			complete = false

			continue
		}
		mapped[name] = got.profile
		durations[name] = ran
		if len(got.deps) > 0 {
			deps[name] = got.deps
		}
		attributable = attributable && got.attributable
	}
	// A partial package is discarded rather than kept, so that "the map has no
	// test here" always means "no test covers this line", never "we did not
	// look". Selecting from half a package would silently skip the other half,
	// and caching half of it would make that permanent.
	if !complete {
		return mapResult{tests: len(names)}
	}
	for name, profile := range mapped {
		id := TestID{Pkg: pkg.importPath, Name: name}
		tm.profiles[id] = profile
		tm.durations[id] = durations[name]
	}
	tm.compiled[pkg.importPath] = compiled
	// A block that could not be placed means some test's record of what it
	// executed is incomplete, and narrowing from it next time could keep a
	// mapping the change reached. The mappings themselves are still right, so
	// they are kept — under a fingerprint that says nothing, which costs the
	// next run with a moved build ID a whole re-map and nothing worse.
	fp := n.fp
	if !attributable {
		log.Errorf("cannot attribute everything the tests of %s executed, so its next change will re-map all of it\n",
			pkg.importPath)
		fp = fingerprint{}
	}
	if id != "" && cacheDir != "" {
		entry := cachedPackage{
			Version: cacheVersion, ImportPath: pkg.importPath, BuildID: id,
			Fingerprint: fp, Tests: mapped, Deps: deps, Durations: durations, Compiled: compiled,
		}
		if err := entry.save(cacheDir); err != nil {
			log.Errorf("cannot write the test map cache for %s: %v\n", pkg.importPath, err)
		}
	}

	return mapResult{tests: len(names), reused: reused, mapped: true}
}

// narrowing is what a package's mapping run starts from: the fingerprint the
// next run will compare against, the mappings the change since left valid, and
// the instrumented dependencies a new mapping's blocks are attributed to.
type narrowing struct {
	fp      fingerprint
	reuse   map[string]Profile
	sources map[string]*depSource
}

// reusableFrom takes the package's current fingerprint and works out which of
// its cached mappings the change since survived.
//
// The fingerprint is returned whether or not anything was reused, because it is
// what the next run will compare against; an empty one says the package could
// not be read, and costs that run a whole re-map.
//
// Narrowing is off under --cross-package: a profile then covers lines in
// packages this fingerprint says nothing about, so "no changed line falls in
// this profile" would be a claim about only part of it.
func (c *Coverage) reusableFrom(pkg *testPackage, cached cachedPackage, hit bool) narrowing {
	if c.crossPackage {
		return narrowing{}
	}
	// The instrumented dependencies are type-checked in one load, before
	// reading them asks for each.
	c.loadTypes(false, c.instrumentedImports(pkg))
	// Read first and returned however the rest goes: every new mapping needs
	// them to keep a dependency's blocks out of its profile.
	sources, depsOK := c.dependencySources(pkg)
	fp, ok := c.fingerprintOf(pkg)
	if !ok {
		log.Errorf("cannot read the sources of %s, so its whole map will be rebuilt\n", pkg.importPath)

		return narrowing{sources: sources}
	}
	// The dependencies whose lines the binary records are fingerprinted
	// beside the package; everything else it is built from goes into Inputs.
	// Without both, a moved build ID cannot be told apart from a moved
	// dependency, so a fingerprint that lacks either must not be narrowed from.
	if !depsOK {
		log.Errorf("cannot read the dependencies of %s, so its whole map will be rebuilt\n", pkg.importPath)

		return narrowing{sources: sources}
	}
	fp.Deps = make(map[string]pkgPrint, len(sources))
	for importPath, src := range sources {
		fp.Deps[importPath] = src.stored
	}
	fp.Inputs, ok = c.buildInputsOf(pkg)
	if !ok {
		log.Errorf("cannot identify what %s is built from, so its whole map will be rebuilt\n", pkg.importPath)

		return narrowing{sources: sources}
	}
	n := narrowing{fp: fp, sources: sources}
	if !hit {
		return n
	}
	reuse, why := reusable(cached, fp)
	if why != "" {
		log.Infof("testmap: re-mapping all of %s: %s\n", pkg.importPath, why)

		return n
	}
	n.reuse = reuse

	return n
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
	out, err := c.cmdContext("go", c.listArgs("-f", goListFormat, c.mapScope())...).CombinedOutput()
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

// compileTests builds the package's test binary once, instrumented for
// coverage over the package and its in-module dependencies, or over the whole
// module under --cross-package. Every test of the package then runs against
// this one binary.
func (c *Coverage) compileTests(pkg *testPackage) (string, error) {
	binary := filepath.Join(c.workDir, strings.NewReplacer("/", "_", ".", "_").Replace(pkg.importPath)+".test")
	args := append([]string{"test", "-c", "-o", binary}, c.testBuildFlags(pkg)...)
	args = append(args, pkg.importPath)

	if out, err := c.cmdContext("go", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w\n%s", err, out)
	}

	return binary, nil
}

// testBuildFlags are the flags a package's test binary is compiled with for
// mapping, besides where to write it. They decide what the binary is, so they
// are folded into the package's Inputs too, and both read them from here so
// that the two cannot drift apart.
func (c *Coverage) testBuildFlags(pkg *testPackage) []string {
	var flags []string
	if c.buildTags != "" {
		flags = append(flags, "-tags", c.buildTags)
	}

	return append(flags, "-coverpkg", c.mappingCoverPkg(pkg))
}

// mappingCoverPkg is the -coverpkg a package's test binary is compiled with:
// testMapCoverPkg's scope, plus — without --cross-package — every dependency
// the binary instruments (see dependencyDirs).
//
// The dependencies are there for narrowing only. What a test executes in one
// is recorded as the entities it reached and kept out of its profile (see
// splitCoverage), so the scope a mapping MEANS is still the package alone, and
// cacheScope still says so.
//
// The list is in Inputs too, through testBuildFlags, so a dependency added to
// or dropped from what the binary links re-maps the package. A listing that
// fails instruments the package alone, and the fingerprint fails with it.
func (c *Coverage) mappingCoverPkg(pkg *testPackage) string {
	scope := c.testMapCoverPkg(pkg.importPath)
	if c.crossPackage {
		return scope
	}
	deps := c.instrumentedImports(pkg)
	sort.Strings(deps)
	paths := append([]string{scope}, deps...)

	return strings.Join(paths, ",")
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

// profileForTest runs one test and records what it executed: its own
// package's lines, and the dependency entities it reached (see
// splitCoverage).
func (c *Coverage) profileForTest(pkg *testPackage, name string, sources map[string]*depSource) (testCoverage, error) {
	// The binary runs in the package directory, as `go test` runs it, so a test
	// reading testdata still finds it. That makes the profile path have to be
	// absolute.
	file, err := filepath.Abs(filepath.Join(c.workDir, "testmap.cov"))
	if err != nil {
		return testCoverage{}, err
	}

	cmd := c.cmdContext(pkg.binary,
		"-test.run", "^"+regexp.QuoteMeta(name)+"$",
		"-test.timeout", testTimeout.String(),
		"-test.coverprofile", file)
	cmd.Dir = pkg.dir

	if out, err := cmd.CombinedOutput(); err != nil {
		return testCoverage{}, fmt.Errorf("%w\n%s", err, out)
	}

	f, err := os.Open(file) //nolint:gosec // G304: the path is Gremlins' own working directory
	if err != nil {
		return testCoverage{}, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	profiles, err := cover.ParseProfilesFromReader(f)
	if err != nil {
		return testCoverage{}, err
	}

	return c.splitCoverage(pkg, profiles, sources), nil
}

// testMapCoverPkg is the coverage scope a package's tests are mapped under.
//
// With --cross-package it must be the whole module: the point is to see a test
// in one package executing a line in another. Without it, a test is only ever
// asked about its own package's code, and that is the scope a mapping means:
// the in-module dependencies mappingCoverPkg adds are instrumented only to be
// narrowed across, and never read as coverage.
//
// --cross-package decides this, NOT the configured --coverpkg, and the order of
// those two checks is the whole of this function. The configured coverpkg is
// the scope of the coverage GATHER, which is a different question: it decides
// which lines the profile attributes to anybody, and so which mutants are
// runnable. It is common and correct to gather over ./... while judging a
// mutant by its own package's tests, and reading the gather's scope here made
// that combination instrument every test binary against the whole module.
//
// The cost of getting it backwards is not the wasted instrumentation the
// comment above describes. It is the cache: an entry is keyed by
// `go tool buildid` of the test binary, so a binary built with -coverpkg ./...
// depends on every package in the module and one edit anywhere invalidates all
// of them at once. Measured on a 15-package module with coverpkg ./... set, a
// one-line addition to the smallest leaf package re-mapped 394 of 394 tests and
// cost 636s against 77s for a fully cached run — within 2% of a cold build. The
// per-package cache was doing nothing.
func (c *Coverage) testMapCoverPkg(importPath string) string {
	if !c.crossPackage {
		return importPath
	}
	if c.coverPkg != "" {
		return c.coverPkg
	}

	return wholeModule
}
