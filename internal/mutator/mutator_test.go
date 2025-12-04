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

package mutator_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

func TestStatusString(t *testing.T) {
	testCases := []struct {
		name           string
		expected       string
		mutationStatus mutator.Status
	}{
		{
			name:           "NotCovered",
			expected:       "NOT COVERED",
			mutationStatus: mutator.NotCovered,
		},
		{
			name:           "Runnable",
			expected:       "RUNNABLE",
			mutationStatus: mutator.Runnable,
		},
		{
			name:           "Lived",
			expected:       "LIVED",
			mutationStatus: mutator.Lived,
		},
		{
			name:           "Killed",
			expected:       "KILLED",
			mutationStatus: mutator.Killed,
		},
		{
			name:           "NotViable",
			expected:       "NOT VIABLE",
			mutationStatus: mutator.NotViable,
		},
		{
			name:           "TimedOut",
			expected:       "TIMED OUT",
			mutationStatus: mutator.TimedOut,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutationStatus.String() != tc.expected {
				t.Error(cmp.Diff(tc.mutationStatus.String(), tc.expected))
			}
		})
	}
}

func TestTypeString(t *testing.T) {
	testCases := []struct {
		name       string
		expected   string
		mutantType mutator.Type
	}{
		{
			name:       "CONDITIONALS_BOUNDARY",
			expected:   "CONDITIONALS_BOUNDARY",
			mutantType: mutator.ConditionalsBoundary,
		},
		{
			name:       "CONDITIONALS_NEGATION",
			expected:   "CONDITIONALS_NEGATION",
			mutantType: mutator.ConditionalsNegation,
		},
		{
			name:       "INCREMENT_DECREMENT",
			expected:   "INCREMENT_DECREMENT",
			mutantType: mutator.IncrementDecrement,
		},
		{
			name:       "INVERT_LOGICAL",
			expected:   "INVERT_LOGICAL",
			mutantType: mutator.InvertLogical,
		},
		{
			name:       "INVERT_NEGATIVES",
			expected:   "INVERT_NEGATIVES",
			mutantType: mutator.InvertNegatives,
		},
		{
			name:       "ARITHMETIC_BASE",
			expected:   "ARITHMETIC_BASE",
			mutantType: mutator.ArithmeticBase,
		},
		{
			name:       "INVERT_LOOPCTRL",
			expected:   "INVERT_LOOPCTRL",
			mutantType: mutator.InvertLoopCtrl,
		},
		{
			name:       "INVERT_ASSIGNMENTS",
			expected:   "INVERT_ASSIGNMENTS",
			mutantType: mutator.InvertAssignments,
		},
		{
			name:       "INVERT_BITWISE",
			expected:   "INVERT_BITWISE",
			mutantType: mutator.InvertBitwise,
		},
		{
			name:       "INVERT_BWASSIGN",
			expected:   "INVERT_BWASSIGN",
			mutantType: mutator.InvertBitwiseAssignments,
		},
		{
			name:       "REMOVE_SELF_ASSIGNMENTS",
			expected:   "REMOVE_SELF_ASSIGNMENTS",
			mutantType: mutator.RemoveSelfAssignments,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutantType.String() != tc.expected {
				t.Error(cmp.Diff(tc.mutantType.String(), tc.expected))
			}
		})
	}
}
