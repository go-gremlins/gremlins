package engine_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

const switchFile = "switch.go"

// mark names one operator inside one case expression of a test source.
type mark struct{ expr, op string }

// aboveTen and belowMinusTen are the marks several test sources share.
var (
	aboveTen      = mark{"n > 10", ">"}
	belowMinusTen = mark{"n < -10", "<"}
)

type caseCoverageTest struct {
	name string
	src  string
	// ran lists the blocks a coverage run reports as executed. Go's coverage profile opens a
	// case clause's block at its body, after the case expression.
	ran     []coverage.Block
	covered []mark
	unread  []mark
}

func caseCoverageTests() []caseCoverageTest {
	every := `package example

func Switch(n int) string {
	switch {
	case n > 10:
		return "big"
	case n > 0:
		return "small"
	}
	return "none"
}
`
	pair := `package example

func Pair(n int) string {
	switch {
	case n > 10, n < -10:
		return "far"
	}
	return "near"
}
`
	later := `package example

func Later(n int) string {
	switch {
	case n > 10, n < -10:
		return "far"
	case n != 0:
		return "near"
	}
	return "zero"
}
`
	byDefault := `package example

func Default(n int) string {
	switch {
	default:
		return "other"
	case n > 10, n < -10:
		return "far"
	}
}
`
	empty := `package example

func Empty(r rune) bool {
	switch {
	case r >= 'a', r == '-':
	default:
		return false
	}
	return true
}
`
	through := `package example

func Through(n int) string {
	switch {
	case n > 10:
		fallthrough
	case n > 0:
		return "positive"
	}
	return "none"
}
`
	unmatched := `package example

func None(n int) string {
	switch {
	case n > 10:
		return "big"
	}
	return "none"
}
`
	tagged := `package example

func Tag(n, a, b int) string {
	switch n {
	case a + b:
		return "sum"
	case a * b:
		return "product"
	}
	return "none"
}
`

	return []caseCoverageTest{
		{
			name:    "a tagless switch whose every clause ran reads each case covered",
			src:     every,
			ran:     stmts(every, `return "big"`, `return "small"`, `return "none"`),
			covered: []mark{aboveTen, {"n > 0", ">"}},
		},
		{
			name:    "a clause entered through its first expression leaves the next unread",
			src:     pair,
			ran:     stmts(pair, `return "far"`, `return "near"`),
			covered: []mark{aboveTen},
			unread:  []mark{belowMinusTen},
		},
		{
			name:    "a later clause that ran proves every expression above it",
			src:     later,
			ran:     stmts(later, `return "near"`),
			covered: []mark{aboveTen, belowMinusTen, {"n != 0", "!="}},
		},
		{
			name:    "a default that ran proves every case, wherever it sits",
			src:     byDefault,
			ran:     stmts(byDefault, `return "other"`),
			covered: []mark{aboveTen, belowMinusTen},
		},
		{
			name:    "an empty clause body that ran proves its first expression",
			src:     empty,
			ran:     append(stmts(empty, `return true`), emptyBody(empty, `case r >= 'a', r == '-':`)),
			covered: []mark{{"r >= 'a'", ">="}},
			unread:  []mark{{"r == '-'", "=="}},
		},
		{
			name:   "a switch holding fallthrough keeps its cases unread",
			src:    through,
			ran:    stmts(through, `fallthrough`, `return "positive"`, `return "none"`),
			unread: []mark{aboveTen, {"n > 0", ">"}},
		},
		{
			name:   "a switch that matched nothing and has no default stays unread",
			src:    unmatched,
			ran:    stmts(unmatched, `return "none"`),
			unread: []mark{aboveTen},
		},
		{
			name:    "a tag switch reads its case expressions the same way",
			src:     tagged,
			ran:     stmts(tagged, `return "product"`),
			covered: []mark{{"a + b", "+"}, {"a * b", "*"}},
		},
	}
}

func TestCaseExpressionCoverage(t *testing.T) {
	t.Parallel()
	for _, tc := range caseCoverageTests() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			statuses := caseMutantStatuses(t, tc.src, tc.ran)

			for _, m := range tc.covered {
				assertCaseStatus(t, statuses, tc.src, m, mutator.Runnable)
			}
			for _, m := range tc.unread {
				assertCaseStatus(t, statuses, tc.src, m, mutator.NotCovered)
			}
		})
	}
}

// caseMutantStatuses runs the engine over src with ran as its coverage and returns the status of
// every mutant, keyed by its "line:column".
func caseMutantStatuses(t *testing.T, src string, ran []coverage.Block) map[string][]mutator.Status {
	t.Helper()
	enabled := map[string]any{configuration.UnleashDryRunKey: true}
	for _, mt := range mutator.Types {
		enabled[configuration.MutantTypeEnabledKey(mt)] = true
	}
	viperSet(enabled)
	defer viperReset()

	mapFS := fstest.MapFS{switchFile: {Data: []byte(src)}}
	mod := gomodule.GoModule{Name: expectedModule, Root: ".", CallingDir: "."}
	cov := coverage.Profile{switchFile: ran}
	mut := engine.New(mod, engine.CodeData{Cov: cov}, newJobDealerStub(t), engine.WithDirFs(mapFS))

	statuses := map[string][]mutator.Status{}
	for _, m := range mut.Run(context.Background()).Mutants {
		at := fmt.Sprintf("%d:%d", m.Position().Line, m.Position().Column)
		statuses[at] = append(statuses[at], m.Status())
	}

	return statuses
}

func assertCaseStatus(t *testing.T, statuses map[string][]mutator.Status, src string, m mark, want mutator.Status) {
	t.Helper()
	line, col := position(src, m.expr, m.op)
	at := fmt.Sprintf("%d:%d", line, col)
	got, ok := statuses[at]
	if !ok {
		t.Fatalf("no mutant at %s (%q in %q); mutants: %v", at, m.op, m.expr, statuses)
	}
	for _, s := range got {
		if s != want {
			t.Errorf("%q in %q at %s: got %s, want %s", m.op, m.expr, at, s, want)
		}
	}
}

// position returns the 1-based line and byte column of op inside the first occurrence of expr.
func position(src, expr, op string) (int, int) {
	start := strings.Index(src, expr)
	if start < 0 {
		panic(fmt.Sprintf("%q is not in the source", expr))
	}
	offset := start + strings.Index(expr, op)
	line := strings.Count(src[:offset], "\n") + 1
	col := offset - strings.LastIndex(src[:offset], "\n")

	return line, col
}

// stmts returns the single-line coverage block of each statement, as a coverage run writes it.
func stmts(src string, statements ...string) []coverage.Block {
	blocks := make([]coverage.Block, 0, len(statements))
	for _, s := range statements {
		line, col := position(src, s, s)
		blocks = append(blocks, coverage.Block{StartLine: line, StartCol: col, EndLine: line, EndCol: col + len(s)})
	}

	return blocks
}

// emptyBody returns the zero-length block a coverage run writes just after the colon of a case
// clause with no statements.
func emptyBody(src, clause string) coverage.Block {
	line, col := position(src, clause, clause)
	end := col + len(clause)

	return coverage.Block{StartLine: line, StartCol: end, EndLine: line, EndCol: end}
}
