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
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/tools/go/packages"

	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

// Placed is a mutant compiled into the schema build: its test binaries run it
// when GREMLINS_MUTANT is ID.
type Placed struct {
	Mutator mutator.Mutator
	ID      int
}

// NetEntry is a mutant that goes through the per-mutant path, and why.
type NetEntry struct {
	Mutator mutator.Mutator
	Reason  string
}

// Plan is how a run's mutants execute: the schema build, the mutants placed
// in it, and the mutants netted to the per-mutant path. Placed and Netted
// are each in stream order.
type Plan struct {
	Build  Build
	Placed []Placed
	Netted []NetEntry
}

// NullRunFunc runs the test binary bin of build b once, with no mutant
// selected, for the package whose directory in the original module is pkgDir.
// It runs the binary where a worker would, in a copy of that directory: never
// in the original module, which Prepare does not write to. b is the build bin
// belongs to, for the overlay a test running the go command needs to see the
// source the binary was built from.
type NullRunFunc func(b *Build, bin, pkgDir string) error

// nullFailed prefixes the reason of a mutant netted by a failing null run.
const nullFailed = "null-mutant run failed: "

// Prepare places the run's mutants in one schema build of mod.
//
// The mutants get ids 1..N in the order runnable lists them. Mutants of one
// package at one operator token form one Site. Each package is loaded with
// tags and rewritten; every package testPkgs names for a package with a
// placed site has its test binary built in workDir as BuildAll builds it,
// within BuildTimeout(buildTimeout, allowance, packages built) -- allowance
// is the per-mutant compile allowance, which bounds one mutant's compile,
// not this one build of every package. One with no test files has none,
// and is no failure: its run passes without reach.
// Each binary then runs once without a mutant selected through nullRun,
// given the package's directory in mod -- the original source, which nullRun
// runs a copy of, as a worker does -- and once more if that fails.
//
// A mutant is netted when it maps to no operator in the loaded source, when
// the rewrite drops its site, when testPkgs names no package for its package,
// when its package's rewritten files cannot be written, or when the build or
// the second null run of a package testPkgs names for it fails. A netted mutant has a reason and an error log line carrying the
// reason's first line, cut to 200 bytes; a reason the line cuts is logged
// whole once per package. Every mutant of runnable ends in exactly one of
// Placed and Netted. Prepare writes nothing outside workDir, which the caller
// removes. A relative mod.Root is taken from the working directory. The error
// is the context's, when it ends first.
func Prepare(ctx context.Context, mod gomodule.GoModule, workDir, tags string, runnable []mutator.Mutator,
	testPkgs func(pkg string) []string, allowance, buildTimeout time.Duration, nullRun NullRunFunc,
) (Plan, error) {
	return prepare(ctx, mod, workDir, tags, runnable, testPkgs, allowance, buildTimeout, nullRun, buildTest)
}

// prepare is Prepare with each package's test binary built by build.
func prepare(ctx context.Context, mod gomodule.GoModule, workDir, tags string, runnable []mutator.Mutator,
	testPkgs func(pkg string) []string, allowance, buildTimeout time.Duration, nullRun NullRunFunc,
	build buildFunc,
) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	// mod.Root is relative when the target was; go/packages reports
	// absolute directories, and every path built here must match them.
	if root, err := filepath.Abs(mod.Root); err == nil {
		mod.Root = root
	}
	p := &preparer{mod: mod, muts: runnable, reasons: make([]string, len(runnable))}
	if len(runnable) == 0 {
		return Plan{}, nil
	}

	rewritten, placedPkgs := p.rewrite(ctx, tags)
	// The loads end early when ctx does, netting what they did not finish.
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	var plan Plan
	need := map[string][]string{}
	var all []string
	for _, pkg := range placedPkgs {
		need[pkg] = testPkgs(pkg)
		all = append(all, need[pkg]...)
	}
	slices.Sort(all)
	all = slices.Compact(all)
	p.netUnselected(need)
	if len(all) > 0 {
		start := time.Now()
		b, errs := buildAll(ctx, mod.Root, workDir, tags, rewritten, all, BuildTimeout(buildTimeout, allowance, len(all)), build)
		log.Infof("schemata: built %d packages in %s\n", len(b.Binaries), time.Since(start).Round(time.Millisecond))
		if err := ctx.Err(); err != nil {
			return Plan{}, err
		}
		failed := map[string]string{}
		for pkg, err := range errs {
			failed[pkg] = err.Error()
		}
		if err := p.nullCheck(ctx, &b, failed, nullRun); err != nil {
			return Plan{}, err
		}
		p.netFailed(need, failed)
		plan.Build = b
	}

	type pkgReason struct{ pkg, reason string }
	fullLogged := map[pkgReason]bool{}
	for i, m := range runnable {
		if r := p.reasons[i]; r != "" {
			plan.Netted = append(plan.Netted, NetEntry{Mutator: m, Reason: r})
			short := shortReason(r)
			log.Errorf("schemata: %s at %s goes through the per-mutant path: %s\n", m.Type(), m.Position(), short)
			if k := (pkgReason{m.Pkg(), r}); short != r && !fullLogged[k] {
				fullLogged[k] = true
				log.Errorf("schemata: per-mutant path for %s: %s\n", m.Pkg(), r)
			}

			continue
		}
		plan.Placed = append(plan.Placed, Placed{Mutator: m, ID: i + 1})
		log.Infof("schemata: id %d = %s %s\n", i+1, m.Position(), m.Type())
	}
	log.Infof("schemata: %d mutants placed, %d netted\n", len(plan.Placed), len(plan.Netted))

	return plan, nil
}

// preparer holds the netting state of one Prepare: reasons[i] is the reason
// mutant i (id i+1) is netted, "" while it is placed.
type preparer struct {
	mod     gomodule.GoModule
	muts    []mutator.Mutator
	reasons []string
	// pkgOf is the import path of each mutant's package, for netFailed.
	pkgOf []string
}

func (p *preparer) net(i int, reason string) {
	if p.reasons[i] == "" {
		p.reasons[i] = reason
	}
}

// file is the absolute path of mutant i's file.
func (p *preparer) file(i int) string {
	name := p.muts[i].Position().Filename
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}

	return filepath.Join(p.mod.Root, p.mod.CallingDir, name)
}

// rewrite loads the mutants' packages and rewrites each, netting what it
// cannot place. It returns the rewritten files by import path and the
// import paths of the packages with a placed site, in stream order.
func (p *preparer) rewrite(ctx context.Context, tags string) (map[string]map[string][]byte, []string) {
	p.pkgOf = make([]string, len(p.muts))
	dirs := make([]string, len(p.muts))
	var patterns []string
	for i := range p.muts {
		dirs[i] = filepath.Dir(p.file(i))
		// A directory outside the module root cannot be loaded as one of its
		// packages; its mutants alone take the per-mutant path.
		pattern, ok := loadPattern(p.mod.Root, dirs[i])
		if !ok {
			p.net(i, fmt.Sprintf("schemata: %s is not under the module root %s", dirs[i], p.mod.Root))

			continue
		}
		patterns = append(patterns, pattern)
	}
	pkgs, err := loadPackages(ctx, p.mod.Root, tags, patterns)
	if err != nil {
		for i := range p.muts {
			p.net(i, err.Error())
		}

		return nil, nil
	}

	byDir := map[string]*packages.Package{}
	for _, lp := range pkgs {
		if lp.Dir != "" {
			byDir[filepath.Clean(lp.Dir)] = lp
		}
	}
	var order []*packages.Package
	groups := map[*packages.Package][]int{}
	for i, m := range p.muts {
		lp := byDir[dirs[i]]
		switch {
		case lp == nil:
			p.net(i, fmt.Sprintf("schemata: no package loaded from %s", dirs[i]))
		case lp.PkgPath != m.Pkg():
			p.net(i, fmt.Sprintf("schemata: package %s loads as %s", m.Pkg(), lp.PkgPath))
		case len(lp.Errors) > 0:
			p.net(i, fmt.Sprintf("schemata: load %s: %v", lp.PkgPath, lp.Errors[0]))
		default:
			if _, ok := groups[lp]; !ok {
				order = append(order, lp)
			}
			groups[lp] = append(groups[lp], i)
			p.pkgOf[i] = lp.PkgPath
		}
	}

	rewritten := map[string]map[string][]byte{}
	var placedPkgs []string
	for _, lp := range order {
		sites := p.sites(lp, groups[lp])
		files, placed, dropped := RewritePackage(ctx, lp, sites, tags)
		for _, d := range dropped {
			for _, mu := range d.Site.Muts {
				p.net(mu.ID-1, d.Err.Error())
			}
		}
		if len(placed) > 0 {
			rewritten[lp.PkgPath] = files
			placedPkgs = append(placedPkgs, lp.PkgPath)
		}
	}

	return rewritten, placedPkgs
}

// tokenKey is where an operator token sits.
type tokenKey struct {
	file         string
	line, column int
}

// sites maps the mutants idx of lp to the operator nodes of its syntax, one
// Site per token, in stream order; a mutant at no operator is netted.
func (p *preparer) sites(lp *packages.Package, idx []int) []Site {
	type op struct {
		node ast.Node
		tok  token.Token
	}
	// ops is keyed by adjusted position, which is how the engine names a
	// mutant. An operator a line directive moves (adjusted and raw positions
	// differ) is never placed, and it poisons its adjusted key: a mutant
	// there may be that operator's or the one whose raw position the key
	// names, and the two cannot be told apart.
	ops := map[tokenKey]op{}
	moved := map[tokenKey]bool{}
	for _, f := range lp.Syntax {
		ast.Inspect(f, func(n ast.Node) bool {
			var pos token.Pos
			var tok token.Token
			switch n := n.(type) {
			case *ast.BinaryExpr:
				pos, tok = n.OpPos, n.Op
			case *ast.UnaryExpr:
				pos, tok = n.OpPos, n.Op
			case *ast.IncDecStmt:
				pos, tok = n.TokPos, n.Tok
			case *ast.AssignStmt:
				pos, tok = n.TokPos, n.Tok
			case *ast.BranchStmt:
				pos, tok = n.TokPos, n.Tok
			default:
				return true
			}
			at, raw := lp.Fset.PositionFor(pos, true), lp.Fset.PositionFor(pos, false)
			k := tokenKey{filepath.Clean(at.Filename), at.Line, at.Column}
			if at != raw {
				moved[k] = true
			} else {
				ops[k] = op{n, tok}
			}

			return true
		})
	}

	var sites []Site
	at := map[tokenKey]int{}
	for _, i := range idx {
		pos := p.muts[i].Position()
		k := tokenKey{p.file(i), pos.Line, pos.Column}
		if moved[k] {
			p.net(i, fmt.Sprintf("schemata: a line directive moves an operator to %s:%d:%d", k.file, k.line, k.column))

			continue
		}
		o, ok := ops[k]
		if !ok {
			p.net(i, fmt.Sprintf("schemata: no mutation site at %s:%d:%d in the loaded source", k.file, k.line, k.column))

			continue
		}
		mu := Mutant{ID: i + 1, Type: p.muts[i].Type()}
		if s, ok := at[k]; ok {
			sites[s].Muts = append(sites[s].Muts, mu)

			continue
		}
		at[k] = len(sites)
		sites = append(sites, Site{Node: o.node, Tok: o.tok, Muts: []Mutant{mu}})
	}

	return sites
}

// nullCheck runs every built binary once with no mutant selected, and once
// more on failure; a second failure marks the package failed.
func (p *preparer) nullCheck(ctx context.Context, b *Build, failed map[string]string, nullRun NullRunFunc) error {
	for _, pkg := range slices.Sorted(maps.Keys(b.Binaries)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir, ok := p.mod.PackageDir(p.mod.Root, pkg)
		if !ok {
			failed[pkg] = fmt.Sprintf("schemata: %s is not a package of module %s", pkg, p.mod.Name)

			continue
		}
		bin := b.Binaries[pkg]
		if nullRun(b, bin, dir) == nil {
			continue
		}
		if err := nullRun(b, bin, dir); err != nil {
			failed[pkg] = nullFailed + firstLine(err)
		}
	}

	return nil
}

// noTestPackage is the reason a mutant is netted when its package's mutants
// select no test package: what to run then is the per-mutant path's to decide.
const noTestPackage = "no test package selected"

// netUnselected nets every still-placed mutant whose package's mutants select
// no test package.
func (p *preparer) netUnselected(need map[string][]string) {
	for i := range p.muts {
		if pkg := p.pkgOf[i]; pkg != "" && len(need[pkg]) == 0 {
			p.net(i, noTestPackage)
		}
	}
}

// netFailed nets every still-placed mutant whose own package, or a package
// need lists for it, failed. The own package counts even when need does not
// list it: BuildAll records a failure to write a package's rewritten files
// under that package, and then no binary compiles its mutants in.
func (p *preparer) netFailed(need map[string][]string, failed map[string]string) {
	for i := range p.muts {
		if p.reasons[i] != "" {
			continue
		}
		own := p.pkgOf[i]
		for _, pkg := range append([]string{own}, need[own]...) {
			if r, ok := failed[pkg]; ok {
				p.net(i, r)

				break
			}
		}
	}
}

// loadPattern is the go/packages pattern, relative to modRoot, of the
// package in directory dir, or false when dir is not under modRoot.
func loadPattern(modRoot, dir string) (string, bool) {
	rel, err := filepath.Rel(modRoot, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return "./" + filepath.ToSlash(rel), true
}

// loadPackages loads the packages the patterns name, under modRoot, with
// what RewritePackage needs, under ctx.
func loadPackages(ctx context.Context, modRoot, tags string, patterns []string) ([]*packages.Package, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	slices.Sort(patterns)
	patterns = slices.Compact(patterns)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedModule | packages.NeedDeps | packages.NeedImports,
		Context: ctx,
		Dir:     modRoot,
	}
	if tags != "" {
		cfg.BuildFlags = []string{"-tags", tags}
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("schemata: load packages: %w", err)
	}

	return pkgs, nil
}

// shortReasonLimit bounds, in bytes, the reason a netted mutant's log line
// carries: a build failure's reason holds the compiler's whole output.
const shortReasonLimit = 200

// shortReason is the first line of reason, cut to at most shortReasonLimit
// bytes without splitting a character.
func shortReason(reason string) string {
	short, _, _ := strings.Cut(reason, "\n")
	if len(short) <= shortReasonLimit {
		return short
	}
	short = short[:shortReasonLimit]
	for len(short) > 0 && !utf8.ValidString(short) {
		short = short[:len(short)-1]
	}

	return short
}

// firstLine is the first non-empty line of err's message.
func firstLine(err error) string {
	for _, l := range strings.Split(err.Error(), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}

	return "no output"
}
