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

package execution

// ErrorType is the type of the error that can generate a specific exit status.
type ErrorType int

// String produces the human readable sentence for the ErrorType.
func (e ErrorType) String() string {
	switch e {
	case EfficacyThreshold:
		return "below efficacy-threshold"
	case MutantCoverageThreshold:
		return "below mutant coverage-threshold"
	}
	panic("this should not happen")
}

const (
	// EfficacyThreshold is the error type raised when efficacy is below threshold.
	EfficacyThreshold ErrorType = iota

	// MutantCoverageThreshold is the error type raised when mutant coverage is
	// below threshold.
	MutantCoverageThreshold
)

var errorMapping = map[ErrorType]int{
	EfficacyThreshold:       10,
	MutantCoverageThreshold: 11,
}

// ExitError is a special Error that is raised when special conditions require
// Gremlins to exit with a specific errorCode.
// If this error is returned and/or properly wrapped, it will reach the main
// function. In the main, the exitCode will be set as the exit code of the
// execution.
type ExitError struct {
	errorType ErrorType
	exitCode  int
}

// NewExitErr instantiates a new ExitError.
func NewExitErr(et ErrorType) *ExitError {
	exitCode := errorMapping[et]

	return &ExitError{exitCode: exitCode, errorType: et}
}

// Error is the implementation of the Error interface and returns
// the ErrorType human readable message.
func (e *ExitError) Error() string {
	return e.errorType.String()
}

// ExitCode returns the exit code associated with the specific ErrorType.
func (e *ExitError) ExitCode() int {
	return e.exitCode
}
