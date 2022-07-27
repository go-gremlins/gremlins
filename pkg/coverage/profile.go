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

package coverage

import (
	"go/token"
)

// Block holds the start and end coordinates of a section of a source file
// covered by tests.
type Block struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Profile is implemented as a map holding a slice of Block per each filename.
type Profile map[string][]Block

// IsCovered checks if the given token.Position is covered by the coverage Profile.
func (p Profile) IsCovered(pos token.Position) bool {
	block, ok := p[pos.Filename]
	if !ok {
		return false
	}
	for _, b := range block {
		if b.StartLine == b.EndLine {
			if pos.Line == b.StartLine && pos.Column >= b.StartCol && pos.Column <= b.EndCol {
				return true
			}
		}
		if b.StartLine < b.EndLine {
			if pos.Line == b.StartLine && pos.Column >= b.StartCol {
				return true
			}
			if pos.Line == b.EndLine && pos.Column <= b.EndCol {
				return true
			}
			if pos.Line > b.StartLine && pos.Line < b.EndLine {
				return true
			}
		}
	}

	return false
}
