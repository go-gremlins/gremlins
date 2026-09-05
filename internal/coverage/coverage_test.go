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
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/coverage"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
)

type commandHolder struct {
	events []struct {
		command string
		args    []string
	}
}

func TestCoverageRun(t *testing.T) {
	testCases := []struct {
		name     string
		callPath string
		wantPath string
		intMode  bool
	}{
		{
			name:     "from root, normal mode",
			callPath: ".",
			wantPath: "./...",
			intMode:  false,
		},
		{
			name:     "from folder, normal mode",
			callPath: "test/pkg",
			wantPath: "./test/pkg/...",
			intMode:  false,
		},
		{
			name:     "from root, integration mode",
			callPath: ".",
			wantPath: "./...",
			intMode:  true,
		},
		{
			name:     "from folder, integration mode",
			callPath: "test/dir",
			wantPath: "./...",
			intMode:  true,
		},
	}
	coverpkg := "./internal/log,./pkg/..."
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			viper.Set(configuration.UnleashTagsKey, "tag1 tag2")
			viper.Set(configuration.UnleashCoverPkgKey, coverpkg)
			viper.Set(configuration.UnleashIntegrationMode, tc.intMode)
			defer viper.Reset()

			wantWorkdir := "workdir"
			wantFilename := "coverage"
			wantFilePath := wantWorkdir + "/" + wantFilename
			holder := &commandHolder{}
			mod := gomodule.GoModule{
				Name:       "example.com",
				Root:       ".",
				CallingDir: tc.callPath,
			}
			cov := coverage.NewWithCmd(fakeExecCommandSuccess(holder), wantWorkdir, mod)

			_, _ = cov.Run()

			firstWant := "go mod download"
			secondWant := fmt.Sprintf("go test -tags tag1 tag2 -coverpkg %s -cover -coverprofile %v %s",
				coverpkg, wantFilePath, tc.wantPath)

			if len(holder.events) != 2 {
				t.Fatal("expected two commands to be executed")
			}
			firstGot := fmt.Sprintf("go %v", strings.Join(holder.events[0].args, " "))
			secondGot := fmt.Sprintf("go %v", strings.Join(holder.events[1].args, " "))

			if !cmp.Equal(firstGot, firstWant) {
				t.Error(cmp.Diff(firstGot, firstWant))
			}
			if !cmp.Equal(secondGot, secondWant) {
				t.Error(cmp.Diff(secondGot, secondWant))
			}
		})
	}
}

func TestCoverageRunFails(t *testing.T) {
	mod := gomodule.GoModule{
		Name:       "example.com",
		CallingDir: "./...",
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
		Name:       module,
		CallingDir: "path",
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
		Name:       "example.com",
		CallingDir: "./...",
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
	os.Exit(0) // skipcq: RVV-A0003
}

func TestCoverageProcessFailure(_ *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(1) // skipcq: RVV-A0003
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
		// #nosec G204 G702 - We are in tests, we don't care
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
		// #nosec G204 G702 - We are in tests, we don't care
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_TEST_PROCESS=1"}
		executed++

		return cmd
	}
}

func TestCoverageReusesProvidedProfile(t *testing.T) {
	viper.Set(configuration.UnleashCoverageProfileKey, "testdata/valid/coverage")
	viper.Set(configuration.UnleashCoverageElapsedKey, "2m42s")
	defer viper.Reset()

	holder := &commandHolder{}
	mod := gomodule.GoModule{
		Name:       "example.com",
		Root:       ".",
		CallingDir: "path",
	}
	cov := coverage.NewWithCmd(fakeExecCommandSuccess(holder), "workdir", mod)

	got, err := cov.Run()
	if err != nil {
		t.Fatal(err)
	}

	want := coverage.Profile{
		"file1.go": {
			{StartLine: 47, StartCol: 2, EndLine: 48, EndCol: 16},
		},
		"file2.go": {
			{StartLine: 52, StartCol: 2, EndLine: 53, EndCol: 16},
		},
	}
	if !cmp.Equal(got.Profile, want) {
		t.Error(cmp.Diff(got.Profile, want))
	}
	if wantElapsed := 2*time.Minute + 42*time.Second; got.Elapsed != wantElapsed {
		t.Errorf("expected elapsed %v, got %v", wantElapsed, got.Elapsed)
	}

	// The whole point of the flag: the test suite must not be run again. Only
	// the module download survives, since the mutant test runs still need it.
	if len(holder.events) != 1 {
		t.Fatalf("expected only 'go mod download' to be executed, got %d commands", len(holder.events))
	}
	if gotCmd := strings.Join(holder.events[0].args, " "); gotCmd != "mod download" {
		t.Errorf("expected 'go mod download', got 'go %s'", gotCmd)
	}
}

func TestCoverageProvidedProfileFails(t *testing.T) {
	testCases := []struct {
		name    string
		profile string
		elapsed string
	}{
		{name: "elapsed not set", profile: "testdata/valid/coverage", elapsed: ""},
		{name: "elapsed not a duration", profile: "testdata/valid/coverage", elapsed: "ages"},
		{name: "elapsed not positive", profile: "testdata/valid/coverage", elapsed: "0s"},
		{name: "profile does not exist", profile: "testdata/valid/not-there", elapsed: "1s"},
		{name: "profile not parseable", profile: "testdata/invalid/coverage", elapsed: "1s"},
	}
	mod := gomodule.GoModule{
		Name:       "example.com",
		Root:       ".",
		CallingDir: "path",
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			viper.Set(configuration.UnleashCoverageProfileKey, tc.profile)
			viper.Set(configuration.UnleashCoverageElapsedKey, tc.elapsed)
			defer viper.Reset()

			cov := coverage.NewWithCmd(fakeExecCommandSuccess(nil), "workdir", mod)

			if _, err := cov.Run(); err == nil {
				t.Error("expected run to report an error")
			}
		})
	}

	t.Run("go mod download fails", func(t *testing.T) {
		viper.Set(configuration.UnleashCoverageProfileKey, "testdata/valid/coverage")
		viper.Set(configuration.UnleashCoverageElapsedKey, "1s")
		defer viper.Reset()

		cov := coverage.NewWithCmd(fakeExecCommandFailure(0), "workdir", mod)

		if _, err := cov.Run(); err == nil {
			t.Error("expected run to report an error")
		}
	})
}
