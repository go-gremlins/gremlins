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

package engine

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

func TestGetTestArgs(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		buildTags         string
		testExecutionTime time.Duration
		testCPU           int
		integrationMode   bool
		pkg               string
		want              []string
	}{
		"should_not_include_tags_flag_when_build_tags_are_empty": {
			testExecutionTime: 10 * time.Second,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-timeout", "12s", "-failfast", "example.com/my/package"},
		},
		"should_include_tags_flag_when_build_tags_are_set": {
			buildTags:         "tag1,tag2",
			testExecutionTime: 10 * time.Second,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-tags", "tag1,tag2", "-timeout", "12s", "-failfast", "example.com/my/package"},
		},
		"should_compute_timeout_as_two_seconds_plus_execution_time": {
			testExecutionTime: 30 * time.Second,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-timeout", "32s", "-failfast", "example.com/my/package"},
		},
		"should_not_include_cpu_flag_when_test_cpu_is_zero": {
			testExecutionTime: 10 * time.Second,
			testCPU:           0,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-timeout", "12s", "-failfast", "example.com/my/package"},
		},
		"should_include_cpu_flag_when_test_cpu_is_nonzero": {
			testExecutionTime: 10 * time.Second,
			testCPU:           4,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-timeout", "12s", "-failfast", "-cpu", "4", "example.com/my/package"},
		},
		"should_use_package_path_when_integration_mode_is_disabled": {
			testExecutionTime: 10 * time.Second,
			integrationMode:   false,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-timeout", "12s", "-failfast", "example.com/my/package"},
		},
		"should_use_dot_dot_dot_path_when_integration_mode_is_enabled": {
			testExecutionTime: 10 * time.Second,
			integrationMode:   true,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-timeout", "12s", "-failfast", "./..."},
		},
		"should_include_all_flags_when_all_options_are_configured": {
			buildTags:         "integration",
			testExecutionTime: 10 * time.Second,
			testCPU:           2,
			integrationMode:   true,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-tags", "integration", "-timeout", "12s", "-failfast", "-cpu", "2", "./..."},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			sut := &mutantExecutor{
				buildTags:         tc.buildTags,
				testExecutionTime: tc.testExecutionTime,
				testCPU:           tc.testCPU,
				integrationMode:   tc.integrationMode,
			}

			if diff := cmp.Diff(tc.want, sut.getTestArgs(tc.pkg)); diff != "" {
				t.Errorf("getTestArgs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetTestFailedStatus(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		want     mutator.Status
		exitCode int
	}{
		"a failing test suite killed the mutant": {
			exitCode: 1,
			want:     mutator.Killed,
		},
		"a build failure means the mutant is not viable": {
			exitCode: 2,
			want:     mutator.NotViable,
		},
		// os/exec reports a negative exit code for a process that did not exit on
		// its own but was terminated by a signal — an OOM kill, for instance. That
		// run reached no verdict, so it is neither a surviving mutant nor a killed
		// one.
		"a signal-terminated run reached no verdict": {
			exitCode: -1,
			want:     mutator.Errored,
		},
		"any other exit code leaves the mutant alive": {
			exitCode: 3,
			want:     mutator.Lived,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := getTestFailedStatus(tc.exitCode); got != tc.want {
				t.Errorf("getTestFailedStatus(%d) = %s, want %s", tc.exitCode, got, tc.want)
			}
		})
	}
}

func TestTestBinaryTerminatedBySignal(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		output string
		want   bool
	}{
		// This is what `go test` prints when the test binary it spawned was killed
		// by a signal: the reason on its own line, then the ordinary FAIL summary,
		// and an exit code of 1 that is indistinguishable from a failing assertion.
		"go reports the test binary was signalled": {
			output: "signal: killed\nFAIL\toomtest\t2.991s\nFAIL\n",
			want:   true,
		},
		"a segmentation violation is reported the same way": {
			output: "signal: segmentation fault\nFAIL\tpkg\t0.004s\nFAIL\n",
			want:   true,
		},
		"an ordinary test failure is not": {
			output: "--- FAIL: TestSomething (0.00s)\n    a_test.go:9: boom\nFAIL\npkg\t0.005s\nFAIL\n",
			want:   false,
		},
		// A test that prints the words itself has not been signalled: the line only
		// counts where go puts it, immediately before the FAIL summary.
		"a test printing the words is not": {
			output: "signal: this is test output\nok\tpkg\t0.005s\n",
			want:   false,
		},
		"no output at all is not": {
			output: "",
			want:   false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := testBinaryTerminatedBySignal([]byte(tc.output)); got != tc.want {
				t.Errorf("testBinaryTerminatedBySignal() = %t, want %t", got, tc.want)
			}
		})
	}
}
