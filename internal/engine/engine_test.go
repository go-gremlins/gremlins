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

package engine_test

import (
	"context"
	"go/token"
	"io"
	"os"
	"testing"
	"testing/fstest"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/diff"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

const (
	defaultFixture = "testdata/fixtures/gtr_go"
	expectedModule = "example.com"
)

var testCodeData = engine.CodeData{Cov: coveredPosition(defaultFixture).Profile}

func coveredPosition(fixture string) coverage.Result {
	fn := filenameFromFixture(fixture)
	p := coverage.Profile{fn: {{StartLine: 6, EndLine: 7, StartCol: 8, EndCol: 9}}}

	return coverage.Result{Profile: p, Elapsed: 10}
}

func notCoveredPosition(fixture string) coverage.Result {
	fn := filenameFromFixture(fixture)
	p := coverage.Profile{fn: {{StartLine: 9, EndLine: 9, StartCol: 8, EndCol: 9}}}

	return coverage.Result{Profile: p, Elapsed: 10}
}

type mutationsTest struct {
	name       string
	fixture    string
	covResult  coverage.Result
	mutantType mutator.Type
	token      token.Token
	mutStatus  mutator.Status
}

var mutationsTests = []mutationsTest{
	// CONDITIONAL BOUNDARIES
	{
		name:       "it recognizes CONDITIONAL_BOUNDARY with GTR",
		fixture:    "testdata/fixtures/gtr_go",
		mutantType: mutator.ConditionalsBoundary,
		token:      token.GTR,
		covResult:  coveredPosition("testdata/fixtures/gtr_go"),
		mutStatus:  mutator.Runnable,
	},
	{
		name:       "it recognizes CONDITIONAL_BOUNDARY with LSS",
		fixture:    "testdata/fixtures/lss_go",
		mutantType: mutator.ConditionalsBoundary,
		token:      token.LSS,
		covResult:  notCoveredPosition("testdata/fixtures/lss_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes CONDITIONAL_BOUNDARY with LEQ",
		fixture:    "testdata/fixtures/leq_go",
		mutantType: mutator.ConditionalsBoundary,
		token:      token.LEQ,
		covResult:  notCoveredPosition("testdata/fixtures/leq_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes CONDITIONAL_BOUNDARY with GEQ",
		fixture:    "testdata/fixtures/geq_go",
		mutantType: mutator.ConditionalsBoundary,
		token:      token.GEQ,
		covResult:  notCoveredPosition("testdata/fixtures/geq_go"),
		mutStatus:  mutator.NotCovered,
	},
	// INCREMENT_DECREMENT
	{
		name:       "it recognizes INCREMENT_DECREMENT with INC",
		fixture:    "testdata/fixtures/inc_go",
		mutantType: mutator.IncrementDecrement,
		token:      token.INC,
		covResult:  notCoveredPosition("testdata/fixtures/inc_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INCREMENT_DECREMENT with DEC",
		fixture:    "testdata/fixtures/dec_go",
		mutantType: mutator.IncrementDecrement,
		token:      token.DEC,
		covResult:  notCoveredPosition("testdata/fixtures/dec_go"),
		mutStatus:  mutator.NotCovered,
	},
	// CONDITIONAL_NEGATION
	{
		name:       "it recognizes CONDITIONAL_NEGATION with EQL",
		fixture:    "testdata/fixtures/eql_go",
		mutantType: mutator.ConditionalsNegation,
		token:      token.EQL,
		covResult:  notCoveredPosition("testdata/fixtures/eql_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes CONDITIONAL_NEGATION with NEQ",
		fixture:    "testdata/fixtures/neq_go",
		mutantType: mutator.ConditionalsNegation,
		token:      token.NEQ,
		covResult:  notCoveredPosition("testdata/fixtures/neq_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes CONDITIONAL_NEGATION with LEQ",
		fixture:    "testdata/fixtures/leq_go",
		mutantType: mutator.ConditionalsNegation,
		token:      token.LEQ,
		covResult:  notCoveredPosition("testdata/fixtures/leq_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes CONDITIONAL_NEGATION with GTR",
		fixture:    "testdata/fixtures/gtr_go",
		mutantType: mutator.ConditionalsNegation,
		token:      token.GTR,
		covResult:  notCoveredPosition("testdata/fixtures/gtr_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes CONDITIONAL_NEGATION with GEQ",
		fixture:    "testdata/fixtures/geq_go",
		mutantType: mutator.ConditionalsNegation,
		token:      token.GEQ,
		covResult:  notCoveredPosition("testdata/fixtures/geq_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes CONDITIONAL_NEGATION with LSS",
		fixture:    "testdata/fixtures/lss_go",
		mutantType: mutator.ConditionalsNegation,
		token:      token.LSS,
		covResult:  notCoveredPosition("testdata/fixtures/lss_go"),
		mutStatus:  mutator.NotCovered,
	},
	// INVERT_NEGATIVES
	{
		name:       "it recognizes INVERT_NEGATIVE with SUB",
		fixture:    "testdata/fixtures/negative_sub_go",
		mutantType: mutator.InvertNegatives,
		token:      token.SUB,
		covResult:  notCoveredPosition("testdata/fixtures/negative_sub_go"),
		mutStatus:  mutator.NotCovered,
	},
	// ARITHMETIC_BASIC
	{
		name:       "it recognizes ARITHMETIC_BASIC with ADD",
		fixture:    "testdata/fixtures/add_go",
		mutantType: mutator.ArithmeticBase,
		token:      token.ADD,
		covResult:  notCoveredPosition("testdata/fixtures/add_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes ARITHMETIC_BASIC with SUB",
		fixture:    "testdata/fixtures/sub_go",
		mutantType: mutator.ArithmeticBase,
		token:      token.SUB,
		covResult:  notCoveredPosition("testdata/fixtures/sub_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes ARITHMETIC_BASIC with MUL",
		fixture:    "testdata/fixtures/mul_go",
		mutantType: mutator.ArithmeticBase,
		token:      token.MUL,
		covResult:  notCoveredPosition("testdata/fixtures/mul_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes ARITHMETIC_BASIC with QUO",
		fixture:    "testdata/fixtures/quo_go",
		mutantType: mutator.ArithmeticBase,
		token:      token.QUO,
		covResult:  notCoveredPosition("testdata/fixtures/quo_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes ARITHMETIC_BASIC with REM",
		fixture:    "testdata/fixtures/rem_go",
		mutantType: mutator.ArithmeticBase,
		token:      token.REM,
		covResult:  notCoveredPosition("testdata/fixtures/rem_go"),
		mutStatus:  mutator.NotCovered,
	},
	// INVERT_LOGICAL
	{
		name:       "it recognizes INVERT_LOGICAL with LAND",
		fixture:    "testdata/fixtures/land_go",
		mutantType: mutator.InvertLogical,
		token:      token.LAND,
		covResult:  notCoveredPosition("testdata/fixtures/land_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_LOGICAL with LOR",
		fixture:    "testdata/fixtures/lor_go",
		mutantType: mutator.InvertLogical,
		token:      token.LOR,
		covResult:  notCoveredPosition("testdata/fixtures/lor_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_LOOPCTRL with CONTINUE",
		fixture:    "testdata/fixtures/loop_continue_go",
		mutantType: mutator.InvertLoopCtrl,
		token:      token.CONTINUE,
		covResult:  notCoveredPosition("testdata/fixtures/loop_continue_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_LOOPCTRL with BREAK",
		fixture:    "testdata/fixtures/loop_break_go",
		mutantType: mutator.InvertLoopCtrl,
		token:      token.BREAK,
		covResult:  notCoveredPosition("testdata/fixtures/loop_break_go"),
		mutStatus:  mutator.NotCovered,
	},
	// INVERT_ASSIGNMENTS
	{
		name:       "it recognizes INVERT_ASSIGNMENTS with ADD_ASSIGN",
		fixture:    "testdata/fixtures/add_assign_go",
		mutantType: mutator.InvertAssignments,
		token:      token.ADD_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/add_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_ASSIGNMENTS with SUB_ASSIGN",
		fixture:    "testdata/fixtures/sub_assign_go",
		mutantType: mutator.InvertAssignments,
		token:      token.SUB_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/sub_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_ASSIGNMENTS with MUL_ASSIGN",
		fixture:    "testdata/fixtures/mul_assign_go",
		mutantType: mutator.InvertAssignments,
		token:      token.MUL_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/mul_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_ASSIGNMENTS with QUO_ASSIGN",
		fixture:    "testdata/fixtures/quo_assign_go",
		mutantType: mutator.InvertAssignments,
		token:      token.QUO_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/quo_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_ASSIGNMENTS with REM_ASSIGN",
		fixture:    "testdata/fixtures/rem_assign_go",
		mutantType: mutator.InvertAssignments,
		token:      token.REM_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/rem_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	// INVERT_BITWISE
	{
		name:       "it recognizes INVERT_BITWISE with AND",
		fixture:    "testdata/fixtures/and_go",
		mutantType: mutator.InvertBitwise,
		token:      token.AND,
		covResult:  notCoveredPosition("testdata/fixtures/and_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BITWISE with OR",
		fixture:    "testdata/fixtures/or_go",
		mutantType: mutator.InvertBitwise,
		token:      token.OR,
		covResult:  notCoveredPosition("testdata/fixtures/or_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BITWISE with XOR",
		fixture:    "testdata/fixtures/xor_go",
		mutantType: mutator.InvertBitwise,
		token:      token.XOR,
		covResult:  notCoveredPosition("testdata/fixtures/xor_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BITWISE with AND_NOT",
		fixture:    "testdata/fixtures/and_not_go",
		mutantType: mutator.InvertBitwise,
		token:      token.AND_NOT,
		covResult:  notCoveredPosition("testdata/fixtures/and_not_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BITWISE with SHL",
		fixture:    "testdata/fixtures/shl_go",
		mutantType: mutator.InvertBitwise,
		token:      token.SHL,
		covResult:  notCoveredPosition("testdata/fixtures/shl_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BITWISE with SHR",
		fixture:    "testdata/fixtures/shr_go",
		mutantType: mutator.InvertBitwise,
		token:      token.SHR,
		covResult:  notCoveredPosition("testdata/fixtures/shr_go"),
		mutStatus:  mutator.NotCovered,
	},
	// REMOVE_SELF_ASSIGNMENTS
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with ADD_ASSIGN",
		fixture:    "testdata/fixtures/add_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.ADD_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/add_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with SUB_ASSIGN",
		fixture:    "testdata/fixtures/sub_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.SUB_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/sub_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with MUL_ASSIGN",
		fixture:    "testdata/fixtures/mul_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.MUL_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/mul_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with QUO_ASSIGN",
		fixture:    "testdata/fixtures/quo_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.QUO_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/quo_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with REM_ASSIGN",
		fixture:    "testdata/fixtures/rem_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.REM_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/rem_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with AND_ASSIGN",
		fixture:    "testdata/fixtures/and_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.AND_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/and_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with OR_ASSIGN",
		fixture:    "testdata/fixtures/or_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.OR_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/or_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with XOR_ASSIGN",
		fixture:    "testdata/fixtures/xor_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.XOR_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/xor_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with SHL_ASSIGN",
		fixture:    "testdata/fixtures/shl_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.SHL_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/shl_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with SHR_ASSIGN",
		fixture:    "testdata/fixtures/shr_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.SHR_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/shr_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes REMOVE_SELF_ASSIGNMENTS with AND_NOT_ASSIGN",
		fixture:    "testdata/fixtures/and_not_assign_go",
		mutantType: mutator.RemoveSelfAssignments,
		token:      token.AND_NOT_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/and_not_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	// INVERT_BWASSIGN
	{
		name:       "it recognizes INVERT_BWASSIGN with AND_ASSIGN",
		fixture:    "testdata/fixtures/and_assign_go",
		mutantType: mutator.InvertBitwiseAssignments,
		token:      token.AND_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/and_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BWASSIGN with OR_ASSIGN",
		fixture:    "testdata/fixtures/or_assign_go",
		mutantType: mutator.InvertBitwiseAssignments,
		token:      token.OR_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/or_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BWASSIGN with XOR_ASSIGN",
		fixture:    "testdata/fixtures/xor_assign_go",
		mutantType: mutator.InvertBitwiseAssignments,
		token:      token.XOR_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/xor_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BWASSIGN with AND_NOT_ASSIGN",
		fixture:    "testdata/fixtures/and_not_assign_go",
		mutantType: mutator.InvertBitwiseAssignments,
		token:      token.AND_NOT_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/and_not_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BWASSIGN with SHL_ASSIGN",
		fixture:    "testdata/fixtures/shl_assign_go",
		mutantType: mutator.InvertBitwiseAssignments,
		token:      token.SHL_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/shl_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it recognizes INVERT_BWASSIGN with SHR_ASSIGN",
		fixture:    "testdata/fixtures/shr_assign_go",
		mutantType: mutator.InvertBitwiseAssignments,
		token:      token.SHR_ASSIGN,
		covResult:  notCoveredPosition("testdata/fixtures/shr_assign_go"),
		mutStatus:  mutator.NotCovered,
	},
	// Common behaviours
	{
		name:       "it works with recursion",
		fixture:    "testdata/fixtures/geq_land_true_go",
		mutantType: mutator.ConditionalsBoundary,
		token:      token.GEQ,
		covResult:  notCoveredPosition("testdata/fixtures/geq_go"),
		mutStatus:  mutator.NotCovered,
	},
	{
		name:       "it skips illegal tokens",
		fixture:    "testdata/fixtures/illegal_go",
		mutantType: mutator.ConditionalsBoundary,
		token:      token.ILLEGAL,
		covResult:  notCoveredPosition("testdata/fixtures/illegal_go"),
	},
}

func TestMutations(t *testing.T) {
	t.Parallel()
	for _, tc := range mutationsTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			viperSet(map[string]any{configuration.UnleashDryRunKey: true})
			defer viperReset()

			mapFS, mod, c := loadFixture(tc.fixture, ".")
			defer c()

			mut := engine.New(mod, engine.CodeData{Cov: tc.covResult.Profile}, newJobDealerStub(t), engine.WithDirFs(mapFS))
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
	for _, mt := range mutator.Types {
		mt := mt
		t.Run(mt.String(), func(t *testing.T) {
			t.Parallel()
			mapFS, mod, c := loadFixture(defaultFixture, ".")
			defer c()

			viperSet(map[string]any{
				configuration.UnleashDryRunKey:         true,
				configuration.MutantTypeEnabledKey(mt): false,
			})
			defer viperReset()

			mut := engine.New(mod, testCodeData, newJobDealerStub(t), engine.WithDirFs(mapFS))
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
	mut := engine.New(mod, engine.CodeData{}, newJobDealerStub(t), engine.WithDirFs(sys))
	res := mut.Run(context.Background())

	if got := res.Mutants; len(got) != 0 {
		t.Errorf("should not receive results")
	}
}

func TestSkipNotDiffMutants(t *testing.T) {
	t.Parallel()
	f, _ := os.Open("testdata/fixtures/geq_go")
	file, _ := io.ReadAll(f)

	sys := fstest.MapFS{
		"file.go": {Data: file},
	}
	mod := gomodule.GoModule{
		Name:       "example.com",
		Root:       ".",
		CallingDir: ".",
	}
	viperSet(map[string]any{configuration.UnleashDryRunKey: true})
	defer viperReset()

	codeData := engine.CodeData{Diff: diff.Diff{
		"file.go": nil,
	}}
	mut := engine.New(mod, codeData, newJobDealerStub(t), engine.WithDirFs(sys))
	res := mut.Run(context.Background())

	if got := res.Mutants; len(got) == 0 {
		t.Errorf("should receive mutants")
	}

	for _, mutant := range res.Mutants {
		if mutant.Status() != mutator.Skipped {
			t.Errorf("all mutants should be skipped")
		}
	}
}

func TestStopsOnCancel(t *testing.T) {
	mapFS, mod, c := loadFixture(defaultFixture, ".")
	defer c()

	mut := engine.New(mod, testCodeData, newJobDealerStub(t), engine.WithDirFs(mapFS))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := mut.Run(ctx)

	if len(res.Mutants) > 0 {
		t.Errorf("expected to receive no mutants, got %d", len(res.Mutants))
	}
}

func TestPackageDiscovery(t *testing.T) {
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
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			viperSet(map[string]any{
				configuration.UnleashIntegrationMode: tc.intMode,
				configuration.UnleashTagsKey:         "tag1 tag2",
			})
			defer viperReset()
			mapFS, mod, c := loadFixture(defaultFixture, tc.fromPkg)
			defer c()

			jds := newJobDealerStub(t)
			mut := engine.New(mod, testCodeData, jds, engine.WithDirFs(mapFS))

			_ = mut.Run(context.Background())

			got := jds.gotMutants[0].Pkg()

			if got != tc.wantPath {
				t.Errorf("want %q, got %q", tc.wantPath, got)

			}
		})
	}
}
