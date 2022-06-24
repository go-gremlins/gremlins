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

package mutator

import "go/token"

type MutantType int

func (mt MutantType) String() string {
	switch mt {
	case ConditionalBoundary:
		return "Conditional Boundary"
	default:
		return ""
	}
}

const (
	ConditionalBoundary MutantType = iota
)

var tokenMutantType = map[token.Token][]MutantType{
	token.GTR: {ConditionalBoundary},
	token.LSS: {ConditionalBoundary},
	token.GEQ: {ConditionalBoundary},
	token.LEQ: {ConditionalBoundary},
}

var mutations = map[MutantType]map[token.Token]token.Token{
	ConditionalBoundary: {
		token.GTR: token.GEQ,
		token.LSS: token.LEQ,
		token.GEQ: token.GTR,
		token.LEQ: token.LSS,
	},
}
