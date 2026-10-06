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

package coverage

import (
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"
)

// A package's source is read as a set of entities: every top-level
// declaration, each printed on its own, plus the remainder whose change no
// entity can carry, which is hashed whole (pkgPrint.Whole). See reusable for
// how a change to each is attributed, and why that is sound.
//
// An entity's print is the declaration's tokens, each with its line relative
// to the declaration and its column, with prose comments dropped and
// directives kept. Prose decides nothing; a //go:embed, //go:noinline or
// //line comment does, and a //go:embed pattern changing is a change to the
// variable under it although none of its tokens moved. The positions are in
// the print because a kept mapping's blocks are moved by the distance their
// declaration moved, which is only right while everything inside it sits
// where it did: a comment line added inside a body moves the blocks below it,
// and so counts as a change.

// pkgPrint is one package directory as narrowing compares it.
type pkgPrint struct {
	// Whole is one hash over everything whose change is attributable to no
	// entity, and so re-maps the whole package: non-Go files and the data
	// below the directory, files that do not parse, the package name, build
	// constraints and the file names that carry one, directives outside any
	// declaration, blank and dot imports, and every cgo file in full.
	Whole string `json:"whole"`

	// Decls is every entity by key: "<profile file>:<name>" for a function,
	// "<profile file>:<receiver>.<name>" for a method, and the kind before
	// the names for the rest (see entityKey).
	Decls map[string]declPrint `json:"decls"`

	// Imports is, per profile file name, each import's local name and the
	// path it is bound to. A name bound to another path changes what every
	// mention of it in the file means without any of their text changing.
	// An unnamed import is under the imported package's own name when the
	// build listing said what it is, and under "?" and its path when it did
	// not, which makes any change to the file's imports reach the whole file.
	Imports map[string]map[string]string `json:"imports,omitempty"`

	// InitOrder is the order the package's variables are initialised in, as
	// the type-checker derives it (see typeFacts), and Typed says it was
	// derived at all. A print the type-checker could not read is never
	// narrowed from: the order is a fact no token of the source holds.
	InitOrder []string `json:"init_order,omitempty"`
	Typed     bool     `json:"typed,omitempty"`

	// Reach is a hash of every package the package imports, directly or not,
	// and then of every package its external tests do, as go/packages lists
	// them alongside the type-check: where the package is initialised among
	// the rest of the binary (see importClosure and closure.reordered).
	Reach string `json:"reach,omitempty"`
}

// The kinds of entity. A function is the empty kind.
const (
	// kindMethod is a method. Its body reaches only its own lines, but a
	// changed signature, or one added or removed, changes which interfaces
	// its receiver satisfies.
	kindMethod = "method"
	// kindInit is an init function. It runs before every test in the binary.
	kindInit = "init"
	// kindRun is a var whose initialiser calls something at
	// initialisation: it runs for every test without any test naming it.
	kindRun = "run"
	// kindMain is TestMain. It runs for every test too, but after every
	// package is initialised, so unlike an initialiser it does not observe
	// the order they were initialised in (see closure.observer).
	kindMain = "main"
	// kindBlank is a declaration with no name to pair it by between two
	// prints: a var or a function named _, or a key two declarations share.
	kindBlank = "blank"
	kindType  = "type"
	kindVar   = "var"
	kindConst = "const"
)

// declPrint is one entity: what it said, what it declares, and where it was.
type declPrint struct {
	// Hash is the entity's whole print.
	Hash string `json:"hash"`
	// Sig is a function's print without its body: directives, receiver, name,
	// type parameters and signature. A body change reaches only the tests that
	// executed the body; a signature change can rebind a call site that did
	// not change — a handler passed as a value and reflected over — so it is
	// a changed name instead. Empty for every other kind.
	Sig string `json:"sig,omitempty"`
	// Names are the names the entity declares, and so the names a change to
	// it changes. A method's is its receiver type's: what a method added,
	// removed or re-signed changes is that type's method set, and every value
	// of the type is made by code that names it (see reusable). Its own name
	// would match every call of any method so spelled — a test type's Fatalf
	// every t.Fatalf in the package. Only a receiver no name can be read off
	// falls back to the method's own name.
	Names []string `json:"names,omitempty"`
	Kind  string   `json:"kind,omitempty"`
	// Linked says a //go:linkname directive binds the declaration to a symbol
	// of another package, which is a change no name in this one carries.
	Linked bool `json:"linked,omitempty"`
	// Observes says a var's initialiser reads another package — a variable
	// of it, by any spelling, or a variable of this package that does, as
	// the type-checker resolves them (see printSet.observes) — which it does
	// at initialisation: what it reads then depends on which packages were
	// initialised before this one, and that order moves when the set of
	// packages one of the binary's instrumented packages reaches changes
	// (see closure.reordered). A call
	// that runs at initialisation is kindRun instead, which is the same thing
	// and more. It is the order across packages, which no package's
	// type-check sees; the order within one is pkgPrint.InitOrder.
	Observes bool `json:"observes,omitempty"`
	// File is the name the coverage profile uses, not the name on disk, so
	// that a span can be compared against a profile without translating
	// either. Empty for a dependency's entities, whose mappings are keys.
	File string `json:"file,omitempty"`
	// Test is the name `go test` runs this entity under, when it is a test.
	// Coverage does not instrument test files, so a test's own lines appear in
	// no profile and a change reaching one is attributed by name instead.
	Test string `json:"test,omitempty"`

	// Start and End are the lines the entity occupied, counted from its first
	// token rather than its doc comment, in that run's coordinates: the
	// profiles they are compared against are too.
	Start int `json:"start,omitempty"`
	End   int `json:"end,omitempty"`

	// text and header are the identifiers the entity mentions: in all of it,
	// and in the part a name's change reaches through without executing it.
	// They are read only from the print being compared against, never the
	// cached one — an entity whose print agrees mentions what it did — so
	// they are not stored.
	text   map[string]bool
	header map[string]bool
}

// testFuncPrefixes are the declarations `go test` runs on their own. They
// match listPattern, which decides what goes into the map in the first place.
var testFuncPrefixes = []string{"Test", "Fuzz", "Example"}

// allowedInitCalls are calls a var initialiser may make and still be a plain
// named entity: they build their value and do nothing else, so a variable
// they initialise is reached only by the code that names it. They are matched
// by the import path the file binds the selector's package name to.
//
// fmt.Errorf is allowed only on terms (see formatsNothing): it formats its
// operands, and formatting a value calls its String or Error method, which is
// the package's code running at initialisation.
var allowedInitCalls = map[string]string{
	"errors": "New",
	"fmt":    "Errorf",
	"regexp": "MustCompile",
}

// pureBuiltins are the predeclared functions that do nothing at
// initialisation but compute their value. A predeclared type name used as a
// conversion is the same.
var pureBuiltins = map[string]bool{
	"make": true, "new": true, "len": true, "cap": true,
	"complex": true, "real": true, "imag": true, "min": true, "max": true,
}

// platformSuffixes are the GOOS and GOARCH values a file name's suffix can
// constrain a build to, as go/build reads them.
var platformSuffixes = map[string]bool{}

func init() {
	for _, name := range strings.Fields(
		"aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos " +
			"386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le " +
			"ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm") {
		platformSuffixes[name] = true
	}
}

// constrainedName reports whether a file name decides when the file is
// compiled: one whose name ends in a GOOS or GOARCH, before any _test. Adding,
// removing or renaming one changes what a build compiles, so the names are
// part of Whole.
//
// A test file's name is not a constraint of that kind. It decides only that
// the file is compiled into the test binary alone, and everything that follows
// from that is something an entity rule already carries: its declarations are
// keyed by its file, so a file added, removed or renamed is its entities
// added and removed; a test file's entities are followed by name, bodies
// included; and a package it newly links is in Inputs.
func constrainedName(name string) bool {
	base := strings.TrimSuffix(strings.TrimSuffix(name, ".go"), "_test")
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return false
	}

	return platformSuffixes[parts[len(parts)-1]]
}

// printSet builds one package directory's print.
type printSet struct {
	c          *Coverage
	importPath string
	out        pkgPrint
	whole      []string
	// declared is every name the package declares at the top level, which is
	// what says whether `len(x)` is the builtin or a call into the package.
	declared map[string]bool
	// stdErrors is, per package name, every var declared once and initialised
	// by errors.New of a literal: an error whose Error method is the standard
	// library's, and so one fmt.Errorf can format without running anything of
	// the package.
	stdErrors map[string]map[string]bool
	// facts is what the type-checker says about the package, or nil when it
	// could not say anything.
	facts *typeFacts
}

// printPackage prints a package directory's files. withTests says whether its
// test files are part of what is compared: they are for the package being
// mapped, and never for a dependency, whose test files are not linked into
// any other package's test binary.
//
// below is the hash of the data below the directory (see hashDataSubtrees),
// which is part of Whole. facts is the package's type facts, nil when it
// could not be type-checked; a print without them is never narrowed from.
func (c *Coverage) printPackage(importPath string, files []goFile, withTests bool, below string, facts *typeFacts) pkgPrint {
	ps := &printSet{
		c:          c,
		importPath: importPath,
		out:        pkgPrint{Decls: map[string]declPrint{}, Imports: map[string]map[string]string{}},
		declared:   map[string]bool{},
		stdErrors:  map[string]map[string]bool{},
		facts:      facts,
	}
	if facts != nil {
		ps.out.InitOrder, ps.out.Reach, ps.out.Typed = facts.initOrder, facts.reach, true
	}
	var kept []*goFile
	packageNames := map[string]bool{}
	for i := range files {
		f := &files[i]
		isGo := strings.HasSuffix(f.name, ".go")
		if isGo && !withTests && strings.HasSuffix(f.name, "_test.go") {
			continue
		}
		switch {
		case !isGo:
			ps.whole = append(ps.whole, "file\x00"+f.name+"\x00"+hashOf(f.data))
		case f.ast == nil:
			ps.whole = append(ps.whole, "unparsed\x00"+f.name+"\x00"+hashOf(f.data))
		default:
			if constrainedName(f.name) {
				ps.whole = append(ps.whole, "constrained\x00"+f.name)
			}
			packageNames[f.ast.Name.Name] = true
			collectDeclared(f.ast, ps.declared)
			kept = append(kept, f)
		}
	}
	ps.collectStdErrors(kept)
	for _, f := range kept {
		ps.printFile(f)
	}
	names := make([]string, 0, len(packageNames))
	for name := range packageNames {
		names = append(names, name)
	}
	sort.Strings(names)
	ps.whole = append(ps.whole, "package\x00"+strings.Join(names, ","), "below\x00"+below)
	ps.out.Whole = hashOf([]byte(strings.Join(ps.whole, "\x00")))

	return ps.out
}

// collectDeclared records the names a file declares at the top level.
func collectDeclared(f *ast.File, into map[string]bool) {
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				into[d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					into[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, n := range s.Names {
						into[n.Name] = true
					}
				}
			}
		}
	}
}

// collectStdErrors finds, per package, the vars initialised as
// `errors.New("literal")` and declared nowhere else in it.
func (ps *printSet) collectStdErrors(files []*goFile) {
	counts := map[string]map[string]int{}
	for _, f := range files {
		pkg := f.ast.Name.Name
		if counts[pkg] == nil {
			counts[pkg], ps.stdErrors[pkg] = map[string]int{}, map[string]bool{}
		}
		names := map[string]bool{}
		collectDeclared(f.ast, names)
		for name := range names {
			counts[pkg][name]++
		}
		imports := ps.importTable(f.ast)
		for _, decl := range f.ast.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, isValue := spec.(*ast.ValueSpec)
				if isValue && len(vs.Names) == 1 && len(vs.Values) == 1 && isStdError(imports, vs.Values[0]) {
					ps.stdErrors[pkg][vs.Names[0].Name] = true
				}
			}
		}
	}
	for pkg, names := range ps.stdErrors {
		for name := range names {
			if counts[pkg][name] != 1 {
				delete(names, name)
			}
		}
	}
}

// isStdError reports whether an expression is errors.New of a literal.
func isStdError(imports map[string]string, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	_, literal := call.Args[0].(*ast.BasicLit)

	return ok && literal && sel.Sel.Name == "New" && imports[pkg.Name] == "errors"
}

// fileTokens is a file's tokens, scanned once.
type fileTokens struct {
	toks []scannedToken
}

type scannedToken struct {
	pos  token.Pos
	line int
	col  int
	tok  token.Token
	lit  string
}

func scanFile(f *goFile) fileTokens {
	tf := f.fset.File(f.ast.Pos())
	var s scanner.Scanner
	s.Init(tf, f.data, nil, scanner.ScanComments)
	var out fileTokens
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		p := tf.Position(pos)
		out.toks = append(out.toks, scannedToken{pos: pos, line: p.Line, col: p.Column, tok: tok, lit: lit})
	}

	return out
}

// isDirective reports whether a comment changes what the code does: a //go:
// directive, or a //line directive, which changes the positions a profile
// reports.
func isDirective(text string) bool {
	return strings.HasPrefix(text, "//go:") || strings.HasPrefix(text, "//line ") || strings.HasPrefix(text, "/*line ")
}

// printRange prints the tokens in [from, to), positioned relative to base. Prose
// comments are dropped unless allComments is set — an Example's // Output:
// comment is what it is checked against. An inserted semicolon is printed
// without its position, which is wherever the line happened to end.
func (ft fileTokens) printRange(from, to token.Pos, base int, allComments bool) string {
	i := sort.Search(len(ft.toks), func(i int) bool { return ft.toks[i].pos >= from })
	var b strings.Builder
	for ; i < len(ft.toks) && ft.toks[i].pos < to; i++ {
		t := ft.toks[i]
		switch {
		case t.tok == token.COMMENT && !allComments && !isDirective(t.lit):
			continue
		case t.tok == token.SEMICOLON && t.lit == "\n":
			b.WriteString(";\n")

			continue
		}
		fmt.Fprintf(&b, "%d:%d:%s:%s\n", t.line-base, t.col, t.tok, t.lit)
	}

	return b.String()
}

// directives are the directive comments in [from, to).
func (ft fileTokens) directives(from, to token.Pos) []string {
	var out []string
	i := sort.Search(len(ft.toks), func(i int) bool { return ft.toks[i].pos >= from })
	for ; i < len(ft.toks) && ft.toks[i].pos < to; i++ {
		if ft.toks[i].tok == token.COMMENT && isDirective(ft.toks[i].lit) {
			out = append(out, ft.toks[i].lit)
		}
	}

	return out
}

// fileState is what printing one file needs to carry from declaration to
// declaration.
type fileState struct {
	f        *goFile
	toks     fileTokens
	profile  string
	isTest   bool
	imports  map[string]string
	ordinals map[string]int
	// covered is every range an entity's print holds, so that the directives
	// outside all of them can go into Whole.
	covered [][2]token.Pos
}

func (ps *printSet) printFile(f *goFile) {
	st := &fileState{
		f:        f,
		toks:     scanFile(f),
		profile:  ps.c.profileFileName(ps.importPath, f.name),
		isTest:   strings.HasSuffix(f.name, "_test.go"),
		imports:  map[string]string{},
		ordinals: map[string]int{},
	}
	if ps.printImports(st) {
		// cgo's preamble is a comment, and the C it holds is compiled into
		// the package; the generated code is not something this can read.
		ps.whole = append(ps.whole, "cgo\x00"+f.name+"\x00"+hashOf(f.data))
	}
	if len(st.imports) > 0 {
		ps.out.Imports[st.profile] = st.imports
	}
	for _, decl := range f.ast.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			ps.printFunc(st, d)
		case *ast.GenDecl:
			ps.printGen(st, d)
		}
	}
	// A directive no declaration holds — a //go:build line, a //go:debug —
	// is about the file, and nothing narrower can carry its change.
	for _, t := range st.toks.toks {
		directive := isDirective(t.lit) || strings.HasPrefix(t.lit, "// +build")
		if t.tok != token.COMMENT || !directive || st.holds(t.pos) {
			continue
		}
		ps.whole = append(ps.whole, "directive\x00"+f.name+"\x00"+t.lit)
	}
}

func (st *fileState) holds(pos token.Pos) bool {
	for _, r := range st.covered {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}

	return false
}

// printImports records a file's import table, reporting whether the file
// uses cgo. A blank or dot import is part of Whole: one runs a package's
// initialisation for its side effects, the other brings every exported name
// of a package into the file unqualified, and neither has a name to follow.
func (ps *printSet) printImports(st *fileState) bool {
	cgo := false
	for _, imp := range st.f.ast.Imports {
		// The parser only accepts a string literal here, which unquotes.
		path, _ := strconv.Unquote(imp.Path.Value)
		switch {
		case imp.Name != nil && (imp.Name.Name == "_" || imp.Name.Name == "."):
			ps.whole = append(ps.whole, "import\x00"+st.f.name+"\x00"+imp.Name.Name+"\x00"+path)
		case path == "C" && imp.Name == nil:
			cgo = true
		}
	}
	st.imports = ps.importTable(st.f.ast)

	return cgo
}

// importTable is the names a file's imports bind, each to its path: under its
// own name when it has one, under the imported package's name when the build
// listing said what it is, and under "?" and its path when it did not. Blank,
// dot and cgo imports bind no name to follow.
func (ps *printSet) importTable(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		switch {
		case imp.Name != nil && (imp.Name.Name == "_" || imp.Name.Name == "."):
		case imp.Name != nil:
			out[imp.Name.Name] = path
		case path == "C":
		default:
			if name, ok := ps.c.pkgNames[path]; ok {
				out[name] = path
			} else {
				out["?"+path] = path
			}
		}
	}

	return out
}

func (ps *printSet) printFunc(st *fileState, fn *ast.FuncDecl) {
	from := fn.Pos()
	if fn.Doc != nil {
		from = fn.Doc.Pos()
	}
	st.covered = append(st.covered, [2]token.Pos{from, fn.End()})
	base := st.f.fset.Position(fn.Pos()).Line
	bodyAt := fn.End()
	if fn.Body != nil {
		bodyAt = fn.Body.Lbrace
	}
	allComments := st.isTest && strings.HasPrefix(fn.Name.Name, "Example")
	sig := st.toks.printRange(from, bodyAt, base, allComments)
	body := st.toks.printRange(bodyAt, fn.End(), base, allComments)

	d := declPrint{
		Hash:   hashOf([]byte(sig + "\x00" + body)),
		Sig:    hashOf([]byte(sig)),
		Linked: linked(st.toks.directives(from, bodyAt)),
		Start:  base,
		End:    st.f.fset.Position(fn.End()).Line,
		text:   identsOf(fn),
		header: identsOf(fn.Recv, fn.Name, fn.Type),
	}
	name := fn.Name.Name
	switch {
	case fn.Recv != nil:
		d.Kind = kindMethod
		d.Names = []string{name}
		if recv := receiverName(fn); recv != "" {
			d.Names = []string{recv}
		}
	case name == "init":
		d.Kind = kindInit
	case name == "_":
		d.Kind = kindBlank
	case st.isTest && name == "TestMain":
		d.Kind = kindMain
	default:
		d.Names = []string{name}
		if st.isTest && isTestFuncName(name) {
			d.Test = name
		}
	}
	ps.add(st, declKey(fn), d)
}

// printGen prints a type, var or const declaration. A parenthesised const
// group is one entity: under iota, inserting a name before B changes B's
// value without changing B's text, and so does changing the expression B
// repeats implicitly. A type or var group is a list of independent specs.
func (ps *printSet) printGen(st *fileState, gd *ast.GenDecl) {
	if gd.Tok == token.IMPORT {
		return
	}
	if gd.Tok == token.CONST && gd.Lparen.IsValid() {
		var names []string
		for _, spec := range gd.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				for _, n := range vs.Names {
					names = append(names, n.Name)
				}
			}
		}
		ps.printSpec(st, gd.Pos(), gd.End(), docStart(gd.Doc, gd.Pos()), kindConst, names, identsOf(gd))

		return
	}
	for _, spec := range gd.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			from, to, doc := specRange(gd, s, s.Doc)
			key := ps.printSpec(st, from, to, doc, kindType, []string{s.Name.Name}, identsOf(s))
			// An alias and the type it names are one type, with one method
			// set: a method declared through either is a method of both, and
			// code that names only the other reaches it. So an alias's change
			// is a change to the type it stands for too. Only a type of this
			// package can gain a method through an alias, so only its name is
			// added; the other direction needs nothing of its own, here or
			// across packages, because the alias's header names the type.
			// `type A = (box)` is the same alias, so the type is unwrapped
			// first.
			if base, local := ast.Unparen(s.Type).(*ast.Ident); s.Assign.IsValid() && local && ps.declared[base.Name] {
				d := ps.out.Decls[key]
				d.Names = append(d.Names, base.Name)
				ps.out.Decls[key] = d
			}
		case *ast.ValueSpec:
			from, to, doc := specRange(gd, s, s.Doc)
			kind := kindConst
			if gd.Tok == token.VAR {
				kind = kindVar
				if ps.runsAtInit(st, s) {
					kind = kindRun
				}
			}
			var names []string
			for _, n := range s.Names {
				if n.Name != "_" {
					names = append(names, n.Name)
				}
			}
			if len(names) == 0 && kind != kindRun {
				kind = kindBlank
			}
			key := ps.printSpec(st, from, to, doc, kind, names, identsOf(s))
			if kind == kindVar && ps.observes(st, s) {
				d := ps.out.Decls[key]
				d.Observes = true
				ps.out.Decls[key] = d
			}
		}
	}
}

func (ps *printSet) printSpec(st *fileState, from, to, doc token.Pos, kind string,
	names []string, idents map[string]bool,
) string {
	st.covered = append(st.covered, [2]token.Pos{doc, to})
	base := st.f.fset.Position(from).Line
	d := declPrint{
		Hash:   hashOf([]byte(st.toks.printRange(doc, to, base, false))),
		Names:  names,
		Kind:   kind,
		Linked: linked(st.toks.directives(doc, to)),
		Start:  base,
		End:    st.f.fset.Position(to).Line,
		text:   idents,
		header: idents,
	}
	key := kind + " " + strings.Join(names, ",")
	if kind == kindRun && len(names) == 0 {
		key = kindRun + " _"
	}

	return ps.add(st, key, d)
}

// add files an entity under its key. A key that is not unique in its file —
// several init functions, several blank declarations — is numbered in source
// order instead. Numbering pairs nothing reliably, which is why every kind
// that can share a key is one whose change re-maps the package whatever it
// is paired with.
//
// It returns the key it filed the entity under.
func (ps *printSet) add(st *fileState, key string, d declPrint) string {
	d.File = st.profile
	full := st.profile + ":" + key
	n := st.ordinals[key]
	st.ordinals[key] = n + 1
	if d.Kind == kindInit || d.Kind == kindBlank || d.Kind == kindMain || d.Kind == kindRun && len(d.Names) == 0 || n > 0 {
		full = fmt.Sprintf("%s#%d", full, n)
		if n > 0 && d.Kind != kindInit && d.Kind != kindRun && d.Kind != kindMain {
			d.Kind = kindBlank
		}
	}
	// A test file is never instrumented, so a test-file entity's body is
	// followed by name like everything else about it.
	if st.isTest {
		d.header = d.text
	}
	ps.out.Decls[full] = d

	return full
}

// runsAtInit reports whether a var spec's initialiser calls something when
// the package is initialised. That call runs for every test, and whatever it
// does — register a handler, seed a table — is reached by tests that never
// name the variable. A function literal's body runs only when it is called,
// so a literal that is not invoked on the spot calls nothing yet.
func (ps *printSet) runsAtInit(st *fileState, s *ast.ValueSpec) bool {
	runs := false
	for _, v := range s.Values {
		ast.Inspect(v, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.CallExpr:
				if !ps.pureCall(st, n.Fun) || !ps.formatsNothing(st, n) {
					runs = true

					return false
				}
			}

			return true
		})
	}

	return runs
}

// pureCall reports whether a call's callee is known to do nothing but compute
// its value: a conversion to a type written out as one, a predeclared function
// the package does not shadow, or an allowlisted constructor. Its arguments
// are still read, so a call among them still counts.
func (ps *printSet) pureCall(st *fileState, fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.ParenExpr:
		return ps.pureCall(st, f.X)
	case *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType,
		*ast.InterfaceType, *ast.StructType:
		return true
	case *ast.Ident:
		if ps.declared[f.Name] {
			return false
		}
		obj := types.Universe.Lookup(f.Name)
		if _, isType := obj.(*types.TypeName); isType {
			return true
		}

		return pureBuiltins[f.Name]
	case *ast.SelectorExpr:
		pkg, ok := f.X.(*ast.Ident)
		if !ok {
			return false
		}
		path, bound := st.imports[pkg.Name]

		return bound && allowedInitCalls[path] == f.Sel.Name
	}

	return false
}

// formatsNothing reports whether a call, if it is fmt.Errorf, formats only
// what runs nothing of the package: a literal, or an error errors.New made of
// one (see printSet.stdErrors). Any other operand can have a String or Error
// method, and Errorf calls it there and then. A call to anything else formats
// nothing.
//
// It is asked only of a callee pureCall allowed, so a selector here is an
// allowlisted constructor of an imported package; the callee is unwrapped as
// pureCall unwraps it, or `(fmt.Errorf)(...)` would pass as no Errorf at all.
func (ps *printSet) formatsNothing(st *fileState, call *ast.CallExpr) bool {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return true
	}
	if pkg, isIdent := sel.X.(*ast.Ident); !isIdent || st.imports[pkg.Name] != "fmt" || sel.Sel.Name != "Errorf" {
		return true
	}
	if call.Ellipsis.IsValid() {
		return false
	}
	for _, arg := range call.Args {
		switch a := arg.(type) {
		case *ast.BasicLit:
		case *ast.Ident:
			if !ps.stdErrors[st.f.ast.Name.Name][a.Name] {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// observes reports whether a var's initialiser reads another package's state
// (see declPrint.Observes). Which variable a name resolves to is the
// type-checker's to say — a dot import, or a variable of this package holding
// another's address, puts no selector in the text — so it is asked
// (typeFacts.observers). Without it the syntax is read instead (see
// readsImports); such a print re-maps the package whenever it is compared
// (see wholeReason), so what the fallback misses is never kept. A var in a
// file the type-check did not read by that name is taken to observe: an
// observer too many costs a re-map only when the order across packages moved.
func (ps *printSet) observes(st *fileState, s *ast.ValueSpec) bool {
	if ps.facts == nil {
		return ps.readsImports(st, s)
	}
	if !ps.facts.observers[observerKey(st.f.name, "")] {
		// The type-check read no file of this name, so it says nothing
		// about this var: it is taken to observe.
		return true
	}
	for _, n := range s.Names {
		if ps.facts.observers[observerKey(st.f.name, n.Name)] {
			return true
		}
	}

	return false
}

// readsImports is observes read off the syntax: a selector on an imported
// package's name anywhere outside a function literal, other than the callee of
// an allowlisted constructor. It misses a read through a dot import, and one
// through a variable of this package, which is why it is only the fallback.
func (ps *printSet) readsImports(st *fileState, s *ast.ValueSpec) bool {
	callees := map[ast.Expr]bool{}
	reads := false
	for _, v := range s.Values {
		ast.Inspect(v, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.CallExpr:
				if ps.pureCall(st, n.Fun) {
					callees[n.Fun] = true
				}
			case *ast.SelectorExpr:
				if pkg, ok := n.X.(*ast.Ident); ok && !callees[n] {
					if _, bound := st.imports[pkg.Name]; bound {
						reads = true
					}
				}
			}

			return !reads
		})
	}

	return reads
}

func linked(directives []string) bool {
	for _, d := range directives {
		if strings.HasPrefix(d, "//go:linkname") {
			return true
		}
	}

	return false
}

func docStart(doc *ast.CommentGroup, pos token.Pos) token.Pos {
	if doc != nil {
		return doc.Pos()
	}

	return pos
}

// specRange is where a spec's print starts and ends, and where its doc
// comment starts. A lone spec carries the keyword and the declaration's doc
// comment, where a //go:embed lives; one in a group carries its own.
func specRange(gd *ast.GenDecl, spec ast.Spec, doc *ast.CommentGroup) (token.Pos, token.Pos, token.Pos) {
	if !gd.Lparen.IsValid() {
		return gd.Pos(), gd.End(), docStart(gd.Doc, gd.Pos())
	}

	return spec.Pos(), spec.End(), docStart(doc, spec.Pos())
}

// identsOf is every identifier the nodes mention, selectors' names included:
// matching is by name, and over-approximate on purpose.
func identsOf(nodes ...ast.Node) map[string]bool {
	out := map[string]bool{}
	for _, n := range nodes {
		if n == nil || isNilNode(n) {
			continue
		}
		ast.Inspect(n, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				out[id.Name] = true
			}

			return true
		})
	}

	return out
}

// isNilNode catches a typed nil inside an ast.Node interface, which a missing
// receiver list is.
func isNilNode(n ast.Node) bool {
	if fl, ok := n.(*ast.FieldList); ok {
		return fl == nil
	}

	return false
}

// receiverName is the base type name of a method's receiver: T for T, *T,
// T[K] and *T[K].
func receiverName(fn *ast.FuncDecl) string {
	if len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}
