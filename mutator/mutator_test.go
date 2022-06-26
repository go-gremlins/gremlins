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

package mutator_test

import (
	"github.com/google/go-cmp/cmp"
	"github.com/k3rn31/gremlins/coverage"
	"github.com/k3rn31/gremlins/mutator"
	"go/token"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func coveredPosition(fixture string) coverage.Profile {
	fn := filenameFromFixture(fixture)
	return coverage.Profile{fn: {{StartLine: 6, EndLine: 7, StartCol: 8, EndCol: 9}}}
}

func notCoveredPosition(fixture string) coverage.Profile {
	fn := filenameFromFixture(fixture)
	return coverage.Profile{fn: {{StartLine: 9, EndLine: 9, StartCol: 8, EndCol: 9}}}
}

func TestMutations(t *testing.T) {
	testCases := []struct {
		name          string
		fixture       string
		mutantType    mutator.MutantType
		token         token.Token
		tokenMutation token.Token
		covProfile    coverage.Profile
		mutStatus     mutator.MutantStatus
	}{
		// CONDITIONAL BOUNDARIES
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with GTR",
			fixture:       "testdata/fixtures/gtr_go",
			mutantType:    mutator.ConditionalsBoundary,
			token:         token.GTR,
			tokenMutation: token.GEQ,
			covProfile:    coveredPosition("testdata/fixtures/gtr_go"),
			mutStatus:     mutator.Runnable,
		},
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with LSS",
			fixture:       "testdata/fixtures/lss_go",
			mutantType:    mutator.ConditionalsBoundary,
			token:         token.LSS,
			tokenMutation: token.LEQ,
			covProfile:    notCoveredPosition("testdata/fixtures/lss_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with LEQ",
			fixture:       "testdata/fixtures/leq_go",
			mutantType:    mutator.ConditionalsBoundary,
			token:         token.LEQ,
			tokenMutation: token.LSS,
			covProfile:    notCoveredPosition("testdata/fixtures/leq_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with GEQ",
			fixture:       "testdata/fixtures/geq_go",
			mutantType:    mutator.ConditionalsBoundary,
			token:         token.GEQ,
			tokenMutation: token.GTR,
			covProfile:    notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:     mutator.NotCovered,
		},
		// INCREMENT_DECREMENT
		{
			name:          "it recognizes INCREMENT_DECREMENT with INC",
			fixture:       "testdata/fixtures/inc_go",
			mutantType:    mutator.IncrementDecrement,
			token:         token.INC,
			tokenMutation: token.DEC,
			covProfile:    notCoveredPosition("testdata/fixtures/inc_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes INCREMENT_DECREMENT with DEC",
			fixture:       "testdata/fixtures/dec_go",
			mutantType:    mutator.IncrementDecrement,
			token:         token.DEC,
			tokenMutation: token.INC,
			covProfile:    notCoveredPosition("testdata/fixtures/dec_go"),
			mutStatus:     mutator.NotCovered,
		},
		// CONDITIONAL_NEGATION
		{
			name:          "it recognizes CONDITIONAL_NEGATION with EQL",
			fixture:       "testdata/fixtures/eql_go",
			mutantType:    mutator.ConditionalsNegation,
			token:         token.EQL,
			tokenMutation: token.NEQ,
			covProfile:    notCoveredPosition("testdata/fixtures/eql_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_NEGATION with NEQ",
			fixture:       "testdata/fixtures/neq_go",
			mutantType:    mutator.ConditionalsNegation,
			token:         token.NEQ,
			tokenMutation: token.EQL,
			covProfile:    notCoveredPosition("testdata/fixtures/neq_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_NEGATION with LEQ",
			fixture:       "testdata/fixtures/leq_go",
			mutantType:    mutator.ConditionalsNegation,
			token:         token.LEQ,
			tokenMutation: token.GTR,
			covProfile:    notCoveredPosition("testdata/fixtures/leq_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_NEGATION with GTR",
			fixture:       "testdata/fixtures/gtr_go",
			mutantType:    mutator.ConditionalsNegation,
			token:         token.GTR,
			tokenMutation: token.LEQ,
			covProfile:    notCoveredPosition("testdata/fixtures/gtr_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_NEGATION with GEQ",
			fixture:       "testdata/fixtures/geq_go",
			mutantType:    mutator.ConditionalsNegation,
			token:         token.GEQ,
			tokenMutation: token.LSS,
			covProfile:    notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_NEGATION with LSS",
			fixture:       "testdata/fixtures/lss_go",
			mutantType:    mutator.ConditionalsNegation,
			token:         token.LSS,
			tokenMutation: token.GEQ,
			covProfile:    notCoveredPosition("testdata/fixtures/lss_go"),
			mutStatus:     mutator.NotCovered,
		},
		// INVERT_NEGATIVES
		{
			name:          "it recognizes INVERT_NEGATIVE with SUB",
			fixture:       "testdata/fixtures/negative_sub_go",
			mutantType:    mutator.InvertNegatives,
			token:         token.SUB,
			tokenMutation: token.ADD,
			covProfile:    notCoveredPosition("testdata/fixtures/negative_sub_go"),
			mutStatus:     mutator.NotCovered,
		},
		// ARITHMETIC_BASIC
		{
			name:          "it recognizes ARITHMETIC_BASIC with ADD",
			fixture:       "testdata/fixtures/add_go",
			mutantType:    mutator.ArithmeticBase,
			token:         token.ADD,
			tokenMutation: token.SUB,
			covProfile:    notCoveredPosition("testdata/fixtures/add_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes ARITHMETIC_BASIC with SUB",
			fixture:       "testdata/fixtures/sub_go",
			mutantType:    mutator.ArithmeticBase,
			token:         token.SUB,
			tokenMutation: token.ADD,
			covProfile:    notCoveredPosition("testdata/fixtures/sub_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes ARITHMETIC_BASIC with MUL",
			fixture:       "testdata/fixtures/mul_go",
			mutantType:    mutator.ArithmeticBase,
			token:         token.MUL,
			tokenMutation: token.QUO,
			covProfile:    notCoveredPosition("testdata/fixtures/mul_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes ARITHMETIC_BASIC with QUO",
			fixture:       "testdata/fixtures/quo_go",
			mutantType:    mutator.ArithmeticBase,
			token:         token.QUO,
			tokenMutation: token.MUL,
			covProfile:    notCoveredPosition("testdata/fixtures/quo_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes ARITHMETIC_BASIC with REM",
			fixture:       "testdata/fixtures/rem_go",
			mutantType:    mutator.ArithmeticBase,
			token:         token.REM,
			tokenMutation: token.MUL,
			covProfile:    notCoveredPosition("testdata/fixtures/rem_go"),
			mutStatus:     mutator.NotCovered,
		},
		// Common behaviours
		{
			name:          "it works with recursion",
			fixture:       "testdata/fixtures/geq_land_true_go",
			mutantType:    mutator.ConditionalsBoundary,
			token:         token.GEQ,
			tokenMutation: token.GTR,
			covProfile:    notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:       "it skips illegal tokens",
			fixture:    "testdata/fixtures/illegal_go",
			mutantType: mutator.ConditionalsBoundary,
			token:      token.ILLEGAL,
			covProfile: notCoveredPosition("testdata/fixtures/illegal_go"),
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, _ := os.Open(tc.fixture)
			src, _ := ioutil.ReadAll(f)
			filename := filenameFromFixture(tc.fixture)
			mapFS := fstest.MapFS{
				filename: {Data: src},
			}
			mut := mutator.New(mapFS, tc.covProfile)
			got := mut.Run()

			if tc.token == token.ILLEGAL {
				if len(got) != 0 {
					t.Errorf("expected no mutator found")
				}
				return
			}

			for _, g := range got {
				if g.Type == tc.mutantType &&
					g.Token == tc.token &&
					g.Mutation == tc.tokenMutation &&
					g.Status == tc.mutStatus {
					return
				}
			}

			t.Errorf("expected mutations list to contain the found mutation")
			t.Log(got)
		})
	}
}

func filenameFromFixture(fix string) string {
	return strings.ReplaceAll(fix, "_go", ".go")
}

func TestSkipTestAndNonGoFiles(t *testing.T) {
	t.Parallel()
	f, _ := os.Open("testdata/fixtures/geq_go")
	file, _ := ioutil.ReadAll(f)

	sys := fstest.MapFS{
		"file_test.go": {Data: file},
		"folder1/file": {Data: file},
	}
	mut := mutator.New(sys, nil)
	got := mut.Run()

	if len(got) != 0 {
		t.Errorf("should not receive results")
	}
}

func TestMutationStatusString(t *testing.T) {
	testCases := []struct {
		name           string
		mutationStatus mutator.MutantStatus
		expected       string
	}{
		{
			"NotCovered",
			mutator.NotCovered,
			"NOT COVERED",
		},
		{
			"Runnable",
			mutator.Runnable,
			"RUNNABLE",
		},
		{
			"Lived",
			mutator.Lived,
			"LIVED",
		},
		{
			"Killed",
			mutator.Killed,
			"KILLED",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutationStatus.String() != tc.expected {
				t.Errorf(cmp.Diff(tc.mutationStatus.String(), tc.expected))
			}
		})
	}
}
