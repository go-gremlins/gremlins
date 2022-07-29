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

package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/pkg/mutant"
)

func TestUnleash(t *testing.T) {
	c, err := newUnleashCmd()
	if err != nil {
		t.Fatal("newUnleashCmd should no fail")
	}
	cmd := c.cmd

	if cmd.Name() != "unleash" {
		t.Errorf("expected 'unleash', got %q", cmd.Name())
	}

	flags := cmd.Flags()

	// Test for dry-run
	f := flags.Lookup("dry-run")
	if f.Value.Type() != "bool" {
		t.Errorf("expected 'dry-run' to be a 'bool', got %q", f.Value.Type())
	}
	if f.DefValue != "false" {
		t.Errorf("expected 'dry-run' have default 'false', got %q", f.DefValue)
	}

	// Test for tags
	f = flags.Lookup("tags")
	if f.Value.Type() != "string" {
		t.Errorf("expected 'tags' to be a 'string', got %q", f.Value.Type())
	}
	if f.DefValue != "" {
		t.Errorf("expected 'tags' not to be set by default, got %q", f.DefValue)
	}

	// test threshold-efficacy
	f = flags.Lookup("threshold-efficacy")
	if f.Value.Type() != "float64" {
		t.Errorf("expected 'threshold-efficacy' to be a 'float64', got %q", f.Value.Type())
	}
	if f.DefValue != "0" {
		t.Errorf("expected 'threshold-efficacy' have default '0', got %q", f.DefValue)
	}

	// test threshold-mcover
	f = flags.Lookup("threshold-mcover")
	if f.Value.Type() != "float64" {
		t.Errorf("expected 'threshold-mcover' to be a 'float64', got %q", f.Value.Type())
	}
	if f.DefValue != "0" {
		t.Errorf("expected 'threshold-mcover' have default '0', got %q", f.DefValue)
	}

	// test for MutantTypes flags
	for _, mt := range mutant.MutantTypes {
		s := strings.ToLower(mt.String())
		mtf := flags.Lookup(s)
		if mtf == nil {
			t.Errorf("expected to have flag for mutant type: %s", mt)

			continue
		}

		if mtf.Value.Type() != "bool" {
			t.Errorf("expected %q to be a %q, got %q", s, "bool", mtf.Value.Type())
		}
		wantDef := fmt.Sprintf("%v", configuration.IsDefaultEnabled(mt))
		if mtf.DefValue != wantDef {
			t.Errorf("expected %q have default %q, got %q", s, wantDef, mtf.DefValue)
		}
	}
}

func TestChangePath(t *testing.T) {
	const wantCalledDir = "aDir"

	t.Run("when passed a dir, it changes to it and returns '.'", func(t *testing.T) {
		var calledDir string
		chdir := func(dir string) error {
			calledDir = dir

			return nil
		}
		getwd := func() (string, error) {
			return "test/dir", nil
		}
		args := []string{wantCalledDir}

		p, wd, _ := changePath(args, chdir, getwd)

		if calledDir != wantCalledDir {
			t.Errorf("expected %q, got %q", wantCalledDir, calledDir)
		}
		if p != "." {
			t.Errorf("expected '.', got %q", p)
		}
		if wd != "test/dir" {
			t.Errorf("expected 'test/dir', got %s", wd)
		}
	})

	t.Run("when Chdir returns error, it returns error", func(t *testing.T) {
		chdir := func(dir string) error { return errors.New("test error") }
		getwd := func() (string, error) { return "", nil }
		args := []string{wantCalledDir}

		_, _, err := changePath(args, chdir, getwd)
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("when Getwd returns error, it returns error", func(t *testing.T) {
		chdir := func(dir string) error { return nil }
		getwd := func() (string, error) { return "", errors.New("test error") }
		args := []string{wantCalledDir}

		_, _, err := changePath(args, chdir, getwd)
		if err == nil {
			t.Errorf("expected an error")
		}
	})
}

func TestRunUnleashCmd(t *testing.T) {
	t.Run("should fail without go.mod", func(t *testing.T) {
		pwd, _ := os.Getwd()
		args := []string{pwd}
		err := runUnleash(nil, args)
		if err == nil {
			t.Fatal("runUnleashCmd should fail")
		}
	})
}

func TestRun(t *testing.T) {
	wd := t.TempDir()
	cd, _ := os.Getwd()
	defer func(dir string) {
		_ = os.Chdir(dir)
	}(cd)

	_ = os.Chdir("./testmodule")
	r, err := run(wd, ".")

	if err != nil {
		t.Fatal(err)
	}
	if r.Elapsed <= 0 {
		t.Errorf("should pass some time!")
	}
}
