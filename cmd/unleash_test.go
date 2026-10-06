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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

func TestUnleash(t *testing.T) {
	c, err := newUnleashCmd(context.Background())
	if err != nil {
		t.Fatal("newUnleashCmd should no fail")
	}
	cmd := c.cmd

	if cmd.Name() != "unleash" {
		t.Errorf("expected 'unleash', got %q", cmd.Name())
	}

	flags := cmd.Flags()

	testCases := []struct {
		name      string
		shorthand string
		flagType  string
		defValue  string
	}{
		{
			name:     "arithmetic-base",
			flagType: "bool",
			defValue: "true",
		},
		{
			name:     "conditionals-boundary",
			flagType: "bool",
			defValue: "true",
		},
		{
			name:     "conditionals_negation",
			flagType: "bool",
			defValue: "true",
		},
		{
			name:     "coverpkg",
			flagType: "string",
			defValue: "",
		},
		{
			name:      "diff",
			shorthand: "D",
			flagType:  "string",
			defValue:  "",
		},
		{
			name:      "dry-run",
			shorthand: "d",
			flagType:  "bool",
			defValue:  "false",
		},
		{
			name:     "increment-decrement",
			flagType: "bool",
			defValue: "true",
		},
		{
			name:      "integration",
			shorthand: "i",
			flagType:  "bool",
			defValue:  "false",
		},
		{
			name:     "invert-assignments",
			flagType: "bool",
			defValue: "false",
		},
		{
			name:     "invert-bitwise",
			flagType: "bool",
			defValue: "false",
		},
		{
			name:     "invert-bwassign",
			flagType: "bool",
			defValue: "false",
		},

		{
			name:     "invert-logical",
			flagType: "bool",
			defValue: "false",
		},
		{
			name:     "invert-loopctrl",
			flagType: "bool",
			defValue: "false",
		},
		{
			name:     "invert-negatives",
			flagType: "bool",
			defValue: "true",
		},
		{
			name:      "output",
			shorthand: "o",
			flagType:  "string",
			defValue:  "",
		},
		{
			name:     "remove-self-assignments",
			flagType: "bool",
			defValue: "false",
		},
		{
			name:     "schemata",
			flagType: "bool",
			defValue: "true",
		},
		{
			name:     "schemata-build-timeout",
			flagType: "string",
			defValue: "",
		},
		{
			name:      "tags",
			shorthand: "t",
			flagType:  "string",
			defValue:  "",
		},
		{
			name:     "test-cpu",
			flagType: "int",
			defValue: "0",
		},
		{
			name:     "threshold-efficacy",
			flagType: "float64",
			defValue: "0",
		},
		{
			name:     "threshold-mcover",
			flagType: "float64",
			defValue: "0",
		},
		{
			name:     "timeout-coefficient",
			flagType: "int",
			defValue: "0",
		},
		{
			name:     "workers",
			flagType: "int",
			defValue: "0",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := flags.Lookup(tc.name)
			if f == nil {
				t.Fatalf("expected flag %q to be registered", tc.name)
			}
			if tc.shorthand != "" && f.Shorthand != tc.shorthand {
				t.Errorf("expected %q to have a shorthand %q, got %q", tc.name, tc.shorthand, f.Shorthand)
			}
			if f.Value.Type() != tc.flagType {
				t.Errorf("expected %q to be type %q, got %q", tc.name, f.Value.Type(), f.Value.Type())
			}
			if f.DefValue != tc.defValue {
				t.Errorf("expected %q to have default value %q, got %q", tc.name, tc.defValue, f.DefValue)
			}
		})
	}

	// test for MutantTypes flags
	for _, mt := range mutator.Types {
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

// TestSchemataIsDefault checks that a run given no schemata flag, config key
// or environment variable resolves to the schema path, and that each of
// --schemata=false, unleash.schemata: false in .gremlins.yaml and
// GREMLINS_UNLEASH_SCHEMATA=false opts out of it.
func TestSchemataIsDefault(t *testing.T) {
	// An environment value reaches configuration.Get as a string, and Get
	// reads a bool by type assertion until go-gremlins/gremlins#216 is fixed,
	// so "true" from the environment is not asserted here; "false" reads as
	// the zero value either way.
	testCases := map[string]struct {
		args []string
		cfg  string
		env  string
		want bool
	}{
		"no_flag":         {args: nil, want: true},
		"explicit_off":    {args: []string{"--schemata=false"}, want: false},
		"explicit_on":     {args: []string{"--schemata"}, want: true},
		"config_absent":   {cfg: "unleash:\n  workers: 2\n", want: true},
		"config_off":      {cfg: "unleash:\n  schemata: false\n", want: false},
		"config_on":       {cfg: "unleash:\n  schemata: true\n", want: true},
		"env_off":         {env: "false", want: false},
		"env_over_config": {cfg: "unleash:\n  schemata: true\n", env: "false", want: false},
		"flag_over_env":   {args: []string{"--schemata"}, env: "false", want: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			if tc.env != "" {
				t.Setenv("GREMLINS_UNLEASH_SCHEMATA", tc.env)
			}
			cPaths := []string{t.TempDir()} // no config file in it
			if tc.cfg != "" {
				p := filepath.Join(t.TempDir(), ".gremlins.yaml")
				if err := os.WriteFile(p, []byte(tc.cfg), 0o600); err != nil {
					t.Fatal(err)
				}
				cPaths = []string{p}
			}
			c, err := newUnleashCmd(context.Background())
			if err != nil {
				t.Fatal("newUnleashCmd should not fail")
			}
			// Initialised in PreRunE, as gremlins does it just before the
			// command runs: cobra's initializers are process-global, and one
			// a root command of another test left behind re-initialises the
			// configuration from the default paths when this command runs.
			c.cmd.PreRunE = func(_ *cobra.Command, _ []string) error { return configuration.Init(cPaths) }
			c.cmd.RunE = func(_ *cobra.Command, _ []string) error { return nil }
			c.cmd.SetArgs(tc.args)
			if err := c.cmd.Execute(); err != nil {
				t.Fatal("Execute should not fail")
			}
			if got := configuration.Get[bool](configuration.UnleashSchemataKey); got != tc.want {
				t.Errorf("schemata = %v, want %v", got, tc.want)
			}
		})
	}
}
