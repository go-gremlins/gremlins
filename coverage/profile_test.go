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
	"github.com/go-gremlins/gremlins/coverage"
	"go/token"
	"testing"
)

func TestIsCovered(t *testing.T) {
	testCases := []struct {
		name        string
		proFilename string
		proStartL   int
		proEndL     int
		proStartC   int
		proEndC     int

		posFilename string
		posL        int
		posC        int

		expected bool
	}{
		{
			name:        "returns true when line and column are covered",
			proFilename: "test",
			proStartL:   10,
			proEndL:     10,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        10,
			posC:        10,
			expected:    true,
		},
		{
			name:        "StartLine < EndLine",
			proFilename: "test",
			proStartL:   10,
			proEndL:     11,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        10,
			posC:        10,
			expected:    true,
		},
		{
			name:        "StartLine < EndLine",
			proFilename: "test",
			proStartL:   10,
			proEndL:     11,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        11,
			posC:        10,
			expected:    true,
		},
		{
			name:        "StartLine < EndLine",
			proFilename: "test",
			proStartL:   10,
			proEndL:     12,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        11,
			posC:        10,
			expected:    true,
		},
		{
			name:        "returns false when line is not covered and column is covered",
			proFilename: "test",
			proStartL:   10,
			proEndL:     10,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        11,
			posC:        10,
			expected:    false,
		},
		{
			name:        "returns false when line is covered and column not covered",
			proFilename: "test",
			proStartL:   10,
			proEndL:     10,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        10,
			posC:        11,
			expected:    false,
		},
		{
			name:        "returns false when filename is not found",
			proFilename: "test_pro",
			proStartL:   10,
			proEndL:     10,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test_pos",
			posL:        10,
			posC:        10,
			expected:    false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			profile := coverage.Profile{
				tc.proFilename: {
					{
						StartLine: tc.proStartL,
						StartCol:  tc.proStartC,
						EndLine:   tc.proEndL,
						EndCol:    tc.proEndC,
					},
				},
			}

			position := token.Position{
				Filename: tc.posFilename,
				Offset:   100,
				Line:     tc.posL,
				Column:   tc.posC,
			}

			got := profile.IsCovered(position)

			if got != tc.expected {
				t.Errorf("expected coverage to be %v, got %v", tc.expected, got)
			}
		})
	}
}
