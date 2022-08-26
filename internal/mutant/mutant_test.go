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
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/mutant"
)

func TestStatusString(t *testing.T) {
	testCases := []struct {
		name           string
		expected       string
		mutationStatus mutant.Status
	}{
		{
			name:           "NotCovered",
			expected:       "NOT COVERED",
			mutationStatus: mutant.NotCovered,
		},
		{
			name:           "Runnable",
			expected:       "RUNNABLE",
			mutationStatus: mutant.Runnable,
		},
		{
			name:           "Lived",
			expected:       "LIVED",
			mutationStatus: mutant.Lived,
		},
		{
			name:           "Killed",
			expected:       "KILLED",
			mutationStatus: mutant.Killed,
		},
		{
			name:           "NotViable",
			expected:       "NOT VIABLE",
			mutationStatus: mutant.NotViable,
		},
		{
			name:           "TimedOut",
			expected:       "TIMED OUT",
			mutationStatus: mutant.TimedOut,
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
		expected   string
		mutantType mutant.Type
	}{
		{
			name:       "CONDITIONALS_BOUNDARY",
			expected:   "CONDITIONALS_BOUNDARY",
			mutantType: mutant.ConditionalsBoundary,
		},
		{
			name:       "CONDITIONALS_NEGATION",
			expected:   "CONDITIONALS_NEGATION",
			mutantType: mutant.ConditionalsNegation,
		},
		{
			name:       "INCREMENT_DECREMENT",
			expected:   "INCREMENT_DECREMENT",
			mutantType: mutant.IncrementDecrement,
		},
		{
			name:       "INVERT_LOGICAL",
			expected:   "INVERT_LOGICAL",
			mutantType: mutant.InvertLogical,
		},
		{
			name:       "INVERT_NEGATIVES",
			expected:   "INVERT_NEGATIVES",
			mutantType: mutant.InvertNegatives,
		},
		{
			name:       "ARITHMETIC_BASE",
			expected:   "ARITHMETIC_BASE",
			mutantType: mutant.ArithmeticBase,
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
