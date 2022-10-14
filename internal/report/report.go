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

package report

import (
	"encoding/json"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/hako/durafmt"

	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/report/internal"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/execution"
)

var (
	fgRed      = color.New(color.FgRed).SprintFunc()
	fgGreen    = color.New(color.FgGreen).SprintFunc()
	fgHiGreen  = color.New(color.FgHiGreen).SprintFunc()
	fgHiBlack  = color.New(color.FgHiBlack).SprintFunc()
	fgHiYellow = color.New(color.FgYellow).SprintFunc()
)

// Results contains the list of mutator.Mutator to be reported
// and the time it took to discover and test them.
type Results struct {
	Module  string
	Mutants []mutator.Mutator
	Elapsed time.Duration
}

type reportStatus struct {
	files map[string][]internal.Mutation

	elapsed *durafmt.Durafmt
	module  string

	killed     int
	lived      int
	timedOut   int
	notCovered int
	notViable  int
	runnable   int

	mutatorStatistics internal.MutatorType

	tEfficacy float64
	mCovered  float64
}

func newReport(results Results) (*reportStatus, bool) {
	if len(results.Mutants) == 0 {

		return nil, false
	}
	rep := &reportStatus{
		module:  results.Module,
		elapsed: durafmt.Parse(results.Elapsed).LimitFirstN(2),
	}
	rep.files = make(map[string][]internal.Mutation)
	for _, m := range results.Mutants {
		rep.files[m.Position().Filename] = append(rep.files[m.Position().Filename], internal.Mutation{
			Line:   m.Position().Line,
			Column: m.Position().Column,
			Type:   m.Type().String(),
			Status: m.Status().String(),
		})

		reportMutationStatus(m, rep)
		reportMutatorType(m, rep)
	}
	if !rep.isDryRun() {
		if rep.killed > 0 {
			rep.tEfficacy = float64(rep.killed) / float64(rep.killed+rep.lived) * 100
		}
		if rep.killed+rep.lived > 0 {
			rep.mCovered = float64(rep.killed+rep.lived) / float64(rep.killed+rep.lived+rep.notCovered) * 100
		}
	} else if rep.runnable > 0 {
		rep.mCovered = float64(rep.runnable) / float64(rep.runnable+rep.notCovered) * 100
	}

	return rep, true
}

func reportMutationStatus(m mutator.Mutator, rep *reportStatus) {
	switch m.Status() {
	case mutator.Killed:
		rep.killed++
	case mutator.Lived:
		rep.lived++
	case mutator.NotCovered:
		rep.notCovered++
	case mutator.TimedOut:
		rep.timedOut++
	case mutator.NotViable:
		rep.notViable++
	case mutator.Runnable:
		rep.runnable++
	}
}

func reportMutatorType(m mutator.Mutator, rep *reportStatus) {
	switch m.Type() {
	case mutator.ArithmeticBase:
		rep.mutatorStatistics.ArithmeticBase++
	case mutator.ConditionalsNegation:
		rep.mutatorStatistics.ConditionalsNegation++
	case mutator.ConditionalsBoundary:
		rep.mutatorStatistics.ConditionalsBoundary++
	case mutator.IncrementDecrement:
		rep.mutatorStatistics.IncrementDecrement++
	case mutator.InvertAssignments:
		rep.mutatorStatistics.InvertAssignments++
	case mutator.InvertBitwiseAssignments:
		rep.mutatorStatistics.InvertBitwiseAssignments++
	case mutator.InvertBitwise:
		rep.mutatorStatistics.InvertBitwise++
	case mutator.InvertLogical:
		rep.mutatorStatistics.InvertLogical++
	case mutator.InvertLoopCtrl:
		rep.mutatorStatistics.InvertLoopCtrl++
	case mutator.InvertNegatives:
		rep.mutatorStatistics.InvertNegatives++
	case mutator.RemoveSelfAssignments:
		rep.mutatorStatistics.RemoveSelfAssignments++
	}
}

func (*reportStatus) isDryRun() bool {
	return configuration.Get[bool](configuration.UnleashDryRunKey)
}

func (r *reportStatus) reportFindings() {
	if r.isDryRun() {
		r.dryRunReport()
	} else {
		r.fullRunReport()
	}
	r.fileReport()
}

func (r *reportStatus) fileReport() {
	if output := configuration.Get[string](configuration.UnleashOutputKey); output != "" {
		files := make([]internal.OutputFile, 0, len(r.files))
		for fName, mutations := range r.files {
			of := internal.OutputFile{Filename: fName}
			of.Mutations = append(of.Mutations, mutations...)
			files = append(files, of)
		}

		result := internal.OutputResult{
			GoModule:          r.module,
			TestEfficacy:      r.tEfficacy,
			MutationsCoverage: r.mCovered,
			MutantsTotal:      r.lived + r.killed + r.notViable,
			MutantsKilled:     r.killed,
			MutantsLived:      r.lived,
			MutantsNotViable:  r.notViable,
			MutantsNotCovered: r.notCovered,
			ElapsedTime:       r.elapsed.Duration().Seconds(),
			MutatorStatistics: r.mutatorStatistics,
			Files:             files,
		}

		jsonResult, _ := json.Marshal(result)
		f, err := os.Create(output)
		if err != nil {
			log.Errorf("impossible to write file: %s\n", err)
		}
		defer func(f *os.File) {
			_ = f.Close()
		}(f)
		if _, err := f.Write(jsonResult); err != nil {
			log.Errorf("impossible to write file: %s\n", err)
		}

	}
}

func (r *reportStatus) dryRunReport() {
	notCovered := fgHiYellow(r.notCovered)
	runnable := fgGreen(r.runnable)
	log.Infoln("")
	log.Infof("Dry run completed in %s\n", r.elapsed.String())
	log.Infof("Runnable: %s, Not covered: %s\n", runnable, notCovered)
	log.Infof("Mutator coverage: %.2f%%\n", r.mCovered)
}

func (r *reportStatus) fullRunReport() {
	killed := fgHiGreen(r.killed)
	lived := fgRed(r.lived)
	timedOut := fgGreen(r.timedOut)
	notViable := fgHiBlack(r.notViable)
	notCovered := fgHiYellow(r.notCovered)
	log.Infoln("")
	log.Infof("Mutation testing completed in %s\n", r.elapsed.String())
	log.Infof("Killed: %s, Lived: %s, Not covered: %s\n", killed, lived, notCovered)
	log.Infof("Timed out: %s, Not viable: %s\n", timedOut, notViable)
	log.Infof("Test efficacy: %.2f%%\n", r.tEfficacy)
	log.Infof("Mutator coverage: %.2f%%\n", r.mCovered)
}

func (r *reportStatus) assess(tEfficacy, rCoverage float64) error {
	if r.isDryRun() {
		return nil
	}

	et := configuration.Get[float64](configuration.UnleashThresholdEfficacyKey)
	if et == 0 {
		et = float64(configuration.Get[int](configuration.UnleashThresholdEfficacyKey))
	}
	if et > 0 && tEfficacy <= et {
		return execution.NewExitErr(execution.EfficacyThreshold)
	}
	ct := configuration.Get[float64](configuration.UnleashThresholdMCoverageKey)
	if ct == 0 {
		ct = float64(configuration.Get[int](configuration.UnleashThresholdMCoverageKey))
	}
	if ct > 0 && rCoverage <= ct {
		return execution.NewExitErr(execution.MutantCoverageThreshold)
	}

	return nil
}

// Do generates the report of the Results received.
// This function uses the log package in gremlins to write to the
// chosen io.Writer, so it is necessary to call log.Init before
// the report generation.
func Do(results Results) error {
	rep, ok := newReport(results)
	if !ok {
		log.Infoln("\nNo results to report.")

		return nil
	}
	rep.reportFindings()

	return rep.assess(rep.tEfficacy, rep.mCovered)
}

// Mutant logs a mutator.Mutator.
// It reports the mutant.Status, the mutator.Type and its position.
// This function uses the log package in gremlins to write to the
// chosen io.Writer, so it is necessary to call log.Init before
// the report generation.
func Mutant(m mutator.Mutator) {
	status := m.Status().String()
	switch m.Status() {
	case mutator.Killed, mutator.Runnable:
		status = fgHiGreen(m.Status())
	case mutator.Lived:
		status = fgRed(m.Status())
	case mutator.NotCovered:
		status = fgHiYellow(m.Status())
	case mutator.TimedOut:
		status = fgGreen(m.Status())
	case mutator.NotViable:
		status = fgHiBlack(m.Status())
	}
	log.Infof("%s%s %s at %s\n", padding(m.Status()), status, m.Type(), m.Position())
}

func padding(s mutator.Status) string {
	var pad string
	padLen := 12 - len(s.String())
	for i := 0; i < padLen; i++ {
		pad += " "
	}

	return pad
}
