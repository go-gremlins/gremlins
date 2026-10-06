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

package schemata_test

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

// beforeDuplicates is a rendered file without the function duplicates
// Render appends after its last line: the part whose lines must be the
// original's, one for one.
func beforeDuplicates(out []byte) []byte {
	if i := bytes.Index(out, []byte("\n\n//line :")); i >= 0 {
		return out[:i+1]
	}

	return out
}

// TestDupBehave is TestFormsBehave for the sites placed by duplicating the
// enclosing function: const declarations, array lengths and constant
// composite literal keys inside function bodies. Every mutant of
// testdata/dup is placed, and under each the schema prints what the plain
// token mutant prints -- a panic in a duplicate on the original's line --
// and records its reach.
func TestDupBehave(t *testing.T) {
	t.Parallel()
	fx := renderFixture(t, "dup", formsMutators)
	if len(fx.gone) != 0 {
		t.Errorf("fixture has mutants that do not compile: %v", fx.gone)
	}
	var subs int
	for _, p := range fx.plain {
		if p.tok == token.SUB && p.mt == mutator.InvertNegatives {
			subs++
		}
	}
	// TwoMutants, M, RecoverInCopy, PanicLine and MapKey each have two
	// mutants at one -, each placed by a duplicate of its own.
	if subs != 5 {
		t.Errorf("fixture has %d INVERT_NEGATIVES mutants at a binary -, want 5", subs)
	}
	fx.runBehave(t, []string{
		"LocalConstArray: 12\n", "LocalConstIota: [1 2 4]\n", "LocalConstTypedFloat: 3\n", "ArrayLenLiteral: 8\n",
		"ArrayIndexKey: [0 0 0 9]\n", "MapKey: map[2:a]\n", "Method: 2\n", "Generic: 7\n", "RecoverInCopy: 8\n",
		"PanicLine: 8\n", "TwoMutants: 4\n",
	})
}

// dupCase is a compile-time site and the file Render makes of it.
type dupCase struct {
	src  string // a file after "package p\n\n"
	tok  token.Token
	muts []mutator.Type
	want string // the rendered file after "package p\n\n"; "" with refusal
	// refusal is a text the SiteError must hold when the site is refused.
	refusal string
}

// TestRenderDup checks the text of a duplicated function: the jump spliced
// after the original's opening brace, unnamed and blank parameters named in
// both, and the copy -- with only the site's operator mutated -- appended
// after a line directive that gives it the original's position.
func TestRenderDup(t *testing.T) {
	t.Parallel()
	ab := mutator.ArithmeticBase
	in := mutator.InvertNegatives
	cases := map[string]dupCase{
		"const_decl": {
			src: "func f() int {\n\tconst k = 1 + 2\n\treturn k\n}\n", tok: token.ADD, muts: []mutator.Type{ab},
			want: "func f() int {if _zzActive == 1 { _zzReached(); return _zz_D1_f() };\n\tconst k = 1 + 2\n\treturn k\n}\n" +
				"\n//line :3:1\nfunc _zz_D1_f() int {\n\tconst k = 1 - 2\n\treturn k\n}\n",
		},
		"no_results": {
			src: "var sink int\n\nfunc f() {\n\tvar a [2 * 2]int\n\tsink = len(a)\n}\n", tok: token.MUL, muts: []mutator.Type{ab},
			want: "var sink int\n\nfunc f() {if _zzActive == 1 { _zzReached(); _zz_D1_f(); return };\n\tvar a [2 * 2]int\n\tsink = len(a)\n}\n" +
				"\n//line :5:1\nfunc _zz_D1_f() {\n\tvar a [2 / 2]int\n\tsink = len(a)\n}\n",
		},
		"two_mutants": {
			src: "func f() int { const k = 6 - 2; return k }\n", tok: token.SUB, muts: []mutator.Type{ab, in},
			want: "func f() int {if _zzActive == 1 { _zzReached(); return _zz_D1_f() };if _zzActive == 2 { _zzReached(); return _zz_D2_f() }; const k = 6 - 2; return k }\n" +
				"\n//line :3:1\nfunc _zz_D1_f() int { const k = 6 + 2; return k }\n" +
				"\n//line :3:1\nfunc _zz_D2_f() int { const k = 6 + 2; return k }\n",
		},
		"method_unnamed": {
			src: "type T struct{}\n\nfunc (T) M(int, string) int { const k = 3 - 1; return k }\n", tok: token.SUB, muts: []mutator.Type{ab},
			want: "type T struct{}\n\nfunc (_zz_a0 T) M(_zz_a1 int, _zz_a2 string) int {if _zzActive == 1 { _zzReached(); return _zz_a0._zz_D1_M(_zz_a1, _zz_a2) }; const k = 3 - 1; return k }\n" +
				"\n//line :5:1\nfunc (_zz_a0 T) _zz_D1_M(_zz_a1 int, _zz_a2 string) int { const k = 3 + 1; return k }\n",
		},
		"blank_params_named_results": {
			src: "type T struct{}\n\nfunc (_ *T) M(a, _ int) (r int) { r = len([...]int{1 + 1: 7}) - a; return }\n", tok: token.ADD, muts: []mutator.Type{ab},
			want: "type T struct{}\n\nfunc (_zz_a0 *T) M(a, _zz_a2 int) (r int) {if _zzActive == 1 { _zzReached(); return _zz_a0._zz_D1_M(a, _zz_a2) }; r = len([...]int{1 + 1: 7}) - a; return }\n" +
				"\n//line :5:1\nfunc (_zz_a0 *T) _zz_D1_M(a, _zz_a2 int) (r int) { r = len([...]int{1 - 1: 7}) - a; return }\n",
		},
		"generic_variadic": {
			src: "func G[E any, F ~int](f F, xs ...E) int { var a [2 + 2]E; return len(a) + len(xs) + int(f) }\n", tok: token.ADD, muts: []mutator.Type{ab},
			want: "func G[E any, F ~int](f F, xs ...E) int {if _zzActive == 1 { _zzReached(); return _zz_D1_G[E, F](f, xs...) }; var a [2 + 2]E; return len(a) + len(xs) + int(f) }\n" +
				"\n//line :3:1\nfunc _zz_D1_G[E any, F ~int](f F, xs ...E) int { var a [2 - 2]E; return len(a) + len(xs) + int(f) }\n",
		},
		"map_key": {
			src: "func f() map[int]bool { return map[int]bool{2 - 1: true} }\n", tok: token.SUB, muts: []mutator.Type{in},
			want: "func f() map[int]bool {if _zzActive == 1 { _zzReached(); return _zz_D1_f() }; return map[int]bool{2 - 1: true} }\n" +
				"\n//line :3:1\nfunc _zz_D1_f() map[int]bool { return map[int]bool{2 + 1: true} }\n",
		},
		"in_func_literal": {
			src: "func f() int {\n\treturn func() int { const k = 2 * 3; return k }()\n}\n", tok: token.MUL, muts: []mutator.Type{ab},
			want: "func f() int {if _zzActive == 1 { _zzReached(); return _zz_D1_f() };\n\treturn func() int { const k = 2 * 3; return k }()\n}\n" +
				"\n//line :3:1\nfunc _zz_D1_f() int {\n\treturn func() int { const k = 2 / 3; return k }()\n}\n",
		},
		// recover only in a closure the function defers, or a shadowing
		// local named recover: the function is still duplicated.
		"recover_in_closure": {
			src: "func f() (r int) { defer func() { _ = recover() }(); const k = 1 + 2; return k }\n", tok: token.ADD, muts: []mutator.Type{ab},
			want: "func f() (r int) {if _zzActive == 1 { _zzReached(); return _zz_D1_f() }; defer func() { _ = recover() }(); const k = 1 + 2; return k }\n" +
				"\n//line :3:1\nfunc _zz_D1_f() (r int) { defer func() { _ = recover() }(); const k = 1 - 2; return k }\n",
		},
		"recover_shadowed": {
			src: "func f() int { recover := func() int { return 0 }; const k = 1 + 2; return k + recover() }\n", tok: token.ADD, muts: []mutator.Type{ab},
			want: "func f() int {if _zzActive == 1 { _zzReached(); return _zz_D1_f() }; recover := func() int { return 0 }; const k = 1 + 2; return k + recover() }\n" +
				"\n//line :3:1\nfunc _zz_D1_f() int { recover := func() int { return 0 }; const k = 1 - 2; return k + recover() }\n",
		},
		"noinline": {
			src: "//go:noinline\nfunc f() int { const k = 2 * 3; return k }\n", tok: token.MUL, muts: []mutator.Type{ab},
			want: "//go:noinline\nfunc f() int {if _zzActive == 1 { _zzReached(); return _zz_D1_f() }; const k = 2 * 3; return k }\n" +
				"\n//line :4:1\nfunc _zz_D1_f() int { const k = 2 / 3; return k }\n",
		},
	}
	runDupCases(t, cases)
}

// TestDupRefusals checks the compile-time sites that are not duplicated,
// each refused with its reason.
func TestDupRefusals(t *testing.T) {
	t.Parallel()
	ab := mutator.ArithmeticBase
	cases := map[string]dupCase{
		"init":                 {src: "var sink int\n\nfunc init() { const k = 1 + 2; sink = k }\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "compile-time site in init"},
		"nosplit":              {src: "//go:nosplit\nfunc f() int { const k = 1 + 2; return k }\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "//go:nosplit"},
		"package_func_literal": {src: "var f = func() int { const k = 1 + 2; return k }\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "in a function literal outside a function declaration"},
		"package_const":        {src: "const k = 1 + 2\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "constant site in a const declaration"},
		"package_array_len":    {src: "var a [1 + 1]int\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "constant site in an array length"},
		"package_key":          {src: "var s = []int{1 + 1: 5}\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "constant composite literal key"},
		"signature_array_len":  {src: "func f(a [1 + 1]int) int { return len(a) }\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "constant site in an array length"},
		"blank_type_param":     {src: "func f[_ any]() int { const k = 1 + 2; return k }\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "blank type parameter"},
		"recover_direct":       {src: "var sink int\n\nfunc handler() { const k = 1 + 2; if recover() != nil { sink = k } }\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "calls recover"},
		"line_directive":       {src: "//line other.go:10\nfunc f() int { const k = 1 + 2; return k }\n", tok: token.ADD, muts: []mutator.Type{ab}, refusal: "line directive"},
	}
	runDupCases(t, cases)
}

// runDupCases renders each case's first site with tok through NewRewriter.
func runDupCases(t *testing.T, cases map[string]dupCase) {
	t.Helper()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := []byte("package p\n\n" + tc.src)
			fset := token.NewFileSet()
			f, info := typeCheck(t, fset, "p.go", src)
			var node ast.Node
			ast.Inspect(f, func(n ast.Node) bool {
				if tn, ok := engine.NewTokenNode(n); ok && tn.Tok() == tc.tok && node == nil {
					node = n
				}

				return true
			})
			if node == nil {
				t.Fatalf("no node with %s", tc.tok)
			}
			site := schemata.Site{Node: node, Tok: tc.tok}
			for i, mt := range tc.muts {
				site.Muts = append(site.Muts, schemata.Mutant{ID: i + 1, Type: mt})
			}
			out, errs := schemata.Render(fset, fset.File(f.Pos()), src, []schemata.Site{site},
				schemata.NewRewriter(info, nil, []*ast.File{f}, testPrefix, &schemata.HelperSet{}))
			if tc.want == "" {
				if len(errs) != 1 || !errors.Is(errs[0].Err, schemata.ErrUnsupported) || !strings.Contains(errs[0].Err.Error(), tc.refusal) {
					t.Errorf("errors %v; want one ErrUnsupported holding %q", errs, tc.refusal)
				}
				if !bytes.Equal(out, src) {
					t.Errorf("refused site changed the file:\n%s", out)
				}

				return
			}
			if len(errs) != 0 {
				t.Fatalf("site refused: %v", errs)
			}
			if got := strings.TrimPrefix(string(out), "package p\n\n"); got != tc.want {
				t.Errorf("rendered\n%s\nwant\n%s", got, tc.want)
			}
			if got, want := bytes.Count(beforeDuplicates(out), []byte("\n")), bytes.Count(src, []byte("\n")); got != want {
				t.Errorf("original part has %d lines, the original %d", got, want)
			}
			typeCheck(t, token.NewFileSet(), "p.go", slices.Concat(out, stubHelpers))
		})
	}
}

// stubHelpers declares the helpers a duplicate's jump calls, for a
// type-check of a rendered file without the helper file.
var stubHelpers = []byte("\nvar _zzActive int\n\nfunc _zzReached() {}\n")

// TestRewritePackageDupTypeError checks that a type error in a duplicate
// drops the mutant the duplicate belongs to, and no other: not the site's
// other mutants, not the function's other sites, not the package.
func TestRewritePackageDupTypeError(t *testing.T) {
	t.Parallel()
	src := "package p\n\nfunc F() int {\n\tconst a uint8 = 200 - 100\n\tconst b = 6 - 2\n\treturn int(a) * b\n}\n"
	type mutantKey struct {
		line int
		mt   mutator.Type
	}
	ab, in, line4, line5 := mutator.ArithmeticBase, mutator.InvertNegatives, 4, 5
	testCases := map[string]struct {
		// breakMutant, when set, is a mutant whose duplicate is made not to
		// type-check.
		breakMutant *mutantKey
		dropped     []mutantKey
	}{
		// 200 + 100 overflows uint8: both mutants at a fail, b's are placed.
		"overflowing_copies": {dropped: []mutantKey{{line4, ab}, {line4, in}}},
		// One of b's two duplicates is broken: its other mutant stays.
		"one_mutant_of_a_site": {breakMutant: &mutantKey{line5, in}, dropped: []mutantKey{{line4, ab}, {line4, in}, {line5, in}}},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := writeModule(t, map[string][]byte{
				"p.go":      []byte(src),
				"p_test.go": []byte("package p\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) { _ = F() }\n"),
			})
			pkg := loadPkg(t, dir, ".")
			sites := pkgSites(pkg)
			key := func(s schemata.Site, m schemata.Mutant) mutantKey {
				return mutantKey{pkg.Fset.Position(s.Node.Pos()).Line, m.Type}
			}
			factory := schemata.RewriterFactory(schemata.NewRewriter)
			if tc.breakMutant != nil {
				factory = func(info *types.Info, sizes types.Sizes, files []*ast.File, prefix string, h *schemata.HelperSet) schemata.Rewriter {
					rw := schemata.NewRewriter(info, sizes, files, prefix, h)

					return func(s schemata.Site, inner func(ast.Node) string) (string, error) {
						out, err := rw(s, inner)
						for _, m := range s.Muts {
							if key(s, m) == *tc.breakMutant {
								err = schemata.BreakDupMutant(err, m.ID)
							}
						}

						return out, err
					}
				}
			}
			files, placed, dropped := schemata.RewritePackageWith(context.Background(), pkg, sites, "", factory)

			var in, got, gotDropped []mutantKey
			for _, s := range sites {
				for _, m := range s.Muts {
					in = append(in, key(s, m))
				}
			}
			for _, s := range placed {
				for _, m := range s.Muts {
					got = append(got, key(s, m))
				}
			}
			for _, d := range dropped {
				if !errors.Is(d.Err, schemata.ErrTypeCheck) {
					t.Errorf("%v dropped with %v, want ErrTypeCheck", d.Site.Muts, d.Err)
				}
				for _, m := range d.Site.Muts {
					gotDropped = append(gotDropped, key(d.Site, m))
				}
			}
			got = append(got, gotDropped...)
			cmp := func(a, b mutantKey) int { return a.line*100 + int(a.mt) - b.line*100 - int(b.mt) }
			for _, l := range [][]mutantKey{in, got, gotDropped, tc.dropped} {
				slices.SortFunc(l, cmp)
			}
			if !slices.Equal(got, in) {
				t.Errorf("placed+dropped = %v, input mutants = %v", got, in)
			}
			if !slices.Equal(gotDropped, tc.dropped) {
				t.Errorf("dropped %v, want %v", gotDropped, tc.dropped)
			}
			checkOverlay(t, pkg.Dir, files)
		})
	}
}
