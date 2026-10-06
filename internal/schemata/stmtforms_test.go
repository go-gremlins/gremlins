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
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

// assignMutators are the mutators with a statement form.
var assignMutators = []mutator.Type{
	mutator.InvertAssignments, mutator.InvertBitwiseAssignments, mutator.RemoveSelfAssignments,
}

// stmtMutators are the mutators TestStmtFormsBehave discovers: every one
// with a form, so that expression sites nest inside statement sites.
var stmtMutators = slices.Concat(formsMutators, assignMutators)

// plainSource is src with p's token rewritten as the engine rewrites it.
func plainSource(t *testing.T, src []byte, fset *token.FileSet, p plainMutant) []byte {
	t.Helper()
	to, ok := engine.TokenMutation(p.mt, p.tok)
	if !ok {
		t.Fatalf("engine has no %s mutation for %s", p.mt, p.tok)
	}
	off := fset.Position(p.pos).Offset

	return slices.Concat(src[:off:off], []byte(to.String()), src[off+len(p.tok.String()):])
}

// viable drops the mutants whose plain source does not type-check, as the
// engine's TypeViability does before generating them, and returns the
// dropped ones.
func viable(t *testing.T, src []byte, fset *token.FileSet, sites []schemata.Site, plain []plainMutant) ([]schemata.Site, []plainMutant, []plainMutant) {
	t.Helper()
	dropped := map[int]bool{}
	var kept, gone []plainMutant
	for _, p := range plain {
		mfset := token.NewFileSet()
		f, err := parser.ParseFile(mfset, "stmt.go", plainSource(t, src, fset, p), 0)
		if err == nil {
			conf := types.Config{Importer: importer.Default()}
			_, err = conf.Check("fixture", mfset, []*ast.File{f}, nil)
		}
		if err != nil {
			dropped[p.id] = true
			gone = append(gone, p)

			continue
		}
		kept = append(kept, p)
	}
	var out []schemata.Site
	for _, s := range sites {
		s.Muts = slices.DeleteFunc(slices.Clone(s.Muts), func(m schemata.Mutant) bool { return dropped[m.ID] })
		if len(s.Muts) > 0 {
			out = append(out, s)
		}
	}

	return out, kept, gone
}

// stmtFixture is a testdata module rendered into one schema binary, with the
// original and the sites' plain mutants to compare it against.
type stmtFixture struct {
	name      string
	src       []byte
	driver    []byte
	fset      *token.FileSet
	plain     []plainMutant
	gone      []plainMutant // mutants dropped as not viable
	schemaBin string
	origBin   string
	gomod     []byte
}

// renderFixture renders every site of the mutators mts in testdata/<name>
// -- name.go, driven by name_test.go -- and builds the schema and the
// original binary.
func renderFixture(t *testing.T, name string, mts []mutator.Type) *stmtFixture {
	t.Helper()
	dir := filepath.Join("testdata", name)
	src, err := os.ReadFile(filepath.Join(dir, name+".go")) //nolint:gosec // G304: the path is a testdata fixture named by this test
	if err != nil {
		t.Fatal(err)
	}
	driver, err := os.ReadFile(filepath.Join(dir, name+"_test.go")) //nolint:gosec // G304: the path is a testdata fixture named by this test
	if err != nil {
		t.Fatal(err)
	}
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod")) //nolint:gosec // G304: the path is a testdata fixture named by this test
	if err != nil {
		t.Fatal(err)
	}
	fx := &stmtFixture{name: name, src: src, driver: driver, fset: token.NewFileSet(), gomod: gomod}
	f, info := typeCheck(t, fx.fset, name+".go", src)
	df, err := parser.ParseFile(fx.fset, name+"_test.go", driver, 0)
	if err != nil {
		t.Fatal(err)
	}
	sites, plain := discoverFor(f, mts)
	sites, fx.plain, fx.gone = viable(t, src, fx.fset, sites, plain)
	sites = schemata.GroupConstantSites(info, []*ast.File{f}, sites)

	prefix := schemata.ChoosePrefix([]*ast.File{f, df})
	h := &schemata.HelperSet{}
	out, errs := schemata.Render(fx.fset, fx.fset.File(f.Pos()), src, sites, schemata.NewRewriter(info, nil, []*ast.File{f}, prefix, h))
	for _, e := range errs {
		t.Errorf("site at %s not rewritten: %v", fx.fset.Position(e.Site.Node.Pos()), e.Err)
	}
	if t.Failed() {
		t.FailNow()
	}
	if got, want := bytes.Count(beforeDuplicates(out), []byte("\n")), bytes.Count(src, []byte("\n")); got != want {
		t.Fatalf("rewrite has %d lines, original %d", got, want)
	}
	fx.schemaBin = buildFixture(t, map[string][]byte{
		"go.mod": gomod, name + ".go": out, name + "_test.go": driver, "gremlins_schemata.go": helperFile(t, h, name, prefix),
	})
	fx.origBin = buildFixture(t, map[string][]byte{"go.mod": gomod, name + ".go": src, name + "_test.go": driver})

	return fx
}

// runBehave is the subtest per run: with no mutant the schema must print
// what the original does, containing each line of mustPrint, and write no
// reach file; under each mutant id it must print what the plain mutant
// prints, and record the reach.
func (fx *stmtFixture) runBehave(t *testing.T, mustPrint []string) {
	t.Helper()
	t.Run("id0", func(t *testing.T) {
		t.Parallel()
		reach := filepath.Join(t.TempDir(), "reached")
		want := runFixture(t, fx.origBin)
		for _, line := range mustPrint {
			if !bytes.Contains(want, []byte(line)) {
				t.Errorf("original output lacks %q:\n%s", line, want)
			}
		}
		if got := runFixture(t, fx.schemaBin, "GREMLINS_REACHED="+reach); !bytes.Equal(got, want) {
			t.Errorf("schema without a mutant printed\n%s\nthe original printed\n%s", got, want)
		}
		if _, err := os.Stat(reach); err == nil {
			t.Error("reach file written with no mutant active")
		}
	})
	for _, p := range fx.plain {
		name := fmt.Sprintf("id%d_%s_%s_line%d", p.id, p.mt, p.tok, fx.fset.Position(p.pos).Line)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := map[string][]byte{
				"go.mod": fx.gomod, fx.name + ".go": plainSource(t, fx.src, fx.fset, p), fx.name + "_test.go": fx.driver,
			}
			want := runFixture(t, buildFixture(t, files))
			reach := filepath.Join(t.TempDir(), "reached")
			got := runFixture(t, fx.schemaBin, "GREMLINS_MUTANT="+strconv.Itoa(p.id), "GREMLINS_REACHED="+reach)
			if !bytes.Equal(got, want) {
				t.Errorf("schema with mutant %d printed\n%s\nthe plain mutant printed\n%s", p.id, got, want)
			}
			if _, err := os.Stat(reach); err != nil {
				t.Errorf("reach file missing: %v", err)
			}
		})
	}
}

// TestStmtFormsBehave is TestFormsBehave for the statement forms: every
// site of testdata/stmt, of every mutator with a form, rendered into one
// schema binary that must print, under each mutant id, exactly what the plain
// token mutant prints, and record that the site was reached.
func TestStmtFormsBehave(t *testing.T) {
	t.Parallel()
	fx := renderFixture(t, "stmt", stmtMutators)
	covered := map[mutator.Type]bool{}
	for _, p := range fx.plain {
		covered[p.mt] = true
	}
	for _, mt := range assignMutators {
		if !covered[mt] {
			t.Errorf("fixture has no %s mutant", mt)
		}
	}
	// The fixture must hold the sites a mutant is missing from: s -= t on a
	// string, and x = n for a count of another type than the value.
	var goneAssign []string
	for _, p := range fx.gone {
		if slices.Contains(assignMutators, p.mt) {
			goneAssign = append(goneAssign, fmt.Sprintf("%s %s line %d", p.mt, p.tok, fx.fset.Position(p.pos).Line))
		}
	}
	if len(goneAssign) == 0 {
		t.Error("no assignment mutant was dropped as not viable")
	}
	t.Logf("not viable: %v", goneAssign)

	// The fixture must hold the helper form of a shift whose count is a
	// constant, with its x = count mutant: SimpleShiftConst's nine sites.
	start := bytes.Index(fx.src, []byte("\nfunc SimpleShiftConst("))
	end := start + bytes.Index(fx.src[start:], []byte("\n}\n"))
	if start < 0 || end < start {
		t.Fatal("fixture has no SimpleShiftConst")
	}
	constShifts := 0
	for _, p := range fx.plain {
		off := fx.fset.Position(p.pos).Offset
		if p.mt == mutator.RemoveSelfAssignments && off > start && off < end {
			constShifts++
		}
	}
	if constShifts != 9 {
		t.Errorf("SimpleShiftConst has %d viable REMOVE_SELF_ASSIGNMENTS mutants, want 9", constShifts)
	}

	// The fixture must exercise what it is there for: each side effect
	// once, the label taken, the closures per iteration.
	fx.runBehave(t, []string{
		"AddAssignSideEffectLHS: [[1 4 3] 1]\n", "AddAssignSideEffectRHS: [6 1]\n",
		"MapAssignNamedMap: [map[a:9 b:9] 2]\n", "LabelledAssign: [4 13]\n", "ForPostAddAssign: [0 3 6 9]\n",
		"SimpleShiftConst: [[48 6 96 6] map[0:48 1:1 2:24] 128 33]\n",
	})
}

// TestLoopCtrlFormsBehave is TestStmtFormsBehave for the break and continue
// form, over testdata/loopctrl.
func TestLoopCtrlFormsBehave(t *testing.T) {
	t.Parallel()
	fx := renderFixture(t, "loopctrl", []mutator.Type{mutator.InvertLoopCtrl})
	var breaks, continues, gone int
	for _, p := range fx.plain {
		if p.tok == token.BREAK {
			breaks++
		} else {
			continues++
		}
	}
	gone = len(fx.gone)
	// Five breaks and three continues compile as their opposite, and the two
	// breaks outside a loop do not.
	if breaks != 5 || continues != 3 || gone != 2 {
		t.Errorf("fixture has %d break, %d continue mutants, %d not viable; want 5, 3 and 2", breaks, continues, gone)
	}
	fx.runBehave(t, []string{
		"BreakInFor: [0 1 2]\n", "ContinueInFor: [0 2 4]\n", "BreakInSwitchInLoop: [0 0 10 2 20 3 30]\n",
		"BreakInSelectInLoop: [0 0 10 2 20]\n", "LabelledBreak: [0]\n", "LabelledContinueOuter: [0 10 20]\n",
		"RangeOverFuncBreak: [0 1 2]\n", "RangeOverFuncContinue: [1 3]\n",
	})
}

// TestAssignMutationsMatchEngine holds the statement forms' table to the
// engine's: for every assignment mutator and every token, both rewrite it to
// the same token, or neither applies.
func TestAssignMutationsMatchEngine(t *testing.T) {
	t.Parallel()
	for _, mt := range assignMutators {
		for tok := token.ILLEGAL; tok <= token.TILDE; tok++ {
			want, wok := engine.TokenMutation(mt, tok)
			got, gok := schemata.AssignMutation(mt, tok)
			if got != want || gok != wok {
				t.Errorf("%s %s: statement forms rewrite to %s (%v), engine to %s (%v)", mt, tok, got, gok, want, wok)
			}
		}
	}
}

// TestNewRewriterAssign is TestNewRewriter for the op= statement forms.
func TestNewRewriterAssign(t *testing.T) {
	t.Parallel()
	ia := mutator.InvertAssignments
	ib := mutator.InvertBitwiseAssignments
	rs := mutator.RemoveSelfAssignments
	cases := map[string]rewriterCase{
		"add_block":         {src: "func f(x, y int) { x += y }", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia, rs}, want: "switch _zzActive { case 1: _zzReached(); x -= y; case 2: _zzReached(); x = y; default: x += y }"},
		"rem_block_remove":  {src: "func f(x, y int) { x %= y }", tok: token.REM_ASSIGN, muts: []mutator.Type{rs}, want: "switch _zzActive { case 1: _zzReached(); x = y; default: x %= y }"},
		"and_block":         {src: "func f(x, y int) { x &= /* c */ y }", tok: token.AND_ASSIGN, muts: []mutator.Type{ib, rs}, want: "switch _zzActive { case 2: _zzReached(); x = /* c */ y; case 1: _zzReached(); x |= /* c */ y; default: x &= /* c */ y }"},
		"andnot_block":      {src: "func f(x, y int) { x &^= y }", tok: token.AND_NOT_ASSIGN, muts: []mutator.Type{ib}, want: "switch _zzActive { case 1: _zzReached(); x &= y; default: x &^= y }"},
		"string_block":      {src: "func f(s, t string) { s += t }", tok: token.ADD_ASSIGN, muts: []mutator.Type{rs}, want: "switch _zzActive { case 1: _zzReached(); s = t; default: s += t }"},
		"generic_block":     {src: "func f[T ~int | ~string](x, y T) { x += y }", tok: token.ADD_ASSIGN, muts: []mutator.Type{rs}, want: "switch _zzActive { case 1: _zzReached(); x = y; default: x += y }"},
		"case_body":         {src: "func f(x int) { switch { case x > 0: x += 1 } }", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia}, want: "switch _zzActive { case 1: _zzReached(); x -= 1; default: x += 1 }"},
		"comm_body":         {src: "func f(x int) { select { default: x *= 2 } }", tok: token.MUL_ASSIGN, muts: []mutator.Type{ia}, want: "switch _zzActive { case 1: _zzReached(); x /= 2; default: x *= 2 }"},
		"labelled":          {src: "func f(x int) int { goto L\nL:\n\tx -= 1\n\treturn x }", tok: token.SUB_ASSIGN, muts: []mutator.Type{ia}, want: "switch _zzActive { case 1: _zzReached(); x += 1; default: x -= 1 }"},
		"block_multiline":   {src: "func f(x, y int) {\n\tx +=\n\t\ty\n}", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia, rs}, want: "_zzADDAssign(1, 2, &x,\n\t\ty)"},
		"for_post":          {src: "func f() { for i := 0; i < 3; i += 2 {} }", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia, rs}, want: "_zzADDAssign(1, 2, &i, 2)"},
		"for_init":          {src: "func f(x float64) { for x /= 2; ; { break } }", tok: token.QUO_ASSIGN, muts: []mutator.Type{ia}, want: "_zzQUOAssign(1, 0, &x, 2)"},
		"if_init":           {src: "func f(x int) bool { if x %= 3; x > 0 { return true }; return false }", tok: token.REM_ASSIGN, muts: []mutator.Type{rs, ia}, want: "_zzREMAssign(2, 1, &x, 3)"},
		"switch_init_map":   {src: "func f(m map[string]uint, k string) { switch m[k] ^= 1; {} }", tok: token.XOR_ASSIGN, muts: []mutator.Type{rs, ib}, want: "_zzXORAssignMap(1, 2, m, k, 1)"},
		"type_switch_init":  {src: "func f(x int, v any) { switch x |= 1; v.(type) {} }", tok: token.OR_ASSIGN, muts: []mutator.Type{ib}, want: "_zzORAssign(0, 1, &x, 1)"},
		"simple_string":     {src: "func f(s string) { if s += \"x\"; s != \"\" {} }", tok: token.ADD_ASSIGN, muts: []mutator.Type{rs}, want: "_zzADDAssignStr(1, &s, \"x\")"},
		"simple_string_map": {src: "type ns map[int]string\nfunc f(m ns) { if m[0] += \"x\"; true {} }", tok: token.ADD_ASSIGN, muts: []mutator.Type{rs}, want: "_zzADDAssignStrMap(1, m, 0, \"x\")"},
		"simple_shl_same":   {src: "func f(x, n uint) { for ; x < 9; x <<= n {} }", tok: token.SHL_ASSIGN, muts: []mutator.Type{rs, ib}, want: "_zzSHLAssign(1, 2, &x, n)"},
		"simple_shl_const":  {src: "func f(x int8) { for ; x < 9; x <<= 1 {} }", tok: token.SHL_ASSIGN, muts: []mutator.Type{rs, ib}, want: "_zzSHLAssign(1, 2, &x, 1)"},
		"simple_shr_mixed":  {src: "func f(x uint64, n uint8) { for ; x > 0; x >>= n {} }", tok: token.SHR_ASSIGN, muts: []mutator.Type{ib}, want: "_zzSHRAssignX(1, &x, n)"},
		"simple_shl_map":    {src: "func f(m map[int]int, n uint8) { for ; m[0] < 9; m[0] <<= n {} }", tok: token.SHL_ASSIGN, muts: []mutator.Type{ib}, want: "_zzSHLAssignXMap(1, m, 0, n)"},
		"simple_named":      {src: "type c float32\nfunc f(x c) { if x *= 2; x > 0 {} }", tok: token.MUL_ASSIGN, muts: []mutator.Type{ia, rs}, want: "_zzMULAssign(1, 2, &x, 2)"},
		"nested_lhs":        {src: "func f(a []int, i int) { for ; ; a[i+1] -= 1 {} }", tok: token.SUB_ASSIGN, muts: []mutator.Type{ia}, want: "_zzSUBAssign(1, 0, &a[i+1], 1)"},
		// Refused: a mutant whose arm would not compile, a foreign mutator,
		// and an operand neither form can reach.
		"string_invert":       {src: "func f(s, t string) { s += t }", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia, rs}, refusal: "INVERT_ASSIGNMENTS"},
		"generic_invert":      {src: "func f[T ~int | ~string](x, y T) { x += y }", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia}, refusal: "INVERT_ASSIGNMENTS"},
		"shl_mixed_remove":    {src: "func f(x uint64, n uint8) { x <<= n }", tok: token.SHL_ASSIGN, muts: []mutator.Type{rs}, refusal: "REMOVE_SELF_ASSIGNMENTS"},
		"shl_float_count":     {src: "func f(x int) { for ; ; x <<= 2.0 {} }", tok: token.SHL_ASSIGN, muts: []mutator.Type{ib}},
		"wrong_mutator":       {src: "func f(x, y int) { x %= y }", tok: token.REM_ASSIGN, muts: []mutator.Type{ib}},
		"generic_map_simple":  {src: "func f[M ~map[string]int](m M) { for ; ; m[\"a\"] += 1 {} }", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia}, refusal: "op-assign in a simple statement, neither addressable nor a map entry"},
		"generic_map_multiln": {src: "func f[M ~map[string]int](m M) {\n\tm[\"a\"] +=\n\t\t1\n}", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia}, refusal: "neither addressable nor a map entry"},
		"simple_generic_str":  {src: "func f[T ~int | ~string](x, y T) { for ; ; x += y {} }", tok: token.ADD_ASSIGN, muts: []mutator.Type{rs}},
		"no_files":            {src: "func f(x, y int) { x += y }", tok: token.ADD_ASSIGN, muts: []mutator.Type{ia}, noFiles: true},
	}
	runRewriterCases(t, cases)
}

// TestNewRewriterBranch is TestNewRewriter for the break and continue form.
func TestNewRewriterBranch(t *testing.T) {
	t.Parallel()
	lc := mutator.InvertLoopCtrl
	cases := map[string]rewriterCase{
		"break_in_for":      {src: "func f() { for { break } }", tok: token.BREAK, muts: []mutator.Type{lc}, want: "if _zzXor(1, false) { continue } else { break }"},
		"continue_in_for":   {src: "func f(x int) { for x < 3 { continue } }", tok: token.CONTINUE, muts: []mutator.Type{lc}, want: "if _zzXor(1, false) { break } else { continue }"},
		"break_in_range":    {src: "func f(s []int) { for range s { break } }", tok: token.BREAK, muts: []mutator.Type{lc}, want: "if _zzXor(1, false) { continue } else { break }"},
		"break_switch_loop": {src: "func f(x int) { for { switch x { case 1: break } } }", tok: token.BREAK, muts: []mutator.Type{lc}, want: "if _zzXor(1, false) { continue } else { break }"},
		"break_select_loop": {src: "func f() { for { select { default: break } } }", tok: token.BREAK, muts: []mutator.Type{lc}, want: "if _zzXor(1, false) { continue } else { break }"},
		"label_break_loop":  {src: "func f() {\nL:\n\tfor { break L } }", tok: token.BREAK, muts: []mutator.Type{lc}, want: "if _zzXor(1, false) { continue L } else { break L }"},
		"label_continue":    {src: "func f() {\nL:\n\tfor { for { continue L } } }", tok: token.CONTINUE, muts: []mutator.Type{lc}, want: "if _zzXor(1, false) { break L } else { continue L }"},
		// Refused: a break whose continue would not compile, a goto, a foreign
		// mutator, and a site token that is not the node's.
		"break_outside_loop":    {src: "func f(x int) { switch x { case 1: break } }", tok: token.BREAK, muts: []mutator.Type{lc}, refusal: "no loop continues"},
		"break_loop_in_funclit": {src: "func f() { for { _ = func() { switch { default: break } } } }", tok: token.BREAK, muts: []mutator.Type{lc}, refusal: "no loop continues"},
		"label_break_switch":    {src: "func f(x int) {\nL:\n\tswitch x { case 1: break L } }", tok: token.BREAK, muts: []mutator.Type{lc}, refusal: "no loop continues"},
		"label_break_other":     {src: "func f() {\nL:\n\tfor { for { break L } }\n}", tok: token.BREAK, muts: []mutator.Type{lc}, want: "if _zzXor(1, false) { continue L } else { break L }"},
		"goto":                  {src: "func f() {\nL:\n\tgoto L }", tok: token.GOTO, muts: []mutator.Type{lc}, refusal: "branch statement goto"},
		"wrong_mutator":         {src: "func f() { for { break } }", tok: token.BREAK, muts: []mutator.Type{mutator.InvertLogical}},
		"wrong_site_token":      {src: "func f() { for { break } }", tok: token.BREAK, siteTok: token.CONTINUE, muts: []mutator.Type{lc}},
		"no_files":              {src: "func f() { for { break } }", tok: token.BREAK, muts: []mutator.Type{lc}, noFiles: true},
	}
	runRewriterCases(t, cases)
}
