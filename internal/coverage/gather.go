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
	"os"
	"time"

	"github.com/go-gremlins/gremlins/internal/log"
)

// Gather produces what a run starts from: the covered-line set, the elapsed
// time every mutant's timeout is derived from, and — with selection — the
// test map.
//
// Without selection it is the coverage gather, as it always was. With it, the
// map is built as well, and it can stand in for the gather: its union is the
// set of lines each package's own tests execute, which is what selection
// judges a mutant by, and the runs it makes can be timed (see SuiteBaseline).
// So when it can, the gather is skipped, and only the packages the map could
// not answer for are gathered, together, in one go test.
//
// It cannot stand in, and the whole scope is gathered and widened by the map
// exactly as before, when:
//   - --cross-package is set: a mutant is then judged by other packages'
//     tests too, and the gather's scope and the map's are different
//     questions;
//   - integration mode is on: every mutant runs the whole module.
func (c *Coverage) Gather(selection bool) (Result, *TestMap, error) {
	if !selection || !c.mapCanStandIn() {
		res, err := c.Run()
		if err != nil {
			return Result{}, nil, fmt.Errorf("failed to gather coverage: %w", err)
		}
		if !selection {
			return res, nil, nil
		}
		tm, err := c.BuildTestMap()
		if err != nil {
			return Result{}, nil, fmt.Errorf("failed to map tests to the code they execute: %w", err)
		}
		// The map sees a line executed only by another package's tests, which a
		// plain coverage run attributes to nobody, leaving the mutants on it
		// untested. Widen the profile with it rather than replacing it: a
		// package the map could not see whole is missing from the union, and its
		// mutants must stay runnable.
		res.Profile = Merge(res.Profile, tm.Union())

		return res, tm, nil
	}

	// Downloaded first, as Run does, so that a cold module cache is not
	// counted into the compile times the baseline is built from.
	_ = os.Chdir(c.mod.Root)
	if err := c.downloadModules(); err != nil {
		return Result{}, nil, fmt.Errorf("failed to gather coverage: impossible to download modules: %w", err)
	}
	tm, err := c.BuildTestMap()
	if err != nil {
		return Result{}, nil, fmt.Errorf("failed to map tests to the code they execute: %w", err)
	}
	res, err := c.runFromTestMap(tm)
	if err != nil {
		return Result{}, nil, fmt.Errorf("failed to gather coverage: %w", err)
	}

	return res, tm, nil
}

// mapCanStandIn reports whether the test map can replace the coverage gather.
// See Gather.
func (c *Coverage) mapCanStandIn() bool {
	return !c.crossPackage && !c.integrationMode
}

// runFromTestMap builds the gather's Result out of the map: the union for the
// covered set and SuiteBaseline for the elapsed time, with the packages the
// map cannot answer for gathered in one go test and added to both.
func (c *Coverage) runFromTestMap(tm *TestMap) (Result, error) {
	baseline, rest := tm.SuiteBaseline()
	profile := tm.Union()
	var gathered time.Duration
	if len(rest) > 0 {
		var err error
		gathered, err = c.executeCoverageOf(rest...)
		if err != nil {
			return Result{}, fmt.Errorf("impossible to gather coverage of %v: %w", rest, err)
		}
		p, err := c.profile()
		if err != nil {
			return Result{}, fmt.Errorf("an error occurred while generating coverage profile: %w", err)
		}
		profile = Merge(profile, p)
	}
	elapsed := baseline + gathered
	log.Infof("Skipped the coverage gather: the test map stands in for %d of %d packages with tests (%d gathered alone in %s); timeout baseline %s\n",
		len(tm.withTests)-len(rest), len(tm.withTests), len(rest), gathered, elapsed)

	return Result{Profile: profile, Elapsed: elapsed}, nil
}

// SuiteBaseline is the timeout baseline the map gives for the packages it
// mapped, and the packages it cannot give one for, which must be gathered.
//
// The baseline is the SUM, over every mapped package with tests, of
//
//	compile(p) + Σ duration(t) for every test t of p
//
// where compile(p) is how long this run's `go test -c` of p took, or, for a
// package served unchanged from the cache without compiling, how long the
// compile took when its entry was mapped; and
// duration(t) is the wall time of the process that ran t alone during
// mapping — start, package initialisation, TestMain and the test, to exit.
//
// It must never come out below what the gather it replaces measures — the
// wall time of `go test -count=1 -cover` over the same packages — because a
// low baseline turns KILLED mutants into TIMED OUT ones. Term by term:
//   - The gather builds each package's test binary; compile(p) is the same
//     build, instrumented for at least as much (p and its in-module
//     dependencies, against -cover's p), measured cold in the same place in
//     the run: the first build of p after the module download. A recorded
//     compile(p) was measured that way on the run that mapped p, possibly on
//     another machine; the timeout it feeds bounds only the test run, so a
//     difference moves padding, not a verdict.
//   - The gather runs each package's suite once, in one process, sharing
//     initialisation and TestMain across its tests and overlapping its
//     t.Parallel ones. Σ duration(t) runs the same tests one per process and
//     never overlaps them, so it pays every shared cost once per test.
//   - The gather builds and runs packages concurrently, which can only make
//     its wall time shorter than the sum of the packages' work; the baseline
//     takes the sum.
//
// A package is returned to be gathered instead when it is unmapped, when it
// has no compile time (neither this run's nor one its cache entry recorded),
// or when any of its tests has no recorded duration: a missing term is never
// read as zero.
//
// What the sum cannot see is a machine slower, or busier, than the one that
// recorded a reused duration or compile time. A package served unchanged from
// the cache is not compiled at all, so its compile(p) is the recorded one too;
// the slack in the per-process overheads is what covers the rest.
func (t *TestMap) SuiteBaseline() (time.Duration, []string) {
	perPkg := make(map[string]time.Duration, len(t.withTests))
	untimed := make(map[string]bool)
	for id := range t.profiles {
		d, ok := t.durations[id]
		if !ok {
			untimed[id.Pkg] = true

			continue
		}
		perPkg[id.Pkg] += d
	}

	var total time.Duration
	var rest []string
	for _, pkg := range t.withTests {
		compiled, ok := t.compiled[pkg]
		if !t.Mapped(pkg) || !ok || untimed[pkg] {
			rest = append(rest, pkg)

			continue
		}
		total += compiled + perPkg[pkg]
	}

	return total, rest
}
