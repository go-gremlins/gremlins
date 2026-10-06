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
	"bytes"
	"path"
	"sort"
	"strings"

	"golang.org/x/tools/cover"
)

// Without --cross-package a package's tests are mapped with the package AND its
// in-module dependencies instrumented. The dependencies are not there to be
// judged — a mutant is only ever judged by its own package's tests, and what a
// test executed in a dependency never reaches TestsFor, Union or the coverage
// merge — but to be narrowed across. Before, a dependency's lines were in no
// profile, so any change to one re-mapped every package that linked it; now a
// test records which of a dependency's entities it executed, so a changed body
// there dirties exactly the tests that did, and a changed name whatever reaches
// it by name (see reusable).
//
// The record is a set of entity keys per test rather than blocks: narrowing
// asks "did this test reach the entity that changed", and keys are all that
// question needs, at a fraction of the cache.

// depSource is one instrumented dependency as the map builder reads it: the
// print it stores, and what it needs to attribute a profile block to an
// entity.
type depSource struct {
	// stored is the dependency's print as the cache keeps it: without
	// positions, because a dependency's mappings are keys, so nothing about
	// them moves when its lines do, and storing positions would only grow
	// every cache entry that links it.
	stored pkgPrint
	dir    string
	// spans is, per profile file name, where each entity sits.
	spans map[string][]keySpan
	// goFiles is the profile name of every Go file of the directory.
	goFiles map[string]bool
	// lineDirectives says a file renames or renumbers what coverage reports
	// (a //line or /*line directive), so a block's name cannot be trusted to
	// point at the function it came from.
	lineDirectives bool
}

type keySpan struct {
	key   string
	start int
	end   int
}

// depWhole is the key a test records when it executed a dependency somewhere
// its blocks could not be attributed to one entity. Any entity of that
// dependency changing then dirties the test. The NUL keeps it from ever
// colliding with a function key, which is made from a file name.
func depWhole(importPath string) string {
	return "pkg\x00" + importPath
}

// depSourceOf reads one instrumented dependency, once per run: a module's
// packages share their dependencies, and reading one again for every package
// that links it would cost what the per-directory hashing it replaces saves.
//
// A dependency's test files are not read at all. They are never linked into
// another package's test binary — not its tests, not its helpers, not an
// export_test.go — so no change to one can reach a mapping made there, and
// reading them would only turn such a change into a re-map.
func (c *Coverage) depSourceOf(dep dependency) (*depSource, bool) {
	if src, done := c.depSources[dep.dir]; done {
		return src, src != nil
	}
	if c.depSources == nil {
		c.depSources = map[string]*depSource{}
	}
	src := c.readDepSource(dep)
	c.depSources[dep.dir] = src

	return src, src != nil
}

func (c *Coverage) readDepSource(dep dependency) *depSource {
	files, ok := readPackageFiles(dep.dir)
	if !ok {
		return nil
	}
	below, ok := hashDataSubtrees(dep.dir)
	if !ok {
		return nil
	}
	printed := c.printPackage(dep.importPath, files, false, below, c.typedFacts(dep.importPath, false))

	src := &depSource{
		stored: pkgPrint{
			Whole: printed.Whole, Decls: map[string]declPrint{}, Imports: printed.Imports,
			InitOrder: printed.InitOrder, Reach: printed.Reach, Typed: printed.Typed,
		},
		dir:     dep.dir,
		spans:   map[string][]keySpan{},
		goFiles: map[string]bool{},
	}
	for key, d := range printed.Decls {
		src.spans[d.File] = append(src.spans[d.File], keySpan{key: key, start: d.Start, end: d.End})
		d.File, d.Start, d.End = "", 0, 0
		src.stored.Decls[key] = d
	}
	for i := range files {
		if !strings.HasSuffix(files[i].name, ".go") || strings.HasSuffix(files[i].name, "_test.go") {
			continue
		}
		src.goFiles[c.profileFileName(dep.importPath, files[i].name)] = true
		if bytes.Contains(files[i].data, []byte("//line ")) || bytes.Contains(files[i].data, []byte("/*line ")) {
			src.lineDirectives = true
		}
	}

	return src
}

// dependencySources reads every dependency the package's test binary
// instruments, keyed by import path, with nil for one that cannot be read.
//
// Every instrumented dependency has an entry either way, because the keys are
// what tells a dependency's block from one of the package's own (see
// splitCoverage). The second result says whether all of them were read, and a
// fingerprint is only usable when they were: every instrumented directory is
// left out of Inputs, so one that is not fingerprinted either would be a
// change nothing sees.
func (c *Coverage) dependencySources(pkg *testPackage) (map[string]*depSource, bool) {
	deps, ok := c.dependencyDirs(pkg)
	sources := map[string]*depSource{}
	for _, dep := range deps {
		if !dep.instrumented {
			continue
		}
		src, srcOK := c.depSourceOf(dep)
		sources[dep.importPath] = src
		ok = ok && srcOK
	}

	return sources, ok
}

// instrumentedImports is the import paths the package's test binary
// instruments besides the package, in no particular order.
func (c *Coverage) instrumentedImports(pkg *testPackage) []string {
	deps, _ := c.dependencyDirs(pkg)
	var paths []string
	for _, dep := range deps {
		if dep.instrumented {
			paths = append(paths, dep.importPath)
		}
	}

	return paths
}

// testCoverage is what one test's run recorded: its own package's profile,
// the dependency entities it executed, and whether every block it recorded
// could be placed.
type testCoverage struct {
	profile      Profile
	deps         []string
	attributable bool
}

// splitCoverage divides one test's raw profile into the package's own lines and
// the dependency entities it executed.
//
// The profile keeps exactly what it held before dependencies were instrumented
// — the package's own lines — because everything downstream reads it as
// coverage: TestsFor selects from it, Union widens the gathered profile with
// it, and the schemata executor's reach check agrees with both. A dependency's
// line executed only by another package's tests must stay NOT COVERED there;
// putting it in the profile would make mutants runnable that are not, and
// would select for them tests their package scoping never runs.
//
// sources has an entry for every dependency the binary instruments, nil where
// it could not be read. A block is a dependency's only when its package is one
// of those; anything else — a //line directive naming a file of no known
// package — stays in the profile, as it always did, and makes the package's
// fingerprint unusable, because nothing can say which change would reach it.
//
// A dependency block is attributed to the entity whose span holds it: a
// function, or a var whose initialiser is a function literal. One that falls
// in no entity, or whose name cannot be trusted — the dependency has a line
// directive, or the name is no Go file of it — is recorded against the whole
// dependency.
//
// Under --cross-package nothing is split: the profile is the whole module's by
// design there, and narrowing is off.
func (c *Coverage) splitCoverage(pkg *testPackage, profiles []*cover.Profile, sources map[string]*depSource) testCoverage {
	out := testCoverage{profile: Profile{}, attributable: true}
	if c.crossPackage {
		out.profile = c.blocksOf(profiles)

		return out
	}
	keys := map[string]bool{}
	for _, p := range profiles {
		owner := path.Dir(p.FileName)
		src, instrumented := sources[owner]
		if owner == pkg.importPath || !instrumented {
			if owner != pkg.importPath {
				out.attributable = false
			}
			c.addBlocks(out.profile, p)

			continue
		}
		if src == nil {
			out.attributable = false

			continue
		}
		c.attribute(src, owner, p, keys)
	}
	for key := range keys {
		out.deps = append(out.deps, key)
	}
	sort.Strings(out.deps)

	return out
}

// attribute records the dependency entities one file's executed blocks fall
// in.
func (c *Coverage) attribute(src *depSource, importPath string, p *cover.Profile, keys map[string]bool) {
	file := c.removeModuleFromPath(p)
	for _, b := range p.Blocks {
		if b.Count == 0 {
			continue
		}
		if src.lineDirectives || !src.goFiles[file] {
			keys[depWhole(importPath)] = true

			continue
		}
		placed := false
		for _, s := range src.spans[file] {
			if b.StartLine >= s.start && b.EndLine <= s.end {
				keys[s.key] = true
				placed = true

				break
			}
		}
		if !placed {
			keys[depWhole(importPath)] = true
		}
	}
}

// blocksOf is the profile of every executed block, under the names a Profile
// uses.
func (c *Coverage) blocksOf(profiles []*cover.Profile) Profile {
	status := make(Profile)
	for _, p := range profiles {
		c.addBlocks(status, p)
	}

	return status
}

func (c *Coverage) addBlocks(into Profile, p *cover.Profile) {
	for _, b := range p.Blocks {
		if b.Count == 0 {
			continue
		}
		fn := c.removeModuleFromPath(p)
		into[fn] = append(into[fn], Block{
			StartLine: b.StartLine,
			StartCol:  b.StartCol,
			EndLine:   b.EndLine,
			EndCol:    b.EndCol,
		})
	}
}

// touchesAny reports whether a test executed any dependency entity a change
// dirtied.
func touchesAny(keys []string, dirty map[string]bool) bool {
	for _, key := range keys {
		if dirty[key] {
			return true
		}
	}

	return false
}
