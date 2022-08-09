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

package configuration_test

import (
	"testing"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/pkg/mutant"
)

func TestMutantDefaultStatus(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		mutantType mutant.Type
		expected   bool
	}{
		{
			mutantType: mutant.ArithmeticBase,
			expected:   true,
		},
		{
			mutantType: mutant.ConditionalsBoundary,
			expected:   true,
		},
		{
			mutantType: mutant.ConditionalsNegation,
			expected:   true,
		},
		{
			mutantType: mutant.IncrementDecrement,
			expected:   true,
		},
		{
			mutantType: mutant.InvertNegatives,
			expected:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.mutantType.String(), func(t *testing.T) {
			t.Parallel()
			got := configuration.IsDefaultEnabled(tc.mutantType)

			if got != tc.expected {
				t.Errorf("expected %s to be %q, got %q", tc.mutantType, enabled(tc.expected), enabled(got))
			}
		})
	}
}

func enabled(b bool) string {
	if b {
		return "enabled"
	}

	return "disabled"
}
