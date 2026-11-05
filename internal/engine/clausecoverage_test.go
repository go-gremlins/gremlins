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
	"fmt"
	"testing"

	"github.com/OpenRAE/go-rae-gremlins/internal/configuration"
	"github.com/OpenRAE/go-rae-gremlins/internal/coverage"
	"github.com/OpenRAE/go-rae-gremlins/internal/engine"
	"github.com/OpenRAE/go-rae-gremlins/internal/mutator"
)

const clausesFixture = "testdata/fixtures/clauses_go"

// The coverage blocks of clauses_go, as go test -coverprofile reports them.
// A profile holds only the blocks that ran.
var (
	classifyHeader = coverage.Block{StartLine: 3, StartCol: 26, EndLine: 4, EndCol: 9}
	classifyBig    = coverage.Block{StartLine: 5, StartCol: 14, EndLine: 6, EndCol: 11}
	classifySmall  = coverage.Block{StartLine: 7, StartCol: 13, EndLine: 8, EndCol: 11}
	eitherHeader   = coverage.Block{StartLine: 13, StartCol: 24, EndLine: 14, EndCol: 9}
	eitherBody     = coverage.Block{StartLine: 15, StartCol: 23, EndLine: 16, EndCol: 11}
	fallHeader     = coverage.Block{StartLine: 21, StartCol: 22, EndLine: 23, EndCol: 9}
	fallFirst      = coverage.Block{StartLine: 24, StartCol: 14, EndLine: 26, EndCol: 14}
	fallSecond     = coverage.Block{StartLine: 27, StartCol: 13, EndLine: 28, EndCol: 6}
	defaultHeader  = coverage.Block{StartLine: 33, StartCol: 25, EndLine: 34, EndCol: 9}
	defaultBody    = coverage.Block{StartLine: 35, StartCol: 10, EndLine: 36, EndCol: 11}
	defaultCase    = coverage.Block{StartLine: 37, StartCol: 14, EndLine: 38, EndCol: 11}
	sendHeader     = coverage.Block{StartLine: 42, StartCol: 36, EndLine: 43, EndCol: 9}
	sendBody       = coverage.Block{StartLine: 44, StartCol: 19, EndLine: 45, EndCol: 14}
	sendDefault    = coverage.Block{StartLine: 46, StartCol: 10, EndLine: 47, EndCol: 15}
)

// clauseMutant names a mutant position in clauses_go and its expected status.
type clauseMutant struct {
	line, col int
	mt        mutator.Type
	want      mutator.Status
}

func TestClauseExpressionCoverage(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		blocks  []coverage.Block
		mutants []clauseMutant
	}{
		{
			name:   "a later case body proves that the earlier case expressions ran",
			blocks: []coverage.Block{classifyHeader, classifySmall},
			mutants: []clauseMutant{
				{line: 5, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
				{line: 7, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
			},
		},
		{
			name:   "a matched first case does not prove the later case expressions",
			blocks: []coverage.Block{classifyHeader, classifyBig},
			mutants: []clauseMutant{
				{line: 5, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
				{line: 7, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.NotCovered},
			},
		},
		{
			name:   "a reached switch with no clause body run proves no case expression",
			blocks: []coverage.Block{classifyHeader},
			mutants: []clauseMutant{
				{line: 5, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.NotCovered},
				{line: 7, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.NotCovered},
			},
		},
		{
			name:   "a matched case proves only its first expression",
			blocks: []coverage.Block{eitherHeader, eitherBody},
			mutants: []clauseMutant{
				{line: 15, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
				{line: 15, col: 17, mt: mutator.ConditionalsBoundary, want: mutator.NotCovered},
				{line: 15, col: 19, mt: mutator.InvertNegatives, want: mutator.NotCovered},
			},
		},
		{
			name:   "a body that fallthrough can reach does not prove its case expression",
			blocks: []coverage.Block{fallHeader, fallFirst, fallSecond},
			mutants: []clauseMutant{
				{line: 24, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
				{line: 27, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.NotCovered},
			},
		},
		{
			name:   "a body after a fallthrough clause that did not run proves its case expression",
			blocks: []coverage.Block{fallHeader, fallSecond},
			mutants: []clauseMutant{
				{line: 24, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
				{line: 27, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
			},
		},
		{
			name:   "a selected default clause proves the later case expressions",
			blocks: []coverage.Block{defaultHeader, defaultBody},
			mutants: []clauseMutant{
				{line: 37, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
			},
		},
		{
			name:   "a matched case after the default clause proves its expression",
			blocks: []coverage.Block{defaultHeader, defaultCase},
			mutants: []clauseMutant{
				{line: 37, col: 9, mt: mutator.ConditionalsBoundary, want: mutator.Runnable},
			},
		},
		{
			name:   "a select clause body proves the send expression",
			blocks: []coverage.Block{sendHeader, sendDefault},
			mutants: []clauseMutant{
				{line: 44, col: 15, mt: mutator.ArithmeticBase, want: mutator.Runnable},
			},
		},
		{
			name:   "a select with its own clause run proves the send expression",
			blocks: []coverage.Block{sendHeader, sendBody},
			mutants: []clauseMutant{
				{line: 44, col: 15, mt: mutator.ArithmeticBase, want: mutator.Runnable},
			},
		},
		{
			name:   "a reached select with no clause body run proves no send expression",
			blocks: []coverage.Block{sendHeader},
			mutants: []clauseMutant{
				{line: 44, col: 15, mt: mutator.ArithmeticBase, want: mutator.NotCovered},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			viperSet(map[string]any{configuration.UnleashDryRunKey: true})
			defer viperReset()

			mapFS, mod, c := loadFixture(clausesFixture, ".")
			defer c()
			profile := coverage.Profile{filenameFromFixture(clausesFixture): tc.blocks}

			mut := engine.New(mod, engine.CodeData{Cov: profile}, newJobDealerStub(t), engine.WithDirFs(mapFS))
			res := mut.Run(context.Background())

			got := make(map[string]mutator.Status)
			for _, m := range res.Mutants {
				got[clauseMutantKey(m.Position().Line, m.Position().Column, m.Type())] = m.Status()
			}
			for _, w := range tc.mutants {
				key := clauseMutantKey(w.line, w.col, w.mt)
				status, ok := got[key]
				if !ok {
					t.Errorf("no mutant %s found", key)

					continue
				}
				if status != w.want {
					t.Errorf("mutant %s: got status %s, want %s", key, status, w.want)
				}
			}
		})
	}
}

func clauseMutantKey(line, col int, mt mutator.Type) string {
	return fmt.Sprintf("%s at %d:%d", mt, line, col)
}
