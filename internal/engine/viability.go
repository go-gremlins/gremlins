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

package engine

import (
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/tools/go/packages"

	"github.com/go-gremlins/gremlins/internal/log"
)

// Viability answers whether rewriting one token leaves the source it sits in
// legal Go.
//
// Gremlins rewrites a token in the AST and writes the file back out. The token
// table says which rewrites are meaningful for a token READ IN ISOLATION, and
// that is all a table can say: whether the result is a program at all depends
// on the types of the operands, on the constant values around it, and on what
// statements the enclosing function admits. None of that is in the token.
//
// A mutant the compiler rejects is not evidence about a test suite in either
// direction. It cannot be detected, so crediting a kill for it pays the suite
// for the compiler's work; recording it NOT VIABLE leaves the package's mutant
// set padded with entries that describe the compiler rather than the tests. So
// it is not generated, and the only oracle precise enough to decide that is a
// type-checker.
type Viability interface {
	// Viable reports whether replacing the token at pos with mutated leaves
	// the package that token belongs to type-correct.
	//
	// It reports TRUE whenever it cannot answer. A checker that cannot load a
	// package must fall back to generating the mutant and letting the compiler
	// adjudicate it, exactly as gremlins did before this existed: silently
	// dropping a mutant nobody proved illegal would shrink a package's mutant
	// set on the strength of a failure to look.
	Viable(pos token.Position, mutated token.Token) bool
}

// TypeViability answers Viable by type-checking the package the token belongs
// to, with the token rewritten in place.
//
// The unit is the PACKAGE, not the expression, because the error a mutation
// causes does not have to appear where the mutation is. `const week = 7 * 24 *
// time.Hour` rewritten to `7 / 24 * time.Hour` is a legal constant expression
// on its own line; it is the `d / week` three lines down that becomes a
// division by zero. Only a checker that sees the whole package sees that.
//
// The cost is one type-check per candidate mutant, which is the cheap end of
// what gremlins already spends: a mutant that IS generated costs a full
// recompile, link and test run, so a check that removes one pays for itself
// many times over. Measured on cerberus's internal/promql (101 files, ~93k
// lines), one re-check of the whole package takes ~100ms against a ~10s
// per-mutant cycle, and generation runs on its own goroutine behind the
// executor pool.
type TypeViability struct {
	// root is the directory the engine's fs.FS is rooted at, which is what
	// the relative filenames in a token.Position are relative to.
	root string
	// tags is the build-tag expression gremlins was given, so that the
	// package is loaded in the same configuration its tests run in.
	tags string

	// mu guards the caches below and the in-place token rewrites the checks
	// perform on the cached ASTs. Generation is single-goroutine today; the
	// lock is what keeps that from being load-bearing.
	mu       sync.Mutex
	packages map[string]*viabilityPackage
	// reported is the set of files already named in an "unchecked" log line,
	// so a file that cannot be checked says so once instead of once per token.
	reported map[string]bool
}

// NewTypeViability builds a Viability rooted at dir, loading packages with the
// given build tags. Nothing is loaded until the first Viable call, and then
// only the package of the file being asked about.
func NewTypeViability(dir, tags string) *TypeViability {
	// Absolute, because the positions this is compared against come from
	// go/packages and `go list` reports absolute paths, while gremlins'
	// GoModule.Root is relative whenever the scope was given as a relative
	// path -- which is how every documented invocation gives it.
	root, err := filepath.Abs(dir)
	if err != nil {
		root = dir
	}

	return &TypeViability{
		root:     root,
		tags:     tags,
		packages: make(map[string]*viabilityPackage),
		reported: make(map[string]bool),
	}
}

// viabilityPackage is one loaded, type-checkable package: its files parsed
// into a single FileSet, an importer that resolves its dependencies from the
// types packages.Load already built, and an index from source position to the
// token at that position so a mutation can be applied in place.
//
// A nil *viabilityPackage in the cache means "this directory cannot be
// checked", which is a permanent answer for the run and always yields viable.
type viabilityPackage struct {
	path     string
	fset     *token.FileSet
	files    []*ast.File
	importer types.Importer
	sizes    types.Sizes
	tokens   map[tokenSite]*token.Token
	// sources is the set of files this package is built from, so that a file
	// the package does NOT contain -- one the build tags excluded, say -- is
	// reported as unchecked rather than passed off as having no illegal
	// mutants.
	sources map[string]bool
}

// buildTagsFlag is how both `go list` (through go/packages, below) and
// `go test` (in getTestArgs) are told which build tags gremlins was given, so
// the package this checks is the one whose tests will run.
const buildTagsFlag = "-tags"

// tokenSite identifies a mutable token by where it is written, which is the
// only handle the engine and the checker share: they parse the same bytes into
// two independent ASTs, and a token.Position is stable across both.
type tokenSite struct {
	file   string
	line   int
	column int
}

// Viable implements Viability.
func (v *TypeViability) Viable(pos token.Position, mutated token.Token) bool {
	file := pos.Filename
	if !filepath.IsAbs(file) {
		file = filepath.Join(v.root, file)
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	pkg := v.packageFor(file)
	if pkg == nil {
		return true
	}
	tok, ok := pkg.tokens[tokenSite{file: file, line: pos.Line, column: pos.Column}]
	if !ok {
		// The engine and the checker parse the same bytes, so every mutable
		// token the engine finds in a file the package contains is indexed
		// here. A miss therefore means the file is not in the package, or
		// that the two disagree about where its tokens are -- and either way
		// this stopped being an oracle for that file, which has to be said
		// out loud rather than answered "viable".
		reason := "the token is not where the loaded package has one"
		if !pkg.sources[file] {
			reason = "the file is not one this package is built from, so no build of it was checked"
		}
		v.unchecked(file, reason)

		return true
	}

	original := *tok
	*tok = mutated
	defer func() { *tok = original }()

	return pkg.typeChecks()
}

// packageFor returns the loaded package owning file, loading it on first ask.
// It returns nil — cached, so the cost is paid once — when the file's package
// cannot be used as an oracle.
func (v *TypeViability) packageFor(file string) *viabilityPackage {
	dir := filepath.Dir(file)
	if pkg, ok := v.packages[dir]; ok {
		return pkg
	}
	pkg := v.load(dir, file)
	v.packages[dir] = pkg

	return pkg
}

func (v *TypeViability) load(dir, file string) *viabilityPackage {
	// A file the engine can see through its fs.FS but that is not on disk is
	// a fixture in a testing/fstest.MapFS, and `go list` has nothing to load.
	// Checked before the load so the common case costs a stat, not a `go
	// list` that will fail. It is also the only "unchecked" case that is not
	// logged: there is no package here to have been an oracle.
	if _, err := os.Stat(file); err != nil {
		return nil
	}

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax,
		Dir:  dir,
		Fset: fset,
	}
	if v.tags != "" {
		cfg.BuildFlags = []string{buildTagsFlag, v.tags}
	}

	loaded, err := packages.Load(cfg, ".")
	if err != nil {
		v.unchecked(dir, err.Error())

		return nil
	}
	if len(loaded) != 1 {
		v.unchecked(dir, "the directory does not resolve to exactly one package")

		return nil
	}

	pkg := &viabilityPackage{
		path:     loaded[0].PkgPath,
		fset:     fset,
		files:    loaded[0].Syntax,
		importer: dependencyImporter(loaded[0]),
		sizes:    types.SizesFor("gc", runtime.GOARCH),
		tokens:   make(map[tokenSite]*token.Token),
		sources:  make(map[string]bool, len(loaded[0].CompiledGoFiles)),
	}
	for _, f := range loaded[0].CompiledGoFiles {
		pkg.sources[f] = true
	}
	for _, f := range pkg.files {
		indexTokens(fset, f, pkg.tokens)
	}

	// The baseline. Everything this checker rejects, it rejects because the
	// mutation is the only thing that changed, so it may only be trusted over
	// a package that type-checks clean to begin with. This is also what
	// catches a misconfigured load — the wrong build tags, an unresolved
	// dependency, a language version this go/types cannot express — without
	// having to enumerate those failures: they all land here, and they all
	// disable the checker for that package rather than suppressing mutants.
	if !pkg.typeChecks() {
		v.unchecked(dir, "the unmutated package does not type-check")

		return nil
	}

	return pkg
}

// unchecked reports source whose mutants will be adjudicated by the compiler
// at run time instead. It is logged rather than silent because the difference
// between "no mutant here is illegal" and "nobody looked" is exactly what a
// NOT VIABLE count is read as evidence of -- and because a checker that
// quietly stops matching anything looks, from the outside, precisely like one
// that has nothing to reject.
func (v *TypeViability) unchecked(path, reason string) {
	if v.reported[path] {
		return
	}
	v.reported[path] = true
	log.Infof("not type-checking mutants in %s (%s); the compiler adjudicates them instead\n", path, reason)
}

// typeChecks reports whether the package's files, as they currently stand,
// type-check without a single error.
func (p *viabilityPackage) typeChecks() bool {
	clean := true
	conf := types.Config{
		Importer: p.importer,
		Sizes:    p.sizes,
		// Collecting rather than aborting: go/types stops at the first error
		// when Error is nil, and a checker that stops early reports the same
		// verdict either way. What matters is that an error handler exists so
		// a hard failure does not panic out of generation.
		Error: func(error) { clean = false },
		// GoVersion is deliberately left empty, which imposes no language
		// version at all. Guessing one wrong would reject legal source and
		// suppress honest mutants; imposing none can only ACCEPT source the
		// real compiler would reject, which leaves the mutant to be
		// adjudicated exactly as it is today.
	}
	_, _ = conf.Check(p.path, p.fset, p.files, nil)

	return clean
}

// indexTokens records every token the engine can mutate in this file, keyed by
// where it is written. It reads nodes through NewTokenNode so that the set of
// mutable tokens has one definition: a node shape the engine learns to mutate
// is a node shape this checker indexes, with no second list to keep in step.
func indexTokens(fset *token.FileSet, file *ast.File, out map[tokenSite]*token.Token) {
	ast.Inspect(file, func(n ast.Node) bool {
		node, ok := NewTokenNode(n)
		if !ok {
			return true
		}
		pos := fset.Position(node.TokPos)
		out[tokenSite{file: pos.Filename, line: pos.Line, column: pos.Column}] = node.tok

		return true
	})
}

// dependencyImporter resolves imports from the type information packages.Load
// already built for the dependency graph, so a re-check costs only the
// package's own files.
func dependencyImporter(root *packages.Package) types.Importer {
	resolved := make(mapImporter)
	var walk func(p *packages.Package)
	walk = func(p *packages.Package) {
		if p == nil || p.Types == nil {
			return
		}
		if _, seen := resolved[p.PkgPath]; seen {
			return
		}
		resolved[p.PkgPath] = p.Types
		for _, dep := range p.Imports {
			walk(dep)
		}
	}
	walk(root)

	return resolved
}

type mapImporter map[string]*types.Package

// Import implements types.Importer.
func (m mapImporter) Import(path string) (*types.Package, error) {
	if pkg, ok := m[path]; ok {
		return pkg, nil
	}

	// An unresolvable import makes the baseline check fail, which disables the
	// checker for the package. That is the intended outcome: a package whose
	// dependencies are not all present cannot decide anything about a mutant.
	return nil, os.ErrNotExist
}
