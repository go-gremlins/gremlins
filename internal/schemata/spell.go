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
)

// spell returns t as the source at pos can name it: a predeclared type, a
// type or type parameter whose name resolves at pos to it, or an exported
// type of a package the file imports, qualified by the import's name where
// that name is not shadowed. A type any of whose spellings would need type
// arguments, or one no name at pos reaches, is refused: a type checker
// rejects a spelling that does not resolve, but one that resolves to some
// other type would change the program.
func (r *rewriter) spell(t types.Type, pos token.Pos) (string, error) {
	if a, ok := t.(*types.Alias); ok {
		if s, err := r.spellObject(a.Obj(), a.TypeArgs(), pos); err == nil {
			return s, nil
		}
		t = types.Unalias(a)
	}
	switch t := t.(type) {
	case *types.Basic:
		if t.Info()&types.IsUntyped != 0 {
			return "", fmt.Errorf("%w: untyped %v has no spelling", ErrUnsupported, t)
		}

		obj := types.Universe.Lookup(t.Name())
		if obj == nil {
			return "", fmt.Errorf("%w: type %v is not predeclared", ErrUnsupported, t)
		}

		return r.spellObject(obj, nil, pos)
	case *types.Named:
		return r.spellObject(t.Obj(), t.TypeArgs(), pos)
	case *types.TypeParam:
		return r.spellObject(t.Obj(), nil, pos)
	}

	return "", fmt.Errorf("%w: type %v has no name to spell it by", ErrUnsupported, t)
}

// spellObject spells the type name obj, which has the type arguments args,
// at pos.
func (r *rewriter) spellObject(obj types.Object, args *types.TypeList, pos token.Pos) (string, error) {
	if args.Len() > 0 {
		return "", fmt.Errorf("%w: type %s has type arguments", ErrUnsupported, obj.Name())
	}
	scope, err := r.scopeAt(pos)
	if err != nil {
		return "", err
	}
	if _, found := scope.LookupParent(obj.Name(), pos); found == obj {
		return obj.Name(), nil
	}
	if obj.Pkg() != nil && obj.Exported() {
		file := r.fileScope(scope)
		for _, name := range file.Names() {
			pn, ok := file.Lookup(name).(*types.PkgName)
			if !ok || pn.Imported() != obj.Pkg() {
				continue
			}
			if _, found := scope.LookupParent(name, pos); found == pn {
				return name + "." + obj.Name(), nil
			}
		}
	}

	return "", fmt.Errorf("%w: type %v is not nameable at the site", ErrUnsupported, obj.Type())
}

// scopeAt returns the innermost scope of the package's files that holds pos.
func (r *rewriter) scopeAt(pos token.Pos) (*types.Scope, error) {
	if r.info.Scopes == nil {
		return nil, fmt.Errorf("%w: no Scopes to name a type in", ErrUnsupported)
	}
	for _, f := range r.files {
		if f.FileStart <= pos && pos <= f.FileEnd {
			if fs := r.info.Scopes[f]; fs != nil {
				if s := fs.Innermost(pos); s != nil {
					return s, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("%w: no scope holds the site", ErrUnsupported)
}

// fileScope returns the file scope s is, or is nested in.
func (r *rewriter) fileScope(s *types.Scope) *types.Scope {
	for s.Parent() != nil && s.Parent().Parent() != types.Universe {
		s = s.Parent()
	}

	return s
}

// spellFunc spells a type at a fixed site.
type spellFunc func(types.Type) (string, error)

// spellAt returns the spellFunc of the site at pos.
func (r *rewriter) spellAt(pos token.Pos) spellFunc {
	return func(t types.Type) (string, error) { return r.spell(t, pos) }
}

// namedBool returns the type a helper call standing for the boolean e, in
// the context ctx, must be converted to: "" when e has type bool, or is the
// operand of a conversion, which converts the helper's bool as it converted
// e's untyped result; else the type e took from its context -- a named
// boolean type or a type parameter, whose spelling at the site the
// conversion is. A type that cannot be spelled there is refused.
func namedBool(info *types.Info, e ast.Expr, ctx ast.Node, spell spellFunc) (string, error) {
	t := info.Types[e].Type
	if t == nil {
		return "", fmt.Errorf("%w: comparison without type information", ErrUnsupported)
	}
	if types.Identical(t, types.Typ[types.Bool]) || types.Identical(t, types.Typ[types.UntypedBool]) {
		return "", nil
	}
	if b, ok := t.Underlying().(*types.Basic); ok && b.Info()&types.IsBoolean == 0 || !ok && !isTypeParam(t) {
		return "", fmt.Errorf("%w: comparison of type %v, not boolean", ErrUnsupported, t)
	}
	if call, ok := ctx.(*ast.CallExpr); ok && info.Types[call.Fun].IsType() && len(call.Args) == 1 && ast.Unparen(call.Args[0]) == e {
		return "", nil
	}
	s, err := spell(t)
	if err != nil {
		return "", fmt.Errorf("%w: comparison of type %v: %s", ErrUnsupported, t, strings.TrimPrefix(err.Error(), ErrUnsupported.Error()+": "))
	}

	return s, nil
}

// isTypeParam reports whether t is a type parameter.
func isTypeParam(t types.Type) bool {
	_, ok := types.Unalias(t).(*types.TypeParam)

	return ok
}

// convert returns call converted to the type spelled conv, or call itself
// when conv is empty.
func convert(conv, call string) string {
	if conv == "" {
		return call
	}

	return conv + "(" + call + ")"
}
