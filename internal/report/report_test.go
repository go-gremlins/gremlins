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

package report_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hectane/go-acl"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/execution"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/report"
	"github.com/go-gremlins/gremlins/internal/report/internal"

	"github.com/go-gremlins/gremlins/internal/configuration"
)

var fakePosition = newPosition("aFolder/aFile.go", 3, 12)

func TestReport(t *testing.T) {
	const (
		testingLine  = "Mutation testing completed in 2 minutes 22 seconds\n"
		coverageLine = "Mutator coverage: 0.00%\n"
	)

	nrTestCases := []struct {
		name    string
		mutants []mutator.Mutator
		want    string
	}{
		{
			name: "reports findings in normal run",
			mutants: []mutator.Mutator{
				stubMutant{status: mutator.Lived, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.Killed, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.NotCovered, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.NotViable, mutantType: mutator.ConditionalsBoundary, position: fakePosition},
				stubMutant{status: mutator.TimedOut, mutantType: mutator.ConditionalsBoundary, position: fakePosition},
				stubMutant{status: mutator.Skipped, mutantType: mutator.ConditionalsBoundary, position: fakePosition},
			},
			want: "\n" +
				// Limit the time reporting to the first two units (millis are excluded)
				testingLine +
				"Killed: 1, Lived: 1, Not covered: 1\n" +
				"Timed out: 1, Not viable: 1, Skipped: 1\n" +
				"Test efficacy: 50.00%\n" +
				"Mutator coverage: 66.67%\n",
		},
		{
			name: "reports findings with no coverage",
			mutants: []mutator.Mutator{
				stubMutant{status: mutator.NotCovered, mutantType: mutator.ConditionalsNegation, position: fakePosition},
			},
			want: "\n" +
				// Limit the time reporting to the first two units (millis are excluded)
				testingLine +
				"Killed: 0, Lived: 0, Not covered: 1\n" +
				"Timed out: 0, Not viable: 0, Skipped: 0\n" +
				"Test efficacy: 0.00%\n" +
				coverageLine,
		},
		{
			name: "reports findings with timeouts",
			mutants: []mutator.Mutator{
				stubMutant{status: mutator.TimedOut, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.TimedOut, mutantType: mutator.ConditionalsBoundary, position: fakePosition},
			},
			want: "\n" +
				// Limit the time reporting to the first two units (millis are excluded)
				testingLine +
				"Killed: 0, Lived: 0, Not covered: 0\n" +
				"Timed out: 2, Not viable: 0, Skipped: 0\n" +
				"Test efficacy: 0.00%\n" +
				coverageLine,
		},
		{
			name:    "reports nothing if no result",
			mutants: []mutator.Mutator{},
			want: "\n" +
				"No results to report.\n",
		},
	}
	for _, tc := range nrTestCases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			log.Init(out, &bytes.Buffer{})
			defer log.Reset()

			data := report.Results{
				Mutants: tc.mutants,
				Elapsed: (2 * time.Minute) + (22 * time.Second) + (123 * time.Millisecond),
			}

			_ = report.Do(data)

			got := out.String()

			if !cmp.Equal(got, tc.want) {
				t.Errorf("%s", cmp.Diff(tc.want, got))
			}
		})
	}

	drTestCases := []struct {
		name    string
		mutants []mutator.Mutator
		want    string
	}{
		{
			name: "reports findings in dry-run",
			mutants: []mutator.Mutator{
				stubMutant{status: mutator.Runnable, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.Runnable, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.NotCovered, mutantType: mutator.ConditionalsNegation, position: fakePosition},
			},
			want: "\n" +
				// Limit the time reporting to the first two units (millis are excluded)
				"Dry run completed in 2 minutes 22 seconds\n" +
				"Runnable: 2, Not covered: 1\n" +
				"Mutator coverage: 66.67%\n",
		},
		{
			name: "reports coverage zero in dry-run with timeout",
			mutants: []mutator.Mutator{
				stubMutant{status: mutator.TimedOut, mutantType: mutator.ConditionalsNegation, position: fakePosition},
			},
			want: "\n" +
				// Limit the time reporting to the first two units (millis are excluded)
				"Dry run completed in 2 minutes 22 seconds\n" +
				"Runnable: 0, Not covered: 0\n" +
				coverageLine,
		},
	}
	for _, tc := range drTestCases {
		t.Run(tc.name, func(t *testing.T) {
			viper.Set(configuration.UnleashDryRunKey, true)
			defer viper.Reset()

			out := &bytes.Buffer{}
			log.Init(out, &bytes.Buffer{})
			defer log.Reset()

			data := report.Results{
				Mutants: tc.mutants,
				Elapsed: (2 * time.Minute) + (22 * time.Second) + (123 * time.Millisecond),
			}

			_ = report.Do(data)

			got := out.String()

			if !cmp.Equal(got, tc.want) {
				t.Errorf("%s", cmp.Diff(tc.want, got))
			}
		})
	}
}

func newPosition(filename string, col, line int) token.Position {
	return token.Position{
		Filename: filename,
		Offset:   0,
		Line:     line,
		Column:   col,
	}
}

func TestAssessment(t *testing.T) {
	testCases := []struct {
		value       any
		name        string
		confKey     string
		expectError bool
	}{
		// Efficacy-threshold as float64
		{
			name:        "efficacy < efficacy-threshold",
			confKey:     configuration.UnleashThresholdEfficacyKey,
			value:       float64(51),
			expectError: true,
		},
		{
			name:        "efficacy >= efficacy-threshold",
			confKey:     configuration.UnleashThresholdEfficacyKey,
			value:       float64(50),
			expectError: false,
		},
		{
			name:        "efficacy-threshold == 0",
			confKey:     configuration.UnleashThresholdEfficacyKey,
			value:       float64(0),
			expectError: false,
		},
		// Efficacy-threshold as float64
		{
			name:        "efficacy < efficacy-threshold",
			confKey:     configuration.UnleashThresholdEfficacyKey,
			value:       51,
			expectError: true,
		},
		// Mutator coverage-threshold as float
		{
			name:        "coverage < coverage-threshold",
			confKey:     configuration.UnleashThresholdMCoverageKey,
			value:       float64(51),
			expectError: true,
		},
		{
			name:        "coverage >= coverage-threshold",
			confKey:     configuration.UnleashThresholdMCoverageKey,
			value:       float64(50),
			expectError: false,
		},
		{
			name:        "coverage-threshold == 0",
			confKey:     configuration.UnleashThresholdMCoverageKey,
			value:       float64(0),
			expectError: false,
		},
		// Mutator coverage-threshold as int
		{
			name:        "coverage < coverage-threshold",
			confKey:     configuration.UnleashThresholdMCoverageKey,
			value:       51,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			log.Init(&bytes.Buffer{}, &bytes.Buffer{})
			defer log.Reset()

			viper.Set(tc.confKey, tc.value)
			defer viper.Reset()

			// Always 50%
			mutants := []mutator.Mutator{
				stubMutant{status: mutator.Killed, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.Lived, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.NotCovered, mutantType: mutator.ConditionalsNegation, position: fakePosition},
				stubMutant{status: mutator.NotCovered, mutantType: mutator.ConditionalsNegation, position: fakePosition},
			}
			data := report.Results{
				Mutants: mutants,
				Elapsed: 1 * time.Minute,
			}

			err := report.Do(data)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected an error")
				}
				var exitErr *execution.ExitError
				if errors.As(err, &exitErr) {
					if exitErr.ExitCode() == 0 {
						t.Errorf("expected exit code to be different from 0, got %d", exitErr.ExitCode())
					}
				} else {
					t.Errorf("expected err to be ExitError")
				}
			} else if err != nil {
				t.Fatal("unexpected error")
			}
		})
	}
}

func TestMutantLog(t *testing.T) {
	out := &bytes.Buffer{}
	defer out.Reset()
	log.Init(out, &bytes.Buffer{})
	defer log.Reset()

	m := stubMutant{status: mutator.Lived, mutantType: mutator.ConditionalsBoundary, position: fakePosition}
	report.Mutant(m)
	m = stubMutant{status: mutator.Killed, mutantType: mutator.ConditionalsBoundary, position: fakePosition}
	report.Mutant(m)
	m = stubMutant{status: mutator.NotCovered, mutantType: mutator.ConditionalsBoundary, position: fakePosition}
	report.Mutant(m)
	m = stubMutant{status: mutator.Runnable, mutantType: mutator.ConditionalsBoundary, position: fakePosition}
	report.Mutant(m)
	m = stubMutant{status: mutator.NotViable, mutantType: mutator.ConditionalsBoundary, position: fakePosition}
	report.Mutant(m)
	m = stubMutant{status: mutator.TimedOut, mutantType: mutator.ConditionalsBoundary, position: fakePosition}
	report.Mutant(m)
	m = stubMutant{status: mutator.Skipped, mutantType: mutator.ConditionalsBoundary, position: fakePosition}
	report.Mutant(m)

	got := out.String()

	want := "" +
		"       LIVED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"      KILLED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		" NOT COVERED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"    RUNNABLE CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"  NOT VIABLE CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"   TIMED OUT CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"     SKIPPED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n"

	if !cmp.Equal(got, want) {
		t.Errorf("%s", cmp.Diff(got, want))
	}
}

func TestReportToFile(t *testing.T) {
	outFile := "findings.json"
	mutants := []mutator.Mutator{
		stubMutant{status: mutator.Killed, mutantType: mutator.ConditionalsNegation, position: newPosition("file1.go", 3, 10)},
		stubMutant{status: mutator.Lived, mutantType: mutator.ArithmeticBase, position: newPosition("file1.go", 8, 20)},
		stubMutant{status: mutator.NotCovered, mutantType: mutator.IncrementDecrement, position: newPosition("file1.go", 7, 40)},
		stubMutant{status: mutator.NotViable, mutantType: mutator.InvertAssignments, position: newPosition("file1.go", 8, 10)},
		stubMutant{status: mutator.NotCovered, mutantType: mutator.InvertLoopCtrl, position: newPosition("file2.go", 3, 20)},
		stubMutant{status: mutator.Killed, mutantType: mutator.IncrementDecrement, position: newPosition("file2.go", 17, 44)},
		stubMutant{status: mutator.NotCovered, mutantType: mutator.ConditionalsBoundary, position: newPosition("file2.go", 3, 500)},
		stubMutant{status: mutator.Lived, mutantType: mutator.InvertBitwise, position: newPosition("file2.go", 3, 100)},
		stubMutant{status: mutator.Killed, mutantType: mutator.InvertBitwiseAssignments, position: newPosition("file2.go", 4, 10)},
		stubMutant{status: mutator.Lived, mutantType: mutator.InvertLogical, position: newPosition("file2.go", 4, 11)},
		stubMutant{status: mutator.NotViable, mutantType: mutator.InvertNegatives, position: newPosition("file3.go", 4, 200)},
		stubMutant{status: mutator.Killed, mutantType: mutator.RemoveSelfAssignments, position: newPosition("file3.go", 4, 100)},
	}
	data := report.Results{
		Module:  "example.com/go/module",
		Mutants: mutants,
		Elapsed: (2 * time.Minute) + (22 * time.Second) + (123 * time.Millisecond),
	}
	f, _ := os.ReadFile("testdata/normal_output.json")
	want := internal.OutputResult{}
	_ = json.Unmarshal(f, &want)

	t.Run("it writes on file when output is set", func(t *testing.T) {
		outDir := t.TempDir()
		output := filepath.Join(outDir, outFile)
		viper.Set(configuration.UnleashOutputKey, output)
		defer viper.Reset()

		if err := report.Do(data); err != nil {
			t.Fatal("error not expected")
		}

		file, err := os.ReadFile(output)
		if err != nil {
			t.Fatal("file not found")
		}

		var got internal.OutputResult
		err = json.Unmarshal(file, &got)
		if err != nil {
			t.Fatal("impossible to unmarshal results")
		}

		if !cmp.Equal(got, want, cmpopts.SortSlices(sortOutputFile), cmpopts.SortSlices(sortMutation)) {
			t.Errorf("%s", cmp.Diff(got, want))
		}
	})

	t.Run("it doesn't write on file when output isn't set", func(t *testing.T) {
		outDir := t.TempDir()
		output := filepath.Join(outDir, outFile)

		if err := report.Do(data); err != nil {
			t.Fatal("error not expected")
		}

		_, err := os.ReadFile(output)
		if err == nil {
			t.Errorf("expected file not found")
		}
	})

	// In this case we don't want to stop the execution with an error, but we just want to log the fact.
	t.Run("it doesn't report error when file is not writeable, but doesn't write file", func(t *testing.T) {
		outDir, cl := notWriteableDir(t)
		defer cl()
		output := filepath.Join(outDir, outFile)
		viper.Set(configuration.UnleashOutputKey, output)
		defer viper.Reset()

		if err := report.Do(data); err != nil {
			t.Fatal("error not expected")
		}

		_, err := os.ReadFile(output)
		if err == nil {
			t.Errorf("expected file not found")
		}
	})
}

func notWriteableDir(t *testing.T) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	outPath, _ := os.MkdirTemp(tmp, "test-")
	_ = os.Chmod(outPath, 0000)
	clean := os.Chmod
	if runtime.GOOS == "windows" {
		_ = acl.Chmod(outPath, 0000)
		clean = acl.Chmod
	}

	return outPath, func() {
		_ = clean(outPath, 0700)
	}
}

func sortOutputFile(x, y internal.OutputFile) bool {
	return x.Filename < y.Filename
}

func sortMutation(x, y internal.Mutation) bool {
	if x.Line == y.Line {

		return x.Column < y.Column
	}

	return x.Line < y.Line
}

type stubMutant struct {
	position   token.Position
	status     mutator.Status
	mutantType mutator.Type
}

func (s stubMutant) Type() mutator.Type {
	return s.mutantType
}

func (stubMutant) SetType(_ mutator.Type) {
	panic("implement me")
}

func (s stubMutant) Status() mutator.Status {
	return s.status
}

func (stubMutant) SetStatus(_ mutator.Status) {
	panic("implement me")
}

func (s stubMutant) Position() token.Position {
	return s.position
}

func (stubMutant) Pos() token.Pos {
	return 123
}

func (stubMutant) Pkg() string {
	panic("implement me")
}

func (stubMutant) SetWorkdir(_ string) {
	panic("implement me")
}

func (stubMutant) Workdir() string {
	panic("implement me")
}

func (stubMutant) Apply() error {
	panic("implement me")
}

func (stubMutant) Rollback() error {
	panic("implement me")
}
