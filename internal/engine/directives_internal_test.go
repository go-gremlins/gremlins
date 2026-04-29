/*
 * Copyright 2026 The Gremlins Authors
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

package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

func parseSrc(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "src.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	return set, file
}

func TestBuildDirectiveIndex_EndOfLine_Untyped(t *testing.T) {
	t.Parallel()

	src := `package p

func F() int {
	a := 1 + 2 //nomutant
	return a
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	pos := positionOf(t, set, src, "+")
	if !idx.isSuppressed(pos, mutator.ArithmeticBase) {
		t.Errorf("expected ArithmeticBase on line %d to be suppressed", pos.Line)
	}
	if !idx.isSuppressed(pos, mutator.InvertBitwise) {
		t.Errorf("expected InvertBitwise on line %d to be suppressed (untyped directive applies to all)", pos.Line)
	}

	posReturn := positionOf(t, set, src, "return")
	if idx.isSuppressed(posReturn, mutator.ArithmeticBase) {
		t.Errorf("did not expect line %d to be suppressed", posReturn.Line)
	}
}

func TestBuildDirectiveIndex_EndOfLine_TypedFilter(t *testing.T) {
	t.Parallel()

	src := `package p

func F() int {
	a := 1 + 2 //nomutant:arithmetic-base
	return a
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	pos := positionOf(t, set, src, "+")
	if !idx.isSuppressed(pos, mutator.ArithmeticBase) {
		t.Errorf("expected ArithmeticBase to be suppressed by typed filter")
	}
	if idx.isSuppressed(pos, mutator.InvertBitwise) {
		t.Errorf("did NOT expect InvertBitwise to be suppressed (not in typed filter)")
	}
}

func TestBuildDirectiveIndex_BlockScope_Function(t *testing.T) {
	t.Parallel()

	src := `package p

//nomutant
func F() int {
	a := 1 + 2
	b := a * 3
	return b
}

func G() int {
	return 1 + 2
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	for _, tok := range []string{"+", "*"} {
		pos := positionOf(t, set, src, tok)
		if !idx.isSuppressed(pos, mutator.ArithmeticBase) {
			t.Errorf("expected token %q at line %d to be suppressed (inside block-scoped F)", tok, pos.Line)
		}
	}

	posG := positionOfNth(t, set, src, "+", 2)
	if idx.isSuppressed(posG, mutator.ArithmeticBase) {
		t.Errorf("did NOT expect token at line %d (inside G) to be suppressed", posG.Line)
	}
}

func TestBuildDirectiveIndex_BlockScope_SingleStatement(t *testing.T) {
	t.Parallel()

	src := `package p

func F() int {
	//nomutant
	a := 1 + 2
	b := 3 * 4
	return a + b
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	posPlus := positionOfNth(t, set, src, "+", 1)
	if !idx.isSuppressed(posPlus, mutator.ArithmeticBase) {
		t.Errorf("expected first '+' at line %d to be suppressed by single-stmt block scope", posPlus.Line)
	}

	posMul := positionOf(t, set, src, "*")
	if idx.isSuppressed(posMul, mutator.ArithmeticBase) {
		t.Errorf("did NOT expect '*' at line %d to be suppressed (block scope is single stmt)", posMul.Line)
	}
}

func TestBuildDirectiveIndex_FileScope(t *testing.T) {
	t.Parallel()

	src := `//nomutant
package p

func F() int {
	a := 1 + 2
	return a * 3
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	for _, tok := range []string{"+", "*"} {
		pos := positionOf(t, set, src, tok)
		for _, mt := range mutator.Types {
			if !idx.isSuppressed(pos, mt) {
				t.Errorf("expected %s at line %d to be suppressed by file-scope directive", mt, pos.Line)
			}
		}
	}
}

func TestBuildDirectiveIndex_FileScope_Typed(t *testing.T) {
	t.Parallel()

	src := `//nomutant:arithmetic-base
package p

func F() int {
	return 1 + 2
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	pos := positionOf(t, set, src, "+")
	if !idx.isSuppressed(pos, mutator.ArithmeticBase) {
		t.Errorf("expected ArithmeticBase to be suppressed by typed file-scope directive")
	}
	if idx.isSuppressed(pos, mutator.InvertBitwise) {
		t.Errorf("did NOT expect InvertBitwise to be suppressed (not listed in typed file-scope directive)")
	}
}

func TestBuildDirectiveIndex_NoOpOnEmptyLine(t *testing.T) {
	t.Parallel()

	src := `package p

func F() int {
	a := 1 + 2

	//nomutant

	return a
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	pos := positionOf(t, set, src, "+")
	if idx.isSuppressed(pos, mutator.ArithmeticBase) {
		t.Errorf("did NOT expect '+' to be suppressed (directive is on a blank-context line)")
	}
}

func TestBuildDirectiveIndex_Malformed(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"empty_typed_filter": `package p

func F() int {
	return 1 + 2 //nomutant:
}
`,
		"unknown_type": `package p

func F() int {
	return 1 + 2 //nomutant:bogus-type
}
`,
	}

	for name, src := range cases {
		src := src
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			set, file := parseSrc(t, src)
			idx := buildDirectiveIndex(set, file) // must not panic

			pos := positionOf(t, set, src, "+")
			if idx.isSuppressed(pos, mutator.ArithmeticBase) {
				t.Errorf("did NOT expect malformed directive to suppress mutants")
			}
		})
	}
}

func TestDirectiveIndex_NilReceiverIsSafe(t *testing.T) {
	t.Parallel()

	// A nil index must answer "not suppressed" without panicking. The
	// engine relies on this so it can call isSuppressed unconditionally
	// even when no index has been built (currently never happens, but
	// the guard is cheap and protects future call sites).
	var idx *directiveIndex
	if idx.isSuppressed(token.Position{Line: 1}, mutator.ArithmeticBase) {
		t.Errorf("nil directiveIndex must return false from isSuppressed")
	}
}

func TestBuildDirectiveIndex_TypedFilterWithEmptyEntries(t *testing.T) {
	t.Parallel()

	// Doubled commas / leading commas leave empty strings in the comma-
	// split type list. Each empty entry should be skipped silently and
	// the surrounding valid types still parsed.
	src := `package p

func F() int {
	return 1 + 2 //nomutant:arithmetic-base,,invert-bitwise
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	pos := positionOf(t, set, src, "+")
	if !idx.isSuppressed(pos, mutator.ArithmeticBase) {
		t.Errorf("expected ArithmeticBase to be suppressed (valid type before doubled comma)")
	}
	if !idx.isSuppressed(pos, mutator.InvertBitwise) {
		t.Errorf("expected InvertBitwise to be suppressed (valid type after doubled comma)")
	}
	if idx.isSuppressed(pos, mutator.ConditionalsBoundary) {
		t.Errorf("did NOT expect ConditionalsBoundary to be suppressed (not in filter)")
	}
}

func TestBuildDirectiveIndex_NestedBlocks_Additive(t *testing.T) {
	t.Parallel()

	// Outer block-scope on the func suppresses InvertBitwise everywhere
	// inside F. Inner block-scope on the assignment additionally suppresses
	// ArithmeticBase on that statement only.
	src := `package p

//nomutant:invert-bitwise
func F() int {
	//nomutant:arithmetic-base
	a := 1 + 2
	b := 3 + 4
	return a + b
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	// Position of the first '+' (inside the inner block-scope). Both the
	// outer and the inner directive cover this position.
	posInner := positionOfNth(t, set, src, "+", 1)
	if !idx.isSuppressed(posInner, mutator.ArithmeticBase) {
		t.Errorf("inner block must suppress ArithmeticBase on its own statement")
	}
	if !idx.isSuppressed(posInner, mutator.InvertBitwise) {
		t.Errorf("outer block must STILL suppress InvertBitwise inside the inner block (additive)")
	}

	// Position of the second '+' (outside the inner block, still inside outer).
	posOuter := positionOfNth(t, set, src, "+", 2)
	if idx.isSuppressed(posOuter, mutator.ArithmeticBase) {
		t.Errorf("inner block should NOT suppress ArithmeticBase outside its range")
	}
	if !idx.isSuppressed(posOuter, mutator.InvertBitwise) {
		t.Errorf("outer block must suppress InvertBitwise outside inner block too")
	}
}

func TestBuildDirectiveIndex_TypedFilterNonApplicableType(t *testing.T) {
	t.Parallel()

	src := `package p

func F() int {
	return 1 + 2 //nomutant:invert-bitwise
}
`
	set, file := parseSrc(t, src)
	idx := buildDirectiveIndex(set, file)

	pos := positionOf(t, set, src, "+")
	if !idx.isSuppressed(pos, mutator.InvertBitwise) {
		t.Errorf("expected InvertBitwise to be suppressed (it is in the typed filter, even if not applicable to '+')")
	}
	if idx.isSuppressed(pos, mutator.ArithmeticBase) {
		t.Errorf("did NOT expect ArithmeticBase to be suppressed (not in typed filter)")
	}
}

// positionOf returns the token.Position of the first occurrence of needle in src.
func positionOf(t *testing.T, set *token.FileSet, src, needle string) token.Position {
	t.Helper()

	return positionOfNth(t, set, src, needle, 1)
}

// positionOfNth returns the position of the n-th (1-based) occurrence of needle
// in src, mapping a byte offset back to a token.Position via the FileSet's
// single registered file.
func positionOfNth(t *testing.T, set *token.FileSet, src, needle string, n int) token.Position {
	t.Helper()
	if n < 1 {
		t.Fatalf("n must be >= 1, got %d", n)
	}
	idx := -1
	rest := src
	consumed := 0
	for i := 0; i < n; i++ {
		off := strings.Index(rest, needle)
		if off < 0 {
			t.Fatalf("could not find occurrence %d of %q in source", n, needle)
		}
		idx = consumed + off
		consumed = idx + len(needle)
		rest = src[consumed:]
	}

	var pos token.Position
	set.Iterate(func(f *token.File) bool {
		pos = f.Position(f.Pos(f.Base() + idx))

		return false
	})

	return pos
}
