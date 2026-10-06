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
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

// TestConstantGroupsBehave is TestFormsBehave for the sites inside a
// constant-valued operand of a non-constant expression: every site of
// testdata/constgroup is placed -- each maximal constant operand as one
// constant site -- and under each mutant id the schema prints what the plain
// token mutant prints, and records the reach.
func TestConstantGroupsBehave(t *testing.T) {
	t.Parallel()
	fx := renderFixture(t, "constgroup", formsMutators)
	// The fixture must hold what it is there for: in Room, both mutants of
	// each of the two - folded into the constant, and the third - outside it.
	roomLine := 0
	for i, l := range strings.Split(string(fx.src), "\n") {
		if strings.Contains(l, "return maxIdentifier - len(databaseNamePrefix)") {
			roomLine = i + 1
		}
	}
	var room []string
	for _, p := range fx.plain {
		if fx.fset.Position(p.pos).Line == roomLine {
			room = append(room, fmt.Sprintf("%s@%d", p.mt, fx.fset.Position(p.pos).Column))
		}
	}
	if len(room) != 6 {
		t.Errorf("Room has mutants %v, want ARITHMETIC_BASE and INVERT_NEGATIVES at each of three -", room)
	}
	// Wide's / -> * overflows int: the plain mutant does not compile, and is
	// never generated.
	if !slices.ContainsFunc(fx.gone, func(p plainMutant) bool { return p.tok == token.QUO }) {
		t.Error("Wide's overflowing / mutant was not dropped as not viable")
	}
	fx.runBehave(t, []string{
		"Room: 51\n", "Paren: 17\n", "NegSum: -21\n", "Compare: true false\n", "FloatOperand: 6\n", "FloatGroup: 5\n",
		"FloatNamed: 7.5\n", "Converted: 17\n", "Wide: 2305843009213693953\n",
		"LenArrayLength: 13\n", "LenArrayLengthGroup: 12\n", "LenElements: 7\n",
	})
}

// TestConstantGroupRefusals renders one function's sites, grouped, and
// requires exactly the mutants named to be refused, each with its reason,
// and every other mutant placed. A per-mutant refusal is a mutant whose
// folded value would not compile -- a division by zero, a value its type
// cannot hold -- so the engine's type check is asked too, and must find the
// plain mutant not viable: it is never generated.
func TestConstantGroupRefusals(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		body string
		// refused maps "<mutator> <token>" to a fragment of the reason.
		refused map[string]string
		// notViable are the refused mutants the engine never generates.
		notViable []string
	}{
		"conversion_to_string_on_the_path": {
			body: "func f(s string) bool { return string(rune(64+1)) == s }",
			refused: map[string]string{
				"ARITHMETIC_BASE +": "conversion to string",
			},
		},
		"division_by_zero": {
			body: "func f(n int) int { return n + 6/(1*0+1) }",
			refused: map[string]string{
				"ARITHMETIC_BASE *": "divides by zero",
			},
			notViable: []string{"ARITHMETIC_BASE *"},
		},
		"every_mutant_refused_each_for_its_reason": {
			body: "func f(v uint8) uint8 { return uint8(250/(1*5)) + v }",
			refused: map[string]string{
				"ARITHMETIC_BASE /": "does not fit",
				"ARITHMETIC_BASE *": "divides by zero",
			},
			notViable: []string{"ARITHMETIC_BASE /", "ARITHMETIC_BASE *"},
		},
		"len_of_a_constant_string": {
			body: "func f(n int) int { return len(string(rune(127+1)))*3 + n }",
			refused: map[string]string{
				"ARITHMETIC_BASE +": "call between a constant group and its site",
			},
		},
		"len_of_a_constant_string_const": {
			body: "const k = 127\nfunc f(n int) int { return len(string(rune(k+1)))*3 + n }",
			refused: map[string]string{
				"ARITHMETIC_BASE +": "call between a constant group and its site",
			},
		},
		"overflow_in_a_conversion": {
			body: "func f(v uint8) uint8 { return uint8(255/5) + v }",
			refused: map[string]string{
				"ARITHMETIC_BASE /": "does not fit",
			},
			notViable: []string{"ARITHMETIC_BASE /"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := []byte("package p\n\n" + tc.body + "\n")
			fset := token.NewFileSet()
			f, info := typeCheck(t, fset, "p.go", src)
			sites, plain := discover(f)
			grouped := schemata.GroupConstantSites(info, []*ast.File{f}, sites)
			out, errs := schemata.Render(fset, fset.File(f.Pos()), src, grouped,
				schemata.NewRewriter(info, nil, []*ast.File{f}, testPrefix, &schemata.HelperSet{}))
			if got, want := bytes.Count(out, []byte("\n")), bytes.Count(src, []byte("\n")); got != want {
				t.Errorf("output has %d lines, the source %d", got, want)
			}
			key := func(id int) string {
				i := slices.IndexFunc(plain, func(p plainMutant) bool { return p.id == id })

				return fmt.Sprintf("%s %s", plain[i].mt, plain[i].tok)
			}
			got := map[string]string{}
			for _, e := range errs {
				if !errors.Is(e.Err, schemata.ErrUnsupported) {
					t.Errorf("refusal %v does not wrap ErrUnsupported", e.Err)
				}
				for _, m := range e.Site.Muts {
					got[key(m.ID)] = e.Err.Error()
				}
			}
			for k, frag := range tc.refused {
				if !strings.Contains(got[k], frag) {
					t.Errorf("%s: refusal %q, want one containing %q", k, got[k], frag)
				}
			}
			for k, reason := range got {
				if _, ok := tc.refused[k]; !ok {
					t.Errorf("%s refused: %s", k, reason)
				}
			}
			if len(got) < len(plain) && bytes.Equal(out, src) {
				t.Error("no mutant was placed")
			}
			if len(tc.notViable) == 0 {
				return
			}
			dir := writeModule(t, map[string][]byte{"p.go": src})
			v := engine.NewTypeViability(dir, "")
			for _, p := range plain {
				k := key(p.id)
				to, _ := engine.TokenMutation(p.mt, p.tok)
				pos := fset.Position(p.pos)
				pos.Filename = filepath.Join(dir, "p.go")
				if want := !slices.Contains(tc.notViable, k); v.Viable(pos, to) != want {
					t.Errorf("%s: engine viability %v, want %v", k, !want, want)
				}
			}
		})
	}
}

// TestGroupConstantSites checks which sites are grouped: those inside a
// maximal constant-valued expression outside a compile-time context, into
// one site per such expression, its mutants theirs; every other site is
// left as it is.
func TestGroupConstantSites(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		body string
		// groups are the texts of the grouped sites' nodes, in order, with
		// the number of members each.
		groups map[string]int
		single int // the sites left alone
	}{
		"room": {
			body:   "const m = 63\nfunc f(s string) int { return m - len(\"ab\") - len(\"_\") - len(s) }",
			groups: map[string]int{`m - len("ab") - len("_")`: 2}, single: 1,
		},
		"lone_constant_site_stays": {
			body: "func f(n int) int { return 2*3 + n }", single: 2,
		},
		"paren_alone_stays": {
			body: "func f(n int) int { return (2*3) + n }", single: 2,
		},
		"conversion_operand": {
			body: "func f(v int64) int64 { return int64(3*4) + v }", groups: map[string]int{"int64(3*4)": 1}, single: 1,
		},
		"const_declaration_stays": {
			body: "func f() int { const k = 2*3 + 1; return k }", single: 2,
		},
		"array_length_stays": {
			body: "func f() int { var a [2*3 + 1]int; return len(a) }", single: 2,
		},
		"array_length_under_lone_site": {
			body: "func f(n int) int { return len([2*3]int{})*2 + n }", single: 3,
		},
		"array_length_under_group": {
			body:   "func f(n int) int { return len([2*3]int{})*2 - 1 + n }",
			groups: map[string]int{"len([2*3]int{})*2 - 1": 2}, single: 2,
		},
		"elements_under_len": {
			body:   "func f(x, n int) int { return len([2]int{x + 1, 0})*3 + n }",
			groups: map[string]int{"len([2]int{x + 1, 0})*3": 2}, single: 1,
		},
		"len_of_a_string_is_not_type_only": {
			body:   "func f(n int) int { return len(string(rune(127+1)))*3 + n }",
			groups: map[string]int{"len(string(rune(127+1)))*3": 2}, single: 1,
		},
		"two_groups": {
			body:   "func f(n int) int { return -(2+1)*n + (4*3 - 1) }",
			groups: map[string]int{"-(2+1)": 2, "(4*3 - 1)": 2}, single: 2,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := []byte("package p\n\n" + tc.body + "\n")
			fset := token.NewFileSet()
			f, info := typeCheck(t, fset, "p.go", src)
			sites, _ := discover(f)
			grouped := schemata.GroupConstantSites(info, []*ast.File{f}, sites)
			gotGroups, single, muts := map[string]int{}, 0, 0
			for _, s := range grouped {
				muts += len(s.Muts)
				if len(s.Members) == 0 {
					single++

					continue
				}
				members := 0
				for _, m := range s.Members {
					members += len(m.Muts)
				}
				if members != len(s.Muts) {
					t.Errorf("group's members hold %d mutants, the group %d", members, len(s.Muts))
				}
				gotGroups[string(src[fset.Position(s.Node.Pos()).Offset:fset.Position(s.Node.End()).Offset])] = len(s.Members)
			}
			want := 0
			for _, s := range sites {
				want += len(s.Muts)
			}
			if muts != want {
				t.Errorf("grouped sites hold %d mutants, the input %d", muts, want)
			}
			if single != tc.single || len(gotGroups) != len(tc.groups) {
				t.Errorf("got %d single sites and groups %v, want %d and %v", single, gotGroups, tc.single, tc.groups)
			}
			for text, n := range tc.groups {
				if gotGroups[text] != n {
					t.Errorf("group %q has %d members, want %d (groups %v)", text, gotGroups[text], n, gotGroups)
				}
			}
		})
	}
}

// TestRewritePackagePlacesAGroupInPart checks the accounting of a constant
// group some of whose mutants are refused, through RewritePackage: each
// input mutant ends in exactly one of placed and dropped, the refused one
// dropped with its reason and the group placed with the rest.
func TestRewritePackagePlacesAGroupInPart(t *testing.T) {
	t.Parallel()
	dir := writeModule(t, map[string][]byte{"p.go": []byte("package p\n\nfunc F(n int) int { return n + 6/(1*0+1) }\n")})
	pkg := loadPkg(t, dir, ".")
	sites := pkgSites(pkg)
	_, placed, dropped := schemata.RewritePackage(context.Background(), pkg, sites, "")
	ends := map[int]int{}
	for _, s := range placed {
		for _, m := range s.Muts {
			ends[m.ID]++
		}
	}
	var refused []schemata.Site
	for _, d := range dropped {
		refused = append(refused, d.Site)
		for _, m := range d.Site.Muts {
			ends[m.ID]++
		}
		if !strings.Contains(d.Err.Error(), "divides by zero") {
			t.Errorf("dropped with %v", d.Err)
		}
	}
	total := 0
	for _, s := range sites {
		for _, m := range s.Muts {
			total++
			if ends[m.ID] != 1 {
				t.Errorf("mutant %d ends in %d of placed and dropped, want 1", m.ID, ends[m.ID])
			}
		}
	}
	if len(refused) != 1 || len(refused[0].Muts) != 1 || len(refused[0].Members) != 1 || refused[0].Members[0].Tok != token.MUL {
		t.Fatalf("dropped %+v, want the one * mutant", refused)
	}
	if len(placed) != 2 || total != 4 {
		t.Errorf("placed %d sites of %d mutants, want the + site and the group", len(placed), total)
	}
}

// TestRenderRefusesAPartialGroupThatMovesLines checks the newline check on a
// constant group some of whose mutants are refused: a float group whose
// witness spans lines would move every later line, so the whole group keeps
// its text and is refused once, with errNewlineChanged, every one of its
// mutants in that one error -- the mutant its fold refused is not reported
// a second time.
func TestRenderRefusesAPartialGroupThatMovesLines(t *testing.T) {
	t.Parallel()
	src := []byte("package p\n\ntype b struct{ r float64 }\n\nfunc f(v b) float64 {\n\treturn v.\n\t\tr * (1.5 + 1.0/(1*0+1))\n}\n")
	fset := token.NewFileSet()
	f, info := typeCheck(t, fset, "p.go", src)
	sites, plain := discover(f)
	grouped := schemata.GroupConstantSites(info, []*ast.File{f}, sites)
	var group schemata.Site
	for _, s := range grouped {
		if len(s.Members) > 0 {
			group = s
		}
	}
	if len(group.Members) != 4 {
		t.Fatalf("group has %d members, want the four sites of (1.5 + 1.0/(1*0+1))", len(group.Members))
	}
	out, errs := schemata.Render(fset, fset.File(f.Pos()), src, grouped,
		schemata.NewRewriter(info, nil, []*ast.File{f}, testPrefix, &schemata.HelperSet{}))
	if got, want := bytes.Count(out, []byte("\n")), bytes.Count(src, []byte("\n")); got != want {
		t.Errorf("output has %d lines, the source %d", got, want)
	}
	if !bytes.Contains(out, []byte("(1.5 + 1.0/(1*0+1))")) {
		t.Errorf("the group's text was not kept:\n%s", out)
	}
	seen := map[int]int{}
	for _, e := range errs {
		if !errors.Is(e.Err, schemata.ErrNewlineChanged) || e.Site.Node != group.Node {
			t.Errorf("SiteError %v at %s, want the group's newline refusal", e.Err, fset.Position(e.Site.Node.Pos()))
		}
		for _, m := range e.Site.Muts {
			seen[m.ID]++
		}
	}
	for _, m := range group.Muts {
		if seen[m.ID] != 1 {
			t.Errorf("mutant %d reported %d times, want once", m.ID, seen[m.ID])
		}
	}
	if len(group.Muts) >= len(plain) {
		t.Errorf("group holds %d of %d mutants: the outer * should stay its own site", len(group.Muts), len(plain))
	}
}
