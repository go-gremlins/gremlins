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

// Profile is implemented as a map holding a slice of Block per each filename.
//
// It holds every block of the cover profile, the ones no test executed
// included, because "no test executed this block" and "this position is in no
// block at all" are different facts: Go's cover tool leaves some evaluated
// expressions outside every block (a `case` expression, or the part of a
// statement after a closure argument), and only the second fact says that the
// position's coverage has to be read off its enclosing statement instead.
type Profile map[string][]Block

// IsCovered checks if the given token.Position lies in a block of the
// coverage Profile that a test executed.
func (p Profile) IsCovered(pos token.Position) bool {
	for _, b := range p[pos.Filename] {
		if !b.NotExecuted && b.contains(pos) {
			return true
		}
	}

	return false
}

// InBlock checks if the given token.Position lies in any block of the
// coverage Profile, executed or not. A position for which it is false is one
// the cover tool never instrumented on its own.
func (p Profile) InBlock(pos token.Position) bool {
	for _, b := range p[pos.Filename] {
		if b.contains(pos) {
			return true
		}
	}

	return false
}

// Block holds the start and end coordinates of a section of a source file
// instrumented by the cover tool, and whether any test executed it.
type Block struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	// NotExecuted is true for a block no test reached. Its zero value is an
	// executed block, so a Profile literal that lists only the executed blocks
	// reads exactly as it did before unexecuted blocks were recorded.
	NotExecuted bool
}

func (b Block) contains(pos token.Position) bool {
	return b.isBetweenLines(pos) || b.isPositionCovered(pos)
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
