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
		mutStatus     mutator.MutationStatus
	}{
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with GTR",
			fixture:       "testdata/fixtures/conditional_boundary_gtr_go",
			mutantType:    mutator.ConditionalBoundary,
			token:         token.GTR,
			tokenMutation: token.GEQ,
			covProfile:    coveredPosition("testdata/fixtures/conditional_boundary_gtr_go"),
			mutStatus:     mutator.Runnable,
		},
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with LSS",
			fixture:       "testdata/fixtures/conditional_boundary_lss_go",
			mutantType:    mutator.ConditionalBoundary,
			token:         token.LSS,
			tokenMutation: token.LEQ,
			covProfile:    notCoveredPosition("testdata/fixtures/conditional_boundary_lss_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with LEQ",
			fixture:       "testdata/fixtures/conditional_boundary_leq_go",
			mutantType:    mutator.ConditionalBoundary,
			token:         token.LEQ,
			tokenMutation: token.LSS,
			covProfile:    notCoveredPosition("testdata/fixtures/conditional_boundary_leq_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with GEQ",
			fixture:       "testdata/fixtures/conditional_boundary_geq_go",
			mutantType:    mutator.ConditionalBoundary,
			token:         token.GEQ,
			tokenMutation: token.GTR,
			covProfile:    notCoveredPosition("testdata/fixtures/conditional_boundary_geq_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:          "it recognizes CONDITIONAL_BOUNDARY with recursion",
			fixture:       "testdata/fixtures/conditional_boundary_geq_land_true_go",
			mutantType:    mutator.ConditionalBoundary,
			token:         token.GEQ,
			tokenMutation: token.GTR,
			covProfile:    notCoveredPosition("testdata/fixtures/conditional_boundary_geq_go"),
			mutStatus:     mutator.NotCovered,
		},
		{
			name:       "it skips illegal CONDITIONAL_BOUNDARY",
			fixture:    "testdata/fixtures/conditional_boundary_illegal_go",
			mutantType: mutator.ConditionalBoundary,
			token:      token.ILLEGAL,
			covProfile: notCoveredPosition("testdata/fixtures/conditional_boundary_illegal_go"),
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
				if g.MutantType == mutator.ConditionalBoundary &&
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
	f, _ := os.Open("testdata/fixtures/conditional_boundary_geq_go")
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
		mutationStatus mutator.MutationStatus
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
