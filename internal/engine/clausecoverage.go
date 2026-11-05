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
	"go/token"

	"github.com/OpenRAE/go-rae-gremlins/internal/coverage"
)

// fileCoverage tells if the tests executed a token position of one parsed file.
//
// A Go coverage profile has a block for each switch and select clause body,
// but no block for the clause expressions. fileCoverage adds the clause
// expressions that the tests evaluated. It derives them from the clause
// bodies that ran and from the Go evaluation order of the statements.
type fileCoverage struct {
	set         *token.FileSet
	profile     coverage.Profile
	clauseExprs []exprSpan
}

// exprSpan is the source range [start, end) of an expression.
type exprSpan struct {
	start token.Pos
	end   token.Pos
}

func newFileCoverage(set *token.FileSet, file *ast.File, profile coverage.Profile) fileCoverage {
	c := fileCoverage{set: set, profile: profile}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SwitchStmt:
			c.clauseExprs = append(c.clauseExprs, c.evaluatedCaseExprs(n.Body)...)
		case *ast.SelectStmt:
			c.clauseExprs = append(c.clauseExprs, c.evaluatedCommExprs(n.Body)...)
		}

		return true
	})

	return c
}

func (c fileCoverage) isCovered(pos token.Pos) bool {
	if c.profile.IsCovered(c.set.Position(pos)) {
		return true
	}
	for _, s := range c.clauseExprs {
		if pos >= s.start && pos < s.end {
			return true
		}
	}

	return false
}

// bodyRan tells if the body of the clause with the given colon ran.
// The coverage block of a clause body starts immediately after the colon,
// also when the body is empty.
func (c fileCoverage) bodyRan(colon token.Pos) bool {
	return c.profile.IsCovered(c.set.Position(colon + 1))
}

// evaluatedCaseExprs returns the case expressions of an expression switch
// that the tests evaluated.
//
// Go evaluates the case expressions from top to bottom and from left to right,
// and stops at the first match. The default clause runs only when no case
// matches. A clause body can also run because the previous clause body ran
// and ends with fallthrough. Thus a body that ran proves only that the first
// clause of its fallthrough chain was reached:
//   - all expressions of the clauses before that clause were evaluated,
//   - the first expression of that clause was evaluated,
//   - all case expressions were evaluated if that clause is the default clause
//     and the body that ran is its own.
func (c fileCoverage) evaluatedCaseExprs(body *ast.BlockStmt) []exprSpan {
	var clauses []*ast.CaseClause
	for _, stmt := range body.List {
		if cc, ok := stmt.(*ast.CaseClause); ok {
			clauses = append(clauses, cc)
		}
	}

	full := 0 // The clauses before this index had all expressions evaluated.
	first := make([]bool, len(clauses))
	for j, cc := range clauses {
		if !c.bodyRan(cc.Colon) {
			continue
		}
		entry := j
		for entry > 0 && endsWithFallthrough(clauses[entry-1]) && c.bodyRan(clauses[entry-1].Colon) {
			entry--
		}
		if entry == j && cc.List == nil {
			full = len(clauses)

			break
		}
		full = max(full, entry)
		first[entry] = true
	}

	var spans []exprSpan
	for k, cc := range clauses {
		switch {
		case k < full:
			for _, e := range cc.List {
				spans = append(spans, exprSpan{start: e.Pos(), end: e.End()})
			}
		case first[k] && len(cc.List) > 0:
			spans = append(spans, exprSpan{start: cc.List[0].Pos(), end: cc.List[0].End()})
		}
	}

	return spans
}

func endsWithFallthrough(cc *ast.CaseClause) bool {
	if len(cc.Body) == 0 {
		return false
	}
	br, ok := cc.Body[len(cc.Body)-1].(*ast.BranchStmt)

	return ok && br.Tok == token.FALLTHROUGH
}

// evaluatedCommExprs returns the communication expressions of a select
// statement that the tests evaluated.
//
// On entry to a select statement, Go evaluates the channel operands of all
// receive operations and the channel and value expressions of all send
// statements. Thus a clause body that ran proves that all of them were
// evaluated. Go evaluates the left side of a receive assignment only for the
// selected clause.
func (c fileCoverage) evaluatedCommExprs(body *ast.BlockStmt) []exprSpan {
	var clauses []*ast.CommClause
	ran := false
	for _, stmt := range body.List {
		if cc, ok := stmt.(*ast.CommClause); ok {
			clauses = append(clauses, cc)
			ran = ran || c.bodyRan(cc.Colon)
		}
	}
	if !ran {
		return nil
	}

	var spans []exprSpan
	for _, cc := range clauses {
		switch s := cc.Comm.(type) {
		case *ast.SendStmt, *ast.ExprStmt:
			spans = append(spans, exprSpan{start: s.Pos(), end: s.End()})
		case *ast.AssignStmt:
			for _, e := range s.Rhs {
				spans = append(spans, exprSpan{start: e.Pos(), end: e.End()})
			}
			if c.bodyRan(cc.Colon) {
				for _, e := range s.Lhs {
					spans = append(spans, exprSpan{start: e.Pos(), end: e.End()})
				}
			}
		}
	}

	return spans
}
