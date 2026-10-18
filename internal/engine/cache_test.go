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

package engine_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/engine/workdir"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

// cacheModule is a module with one mutated package, a, whose tests read a
// data file and an environment variable; a dependency of it, dep, with tests
// of its own; and a package nothing in a's closure imports, other.
var cacheModule = map[string]string{
	"go.mod":     "module example.com/cachemod\n\ngo 1.25\n",
	"dep/dep.go": "package dep\n\n// Threshold is what a compares against.\nfunc Threshold() int { return 1 }\n",
	"dep/dep_test.go": `package dep

import "testing"

func TestThreshold(t *testing.T) {
	if Threshold() != 1 {
		t.Fatal("threshold")
	}
}
`,
	"other/other.go": "package other\n\n// X is in no closure the mutants of a depend on.\nfunc X() int { return 2 }\n",
	"a/a.go": `package a

import "example.com/cachemod/dep"

// Big reports whether x is above the threshold.
func Big(x int) bool { return x > dep.Threshold() }
`,
	"a/a_test.go": `package a

import (
	"os"
	"testing"
)

func TestBig(t *testing.T) {
	if _, err := os.ReadFile("testdata/cases.txt"); err != nil {
		t.Fatal(err)
	}
	_ = os.Getenv("CACHE_PROBE")
	if Big(1) {
		t.Fatal("1 is not big")
	}
	if !Big(2) {
		t.Fatal("2 is big")
	}
}
`,
	"a/testdata/cases.txt": "1 2\n",
}

// The verdicts the cache tests expect, by mutator.
const (
	boundaryKilled = "CONDITIONALS_BOUNDARY KILLED"
	boundaryLived  = "CONDITIONALS_BOUNDARY LIVED"
	negationKilled = "CONDITIONALS_NEGATION KILLED"
)

// cacheIgnoredPrefix starts the variables the cache tests leave out of the key.
const cacheIgnoredPrefix = "CACHE_IGNORED_"

// goTestSubcommand is the argument that makes an executor command a test run.
const goTestSubcommand = "test"

// cacheRun is what one gremlins run over the module reported.
type cacheRun struct {
	statuses []string
	// goTests counts the `go test` invocations the run made, one per mutant
	// whose verdict did not come from the cache.
	goTests int64
	hits    int64
	stores  int64
}

// runWithCache runs gremlins over dir's package a, with the cache in cacheDir.
func runWithCache(t *testing.T, dir, cacheDir, runBound string) cacheRun {
	t.Helper()

	return runWithCacheIn(t, dir, cacheDir, runBound, t.TempDir())
}

// runWithCacheIn is runWithCache with the workers' copies under workDir.
func runWithCacheIn(t *testing.T, dir, cacheDir, runBound, workDir string) cacheRun {
	t.Helper()

	viperSet(map[string]any{
		configuration.UnleashDryRunKey:           false,
		configuration.UnleashTimeoutMaxKey:       runBound,
		configuration.UnleashCompileAllowanceKey: generousCompileAllowance,
		configuration.UnleashCacheDirKey:         cacheDir,
		configuration.UnleashCacheEnvIgnoreKey:   []string{cacheIgnoredPrefix + "*"},
	})
	defer viperReset()

	var goTests atomic.Int64
	counting := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == goTestSubcommand {
			goTests.Add(1)
		}

		//nolint:gosec // the go command the executor asked for
		return exec.CommandContext(ctx, name, args...)
	}

	mod := gomodule.GoModule{Name: "example.com/cachemod", Root: dir, CallingDir: "a"}
	// Every line of a.go is covered: what is under test is the cache, not
	// the coverage run.
	cov := coverage.Profile{"a.go": {{StartLine: 1, StartCol: 1, EndLine: 1000, EndCol: 1}}}
	wdDealer := workdir.NewCachedDealer(workDir, dir)
	defer wdDealer.Clean()
	jDealer := engine.NewExecutorDealer(mod, wdDealer, baselineElapsed, engine.WithExecContext(counting))
	eng := engine.New(mod, engine.CodeData{Cov: cov}, jDealer)
	res := eng.Run(context.Background())

	run := cacheRun{goTests: goTests.Load()}
	run.hits, run.stores, _ = jDealer.CacheStats()
	for _, m := range res.Mutants {
		run.statuses = append(run.statuses, m.Type().String()+" "+m.Status().String())
	}
	sort.Strings(run.statuses)

	return run
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Dir(p), filepath.Base(p), content)
	}

	return dir
}

func appendTo(t *testing.T, dir, name, text string) {
	t.Helper()

	p := filepath.Join(dir, filepath.FromSlash(name))
	//nolint:gosec // a file of the throwaway module
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // a file of the throwaway module
	if err := os.WriteFile(p, append(b, text...), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestTheVerdictCacheIsInvalidatedByEverythingAVerdictReadsAndNothingElse
// runs the same mutants again and again, changing one thing between runs, and
// asserts after each change whether gremlins ran the tests again.
//
// Each step is a pair: the run straight after the change, and a repeat that
// must come entirely from the cache. The repeat is what shows the change was
// stored under its new key, so the next step's miss is caused by its own
// change and not left over from this one.
func TestTheVerdictCacheIsInvalidatedByEverythingAVerdictReadsAndNothingElse(t *testing.T) {
	dir := writeModule(t, cacheModule)
	cacheDir := t.TempDir()
	t.Setenv("CACHE_PROBE", "one")

	first := runWithCache(t, dir, cacheDir, gapRunBound)
	if len(first.statuses) == 0 {
		t.Fatal("no mutants")
	}
	for _, s := range first.statuses {
		if !strings.HasSuffix(s, mutator.Killed.String()) {
			t.Fatalf("expected every mutant KILLED, got %v", first.statuses)
		}
	}
	mutants := int64(len(first.statuses))
	if first.goTests != mutants || first.stores != mutants || first.hits != 0 {
		t.Fatalf("a cold cache must run and store every mutant: %+v", first)
	}

	assertHit := func(t *testing.T, why string) {
		t.Helper()
		got := runWithCache(t, dir, cacheDir, gapRunBound)
		if got.goTests != 0 || got.hits != mutants {
			t.Fatalf("%s: expected every verdict from the cache, got %+v", why, got)
		}
		if strings.Join(got.statuses, ",") != strings.Join(first.statuses, ",") {
			t.Fatalf("%s: cached verdicts %v differ from the run's %v", why, got.statuses, first.statuses)
		}
	}
	assertMiss := func(t *testing.T, why string) {
		t.Helper()
		got := runWithCache(t, dir, cacheDir, gapRunBound)
		if got.goTests != mutants || got.hits != 0 {
			t.Fatalf("%s: expected every mutant to run again, got %+v", why, got)
		}
		assertHit(t, why+", repeated")
	}

	assertHit(t, "nothing changed")

	steps := []struct {
		name   string
		change func(t *testing.T)
		miss   bool
	}{
		{
			name:   "the mutated package's test file changes",
			change: func(t *testing.T) { appendTo(t, dir, "a/a_test.go", "\n// edited\n") },
			miss:   true,
		},
		{
			name:   "a data file the tests read changes",
			change: func(t *testing.T) { appendTo(t, dir, "a/testdata/cases.txt", "3\n") },
			miss:   true,
		},
		{
			name:   "an environment variable the tests read changes",
			change: func(t *testing.T) { t.Setenv("CACHE_PROBE", "two") },
			miss:   true,
		},
		{
			name:   "a dependency's source changes",
			change: func(t *testing.T) { appendTo(t, dir, "dep/dep.go", "\n// edited\n") },
			miss:   true,
		},
		{
			name:   "the module's go.mod changes",
			change: func(t *testing.T) { appendTo(t, dir, "go.mod", "\n// edited\n") },
			miss:   true,
		},
		{
			name:   "a dependency's own test file changes",
			change: func(t *testing.T) { appendTo(t, dir, "dep/dep_test.go", "\n// edited\n") },
			miss:   false,
		},
		{
			name:   "a package outside the closure changes",
			change: func(t *testing.T) { appendTo(t, dir, "other/other.go", "\n// edited\n") },
			miss:   false,
		},
		{
			// An initialiser or a TestMain could have read it before the
			// test log started, so any variable of the environment counts.
			name:   "an environment variable the tests never read changes",
			change: func(t *testing.T) { t.Setenv("CACHE_UNREAD", "x") },
			miss:   true,
		},
		{
			name:   "a variable the ignore list names changes",
			change: func(t *testing.T) { t.Setenv(cacheIgnoredPrefix+"RUN", "x") },
			miss:   false,
		},
	}
	for _, step := range steps {
		step.change(t)
		if step.miss {
			assertMiss(t, step.name)
		} else {
			assertHit(t, step.name)
		}
	}

	t.Run("a corrupt entry is a miss", func(t *testing.T) {
		var entries []string
		_ = filepath.WalkDir(cacheDir, func(p string, d os.DirEntry, _ error) error {
			if d != nil && !d.IsDir() {
				entries = append(entries, p)
			}

			return nil
		})
		if len(entries) == 0 {
			t.Fatal("no entries on disk")
		}
		for _, p := range entries {
			appendTo(t, filepath.Dir(p), filepath.Base(p), "x")
		}
		assertMiss(t, "every entry corrupted")
	})
}

// TestOnlyVerdictsTheCacheCanProveAreStored covers which verdicts are kept: a
// LIVED and a KILLED from a binary that finished are, and a KILLED from a test
// binary that died before flushing its test log and a timed-out mutant are
// not.
func TestOnlyVerdictsTheCacheCanProveAreStored(t *testing.T) {
	testCases := []struct {
		name       string
		test       string
		wantStores int64
		want       []string
	}{
		{
			// A panicking binary never returns from testing.M.Run, so its
			// test log may have lost its tail. The test reads enough
			// variables first that the log's buffer has already been flushed
			// at least once: the log on disk is a well-formed prefix, and only
			// the missing memory profile keeps it from being stored.
			name: "a KILLED from a panicking test binary",
			test: `package a

import (
	"fmt"
	"os"
	"testing"
)

func TestBig(t *testing.T) {
	for i := range 2000 {
		_ = os.Getenv(fmt.Sprintf("CACHE_FILL_%d", i))
	}
	if Big(1) {
		panic("1 is not big")
	}
	if !Big(2) {
		panic("2 is big")
	}
}
`,
			wantStores: 0,
			want:       []string{boundaryKilled, negationKilled},
		},
		{
			// The negation mutant makes Big(2) false and the loop never ends;
			// the boundary mutant is killed normally and is the one stored.
			name: "a RUN TIMED OUT beside a normal KILLED",
			test: `package a

import "testing"

func TestBig(t *testing.T) {
	for !Big(2) {
	}
	if Big(1) {
		t.Fatal("1 is not big")
	}
}
`,
			wantStores: 1,
			want:       []string{boundaryKilled, "CONDITIONALS_NEGATION RUN TIMED OUT"},
		},
		{
			name: "a LIVED beside a KILLED",
			test: `package a

import "testing"

func TestBig(t *testing.T) {
	if !Big(5) {
		t.Fatal("5 is big")
	}
	if Big(0) {
		t.Fatal("0 is not big")
	}
}
`,
			wantStores: 2,
			want:       []string{boundaryLived, negationKilled},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			for k, v := range cacheModule {
				files[k] = v
			}
			files["a/a_test.go"] = tc.test
			dir := writeModule(t, files)
			cacheDir := t.TempDir()

			for i := range 2 {
				got := runWithCache(t, dir, cacheDir, "2s")
				if strings.Join(got.statuses, ",") != strings.Join(tc.want, ",") {
					t.Fatalf("run %d: got %v, want %v", i, got.statuses, tc.want)
				}
				wantStores, wantHits := tc.wantStores, int64(0)
				if i == 1 {
					wantStores, wantHits = 0, tc.wantStores
				}
				if got.stores != wantStores || got.hits != wantHits {
					t.Fatalf("run %d: stores %d hits %d, want stores %d hits %d", i, got.stores, got.hits, wantStores, wantHits)
				}
			}
		})
	}
}

// TestInputsOutsideTheTestLogStillInvalidate covers reads whose change flips
// a verdict but which the test log alone does not tie to the module: a
// TestMain that reads a file of its package tree or a variable before
// testing.M.Run starts the log, a test that reaches its data by an absolute
// path through a work directory behind a symbolic link, which the binary sees
// resolved, and a fixture or a flag file under the temporary directory named
// by a variable whose value never changes.
//
// Each case is LIVED on a loose expectation and KILLED on a strict one, so a
// stale entry would report LIVED after the change.
func TestInputsOutsideTheTestLogStillInvalidate(t *testing.T) {
	const strictBody = `
func TestBig(t *testing.T) {
	if !Big(5) {
		t.Fatal("5 is big")
	}
	if strict() && Big(1) {
		t.Fatal("1 is not big")
	}
}
`
	flag := filepath.Join(t.TempDir(), "flag")
	t.Setenv("CACHE_FLAG", flag)
	tmpFixture := filepath.Join(t.TempDir(), "fixture")
	writeFile(t, filepath.Dir(tmpFixture), filepath.Base(tmpFixture), "loose\n")
	t.Setenv("CACHE_FIXTURE", tmpFixture)
	testCases := []struct {
		name      string
		test      string
		linkedDir bool
		tighten   func(t *testing.T, dir string)
	}{
		{
			name: "a TestMain reads a file of a subdirectory of its package",
			test: `package a

import (
	"os"
	"strings"
	"testing"
)

var mode string

func strict() bool { return mode == "strict" }

func TestMain(m *testing.M) {
	b, _ := os.ReadFile("fixtures/want")
	mode = strings.TrimSpace(string(b))
	os.Exit(m.Run())
}
` + strictBody,
			tighten: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "a", "fixtures"), "want", "strict\n")
			},
		},
		{
			name: "a TestMain reads a variable",
			test: `package a

import (
	"os"
	"testing"
)

var mode string

func strict() bool { return mode == "strict" }

func TestMain(m *testing.M) {
	mode = os.Getenv("CACHE_MODE")
	os.Exit(m.Run())
}
` + strictBody,
			tighten: func(t *testing.T, _ string) { t.Setenv("CACHE_MODE", "strict") },
		},
		{
			name: "a test reads a temporary fixture a variable names",
			test: `package a

import (
	"os"
	"strings"
	"testing"
)

func strict() bool {
	b, _ := os.ReadFile(os.Getenv("CACHE_FIXTURE"))

	return strings.TrimSpace(string(b)) == "strict"
}
` + strictBody,
			tighten: func(t *testing.T, _ string) {
				writeFile(t, filepath.Dir(tmpFixture), filepath.Base(tmpFixture), "strict\n")
			},
		},
		{
			name: "a test checks that a temporary flag file does not exist",
			test: `package a

import (
	"os"
	"testing"
)

func strict() bool {
	_, err := os.Stat(os.Getenv("CACHE_FLAG"))

	return err == nil
}
` + strictBody,
			tighten: func(t *testing.T, _ string) {
				writeFile(t, filepath.Dir(flag), filepath.Base(flag), "")
			},
		},
		{
			name: "a TestMain reads a file of its package directory",
			test: `package a

import (
	"os"
	"strings"
	"testing"
)

var mode string

func strict() bool { return mode == "strict" }

func TestMain(m *testing.M) {
	b, _ := os.ReadFile("want.txt")
	mode = strings.TrimSpace(string(b))
	os.Exit(m.Run())
}
` + strictBody,
		},
		{
			name: "a test reads by absolute path through a linked work directory",
			test: `package a

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func strict() bool {
	p, err := filepath.Abs("want.txt")
	if err != nil {
		panic(err)
	}
	b, _ := os.ReadFile(p)

	return strings.TrimSpace(string(b)) == "strict"
}
` + strictBody,
			linkedDir: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			for k, v := range cacheModule {
				files[k] = v
			}
			files["a/a_test.go"] = tc.test
			files["a/want.txt"] = "loose\n"
			files["a/fixtures/want"] = "loose\n"
			dir := writeModule(t, files)
			cacheDir := t.TempDir()
			workDir := t.TempDir()
			if tc.linkedDir {
				link := filepath.Join(t.TempDir(), "work-link")
				if err := os.Symlink(workDir, link); err != nil {
					t.Fatal(err)
				}
				workDir = link
			}
			run := func() cacheRun { return runWithCacheIn(t, dir, cacheDir, gapRunBound, workDir) }

			loose := []string{boundaryLived, negationKilled}
			if got := run(); strings.Join(got.statuses, ",") != strings.Join(loose, ",") || got.stores != 2 {
				t.Fatalf("cold run: %+v, want %v stored", got, loose)
			}
			if got := run(); got.hits != 2 || got.goTests != 0 {
				t.Fatalf("repeat: %+v, want every verdict from the cache", got)
			}
			if tc.tighten != nil {
				tc.tighten(t, dir)
			} else {
				writeFile(t, filepath.Join(dir, "a"), "want.txt", "strict\n")
			}
			strict := []string{boundaryKilled, negationKilled}
			if got := run(); strings.Join(got.statuses, ",") != strings.Join(strict, ",") {
				t.Fatalf("after the change: %v, want %v", got.statuses, strict)
			}
		})
	}
}
