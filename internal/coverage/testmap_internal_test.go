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

	t.Run("honours a configured cover-pkg either way", func(t *testing.T) {
		t.Parallel()

		for _, c := range []*Coverage{{coverPkg: "./internal/..."}, {coverPkg: "./internal/...", crossPackage: true}} {
			if got := c.testMapCoverPkg(pkg); got != "./internal/..." {
				t.Errorf("want ./internal/..., got %s", got)
			}
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

func TestCacheKeyChangesWithWhatItCovers(t *testing.T) {
	t.Parallel()

	base := cacheKey("./...", "")

	// Both of these change what a profile means, across every package at once:
	// the coverage scope decides which packages appear in it, and the build tags
	// decide which files exist at all.
	if cacheKey("./internal/...", "") == base {
		t.Error("a different coverage scope must not share a cache")
	}
	if cacheKey("./...", "integration") == base {
		t.Error("different build tags must not share a cache")
	}
	if cacheKey("./...", "") != base {
		t.Error("the same inputs must give the same key")
	}
}

func TestLoadCacheIsEmptyRatherThanWrong(t *testing.T) {
	t.Parallel()

	entry := cachedPackage{BuildID: "id", Tests: map[string]Profile{
		"TestOne": {"a.go": {{StartLine: 1, StartCol: 2, EndLine: 3, EndCol: 4}}},
	}}

	t.Run("a cache written with the same key round-trips", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "testmap.json")
		written := &mapCache{Version: cacheVersion, Key: "k", Packages: map[string]cachedPackage{"p": entry}}
		if err := written.save(path); err != nil {
			t.Fatalf("save() error: %v", err)
		}

		if diff := cmp.Diff(written, loadCache(path, "k")); diff != "" {
			t.Errorf("cache round-trip mismatch (-want +got):\n%s", diff)
		}
	})

	// Every one of these costs a rebuild, which is what happens without a cache
	// at all. None of them is worth failing a run for, and none may yield a
	// half-read map.
	unusable := map[string]struct {
		content string
		key     string
		write   bool
	}{
		"a file that is not there":     {write: false, key: "k"},
		"a file that is not json":      {write: true, content: "{not json", key: "k"},
		"a cache from another version": {write: true, content: `{"version":999,"key":"k","packages":{"p":{}}}`, key: "k"},
		"a cache under another key":    {write: true, content: `{"version":1,"key":"other","packages":{"p":{}}}`, key: "k"},
		"a cache with no packages map": {write: true, content: `{"version":1,"key":"k"}`, key: "k"},
	}
	for name, tc := range unusable {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "testmap.json")
			if tc.write {
				if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
					t.Fatalf("cannot write the case: %v", err)
				}
			}

			got := loadCache(path, tc.key)

			if len(got.Packages) != 0 {
				t.Errorf("want an empty cache, got %d packages", len(got.Packages))
			}
			if got.Version != cacheVersion || got.Key != tc.key {
				t.Errorf("want a usable empty cache, got version %d key %q", got.Version, got.Key)
			}
		})
	}
}

func TestCachePathIsOutsideTheModule(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := &Coverage{cacheDir: dir, mod: gomodule.GoModule{Name: "example.com", Root: "."}}

	path, err := c.cachePath()
	if err != nil {
		t.Fatalf("cachePath() error: %v", err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("want the cache under %s, got %s", dir, path)
	}

	// Two checkouts of the same module must not share a map: the same code at
	// two paths can still map differently, and the second would inherit it.
	other := &Coverage{cacheDir: dir, mod: gomodule.GoModule{Name: "example.com", Root: t.TempDir()}}
	otherPath, err := other.cachePath()
	if err != nil {
		t.Fatalf("cachePath() error: %v", err)
	}
	if path == otherPath {
		t.Error("two checkouts of the same module must not share a cache file")
	}
}
