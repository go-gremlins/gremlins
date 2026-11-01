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

	"github.com/OpenRAE/go-rae-gremlins/internal/configuration"
	"github.com/OpenRAE/go-rae-gremlins/internal/mutator"
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
			flagType: boolType,
			defValue: trueValue,
		},
		{
			name:     "conditionals-boundary",
			flagType: boolType,
			defValue: trueValue,
		},
		{
			name:     "conditionals_negation",
			flagType: boolType,
			defValue: trueValue,
		},
		{
			name:     "coverpkg",
			flagType: stringType,
			defValue: "",
		},
		{
			name:      "diff",
			shorthand: "D",
			flagType:  stringType,
			defValue:  "",
		},
		{
			name:      "dry-run",
			shorthand: "d",
			flagType:  boolType,
			defValue:  falseValue,
		},
		{
			name:     "increment-decrement",
			flagType: boolType,
			defValue: trueValue,
		},
		{
			name:      "integration",
			shorthand: "i",
			flagType:  boolType,
			defValue:  falseValue,
		},
		{
			name:     "invert-assignments",
			flagType: boolType,
			defValue: falseValue,
		},
		{
			name:     "invert-bitwise",
			flagType: boolType,
			defValue: falseValue,
		},
		{
			name:     "invert-bwassign",
			flagType: boolType,
			defValue: falseValue,
		},

		{
			name:     "invert-logical",
			flagType: boolType,
			defValue: falseValue,
		},
		{
			name:     "invert-loopctrl",
			flagType: boolType,
			defValue: falseValue,
		},
		{
			name:     "invert-negatives",
			flagType: boolType,
			defValue: trueValue,
		},
		{
			name:      "output",
			shorthand: "o",
			flagType:  stringType,
			defValue:  "",
		},
		{
			name:     "remove-self-assignments",
			flagType: boolType,
			defValue: falseValue,
		},
		{
			name:      "tags",
			shorthand: "t",
			flagType:  stringType,
			defValue:  "",
		},
		{
			name:     "test-cpu",
			flagType: intType,
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
			flagType: intType,
			defValue: "0",
		},
		{
			name:     "workers",
			flagType: intType,
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

		if mtf.Value.Type() != boolType {
			t.Errorf("expected %q to be a %q, got %q", s, boolType, mtf.Value.Type())
		}
		wantDef := fmt.Sprintf("%v", configuration.IsDefaultEnabled(mt))
		if mtf.DefValue != wantDef {
			t.Errorf("expected %q have default %q, got %q", s, wantDef, mtf.DefValue)
		}
	}
}
