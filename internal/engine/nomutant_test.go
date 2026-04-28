/*
 * Copyright 2026 The Gremlins Authors
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
	"testing"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/diff"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

// fullyCovered builds a coverage profile that marks every line in the
// fixture file as covered, so a mutant's status is Runnable unless an
// inline directive moves it to Skipped. Distinguishing Runnable from
// Skipped is the whole point of the integration test.
func fullyCovered(fixture string) coverage.Result {
	fn := filenameFromFixture(fixture)
	p := coverage.Profile{fn: {{StartLine: 1, EndLine: 1000, StartCol: 1, EndCol: 1000}}}

	return coverage.Result{Profile: p, Elapsed: 10}
}

func TestNomutantDirective(t *testing.T) {
	t.Parallel()

	// expect describes one mutant the test wants to find in the result set.
	type expect struct {
		line   int
		mType  mutator.Type
		status mutator.Status
	}

	cases := []struct {
		name    string
		fixture string
		expects []expect
	}{
		{
			name:    "end-of-line untyped suppresses every mutator on that line",
			fixture: "testdata/fixtures/nomutant_eol_go",
			expects: []expect{
				// Line 4 (`a := 1 + 2 //nomutant`): suppressed.
				{line: 4, mType: mutator.ArithmeticBase, status: mutator.Skipped},
				// Line 5 (`b := 3 + 4`): not suppressed.
				{line: 5, mType: mutator.ArithmeticBase, status: mutator.Runnable},
			},
		},
		{
			name:    "end-of-line typed filter only suppresses listed types",
			fixture: "testdata/fixtures/nomutant_eol_typed_go",
			expects: []expect{
				// Line 4 has `//nomutant:invert-bitwise`. ArithmeticBase
				// (which actually applies to `+`) must still be Runnable
				// because it isn't in the filter.
				{line: 4, mType: mutator.ArithmeticBase, status: mutator.Runnable},
			},
		},
		{
			name:    "block-scope above a func suppresses every mutant inside",
			fixture: "testdata/fixtures/nomutant_block_func_go",
			expects: []expect{
				// Line 5 is inside the block-scoped `suppressed()`.
				{line: 5, mType: mutator.ArithmeticBase, status: mutator.Skipped},
				// Line 10 is inside `notSuppressed()`.
				{line: 10, mType: mutator.ArithmeticBase, status: mutator.Runnable},
			},
		},
		{
			name:    "file-scope suppresses every mutant in the file",
			fixture: "testdata/fixtures/nomutant_file_go",
			expects: []expect{
				{line: 5, mType: mutator.ArithmeticBase, status: mutator.Skipped},
				{line: 6, mType: mutator.ArithmeticBase, status: mutator.Skipped},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			viperSet(map[string]any{configuration.UnleashDryRunKey: true})
			defer viperReset()

			mapFS, mod, c := loadFixture(tc.fixture, ".")
			defer c()

			cov := fullyCovered(tc.fixture)
			mut := engine.New(mod, engine.CodeData{Cov: cov.Profile}, newJobDealerStub(t), engine.WithDirFs(mapFS))
			res := mut.Run(context.Background())

			for _, want := range tc.expects {
				found := false
				for _, m := range res.Mutants {
					if m.Position().Line == want.line && m.Type() == want.mType {
						found = true
						if m.Status() != want.status {
							t.Errorf("line %d %s: got status %s, want %s",
								want.line, want.mType, m.Status(), want.status)
						}

						break
					}
				}
				if !found {
					t.Errorf("expected to find a %s mutant on line %d; got mutants: %v",
						want.mType, want.line, summarize(res.Mutants))
				}
			}
		})
	}
}

// TestNomutantDirective_OverridesNotCovered verifies that a //nomutant
// directive on an uncovered line still produces a Skipped mutant. The
// directive is evaluated before the coverage-derived status, so an
// explicit "do not test this" wins over an implicit "we couldn't test
// this anyway." Either status would prevent execution, but we choose
// Skipped so the user can audit which suppressions actually fired.
func TestNomutantDirective_OverridesNotCovered(t *testing.T) {
	t.Parallel()
	viperSet(map[string]any{configuration.UnleashDryRunKey: true})
	defer viperReset()

	mapFS, mod, c := loadFixture("testdata/fixtures/nomutant_eol_go", ".")
	defer c()

	// Empty coverage profile → every position is "not covered".
	emptyCov := coverage.Result{Profile: coverage.Profile{}}
	mut := engine.New(mod, engine.CodeData{Cov: emptyCov.Profile}, newJobDealerStub(t), engine.WithDirFs(mapFS))
	res := mut.Run(context.Background())

	var (
		foundSuppressed, foundUnsuppressed bool
	)
	for _, m := range res.Mutants {
		if m.Type() != mutator.ArithmeticBase {
			continue
		}
		switch m.Position().Line {
		case 4:
			foundSuppressed = true
			if m.Status() != mutator.Skipped {
				t.Errorf("line 4 (directive on uncovered line): got %s, want SKIPPED (directive must win over coverage)", m.Status())
			}
		case 5:
			foundUnsuppressed = true
			if m.Status() != mutator.NotCovered {
				t.Errorf("line 5 (no directive, no coverage): got %s, want NOT COVERED", m.Status())
			}
		}
	}
	if !foundSuppressed || !foundUnsuppressed {
		t.Errorf("expected to find both line-4 and line-5 ArithmeticBase mutants; got %v", summarize(res.Mutants))
	}
}

// TestNomutantDirective_WithDiffMode verifies that the directive and
// diff-mode coexist without panicking. Both end up assigning Skipped to
// the same mutants, so the assertion is mostly that nothing blows up
// and both code paths still emit mutants.
func TestNomutantDirective_WithDiffMode(t *testing.T) {
	t.Parallel()
	viperSet(map[string]any{configuration.UnleashDryRunKey: true})
	defer viperReset()

	mapFS, mod, c := loadFixture("testdata/fixtures/nomutant_eol_go", ".")
	defer c()

	// Non-empty diff that lacks an entry for our file → IsChanged returns
	// false for every position in this file, so mutationStatus would
	// already assign Skipped. The directive on line 4 also says Skipped.
	// Both code paths agree; the test pins down that they coexist.
	// (An empty diff.Diff{} means "diff-mode off" — IsChanged returns
	// true for everything in that case, which would defeat the test.)
	codeData := engine.CodeData{
		Cov:  fullyCovered("testdata/fixtures/nomutant_eol_go").Profile,
		Diff: diff.Diff{"unrelated.go": nil},
	}
	mut := engine.New(mod, codeData, newJobDealerStub(t), engine.WithDirFs(mapFS))
	res := mut.Run(context.Background())

	if len(res.Mutants) == 0 {
		t.Fatalf("expected mutants to be emitted, got none")
	}
	for _, m := range res.Mutants {
		if m.Status() != mutator.Skipped {
			t.Errorf("with empty diff every mutant should be Skipped; got %s at line %d",
				m.Status(), m.Position().Line)
		}
	}
}

// TestNomutantDirective_AdjacentTypedLines verifies that two end-of-line
// directives with different typed filters on adjacent lines do not bleed
// into each other.
func TestNomutantDirective_AdjacentTypedLines(t *testing.T) {
	t.Parallel()
	viperSet(map[string]any{configuration.UnleashDryRunKey: true})
	defer viperReset()

	mapFS, mod, c := loadFixture("testdata/fixtures/nomutant_adjacent_typed_go", ".")
	defer c()

	cov := fullyCovered("testdata/fixtures/nomutant_adjacent_typed_go")
	mut := engine.New(mod, engine.CodeData{Cov: cov.Profile}, newJobDealerStub(t), engine.WithDirFs(mapFS))
	res := mut.Run(context.Background())

	// Line 4 has //nomutant:arithmetic-base — only ArithmeticBase suppressed.
	// Line 5 has //nomutant:invert-bitwise — InvertBitwise listed but does
	// not apply to '+'. ArithmeticBase still applies to '+' on line 5 and
	// must NOT be suppressed (would indicate cross-line bleed).
	var (
		line4Status, line5Status mutator.Status
		foundLine4, foundLine5   bool
	)
	for _, m := range res.Mutants {
		if m.Type() != mutator.ArithmeticBase {
			continue
		}
		switch m.Position().Line {
		case 4:
			foundLine4 = true
			line4Status = m.Status()
		case 5:
			foundLine5 = true
			line5Status = m.Status()
		}
	}
	if !foundLine4 || !foundLine5 {
		t.Fatalf("expected ArithmeticBase mutants on both line 4 and line 5; got %v", summarize(res.Mutants))
	}
	if line4Status != mutator.Skipped {
		t.Errorf("line 4 ArithmeticBase: got %s, want SKIPPED (typed filter includes arithmetic-base)", line4Status)
	}
	if line5Status != mutator.Runnable {
		t.Errorf("line 5 ArithmeticBase: got %s, want RUNNABLE (line 5's filter targets invert-bitwise only — directive must NOT bleed from line 4)", line5Status)
	}
}

func summarize(ms []mutator.Mutator) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Position().String()+" "+m.Type().String()+" "+m.Status().String())
	}

	return out
}
