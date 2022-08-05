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
	"go/token"
	"testing"

	"github.com/go-gremlins/gremlins/pkg/coverage"
)

func TestIsCovered(t *testing.T) {
	testCases := []struct {
		name        string
		proFilename string
		posFilename string
		proStartL   int
		proEndL     int
		proStartC   int
		proEndC     int
		posL        int
		posC        int
		expected    bool
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
		tCase := tc
		t.Run(tCase.name, func(t *testing.T) {
			profile := coverage.Profile{
				tCase.proFilename: {
					{
						StartLine: tCase.proStartL,
						StartCol:  tCase.proStartC,
						EndLine:   tCase.proEndL,
						EndCol:    tCase.proEndC,
					},
				},
			}

			position := token.Position{
				Filename: tCase.posFilename,
				Offset:   100,
				Line:     tCase.posL,
				Column:   tCase.posC,
			}

			got := profile.IsCovered(position)

			if got != tCase.expected {
				t.Errorf("expected coverage to be %v, got %v", tCase.expected, got)
			}
		})
	}
}
