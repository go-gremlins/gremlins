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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/go-gremlins/gremlins/internal/gomodule"
)

func TestParsePackageList(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		out  string
		want []testPackage
	}{
		"reads the import path, the directory and whether there are tests": {
			out: "example.com\t/src\t2\t0\nexample.com/vm\t/src/vm\t0\t1\n",
			want: []testPackage{
				{importPath: "example.com", dir: "/src", hasTests: true},
				{importPath: "example.com/vm", dir: "/src/vm", hasTests: true},
			},
		},
		// A package with no test files is not an error and not a gap: it has
		// nothing to map, which is a complete answer.
		"a package with no test files of either kind has no tests": {
			out:  "example.com/empty\t/src/empty\t0\t0\n",
			want: []testPackage{{importPath: "example.com/empty", dir: "/src/empty", hasTests: false}},
		},
		"packages come back in a stable order": {
			out: "example.com/vm\t/src/vm\t1\t0\nexample.com\t/src\t1\t0\n",
			want: []testPackage{
				{importPath: "example.com", dir: "/src", hasTests: true},
				{importPath: "example.com/vm", dir: "/src/vm", hasTests: true},
			},
		},
		// go writes build diagnostics to the same stream, and none of them have
		// the shape of a package line.
		"anything that is not a package line is skipped": {
			out: "go: downloading example.com v1.0.0\nexample.com\t/src\t1\t0\n" +
				"# example.com/broken\nexample.com/x\t/src/x\tnot-a-number\t0\n",
			want: []testPackage{{importPath: "example.com", dir: "/src", hasTests: true}},
		},
		"no output at all yields no packages": {
			out:  "",
			want: nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := parsePackageList(tc.out)
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(testPackage{})); diff != "" {
				t.Errorf("parsePackageList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseTestNames(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		out  string
		want []string
	}{
		"keeps the names the binary listed": {
			out:  "TestOne\nExampleTwo\nFuzzThree\n",
			want: []string{"TestOne", "ExampleTwo", "FuzzThree"},
		},
		// The binary writes this to the same stream when it is coverage-built
		// but not given a GOCOVERDIR, which is exactly how it is listed here.
		"drops the GOCOVERDIR warning the binary prints while listing": {
			out:  "TestOne\nwarning: GOCOVERDIR not set, no coverage data emitted\n",
			want: []string{"TestOne"},
		},
		"drops anything else that is not a test name": {
			out:  "BenchmarkOne\nTestOne\n--- FAIL: something\n",
			want: []string{"TestOne"},
		},
		"no output at all yields no names": {
			out:  "",
			want: nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tc.want, parseTestNames(tc.out)); diff != "" {
				t.Errorf("parseTestNames() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTestMapCoverPkg(t *testing.T) {
	t.Parallel()

	const pkg = "example.com/internal/vm"

	// Without --cross-package a test is only ever asked about its own package's
	// code, so instrumenting the rest of the module would record coverage that
	// nothing reads.
	t.Run("covers only the package being mapped", func(t *testing.T) {
		t.Parallel()

		c := &Coverage{}
		if got := c.testMapCoverPkg(pkg); got != pkg {
			t.Errorf("want %s, got %s", pkg, got)
		}
	})

	// With it, the point is to see a test in one package executing a line in
	// another, which only whole-module instrumentation records.
	t.Run("covers the whole module for cross-package", func(t *testing.T) {
		t.Parallel()

		c := &Coverage{crossPackage: true}
		if got := c.testMapCoverPkg(pkg); got != wholeModule {
			t.Errorf("want %s, got %s", wholeModule, got)
		}
	})

	// A configured --coverpkg is the scope of the coverage GATHER, and it applies
	// here only where the two questions coincide: under --cross-package, where
	// the map has to see outside the package anyway. Gathering over ./... while
	// judging a mutant by its own package's tests is a normal and correct
	// combination, and it used to instrument every test binary against the whole
	// module — which made every binary's build ID move whenever any file in the
	// module changed, so the per-package cache never hit.
	t.Run("a configured cover-pkg applies only under cross-package", func(t *testing.T) {
		t.Parallel()

		withCross := &Coverage{coverPkg: "./internal/...", crossPackage: true}
		if got := withCross.testMapCoverPkg(pkg); got != "./internal/..." {
			t.Errorf("with --cross-package: want ./internal/..., got %s", got)
		}

		withoutCross := &Coverage{coverPkg: "./internal/..."}
		if got := withoutCross.testMapCoverPkg(pkg); got != pkg {
			t.Errorf("without --cross-package: want %s, got %s", pkg, got)
		}
	})
}

func TestMapScope(t *testing.T) {
	t.Parallel()

	mod := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "internal/vm"}

	t.Run("maps only the scanned path when mutants stay in their package", func(t *testing.T) {
		t.Parallel()

		c := &Coverage{mod: mod}
		if got := c.mapScope(); got != "./internal/vm/..." {
			t.Errorf("want ./internal/vm/..., got %s", got)
		}
	})

	// A test that kills a cross-package mutant can be anywhere, so a listing
	// narrowed to the scanned path could not see it.
	t.Run("maps the whole module for cross-package", func(t *testing.T) {
		t.Parallel()

		c := &Coverage{mod: mod, crossPackage: true}
		if got := c.mapScope(); got != wholeModule {
			t.Errorf("want %s, got %s", wholeModule, got)
		}
	})
}

func TestCacheScopeIsWhatAMappingMeans(t *testing.T) {
	t.Parallel()

	whole := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}
	scoped := gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "internal/vm"}

	// How much of the module was scanned decides which packages get mapped, and
	// nothing about what any one mapping says: without --cross-package every test
	// binary is built with -coverpkg set to its own package, so an entry is a
	// function of that package alone. Dividing the cache by the scanned path
	// would mean a run scoped to one package could not read the map a
	// whole-module run had just built for it — which is the whole point of
	// having a cache in CI.
	t.Run("the scanned path does not divide the cache", func(t *testing.T) {
		t.Parallel()

		a := &Coverage{mod: whole}
		b := &Coverage{mod: scoped}
		if a.cacheScope() != b.cacheScope() {
			t.Errorf("a scoped run must share the whole-module map, got %q and %q",
				a.cacheScope(), b.cacheScope())
		}
	})

	// Cross-package instruments each binary over the module, so its entries
	// record lines outside the package under test: the same tests, with strictly
	// more in them. Read as a narrow map it would say no test reaches a line that
	// one does, and the mutant there would be judged by fewer tests than execute
	// it.
	t.Run("cross-package does divide it", func(t *testing.T) {
		t.Parallel()

		narrow := &Coverage{mod: whole}
		wide := &Coverage{mod: whole, crossPackage: true}
		if narrow.cacheScope() == wide.cacheScope() {
			t.Error("a cross-package map records other packages' lines and cannot be read as a narrow one")
		}
	})

	// Two cross-package runs with different --coverpkg record a different set of
	// packages per test, and the scanned path could not tell them apart: it
	// answered "./..." for both.
	t.Run("so does a different cover-pkg under cross-package", func(t *testing.T) {
		t.Parallel()

		module := &Coverage{mod: whole, crossPackage: true}
		subtree := &Coverage{mod: whole, crossPackage: true, coverPkg: "./internal/..."}
		if module.cacheScope() == subtree.cacheScope() {
			t.Error("different --coverpkg values instrument different packages and must not share a cache")
		}
	})

	// A --coverpkg that cross-package is not on to use changes nothing about the
	// map, for the same reason the scanned path does not: the binaries are still
	// built against their own packages.
	t.Run("a cover-pkg without cross-package does not divide it", func(t *testing.T) {
		t.Parallel()

		plain := &Coverage{mod: whole}
		withCoverPkg := &Coverage{mod: whole, coverPkg: "./internal/..."}
		if plain.cacheScope() != withCoverPkg.cacheScope() {
			t.Error("without --cross-package a configured --coverpkg does not reach the test map")
		}
	})
}

func TestCacheKeyChangesWithWhatItCovers(t *testing.T) {
	t.Parallel()

	base := cacheKey("self", "")

	// Both of these change what a profile means, across every package at once:
	// the coverage scope decides which packages appear in it, and the build tags
	// decide which files exist at all.
	if cacheKey(wholeModule, "") == base {
		t.Error("a different coverage scope must not share a cache")
	}
	if cacheKey("self", "integration") == base {
		t.Error("different build tags must not share a cache")
	}
	if cacheKey("self", "") != base {
		t.Error("the same inputs must give the same key")
	}
}

func TestLoadCachedPackageIsAMissRatherThanWrong(t *testing.T) {
	t.Parallel()

	entry := cachedPackage{
		Version:    cacheVersion,
		ImportPath: "example.com/p",
		BuildID:    "id",
		Tests: map[string]Profile{
			"TestOne": {"a.go": {{StartLine: 1, StartCol: 2, EndLine: 3, EndCol: 4}}},
		},
		// The package's own print is embedded in the fingerprint, and has to
		// come back as the fields it was written from.
		Fingerprint: fingerprint{
			pkgPrint: pkgPrint{
				Whole:   "whole",
				Decls:   map[string]declPrint{"a.go:F": {Hash: "h", Sig: "s", Names: []string{"F"}, File: "a.go", Start: 1, End: 3}},
				Imports: map[string]map[string]string{"a.go": {"fmt": "fmt"}},
			},
			Inputs: "inputs",
			Deps:   map[string]pkgPrint{"example.com/q": {Whole: "q", Decls: map[string]declPrint{"q/q.go:G": {Hash: "g"}}}},
		},
	}

	t.Run("a package written into a directory round-trips", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := entry.save(dir); err != nil {
			t.Fatalf("save() error: %v", err)
		}

		got, ok := loadCachedPackage(dir, entry.ImportPath)
		if !ok {
			t.Fatal("want the package read back, got a miss")
		}
		if diff := cmp.Diff(entry, got, cmp.AllowUnexported(fingerprint{}), cmpopts.IgnoreUnexported(declPrint{})); diff != "" {
			t.Errorf("cache round-trip mismatch (-want +got):\n%s", diff)
		}
	})

	// Every one of these costs a re-map of one package, which is what happens
	// without a cache at all. None of them is worth failing a run for, and none
	// may yield a half-read mapping.
	unusable := map[string]string{
		"a file that is not there": "",
		"a file that is not json":  "{not json",
		"a file from another version": `{"version":999,"import_path":"example.com/p",` +
			`"build_id":"id","tests":{}}`,
		"a file naming another package": `{"version":` + strconv.Itoa(cacheVersion) + `,"import_path":"example.com/other",` +
			`"build_id":"id","tests":{}}`,
		"a file with no tests map": `{"version":` + strconv.Itoa(cacheVersion) + `,"import_path":"example.com/p","build_id":"id"}`,
		"a file with no build ID": `{"version":` + strconv.Itoa(cacheVersion) + `,"import_path":"example.com/p",` +
			`"build_id":"","tests":{}}`,
	}
	for name, content := range unusable {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if content != "" {
				if err := os.WriteFile(cacheFilePath(dir, "example.com/p"), []byte(content), 0o600); err != nil {
					t.Fatalf("cannot write the case: %v", err)
				}
			}

			if got, ok := loadCachedPackage(dir, "example.com/p"); ok {
				t.Errorf("want a miss, got %d tests", len(got.Tests))
			}
		})
	}
}

// The point of one file per package: a run that maps one package leaves every
// other package's file exactly where it was. Under the previous layout — one
// file per module, rewritten from what the run saw — this destroyed the rest.
func TestSavingOnePackageLeavesTheOthersAlone(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, path := range []string{"example.com/a", "example.com/b"} {
		entry := cachedPackage{
			Version: cacheVersion, ImportPath: path, BuildID: "id",
			Tests: map[string]Profile{"TestOne": {"a.go": {{StartLine: 1}}}},
		}
		if err := entry.save(dir); err != nil {
			t.Fatalf("save() error: %v", err)
		}
	}

	rewritten := cachedPackage{
		Version: cacheVersion, ImportPath: "example.com/a", BuildID: "changed",
		Tests: map[string]Profile{"TestTwo": {"a.go": {{StartLine: 9}}}},
	}
	if err := rewritten.save(dir); err != nil {
		t.Fatalf("save() error: %v", err)
	}

	if got, ok := loadCachedPackage(dir, "example.com/b"); !ok || got.BuildID != "id" {
		t.Errorf("want the untouched package still cached, got %+v (ok=%t)", got, ok)
	}
	if got, _ := loadCachedPackage(dir, "example.com/a"); got.BuildID != "changed" {
		t.Errorf("want the rewritten package updated, got build ID %q", got.BuildID)
	}
}

func TestCachePathIsOutsideTheModule(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := &Coverage{cacheDir: dir, mod: gomodule.GoModule{Name: "example.com", Root: "."}}

	path, err := c.cacheDirPath("key")
	if err != nil {
		t.Fatalf("cacheDirPath() error: %v", err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("want the cache under %s, got %s", dir, path)
	}

	// Two checkouts of the same module share a map: that is what lets a CI
	// runner use a map another runner made under its own work directory. Each
	// entry still has to agree with the package's build ID, or with its
	// fingerprint and Inputs, before anything in it is used.
	other := &Coverage{cacheDir: dir, mod: gomodule.GoModule{Name: "example.com", Root: t.TempDir()}}
	otherPath, err := other.cacheDirPath("key")
	if err != nil {
		t.Fatalf("cacheDirPath() error: %v", err)
	}
	if path != otherPath {
		t.Errorf("want two checkouts of one module to share a cache directory, got %s and %s", path, otherPath)
	}

	// Two modules never do, wherever they are.
	another := &Coverage{cacheDir: dir, mod: gomodule.GoModule{Name: "example.org", Root: "."}}
	anotherPath, err := another.cacheDirPath("key")
	if err != nil {
		t.Fatalf("cacheDirPath() error: %v", err)
	}
	if path == anotherPath {
		t.Error("two modules must not share a cache directory")
	}

	// The key names a directory rather than living inside the files, so a map
	// gathered under a different coverage scope cannot be read as this one.
	underAnotherKey, err := c.cacheDirPath("other")
	if err != nil {
		t.Fatalf("cacheDirPath() error: %v", err)
	}
	if path == underAnotherKey {
		t.Error("two cache keys must not share a directory")
	}
}

func TestCacheFilePathSeparatesImportPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := cacheFilePath(dir, "example.com/a")
	if a == cacheFilePath(dir, "example.com/b") {
		t.Error("two import paths must not name one file")
	}
	if a != cacheFilePath(dir, "example.com/a") {
		t.Error("the same import path must name the same file")
	}
	// An import path holds separators, so it cannot be a file name as it is.
	if filepath.Dir(a) != dir {
		t.Errorf("want the file directly under %s, got %s", dir, a)
	}
}

// The flags a test binary is compiled with decide what it is, so a change to
// them must stop a map being reused even where nothing else would: they are
// read from one place for the compile and for Inputs.
func TestBuildInputsChangeWithTheCompileFlags(t *testing.T) {
	t.Parallel()

	inputsWith := func(change func(c *Coverage)) string {
		t.Helper()

		root := t.TempDir()
		c := &Coverage{
			mod: gomodule.GoModule{Name: "example.com", Root: root},
			env: &goEnvironment{version: "go-fixture"},
			// No dependencies: the flags are the only thing that differs.
			cmdContext: func(string, ...string) *exec.Cmd { return exec.Command("true") },
		}
		change(c)
		sum, ok := c.buildInputsOf(&testPackage{importPath: "example.com/p", dir: root})
		if !ok {
			t.Fatal("buildInputsOf() failed")
		}

		return sum
	}

	plain := inputsWith(func(*Coverage) {})
	if plain != inputsWith(func(*Coverage) {}) {
		t.Fatal("want the same inputs from the same flags, wherever the module is")
	}
	changes := map[string]func(c *Coverage){
		"build tags": func(c *Coverage) { c.buildTags = "integration" },
		"the coverage scope": func(c *Coverage) {
			c.crossPackage = true
			c.coverPkg = "example.com/..."
		},
	}
	for name, change := range changes {
		if inputsWith(change) == plain {
			t.Errorf("want %s to change the inputs", name)
		}
	}
}

// Which packages the binary links, the standard library's included, and the
// go.mod of every main module it links a package of are part of what it is
// built from: a newly linked package's initialisation runs in every test, and
// a member's go and godebug lines change what its unchanged source does.
func TestBuildInputsChangeWithWhatIsLinked(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	member := filepath.Join(root, "member", "go.mod")
	vendored := filepath.Join(root, "vendored", "go.mod")
	for _, path := range []string{member, vendored} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("cannot create the directory: %v", err)
		}
	}
	inputsWith := func(memberMod, vendoredMod string, extra ...string) string {
		t.Helper()

		if err := os.WriteFile(member, []byte(memberMod), 0o600); err != nil {
			t.Fatalf("cannot write the go.mod: %v", err)
		}
		if err := os.WriteFile(vendored, []byte(vendoredMod), 0o600); err != nil {
			t.Fatalf("cannot write the go.mod: %v", err)
		}
		listing := strings.Join(append([]string{
			"example.com/p\t" + root + "\ttrue\tp\t" + filepath.Join(root, "go.mod"),
			"example.com/member/m\t" + filepath.Dir(member) + "\ttrue\tm\t" + member,
			"example.org/vendored\t" + filepath.Dir(vendored) + "\tfalse\tvendored\t" + vendored,
			"testing\t/nonexistent/goroot/src/testing\t\ttesting\t",
			// The synthesised test main has no directory, and is linked.
			"example.com/p.test\t\t\tmain\t",
		}, extra...), "\n")
		c := &Coverage{
			mod: gomodule.GoModule{Name: "example.com", Root: root},
			env: &goEnvironment{version: "go-fixture", root: "/nonexistent/goroot"},
			cmdContext: func(string, ...string) *exec.Cmd {
				return exec.Command("printf", "%s\n", listing) //nolint:gosec // a fixed program, given the test's own listing
			},
			// The vendored directory is hashed whole; only its go.mod is
			// of interest here, and it is the same file either way.
			dirHashes: map[string]string{filepath.Dir(vendored): "vendored"},
		}
		sum, ok := c.buildInputsOf(&testPackage{importPath: "example.com/p", dir: root})
		if !ok {
			t.Fatal("buildInputsOf() failed")
		}

		return sum
	}

	plain := inputsWith("module example.com/member\n\ngo 1.22\n", "module example.org/vendored\n")
	if diff := cmp.Diff(plain, inputsWith("module example.com/member\n\ngo 1.22\n", "module example.org/vendored\n\ngo 1.25\n")); diff != "" {
		t.Errorf("want a go.mod outside every main module left to go.sum, got the inputs moved:\n%s", diff)
	}
	if inputsWith("module example.com/member\n\ngo 1.22\n\ngodebug panicnil=1\n", "module example.org/vendored\n") == plain {
		t.Error("want a workspace member's go.mod to change the inputs")
	}
	if inputsWith("module example.com/member\n\ngo 1.22\n", "module example.org/vendored\n",
		"crypto/md5\t/nonexistent/goroot/src/crypto/md5\t\tmd5\t") == plain {
		t.Error("want a newly linked standard-library package to change the inputs")
	}
}

func TestTestBuildFlags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	own := filepath.Join(root, "p")
	// What `go list -deps -test` says the package's test binary links: two
	// packages of main modules (in either order), the package itself, a
	// replace target, and the standard library.
	listing := strings.Join([]string{
		"example.com/z\t" + filepath.Join(root, "z") + "\ttrue",
		"example.com/a [example.com/p.test]\t" + filepath.Join(root, "a") + "\ttrue",
		"example.com/p\t" + own + "\ttrue",
		"example.org/replaced\t" + filepath.Join(root, "r") + "\tfalse",
		"testing\t/nonexistent/goroot/src/testing\t",
	}, "\n")
	newCov := func(cross bool) *Coverage {
		return &Coverage{
			buildTags: "integration", crossPackage: cross,
			mod: gomodule.GoModule{Name: "example.com", Root: root},
			env: &goEnvironment{version: "go-fixture", root: "/nonexistent/goroot"},
			cmdContext: func(string, ...string) *exec.Cmd {
				return exec.Command("printf", "%s\n", listing) //nolint:gosec // a fixed program, given the test's own listing
			},
		}
	}
	pkg := &testPackage{importPath: "example.com/p", dir: own}

	// The package's own lines are what its mapping means; its main-module
	// dependencies are instrumented beside it so that a change in one can be
	// narrowed across rather than re-mapping the package.
	want := []string{"-tags", "integration", "-coverpkg", "example.com/p,example.com/a,example.com/z"}
	if diff := cmp.Diff(want, newCov(false).testBuildFlags(pkg)); diff != "" {
		t.Errorf("testBuildFlags() mismatch (-want +got):\n%s", diff)
	}

	// Cross-package mapping instruments the whole module already.
	want = []string{"-tags", "integration", "-coverpkg", wholeModule}
	if diff := cmp.Diff(want, newCov(true).testBuildFlags(pkg)); diff != "" {
		t.Errorf("cross-package testBuildFlags() mismatch (-want +got):\n%s", diff)
	}
}

func TestTestIDNamesThePackageAndTheTest(t *testing.T) {
	t.Parallel()

	if got := (TestID{Pkg: "example.com/p", Name: "TestF"}).String(); got != "example.com/p.TestF" {
		t.Errorf("String() = %q", got)
	}
}

// Without a directory named for it the cache goes under the user's cache
// directory, and a user with none still gets a run, just not a cache.
func TestCachePathDefaultsToTheUserCacheDirectory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("HOME", base)

	c := &Coverage{mod: gomodule.GoModule{Name: "example.com", Root: "."}}
	path, err := c.cacheDirPath("key")
	if err != nil {
		t.Fatalf("cacheDirPath() error: %v", err)
	}
	if !strings.HasPrefix(path, base) {
		t.Errorf("want the cache under %s, got %s", base, path)
	}

	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := c.cacheDirPath("key"); err == nil {
		t.Error("want an error when there is no user cache directory")
	}
}

// A dependency directory that stays in Inputs is hashed once per run however
// many packages link it, and a failure to read it is remembered as one.
func TestHashDirIsReadOncePerRun(t *testing.T) {
	t.Parallel()

	c := &Coverage{}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatalf("cannot write the source: %v", err)
	}
	first, ok := c.hashDir(dir)
	if !ok {
		t.Fatal("want the directory hashed")
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package b\n"), 0o600); err != nil {
		t.Fatalf("cannot write the source: %v", err)
	}
	if again, ok := c.hashDir(dir); !ok || again != first {
		t.Error("want the run's first reading back")
	}

	gone := filepath.Join(t.TempDir(), "gone")
	for range 2 {
		if _, ok := c.hashDir(gone); ok {
			t.Error("want a failure for a directory that is not there")
		}
	}
}
