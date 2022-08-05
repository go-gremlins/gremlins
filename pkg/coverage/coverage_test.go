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
	"github.com/go-gremlins/gremlins/pkg/coverage"
)

type commandHolder struct {
	command string
	args    []string
}

func TestCoverageRun(t *testing.T) {
	viper.Set(configuration.UnleashTagsKey, "tag1 tag2")
	defer viper.Reset()

	wantWorkdir := "workdir"
	wantFilename := "coverage"
	wantFilePath := wantWorkdir + "/" + wantFilename
	holder := &commandHolder{}
	cov := coverage.NewWithCmdAndPackage(
		fakeExecCommandSuccess(holder),
		"example.com",
		wantWorkdir,
		".")

	_, _ = cov.Run()

	want := fmt.Sprintf("go test -tags tag1 tag2 -cover -coverprofile %v ./...", wantFilePath)
	got := fmt.Sprintf("go %v", strings.Join(holder.args, " "))

	if !cmp.Equal(got, want) {
		t.Errorf(cmp.Diff(got, want))
	}
}

func TestCoverageRunFails(t *testing.T) {
	t.Parallel()
	cov := coverage.NewWithCmdAndPackage(fakeExecCommandFailure, "example.com", "workdir", "./...")
	if _, err := cov.Run(); err == nil {
		t.Error("expected run to report an error")
	}
}

func TestCoverageParsesOutput(t *testing.T) {
	t.Parallel()
	module := "example.com"
	cov := coverage.NewWithCmdAndPackage(fakeExecCommandSuccess(nil), module, "testdata/valid", "./...")
	profile := coverage.Profile{
		"path/file1.go": {
			{
				StartLine: 47,
				StartCol:  2,
				EndLine:   48,
				EndCol:    16,
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
	want := coverage.Result{
		Module:  module,
		Profile: profile,
	}

	got, err := cov.Run()
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(got.Profile, want.Profile) {
		t.Error(cmp.Diff(got, want))
	}
	if !cmp.Equal(got.Module, want.Module) {
		t.Error(cmp.Diff(got, want))
	}
	if got.Elapsed == 0 {
		t.Errorf("expected elapsed time to be greater than 0")
	}
}

func TestCoverageNew(t *testing.T) {
	t.Run("does not return error if it can retrieve module", func(t *testing.T) {
		t.Parallel()
		path := t.TempDir()
		goMod := path + "/go.mod"
		err := os.WriteFile(goMod, []byte("module example.com"), 0600)
		if err != nil {
			t.Fatal(err)
		}

		_, err = coverage.New(t.TempDir(), path)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("returns error if go.mod is invalid", func(t *testing.T) {
		t.Parallel()
		path := t.TempDir()
		goMod := path + "/go.mod"
		err := os.WriteFile(goMod, []byte(""), 0600)
		if err != nil {
			t.Fatal(err)
		}

		_, err = coverage.New(t.TempDir(), path)
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("returns error if it cannot find module", func(t *testing.T) {
		t.Parallel()
		_, err := coverage.New(t.TempDir(), t.TempDir())
		if err == nil {
			t.Errorf("expected an error")
		}
	})
}

func TestParseOutputFail(t *testing.T) {
	t.Parallel()
	cov := coverage.NewWithCmdAndPackage(fakeExecCommandSuccess(nil), "example.com", "testdata/invalid", "./...")

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
