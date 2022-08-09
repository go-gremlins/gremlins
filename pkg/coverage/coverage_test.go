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
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/pkg/coverage"
)

type commandHolder struct {
	events []struct {
		command string
		args    []string
	}
}

func TestCoverageRun(t *testing.T) {
	viper.Set(configuration.UnleashTagsKey, "tag1 tag2")
	defer viper.Reset()

	wantWorkdir := "workdir"
	wantFilename := "coverage"
	wantFilePath := wantWorkdir + "/" + wantFilename
	holder := &commandHolder{}
	mod := gomodule.GoModule{
		Name:   "example.com",
		PkgDir: ".",
	}
	cov := coverage.NewWithCmd(fakeExecCommandSuccess(holder), wantWorkdir, mod)

	_, _ = cov.Run()

	firstWant := "go mod download"
	secondWant := fmt.Sprintf("go test -tags tag1 tag2 -cover -coverprofile %v ./...", wantFilePath)

	if len(holder.events) != 2 {
		t.Fatal("expected two commands to be executed")
	}
	firstGot := fmt.Sprintf("go %v", strings.Join(holder.events[0].args, " "))
	secondGot := fmt.Sprintf("go %v", strings.Join(holder.events[1].args, " "))

	if !cmp.Equal(firstGot, firstWant) {
		t.Errorf(cmp.Diff(firstGot, firstWant))
	}
	if !cmp.Equal(secondGot, secondWant) {
		t.Errorf(cmp.Diff(secondGot, secondWant))
	}
}

func TestCoverageRunFails(t *testing.T) {
	mod := gomodule.GoModule{
		Name:   "example.com",
		PkgDir: "./...",
	}

	t.Run("failure of: go mod download", func(t *testing.T) {
		cov := coverage.NewWithCmd(fakeExecCommandFailure(0), "workdir", mod)
		if _, err := cov.Run(); err == nil {
			t.Error("expected run to report an error")
		}
	})

	t.Run("failure of: go test", func(t *testing.T) {
		cov := coverage.NewWithCmd(fakeExecCommandFailure(1), "workdir", mod)
		if _, err := cov.Run(); err == nil {
			t.Error("expected run to report an error")
		}
	})
}

func TestCoverageParsesOutput(t *testing.T) {
	module := "example.com"
	mod := gomodule.GoModule{
		Name:   module,
		PkgDir: "path",
	}
	cov := coverage.NewWithCmd(fakeExecCommandSuccess(nil), "testdata/valid", mod)
	profile := coverage.Profile{
		"file1.go": {
			{
				StartLine: 47,
				StartCol:  2,
				EndLine:   48,
				EndCol:    16,
			},
		},
		"file2.go": {
			{
				StartLine: 52,
				StartCol:  2,
				EndLine:   53,
				EndCol:    16,
			},
		},
	}
	want := coverage.Result{
		Profile: profile,
	}

	got, err := cov.Run()
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(got.Profile, want.Profile) {
		t.Error(cmp.Diff(got, want))
	}
	if got.Elapsed == 0 {
		t.Errorf("expected elapsed time to be greater than 0")
	}
}

func TestParseOutputFail(t *testing.T) {
	mod := gomodule.GoModule{
		Name:   "example.com",
		PkgDir: "./...",
	}
	cov := coverage.NewWithCmd(fakeExecCommandSuccess(nil), "testdata/invalid", mod)

	if _, err := cov.Run(); err == nil {
		t.Errorf("espected an error")
	}
}

func TestCoverageProcessSuccess(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}

func TestCoverageProcessFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

type execContext = func(name string, args ...string) *exec.Cmd

func fakeExecCommandSuccess(got *commandHolder) execContext {
	return func(command string, args ...string) *exec.Cmd {
		if got != nil {
			got.events = append(got.events, struct {
				command string
				args    []string
			}{command: command, args: args})
		}
		cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
		cs = append(cs, args...)
		// #nosec G204 - We are in tests, we don't care
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_TEST_PROCESS=1"}

		return cmd
	}
}

func fakeExecCommandFailure(run int) execContext {
	var executed int

	return func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestCoverageProcessSuccess", "--", command}
		if executed == run {
			cs = []string{"-test.run=TestCoverageProcessFailure", "--", command}
		}
		cs = append(cs, args...)
		// #nosec G204 - We are in tests, we don't care
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_TEST_PROCESS=1"}
		executed++

		return cmd
	}
}
