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

func summarize(ms []mutator.Mutator) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Position().String()+" "+m.Type().String()+" "+m.Status().String())
	}

	return out
}
