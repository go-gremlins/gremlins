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

package schemata

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// assignMutations mirrors the engine's rewrite table for the op= statements:
// the statement forms' arms and helpers are written from it, so it must
// rewrite exactly as the engine does (the tests hold it to
// engine.TokenMutation).
var assignMutations = map[mutator.Type]map[token.Token]token.Token{
	mutator.InvertAssignments: {
		token.ADD_ASSIGN: token.SUB_ASSIGN, token.SUB_ASSIGN: token.ADD_ASSIGN,
		token.MUL_ASSIGN: token.QUO_ASSIGN, token.QUO_ASSIGN: token.MUL_ASSIGN, token.REM_ASSIGN: token.MUL_ASSIGN,
	},
	mutator.InvertBitwiseAssignments: {
		token.AND_ASSIGN: token.OR_ASSIGN, token.OR_ASSIGN: token.AND_ASSIGN, token.XOR_ASSIGN: token.AND_ASSIGN,
		token.AND_NOT_ASSIGN: token.AND_ASSIGN, token.SHL_ASSIGN: token.SHR_ASSIGN, token.SHR_ASSIGN: token.SHL_ASSIGN,
	},
	mutator.RemoveSelfAssignments: {
		token.ADD_ASSIGN: token.ASSIGN, token.SUB_ASSIGN: token.ASSIGN, token.MUL_ASSIGN: token.ASSIGN,
		token.QUO_ASSIGN: token.ASSIGN, token.REM_ASSIGN: token.ASSIGN, token.AND_ASSIGN: token.ASSIGN,
		token.OR_ASSIGN: token.ASSIGN, token.XOR_ASSIGN: token.ASSIGN, token.AND_NOT_ASSIGN: token.ASSIGN,
		token.SHL_ASSIGN: token.ASSIGN, token.SHR_ASSIGN: token.ASSIGN,
	},
}

func assignMutation(mt mutator.Type, tok token.Token) (token.Token, bool) {
	to, ok := assignMutations[mt][tok]

	return to, ok
}

// assignForm is the form of one op= operator: the stem of its helpers'
// names, its mutators in the order of the helpers' id parameters (the order
// the engine discovers them in), and the constraint the operand's type must
// satisfy for the helper form.
type assignForm struct {
	name string
	mts  []mutator.Type
	elem string
}

var (
	arithAssign   = []mutator.Type{mutator.InvertAssignments, mutator.RemoveSelfAssignments}
	bitwiseAssign = []mutator.Type{mutator.RemoveSelfAssignments, mutator.InvertBitwiseAssignments}
)

// assignTokens lists the op= operators, in the order of their helpers in the
// helper file.
var assignTokens = []token.Token{
	token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN, token.QUO_ASSIGN, token.REM_ASSIGN,
	token.AND_ASSIGN, token.OR_ASSIGN, token.XOR_ASSIGN, token.AND_NOT_ASSIGN, token.SHL_ASSIGN, token.SHR_ASSIGN,
}

var assignForms = map[token.Token]assignForm{
	token.ADD_ASSIGN:     {"ADD", arithAssign, "Number"},
	token.SUB_ASSIGN:     {"SUB", arithAssign, "Number"},
	token.MUL_ASSIGN:     {"MUL", arithAssign, "Number"},
	token.QUO_ASSIGN:     {"QUO", arithAssign, "Number"},
	token.REM_ASSIGN:     {"REM", arithAssign, "Integer"},
	token.AND_ASSIGN:     {"AND", bitwiseAssign, "Integer"},
	token.OR_ASSIGN:      {"OR", bitwiseAssign, "Integer"},
	token.XOR_ASSIGN:     {"XOR", bitwiseAssign, "Integer"},
	token.AND_NOT_ASSIGN: {"ANDNOT", bitwiseAssign, "Integer"},
	token.SHL_ASSIGN:     {"SHL", bitwiseAssign, "Integer"},
	token.SHR_ASSIGN:     {"SHR", bitwiseAssign, "Integer"},
}

// assignIDNames names a helper's id parameter for each mutator.
var assignIDNames = map[mutator.Type]string{
	mutator.InvertAssignments:        "idI",
	mutator.RemoveSelfAssignments:    "idR",
	mutator.InvertBitwiseAssignments: "idB",
}

var stringConstraint = constraint(types.String)

// helperConstraints mirrors each helper constraint an assignForm names.
var helperConstraints = map[string]*types.Interface{"Number": numberConstraint, "Integer": integerConstraint}

// assign rewrites the op= statement n. In a block -- a block statement, a
// case or comm clause body, behind any labels -- it becomes a switch on the
// active mutant on the statement's one line, each arm the statement with its
// operator rewritten, its operands' text repeated: only one arm runs, so
// each operand is evaluated once, as before. In a simple-statement position
// (an if, switch or for clause), or where an operand spans lines, it becomes
// a call to a fixed helper, which reaches the operand as &x or, for a map
// entry, as the map and the key.
func (r *rewriter) assign(s Site, n *ast.AssignStmt, inner func(ast.Node) string) (string, error) {
	f, ok := assignForms[n.Tok]
	if !ok || len(n.Lhs) != 1 || len(n.Rhs) != 1 {
		return "", fmt.Errorf("%w: assignment %s", ErrUnsupported, n.Tok)
	}
	id, err := ids(s, f.mts...)
	if err != nil {
		return "", err
	}
	lhs, rhs := n.Lhs[0], n.Rhs[0]
	lt, rt := r.info.Types[lhs].Type, r.info.Types[rhs].Type
	if lt == nil || rt == nil {
		return "", fmt.Errorf("%w: operand without type information", ErrUnsupported)
	}
	// Refuse a mutant whose arm would not compile; the engine generates
	// none such, but a site is not trusted to come from it.
	if id[0] != 0 && f.mts[0] == mutator.InvertAssignments && !types.Satisfies(lt, numberConstraint) {
		return "", fmt.Errorf("%w: %s on an operand of type %v", ErrUnsupported, mutator.InvertAssignments, lt)
	}
	shift := n.Tok == token.SHL_ASSIGN || n.Tok == token.SHR_ASSIGN
	if shift && id[0] != 0 && !untyped(rt) && !types.Identical(lt, rt) {
		return "", fmt.Errorf("%w: %s with a count of type %v, not %v", ErrUnsupported, mutator.RemoveSelfAssignments, rt, lt)
	}
	block, err := r.inBlock(n)
	if err != nil {
		return "", err
	}
	tokEnd := n.TokPos + token.Pos(len(n.Tok.String()))
	l := strings.TrimRight(inner(srcRange{n.Pos(), n.TokPos}), " \t")
	v := strings.TrimLeft(inner(srcRange{tokEnd, n.End()}), " \t")
	if block && !strings.Contains(l+v, "\n") {
		return r.assignSwitch(n.Tok, f.mts, id, l, v), nil
	}

	name, c := f.name+"Assign", f.elem
	switch {
	case n.Tok == token.ADD_ASSIGN && types.Satisfies(lt, stringConstraint):
		name, c, id = "ADDAssignStr", "", id[1:]
	case shift && id[0] == 0:
		name, id = f.name+"AssignX", id[1:]
		if err := r.shiftCount(rt); err != nil {
			return "", err
		}
	}
	if c != "" {
		if err := satisfies(lt, helperConstraints[c]); err != nil {
			return "", err
		}
	}
	args, onMap, ok := r.target(lhs, inner)
	if !ok {
		where := "a simple statement"
		if block {
			where = "a statement over several lines"
		}

		return "", fmt.Errorf("%w: op-assign in %s, neither addressable nor a map entry", ErrUnsupported, where)
	}
	if onMap {
		name += "Map"
	}

	return r.call(name, id, append(args, v)...), nil
}

// assignSwitch is the one-line switch form: an arm per mutant present at
// the site, in the order of mts, then the original statement.
func (r *rewriter) assignSwitch(tok token.Token, mts []mutator.Type, id []int, l, v string) string {
	var b strings.Builder
	b.WriteString("switch " + r.prefix + "Active {")
	for i, mt := range mts {
		if id[i] == 0 {
			continue
		}
		to, _ := assignMutation(mt, tok)
		fmt.Fprintf(&b, " case %d: %sReached(); %s %s %s;", id[i], r.prefix, l, to, v)
	}
	fmt.Fprintf(&b, " default: %s %s %s }", l, tok, v)

	return b.String()
}

// branch rewrites the break or continue n as an if on the active mutant, the
// original statement in the else arm and the other one, with the same label,
// in the then arm. An if is not a breakable statement, so an unlabelled break
// still leaves the switch, select or loop it did, and the whole form is one
// line.
func (r *rewriter) branch(s Site, n *ast.BranchStmt) (string, error) {
	id, err := ids(s, mutator.InvertLoopCtrl)
	if err != nil {
		return "", err
	}
	other, ok := map[token.Token]token.Token{token.BREAK: token.CONTINUE, token.CONTINUE: token.BREAK}[n.Tok]
	if !ok {
		return "", fmt.Errorf("%w: branch statement %s", ErrUnsupported, n.Tok)
	}
	// A continue is always in a loop, so a break is a mutant that compiles;
	// a break need not be, so refuse a continue that would not.
	if n.Tok == token.BREAK && !r.continuable(n) {
		return "", fmt.Errorf("%w: %s where no loop continues", ErrUnsupported, mutator.InvertLoopCtrl)
	}
	label := ""
	if n.Label != nil {
		label = " " + n.Label.Name
	}

	return fmt.Sprintf("if %s { %s%s } else { %s%s }", r.call("Xor", id, "false"), other, label, n.Tok, label), nil
}

// continuable reports whether a continue could stand where the break n does:
// for a labelled break, the label is a loop's; otherwise some loop of the
// same function encloses it.
func (r *rewriter) continuable(n *ast.BranchStmt) bool {
	for c, ok := ast.Node(n), true; ok; c, ok = r.parent(c) {
		switch c := c.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		case *ast.ForStmt, *ast.RangeStmt:
			if n.Label == nil {
				return true
			}
		case *ast.LabeledStmt:
			if n.Label != nil && c.Label.Name == n.Label.Name {
				switch c.Stmt.(type) {
				case *ast.ForStmt, *ast.RangeStmt:
					return true
				}

				return false
			}
		}
	}

	return false
}

// shiftCount refuses a count the helper's U cannot be inferred as an integer
// type from: an untyped constant is inferred as its default type, int or rune
// for an integer constant, but float64 for 2.0, legal Go as a count.
func (r *rewriter) shiftCount(t types.Type) error {
	if k := basicKind(t); untyped(t) && k != types.UntypedInt && k != types.UntypedRune {
		return fmt.Errorf("%w: shift count of type %v", ErrUnsupported, t)
	} else if !untyped(t) {
		return satisfies(t, integerConstraint)
	}

	return nil
}

// inBlock reports whether the statement st is in a block -- where the switch
// form is a legal statement -- rather than in a simple-statement position,
// where only an expression statement is. Labels are looked through.
func (r *rewriter) inBlock(st ast.Stmt) (bool, error) {
	child := ast.Node(st)
	p, ok := r.parent(child)
	for ok {
		l, label := p.(*ast.LabeledStmt)
		if !label {
			break
		}
		child = l
		p, ok = r.parent(l)
	}
	if !ok {
		return false, fmt.Errorf("%w: statement outside the package's files", ErrUnsupported)
	}
	switch p := p.(type) {
	case *ast.BlockStmt, *ast.CaseClause:
		return true, nil
	case *ast.CommClause:
		if p.Comm != child {
			return true, nil
		}
	case *ast.ForStmt:
		if p.Init == child || p.Post == child {
			return false, nil
		}
	case *ast.IfStmt:
		if p.Init == child {
			return false, nil
		}
	case *ast.SwitchStmt:
		if p.Init == child {
			return false, nil
		}
	case *ast.TypeSwitchStmt:
		if p.Init == child {
			return false, nil
		}
	}

	return false, fmt.Errorf("%w: statement in a %T", ErrUnsupported, p)
}

// target returns the helper arguments that reach the assigned operand x, and
// whether they are a map entry's: &x for an addressable x, or the map and the
// key for a map entry, which is not addressable. The last result is false
// for anything else.
func (r *rewriter) target(x ast.Expr, inner func(ast.Node) string) ([]string, bool, bool) {
	if r.info.Types[x].Addressable() {
		return []string{"&" + inner(x)}, false, true
	}
	ix, isIndex := ast.Unparen(x).(*ast.IndexExpr)
	if !isIndex || !r.isMap(ix.X) {
		return nil, false, false
	}
	k := strings.Trim(inner(srcRange{ix.Lbrack + 1, ix.Rbrack}), " \t")

	return []string{inner(ix.X), k}, true, true
}

// parent returns n's parent in the package's files, building the parent
// map on first use.
func (r *rewriter) parent(n ast.Node) (ast.Node, bool) {
	if r.parents == nil {
		r.parents = map[ast.Node]ast.Node{}
		for _, f := range r.files {
			var stack []ast.Node
			ast.Inspect(f, func(n ast.Node) bool {
				if n == nil {
					stack = stack[:len(stack)-1]

					return false
				}
				if len(stack) > 0 {
					r.parents[n] = stack[len(stack)-1]
				}
				stack = append(stack, n)

				return true
			})
		}
	}
	p, ok := r.parents[n]

	return p, ok
}
