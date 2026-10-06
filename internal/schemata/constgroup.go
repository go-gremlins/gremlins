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
	"go/constant"
	"go/token"
	"go/types"
	"math"
)

// A site inside a larger constant expression cannot switch at run time on
// its own: its operands are folded exactly by the compiler, with the rest of
// the expression, where a run-time form would compute in the context's
// type, which can overflow, round, or give an untyped constant another type.
// So, outside a compile-time context (see context), the maximal constant
// expression holding it -- the outermost constant-valued ancestor, whose
// parent is not constant -- is rewritten whole, as one constant site:
//
//	room := maxIdentifier - len(prefix) - len("_") - len(sfx)
//
// has its first two - folded into one value per mutant, c0 the original's
// and cK the expression's with that one operator replaced, spelled by the
// form the expression's type takes (constantForm).

// GroupConstantSites returns sites with the sites inside each maximal
// constant expression M, outside a compile-time context, gathered into one
// Site for M: Node M, Tok token.ILLEGAL, Members the sites, Muts all their
// mutants. A group takes the place of its first member; every other site is
// returned as it is. A lone site that is M, or is it but for parentheses, is
// left alone: its own constant form folds it.
//
// Membership follows the site's path up to M (see pathKind):
//
//   - through parentheses, unary and binary expressions and basic numeric
//     conversions only: a member whose mutants M's fold computes through;
//   - across len or cap of an array or a pointer to one, or unsafe.Sizeof,
//     Alignof or Offsetof -- whose type alone gives the result, as in
//     len([2]int{x + 1, 0}), the operand never evaluated -- not len of a
//     constant string, whose value gives it: a member whose mutants leave
//     M's value as it is, so each is placed with M's own value, its reach
//     recorded where M is evaluated;
//   - in a compile-time context, such as the array length of
//     len([2*3]int{}): not a member. It is placed as without grouping, by
//     duplicating its function, whose jump runs before M while its mutant is
//     active; M's fold takes len's recorded value and never needs its
//     mutant's.
//   - across anything else (a call such as min, say): a member, refused with
//     the reason.
//
// info must hold the Types of files, the package syntax the sites come from.
func GroupConstantSites(info *types.Info, files []*ast.File, sites []Site) []Site {
	r := &rewriter{info: info, files: files}
	var roots []ast.Expr
	for _, s := range sites {
		e, ok := s.Node.(ast.Expr)
		if !ok || !info.Types[e].IsValue() || info.Types[e].Value == nil {
			continue
		}
		if m := r.maximal(e); m != nil {
			roots = append(roots, m)
		}
	}
	// A root inside another root's subtree -- under a non-constant node,
	// such as len([2]int{x + 2*3}) -- joins the outer one.
	outer := func(n ast.Node) ast.Expr {
		var best ast.Expr
		for _, m := range roots {
			if m.Pos() <= n.Pos() && n.End() <= m.End() && (best == nil || m.Pos() < best.Pos() || m.End() > best.End()) {
				best = m
			}
		}

		return best
	}
	// rootOf is the group root s joins, or nil.
	rootOf := func(s Site) ast.Expr {
		if s.Node == nil {
			return nil
		}
		m := outer(s.Node)
		if m == nil {
			return nil
		}
		if e, ok := s.Node.(ast.Expr); ok {
			if _, fn, err := r.context(e); fn != nil || err != nil {
				return nil
			}
		}

		return m
	}
	members := map[ast.Expr][]Site{}
	for _, s := range sites {
		if m := rootOf(s); m != nil {
			members[m] = append(members[m], s)
		}
	}
	var out []Site
	placed := map[ast.Expr]bool{}
	for _, s := range sites {
		m := rootOf(s)
		ms := members[m]
		if m == nil || len(ms) == 1 && ast.Unparen(m) == s.Node {
			out = append(out, s)

			continue
		}
		if placed[m] {
			continue
		}
		placed[m] = true
		g := Site{Node: m, Tok: token.ILLEGAL, Members: ms}
		for _, x := range ms {
			g.Muts = append(g.Muts, x.Muts...)
		}
		out = append(out, g)
	}

	return out
}

// pathKinds of a site's path up to its group root.
const (
	// pathFold: through nodes the root's fold computes through.
	pathFold = iota
	// pathTypeOnly: across a builtin whose constant result depends on its
	// operand's type alone, through fold nodes above it.
	pathTypeOnly
	// pathOther: anything else.
	pathOther
)

// pathKind classifies the path from n up to the root m, which holds n: a
// pathKind constant.
func (r *rewriter) pathKind(n ast.Node, m ast.Expr) int {
	var path []ast.Node
	for p := n; p != m; {
		q, ok := r.parent(p)
		if !ok {
			return pathOther
		}
		path = append(path, q)
		p = q
	}
	kind := pathFold
	for i := len(path) - 1; i >= 0; i-- {
		switch {
		case r.foldNode(path[i]):
		case r.typeOnly(path[i]):
			return pathTypeOnly
		default:
			kind = pathOther
		}
		if kind == pathOther {
			break
		}
	}

	return kind
}

// foldNode reports whether foldPath computes through n.
func (r *rewriter) foldNode(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.ParenExpr, *ast.UnaryExpr, *ast.BinaryExpr:
		return true
	case *ast.CallExpr:
		tv := r.info.Types[n.Fun]
		if !tv.IsType() {
			return false
		}
		b, ok := tv.Type.Underlying().(*types.Basic)

		return ok && b.Info()&(types.IsInteger|types.IsFloat) != 0
	}

	return false
}

// typeOnly reports whether n is a constant call of len or cap of an array or
// a pointer to one, or of unsafe.Sizeof, Alignof or Offsetof: its value is
// its operand's type's, the operand never evaluated, whatever operator in it
// a mutant rewrites.
func (r *rewriter) typeOnly(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok || r.info.Types[call].Value == nil || !r.info.Types[call.Fun].IsBuiltin() {
		return false
	}
	var name string
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		name = f.Name
	case *ast.SelectorExpr:
		name = f.Sel.Name
	}
	switch name {
	case "len", "cap":
		// Of an array, or a pointer to one, only: len of a constant string
		// is constant too, and is the string's value's.
		if len(call.Args) != 1 {
			return false
		}
		t := r.info.Types[call.Args[0]].Type
		if t == nil {
			return false
		}
		if p, ok := t.Underlying().(*types.Pointer); ok {
			t = p.Elem()
		}
		_, array := t.Underlying().(*types.Array)

		return array
	case "Sizeof", "Alignof", "Offsetof":
		return true
	}

	return false
}

// maximal returns the maximal constant expression holding the constant e,
// or nil when e lies in a compile-time context, which the constant forms
// and function duplication handle site by site.
func (r *rewriter) maximal(e ast.Expr) ast.Expr {
	m := e
	for {
		p, ok := r.parent(m)
		if !ok {
			return nil
		}
		x, ok := p.(ast.Expr)
		if !ok || r.info.Types[x].Value == nil {
			break
		}
		m = x
	}
	if _, fn, err := r.context(m); fn != nil || err != nil {
		return nil
	}

	return m
}

// refusedMutantsError is what a rewriter returns, as its error, with a
// replacement that places some of a group's mutants: refused are the others,
// each a Site of the group holding the one mutant, and why.
type refusedMutantsError struct {
	refused []SiteError
	// all is set when the group places none: the rewrite is empty.
	all bool
}

func (m *refusedMutantsError) Error() string {
	return fmt.Sprintf("%v: %d mutants of a constant group refused", ErrUnsupported, len(m.refused))
}

func (m *refusedMutantsError) Unwrap() error { return ErrUnsupported }

// constantGroup rewrites the group g as one constant site: its maximal
// constant expression folded once per mutant of every member, each with
// that one member's operator mutated. A mutant whose value cannot be folded
// is refused alone, through refusedMutantsError; the group is refused when none
// can be.
func (r *rewriter) constantGroup(g Site, inner func(ast.Node) string) (string, error) {
	m, ok := g.Node.(ast.Expr)
	if !ok || r.info.Types[m].Value == nil {
		return "", fmt.Errorf("%w: constant group of a %T without a constant value", ErrUnsupported, g.Node)
	}
	ctx, fn, err := r.context(m)
	if err != nil {
		return "", err
	}
	if fn != nil {
		return "", fmt.Errorf("%w: constant group in a compile-time context", ErrUnsupported)
	}
	c := folded{c0: r.info.Types[m].Value}
	// The group's mutants: a member may still list one dropped earlier.
	live := map[int]bool{}
	for _, mu := range g.Muts {
		live[mu.ID] = true
	}
	var refused []SiteError
	refuse := func(s Site, mu Mutant, err error) {
		one := Site{Node: g.Node, Tok: g.Tok, Muts: []Mutant{mu},
			Members: []Site{{Node: s.Node, Tok: s.Tok, Muts: []Mutant{mu}}}}
		refused = append(refused, SiteError{Site: one, Err: err})
	}
	for _, s := range g.Members {
		// A mutant beyond a type-only builtin leaves m's value as it is.
		if r.pathKind(s.Node, m) == pathTypeOnly {
			for _, mu := range s.Muts {
				if live[mu.ID] {
					c.ids = append(c.ids, mu.ID)
					c.cs = append(c.cs, c.c0)
				}
			}

			continue
		}
		e, ok := s.Node.(ast.Expr)
		op, _ := siteMutators(e)
		// The fold of the expression as written must give the type
		// checker's value, or the fold is not the compiler's.
		var check error
		if ok {
			check = r.checkFold(m, e, op, c.c0)
		}
		for _, mu := range s.Muts {
			if !live[mu.ID] {
				continue
			}
			to, has := constMutation(mu.Type, op)
			switch {
			case !ok:
				refuse(s, mu, fmt.Errorf("%w: %T site in a constant group", ErrUnsupported, s.Node))
			case !has:
				refuse(s, mu, fmt.Errorf("%w: no %s mutation of %s", ErrUnsupported, mu.Type, op))
			case check != nil:
				refuse(s, mu, check)
			default:
				v, err := r.foldPath(m, e, to)
				if err != nil {
					refuse(s, mu, err)

					continue
				}
				c.ids = append(c.ids, mu.ID)
				c.cs = append(c.cs, v)
			}
		}
	}
	if len(c.ids) == 0 {
		// Each refused for its own reason.
		return "", &refusedMutantsError{refused: refused, all: true}
	}
	out, err := constantForm(r.info, r.sizes, m, c, r.prefix, r.h, ctx, inner, r.spellAt(m.Pos()))
	if err != nil {
		return "", err
	}
	out = keepLines(out, inner(m))
	if len(refused) > 0 {
		return out, &refusedMutantsError{refused: refused}
	}

	return out, nil
}

// checkFold refuses the site e of the maximal expression m when folding m
// with e's operator as written does not give c0, the type checker's value.
func (r *rewriter) checkFold(m, e ast.Expr, op token.Token, c0 constant.Value) error {
	v, err := r.foldPath(m, e, op)
	if err != nil {
		return err
	}
	same := numeric(v) && numeric(c0) || !numeric(v) && v.Kind() == c0.Kind()
	if !same || !constant.Compare(v, token.EQL, c0) {
		return fmt.Errorf("%w: constant group folds to %v, the type checker to %v", ErrUnsupported, v, c0)
	}

	return nil
}

// foldPath evaluates n, which holds the site e or is it, with e's operator
// replaced by op and every other node as written: a node not holding e takes
// its recorded value; e, and the parentheses, unary and binary expressions
// and basic numeric conversions between it and n, are folded as the
// compiler folds them, each value rounded to, and held to fit, its node's
// type. Any other node on the way is refused.
func (r *rewriter) foldPath(n, e ast.Expr, op token.Token) (constant.Value, error) {
	if n.Pos() > e.Pos() || e.End() > n.End() {
		if v := r.info.Types[n].Value; v != nil {
			return v, nil
		}

		return nil, fmt.Errorf("%w: operand without a constant value", ErrUnsupported)
	}
	var v constant.Value
	var err error
	switch x := n.(type) {
	case *ast.ParenExpr:
		return r.foldPath(x.X, e, op)
	case *ast.UnaryExpr:
		xop := x.Op
		if n == e {
			xop = op
		}
		var xv constant.Value
		if xv, err = r.foldPath(x.X, e, op); err == nil {
			v, err = r.foldUnary(x, xop, xv)
		}
	case *ast.BinaryExpr:
		xop := x.Op
		if n == e {
			xop = op
		}
		var xv, yv constant.Value
		if xv, err = r.foldPath(x.X, e, op); err == nil {
			if yv, err = r.foldPath(x.Y, e, op); err == nil {
				v, err = foldBinary(r.info, x, xop, xv, yv)
			}
		}
	case *ast.CallExpr:
		v, err = r.foldConversion(x, e, op)
	default:
		return nil, fmt.Errorf("%w: %T between a constant group and its site", ErrUnsupported, n)
	}
	if err != nil {
		return nil, err
	}

	return r.represent(v, r.info.Types[n].Type)
}

// foldUnary evaluates op x, for the unary expression u.
func (r *rewriter) foldUnary(u *ast.UnaryExpr, op token.Token, x constant.Value) (constant.Value, error) {
	var prec uint
	switch op { //nolint:exhaustive // the operators of a constant unary expression; default refuses the rest
	case token.ADD, token.SUB:
		if !numeric(x) {
			return nil, fmt.Errorf("%w: unary operand %v", ErrUnsupported, x)
		}
	case token.XOR:
		if x = constant.ToInt(x); x.Kind() != constant.Int {
			return nil, fmt.Errorf("%w: unary ^ of %v", ErrUnsupported, x)
		}
		// ^x of an unsigned type flips its bits only.
		if b, ok := r.info.Types[u].Type.Underlying().(*types.Basic); ok && b.Info()&types.IsUnsigned != 0 {
			prec = intBits(b, r.sizes)
		}
	case token.NOT:
		if x.Kind() != constant.Bool {
			return nil, fmt.Errorf("%w: unary ! of %v", ErrUnsupported, x)
		}
	default:
		return nil, fmt.Errorf("%w: unary %s in a constant group", ErrUnsupported, op)
	}

	return constant.UnaryOp(op, x, prec), nil
}

// foldConversion evaluates the conversion call, holding e, to a basic
// integer or float type, as the compiler converts a constant.
func (r *rewriter) foldConversion(call *ast.CallExpr, e ast.Expr, op token.Token) (constant.Value, error) {
	tv := r.info.Types[call.Fun]
	if !tv.IsType() || len(call.Args) != 1 {
		return nil, fmt.Errorf("%w: call between a constant group and its site", ErrUnsupported)
	}
	b, ok := tv.Type.Underlying().(*types.Basic)
	if !ok || b.Info()&(types.IsInteger|types.IsFloat) == 0 {
		return nil, fmt.Errorf("%w: conversion to %v between a constant group and its site", ErrUnsupported, tv.Type)
	}
	x, err := r.foldPath(call.Args[0], e, op)
	if err != nil {
		return nil, err
	}
	if b.Info()&types.IsInteger != 0 {
		if x = constant.ToInt(x); x.Kind() != constant.Int {
			return nil, fmt.Errorf("%w: value %v does not fit %v", ErrUnsupported, x, tv.Type)
		}

		return x, nil
	}
	if x = constant.ToFloat(x); x.Kind() != constant.Float && x.Kind() != constant.Int {
		return nil, fmt.Errorf("%w: value %v does not fit %v", ErrUnsupported, x, tv.Type)
	}

	return x, nil
}

// represent holds v to the type t, as the compiler holds a constant: an
// integer type's value must be an integer that fits it, a float type's is
// rounded to it and must be finite; an untyped constant is exact.
func (r *rewriter) represent(v constant.Value, t types.Type) (constant.Value, error) {
	if t == nil {
		return v, nil
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsUntyped != 0 {
		return v, nil
	}
	switch {
	case b.Info()&types.IsInteger != 0:
		i := constant.ToInt(v)
		if i.Kind() != constant.Int || !fitsInt(i, b, r.sizes) {
			return nil, fmt.Errorf("%w: value %v does not fit %v", ErrUnsupported, v, t)
		}

		return i, nil
	case b.Kind() == types.Float32 || b.Kind() == types.Float64:
		f := constant.ToFloat(v)
		if f.Kind() != constant.Float && f.Kind() != constant.Int {
			return nil, fmt.Errorf("%w: value %v does not fit %v", ErrUnsupported, v, t)
		}
		var g float64
		if b.Kind() == types.Float32 {
			g32, _ := constant.Float32Val(f)
			g = float64(g32)
		} else {
			g, _ = constant.Float64Val(f)
		}
		if math.IsInf(g, 0) {
			return nil, fmt.Errorf("%w: value %v does not fit %v", ErrUnsupported, v, t)
		}

		return constant.MakeFloat64(g), nil
	}

	return v, nil
}

// intBits is the width in bits of the integer type b on the platform sizes
// describes, nil meaning the host's.
func intBits(b *types.Basic, sizes types.Sizes) uint {
	if sizes == nil {
		sizes = hostSizes()
	}

	return uint(sizes.Sizeof(b)) * 8 //nolint:gosec // a basic type's size is at most 16
}
