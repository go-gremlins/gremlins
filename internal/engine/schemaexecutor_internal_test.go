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

package engine

import (
	"bytes"
	"errors"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

func TestClassifyDirect(t *testing.T) {
	t.Parallel()

	exitErr := errors.New("exit status 1")
	pos := token.Position{Filename: "a.go", Line: 3, Column: 4}

	testCases := map[string]struct {
		err         error
		exitCode    int
		sawTimeout  bool
		reached     bool
		deadlineHit bool
		cancelled   bool
		want        mutator.Status
	}{
		"test_timeout_marker":                {err: exitErr, exitCode: 2, sawTimeout: true, reached: true, want: mutator.RunTimedOut},
		"timeout_marker_wins_over_deadline":  {err: exitErr, exitCode: -1, sawTimeout: true, deadlineHit: true, want: mutator.RunTimedOut},
		"timeout_marker_wins_over_cancelled": {err: exitErr, exitCode: 2, sawTimeout: true, cancelled: true, want: mutator.RunTimedOut},
		"marker_without_error_is_ignored":    {exitCode: 0, sawTimeout: true, reached: true, want: mutator.Lived},
		"deadline_hit":                       {err: exitErr, exitCode: -1, reached: true, deadlineHit: true, want: mutator.TimedOut},
		"deadline_wins_over_cancelled":       {err: exitErr, exitCode: -1, deadlineHit: true, cancelled: true, want: mutator.TimedOut},
		"signalled":                          {err: exitErr, exitCode: -1, reached: true, want: mutator.Errored},
		"start_failure":                      {err: exitErr, exitCode: -1, want: mutator.Errored},
		"pass_reached":                       {exitCode: 0, reached: true, want: mutator.Lived},
		"pass_without_reach":                 {exitCode: 0, want: mutator.NotCovered},
		"fail_reached":                       {err: exitErr, exitCode: 1, reached: true, want: mutator.Killed},
		"fail_without_reach":                 {err: exitErr, exitCode: 1, want: mutator.Errored},
		"panic_exit_2_reached":               {err: exitErr, exitCode: 2, reached: true, want: mutator.Killed},
		"panic_exit_2_without_reach":         {err: exitErr, exitCode: 2, want: mutator.Errored},
		"other_exit_code_reached":            {err: exitErr, exitCode: 3, reached: true, want: mutator.Killed},
		"other_exit_code_without_reach":      {err: exitErr, exitCode: 3, want: mutator.Errored},
		"os_exit_42_reached":                 {err: exitErr, exitCode: 42, reached: true, want: mutator.Killed},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := classifyDirect(tc.err, tc.exitCode, tc.sawTimeout, tc.reached, tc.deadlineHit, tc.cancelled, pos)
			if got != tc.want {
				t.Errorf("classifyDirect() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClassifyDirectCancelledTakesTheConfiguredStatus holds a cancelled run
// to the status --on-shutdown-status names, spelt out here as a literal for
// each setting: the default (NOT COVERED) is also what an unset or unknown
// value gives, so a classification that ignored the setting could not be
// told from one that read it. A cancelled run is that whatever the binary
// did, a reach included, except that its own timeout and the backstop win.
//
// It is not parallel: it sets the shutdown status in the configuration.
func TestClassifyDirectCancelledTakesTheConfiguredStatus(t *testing.T) {
	exitErr := errors.New("exit status 1")
	pos := token.Position{Filename: "a.go", Line: 3, Column: 4}
	for value, want := range map[string]mutator.Status{
		"not-run":   mutator.NotCovered,
		"timed-out": mutator.TimedOut,
		"lived":     mutator.Lived,
		"nonsense":  mutator.NotCovered,
	} {
		t.Run(value, func(t *testing.T) {
			configuration.Set(configuration.UnleashOnShutdownStatusKey, value)
			defer configuration.Set(configuration.UnleashOnShutdownStatusKey, "")
			for _, reached := range []bool{false, true} {
				if got := classifyDirect(exitErr, -1, false, reached, false, true, pos); got != want {
					t.Errorf("cancelled run (reached %v) with on-shutdown-status %q = %v, want %v", reached, value, got, want)
				}
			}
			if got := classifyDirect(exitErr, 1, true, true, false, true, pos); got != mutator.RunTimedOut {
				t.Errorf("a run that hit its own timeout while cancelled = %v, want RunTimedOut", got)
			}
		})
	}
}

// TestClassifyDirectLogsEveryErroredRow checks that a run classified ERRORED
// says so, naming the mutant and the cause, and that no other status logs:
// an ERRORED row is a broken baseline or environment, and its log line is the
// only account of it.
//
// It is not parallel: the logger is a process-wide singleton.
func TestClassifyDirectLogsEveryErroredRow(t *testing.T) {
	signalErr := errors.New("signal: killed")
	exitErr := errors.New("exit status 3")
	pos := token.Position{Filename: "a.go", Line: 3, Column: 4}
	testCases := map[string]struct {
		err      error
		exitCode int
		reached  bool
		want     mutator.Status
		logged   []string // substrings of the one log line; none for no line
	}{
		"signalled":            {err: signalErr, exitCode: -1, want: mutator.Errored, logged: []string{"a.go:3:4", "reached no verdict", "signal: killed"}},
		"signalled_reached":    {err: signalErr, exitCode: -1, reached: true, want: mutator.Errored, logged: []string{"a.go:3:4", "signal: killed"}},
		"failed_without_reach": {err: exitErr, exitCode: 3, want: mutator.Errored, logged: []string{"a.go:3:4", "without reaching", "exit 3"}},
		"killed":               {err: exitErr, exitCode: 3, reached: true, want: mutator.Killed},
		"lived":                {exitCode: 0, reached: true, want: mutator.Lived},
		"not_covered":          {exitCode: 0, want: mutator.NotCovered},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			log.Reset()
			log.Init(&buf, &buf)
			defer log.Reset()
			if got := classifyDirect(tc.err, tc.exitCode, false, tc.reached, false, false, pos); got != tc.want {
				t.Fatalf("status = %v, want %v", got, tc.want)
			}
			out := buf.String()
			if len(tc.logged) == 0 {
				if out != "" {
					t.Errorf("logged %q for a status that is not ERRORED", out)
				}

				return
			}
			if n := strings.Count(out, "\n"); n != 1 {
				t.Errorf("logged %d lines, want one: %q", n, out)
			}
			for _, sub := range tc.logged {
				if !strings.Contains(out, sub) {
					t.Errorf("log line %q lacks %q", out, sub)
				}
			}
		})
	}
}

func TestCombineRuns(t *testing.T) {
	t.Parallel()

	var (
		runTimedOut = runResult{status: mutator.RunTimedOut}
		timedOut    = runResult{status: mutator.TimedOut}
		signalled   = runResult{status: mutator.Errored, noVerdict: true}
		killed      = runResult{status: mutator.Killed}
		failed      = runResult{status: mutator.Errored}
		lived       = runResult{status: mutator.Lived}
		notCovered  = runResult{status: mutator.NotCovered}
	)
	testCases := map[string]struct {
		runs []runResult
		want mutator.Status
	}{
		"no_runs":                         {want: mutator.NotCovered},
		"run_timeout_beats_backstop":      {runs: []runResult{timedOut, runTimedOut}, want: mutator.RunTimedOut},
		"backstop_beats_no_verdict":       {runs: []runResult{signalled, timedOut}, want: mutator.TimedOut},
		"no_verdict_beats_kill":           {runs: []runResult{killed, signalled}, want: mutator.Errored},
		"kill_beats_unreached_failure":    {runs: []runResult{failed, killed}, want: mutator.Killed},
		"unreached_failure_beats_lived":   {runs: []runResult{lived, failed}, want: mutator.Errored},
		"reached_first":                   {runs: []runResult{lived, notCovered}, want: mutator.Lived},
		"reached_last":                    {runs: []runResult{notCovered, lived}, want: mutator.Lived},
		"none_reached":                    {runs: []runResult{notCovered, notCovered}, want: mutator.NotCovered},
		"kill_then_timeout":               {runs: []runResult{killed, runTimedOut}, want: mutator.RunTimedOut},
		"timeout_then_kill_order_ignored": {runs: []runResult{runTimedOut, killed, lived}, want: mutator.RunTimedOut},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := combineRuns(tc.runs); got != tc.want {
				t.Errorf("combineRuns() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOverlayCacheRemembersAFailure checks that a worker copy whose overlay
// cannot be written is not retried for each mutant: the second ask returns
// the first's error and touches nothing, while another worker copy, whose
// root is another key, is still tried. The failure here is a temp directory
// whose path has whitespace, which cannot go in GOFLAGS.
func TestOverlayCacheRemembersAFailure(t *testing.T) {
	t.Parallel()
	spaced := filepath.Join(t.TempDir(), "has space")
	if err := os.Mkdir(spaced, 0o700); err != nil {
		t.Fatal(err)
	}
	good := t.TempDir()
	build := &schemata.Build{Src: t.TempDir(), Rewritten: []string{"a.go"}}
	c := newOverlayCache()

	_, first := c.get(build, "/copy/one", spaced)
	if first == nil || !strings.Contains(first.Error(), "whitespace") {
		t.Fatalf("first get error = %v, want the whitespace error", first)
	}
	// Removed, the directory makes a retry fail differently (no such
	// directory), so a second get that returns the first's error did not retry.
	if err := os.RemoveAll(spaced); err != nil {
		t.Fatal(err)
	}
	if _, second := c.get(build, "/copy/one", spaced); !errors.Is(second, first) {
		t.Errorf("second get error = %v, want the first's, %v", second, first)
	}
	path, err := c.get(build, "/copy/two", good)
	if err != nil {
		t.Fatalf("another worker copy: %v", err)
	}
	if again, _ := c.get(build, "/copy/two", good); again != path {
		t.Errorf("a cached overlay changed: %q then %q", path, again)
	}
}

// TestRunOutputExplain feeds runOutput in the given writes and checks the
// account explain gives: failure lines found across write boundaries and
// past the head, their number and length bounded, a last line without a
// newline still read.
func TestRunOutputExplain(t *testing.T) {
	t.Parallel()
	manyFails := strings.Repeat("--- FAIL: TestX (0.00s)\n", maxFailureLines+3)
	longName := strings.Repeat("y", 2*maxFailureLineSize)
	testCases := map[string]struct {
		writes []string
		logged []string
		absent []string
	}{
		"split_across_writes": {
			writes: []string{"ok\n--- FA", "IL: TestSplit (0.0", "0s)\npan", "ic: boom\n"},
			logged: []string{"    failing: --- FAIL: TestSplit (0.00s)\n", "    failing: panic: boom\n", "        ok\n"},
		},
		"failures_bounded": {
			writes: []string{manyFails},
			logged: []string{"    failing: and 3 more\n"},
		},
		"failure_line_bounded": {
			writes: []string{"--- FAIL: Test" + longName + "\n"},
			logged: []string{"    failing: --- FAIL: Test" + longName[:maxFailureLineSize-len("--- FAIL: Test")] + "\n"},
		},
		"ends_mid_line": {
			writes: []string{"start\npanic: no newline"},
			logged: []string{"    failing: panic: no newline\n", "        panic: no newline\n"},
			absent: []string{"truncated"},
		},
		"no_output": {
			logged: []string{"p exited 1\n    no output\n"},
			absent: []string{"failing", "output ("},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			o := newRunOutput()
			for _, w := range tc.writes {
				if n, err := o.Write([]byte(w)); n != len(w) || err != nil {
					t.Fatalf("Write = %d, %v, want %d, nil", n, err, len(w))
				}
			}
			got := o.explain("p", 1)
			if !strings.HasPrefix(got, "p exited 1\n") {
				t.Errorf("explanation does not start with the package and exit code:\n%s", got)
			}
			if n := strings.Count(got, "    failing: "); n > maxFailureLines+1 {
				t.Errorf("%d failure lines, want at most %d and a count", n, maxFailureLines)
			}
			for _, s := range tc.logged {
				if !strings.Contains(got, s) {
					t.Errorf("explanation lacks %q:\n%s", s, got)
				}
			}
			for _, s := range tc.absent {
				if strings.Contains(got, s) {
					t.Errorf("explanation holds %q:\n%s", s, got)
				}
			}
		})
	}
}
