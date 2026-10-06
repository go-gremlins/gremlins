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
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
)

// gathers returns the coverage gathers the last Coverage issued, one entry per
// `go test -cover` invocation, each naming the packages it gathered.
func (h *cacheHarness) gathers() []string {
	h.t.Helper()

	data, err := os.ReadFile(h.logPath)
	if err != nil {
		return nil
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if pkgs, ok := strings.CutPrefix(line, "gather "); ok {
			got = append(got, pkgs)
		}
	}

	return got
}

func gather(t *testing.T, cov *coverage.Coverage, selection bool) (coverage.Result, *coverage.TestMap) {
	t.Helper()

	res, tm, err := cov.Gather(selection)
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	return res, tm
}

// With every package in scope mapped, the map already holds both things a
// gather produces, and the run gathers nothing. The covered set is the map's
// union exactly — so a mutant is NOT COVERED precisely where no test of its
// own package executes its line.
func TestGatherWithACompleteMapGathersNothing(t *testing.T) {
	h := newCacheHarness(t)

	res, tm := gather(t, h.coverage("TestTestMapHelperProcess", "", ""), true)

	if got := h.gathers(); len(got) != 0 {
		t.Errorf("want no coverage gather, got %v", got)
	}
	if tm == nil {
		t.Fatal("want the test map returned for selection")
	}
	if diff := cmp.Diff(tm.Union(), res.Profile, blockOrder()); diff != "" {
		t.Errorf("the covered set is not the map's union (-want +got):\n%s", diff)
	}
	// The clamp on vm.go is executed only by the root package's test. A
	// module-wide gather calls it covered; the map, which judges a line by
	// its own package's tests, does not, and its mutants are NOT COVERED.
	clamp := token.Position{Filename: "vm/vm.go", Line: clampedLine, Column: 3}
	if res.Profile.IsCovered(clamp) {
		t.Error("a line only another package's tests execute must not be covered")
	}
	if !res.Profile.IsCovered(token.Position{Filename: "vm/vm.go", Line: vmOwnLine, Column: 3}) {
		t.Error("a line the package's own test executes must be covered")
	}
	if res.Elapsed <= 0 {
		t.Errorf("want a positive timeout baseline, got %s", res.Elapsed)
	}
}

// A package the map could not see whole runs its whole suite for every
// mutant, which the map cannot time or say anything about. It is gathered,
// and only it.
func TestGatherGathersAnUnmappedPackageAlone(t *testing.T) {
	h := newCacheHarness(t)

	res, tm := gather(t, h.coverage("TestTestMapHelperProcessFailingTest", "", ""), true)

	if tm.Mapped("example.com") {
		t.Fatal("the fixture should leave the root package unmapped")
	}
	if diff := cmp.Diff([]string{"example.com"}, h.gathers()); diff != "" {
		t.Errorf("want the unmapped package gathered alone (-want +got):\n%s", diff)
	}
	// The gathered package's lines are covered, and the mapped packages'
	// union is still there beside them.
	if !res.Profile.IsCovered(token.Position{Filename: "root.go", Line: 10, Column: 30}) {
		t.Errorf("want the gathered package's lines covered: %v", res.Profile)
	}
	if !res.Profile.IsCovered(token.Position{Filename: "calc/calc.go", Line: 8, Column: 1}) {
		t.Error("want the mapped packages' union kept")
	}
}

// calcPkg is the fixture package the map-scope cases keep in or leave out.
const calcPkg = "example.com/calc"

// A --diff run maps only the packages that can hold its mutants. A package
// outside that scope holds none, so it needs no coverage at all: it is not
// gathered for being unmapped. One inside it that the map failed on is
// gathered, alone.
func TestGatherComposesWithTheMapScope(t *testing.T) {
	t.Run("an out-of-scope package is not gathered", func(t *testing.T) {
		h := newCacheHarness(t)
		// The root package's tests cannot be mapped, but it is not in scope.
		h.scope = []string{scoped, calcPkg}

		_, tm := gather(t, h.coverage("TestTestMapHelperProcessFailingTest", "", ""), true)

		if tm.Mapped("example.com") {
			t.Fatal("an out-of-scope package should be left unmapped")
		}
		if got := h.gathers(); len(got) != 0 {
			t.Errorf("want nothing gathered, got %v", got)
		}
	})

	t.Run("an in-scope package the map failed on is gathered alone", func(t *testing.T) {
		h := newCacheHarness(t)
		h.scope = []string{"example.com", scoped}

		_, tm := gather(t, h.coverage("TestTestMapHelperProcessFailingTest", "", ""), true)

		if tm.Mapped(calcPkg) {
			t.Fatal("an out-of-scope package should be left unmapped")
		}
		if diff := cmp.Diff([]string{"example.com"}, h.gathers()); diff != "" {
			t.Errorf("want only the in-scope unmapped package gathered (-want +got):\n%s", diff)
		}
	})
}

// A map entry with no durations cannot give a baseline, so its package is
// gathered rather than timed at zero.
func TestGatherGathersAPackageWhoseDurationsAreMissing(t *testing.T) {
	h := newCacheHarness(t)
	h.build("TestTestMapHelperProcess", "")

	path := h.cacheFile(scoped)
	// #nosec G304 - the path comes from a directory this test created
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if _, ok := entry["durations"]; !ok {
		t.Fatal("the cache entry should record durations")
	}
	delete(entry, "durations")
	data, err = json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, tm := gather(t, h.coverage("TestTestMapHelperProcess", "", ""), true)

	if !tm.Mapped(scoped) {
		t.Fatal("the entry should still be reused as a mapping")
	}
	if diff := cmp.Diff([]string{scoped}, h.gathers()); diff != "" {
		t.Errorf("want only the package without durations gathered (-want +got):\n%s", diff)
	}
}

// The baseline from the map has to be at least what a gather of the same
// scope measures, or KILLED mutants turn into spurious TIMED OUT ones. The
// fake gather compiles each package and runs its suite once, serially, as
// slowly as the map's own compile and runs take.
func TestGatherBaselineIsNeverBelowAGatherOfTheSameScope(t *testing.T) {
	// The root package cannot be scoped to alone here: it is the module root.
	scopes := []string{"", scoped, calcPkg}
	for _, scope := range scopes {
		t.Run("scope "+scope, func(t *testing.T) {
			h := newCacheHarness(t)
			h.env = []string{compileDelayEnv + "=60ms", runDelayEnv + "=40ms"}

			gathered, err := h.coverage("TestTestMapHelperProcess", "", scope).Run()
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if len(h.gathers()) != 1 {
				t.Fatalf("want the reference run to gather once, got %v", h.gathers())
			}
			mapped, _ := gather(t, h.coverage("TestTestMapHelperProcess", "", scope), true)
			if got := h.gathers(); len(got) != 0 {
				t.Fatalf("want no gather from a complete map, got %v", got)
			}

			if mapped.Elapsed < gathered.Elapsed {
				t.Errorf("the map's baseline %s is below the gather's %s", mapped.Elapsed, gathered.Elapsed)
			}
		})
	}
}

// A mapping reused from the cache keeps the duration it was recorded with:
// the run that reuses it never ran the test, and has nothing else to time it
// by.
func TestGatherBaselineKeepsRecordedDurationsForReusedMappings(t *testing.T) {
	const runDelay = 100 * time.Millisecond

	t.Run("a whole package reused", func(t *testing.T) {
		h := newCacheHarness(t)
		h.env = []string{runDelayEnv + "=" + runDelay.String()}
		h.build("TestTestMapHelperProcess", "")

		h.env = nil
		res, _ := gather(t, h.coverage("TestTestMapHelperProcess", "", ""), true)

		if got := h.testsRun(); len(got) != 0 {
			t.Fatalf("want every mapping reused, got %v run", got)
		}
		if want := 5 * runDelay; res.Elapsed < want {
			t.Errorf("want the recorded durations, at least %s, got %s", want, res.Elapsed)
		}
	})

	t.Run("one mapping of a changed package reused", func(t *testing.T) {
		h := newCacheHarness(t)
		h.env = []string{runDelayEnv + "=" + runDelay.String()}
		h.build("TestTestMapHelperProcess", "")

		h.env = nil
		h.edit("calc/calc.go", strings.Replace(calcSource, "n * 2", "n + n", 1))
		res, _ := gather(t, h.coverage("TestTestMapHelperProcess", "example.com/calc=changed", ""), true)

		if diff := cmp.Diff([]string{"TestDouble"}, h.testsRun()); diff != "" {
			t.Fatalf("want only the changed test re-run (-want +got):\n%s", diff)
		}
		if got := h.gathers(); len(got) != 0 {
			t.Fatalf("want no gather, got %v", got)
		}
		// Four mappings reused at their recorded 100ms each.
		if want := 4 * runDelay; res.Elapsed < want {
			t.Errorf("want the recorded durations, at least %s, got %s", want, res.Elapsed)
		}
	})
}

// Each of these makes the map an unsafe or meaningless source for the gather,
// and the run gathers the whole scope exactly as it did before the map could
// stand in for it.
func TestGatherGathersTheWholeScopeWhenTheMapCannotStandIn(t *testing.T) {
	t.Run("without test selection", func(t *testing.T) {
		h := newCacheHarness(t)

		_, tm := gather(t, h.coverage("TestTestMapHelperProcess", "", ""), false)

		if tm != nil {
			t.Error("want no map built without selection")
		}
		if diff := cmp.Diff([]string{"./..."}, h.gathers()); diff != "" {
			t.Errorf("want the whole scope gathered (-want +got):\n%s", diff)
		}
	})

	t.Run("with --cross-package", func(t *testing.T) {
		viper.Set(configuration.UnleashCrossPackageKey, true)
		t.Cleanup(func() { viper.Set(configuration.UnleashCrossPackageKey, false) })
		h := newCacheHarness(t)

		res, tm := gather(t, h.coverage("TestTestMapHelperProcess", "", ""), true)

		if diff := cmp.Diff([]string{"./..."}, h.gathers()); diff != "" {
			t.Errorf("want the whole scope gathered (-want +got):\n%s", diff)
		}
		// The gather is widened by the map, as it always was.
		if diff := cmp.Diff(coverage.Merge(gatheredProfile(t), tm.Union()), res.Profile, blockOrder()); diff != "" {
			t.Errorf("want the gather merged with the union (-want +got):\n%s", diff)
		}
	})

}

// gatheredProfile is what the fake gather writes for the whole module.
func gatheredProfile(t *testing.T) coverage.Profile {
	t.Helper()

	block := func(sl, sc, el, ec int) coverage.Block {
		return coverage.Block{StartLine: sl, StartCol: sc, EndLine: el, EndCol: ec}
	}

	return coverage.Profile{
		"root.go":      {block(6, 26, 6, 50), block(10, 26, 10, 50)},
		"vm/vm.go":     {block(4, 29, 6, 15), block(6, 15, 8, 3)},
		"calc/calc.go": {block(3, 24, 5, 2), block(7, 24, 9, 2)},
	}
}
