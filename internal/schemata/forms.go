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
	"strconv"
	"strings"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// Mirrors of the helper constraints, for checking an operand type against
// them before a call the compiler would reject is emitted.
var (
	integerKinds = []types.BasicKind{
		types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr,
	}
	integerConstraint = constraint(integerKinds...)
	numberConstraint  = constraint(append(integerKinds[:len(integerKinds):len(integerKinds)],
		types.Float32, types.Float64, types.Complex64, types.Complex128)...)
	orderedConstraint = constraint(append(integerKinds[:len(integerKinds):len(integerKinds)],
		types.Float32, types.Float64, types.String)...)
)

// constraint is the interface ~k1 | ~k2 | ... over basic kinds.
func constraint(kinds ...types.BasicKind) *types.Interface {
	terms := make([]*types.Term, len(kinds))
	for i, k := range kinds {
		terms[i] = types.NewTerm(true, types.Typ[k])
	}
	it := types.NewInterfaceType(nil, []types.Type{types.NewUnion(terms)})
	it.Complete()

	return it
}

// orderedHelpers names the comparison helper of each ordered operator.
var orderedHelpers = map[token.Token]string{
	token.LSS: "LSS", token.LEQ: "LEQ", token.GTR: "GTR", token.GEQ: "GEQ",
}

// arithHelpers names the ARITHMETIC_BASE helper of each binary operator but
// -, which INVERT_NEGATIVES shares and SUB handles.
var arithHelpers = map[token.Token]string{
	token.ADD: "ADD", token.MUL: "MUL", token.QUO: "QUO", token.REM: "REM",
}

// bitwiseHelpers names the INVERT_BITWISE helper of each bitwise operator.
var bitwiseHelpers = map[token.Token]string{
	token.AND: "AND", token.OR: "OR", token.XOR: "XOR", token.AND_NOT: "ANDNOT",
	token.SHL: "SHL", token.SHR: "SHR",
}

// srcRange is a source range that is not itself a node, passed to inner to read
// the text between and around a site's operands.
type srcRange struct{ pos, end token.Pos }

func (s srcRange) Pos() token.Pos { return s.pos }
func (s srcRange) End() token.Pos { return s.end }

// NewRewriter returns the Rewriter for the expression mutators' sites:
// comparisons, binary and unary arithmetic, bitwise operators and shifts,
// && and ||, and ++/-- statements. Each site
// becomes one call to a fixed helper prefixed with prefix, recorded in h,
// whose arguments are the site's operands -- each appearing exactly once, in
// their original order, with their original line breaks -- so that with no
// mutant active the program evaluates what it did before, and with one of
// the site's mutants active it evaluates what the engine's token mutant
// would. A constant-valued site takes one of the constant forms instead
// (see constantForm), which depend on the context the site is in: files are
// the package's syntax, where that context is looked up. A constant group
// (see GroupConstantSites) takes one constant form for its whole
// expression. info must hold the Types and Uses of the package the sites come from; sizes are the target
// platform's (the package's TypesSizes), against which a constant's mutant
// values are held to fit their type, nil meaning the host's. A site outside those
// forms, or one whose operand types the helper cannot take, is refused with
// ErrUnsupported.
func NewRewriter(info *types.Info, sizes types.Sizes, files []*ast.File, prefix string, h *HelperSet) Rewriter {
	r := &rewriter{info: info, sizes: sizes, files: files, prefix: prefix, h: h}

	return r.rewrite
}

type rewriter struct {
	info   *types.Info
	sizes  types.Sizes
	files  []*ast.File
	prefix string
	h      *HelperSet

	// parents maps each node of files to its parent, built by the first
	// call of parent.
	parents map[ast.Node]ast.Node
}

func (r *rewriter) rewrite(s Site, inner func(ast.Node) string) (string, error) {
	if len(s.Muts) == 0 {
		return "", fmt.Errorf("%w: site has no mutants", ErrUnsupported)
	}
	if len(s.Members) > 0 {
		return r.constantGroup(s, inner)
	}
	switch n := s.Node.(type) {
	case *ast.BinaryExpr:
		if err := checkTok(s, n.Op); err != nil {
			return "", err
		}
		// The | of a type union is a BinaryExpr too, but no value.
		if !r.info.Types[n].IsValue() {
			return "", fmt.Errorf("%w: %s not in a value expression", ErrUnsupported, n.Op)
		}
		if r.info.Types[n].Value != nil {
			return r.constant(s, n, inner)
		}

		return r.binary(s, n, inner)
	case *ast.UnaryExpr:
		if err := checkTok(s, n.Op); err != nil {
			return "", err
		}
		if r.info.Types[n].Value != nil {
			return r.constant(s, n, inner)
		}

		return r.unary(s, n, inner)
	case *ast.IncDecStmt:
		if err := checkTok(s, n.Tok); err != nil {
			return "", err
		}

		return r.incDec(s, n, inner)
	case *ast.AssignStmt:
		if err := checkTok(s, n.Tok); err != nil {
			return "", err
		}

		return r.assign(s, n, inner)
	case *ast.BranchStmt:
		if err := checkTok(s, n.Tok); err != nil {
			return "", err
		}

		return r.branch(s, n)
	}

	return "", fmt.Errorf("%w: %T site", ErrUnsupported, s.Node)
}

// checkTok refuses a site whose token is not its node's.
func checkTok(s Site, op token.Token) error {
	if s.Tok != op {
		return fmt.Errorf("%w: site token %s is not the node's %s", ErrUnsupported, s.Tok, op)
	}

	return nil
}

// ids returns the site's mutant ids in the order of types, 0 for a type
// that is absent. Any mutant of another type, or a repeated type, is refused.
func ids(s Site, mts ...mutator.Type) ([]int, error) {
	out := make([]int, len(mts))
	for _, m := range s.Muts {
		i := -1
		for j, mt := range mts {
			if mt == m.Type {
				i = j
			}
		}
		switch {
		case i < 0:
			return nil, fmt.Errorf("%w: no form for %s at %s", ErrUnsupported, m.Type, s.Tok)
		case out[i] != 0:
			return nil, fmt.Errorf("%w: %s twice at one site", ErrUnsupported, m.Type)
		}
		out[i] = m.ID
	}

	return out, nil
}

// call returns prefix+name(ids..., args...) and records the helper. An
// argument that starts on a new line follows its comma directly.
func (r *rewriter) call(name string, ids []int, args ...string) string {
	r.h.Use(name)
	parts := make([]string, 0, len(ids)+len(args))
	for _, id := range ids {
		parts = append(parts, strconv.Itoa(id))
	}
	parts = append(parts, args...)
	var b strings.Builder
	b.WriteString(r.prefix + name + "(")
	for i, p := range parts {
		switch {
		case i == 0:
		case strings.HasPrefix(p, "\n"):
			b.WriteString(",")
		default:
			b.WriteString(", ")
		}
		b.WriteString(p)
	}
	b.WriteString(")")

	return b.String()
}

func (r *rewriter) binary(s Site, e *ast.BinaryExpr, inner func(ast.Node) string) (string, error) {
	switch e.Op { //nolint:exhaustive // only the expression mutators' operators have forms; default refuses the rest
	case token.EQL, token.NEQ:
		id, err := ids(s, mutator.ConditionalsNegation)
		if err != nil {
			return "", err
		}
		conv, err := r.boolResult(e)
		if err != nil {
			return "", err
		}
		// The comparison stays as written, an argument of its own: no
		// operand types to match, and nothing to re-parenthesise.
		return convert(conv, r.call("Xor", id, inner(e))), nil
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
		id, err := ids(s, mutator.ConditionalsBoundary, mutator.ConditionalsNegation)
		if err != nil {
			return "", err
		}
		conv, err := r.boolResult(e)
		if err != nil {
			return "", err
		}
		if err := r.operands(e, orderedConstraint); err != nil {
			return "", err
		}

		return convert(conv, r.call(orderedHelpers[e.Op], id, operandText(e, inner)...)), nil
	case token.SUB:
		id, err := ids(s, mutator.ArithmeticBase, mutator.InvertNegatives)
		if err != nil {
			return "", err
		}
		if err := r.operands(e, numberConstraint); err != nil {
			return "", err
		}

		return r.call("SUB", id, operandText(e, inner)...), nil
	case token.ADD, token.MUL, token.QUO, token.REM:
		id, err := ids(s, mutator.ArithmeticBase)
		if err != nil {
			return "", err
		}
		c := numberConstraint
		if e.Op == token.REM {
			c = integerConstraint
		}
		if err := r.operands(e, c); err != nil {
			return "", err
		}

		return r.call(arithHelpers[e.Op], id, operandText(e, inner)...), nil
	case token.AND, token.OR, token.XOR, token.AND_NOT:
		id, err := ids(s, mutator.InvertBitwise)
		if err != nil {
			return "", err
		}
		if err := r.operands(e, integerConstraint); err != nil {
			return "", err
		}

		return r.call(bitwiseHelpers[e.Op], id, operandText(e, inner)...), nil
	case token.SHL, token.SHR:
		id, err := ids(s, mutator.InvertBitwise)
		if err != nil {
			return "", err
		}

		return r.shift(e, id, inner)
	case token.LAND, token.LOR:
		id, err := ids(s, mutator.InvertLogical)
		if err != nil {
			return "", err
		}
		if err := r.logicalOperands(e); err != nil {
			return "", err
		}
		// l && r is Xor(id, Xor(id, l) && Xor(id, r)): with the mutant
		// active, !(!l && !r), which is l || r and, like it, evaluates r
		// only when l is false; and l || r likewise becomes l && r.
		ops := operandText(e, inner)
		x, y := r.call("Xor", id, ops[0]), r.call("Xor", id, ops[1])

		return r.call("Xor", id, x+" "+e.Op.String()+" "+y), nil
	default:
		return "", fmt.Errorf("%w: binary %s", ErrUnsupported, e.Op)
	}
}

// shift rewrites a non-constant shift. Its result has the left operand's
// type, which the helper's T is inferred from, and its count may be of any
// integer type U. A constant left operand (1 << n) takes the type of the
// context, which inference would not give it, so the helper is instantiated
// with that type explicitly -- if it is a predeclared integer type, the only
// kind this can spell without an import.
func (r *rewriter) shift(e *ast.BinaryExpr, id []int, inner func(ast.Node) string) (string, error) {
	l, c := r.info.Types[e.X], r.info.Types[e.Y]
	if l.Type == nil || c.Type == nil {
		return "", fmt.Errorf("%w: operand without type information", ErrUnsupported)
	}
	// An untyped constant count is inferred as its default type: int or
	// rune, both integers; a float (x << 2.0, legal Go) would not be.
	if k := basicKind(c.Type); untyped(c.Type) && k != types.UntypedInt && k != types.UntypedRune {
		return "", fmt.Errorf("%w: shift count of type %v", ErrUnsupported, c.Type)
	} else if !untyped(c.Type) {
		if err := satisfies(c.Type, integerConstraint); err != nil {
			return "", err
		}
	}
	name := bitwiseHelpers[e.Op]
	if l.Value == nil {
		if err := satisfies(l.Type, integerConstraint); err != nil {
			return "", err
		}

		return r.call(name, id, operandText(e, inner)...), nil
	}
	t, ok := r.info.Types[e].Type.(*types.Basic)
	if !ok || t.Info()&types.IsInteger == 0 || t.Info()&types.IsUntyped != 0 {
		return "", fmt.Errorf("%w: shift of an untyped constant in a %s context", ErrUnsupported, r.info.Types[e].Type)
	}
	out := r.call(name, id, operandText(e, inner)...)

	return r.prefix + name + "[" + t.Name() + "]" + strings.TrimPrefix(out, r.prefix+name), nil
}

// logicalOperands refuses a && or || whose rewrite would not have its type.
// Xor returns its operand's type, which for an untyped operand (a
// comparison, a constant) is bool: in a context of a named boolean type the
// rewrite would then not compile. go/types records the context's type even
// for such an operand, so whether an operand is typed is read from its
// syntax instead.
func (r *rewriter) logicalOperands(e *ast.BinaryExpr) error {
	t := r.info.Types[e].Type
	if t == nil {
		return fmt.Errorf("%w: operand without type information", ErrUnsupported)
	}
	if types.Identical(t, types.Typ[types.Bool]) || types.Identical(t, types.Typ[types.UntypedBool]) {
		return nil
	}
	if !r.typedBool(e.X) || !r.typedBool(e.Y) {
		return fmt.Errorf("%w: %s of type %v with an operand that may be untyped", ErrUnsupported, e.Op, t)
	}

	return nil
}

// typedBool reports whether the boolean x is certainly typed: a variable, a
// typed constant, a field, a call, an index, a dereference, a type
// assertion, or ! or a logical operator over such operands.
func (r *rewriter) typedBool(x ast.Expr) bool {
	switch x := ast.Unparen(x).(type) {
	case *ast.Ident:
		return r.typedObject(x)
	case *ast.SelectorExpr:
		if _, ok := r.info.Selections[x]; ok {
			return true
		}

		return r.typedObject(x.Sel)
	case *ast.CallExpr:
		return !r.info.Types[x.Fun].IsBuiltin()
	case *ast.IndexExpr, *ast.StarExpr, *ast.TypeAssertExpr:
		return true
	case *ast.UnaryExpr:
		return x.Op == token.NOT && r.typedBool(x.X)
	case *ast.BinaryExpr:
		return (x.Op == token.LAND || x.Op == token.LOR) && r.typedBool(x.X) && r.typedBool(x.Y)
	}

	return false
}

// typedObject reports whether id names a variable or a typed constant.
func (r *rewriter) typedObject(id *ast.Ident) bool {
	switch obj := r.info.Uses[id].(type) {
	case *types.Var:
		return true
	case *types.Const:
		return !untyped(obj.Type())
	}

	return false
}

// operandText splits e's text at its operator: the left operand with the
// space before the operator dropped, and the right operand with the space
// after it dropped -- but any line break or comment kept, so the call has
// the lines the expression had.
func operandText(e *ast.BinaryExpr, inner func(ast.Node) string) []string {
	opEnd := e.OpPos + token.Pos(len(e.Op.String()))

	return []string{
		strings.TrimRight(inner(srcRange{e.Pos(), e.OpPos}), " \t"),
		strings.TrimLeft(inner(srcRange{opEnd, e.End()}), " \t"),
	}
}

// boolResult returns the type a comparison's helper call, which returns
// bool, is converted to (see namedBool).
func (r *rewriter) boolResult(e ast.Expr) (string, error) {
	return namedBool(r.info, e, r.nonParenParent(e), r.spellAt(e.Pos()))
}

// nonParenParent returns e's nearest ancestor that is not a parenthesis, or nil.
func (r *rewriter) nonParenParent(e ast.Expr) ast.Node {
	n, _ := r.parent(e)
	for {
		p, paren := n.(*ast.ParenExpr)
		if !paren {
			return n
		}
		n = r.parents[p]
	}
}

// operands refuses a binary site unless both operands have one type, which
// satisfies c, for the helper's single type parameter to be inferred as. An
// untyped constant operand takes the other operand's type, as it did in the
// original expression.
func (r *rewriter) operands(e *ast.BinaryExpr, c *types.Interface) error {
	l, rt := r.info.Types[e.X], r.info.Types[e.Y]
	var t types.Type
	switch {
	case l.Type == nil || rt.Type == nil:
		return fmt.Errorf("%w: operand without type information", ErrUnsupported)
	case !untyped(l.Type) && !untyped(rt.Type):
		if !types.Identical(l.Type, rt.Type) {
			return fmt.Errorf("%w: operand types %v and %v differ", ErrUnsupported, l.Type, rt.Type)
		}
		t = l.Type
	case !untyped(l.Type) && rt.Value != nil:
		t = l.Type
	case !untyped(rt.Type) && l.Value != nil:
		t = rt.Type
	default:
		return fmt.Errorf("%w: untyped operands %v and %v", ErrUnsupported, l.Type, rt.Type)
	}

	return satisfies(t, c)
}

func satisfies(t types.Type, c *types.Interface) error {
	if !types.Satisfies(t, c) {
		return fmt.Errorf("%w: operand type %v outside the helper's constraint %v", ErrUnsupported, t, c)
	}

	return nil
}

func untyped(t types.Type) bool {
	b, ok := t.(*types.Basic)

	return ok && b.Info()&types.IsUntyped != 0
}

func (r *rewriter) unary(s Site, e *ast.UnaryExpr, inner func(ast.Node) string) (string, error) {
	var name string
	var id []int
	var err error
	switch e.Op { //nolint:exhaustive // only - and + are unary sites; default refuses the rest
	case token.SUB:
		name = "NEG"
		id, err = ids(s, mutator.ArithmeticBase, mutator.InvertNegatives)
	case token.ADD:
		name = "POS"
		id, err = ids(s, mutator.ArithmeticBase)
	default:
		return "", fmt.Errorf("%w: unary %s", ErrUnsupported, e.Op)
	}
	if err != nil {
		return "", err
	}
	t := r.info.Types[e.X].Type
	if t == nil || untyped(t) {
		return "", fmt.Errorf("%w: unary operand of type %v", ErrUnsupported, t)
	}
	if err := satisfies(t, numberConstraint); err != nil {
		return "", err
	}
	// The operand with any space after the operator dropped, line breaks and
	// comments kept.
	x := strings.TrimLeft(inner(srcRange{e.OpPos + 1, e.End()}), " \t")

	return r.call(name, id, x), nil
}

// incDec rewrites x++ / x-- as a call statement, legal wherever the
// statement was, a for post statement included. The operand is evaluated
// once, by the call's arguments: &x for an addressable x, or the map and the
// key for a map entry, which is not addressable.
func (r *rewriter) incDec(s Site, st *ast.IncDecStmt, inner func(ast.Node) string) (string, error) {
	id, err := ids(s, mutator.IncrementDecrement)
	if err != nil {
		return "", err
	}
	inc := strconv.FormatBool(st.Tok == token.INC)
	t := r.info.Types[st.X].Type
	if t == nil {
		return "", fmt.Errorf("%w: operand without type information", ErrUnsupported)
	}
	if err := satisfies(t, numberConstraint); err != nil {
		return "", err
	}
	args, onMap, ok := r.target(st.X, inner)
	if !ok {
		return "", fmt.Errorf("%w: %s operand neither addressable nor a map entry", ErrUnsupported, st.Tok)
	}
	name := "IncDec"
	if onMap {
		name = "IncDecMap"
	}

	return r.call(name, id, append(args, inc)...), nil
}

// isMap reports whether x has a map type. An operand of type-parameter
// type is refused even when its core type is a map: inferring IncDecMap's
// type arguments from it is not something this prototype attempts.
func (r *rewriter) isMap(x ast.Expr) bool {
	t := r.info.Types[x].Type
	if t == nil {
		return false
	}
	if _, ok := t.(*types.TypeParam); ok {
		return false
	}
	_, ok := t.Underlying().(*types.Map)

	return ok
}
