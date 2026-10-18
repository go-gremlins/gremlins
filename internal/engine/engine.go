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

// Package engine orchestrates mutation testing by discovering, applying, and testing mutations.
package engine

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/diff"
	"github.com/go-gremlins/gremlins/internal/engine/workerpool"
	"github.com/go-gremlins/gremlins/internal/exclusion"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/report"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
)

// Engine is the "engine" that performs the mutation testing.
//
// It traverses the AST of the project, finds which TokenMutator can be applied and
// performs the actual mutation testing.
type Engine struct {
	fs           fs.FS
	jDealer      ExecutorDealer
	codeData     CodeData
	mutantStream chan mutator.Mutator
	module       gomodule.GoModule
	logger       report.MutantLogger
	viability    Viability
}

// CodeData is used to check if the mutant should be executed.
type CodeData struct {
	Cov       coverage.Profile
	Diff      diff.Diff
	Exclusion exclusion.Rules
}

// Option for the Engine initialization.
type Option func(m Engine) Engine

// New instantiates an Engine.
//
// It gets a fs.FS on which to perform the analysis, a CodeData to
// check if the mutants are executable and a sets of Option.
func New(mod gomodule.GoModule, codeData CodeData, jDealer ExecutorDealer, opts ...Option) Engine {
	dir := filepath.Join(mod.Root, mod.CallingDir)
	mut := Engine{
		module:   mod,
		jDealer:  jDealer,
		codeData: codeData,
		fs:       os.DirFS(dir),
		logger:   report.NewLogger(),
		viability: NewTypeViability(
			dir,
			configuration.Get[string](configuration.UnleashTagsKey),
		),
	}
	for _, opt := range opts {
		mut = opt(mut)
	}

	return mut
}

// WithDirFs overrides the fs.FS of the module (mainly used for testing purposes).
func WithDirFs(dirFS fs.FS) Option {
	return func(m Engine) Engine {
		m.fs = dirFS

		return m
	}
}

// WithViability overrides the Viability the Engine consults before emitting a
// mutant. A nil Viability emits every mutant the token table maps, which is
// what gremlins did before one existed; the gate in unary_mutation_test.go
// uses it to show what the default checker is holding back.
func WithViability(v Viability) Option {
	return func(m Engine) Engine {
		m.viability = v

		return m
	}
}

// Run executes the mutation testing.
//
// It walks the fs.FS provided and checks every .go file which is not a test.
// For each file it will scan for tokenMutations and gather all the mutants found.
func (mu *Engine) Run(ctx context.Context) report.Results {
	// If the dealer is the standard MutantExecutorDealer, hand it the run
	// context so that in-flight mutants can be marked correctly when the
	// runner cancels (e.g. SIGTERM at job timeout) instead of falling
	// through to the default Lived branch.
	if d, ok := mu.jDealer.(*MutantExecutorDealer); ok {
		d.SetRunCtx(ctx)
	}
	mu.mutantStream = make(chan mutator.Mutator)
	go func() {
		defer close(mu.mutantStream)
		_ = fs.WalkDir(mu.fs, ".", func(path string, _ fs.DirEntry, _ error) error {
			isGoCode := filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go")

			if isGoCode && !mu.codeData.Exclusion.IsFileExcluded(path) {
				mu.runOnFile(path)
			}

			return nil
		})
	}()

	start := time.Now()
	res := mu.executeTests(ctx)
	res.Elapsed = time.Since(start)
	res.Module = mu.module.Name

	return res
}

func (mu *Engine) runOnFile(fileName string) {
	src, _ := mu.fs.Open(fileName)
	set := token.NewFileSet()
	file, _ := parser.ParseFile(set, fileName, src, parser.ParseComments)
	_ = src.Close()

	// ancestors is the path from the file down to the node being visited,
	// kept here rather than recomputed per mutant: a mutant emitted earlier
	// in this walk may already be rewriting its token in an executor, and a
	// later walk over the whole tree would read that token concurrently.
	var ancestors []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			ancestors = ancestors[:len(ancestors)-1]

			return true
		}
		ancestors = append(ancestors, node)
		n, ok := NewTokenNode(node)
		if !ok {
			return true
		}
		mu.findMutations(fileName, set, file, n, ancestors)

		return true
	})
}

func (mu *Engine) findMutations(fileName string, set *token.FileSet, file *ast.File, node *NodeToken, ancestors []ast.Node) {
	mutantTypes, ok := MutantTypesFor(node)
	if !ok {
		return
	}

	pkg := mu.pkgName(fileName, file.Name.Name)
	position := set.Position(node.TokPos)
	// Everything read off the node is read before the first of its mutants
	// is emitted. From then on an executor may be rewriting the node's token
	// in place (TokenMutator.Apply), so a later read would race with it and
	// could see the mutated token instead of the source's.
	tok := node.Tok()
	status := mu.mutationStatus(set, node.TokPos, ancestors)
	for _, mt := range mutantTypes {
		if !configuration.Get[bool](configuration.MutantTypeEnabledKey(mt)) {
			continue
		}
		if !mu.viable(position, tok, mt) {
			continue
		}
		mutantType := mt
		tm := NewTokenMutant(pkg, set, file, node)
		tm.SetType(mutantType)
		tm.SetStatus(status)

		mu.mutantStream <- tm
	}
}

// viable reports whether applying mt to tok at position yields legal Go. The
// token table maps a token to the mutations that are MEANINGFUL for it; only
// the source around it decides whether they are legal. See Viability.
func (mu *Engine) viable(position token.Position, tok token.Token, mt mutator.Type) bool {
	if mu.viability == nil {
		return true
	}
	mutated, ok := tokenMutations[mt][tok]
	if !ok {
		return true
	}

	return mu.viability.Viable(position, mutated)
}

func (mu *Engine) pkgName(fileName, fPkg string) string {
	var pkg string
	fn := fmt.Sprintf("%s/%s", mu.module.CallingDir, fileName)
	p := filepath.Dir(fn)
	for {
		if strings.HasSuffix(p, fPkg) {
			pkg = fmt.Sprintf("%s/%s", mu.module.Name, p)

			break
		}
		d := filepath.Dir(p)
		if d == p {
			pkg = mu.module.Name

			break
		}
		p = d
	}

	return normalisePkgPath(pkg)
}

func normalisePkgPath(pkg string) string {
	sep := fmt.Sprintf("%c", os.PathSeparator)

	return strings.ReplaceAll(pkg, sep, "/")
}

func (mu *Engine) mutationStatus(set *token.FileSet, tokPos token.Pos, ancestors []ast.Node) mutator.Status {
	var status mutator.Status
	pos := set.Position(tokPos)

	if mu.reached(set, tokPos, ancestors) {
		status = mutator.Runnable
	}

	// The diff names files relative to the module root, and pos relative to
	// the directory the run is scoped to.
	modPos := pos
	modPos.Filename = mu.module.ModulePath(pos.Filename)
	if !mu.codeData.Diff.IsChanged(modPos) {
		status = mutator.Skipped
	}

	return status
}

// reached reports whether a test executed the code at tokPos.
//
// A position inside a block of the cover profile is reached when a test
// executed that block. Go's cover tool leaves some evaluated expressions
// outside every block: a `case` expression (the clause's block starts after
// its colon, and the switch head's ends at the switch's opening brace), and
// whatever follows a closure argument in a statement (the enclosing block ends
// where the closure starts, the closure's body is a block of its own, and the
// next block starts at the statement's own body).
//
// An expression of a switch's case list is evaluated only when the switch got
// that far: caseReached reads that off the clause bodies the profile records.
// The left-hand side of a select's receive is evaluated only when its case is
// selected, which its body records. Every other gap is evaluated whenever its
// enclosing statement runs, so its coverage is that statement's: the nearest
// enclosing statement whose position the profile does instrument. That is the
// grain the cover tool applies everywhere else, where `if a || b` is covered
// when `b` is short-circuited away. The rest of a select's communication
// clauses are of this kind: entering a select evaluates the channel and value
// operands of every one of them.
//
// ancestors is the path from the file down to the node carrying the token,
// innermost last. The walk never leaves the function the position is in, so a
// closure no test calls is never credited with its enclosing statement. A
// position whose nearest instrumented statement is in a block no test executed
// is not reached: the gap rule reads only gaps and never promotes a block the
// profile says is dead.
func (mu *Engine) reached(set *token.FileSet, tokPos token.Pos, ancestors []ast.Node) bool {
	cov := mu.codeData.Cov
	pos := set.Position(tokPos)
	if cov.InBlock(pos) {
		return cov.IsCovered(pos)
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		switch n := ancestors[i].(type) {
		case *ast.FuncLit, *ast.FuncDecl:
			return false
		case *ast.CaseClause:
			if tokPos < n.Colon && i >= 2 {
				if body, ok := ancestors[i-1].(*ast.BlockStmt); ok {
					return caseReached(set, cov, body, n, tokPos)
				}
			}
		case *ast.CommClause:
			// A receive's left-hand side is evaluated only when its case is
			// the one selected, so it is reached when that case's body ran.
			if as, ok := n.Comm.(*ast.AssignStmt); ok && len(as.Lhs) > 0 &&
				tokPos >= as.Lhs[0].Pos() && tokPos < as.Lhs[len(as.Lhs)-1].End() {
				return bodyRan(set, cov, n.Colon)
			}
		case *ast.BlockStmt:
			continue
		case ast.Stmt:
			stmtPos := set.Position(n.Pos())
			if cov.InBlock(stmtPos) {
				return cov.IsCovered(stmtPos)
			}
		}
	}

	return false
}

// bodyRan reports whether a test executed the clause body that starts after
// the colon at colon.
func bodyRan(set *token.FileSet, cov coverage.Profile, colon token.Pos) bool {
	p := set.Position(colon + 1)

	return cov.InBlock(p) && cov.IsCovered(p)
}

// caseReached reports whether a test evaluated the expression at tokPos in the
// case list of clause, one of the clauses of a switch whose body is body.
//
// A switch evaluates its case lists in order, and each list's expressions in
// order, and stops at the first match; it takes the default clause only after
// every expression failed. So every expression of clause's list was evaluated
// when the default clause's body ran, wherever it sits, or when the body of
// any clause after it ran. The body of clause itself proves only that its
// first expression was evaluated: the list may have matched there. A body
// entered through the previous clause's fallthrough was not selected by any
// list, and proves nothing. A switch whose lists all failed with no default
// clause to take leaves no trace in the profile, so that evaluation is not
// credited.
func caseReached(set *token.FileSet, cov coverage.Profile, body *ast.BlockStmt, clause *ast.CaseClause, tokPos token.Pos) bool {
	first := len(clause.List) > 0 && tokPos < clause.List[0].End()
	after := false
	for i, st := range body.List {
		c, ok := st.(*ast.CaseClause)
		if !ok {
			continue
		}
		own := c == clause
		if !bodyRan(set, cov, c.Colon) || enteredByFallthrough(body, i) {
			after = after || own

			continue
		}
		if c.List == nil || after || (own && first) {
			return true
		}
		after = after || own
	}

	return false
}

// enteredByFallthrough reports whether the clause before body.List[i] ends in
// a fallthrough statement, labelled or not, through which body.List[i]'s body
// can be entered without its case list matching.
func enteredByFallthrough(body *ast.BlockStmt, i int) bool {
	if i == 0 {
		return false
	}
	prev, ok := body.List[i-1].(*ast.CaseClause)
	if !ok {
		return false
	}
	stmts := prev.Body
	for len(stmts) > 0 {
		if _, empty := stmts[len(stmts)-1].(*ast.EmptyStmt); !empty {
			break
		}
		stmts = stmts[:len(stmts)-1]
	}
	if len(stmts) == 0 {
		return false
	}
	last := stmts[len(stmts)-1]
	for {
		l, ok := last.(*ast.LabeledStmt)
		if !ok {
			break
		}
		last = l.Stmt
	}
	br, ok := last.(*ast.BranchStmt)

	return ok && br.Tok == token.FALLTHROUGH
}

func (mu *Engine) executeTests(ctx context.Context) report.Results {
	pool := workerpool.Initialize("mutator")
	pool.Start()

	var mutants []mutator.Mutator
	outCh := make(chan mutator.Mutator)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for mut := range mu.mutantStream {
			ok := checkDone(ctx)
			if !ok {
				pool.Stop()

				break
			}
			wg.Add(1)
			pool.AppendExecutor(mu.jDealer.NewExecutor(mut, outCh, wg))
		}
	}()

	go func() {
		wg.Wait()
		close(outCh)
	}()

	for m := range outCh {
		mu.logger.Mutant(m)
		mutants = append(mutants, m)
	}

	return results(mutants)
}

func checkDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	default:
		return true
	}
}

func results(m []mutator.Mutator) report.Results {
	return report.Results{Mutants: m}
}
