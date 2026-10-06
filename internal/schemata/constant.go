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
	"go/build"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// constMutations mirrors the engine's rewrite table for the operators of the
// expression mutators' sites: a constant site's mutated values are
// folded from it, so it must rewrite exactly as the engine does (the tests
// hold it to engine.TokenMutation).
var constMutations = map[mutator.Type]map[token.Token]token.Token{
	mutator.ArithmeticBase: {
		token.ADD: token.SUB, token.SUB: token.ADD, token.MUL: token.QUO,
		token.QUO: token.MUL, token.REM: token.MUL,
	},
	mutator.ConditionalsBoundary: {
		token.GEQ: token.GTR, token.GTR: token.GEQ, token.LEQ: token.LSS, token.LSS: token.LEQ,
	},
	mutator.ConditionalsNegation: {
		token.EQL: token.NEQ, token.NEQ: token.EQL, token.GEQ: token.LSS,
		token.GTR: token.LEQ, token.LEQ: token.GTR, token.LSS: token.GEQ,
	},
	mutator.InvertNegatives: {token.SUB: token.ADD},
	mutator.InvertBitwise: {
		token.AND: token.OR, token.OR: token.AND, token.XOR: token.AND, token.AND_NOT: token.AND,
		token.SHL: token.SHR, token.SHR: token.SHL,
	},
	mutator.InvertLogical: {token.LAND: token.LOR, token.LOR: token.LAND},
}

func constMutation(mt mutator.Type, tok token.Token) (token.Token, bool) {
	to, ok := constMutations[mt][tok]

	return to, ok
}

// maxWitnessArity bounds the parameters, and the results, of a callee used
// as a float site's witness.
const maxWitnessArity = 4

// constant rewrites the constant-valued site e. A constant is folded at
// compile time and may be needed there, so it cannot take a constant form
// where Go requires one -- anywhere in a const declaration, an array length,
// a constant composite literal key -- nor where it is itself an operand of a
// larger constant expression, which keeps the site's mutants out of the
// outer folding. In the first three, inside a function declaration's body,
// the site is placed by duplicating the function (see dupSite); elsewhere,
// and in the fourth, it is refused. The replacement keeps the line count of
// the text it replaces.
func (r *rewriter) constant(s Site, e ast.Expr, inner func(ast.Node) string) (string, error) {
	ctx, fn, err := r.context(e)
	if err != nil {
		return "", err
	}
	if fn != nil {
		return "", r.dupSite(s, e, fn)
	}
	if x, ok := ctx.(ast.Expr); ok && r.info.Types[x].Value != nil {
		return "", fmt.Errorf("%w: operand of a constant expression", ErrUnsupported)
	}
	c, err := foldSite(r.info, e, s)
	if err != nil {
		return "", err
	}
	out, err := constantForm(r.info, r.sizes, e, c, r.prefix, r.h, ctx, inner, r.spellAt(e.Pos()))
	if err != nil {
		return "", err
	}

	return keepLines(out, inner(e)), nil
}

// keepLines returns the one-line replacement out of the text orig with
// orig's line breaks put after out's first opening parenthesis, where they
// cannot end a statement.
func keepLines(out, orig string) string {
	if n := strings.Count(orig, "\n"); n > 0 {
		i := strings.Index(out, "(") + 1
		out = out[:i] + strings.Repeat("\n", n) + out[i:]
	}

	return out
}

// context returns e's context: its nearest ancestor that is not a
// parenthesis. When e lies, at any depth, in a const declaration, an array
// length or a constant composite literal key -- anything there must stay
// constant -- it returns instead the function declaration whose body holds
// e, to duplicate; outside one, it refuses e.
func (r *rewriter) context(e ast.Expr) (ast.Node, *ast.FuncDecl, error) {
	if _, ok := r.parent(e); !ok {
		return nil, nil, fmt.Errorf("%w: constant site outside the package's files", ErrUnsupported)
	}
	var ctx ast.Node
	var fn *ast.FuncDecl
	compileTime, inBody, inLit := "", false, false
	for child, n := ast.Node(e), r.parents[e]; n != nil; child, n = n, r.parents[n] {
		if _, paren := n.(*ast.ParenExpr); ctx == nil && !paren {
			ctx = n
		}
		switch n := n.(type) {
		case *ast.GenDecl:
			if n.Tok == token.CONST && compileTime == "" {
				compileTime = "constant site in a const declaration"
			}
		case *ast.ArrayType:
			if n.Len == child && compileTime == "" {
				compileTime = "constant site in an array length"
			}
		case *ast.KeyValueExpr:
			if _, lit := r.parents[n].(*ast.CompositeLit); lit && n.Key == child && r.info.Types[n.Key].Value != nil && compileTime == "" {
				compileTime = "constant composite literal key"
			}
		case *ast.FuncLit:
			inLit = true
		case *ast.FuncDecl:
			fn, inBody = n, child == n.Body
		}
	}
	switch {
	case compileTime == "":
		return ctx, nil, nil
	case fn != nil && inBody:
		return nil, fn, nil
	case fn == nil && inLit:
		return nil, nil, fmt.Errorf("%w: %s, in a function literal outside a function declaration", ErrUnsupported, compileTime)
	}

	return nil, nil, fmt.Errorf("%w: %s", ErrUnsupported, compileTime)
}

// constantForm returns the replacement for the constant-valued expression e,
// whose context -- nearest non-parenthesis ancestor -- is ctx and is not
// constant. c holds e's original value c0 and each mutant's value cK, folded
// exactly, as the compiler folds them, with the mutated operator; which form
// spells them depends on the type the constant takes in its context:
//
//   - an integer type: the shift form, a non-constant expression of
//     untyped constants whose type the context gives, as it gave the
//     constant's: (c0*(1-(1<<PBit(id1)-1)-...) + c1*(1<<PBit(id1)-1) + ...).
//     A constant of a typed operand, or of an untyped rune, adds that
//     operand times 0, so that an interface or inference context still sees
//     the constant's type.
//   - a boolean type: PBool<n>(id1, ..., c0, c1, ...), a helper per arity,
//     converted to the type when it is not bool (see namedBool).
//   - float32 or float64, as a call argument, the right side of an
//     assignment or an operand of a binary expression: PSite<id>(witness), a
//     helper per site (named after its first mutant) returning T(c0) or
//     T(cK) for the T it infers from the witness -- the callee, &lhs, or the
//     other operand -- which is how a type only another package can name is
//     reached.
//   - float32 or float64 anywhere else, or where no witness is found:
//     PSite<id>[T](), the same helper instantiated with the type T the
//     constant took, if spell can name it at the site.
//
// Anything else is refused with ErrUnsupported, as is a value that does not
// fit the type.
func constantForm(info *types.Info, sizes types.Sizes, e ast.Expr, c folded, prefix string, h *HelperSet, ctx ast.Node, inner func(ast.Node) string, spell spellFunc) (string, error) {
	// In a conversion T(c) the shift form's untyped 1 takes the type T, which
	// for a type parameter is not an integer type the shift accepts, whatever
	// type go/types records for c.
	if call, ok := ctx.(*ast.CallExpr); ok && info.Types[call.Fun].IsType() {
		if _, basic := info.Types[call.Fun].Type.Underlying().(*types.Basic); !basic {
			return "", fmt.Errorf("%w: constant converted to %v", ErrUnsupported, info.Types[call.Fun].Type)
		}
	}
	t := info.Types[e].Type
	b, _ := t.Underlying().(*types.Basic)
	switch {
	case b == nil || b.Info()&types.IsUntyped != 0 && b.Kind() != types.UntypedBool:
		return "", fmt.Errorf("%w: constant of type %v", ErrUnsupported, t)
	case b.Info()&types.IsBoolean != 0:
		return boolForm(info, e, c, prefix, h, ctx, spell)
	case b.Info()&types.IsInteger != 0:
		return intForm(info, sizes, e, c, b, prefix, h, inner)
	case b.Kind() == types.Float32 || b.Kind() == types.Float64:
		return floatForm(info, e, c, b, prefix, h, ctx, inner, spell)
	}

	return "", fmt.Errorf("%w: constant of type %v", ErrUnsupported, t)
}

// folded is a constant site's original value and its mutants' values, with
// their ids, in the order of the site's mutator types.
type folded struct {
	c0  constant.Value
	ids []int
	cs  []constant.Value
}

// foldSite folds the site's original value and each present mutant's.
func foldSite(info *types.Info, e ast.Expr, s Site) (folded, error) {
	op, mts := siteMutators(e)
	if mts == nil {
		return folded{}, fmt.Errorf("%w: constant %s site", ErrUnsupported, op)
	}
	all, err := ids(s, mts...)
	if err != nil {
		return folded{}, err
	}
	out := folded{c0: info.Types[e].Value}
	for i, id := range all {
		if id == 0 {
			continue
		}
		to, ok := constMutation(mts[i], op)
		if !ok {
			return folded{}, fmt.Errorf("%w: no %s mutation of %s", ErrUnsupported, mts[i], op)
		}
		v, err := fold(info, e, to)
		if err != nil {
			return folded{}, err
		}
		out.ids = append(out.ids, id)
		out.cs = append(out.cs, v)
	}

	return out, nil
}

// siteMutators returns the operator of the site e and the mutator types
// whose mutants a constant site of it takes, in their fixed order; nil for
// an operator no constant form folds.
func siteMutators(e ast.Expr) (token.Token, []mutator.Type) {
	var mts []mutator.Type
	var op token.Token
	switch e := e.(type) {
	case *ast.BinaryExpr:
		op = e.Op
		switch op { //nolint:exhaustive // only the expression mutators' operators fold; default refuses the rest
		case token.ADD, token.MUL, token.QUO, token.REM:
			mts = []mutator.Type{mutator.ArithmeticBase}
		case token.SUB:
			mts = []mutator.Type{mutator.ArithmeticBase, mutator.InvertNegatives}
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			mts = []mutator.Type{mutator.ConditionalsBoundary, mutator.ConditionalsNegation}
		case token.EQL, token.NEQ:
			mts = []mutator.Type{mutator.ConditionalsNegation}
		case token.AND, token.OR, token.XOR, token.AND_NOT, token.SHL, token.SHR:
			mts = []mutator.Type{mutator.InvertBitwise}
		case token.LAND, token.LOR:
			mts = []mutator.Type{mutator.InvertLogical}
		}
	case *ast.UnaryExpr:
		op = e.Op
		switch op { //nolint:exhaustive // only - and + are unary sites
		case token.SUB:
			mts = []mutator.Type{mutator.ArithmeticBase, mutator.InvertNegatives}
		case token.ADD:
			mts = []mutator.Type{mutator.ArithmeticBase}
		}
	}

	return op, mts
}

// fold evaluates e with its operator replaced by op, as go/types folds a
// constant expression: exactly, with integer division when the operands'
// type is an integer type, and bitwise operators and shifts on integers.
func fold(info *types.Info, e ast.Expr, op token.Token) (constant.Value, error) {
	switch e := e.(type) {
	case *ast.UnaryExpr:
		x := info.Types[e.X].Value
		if x == nil || x.Kind() != constant.Int && x.Kind() != constant.Float {
			return nil, fmt.Errorf("%w: unary operand %v", ErrUnsupported, x)
		}

		return constant.UnaryOp(op, x, 0), nil
	case *ast.BinaryExpr:
		return foldBinary(info, e, op, info.Types[e.X].Value, info.Types[e.Y].Value)
	}

	return nil, fmt.Errorf("%w: constant %T", ErrUnsupported, e)
}

// foldBinary evaluates x op y, the operands of e with e's operator replaced
// by op, as fold does.
func foldBinary(info *types.Info, e *ast.BinaryExpr, op token.Token, x, y constant.Value) (constant.Value, error) {
	if x == nil || y == nil {
		return nil, fmt.Errorf("%w: operand without a constant value", ErrUnsupported)
	}
	switch op { //nolint:exhaustive // the comparisons and the rest are handled below
	case token.LAND, token.LOR:
		if x.Kind() != constant.Bool || y.Kind() != constant.Bool {
			return nil, fmt.Errorf("%w: logical operands %v and %v", ErrUnsupported, x, y)
		}

		return constant.BinaryOp(x, op, y), nil
	case token.AND, token.OR, token.XOR, token.AND_NOT, token.SHL, token.SHR:
		return foldBitwise(x, op, y)
	}
	if op.IsOperator() && op.Precedence() == token.EQL.Precedence() {
		return constant.MakeBool(constant.Compare(x, op, y)), nil
	}
	if !numeric(x) || !numeric(y) {
		return nil, fmt.Errorf("%w: operands %v and %v", ErrUnsupported, x, y)
	}
	if (op == token.QUO || op == token.REM) && constant.Sign(y) == 0 {
		return nil, fmt.Errorf("%w: mutant divides by zero", ErrUnsupported)
	}
	if op == token.QUO && integerOperation(info, e) {
		x, y = constant.ToInt(x), constant.ToInt(y)
		if x.Kind() != constant.Int || y.Kind() != constant.Int {
			return nil, fmt.Errorf("%w: integer division of non-integers", ErrUnsupported)
		}
		op = token.QUO_ASSIGN // go/constant's integer division
	}
	if op == token.REM && (x.Kind() != constant.Int || y.Kind() != constant.Int) {
		return nil, fmt.Errorf("%w: remainder of non-integers", ErrUnsupported)
	}

	return constant.BinaryOp(x, op, y), nil
}

// foldBitwise folds a bitwise operator or a shift. Its operands are
// integers, or untyped constants of integer value; a shift count is
// non-negative and, in a program that type-checks, small.
func foldBitwise(x constant.Value, op token.Token, y constant.Value) (constant.Value, error) {
	x, y = constant.ToInt(x), constant.ToInt(y)
	if x.Kind() != constant.Int || y.Kind() != constant.Int {
		return nil, fmt.Errorf("%w: bitwise operands %v and %v", ErrUnsupported, x, y)
	}
	if op != token.SHL && op != token.SHR {
		return constant.BinaryOp(x, op, y), nil
	}
	n, exact := constant.Uint64Val(y)
	if !exact || n > maxShiftCount {
		return nil, fmt.Errorf("%w: shift count %v", ErrUnsupported, y)
	}

	return constant.Shift(x, op, uint(n)), nil
}

// maxShiftCount bounds a folded shift's count, as go/types bounds a constant
// shift's: anything it accepts is far below.
const maxShiftCount = 1 << 12

func numeric(v constant.Value) bool {
	return v.Kind() == constant.Int || v.Kind() == constant.Float
}

// integerOperation reports whether the binary e operates in an integer
// type: its typed operand's, or, both untyped, the larger untyped kind.
func integerOperation(info *types.Info, e *ast.BinaryExpr) bool {
	t := operationOperand(info, e)
	b, ok := info.Types[t].Type.Underlying().(*types.Basic)

	return ok && b.Info()&types.IsInteger != 0
}

// operationOperand returns the operand of e whose type the operation has:
// a shift's left operand; else the typed one, or, both untyped, the one of
// larger kind (int < rune < float < complex, which is the order of the
// untyped basic kinds).
func operationOperand(info *types.Info, e ast.Expr) ast.Expr {
	switch e := e.(type) {
	case *ast.UnaryExpr:
		return e.X
	case *ast.BinaryExpr:
		if e.Op == token.SHL || e.Op == token.SHR {
			return e.X
		}
		lt, rt := info.Types[e.X].Type, info.Types[e.Y].Type
		switch {
		case !writtenUntyped(info, e.X):
			return e.X
		case !writtenUntyped(info, e.Y):
			return e.Y
		case basicKind(rt) > basicKind(lt):
			return e.Y
		}

		return e.X
	}

	return e
}

// writtenUntyped reports whether e is an untyped constant as written: a
// literal, an untyped named constant, or an operation of only those. Its
// recorded type cannot tell: go/types records an untyped operand with the
// type it is converted to, so the 200 in 200*time.Millisecond is recorded as
// time.Duration. Taken as the typed operand, the literal was spelled as the
// carrier of the constant's type, (200)*0, which is untyped again: under :=
// the constant then defaulted to int and the package stopped compiling.
func writtenUntyped(info *types.Info, e ast.Expr) bool {
	switch e := ast.Unparen(e).(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return untypedConst(info.Uses[e])
	case *ast.SelectorExpr:
		return untypedConst(info.Uses[e.Sel])
	case *ast.UnaryExpr:
		return writtenUntyped(info, e.X)
	case *ast.BinaryExpr:
		if e.Op == token.SHL || e.Op == token.SHR {
			return writtenUntyped(info, e.X)
		}

		return writtenUntyped(info, e.X) && writtenUntyped(info, e.Y)
	}

	return false
}

// untypedConst reports whether obj is a constant declared without a type.
func untypedConst(obj types.Object) bool {
	c, ok := obj.(*types.Const)

	return ok && untyped(c.Type())
}

// basicKind is t's kind if t is a basic type, else Invalid.
func basicKind(t types.Type) types.BasicKind {
	if b, ok := t.(*types.Basic); ok {
		return b.Kind()
	}

	return types.Invalid
}

// intForm is the shift form. Each value must fit the context's type b.
func intForm(info *types.Info, sizes types.Sizes, e ast.Expr, c folded, b *types.Basic, prefix string, h *HelperSet, inner func(ast.Node) string) (string, error) {
	vals := append([]constant.Value{c.c0}, c.cs...)
	lits := make([]string, len(vals))
	for i, v := range vals {
		v = constant.ToInt(v)
		if v.Kind() != constant.Int || !fitsInt(v, b, sizes) {
			return "", fmt.Errorf("%w: value %v does not fit %v", ErrUnsupported, vals[i], b)
		}
		lits[i] = v.ExactString()
	}
	h.Use("Bit")
	var sb strings.Builder
	sb.WriteString("(")
	// The constant's type, where the spelling would lose it: a typed operand
	// carries it; an untyped rune operand gives its default type.
	if op := operationOperand(info, e); basicKind(info.Types[op].Type) != types.UntypedInt {
		sb.WriteString("(" + inner(op) + ")*0 + ")
	}
	sb.WriteString(lits[0] + "*(1")
	for _, id := range c.ids {
		sb.WriteString("-" + bitTerm(prefix, id))
	}
	sb.WriteString(")")
	for i, id := range c.ids {
		sb.WriteString(" + " + lits[i+1] + "*" + bitTerm(prefix, id))
	}
	sb.WriteString(")")

	return sb.String(), nil
}

// bitTerm is 1 when mutant id is active, else 0, in the context's type.
func bitTerm(prefix string, id int) string {
	return "(1<<" + prefix + "Bit(" + strconv.Itoa(id) + ")-1)"
}

// fitsInt reports whether the integer v is representable in b, an integer
// type of the platform sizes describes -- the target's, which int, uint and
// uintptr depend on. With no sizes it assumes the host's.
func fitsInt(v constant.Value, b *types.Basic, sizes types.Sizes) bool {
	if sizes == nil {
		sizes = hostSizes()
	}
	bits := uint(64)
	switch sizes.Sizeof(b) {
	case 1:
		bits = 8
	case 2:
		bits = 16
	case 4:
		bits = 32
	}
	lo, hi := constant.MakeInt64(0), constant.Shift(constant.MakeInt64(1), token.SHL, bits)
	if b.Info()&types.IsUnsigned == 0 {
		hi = constant.Shift(constant.MakeInt64(1), token.SHL, bits-1)
		lo = constant.UnaryOp(token.SUB, hi, 0)
	}

	return constant.Compare(v, token.GEQ, lo) && constant.Compare(v, token.LSS, hi)
}

// hostSizes are the gc compiler's sizes for the host's GOARCH, or amd64's.
func hostSizes() types.Sizes {
	if s := types.SizesFor("gc", build.Default.GOARCH); s != nil {
		return s
	}

	return types.SizesFor("gc", "amd64")
}

// boolForm calls the helper of the site's arity.
func boolForm(info *types.Info, e ast.Expr, c folded, prefix string, h *HelperSet, ctx ast.Node, spell spellFunc) (string, error) {
	conv, err := namedBool(info, e, ctx, spell)
	if err != nil {
		return "", err
	}
	n := len(c.ids)
	name := prefix + "Bool" + strconv.Itoa(n)
	var params, args, arms strings.Builder
	for i, id := range c.ids {
		k := strconv.Itoa(i + 1)
		params.WriteString("id" + k + ", ")
		args.WriteString(strconv.Itoa(id) + ", ")
		fmt.Fprintf(&arms, "\tcase %[1]sActive != 0 && %[1]sActive == id%[2]s:\n\t\t%[1]sReached()\n\t\treturn c%[2]s\n", prefix, k)
	}
	cs := []string{"c0"}
	vals := []string{c.c0.ExactString()}
	for i, v := range c.cs {
		cs = append(cs, "c"+strconv.Itoa(i+1))
		vals = append(vals, v.ExactString())
	}
	h.AddRaw(fmt.Sprintf("// %[1]s is a constant comparison's value: c0, or cK while mutant idK is active.\n"+
		"func %[1]s(%[2]sint, %[3]s bool) bool {\n\tswitch {\n%[4]s\t}\n\treturn c0\n}",
		name, strings.TrimSuffix(params.String(), ", ")+" ", strings.Join(cs, ", "), arms.String()))

	return convert(conv, name+"("+args.String()+strings.Join(vals, ", ")+")"), nil
}

// floatForm is the witness form, in a call argument, assignment or binary
// expression context, else the explicit form.
func floatForm(info *types.Info, e ast.Expr, c folded, b *types.Basic, prefix string, h *HelperSet, ctx ast.Node, inner func(ast.Node) string, spell spellFunc) (string, error) {
	vals := append([]constant.Value{c.c0}, c.cs...)
	lits := make([]string, len(vals))
	for i, v := range vals {
		lit, err := floatLit(v, b)
		if err != nil {
			return "", err
		}
		lits[i] = lit
	}
	name := prefix + "Site" + strconv.Itoa(c.ids[0])
	typeParams, witnessType, witness, err := floatWitness(info, e, b, ctx, inner)
	call := name + "(" + witness + ")"
	if err != nil {
		t, serr := spell(info.Types[e].Type)
		if serr != nil {
			return "", fmt.Errorf("%w; %s", err, strings.TrimPrefix(serr.Error(), ErrUnsupported.Error()+": "))
		}
		typeParams, witnessType, call = "T ~"+b.Name(), "", name+"["+t+"]()"
	}
	// Each value is converted to b, the constant's basic type, before T: an
	// untyped constant converted to a type parameter takes its default type,
	// float64, first, and a float32 T would then round it twice -- a value
	// near a float32 rounding midpoint would round otherwise than the plain
	// mutant, which converts the exact constant to float32 once. b(lit) is
	// that one rounding; b to T, of underlying type b, is exact.
	var arms strings.Builder
	for i, id := range c.ids {
		fmt.Fprintf(&arms, "\tif %[1]sActive != 0 && %[1]sActive == %[2]d {\n\t\t%[1]sReached()\n\t\treturn T(%[4]s(%[3]s))\n\t}\n", prefix, id, lits[i+1], b.Name())
	}
	h.AddRaw(fmt.Sprintf("// %[1]s is a float constant's value, in the type T inferred from the witness\n"+
		"// or given explicitly.\n"+
		"func %[1]s[%[2]s](%[3]s) T {\n%[4]s\treturn T(%[6]s(%[5]s))\n}",
		name, typeParams, witnessType, arms.String(), lits[0], b.Name()))

	return call, nil
}

// floatWitness returns the type parameters, the parameter type and the
// argument of the witness form of e, a float constant of type b in the
// context ctx, or why it has none.
func floatWitness(info *types.Info, e ast.Expr, b *types.Basic, ctx ast.Node, inner func(ast.Node) string) (string, string, string, error) {
	core := "~" + b.Name()
	switch ctx := ctx.(type) {
	case *ast.CallExpr:
		sig, err := witnessCallee(info, e, ctx, b)
		if err != nil {
			return "", "", "", err
		}
		i := slices.IndexFunc(ctx.Args, func(a ast.Expr) bool { return ast.Unparen(a) == e })
		var tps, ps, rs []string
		for j := range sig.Params().Len() {
			if j == i {
				ps = append(ps, "T")

				continue
			}
			tps = append(tps, "A"+strconv.Itoa(j))
			ps = append(ps, "A"+strconv.Itoa(j))
		}
		for j := range sig.Results().Len() {
			tps = append(tps, "R"+strconv.Itoa(j))
			rs = append(rs, "R"+strconv.Itoa(j))
		}
		typeParams := "T " + core
		if len(tps) > 0 {
			typeParams += ", " + strings.Join(tps, ", ") + " any"
		}
		witnessType := "func(" + strings.Join(ps, ", ") + ")"
		if len(rs) > 0 {
			witnessType += " (" + strings.Join(rs, ", ") + ")"
		}

		return typeParams, witnessType, inner(ctx.Fun), nil
	case *ast.AssignStmt:
		lhs, err := witnessLHS(info, e, ctx, b)
		if err != nil {
			return "", "", "", err
		}

		return "T " + core, "*T", "&" + inner(lhs), nil
	case *ast.BinaryExpr:
		other, err := witnessOperand(info, e, ctx, b)
		if err != nil {
			return "", "", "", err
		}

		return "T " + core, "T", inner(other), nil
	}

	return "", "", "", fmt.Errorf("%w: float constant in a %T", ErrUnsupported, ctx)
}

// floatLit spells v as an untyped constant expression of its exact value,
// refusing a value that overflows b. Its integers are spelled as float
// literals: an untyped integer constant of over 512 bits overflows the
// compiler, where a float one of the same value -- math.MaxFloat64, whose
// integer value takes 1024 bits -- does not.
func floatLit(v constant.Value, b *types.Basic) (string, error) {
	v = constant.ToFloat(v)
	f, _ := constant.Float64Val(v)
	if b.Kind() == types.Float32 {
		f32, _ := constant.Float32Val(v)
		f = float64(f32)
	}
	num, den := constant.Num(v), constant.Denom(v)
	if math.IsInf(f, 0) || num.Kind() != constant.Int || den.Kind() != constant.Int {
		return "", fmt.Errorf("%w: value %v does not fit %v", ErrUnsupported, v, b)
	}
	if constant.Compare(den, token.EQL, constant.MakeInt64(1)) {
		return num.ExactString() + ".0", nil
	}

	return num.ExactString() + ".0 / " + den.ExactString() + ".0", nil
}

// witnessCallee returns the signature of the call ctx that e is an argument
// of, if its callee can be passed as the witness: evaluated twice without
// effect (identifiers and selectors only), a function or variable that is
// not generic, with at most maxWitnessArity parameters and results, e not
// variadic, and e's parameter of the type b the constant took.
func witnessCallee(info *types.Info, e ast.Expr, call *ast.CallExpr, b *types.Basic) (*types.Signature, error) {
	i := slices.IndexFunc(call.Args, func(a ast.Expr) bool { return ast.Unparen(a) == e })
	if i < 0 || call.Ellipsis.IsValid() {
		return nil, fmt.Errorf("%w: float constant not a plain call argument", ErrUnsupported)
	}
	if info.Uses == nil {
		return nil, fmt.Errorf("%w: no Uses to check a float constant's callee", ErrUnsupported)
	}
	id := calleeIdent(info, call.Fun)
	if id == nil {
		return nil, fmt.Errorf("%w: float constant's callee is not a name or a qualified name", ErrUnsupported)
	}
	switch obj := info.Uses[id].(type) {
	case *types.Func:
		if obj.Signature().TypeParams() != nil {
			return nil, fmt.Errorf("%w: float constant's callee is generic", ErrUnsupported)
		}
	case *types.Var:
	default:
		return nil, fmt.Errorf("%w: float constant's callee is a %T", ErrUnsupported, obj)
	}
	sig, ok := info.Types[call.Fun].Type.Underlying().(*types.Signature)
	switch {
	case !ok:
		return nil, fmt.Errorf("%w: float constant's callee has no signature", ErrUnsupported)
	case sig.Variadic() && i >= sig.Params().Len()-1:
		return nil, fmt.Errorf("%w: float constant is a variadic argument", ErrUnsupported)
	case sig.Params().Len() > maxWitnessArity || sig.Results().Len() > maxWitnessArity:
		return nil, fmt.Errorf("%w: float constant's callee has over %d parameters or results", ErrUnsupported, maxWitnessArity)
	case !types.Identical(sig.Params().At(i).Type().Underlying(), b):
		return nil, fmt.Errorf("%w: float constant's parameter is %v", ErrUnsupported, sig.Params().At(i).Type())
	}

	return sig, nil
}

// calleeIdent returns the identifier naming a callee that is a plain or
// package-qualified name, or nil. Evaluating either a second time, as the
// witness, has no effect and cannot panic. A method value or a field
// (x.M, x.f) is refused: a nil x panics when the witness is evaluated, among
// the arguments, where the original panics at the call, after all of them --
// a later argument's effects would be lost.
func calleeIdent(info *types.Info, fun ast.Expr) *ast.Ident {
	switch f := ast.Unparen(fun).(type) {
	case *ast.Ident:
		return f
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok {
			if _, pkg := info.Uses[x].(*types.PkgName); pkg {
				return f.Sel
			}
		}
	}

	return nil
}

// callFree reports whether x is identifiers and selectors only.
func callFree(x ast.Expr) bool {
	switch x := ast.Unparen(x).(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return callFree(x.X)
	}

	return false
}

// witnessLHS returns the left side assigned e by the single plain assignment
// st, if it can be passed as &lhs: addressable, of the type b the constant
// took, and identifiers and selectors only. The witness is the original
// text, so the lhs must hold no site (no index, no call), whose rendered
// form the assignment would evaluate but the witness would not; and it must
// dereference no pointer, which the witness would do before the site records
// its reach.
func witnessLHS(info *types.Info, e ast.Expr, st *ast.AssignStmt, b *types.Basic) (ast.Expr, error) {
	i := slices.IndexFunc(st.Rhs, func(r ast.Expr) bool { return ast.Unparen(r) == e })
	if st.Tok != token.ASSIGN || i < 0 || len(st.Lhs) != 1 || len(st.Rhs) != 1 {
		return nil, fmt.Errorf("%w: float constant not the value of a plain assignment", ErrUnsupported)
	}
	lhs := st.Lhs[i]
	tv := info.Types[lhs]
	switch {
	case !tv.Addressable():
		return nil, fmt.Errorf("%w: float constant assigned to a non-addressable operand", ErrUnsupported)
	case tv.Type == nil || !types.Identical(tv.Type.Underlying(), b):
		return nil, fmt.Errorf("%w: float constant assigned to a %v", ErrUnsupported, tv.Type)
	case !callFree(lhs):
		return nil, fmt.Errorf("%w: float constant assigned to more than a name or selector", ErrUnsupported)
	case !derefFree(info, lhs):
		return nil, fmt.Errorf("%w: float constant assigned through a pointer", ErrUnsupported)
	}

	return lhs, nil
}

// witnessOperand returns the other operand of the binary expression bin
// that e is an operand of, if it can be passed as the witness: of the type b
// the constant took, and, evaluated a second time, without effect and unable
// to panic -- identifiers and selectors only, dereferencing no pointer.
func witnessOperand(info *types.Info, e ast.Expr, bin *ast.BinaryExpr, b *types.Basic) (ast.Expr, error) {
	var other ast.Expr
	switch ast.Unparen(e) {
	case ast.Unparen(bin.X):
		other = bin.Y
	case ast.Unparen(bin.Y):
		other = bin.X
	default:
		return nil, fmt.Errorf("%w: float constant not an operand of its binary context", ErrUnsupported)
	}
	tv := info.Types[other]
	switch {
	case tv.Type == nil || untyped(tv.Type) || !types.Identical(tv.Type.Underlying(), b):
		return nil, fmt.Errorf("%w: float constant's other operand is a %v", ErrUnsupported, tv.Type)
	case !callFree(other):
		return nil, fmt.Errorf("%w: float constant's other operand is more than a name or selector", ErrUnsupported)
	case !derefFree(info, other):
		return nil, fmt.Errorf("%w: float constant's other operand goes through a pointer", ErrUnsupported)
	}

	return other, nil
}

// derefFree reports whether evaluating &x, for x of identifiers and selectors
// only, dereferences no pointer: no field selection in it goes through one,
// implicitly or through an embedded field. A nil pointer there would panic
// among the witness's arguments, before the site records its reach, where
// the original panics at the store, after it. A package-qualified name
// dereferences nothing; a selection the type information does not hold is
// refused.
func derefFree(info *types.Info, x ast.Expr) bool {
	sel, ok := ast.Unparen(x).(*ast.SelectorExpr)
	if !ok {
		return true
	}
	if id, ok := ast.Unparen(sel.X).(*ast.Ident); ok && info.Uses != nil {
		if _, pkg := info.Uses[id].(*types.PkgName); pkg {
			return true
		}
	}
	s, ok := info.Selections[sel]
	if !ok || s.Kind() != types.FieldVal || s.Indirect() {
		return false
	}
	if _, ptr := s.Recv().Underlying().(*types.Pointer); ptr {
		return false
	}

	return derefFree(info, sel.X)
}
