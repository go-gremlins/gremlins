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
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Facts the compiler derives are asked of the compiler. A print holds what the
// source says; what the type-checker works out from it — which variable waits
// for which, through function bodies nothing executes — is in no token of it,
// and every hole the reuse rule has had was one of those facts read off the
// syntax, or not read at all. So the facts that decide what runs before any
// test are taken from go/types, over the same files, build tags and
// environment the test binary is compiled from.
//
// The first is the order a package's variables are initialised in
// (types.Info.InitOrder). Go initialises a package's variables in declaration
// order, except that a variable waits for every variable its initialiser
// depends on, and that dependency is found through the bodies of the
// functions it calls, transitively. So a function body no test executed can
// reorder initialisation — `_ = b` added to a helper an initialiser's callee
// only names — and so can swapping two lines that print the same. Neither is
// a change to any name a test reaches.
//
// The second is which variables read another package's state when they are
// initialised (typeFacts.observers). A read of another package is a name that
// resolves to one of its variables, which the syntax cannot tell from a name
// of this package's own once a dot import is involved, nor follow through a
// variable of this package that holds the other's address.

// typeFacts is what the type-checker says about one package directory.
type typeFacts struct {
	// initOrder is every package-level variable with an initialiser, in the
	// order the package initialises them: the package as its test binary
	// links it — its test files included — and then its external test
	// package, whose names carry xtestPrefix. A blank variable has no name
	// to follow, so it is named by its file and its place among the file's
	// blanks.
	initOrder []string
	// reach is a hash of every package the package imports, directly or
	// not, as its test binary links it — and then of its external test
	// package's — which is its place in the order packages are initialised
	// in (see importClosure).
	reach string
	// observers is every package-level variable whose initialiser reads a
	// variable of another package, directly or through variables of its own
	// package that do, keyed by its file's name and its own (see
	// observerKey): the package's variables and its external test
	// package's, which are in files of their own. What it reads then
	// depends on which packages were initialised first (see
	// declPrint.Observes). Each file the type-check read is in it under an
	// empty name, so that a file it did not read under that name — cgo's,
	// compiled from a file of another — is known to be one it says nothing
	// about.
	observers map[string]bool
}

// observerKey is how typeFacts.observers names a variable.
func observerKey(file, name string) string {
	return file + "\x00" + name
}

// xtestPrefix marks a variable of the external test package, whose names can
// coincide with the package's own.
const xtestPrefix = "xtest\x00"

// typeLoader type-checks packages by import path: with their test files and
// their external test package when tests is set, as they are linked into any
// other test binary when it is not. A package it could not type-check
// cleanly is missing from the result.
type typeLoader func(tests bool, importPaths []string) map[string]typeFacts

// typesKey is a package as it was type-checked.
type typesKey struct {
	importPath string
	tests      bool
}

// typeFactsOf is a package's type facts, type-checking it on first use. A
// package that cannot be type-checked reports false, and its print is then
// one reuse never narrows from.
func (c *Coverage) typeFactsOf(importPath string, tests bool) (typeFacts, bool) {
	c.loadTypes(tests, []string{importPath})
	facts, ok := c.types[typesKey{importPath, tests}]

	return facts, ok
}

// typedFacts is typeFactsOf as a print takes it: nil for a package that
// cannot be type-checked.
func (c *Coverage) typedFacts(importPath string, tests bool) *typeFacts {
	facts, ok := c.typeFactsOf(importPath, tests)
	if !ok {
		return nil
	}

	return &facts
}

// loadTypes type-checks every package of a set that has not been already, in
// one load: the package being mapped is asked for with its instrumented
// dependencies, and each load is a `go list` and a build of export data, so
// asking one at a time would cost that for every dependency.
//
// A package that failed is remembered as failed and not asked for again.
func (c *Coverage) loadTypes(tests bool, importPaths []string) {
	if c.types == nil {
		c.types = map[typesKey]typeFacts{}
		c.typesTried = map[typesKey]bool{}
	}
	var missing []string
	for _, p := range importPaths {
		key := typesKey{p, tests}
		if !c.typesTried[key] {
			c.typesTried[key] = true
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return
	}
	load := c.typeLoader
	if load == nil {
		load = c.loadTypesFromSource
	}
	for p, facts := range load(tests, missing) {
		c.types[typesKey{p, tests}] = facts
	}
}

// loadTypesFromSource is the typeLoader of a real run: go/packages, under the
// build tags the test binaries are compiled with, from the module root.
// Dependencies outside the set are read from export data, which the build
// cache usually holds already from earlier runs. The test binary is no longer
// compiled first: the fingerprint is taken before it, so that an unchanged
// package can skip the compile.
//
// The import graph is a second load that asks for nothing but names and
// imports: asking the first for dependencies would type-check every one of
// them from source.
func (c *Coverage) loadTypesFromSource(tests bool, importPaths []string) map[string]typeFacts {
	var loads []map[string]*variants
	for _, mode := range []packages.LoadMode{
		packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		packages.NeedName | packages.NeedImports | packages.NeedDeps,
	} {
		found, ok := c.loadVariants(mode, tests, importPaths)
		if !ok {
			return nil
		}
		loads = append(loads, found)
	}

	return factsOf(loads[0], loads[1], tests)
}

// factsOf reads each package's facts off its type-checked variants and the
// same variants as the import listing returned them. A package either says
// nothing about is left out.
func factsOf(typed, graph map[string]*variants, tests bool) map[string]typeFacts {
	out := map[string]typeFacts{}
	for path, v := range typed {
		g := graph[path]
		order, ok := initOrderOf(v.linked(tests), "")
		inGraph := g.linked(tests)
		if !ok || inGraph == nil {
			continue
		}
		reach := []string{strings.Join(importClosure(inGraph), "\x00")}
		observers := observersOf(v.linked(tests), map[string]bool{})
		if tests && v.xtest != nil {
			more, xok := initOrderOf(v.xtest, xtestPrefix)
			if !xok || g.xtest == nil {
				continue
			}
			order = append(order, more...)
			reach = append(reach, strings.Join(importClosure(g.xtest), "\x00"))
			observers = observersOf(v.xtest, observers)
		}
		out[path] = typeFacts{
			initOrder: order,
			reach:     hashOf([]byte(strings.Join(reach, "\x00xtest\x00"))),
			observers: observers,
		}
	}

	return out
}

// variants is one package directory as go/packages returns it. Under Tests, a
// package with test files comes back twice, as itself and as recompiled for
// its test binary, and its external tests as a third package.
type variants struct {
	plain, test, xtest *packages.Package
}

// linked is the variant a binary links: the recompiled one in the package's
// own test binary, the plain one in any other. Nil on a nil receiver, which
// is a package the load did not return.
func (v *variants) linked(tests bool) *packages.Package {
	if v == nil {
		return nil
	}
	if tests && v.test != nil {
		return v.test
	}

	return v.plain
}

// loadVariants loads a set of packages in one mode and sorts what comes back
// into each one's variants. A load that fails says nothing about any of them.
func (c *Coverage) loadVariants(mode packages.LoadMode, tests bool, importPaths []string) (map[string]*variants, bool) {
	cfg := &packages.Config{Mode: mode, Dir: c.absRoot(), Tests: tests}
	if c.buildTags != "" {
		cfg.BuildFlags = []string{"-tags", c.buildTags}
	}
	loaded, err := packages.Load(cfg, importPaths...)
	if err != nil {
		return nil, false
	}
	want := map[string]bool{}
	for _, p := range importPaths {
		want[p] = true
	}
	found := map[string]*variants{}
	for _, p := range loaded {
		root, isTest := strings.CutSuffix(p.ID, " ["+strings.TrimSuffix(p.PkgPath, "_test")+".test]")
		path := strings.TrimSuffix(root, "_test")
		if !want[path] {
			continue
		}
		v := found[path]
		if v == nil {
			v = &variants{}
			found[path] = v
		}
		switch {
		case root != path:
			v.xtest = p
		case isTest:
			v.test = p
		default:
			v.plain = p
		}
	}

	return found, true
}

// importClosure is every package a package imports, directly or not, by
// import path and sorted.
//
// The IDs are import paths, with the test binary a package was recompiled
// for when it was. It is what decides where the package is initialised among
// the rest of the binary. Go initialises, each time, the first package by import path whose
// imports are all initialised, and the set already initialised always holds
// every import of each of its members — so "every import initialised" and
// "every package it reaches initialised" are the same condition, and the
// order is a function of the packages linked and each one's closure alone.
// An import added that the package already reached through another moves
// nothing.
func importClosure(p *packages.Package) []string {
	seen := map[string]bool{}
	var walk func(q *packages.Package)
	walk = func(q *packages.Package) {
		for _, imp := range q.Imports {
			if seen[imp.ID] {
				continue
			}
			seen[imp.ID] = true
			walk(imp)
		}
	}
	walk(p)
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)

	return out
}

// initOrderOf names a type-checked package's initialisers in order. A package
// with any error says nothing: an initialiser it could not resolve is a
// dependency it did not record.
func initOrderOf(p *packages.Package, prefix string) ([]string, bool) {
	if p == nil || len(p.Errors) > 0 || len(p.TypeErrors) > 0 || p.TypesInfo == nil || p.Fset == nil {
		return nil, false
	}
	blanks := blankNames(p)
	var order []string
	for _, init := range p.TypesInfo.InitOrder {
		for _, v := range init.Lhs {
			name := v.Name()
			if name == "_" {
				name = blanks[v]
			}
			order = append(order, prefix+name)
		}
	}

	return order, true
}

// observersOf adds to a set every package-level variable of a type-checked
// package whose initialiser reads a variable of another package, and returns
// the set. It is asked only of a package initOrderOf read cleanly.
//
// A read is any name in the initialiser that resolves to a variable whose
// package is not this one — however it is spelled: `reg.Names`, `Names`
// under a dot import. A field is not one: it is read off a value, and the
// value is the read that counts — `reg.Cfg.Size` reads reg.Cfg, while a
// field of an imported type selected off this package's value reads nothing
// of the other package. A constant,
// type or function of another package carries no state of its own, and a
// call runs code, which the print classifies as kindRun before this is
// asked. A function literal's body runs only when it is called.
//
// A variable that reads one of this package's variables that does is an
// observer too: `var names = &reg.Names; var seen = len(*names)` reads reg
// through names. The order within the package puts names first, so seen sees
// whatever names did.
func observersOf(p *packages.Package, into map[string]bool) map[string]bool {
	type spec struct {
		file  string
		lhs   []*types.Var
		reads []*types.Var
	}
	var specs []spec
	// observing is every variable a read of which observes the order: another
	// package's, and this package's observers as they are found.
	observing := map[*types.Var]bool{}
	for _, f := range p.Syntax {
		// The file's own name, not the one a //line directive gives it:
		// the print is keyed by the name on disk.
		file := filepath.Base(p.Fset.PositionFor(f.Package, false).Filename)
		into[observerKey(file, "")] = true
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, s := range gd.Specs {
				vs, _ := s.(*ast.ValueSpec)
				sp := spec{file: file}
				for _, n := range vs.Names {
					if v, isVar := p.TypesInfo.Defs[n].(*types.Var); isVar {
						sp.lhs = append(sp.lhs, v)
					}
				}
				for _, val := range vs.Values {
					ast.Inspect(val, func(n ast.Node) bool {
						if _, lit := n.(*ast.FuncLit); lit {
							return false
						}
						id, isIdent := n.(*ast.Ident)
						if !isIdent {
							return true
						}
						v, isVar := p.TypesInfo.Uses[id].(*types.Var)
						switch {
						case !isVar || v.IsField():
						case v.Pkg() != p.Types:
							observing[v] = true
							sp.reads = append(sp.reads, v)
						case v.Parent() == p.Types.Scope():
							sp.reads = append(sp.reads, v)
						}

						return true
					})
				}
				specs = append(specs, sp)
			}
		}
	}
	for grew := true; grew; {
		grew = false
		for _, sp := range specs {
			if !readsAny(sp.reads, observing) {
				continue
			}
			for _, v := range sp.lhs {
				if !observing[v] {
					observing[v] = true
					grew = true
				}
				into[observerKey(sp.file, v.Name())] = true
			}
		}
	}

	return into
}

func readsAny(reads []*types.Var, set map[*types.Var]bool) bool {
	for _, v := range reads {
		if set[v] {
			return true
		}
	}

	return false
}

// blankNames names every blank variable an initialiser assigns by its file
// and its place among that file's blanks, which is what pairs it across two
// prints as well as anything can: its position moves with every edit above it.
func blankNames(p *packages.Package) map[*types.Var]string {
	type blank struct {
		v   *types.Var
		pos token.Position
	}
	var all []blank
	for _, init := range p.TypesInfo.InitOrder {
		for _, v := range init.Lhs {
			if v.Name() == "_" {
				all = append(all, blank{v, p.Fset.Position(v.Pos())})
			}
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].pos.Filename != all[j].pos.Filename {
			return all[i].pos.Filename < all[j].pos.Filename
		}

		return all[i].pos.Offset < all[j].pos.Offset
	})
	out := map[*types.Var]string{}
	seen := map[string]int{}
	for _, b := range all {
		file := filepath.Base(b.pos.Filename)
		out[b.v] = fmt.Sprintf("_@%s#%d", file, seen[file])
		seen[file]++
	}

	return out
}

// sameInitOrder reports whether every variable two prints both initialise is
// initialised in the same order relative to the others.
//
// A variable only one of them has is left out of the comparison, and that is
// sound for the same reason the rest of the closure is: one added or removed
// is a changed name, so everything that reads it is dirty by name; one whose
// initialiser has an effect re-maps the package as kindRun before this is
// asked; and a plain initialiser computes a value and nothing else, so where
// it lands moves no other variable — Go's order is declaration order, held
// back only by dependencies, and a variable that newly depends on it mentions
// a name that changed. What is left is two variables that were both there
// trading places, which is what this catches.
func sameInitOrder(was, now []string) bool {
	_, _, swapped := initOrderSwap(was, now)

	return !swapped
}

// initOrderSwap is sameInitOrder's answer with its evidence: when the two
// orders disagree, the variables at the first place they do, as the first
// order has it and as the second does.
func initOrderSwap(was, now []string) (string, string, bool) {
	in := func(list []string) map[string]bool {
		out := make(map[string]bool, len(list))
		for _, name := range list {
			out[name] = true
		}

		return out
	}
	common := func(list []string, other map[string]bool) []string {
		var out []string
		for _, name := range list {
			if other[name] {
				out = append(out, name)
			}
		}

		return out
	}
	a, b := common(was, in(now)), common(now, in(was))
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i], b[i], true
		}
	}
	// One lists a shared name more often than the other, which is no order
	// two variables can be compared by.
	if len(a) > len(b) {
		return a[len(b)], a[len(b)], true
	}
	if len(b) > len(a) {
		return b[len(a)], b[len(a)], true
	}

	return "", "", false
}
