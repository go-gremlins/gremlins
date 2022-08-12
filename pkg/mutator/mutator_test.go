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
	"context"
	"errors"
	"fmt"
	"go/token"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/pkg/coverage"
	"github.com/go-gremlins/gremlins/pkg/mutant"
	"github.com/go-gremlins/gremlins/pkg/mutator"
)

var viperMutex sync.RWMutex

func init() {
	viperMutex.Lock()
	viperReset()
}

const defaultFixture = "testdata/fixtures/gtr_go"

func viperSet(set map[string]any) {
	viperMutex.Lock()
	for k, v := range set {
		configuration.Set(k, v)
	}
}

func viperReset() {
	configuration.Reset()
	for _, mt := range mutant.MutantTypes {
		configuration.Set(configuration.MutantTypeEnabledKey(mt), true)
	}
	viperMutex.Unlock()
}

const expectedTimeout = 10 * time.Second
const expectedModule = "example.com"

func coveredPosition(fixture string) coverage.Result {
	fn := filenameFromFixture(fixture)
	p := coverage.Profile{fn: {{StartLine: 6, EndLine: 7, StartCol: 8, EndCol: 9}}}

	return coverage.Result{Profile: p, Elapsed: expectedTimeout}
}

func notCoveredPosition(fixture string) coverage.Result {
	fn := filenameFromFixture(fixture)
	p := coverage.Profile{fn: {{StartLine: 9, EndLine: 9, StartCol: 8, EndCol: 9}}}

	return coverage.Result{Profile: p, Elapsed: expectedTimeout}
}

type dealerStub struct {
	t *testing.T
}

func (d dealerStub) Get() (string, func(), error) {
	return d.t.TempDir(), func() {}, nil
}

func TestMutations(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		fixture    string
		covResult  coverage.Result
		mutantType mutant.Type
		token      token.Token
		mutStatus  mutant.Status
	}{
		// CONDITIONAL BOUNDARIES
		{
			name:       "it recognizes CONDITIONAL_BOUNDARY with GTR",
			fixture:    "testdata/fixtures/gtr_go",
			mutantType: mutant.ConditionalsBoundary,
			token:      token.GTR,
			covResult:  coveredPosition("testdata/fixtures/gtr_go"),
			mutStatus:  mutant.Runnable,
		},
		{
			name:       "it recognizes CONDITIONAL_BOUNDARY with LSS",
			fixture:    "testdata/fixtures/lss_go",
			mutantType: mutant.ConditionalsBoundary,
			token:      token.LSS,
			covResult:  notCoveredPosition("testdata/fixtures/lss_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_BOUNDARY with LEQ",
			fixture:    "testdata/fixtures/leq_go",
			mutantType: mutant.ConditionalsBoundary,
			token:      token.LEQ,
			covResult:  notCoveredPosition("testdata/fixtures/leq_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_BOUNDARY with GEQ",
			fixture:    "testdata/fixtures/geq_go",
			mutantType: mutant.ConditionalsBoundary,
			token:      token.GEQ,
			covResult:  notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:  mutant.NotCovered,
		},
		// INCREMENT_DECREMENT
		{
			name:       "it recognizes INCREMENT_DECREMENT with INC",
			fixture:    "testdata/fixtures/inc_go",
			mutantType: mutant.IncrementDecrement,
			token:      token.INC,
			covResult:  notCoveredPosition("testdata/fixtures/inc_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes INCREMENT_DECREMENT with DEC",
			fixture:    "testdata/fixtures/dec_go",
			mutantType: mutant.IncrementDecrement,
			token:      token.DEC,
			covResult:  notCoveredPosition("testdata/fixtures/dec_go"),
			mutStatus:  mutant.NotCovered,
		},
		// CONDITIONAL_NEGATION
		{
			name:       "it recognizes CONDITIONAL_NEGATION with EQL",
			fixture:    "testdata/fixtures/eql_go",
			mutantType: mutant.ConditionalsNegation,
			token:      token.EQL,
			covResult:  notCoveredPosition("testdata/fixtures/eql_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with NEQ",
			fixture:    "testdata/fixtures/neq_go",
			mutantType: mutant.ConditionalsNegation,
			token:      token.NEQ,
			covResult:  notCoveredPosition("testdata/fixtures/neq_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with LEQ",
			fixture:    "testdata/fixtures/leq_go",
			mutantType: mutant.ConditionalsNegation,
			token:      token.LEQ,
			covResult:  notCoveredPosition("testdata/fixtures/leq_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with GTR",
			fixture:    "testdata/fixtures/gtr_go",
			mutantType: mutant.ConditionalsNegation,
			token:      token.GTR,
			covResult:  notCoveredPosition("testdata/fixtures/gtr_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with GEQ",
			fixture:    "testdata/fixtures/geq_go",
			mutantType: mutant.ConditionalsNegation,
			token:      token.GEQ,
			covResult:  notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes CONDITIONAL_NEGATION with LSS",
			fixture:    "testdata/fixtures/lss_go",
			mutantType: mutant.ConditionalsNegation,
			token:      token.LSS,
			covResult:  notCoveredPosition("testdata/fixtures/lss_go"),
			mutStatus:  mutant.NotCovered,
		},
		// INVERT_NEGATIVES
		{
			name:       "it recognizes INVERT_NEGATIVE with SUB",
			fixture:    "testdata/fixtures/negative_sub_go",
			mutantType: mutant.InvertNegatives,
			token:      token.SUB,
			covResult:  notCoveredPosition("testdata/fixtures/negative_sub_go"),
			mutStatus:  mutant.NotCovered,
		},
		// ARITHMETIC_BASIC
		{
			name:       "it recognizes ARITHMETIC_BASIC with ADD",
			fixture:    "testdata/fixtures/add_go",
			mutantType: mutant.ArithmeticBase,
			token:      token.ADD,
			covResult:  notCoveredPosition("testdata/fixtures/add_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes ARITHMETIC_BASIC with SUB",
			fixture:    "testdata/fixtures/sub_go",
			mutantType: mutant.ArithmeticBase,
			token:      token.SUB,
			covResult:  notCoveredPosition("testdata/fixtures/sub_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes ARITHMETIC_BASIC with MUL",
			fixture:    "testdata/fixtures/mul_go",
			mutantType: mutant.ArithmeticBase,
			token:      token.MUL,
			covResult:  notCoveredPosition("testdata/fixtures/mul_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes ARITHMETIC_BASIC with QUO",
			fixture:    "testdata/fixtures/quo_go",
			mutantType: mutant.ArithmeticBase,
			token:      token.QUO,
			covResult:  notCoveredPosition("testdata/fixtures/quo_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it recognizes ARITHMETIC_BASIC with REM",
			fixture:    "testdata/fixtures/rem_go",
			mutantType: mutant.ArithmeticBase,
			token:      token.REM,
			covResult:  notCoveredPosition("testdata/fixtures/rem_go"),
			mutStatus:  mutant.NotCovered,
		},
		// Common behaviours
		{
			name:       "it works with recursion",
			fixture:    "testdata/fixtures/geq_land_true_go",
			mutantType: mutant.ConditionalsBoundary,
			token:      token.GEQ,
			covResult:  notCoveredPosition("testdata/fixtures/geq_go"),
			mutStatus:  mutant.NotCovered,
		},
		{
			name:       "it skips illegal tokens",
			fixture:    "testdata/fixtures/illegal_go",
			mutantType: mutant.ConditionalsBoundary,
			token:      token.ILLEGAL,
			covResult:  notCoveredPosition("testdata/fixtures/illegal_go"),
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			viperSet(map[string]any{configuration.UnleashDryRunKey: true})
			defer viperReset()

			mapFS, mod, c := loadFixture(tc.fixture, ".")
			defer c()

			mut := mutator.New(mod, tc.covResult, dealerStub{t: t}, mutator.WithDirFs(mapFS))
			res := mut.Run(context.Background())
			got := res.Mutants

			if res.Module != expectedModule {
				t.Errorf("expected module to be %q, got %q", expectedModule, res.Module)
			}

			if tc.token == token.ILLEGAL {
				if len(got) != 0 {
					t.Errorf("expected no mutator found")
				}

				return
			}

			for _, g := range got {
				if g.Type() == tc.mutantType && g.Status() == tc.mutStatus && g.Pos() > 0 {
					// PASS
					return
				}
			}

			t.Errorf("expected tokenMutations list to contain the found mutation")
			t.Log(got)
		})
	}
}

func TestMutantSkipDisabled(t *testing.T) {
	t.Parallel()
	for _, mt := range mutant.MutantTypes {
		t.Run(mt.String(), func(t *testing.T) {
			t.Parallel()
			mapFS, mod, c := loadFixture(defaultFixture, ".")
			defer c()

			viperSet(map[string]any{
				configuration.UnleashDryRunKey:         true,
				configuration.MutantTypeEnabledKey(mt): false},
			)
			defer viperReset()

			mut := mutator.New(mod, coveredPosition(defaultFixture), dealerStub{t: t},
				mutator.WithExecContext(fakeExecCommandSuccess), mutator.WithDirFs(mapFS))
			res := mut.Run(context.Background())
			got := res.Mutants

			for _, m := range got {
				if m.Type() == mt {
					t.Fatalf("expected %q not to be in the found tokens", mt)
				}
			}
		})
	}
}

func TestSkipTestAndNonGoFiles(t *testing.T) {
	t.Parallel()
	f, _ := os.Open("testdata/fixtures/geq_go")
	file, _ := io.ReadAll(f)

	sys := fstest.MapFS{
		"file_test.go": {Data: file},
		"folder1/file": {Data: file},
	}
	mod := gomodule.GoModule{
		Name:       "example.com",
		Root:       ".",
		CallingDir: ".",
	}
	viperSet(map[string]any{configuration.UnleashDryRunKey: true})
	defer viperReset()
	mut := mutator.New(mod, coverage.Result{}, dealerStub{t: t}, mutator.WithDirFs(sys))
	res := mut.Run(context.Background())

	if got := res.Mutants; len(got) != 0 {
		t.Errorf("should not receive results")
	}
}

type commandHolder struct {
	command string
	args    []string
	timeout time.Duration
}

type execContext = func(ctx context.Context, name string, args ...string) *exec.Cmd

func TestMutatorRun(t *testing.T) {
	testCases := []struct {
		name     string
		fromPkg  string
		wantPath string
		intMode  bool
	}{
		{
			name:     "from root, normal mode",
			fromPkg:  ".",
			intMode:  false,
			wantPath: "example.com",
		},
		{
			name:     "from subpackage, normal mode",
			fromPkg:  "testdata/main/fixture",
			intMode:  false,
			wantPath: "example.com/testdata/main",
		},
		{
			name:     "from root, integration mode",
			fromPkg:  ".",
			intMode:  true,
			wantPath: "./...",
		},
		{
			name:     "from subpackage, integration mode",
			fromPkg:  "testdata/fixture",
			intMode:  true,
			wantPath: "./testdata/fixture/...",
		},
	}
	for _, tc := range testCases {

		t.Run(tc.name, func(t *testing.T) {
			viperSet(map[string]any{
				configuration.UnleashIntegrationMode: tc.intMode,
				configuration.UnleashTagsKey:         "tag1 tag2",
			})
			defer viperReset()
			mapFS, mod, c := loadFixture(defaultFixture, tc.fromPkg)
			defer c()

			holder := &commandHolder{}
			mut := mutator.New(mod, coveredPosition(defaultFixture), dealerStub{t: t},
				mutator.WithDirFs(mapFS),
				mutator.WithExecContext(fakeExecCommandSuccessWithHolder(holder)),
				mutator.WithApplyAndRollback(
					func(m mutant.Mutant) error {
						return nil
					},
					func(m mutant.Mutant) error {
						return nil
					}))

			_ = mut.Run(context.Background())

			want := "go test -tags tag1 tag2 -timeout 21s -failfast " + tc.wantPath
			got := fmt.Sprintf("go %v", strings.Join(holder.args, " "))

			if !cmp.Equal(got, want) {
				t.Errorf(cmp.Diff(got, want))
			}

			timeoutDifference := absTimeDiff(holder.timeout, expectedTimeout*2)
			diffThreshold := 70 * time.Microsecond
			if timeoutDifference > diffThreshold {
				t.Errorf("expected timeout to be within %s from the set timeout, got %s", diffThreshold, timeoutDifference)
			}
		})
	}
}

func absTimeDiff(a, b time.Duration) time.Duration {
	if a > b {
		return a - b
	}

	return b - a
}

func TestMutatorTestExecution(t *testing.T) {
	testCases := []struct {
		name          string
		fixture       string
		testResult    execContext
		covResult     coverage.Result
		wantMutStatus mutant.Status
	}{
		{
			name:          "it skips NOT_COVERED",
			fixture:       "testdata/fixtures/gtr_go",
			testResult:    fakeExecCommandSuccess,
			covResult:     notCoveredPosition("testdata/fixtures/gtr_go"),
			wantMutStatus: mutant.NotCovered,
		},
		{
			name:          "if tests pass then mutation is LIVED",
			fixture:       "testdata/fixtures/gtr_go",
			testResult:    fakeExecCommandSuccess,
			covResult:     coveredPosition("testdata/fixtures/gtr_go"),
			wantMutStatus: mutant.Lived,
		},
		{
			name:          "if tests fails then mutation is KILLED",
			fixture:       "testdata/fixtures/gtr_go",
			testResult:    fakeExecCommandTestsFailure,
			covResult:     coveredPosition("testdata/fixtures/gtr_go"),
			wantMutStatus: mutant.Killed,
		},
		{
			name:          "if build fails then mutation is BUILD FAILED",
			fixture:       "testdata/fixtures/gtr_go",
			testResult:    fakeExecCommandBuildFailure,
			covResult:     coveredPosition("testdata/fixtures/gtr_go"),
			wantMutStatus: mutant.NotViable,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			viperSet(map[string]any{configuration.UnleashDryRunKey: false})
			defer viperReset()
			mapFS, mod, c := loadFixture(tc.fixture, ".")
			defer c()

			mut := mutator.New(mod, tc.covResult, dealerStub{t: t},
				mutator.WithDirFs(mapFS),
				mutator.WithExecContext(tc.testResult),
				mutator.WithApplyAndRollback(
					func(m mutant.Mutant) error {
						return nil
					},
					func(m mutant.Mutant) error {
						return nil
					}))
			res := mut.Run(context.Background())
			got := res.Mutants

			if len(got) < 1 {
				t.Fatal("no mutants received")
			}
			if got[0].Status() != tc.wantMutStatus {
				t.Errorf("expected mutation to be %v, but got: %v", tc.wantMutStatus, got[0].Status())
			}
			if tc.wantMutStatus != mutant.NotCovered && res.Elapsed <= 0 {
				t.Errorf("expected elapsed time to be greater than zero, got %s", res.Elapsed)
			}
		})
	}
}

func TestApplyAndRollbackError(t *testing.T) {
	t.Run("apply fails", func(t *testing.T) {
		mapFS, mod, c := loadFixture(defaultFixture, ".")
		defer c()

		mut := mutator.New(mod, coveredPosition(defaultFixture), dealerStub{t: t},
			mutator.WithDirFs(mapFS),
			mutator.WithExecContext(fakeExecCommandSuccess),
			mutator.WithApplyAndRollback(
				func(m mutant.Mutant) error {
					return errors.New("test error")
				},
				func(m mutant.Mutant) error {
					return nil
				}))
		res := mut.Run(context.Background())
		got := res.Mutants

		if len(got) != 0 {
			t.Fatal("expected no mutants")
		}
	})

	t.Run("rollback fails", func(t *testing.T) {
		mapFS, mod, c := loadFixture(defaultFixture, ".")
		defer c()

		mut := mutator.New(mod, coveredPosition(defaultFixture), dealerStub{t: t},
			mutator.WithDirFs(mapFS),
			mutator.WithExecContext(fakeExecCommandSuccess),
			mutator.WithApplyAndRollback(
				func(m mutant.Mutant) error {
					return nil
				},
				func(m mutant.Mutant) error {
					return errors.New("test error")
				}))
		res := mut.Run(context.Background())
		got := res.Mutants

		if len(got) < 1 { // For now, in case of rollback failure, we expect the mutations still to be reported.
			t.Fatal("expected no mutants")
		}
	})
}

func TestStopsOnCancel(t *testing.T) {
	mapFS, mod, c := loadFixture(defaultFixture, ".")
	defer c()

	mut := mutator.New(mod, coveredPosition(defaultFixture), dealerStub{t: t},
		mutator.WithDirFs(mapFS),
		mutator.WithExecContext(fakeExecCommandSuccess),
		mutator.WithApplyAndRollback(
			func(m mutant.Mutant) error {
				return nil
			},
			func(m mutant.Mutant) error {
				return nil
			}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := mut.Run(ctx)

	if len(res.Mutants) > 0 {
		t.Errorf("expected to receive no mutants, got %d", len(res.Mutants))
	}
}

// loadFixture loads a fixture into a mapFS and returns also the GoModule.
//
//	fromPackage parameters can be path/pkgName.
func loadFixture(fixture, fromPackage string) (fstest.MapFS, gomodule.GoModule, func()) {
	f, _ := os.Open(fixture)
	src, _ := io.ReadAll(f)
	filename := filenameFromFixture(fixture)
	mapFS := fstest.MapFS{
		filename: {Data: src},
	}

	return mapFS, gomodule.GoModule{
			Name:       "example.com",
			Root:       ".",
			CallingDir: fromPackage,
		}, func() {
			_ = f.Close()
		}
}

func TestCoverageProcessSuccess(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}

func TestProcessTestsFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

func TestProcessBuildFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(2)
}

func fakeExecCommandSuccess(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
	cs = append(cs, args...)
	// #nosec G204 - We are in tests, we don't care
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}

	return cmd
}

func fakeExecCommandSuccessWithHolder(got *commandHolder) execContext {
	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		dl, _ := ctx.Deadline()
		if got != nil {
			got.command = command
			got.args = args
			got.timeout = time.Until(dl)
		}
		cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
		cs = append(cs, args...)

		return getCmd(ctx, cs)
	}
}

func fakeExecCommandTestsFailure(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessTestsFailure", "--", command}
	cs = append(cs, args...)

	return getCmd(ctx, cs)
}

func fakeExecCommandBuildFailure(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestProcessBuildFailure", "--", command}
	cs = append(cs, args...)

	return getCmd(ctx, cs)
}

func getCmd(ctx context.Context, cs []string) *exec.Cmd {
	// #nosec G204 - We are in tests, we don't care
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}

	return cmd
}

func filenameFromFixture(fix string) string {
	return strings.ReplaceAll(fix, "_go", ".go")
}
