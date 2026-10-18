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

package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// vetZooSource is exactly the shape INVERT_LOGICAL produces from the extremely
// common `name == constA || name == constB`: a conjunction of equalities
// against two DISTINCT CONSTANTS, which is what vet's bools analyzer looks for
// (it ignores the same shape over variables, whose values it cannot compare).
// `go build` accepts this without a word; `go vet` calls it "suspect and", and
// `go test` runs that analyzer before it builds anything.
//
// It is taken from a real one: cerberus's internal/logql/detected_level.go
// reads `name == detectedLevelLabel || name == levelLabelAlias`, and its
// INVERT_LOGICAL mutant was recorded NOT VIABLE on that account.
const (
	vetZooSource = `package vetzoo

const (
	detectedLevelLabel = "detected_level"
	levelLabelAlias    = "level"
)

func IsLevelLabel(name string) bool { return name == detectedLevelLabel && name == levelLabelAlias }
`
	vetZooTest = `package vetzoo

import "testing"

func TestIsLevelLabel(t *testing.T) {
	if IsLevelLabel("detected_level") {
		t.Fatal("IsLevelLabel reported true for a name that cannot equal both constants")
	}
}
`
	// vetSuspectAnd is what `go test` prints when it refuses to build this.
	vetSuspectAnd = "suspect and"

	// vetZooRunBound is only there because getTestArgs requires a run bound;
	// nothing in this test is timing-sensitive.
	vetZooRunBound = time.Minute

	// noTestCache is added to both invocations below. Go's test result cache
	// is keyed on the test BINARY and the arguments handed to the binary, and
	// -vet is neither -- it is a build-time flag. So the two invocations below
	// share a cache entry, and without this the second one reuses the first
	// one's verdict and the comparison measures the cache rather than vet.
	noTestCache = "-count=1"
)

// uncached returns args with the test result cache disabled, keeping `test`
// first because `go` reads the subcommand from there.
func uncached(args []string) []string {
	return slices.Insert(slices.Clone(args), 1, noTestCache)
}

// TestMutantsAreRunWithVetDisabled pins the decision that a mutant is
// adjudicated by the compiler and the test suite, and by nothing else.
//
// `go test` runs a subset of `go vet` before it builds anything, and reports a
// finding as `FAIL pkg [build failed]` — indistinguishable, from the outside,
// from source that does not compile. One of those analyzers, bools, rejects
// precisely the output of INVERT_LOGICAL on an equality disjunction, so with
// vet left on a whole class of mutants could never reach a test binary at all.
//
// Every one of them is legal Go and a real change of behaviour, so the answer
// is not to stop generating them — that would drop from the corpus mutants the
// suite ought to be asked about. It is to stop asking a style analyzer whether
// a mutant may be adjudicated.
//
// Both halves are asserted behaviourally, because only the pair is evidence:
// the first shows the mutant now builds and runs, and the second shows that it
// is `-vet=off` that makes it, rather than the shape having stopped tripping
// vet at some toolchain version.
func TestMutantsAreRunWithVetDisabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":      "module example.com/vetzoo\n\ngo 1.25\n",
		"zoo.go":      vetZooSource,
		"zoo_test.go": vetZooTest,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	sut := &mutantExecutor{testExecutionTime: vetZooRunBound}
	args := sut.getTestArgs("./...")
	if !slices.Contains(args, vetOff) {
		t.Fatalf("gremlins does not pass %s to go test; args were %v", vetOff, args)
	}

	asGremlinsRuns := uncached(args)
	out, err := goTest(t, dir, asGremlinsRuns)
	if err != nil {
		t.Errorf("go test %v rejected a mutant that compiles:\n%s", asGremlinsRuns, out)
	}

	withVet := uncached(slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == vetOff }))
	out, err = goTest(t, dir, withVet)
	if err == nil {
		t.Fatalf("go test %v accepted the suspect conjunction, so %s is no longer load-bearing "+
			"and this test proves nothing:\n%s", withVet, vetOff, out)
	}
	if !strings.Contains(out, vetSuspectAnd) {
		t.Errorf("go test %v failed for a reason other than %q, so the comparison above is not "+
			"measuring vet:\n%s", withVet, vetSuspectAnd, out)
	}
}

func goTest(t *testing.T, dir string, args []string) (string, error) {
	t.Helper()

	//nolint:gosec // args come from getTestArgs and this file's own constants
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	return string(out), err
}
