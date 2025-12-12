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

	"github.com/go-gremlins/gremlins/internal/coverage"
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
		// Single line coverage
		{
			name:        "true when single line, matches first column",
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
			name:        "true when single line, matches last column",
			proFilename: "test",
			proStartL:   10,
			proEndL:     10,
			proStartC:   10,
			proEndC:     11,
			posFilename: "test",
			posL:        10,
			posC:        11,
			expected:    true,
		},
		{
			name:        "true when single line, matches column lies in between",
			proFilename: "test",
			proStartL:   10,
			proEndL:     10,
			proStartC:   10,
			proEndC:     12,
			posFilename: "test",
			posL:        10,
			posC:        11,
			expected:    true,
		},
		{
			name:        "false when single line and column lies before",
			proFilename: "test",
			proStartL:   10,
			proEndL:     10,
			proStartC:   10,
			proEndC:     12,
			posFilename: "test",
			posL:        10,
			posC:        9,
			expected:    false,
		},
		{
			name:        "false when single line and column lies after",
			proFilename: "test",
			proStartL:   10,
			proEndL:     10,
			proStartC:   10,
			proEndC:     12,
			posFilename: "test",
			posL:        10,
			posC:        13,
			expected:    false,
		},
		// Multi line - in between
		{
			name:        "true when multi line and line lies in between (all cols covered)",
			proFilename: "test",
			proStartL:   10,
			proEndL:     12,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        11,
			posC:        1,
			expected:    true,
		},
		// Multi line - first line
		{
			name:        "true when covered on first line and col matches first col",
			proFilename: "test",
			proStartL:   10,
			proEndL:     11,
			proStartC:   10,
			proEndC:     11,
			posFilename: "test",
			posL:        10,
			posC:        10,
			expected:    true,
		},
		{
			name:        "true when covered on first line and col is covered to end of line",
			proFilename: "test",
			proStartL:   10,
			proEndL:     11,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        10,
			posC:        200,
			expected:    true,
		},
		{
			name:        "false when covered on first line and col lies before col",
			proFilename: "test",
			proStartL:   10,
			proEndL:     11,
			proStartC:   10,
			proEndC:     10,
			posFilename: "test",
			posL:        10,
			posC:        9,
			expected:    false,
		},
		// Multi line - last line
		{
			name:        "true when covered on last line and col matches last col",
			proFilename: "test",
			proStartL:   10,
			proEndL:     11,
			proStartC:   10,
			proEndC:     11,
			posFilename: "test",
			posL:        11,
			posC:        11,
			expected:    true,
		},
		{
			name:        "true when covered on last line and col lies before last col (ignores first col)",
			proFilename: "test",
			proStartL:   10,
			proEndL:     11,
			proStartC:   10,
			proEndC:     11,
			posFilename: "test",
			posL:        11,
			posC:        9,
			expected:    true,
		},
		{
			name:        "false when covered on last line and col lies after last col",
			proFilename: "test",
			proStartL:   10,
			proEndL:     11,
			proStartC:   10,
			proEndC:     11,
			posFilename: "test",
			posL:        11,
			posC:        12,
			expected:    false,
		},
	}

	for _, tc := range testCases {
		tCase := tc
		t.Run(tCase.name, func(t *testing.T) {
			profile := coverage.Profile {
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

func TestIsCovered_PathsWithSlashes(t *testing.T) {
    profile := coverage.Profile{
        "internal/coverage/file.go": []coverage.Block{
            {StartLine: 10, StartCol: 1, EndLine: 20, EndCol: 1},
        },
    }
    
    pos := token.Position{
        Filename: "internal/coverage/file.go",
        Line:     15,
        Column:   5,
    }
    
    if !profile.IsCovered(pos) {
        t.Error("expected position to be covered")
    }
}
