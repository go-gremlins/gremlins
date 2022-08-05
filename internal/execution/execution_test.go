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

package execution_test

import (
	"testing"

	"github.com/go-gremlins/gremlins/internal/execution"
)

func TestExitErr(t *testing.T) {
	testCases := []struct {
		name         string
		wantExitMsg  string
		errorType    execution.ErrorType
		wantExitCode int
	}{
		{
			name:         "efficacy-threshold",
			errorType:    execution.EfficacyThreshold,
			wantExitMsg:  "below efficacy-threshold",
			wantExitCode: 10,
		},
		{
			name:         "coverage-threshold",
			errorType:    execution.MutantCoverageThreshold,
			wantExitMsg:  "below mutant coverage-threshold",
			wantExitCode: 11,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := execution.NewExitErr(tc.errorType)

			exitCode := err.ExitCode()
			exitMessage := err.Error()

			if exitCode != tc.wantExitCode {
				t.Errorf("want %d, got %d", tc.wantExitCode, exitCode)
			}
			if exitMessage != tc.wantExitMsg {
				t.Errorf("want %q, got %q", tc.wantExitMsg, exitMessage)
			}
		})
	}
}
