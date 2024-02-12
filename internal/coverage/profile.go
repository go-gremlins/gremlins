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
	"path/filepath"
)

// Profile is implemented as a map holding a slice of Block per each filename.
type Profile map[string][]Block

// IsCovered checks if the given token.Position is covered by the coverage Profile.
func (p Profile) IsCovered(pos token.Position) bool {
	blocks, ok := p[filepath.FromSlash(pos.Filename)]
	if !ok {
		return false
	}
	for _, b := range blocks {
		if b.isBetweenLines(pos) {
			return true
		}
		if covered := b.isPositionCovered(pos); covered {
			return true
		}
	}

	return false
}

// Block holds the start and end coordinates of a section of a source file
// covered by tests.
type Block struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

func (b Block) isPositionCovered(pos token.Position) bool {
	if b.isSingleLine() && b.isOnFirstLine(pos) && b.isSingleLineColCovered(pos) {
		return true
	}
	if b.isMultiLine() && b.isOnFirstLine(pos) && b.isCoveredToEndOfLine(pos) {
		return true
	}
	if b.isMultiLine() && b.isOnLastLine(pos) && b.isCoveredFromStartOfLine(pos) {
		return true
	}

	return false
}

func (b Block) isSingleLine() bool {
	return b.StartLine == b.EndLine
}

func (b Block) isMultiLine() bool {
	return b.StartLine < b.EndLine
}

func (b Block) isOnFirstLine(pos token.Position) bool {
	return pos.Line == b.StartLine
}

func (b Block) isOnLastLine(pos token.Position) bool {
	return pos.Line == b.EndLine
}

func (b Block) isBetweenLines(pos token.Position) bool {
	return pos.Line > b.StartLine && pos.Line < b.EndLine
}

func (b Block) isSingleLineColCovered(pos token.Position) bool {
	return pos.Column >= b.StartCol && pos.Column <= b.EndCol
}

func (b Block) isCoveredToEndOfLine(pos token.Position) bool {
	return pos.Column >= b.StartCol
}
func (b Block) isCoveredFromStartOfLine(pos token.Position) bool {
	return pos.Column <= b.EndCol
}
