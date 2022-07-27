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
	"time"

	"github.com/fatih/color"
	"github.com/go-gremlins/gremlins/pkg/log"
	"github.com/go-gremlins/gremlins/pkg/mutant"
	"github.com/hako/durafmt"
)

var (
	fgRed      = color.New(color.FgRed).SprintFunc()
	fgGreen    = color.New(color.FgGreen).SprintFunc()
	fgHiGreen  = color.New(color.FgHiGreen).SprintFunc()
	fgHiBlack  = color.New(color.FgHiBlack).SprintFunc()
	fgHiYellow = color.New(color.FgYellow).SprintFunc()
)

// Results contains the list of mutant.Mutant to be reported
// and the time it took to discover and test them.
type Results struct {
	Mutants []mutant.Mutant
	Elapsed time.Duration
}

// Do generates the report of the Results received.
// This function uses the log package in gremlins to write to the
// chosen io.Writer, so it is necessary to call log.Init before
// the report generation.
func Do(results Results) {
	if len(results.Mutants) == 0 {
		log.Infoln("\nNo results to report.")

		return
	}
	var k, l, t, nc, nv, r int
	for _, m := range results.Mutants {
		switch m.Status() {
		case mutant.Killed:
			k++
		case mutant.Lived:
			l++
		case mutant.NotCovered:
			nc++
		case mutant.TimedOut:
			t++
		case mutant.NotViable:
			nv++
		case mutant.Runnable:
			r++
		}
	}
	elapsed := durafmt.Parse(results.Elapsed).LimitFirstN(2)
	notCovered := fgHiYellow(nc)
	if r > 0 {
		runnable := fgGreen(r)
		rCoverage := float64(r) / float64(r+nc) * 100
		log.Infoln("")
		log.Infof("Dry run completed in %s\n", elapsed.String())
		log.Infof("Runnable: %s, Not covered: %s\n", runnable, notCovered)
		log.Infof("Mutant coverage: %.2f%%\n", rCoverage)

		return
	}
	tEfficacy := float64(k) / float64(k+l) * 100
	rCoverage := float64(k+l) / float64(k+l+nc) * 100
	killed := fgHiGreen(k)
	lived := fgRed(l)
	timedOut := fgGreen(t)
	notViable := fgHiBlack(nv)
	log.Infoln("")
	log.Infof("Mutation testing completed in %s\n", elapsed.String())
	log.Infof("Killed: %s, Lived: %s, Not covered: %s\n", killed, lived, notCovered)
	log.Infof("Timed out: %s, Not viable: %s\n", timedOut, notViable)
	log.Infof("Test efficacy: %.2f%%\n", tEfficacy)
	log.Infof("Mutant coverage: %.2f%%\n", rCoverage)
}

// Mutant logs a mutant.Mutant.
// It reports the mutant.Status, the mutant.Type and its position.
// This function uses the log package in gremlins to write to the
// chosen io.Writer, so it is necessary to call log.Init before
// the report generation.
func Mutant(m mutant.Mutant) {
	status := m.Status().String()
	switch m.Status() {
	case mutant.Killed, mutant.Runnable:
		status = fgHiGreen(m.Status())
	case mutant.Lived:
		status = fgRed(m.Status())
	case mutant.NotCovered:
		status = fgHiYellow(m.Status())
	case mutant.TimedOut:
		status = fgGreen(m.Status())
	case mutant.NotViable:
		status = fgHiBlack(m.Status())
	}
	log.Infof("%s%s %s at %s\n", padding(m.Status()), status, m.Type(), m.Position())
}

func padding(s mutant.Status) string {
	var pad string
	padLen := 12 - len(s.String())
	for i := 0; i < padLen; i++ {
		pad += " "
	}

	return pad
}
