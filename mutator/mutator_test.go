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
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/k3rn31/gremlins/coverage"
	"github.com/k3rn31/gremlins/mutator"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
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
		name       string
		fixture    string
		mutantType mutator.MutantType
		token      token.Token
		covProfile coverage.Profile
		mutStatus  mutator.MutantStatus
	}{
		// CONDITIONAL BOUNDARIES
		{
			name:       "it recognizes CONDITIONAL_BOUNDARY with GTR",
			fixture:    "testdata/fixtures/gtr_go",
			mutantType: mutator.ConditionalsBoundary,
			token:      token.GTR,
			covProfile: coveredPosition("testdata/fixtures/gtr_go"),
			mutStatus:  mutator.Runnable,
		},
		{
			name:       "it recognizes CONDITIONAL_BOUNDARY with LSS",
			fixture:    "testdata/fixtures/lss_go",
			mutantType: mutator.ConditionalsBoundary,
			token:      token.LSS,
			covProfile: notCoveredPosition("testdata/fixtures/lss_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_BOUNDARY with LEQ",
			fixture:    "testdata/fixtures/leq_go",
			mutantType: mutator.ConditionalsBoundary,
			token:      token.LEQ,
			covProfile: notCoveredPosition("testdata/fixtures/leq_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_BOUNDARY with GEQ",
			fixture:    "testdata/fixtures/geq_go",
			mutantType: mutator.ConditionalsBoundary,
			token:      token.GEQ,
			covProfile: notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:  mutator.NotCovered,
		},
		// INCREMENT_DECREMENT
		{
			name:       "it recognizes INCREMENT_DECREMENT with INC",
			fixture:    "testdata/fixtures/inc_go",
			mutantType: mutator.IncrementDecrement,
			token:      token.INC,
			covProfile: notCoveredPosition("testdata/fixtures/inc_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes INCREMENT_DECREMENT with DEC",
			fixture:    "testdata/fixtures/dec_go",
			mutantType: mutator.IncrementDecrement,
			token:      token.DEC,
			covProfile: notCoveredPosition("testdata/fixtures/dec_go"),
			mutStatus:  mutator.NotCovered,
		},
		// CONDITIONAL_NEGATION
		{
			name:       "it recognizes CONDITIONAL_NEGATION with EQL",
			fixture:    "testdata/fixtures/eql_go",
			mutantType: mutator.ConditionalsNegation,
			token:      token.EQL,
			covProfile: notCoveredPosition("testdata/fixtures/eql_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with NEQ",
			fixture:    "testdata/fixtures/neq_go",
			mutantType: mutator.ConditionalsNegation,
			token:      token.NEQ,
			covProfile: notCoveredPosition("testdata/fixtures/neq_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with LEQ",
			fixture:    "testdata/fixtures/leq_go",
			mutantType: mutator.ConditionalsNegation,
			token:      token.LEQ,
			covProfile: notCoveredPosition("testdata/fixtures/leq_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with GTR",
			fixture:    "testdata/fixtures/gtr_go",
			mutantType: mutator.ConditionalsNegation,
			token:      token.GTR,
			covProfile: notCoveredPosition("testdata/fixtures/gtr_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with GEQ",
			fixture:    "testdata/fixtures/geq_go",
			mutantType: mutator.ConditionalsNegation,
			token:      token.GEQ,
			covProfile: notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with LSS",
			fixture:    "testdata/fixtures/lss_go",
			mutantType: mutator.ConditionalsNegation,
			token:      token.LSS,
			covProfile: notCoveredPosition("testdata/fixtures/lss_go"),
			mutStatus:  mutator.NotCovered,
		},
		// INVERT_NEGATIVES
		{
			name:       "it recognizes INVERT_NEGATIVE with SUB",
			fixture:    "testdata/fixtures/negative_sub_go",
			mutantType: mutator.InvertNegatives,
			token:      token.SUB,
			covProfile: notCoveredPosition("testdata/fixtures/negative_sub_go"),
			mutStatus:  mutator.NotCovered,
		},
		// ARITHMETIC_BASIC
		{
			name:       "it recognizes ARITHMETIC_BASIC with ADD",
			fixture:    "testdata/fixtures/add_go",
			mutantType: mutator.ArithmeticBase,
			token:      token.ADD,
			covProfile: notCoveredPosition("testdata/fixtures/add_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes ARITHMETIC_BASIC with SUB",
			fixture:    "testdata/fixtures/sub_go",
			mutantType: mutator.ArithmeticBase,
			token:      token.SUB,
			covProfile: notCoveredPosition("testdata/fixtures/sub_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes ARITHMETIC_BASIC with MUL",
			fixture:    "testdata/fixtures/mul_go",
			mutantType: mutator.ArithmeticBase,
			token:      token.MUL,
			covProfile: notCoveredPosition("testdata/fixtures/mul_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes ARITHMETIC_BASIC with QUO",
			fixture:    "testdata/fixtures/quo_go",
			mutantType: mutator.ArithmeticBase,
			token:      token.QUO,
			covProfile: notCoveredPosition("testdata/fixtures/quo_go"),
			mutStatus:  mutator.NotCovered,
		},
		{
			name:       "it recognizes ARITHMETIC_BASIC with REM",
			fixture:    "testdata/fixtures/rem_go",
			mutantType: mutator.ArithmeticBase,
			token:      token.REM,
			covProfile: notCoveredPosition("testdata/fixtures/rem_go"),
			mutStatus:  mutator.NotCovered,
		},
		// Common behaviours
		{
			name:       "it works with recursion",
			fixture:    "testdata/fixtures/geq_land_true_go",
			mutantType: mutator.ConditionalsBoundary,
			token:      token.GEQ,
			covProfile: notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:  mutator.NotCovered,
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
			mut := mutator.New(mapFS, tc.covProfile, mutator.WithDryRun(true))
			got := mut.Run()

			if tc.token == token.ILLEGAL {
				if len(got) != 0 {
					t.Errorf("expected no mutator found")
				}
				return
			}

			for _, g := range got {
				if g.Type == tc.mutantType && g.Status == tc.mutStatus && g.TokPos > 0 {
					// PASS
					return
				}
			}

			t.Errorf("expected mutations list to contain the found mutation")
			t.Log(got)
		})
	}
}

func TestSkipTestAndNonGoFiles(t *testing.T) {
	t.Parallel()
	f, _ := os.Open("testdata/fixtures/geq_go")
	file, _ := ioutil.ReadAll(f)

	sys := fstest.MapFS{
		"file_test.go": {Data: file},
		"folder1/file": {Data: file},
	}
	mut := mutator.New(sys, nil, mutator.WithDryRun(true))
	got := mut.Run()

	if len(got) != 0 {
		t.Errorf("should not receive results")
	}
}

type commandHolder struct {
	command string
	args    []string
}

type execContext = func(name string, args ...string) *exec.Cmd

func TestMutatorRun(t *testing.T) {
	t.Parallel()
	f, _ := os.Open("testdata/fixtures/gtr_go")
	defer f.Close()
	src, _ := ioutil.ReadAll(f)
	filename := filenameFromFixture("testdata/fixtures/gtr_go")
	mapFS := fstest.MapFS{
		filename: {Data: src},
	}

	holder := &commandHolder{}
	mut := mutator.New(mapFS, coveredPosition("testdata/fixtures/gtr_go"),
		mutator.WithExecContext(fakeExecCommandSuccessWithHolder(holder)),
		mutator.WithBuildTags("tag1 tag2"),
		mutator.WithApplyAndRollback(
			func(m mutator.Mutant) error {
				return nil
			},
			func(m mutator.Mutant) error {
				return nil
			}))

	_ = mut.Run()

	want := "go test -timeout 5s -tags tag1 tag2 ./..."
	got := fmt.Sprintf("go %v", strings.Join(holder.args, " "))

	if !cmp.Equal(got, want) {
		t.Errorf(cmp.Diff(got, want))
	}
}

func TestMutatorTestExecution(t *testing.T) {
	testCases := []struct {
		name          string
		fixture       string
		testResult    execContext
		covProfile    coverage.Profile
		wantMutStatus mutator.MutantStatus
	}{
		{
			name:          "it skips NOT_COVERED",
			fixture:       "testdata/fixtures/gtr_go",
			testResult:    fakeExecCommandSuccess,
			covProfile:    notCoveredPosition("testdata/fixtures/gtr_go"),
			wantMutStatus: mutator.NotCovered,
		},
		{
			name:          "if tests pass then mutation is LIVED",
			fixture:       "testdata/fixtures/gtr_go",
			testResult:    fakeExecCommandSuccess,
			covProfile:    coveredPosition("testdata/fixtures/gtr_go"),
			wantMutStatus: mutator.Lived,
		},
		{
			name:          "if tests fails then mutation is KILLED",
			fixture:       "testdata/fixtures/gtr_go",
			testResult:    fakeExecCommandFailure,
			covProfile:    coveredPosition("testdata/fixtures/gtr_go"),
			wantMutStatus: mutator.Killed,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, _ := os.Open(tc.fixture)
			defer f.Close()
			src, _ := ioutil.ReadAll(f)
			filename := filenameFromFixture(tc.fixture)
			mapFS := fstest.MapFS{
				filename: {Data: src},
			}

			mut := mutator.New(mapFS, tc.covProfile,
				mutator.WithExecContext(tc.testResult),
				mutator.WithApplyAndRollback(
					func(m mutator.Mutant) error {
						return nil
					},
					func(m mutator.Mutant) error {
						return nil
					}))
			got := mut.Run()

			if len(got) < 1 {
				t.Fatal("no mutants received")
			}
			if got[0].Status != tc.wantMutStatus {
				t.Errorf("expected mutation to be %v, but got: %v", tc.wantMutStatus, got[0].Status)
			}
		})
	}
}

func TestCoverageProcessSuccess(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}

func TestCoverageProcessFailure(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

func fakeExecCommandSuccess(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
	cs = append(cs, args...)
	// #nosec G204 - We are in tests, we don't care
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	return cmd
}

func fakeExecCommandSuccessWithHolder(got *commandHolder) execContext {
	return func(command string, args ...string) *exec.Cmd {
		if got != nil {
			got.command = command
			got.args = args
		}
		cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
		cs = append(cs, args...)
		// #nosec G204 - We are in tests, we don't care
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_TEST_PROCESS=1"}

		return cmd
	}
}

func fakeExecCommandFailure(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestCoverageProcessFailure", "--", command}
	cs = append(cs, args...)
	// #nosec G204 - We are in tests, we don't care
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	return cmd
}

func filenameFromFixture(fix string) string {
	return strings.ReplaceAll(fix, "_go", ".go")
}
