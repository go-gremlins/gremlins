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
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/internal/mutant"

	"github.com/go-gremlins/gremlins/internal/configuration"
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
			name:      "dry-run",
			shorthand: "d",
			flagType:  "bool",
			defValue:  "false",
		},
		{
			name:      "tags",
			shorthand: "t",
			flagType:  "string",
			defValue:  "",
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
			name:      "output",
			shorthand: "o",
			flagType:  "string",
			defValue:  "",
		},
		{
			name:      "integration",
			shorthand: "i",
			flagType:  "bool",
			defValue:  "false",
		},
		{
			name:     "test-cpu",
			flagType: "int",
			defValue: "0",
		},
		{
			name:     "workers",
			flagType: "int",
			defValue: "0",
		},
		{
			name:     "timeout-coefficient",
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
	for _, mt := range mutant.Types {
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
