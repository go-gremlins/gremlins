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

package mutant_test

import (
	"github.com/google/go-cmp/cmp"
	"github.com/k3rn31/gremlins/mutant"
	"testing"
)

func TestStatusString(t *testing.T) {
	testCases := []struct {
		name           string
		mutationStatus mutant.Status
		expected       string
	}{
		{
			"NotCovered",
			mutant.NotCovered,
			"NOT COVERED",
		},
		{
			"Runnable",
			mutant.Runnable,
			"RUNNABLE",
		},
		{
			"Lived",
			mutant.Lived,
			"LIVED",
		},
		{
			"Killed",
			mutant.Killed,
			"KILLED",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutationStatus.String() != tc.expected {
				t.Errorf(cmp.Diff(tc.mutationStatus.String(), tc.expected))
			}
		})
	}
}

func TestTypeString(t *testing.T) {
	testCases := []struct {
		name       string
		mutantType mutant.Type
		expected   string
	}{
		{
			"CONDITIONALS_BOUNDARY",
			mutant.ConditionalsBoundary,
			"CONDITIONALS_BOUNDARY",
		},
		{
			"CONDITIONALS_NEGATION",
			mutant.ConditionalsNegation,
			"CONDITIONALS_NEGATION",
		},
		{
			"INCREMENT_DECREMENT",
			mutant.IncrementDecrement,
			"INCREMENT_DECREMENT",
		},
		{
			"INVERT_NEGATIVES",
			mutant.InvertNegatives,
			"INVERT_NEGATIVES",
		},
		{
			"ARITHMETIC_BASE",
			mutant.ArithmeticBase,
			"ARITHMETIC_BASE",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutantType.String() != tc.expected {
				t.Errorf(cmp.Diff(tc.mutantType.String(), tc.expected))
			}
		})
	}
}
