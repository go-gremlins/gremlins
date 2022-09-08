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

package configuration

import (
	"github.com/go-gremlins/gremlins/internal/mutator"
)

var mutationEnabled = map[mutator.Type]bool{
	mutator.ArithmeticBase:       true,
	mutator.ConditionalsBoundary: true,
	mutator.ConditionalsNegation: true,
	mutator.IncrementDecrement:   true,
	mutator.InvertLogical:        false,
	mutator.InvertNegatives:      true,
	mutator.InvertLoopCtrl:       true,
}

// IsDefaultEnabled returns the default enabled/disabled state of the mutation.
// It gets the state from the table above that must be kept up to date when adding
// new mutant types.
func IsDefaultEnabled(mt mutator.Type) bool {
	return mutationEnabled[mt]
}
