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

// Package deps answers which packages of a module depend on which, so that a
// mutation can be tested against the packages it could break.
package deps

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// execContext runs a command, so tests can stand in for the go tool.
type execContext = func(name string, args ...string) *exec.Cmd

// listFormat asks go list for what a dependency graph needs. Deps is already
// transitive; the two import lists are not, and they are here because a package
// whose TESTS import P exercises P even when none of its own code does. Leaving
// them out would silently miss real coverage of the mutated package.
const listFormat = `{{.ImportPath}}|{{join .Deps ","}}|{{join .TestImports ","}}|{{join .XTestImports ","}}`

// Graph holds, for each package, the packages that depend on it.
type Graph struct {
	dependents map[string][]string
}

// Dependents returns the packages whose code or tests depend on pkg, in a
// stable order. The package itself is not among them.
func (g *Graph) Dependents(pkg string) []string {
	if g == nil {
		return nil
	}

	return g.dependents[pkg]
}

// Len returns the number of packages that have dependents.
func (g *Graph) Len() int {
	if g == nil {
		return 0
	}

	return len(g.dependents)
}

// New builds the dependency graph of the packages under scanPath.
//
// It costs one `go list`, which compiles nothing and runs nothing. That is the
// whole of what cross-package mutation testing needs to know: unlike selecting
// tests by coverage, "which packages could this change break" is answered
// statically.
func New(cmdContext execContext, scanPath string) (*Graph, error) {
	out, err := cmdContext("go", "list", "-f", listFormat, scanPath).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("impossible to list the packages of the module: %w\n%s", err, out)
	}

	return parse(string(out)), nil
}

type pkgInfo struct {
	deps        []string
	testImports []string
}

func parse(out string) *Graph {
	pkgs := make(map[string]pkgInfo)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) != 4 {
			continue
		}
		pkgs[fields[0]] = pkgInfo{
			deps:        splitList(fields[1]),
			testImports: append(splitList(fields[2]), splitList(fields[3])...),
		}
	}

	dependents := make(map[string]map[string]struct{})
	for pkg, info := range pkgs {
		for reached := range reaches(pkgs, info) {
			if reached == pkg {
				continue
			}
			if _, inModule := pkgs[reached]; !inModule {
				continue
			}
			if _, ok := dependents[reached]; !ok {
				dependents[reached] = make(map[string]struct{})
			}
			dependents[reached][pkg] = struct{}{}
		}
	}

	g := &Graph{dependents: make(map[string][]string, len(dependents))}
	for pkg, set := range dependents {
		list := make([]string, 0, len(set))
		for d := range set {
			list = append(list, d)
		}
		sort.Strings(list)
		g.dependents[pkg] = list
	}

	return g
}

// reaches returns everything a package depends on, through its own code or
// through its tests. A test import brings its own transitive dependencies with
// it, which is why the import's Deps are folded in rather than the import alone.
func reaches(pkgs map[string]pkgInfo, info pkgInfo) map[string]struct{} {
	reached := make(map[string]struct{}, len(info.deps))
	for _, d := range info.deps {
		reached[d] = struct{}{}
	}
	for _, ti := range info.testImports {
		reached[ti] = struct{}{}
		for _, d := range pkgs[ti].deps {
			reached[d] = struct{}{}
		}
	}

	return reached
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}

	return strings.Split(s, ",")
}
