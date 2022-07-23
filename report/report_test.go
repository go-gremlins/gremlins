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
	"github.com/google/go-cmp/cmp"
	"github.com/k3rn31/gremlins/log"
	"github.com/k3rn31/gremlins/mutant"
	"github.com/k3rn31/gremlins/report"
	"go/token"
	"testing"
	"time"
)

func TestReport(t *testing.T) {
	t.Run("it reports findings in normal run", func(t *testing.T) {
		out := &bytes.Buffer{}
		log.Init(out, &bytes.Buffer{})
		defer log.Reset()

		mutants := []mutant.Mutant{
			stubMutant{mutant.Lived, mutant.ConditionalsNegation},
			stubMutant{mutant.Killed, mutant.ConditionalsNegation},
			stubMutant{mutant.NotCovered, mutant.ConditionalsNegation},
		}
		data := report.Results{
			Mutants: mutants,
			Elapsed: (2 * time.Minute) + (22 * time.Second) + (123 * time.Millisecond),
		}

		report.Do(data)

		got := out.String()

		want := "\n" +
			// Limit the time reporting to the first two units (millis are excluded)
			"Mutation testing completed in 2 minutes 22 seconds\n" +
			"Killed: 1, Lived: 1, Not covered: 1\n" +
			"Test efficacy: 50.00%\n" +
			"Mutant coverage: 66.67%\n"

		if !cmp.Equal(got, want) {
			t.Errorf(cmp.Diff(want, got))
		}
	})

	t.Run("it reports findings in dry-run", func(t *testing.T) {
		out := &bytes.Buffer{}
		log.Init(out, &bytes.Buffer{})
		defer log.Reset()

		mutants := []mutant.Mutant{
			stubMutant{mutant.Runnable, mutant.ConditionalsNegation},
			stubMutant{mutant.Runnable, mutant.ConditionalsNegation},
			stubMutant{mutant.NotCovered, mutant.ConditionalsNegation},
		}
		data := report.Results{
			Mutants: mutants,
			Elapsed: (2 * time.Minute) + (22 * time.Second) + (123 * time.Millisecond),
		}

		report.Do(data)

		got := out.String()

		want := "\n" +
			// Limit the time reporting to the first two units (millis are excluded)
			"Dry run completed in 2 minutes 22 seconds\n" +
			"Runnable: 2, Not covered: 1\n" +
			"Mutant coverage: 66.67%\n"

		if !cmp.Equal(got, want) {
			t.Errorf(cmp.Diff(want, got))
		}
	})

	t.Run("it reports nothing if no result", func(t *testing.T) {
		out := &bytes.Buffer{}
		log.Init(out, &bytes.Buffer{})
		defer log.Reset()

		var mutants []mutant.Mutant
		data := report.Results{
			Mutants: mutants,
		}

		report.Do(data)

		got := out.String()

		want := "\n" +
			"No results to report.\n"

		if !cmp.Equal(got, want) {
			t.Errorf(cmp.Diff(want, got))
		}
	})
}

func TestMutantLog(t *testing.T) {
	out := &bytes.Buffer{}
	defer out.Reset()
	log.Init(out, &bytes.Buffer{})
	defer log.Reset()

	m := stubMutant{mutant.Lived, mutant.ConditionalsBoundary}
	report.Mutant(m)
	m = stubMutant{mutant.Killed, mutant.ConditionalsBoundary}
	report.Mutant(m)
	m = stubMutant{mutant.NotCovered, mutant.ConditionalsBoundary}
	report.Mutant(m)
	m = stubMutant{mutant.Runnable, mutant.ConditionalsBoundary}
	report.Mutant(m)

	got := out.String()

	want := "" +
		"       LIVED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"      KILLED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		" NOT COVERED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"    RUNNABLE CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n"

	if !cmp.Equal(got, want) {
		t.Errorf(cmp.Diff(got, want))
	}
}

type stubMutant struct {
	status     mutant.Status
	mutantType mutant.Type
}

func (s stubMutant) Type() mutant.Type {
	return s.mutantType
}

func (stubMutant) SetType(_ mutant.Type) {
	panic("implement me")
}

func (s stubMutant) Status() mutant.Status {
	return s.status
}

func (stubMutant) SetStatus(_ mutant.Status) {
	panic("implement me")
}

func (stubMutant) Position() token.Position {
	return token.Position{
		Filename: "aFolder/aFile.go",
		Offset:   0,
		Line:     12,
		Column:   3,
	}
}

func (stubMutant) Pos() token.Pos {
	return 123
}

func (stubMutant) SetWorkdir(_ string) {
	panic("implement me")
}

func (stubMutant) Apply() error {
	panic("implement me")
}

func (stubMutant) Rollback() error {
	panic("implement me")
}
