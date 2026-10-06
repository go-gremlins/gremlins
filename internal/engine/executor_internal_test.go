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
	"strings"
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
		tests             []string
		want              []string
	}{
		"should_not_include_tags_flag_when_build_tags_are_empty": {
			testExecutionTime: 10 * time.Second,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-vet=off", "-timeout", "10s", "-failfast", "example.com/my/package"},
		},
		"should_include_tags_flag_when_build_tags_are_set": {
			buildTags:         "tag1,tag2",
			testExecutionTime: 10 * time.Second,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-vet=off", "-tags", "tag1,tag2", "-timeout", "10s", "-failfast", "example.com/my/package"},
		},
		"should_pass_the_run_bound_verbatim_so_compile_time_is_not_charged_to_it": {
			testExecutionTime: 30 * time.Second,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-vet=off", "-timeout", "30s", "-failfast", "example.com/my/package"},
		},
		"should_not_include_cpu_flag_when_test_cpu_is_zero": {
			testExecutionTime: 10 * time.Second,
			testCPU:           0,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-vet=off", "-timeout", "10s", "-failfast", "example.com/my/package"},
		},
		"should_include_cpu_flag_when_test_cpu_is_nonzero": {
			testExecutionTime: 10 * time.Second,
			testCPU:           4,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-vet=off", "-timeout", "10s", "-failfast", "-cpu", "4", "example.com/my/package"},
		},
		"should_use_package_path_when_integration_mode_is_disabled": {
			testExecutionTime: 10 * time.Second,
			integrationMode:   false,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-vet=off", "-timeout", "10s", "-failfast", "example.com/my/package"},
		},
		"should_use_dot_dot_dot_path_when_integration_mode_is_enabled": {
			testExecutionTime: 10 * time.Second,
			integrationMode:   true,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-vet=off", "-timeout", "10s", "-failfast", "./..."},
		},
		"should_include_all_flags_when_all_options_are_configured": {
			buildTags:         "integration",
			testExecutionTime: 10 * time.Second,
			testCPU:           2,
			integrationMode:   true,
			pkg:               "example.com/my/package",
			want:              []string{"test", "-vet=off", "-tags", "integration", "-timeout", "10s", "-failfast", "-cpu", "2", "./..."},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			sut := &mutantExecutor{
				buildTags:         tc.buildTags,
				testExecutionTime: tc.testExecutionTime,
				testCPU:           tc.testCPU,
				testSelection:     testSelection{integrationMode: tc.integrationMode},
			}

			sel := testRun{pkgs: []string{tc.pkg}, tests: tc.tests}
			if diff := cmp.Diff(tc.want, sut.getTestArgs(sel)); diff != "" {
				t.Errorf("getTestArgs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestOutputScanner pins the two properties the verdict classification depends
// on: a marker is recognised however the child's output happens to be chunked,
// and the scanner never grows with the volume it is fed.
func TestOutputScanner(t *testing.T) {
	t.Parallel()

	t.Run("recognises a marker split across writes", func(t *testing.T) {
		t.Parallel()

		// The pipe between `go test` and gremlins chunks at whatever boundary
		// the kernel chooses, so the marker is split at every offset in turn
		// rather than at one arbitrary place.
		for i := 1; i < len(testTimeoutMarker); i++ {
			sut := newOutputScanner()
			mustWrite(t, sut, "some earlier output\n"+testTimeoutMarker[:i])
			mustWrite(t, sut, testTimeoutMarker[i:]+"10s\n\ngoroutine 1 [running]:\n")

			if !sut.sawTestTimeout() {
				t.Fatalf("marker split after %d bytes went unnoticed", i)
			}
			if sut.sawBuildFailure() {
				t.Fatalf("marker split after %d bytes was also read as a build failure", i)
			}
		}
	})

	t.Run("recognises a build failure and a setup failure", func(t *testing.T) {
		t.Parallel()

		for name, output := range map[string]string{
			"build": "# example.com/pkg [example.com/pkg.test]\n" +
				"./x.go:5:2: undefined: missing\nFAIL\texample.com/pkg [build failed]\n",
			"setup": "FAIL\texample.com/pkg [setup failed]\n",
		} {
			sut := newOutputScanner()
			mustWrite(t, sut, output)

			if !sut.sawBuildFailure() {
				t.Errorf("%s failure went unnoticed", name)
			}
			if sut.sawTestTimeout() {
				t.Errorf("%s failure was also read as a timeout", name)
			}
		}
	})

	t.Run("an ordinary failure trips neither marker", func(t *testing.T) {
		t.Parallel()

		sut := newOutputScanner()
		mustWrite(t, sut, "--- FAIL: TestX (0.00s)\n    x_test.go:9: boom\nFAIL\texample.com/pkg\t0.005s\n")

		if sut.sawTestTimeout() || sut.sawBuildFailure() {
			t.Error("a failing test must be left to the exit status, which makes it a detection")
		}
	})

	t.Run("retains a constant amount however much it is fed", func(t *testing.T) {
		t.Parallel()

		// A runaway mutant can print without bound; buffering that output would
		// trade the resource exhaustion this fork exists to prevent for another.
		sut := newOutputScanner()
		const chunk = 64 * 1024
		const chunks = 64
		for i := 0; i < chunks; i++ {
			mustWrite(t, sut, strings.Repeat("x", chunk))
		}

		if len(sut.tail) > sut.carry {
			t.Errorf("retained %d bytes after %d, want at most %d", len(sut.tail), chunk*chunks, sut.carry)
		}
	})
}

func mustWrite(t *testing.T, sut *outputScanner, s string) {
	t.Helper()

	n, err := sut.Write([]byte(s))
	if err != nil || n != len(s) {
		t.Fatalf("Write(%d bytes) = %d, %v", len(s), n, err)
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

func TestScannerSeesASignalledTestBinary(t *testing.T) {
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

			scanner := newOutputScanner()
			if _, err := scanner.Write([]byte(tc.output)); err != nil {
				t.Fatalf("write to the scanner: %v", err)
			}
			if got := scanner.sawSignalledTestBinary(); got != tc.want {
				t.Errorf("sawSignalledTestBinary() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestScannerSeesASignalledTestBinarySplitAcrossWrites pins the reason the
// scanner keeps a tail at all. The marker is two lines with a variable middle,
// and the child writes when it pleases, so the pattern arrives split far more
// often than not.
func TestScannerSeesASignalledTestBinarySplitAcrossWrites(t *testing.T) {
	t.Parallel()

	scanner := newOutputScanner()
	for _, chunk := range []string{"signal: ", "killed\nFA", "IL\tpkg\t2.9s\nFAIL\n"} {
		if _, err := scanner.Write([]byte(chunk)); err != nil {
			t.Fatalf("write to the scanner: %v", err)
		}
	}
	if !scanner.sawSignalledTestBinary() {
		t.Error("sawSignalledTestBinary() = false for a marker split across three writes, want true")
	}
}
