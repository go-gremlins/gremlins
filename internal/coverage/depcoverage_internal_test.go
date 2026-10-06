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
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/tools/cover"

	"github.com/go-gremlins/gremlins/internal/gomodule"
)

// depSource declares Clamp over lines 3-9 and a package-level function
// literal over lines 11-13; its test file declares TestClamp.
const depSourceText = `package dep

func Clamp(n, hi int) int {
	x := n
	if x > hi {
		x = hi
	}
	return x
}

var double = func(n int) int {
	return n * 2
}
`

const depTestText = `package dep

import "testing"

func TestClamp(t *testing.T) {
	if Clamp(3, 2) != 2 {
		t.Fail()
	}
}
`

// writeDep writes a dependency's directory and reads it the way the map
// builder does.
func writeDep(t *testing.T, c *Coverage, files map[string]string) *depSource {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("cannot write the source: %v", err)
		}
	}
	src, ok := c.depSourceOf(dependency{importPath: "example.com/dep", dir: dir, instrumented: true})
	if !ok {
		t.Fatal("want the dependency read, got a failure")
	}

	return src
}

func block(file string, start, end int) *cover.Profile {
	return &cover.Profile{FileName: file, Blocks: []cover.ProfileBlock{
		{StartLine: start, StartCol: 1, EndLine: end, EndCol: 2, Count: 1},
	}}
}

func newDepCoverage() *Coverage {
	return &Coverage{mod: gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}}
}

// What a test executed in its own package goes into its profile, exactly as
// before dependencies were instrumented: TestsFor, Union and the coverage
// merge read nothing else. What it executed in a dependency is kept only as
// the functions it reached, which is all narrowing needs to ask.
func TestSplitCoverageKeepsDependenciesOutOfTheProfile(t *testing.T) {
	t.Parallel()

	c := newDepCoverage()
	src := writeDep(t, c, map[string]string{"dep.go": depSourceText, "dep_test.go": depTestText})
	pkg := &testPackage{importPath: "example.com/p"}
	sources := map[string]*depSource{"example.com/dep": src}

	got := c.splitCoverage(pkg, []*cover.Profile{
		block("example.com/p/p.go", 3, 5),
		block("example.com/dep/dep.go", 4, 6),
		block("example.com/dep/dep.go", 6, 8),
		// Inside the package-level function literal: the var holding it is
		// the entity a change to it is attributed to.
		block("example.com/dep/dep.go", 11, 13),
		// Instrumented and never executed: no part of what the test reached,
		// though a block of a name it cannot place would otherwise record the
		// whole dependency.
		{FileName: "example.com/dep/gen.y", Blocks: []cover.ProfileBlock{
			{StartLine: 4, StartCol: 1, EndLine: 6, EndCol: 2, Count: 0},
		}},
	}, sources)

	wantProfile := Profile{"p/p.go": {{StartLine: 3, StartCol: 1, EndLine: 5, EndCol: 2}}}
	if diff := cmp.Diff(wantProfile, got.profile); diff != "" {
		t.Errorf("the profile holds more than the package's own lines (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"dep/dep.go:Clamp", "dep/dep.go:var double"}, got.deps); diff != "" {
		t.Errorf("want each dependency entity once, by key (-want +got):\n%s", diff)
	}
	if !got.attributable {
		t.Error("want every block attributed")
	}
}

// A block whose name cannot be trusted to point at the function it came from
// is recorded against the dependency as a whole, so that any change to any of
// its functions dirties the test.
func TestSplitCoverageRecordsAnUnattributableDependencyAsAWhole(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		files map[string]string
		block *cover.Profile
	}{
		// A //line directive renames the file and renumbers the lines a block
		// is reported under, so the name says nothing about where it came
		// from.
		"a line directive": {
			files: map[string]string{"dep.go": depSourceText, "gen.go": "package dep\n\n//line gen.y:100\nfunc G() {}\n"},
			block: block("example.com/dep/dep.go", 4, 6),
		},
		// A file the dependency's directory does not hold as Go source.
		"an unknown file": {
			files: map[string]string{"dep.go": depSourceText},
			block: block("example.com/dep/gen.y", 100, 101),
		},
		// A block of a Go file of it that no entity holds: the profile and
		// the print disagree, and no one key can stand for it.
		"a block outside every entity": {
			files: map[string]string{"dep.go": depSourceText},
			block: block("example.com/dep/dep.go", 15, 16),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newDepCoverage()
			src := writeDep(t, c, tc.files)
			got := c.splitCoverage(&testPackage{importPath: "example.com/p"},
				[]*cover.Profile{tc.block}, map[string]*depSource{"example.com/dep": src})
			if diff := cmp.Diff([]string{depWhole("example.com/dep")}, got.deps); diff != "" {
				t.Errorf("want the dependency recorded whole (-want +got):\n%s", diff)
			}
			if len(got.profile) != 0 {
				t.Errorf("want nothing in the profile, got %v", got.profile)
			}
		})
	}
}

// A block narrowing cannot place makes the package's fingerprint unusable for
// next time, rather than a mapping that claims to know what it executed.
func TestSplitCoverageReportsWhatItCannotPlace(t *testing.T) {
	t.Parallel()

	c := newDepCoverage()
	pkg := &testPackage{importPath: "example.com/p"}

	// A package that is neither this one nor an instrumented dependency. Its
	// lines stay in the profile, as they always were: the split only ever
	// removes what is demonstrably a dependency's.
	got := c.splitCoverage(pkg, []*cover.Profile{block("example.com/q/q.go", 1, 2)}, map[string]*depSource{})
	if got.attributable {
		t.Error("want a block from an unknown package to be unattributable")
	}
	if _, ok := got.profile["q/q.go"]; !ok {
		t.Error("want the unknown package's lines left in the profile")
	}

	// An instrumented dependency whose source could not be read.
	got = c.splitCoverage(pkg, []*cover.Profile{block("example.com/dep/dep.go", 4, 6)},
		map[string]*depSource{"example.com/dep": nil})
	if got.attributable {
		t.Error("want a block from an unreadable dependency to be unattributable")
	}
	if len(got.profile) != 0 {
		t.Errorf("want a dependency's lines kept out of the profile, got %v", got.profile)
	}
}

// Under --cross-package a profile is the whole module's by design, and
// narrowing is off: nothing is split.
func TestSplitCoverageLeavesACrossPackageProfileWhole(t *testing.T) {
	t.Parallel()

	c := newDepCoverage()
	c.crossPackage = true
	got := c.splitCoverage(&testPackage{importPath: "example.com/p"}, []*cover.Profile{
		block("example.com/p/p.go", 3, 5),
		block("example.com/dep/dep.go", 4, 6),
	}, nil)
	if len(got.profile) != 2 || got.deps != nil {
		t.Errorf("want the profile whole and no dependency keys, got %v and %v", got.profile, got.deps)
	}
}

// A dependency's test files are never linked into another package's test
// binary, so nothing in them — tests, helpers, an init — is part of what
// narrowing compares, and a change to one moves nothing; a declaration's
// signature is printed apart from its body, because only a body change is
// attributable to the tests that executed it.
func TestDepSourceDescribesWhatAnotherPackageLinks(t *testing.T) {
	t.Parallel()

	c := newDepCoverage()
	src := writeDep(t, c, map[string]string{"dep.go": depSourceText, "dep_test.go": depTestText})
	for key := range src.stored.Decls {
		if strings.Contains(key, "_test.go") {
			t.Errorf("want nothing of a test file in a dependency's print, got %s", key)
		}
	}
	if _, ok := src.stored.Decls["dep/dep.go:Clamp"]; !ok {
		t.Errorf("want Clamp described, got %v", src.stored.Decls)
	}
	// The stored print has no positions: a dependency's mappings are keys.
	if d := src.stored.Decls["dep/dep.go:Clamp"]; d.File != "" || d.Start != 0 || d.End != 0 {
		t.Errorf("want no positions stored for a dependency, got %+v", d)
	}
	testsChanged := writeDep(t, newDepCoverage(), map[string]string{
		"dep.go":        depSourceText,
		"dep_test.go":   "package dep\n\nimport \"testing\"\n\nfunc init() {}\n\nfunc TestMain(m *testing.M) {\n\tm.Run()\n}\n",
		"other_test.go": "package dep\n\nvar Exported = Clamp\n",
	})
	if diff := cmp.Diff(src.stored, testsChanged.stored, cmpopts.IgnoreUnexported(declPrint{})); diff != "" {
		t.Errorf("want a dependency's test files to move nothing (-want +got):\n%s", diff)
	}

	bodyChanged := writeDep(t, newDepCoverage(), map[string]string{
		"dep.go": strings.Replace(depSourceText, "x := n\n", "x := n + 0\n", 1),
	})
	sigChanged := writeDep(t, newDepCoverage(), map[string]string{
		"dep.go": strings.Replace(depSourceText, "Clamp(n, hi int)", "Clamp(n, hi int64)", 1),
	})
	was := src.stored.Decls["dep/dep.go:Clamp"]
	body := bodyChanged.stored.Decls["dep/dep.go:Clamp"]
	sig := sigChanged.stored.Decls["dep/dep.go:Clamp"]
	if body.Hash == was.Hash || body.Sig != was.Sig {
		t.Errorf("a body change: want the hash moved and the signature kept, got %+v then %+v", was, body)
	}
	if sig.Sig == was.Sig {
		t.Errorf("a signature change: want the signature moved, got %+v then %+v", was, sig)
	}
}

// Reading a dependency is done once per run however many packages link it,
// and a failure to read it is remembered as one.
func TestDepSourceIsReadOncePerRun(t *testing.T) {
	t.Parallel()

	c := newDepCoverage()
	first := writeDep(t, c, map[string]string{"dep.go": depSourceText})
	again, ok := c.depSourceOf(dependency{importPath: "example.com/dep", dir: first.dir, instrumented: true})
	if !ok || again != first {
		t.Error("want the same reading back")
	}

	gone := dependency{importPath: "example.com/gone", dir: filepath.Join(t.TempDir(), "gone"), instrumented: true}
	for range 2 {
		if _, ok := c.depSourceOf(gone); ok {
			t.Error("want a failure for a directory that is not there")
		}
	}
}

// Every instrumented dependency keeps an entry whether or not it could be
// read, because the entries are what keep its blocks out of a profile; only
// whether all of them were read decides if the fingerprint is usable.
func TestDependencySourcesReportsAnUnreadableDependency(t *testing.T) {
	t.Parallel()

	c := newDepCoverage()
	readable := writeDep(t, c, map[string]string{"dep.go": depSourceText})
	pkg := &testPackage{importPath: "example.com/p"}
	c.depListings = map[string]depListing{pkg.importPath: {ok: true, deps: []dependency{
		{importPath: "example.com/dep", dir: readable.dir, instrumented: true},
		{importPath: "example.com/gone", dir: filepath.Join(t.TempDir(), "gone"), instrumented: true},
		{importPath: "example.org/replaced", dir: readable.dir},
	}}}

	sources, ok := c.dependencySources(pkg)
	if ok {
		t.Error("want an unreadable dependency to make the sources unusable")
	}
	want := map[string]bool{"example.com/dep": true, "example.com/gone": false}
	got := map[string]bool{}
	for importPath, src := range sources {
		got[importPath] = src != nil
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("want an entry per instrumented dependency, nil where unreadable (-want +got):\n%s", diff)
	}
}
