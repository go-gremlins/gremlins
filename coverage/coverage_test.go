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

package coverage_test

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/k3rn31/gremlins/coverage"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type commandHolder struct {
	command string
	args    []string
}

func TestCoverageRun(t *testing.T) {
	t.Parallel()
	wantWorkdir := "workdir"
	wantFilename := "coverage"
	wantFilePath := wantWorkdir + "/" + wantFilename
	holder := &commandHolder{}
	cov := coverage.NewWithCmdAndPackage(fakeExecCommandSuccess(holder), "example.com", wantWorkdir, ".")

	_, _ = cov.Run()

	want := fmt.Sprintf("go test -cover -coverprofile %v ./...", wantFilePath)
	got := fmt.Sprintf("go %v", strings.Join(holder.args, " "))

	if !cmp.Equal(got, want) {
		t.Errorf(cmp.Diff(got, want))
	}
}

func TestCoverageRunFails(t *testing.T) {
	t.Parallel()
	cov := coverage.NewWithCmdAndPackage(fakeExecCommandFailure, "example.com", "workdir", "./...")
	_, err := cov.Run()
	if err == nil {
		t.Error("expected run to report an error")
	}
}

func TestCoverageParsesOutput(t *testing.T) {
	t.Parallel()
	cov := coverage.NewWithCmdAndPackage(fakeExecCommandSuccess(nil), "example.com", "testdata/valid", "./...")
	want := coverage.Profile{
		"path/file1.go": {
			{
				StartLine: 47,
				StartCol:  2,
				EndLine:   48,
				EndCol:    16,
			},
			{
				StartLine: 48,
				StartCol:  4,
				EndLine:   49,
				EndCol:    20,
			},
		},
		"path2/file2.go": {
			{
				StartLine: 52,
				StartCol:  2,
				EndLine:   53,
				EndCol:    16,
			},
		},
	}

	got, err := cov.Run()
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(got, want) {
		t.Error(cmp.Diff(got, want))
	}
}

func TestParseOutputFail(t *testing.T) {
	t.Parallel()
	cov := coverage.NewWithCmdAndPackage(fakeExecCommandSuccess(nil), "example.com", "testdata/invalid", "./...")

	_, err := cov.Run()
	if err == nil {
		t.Errorf("espected an error")
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

type execContext = func(name string, args ...string) *exec.Cmd

func fakeExecCommandSuccess(got *commandHolder) execContext {
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
