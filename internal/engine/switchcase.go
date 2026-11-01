package engine

import (
	"go/ast"
	"go/token"

	"github.com/go-gremlins/gremlins/internal/coverage"
)

// caseSpan is the extent of the case expressions a coverage run proves were evaluated.
type caseSpan struct{ from, to token.Pos }

// evaluatedCases lists the case expressions of file the coverage profile proves were evaluated.
// Go's profile opens a clause's block at its body, so the bodies that ran stand in for the cases.
func evaluatedCases(set *token.FileSet, file *ast.File, cov coverage.Profile) []caseSpan {
	var spans []caseSpan
	ast.Inspect(file, func(node ast.Node) bool {
		if sw, ok := node.(*ast.SwitchStmt); ok {
			spans = append(spans, switchEvaluatedCases(set, sw, cov)...)
		}

		return true
	})

	return spans
}

// switchEvaluatedCases proves a clause's first expression by its own body, and every expression
// of a clause by a later clause's body or the default's; a switch holding fallthrough proves none.
func switchEvaluatedCases(set *token.FileSet, sw *ast.SwitchStmt, cov coverage.Profile) []caseSpan {
	var cases []*ast.CaseClause
	var ran []bool
	last := -1
	for _, stmt := range sw.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		if fallsThrough(clause) {
			return nil
		}
		if clause.List == nil {
			if bodyRan(set, clause, cov) {
				last = len(sw.Body.List)
			}

			continue
		}
		cases = append(cases, clause)
		ran = append(ran, bodyRan(set, clause, cov))
		if ran[len(ran)-1] {
			last = max(last, len(cases)-1)
		}
	}

	var spans []caseSpan
	for i, clause := range cases {
		switch {
		case i < last:
			spans = append(spans, caseSpan{clause.List[0].Pos(), clause.List[len(clause.List)-1].End()})
		case ran[i]:
			spans = append(spans, caseSpan{clause.List[0].Pos(), clause.List[0].End()})
		}
	}

	return spans
}

// bodyRan reads whether a clause's body ran: its block starts at the first statement, or just
// after the colon when the body is empty.
func bodyRan(set *token.FileSet, clause *ast.CaseClause, cov coverage.Profile) bool {
	start := clause.Colon + 1
	if len(clause.Body) > 0 {
		start = clause.Body[0].Pos()
	}

	return cov.IsCovered(set.Position(start))
}

func fallsThrough(clause *ast.CaseClause) bool {
	if len(clause.Body) == 0 {
		return false
	}
	branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)

	return ok && branch.Tok == token.FALLTHROUGH
}

func inCases(spans []caseSpan, pos token.Pos) bool {
	for _, s := range spans {
		if pos >= s.from && pos < s.to {
			return true
		}
	}

	return false
}
