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
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fingerprint is what a package's source looked like when its map was made.
//
// It exists because the build ID is too coarse to invalidate a map with. Every
// test of a package shares one test binary, so one changed line moves the build
// ID and dirties all of the package's mappings — and the package under mutation
// is by definition the package that was changed, so that is the case that
// always happens rather than the rare one.
//
// It is the package's own print (see pkgPrint) — every entity of the package
// and its test files, and the remainder hashed whole — plus one print per
// instrumented dependency, and Inputs for everything else the binary is built
// from. reusable reads two of them to decide which mappings a change reached.
type fingerprint struct {
	pkgPrint

	// Inputs is everything the test binary is built from except this package
	// and the dependencies in Deps: see buildInputsOf, which fills it in.
	// Without it a moved build ID could never be told apart from a moved
	// dependency, and narrowing would keep stale mappings. With it, a moved
	// build ID under an unchanged fingerprint (Deps included) and unchanged
	// Inputs is a moved checkout, and the whole map is kept.
	//
	// It is not read off the filesystem like the rest, so fingerprintOf leaves
	// it empty and the caller sets it — which is also what makes a fingerprint
	// with no inputs unusable rather than optimistic.
	Inputs string `json:"inputs"`

	// Deps is every dependency the test binary instruments besides this
	// package, by import path: the packages of every main module it links (see
	// dependencyDirs). Their lines are recorded per test, as the entities it
	// executed there, so a dependency is printed like the package rather than
	// hashed into Inputs — which is what lets a change in one dirty only the
	// tests it can reach. A dependency's test files are not part of its print:
	// they are never linked into this package's test binary.
	Deps map[string]pkgPrint `json:"deps,omitempty"`
}

// goFile is a package file as the fingerprint reads it: its bytes, and its
// syntax when it has any.
type goFile struct {
	fset *token.FileSet
	ast  *ast.File
	name string
	data []byte
}

// fingerprintOf reads a package's directory and records its shape.
//
// The whole directory is read rather than the file list `go list` reports, so
// that nothing a change could hide in is left out: a C file, an embedded
// asset, a file excluded by a build tag. Anything unreadable is a failure to
// fingerprint, and anything unparseable is folded into Whole — both cost a
// re-map of the package and neither can give a wrong answer.
func (c *Coverage) fingerprintOf(pkg *testPackage) (fingerprint, bool) {
	files, ok := readPackageFiles(pkg.dir)
	if !ok {
		return fingerprint{}, false
	}
	below, ok := hashDataSubtrees(pkg.dir)
	if !ok {
		return fingerprint{}, false
	}

	printed := c.printPackage(pkg.importPath, files, true, below, c.typedFacts(pkg.importPath, true))

	return fingerprint{pkgPrint: printed}, true
}

// hashDataSubtrees hashes what a package's directory holds below its top level
// and does not compile: testdata, and the trees an //go:embed pattern reaches
// into.
//
// They belong in Whole because a test can behave differently on new input
// without a line of the package changing, which would leave a kept mapping
// describing a path the test no longer takes. The build ID does see embedded
// files, so on its own that case ends in a whole re-map — but a run that has
// already found a reason to narrow would never get that far.
//
// Subdirectories holding Go files are skipped: those are packages of their own,
// and a package this one imports is covered by what it is built from, while one
// it does not import has no business dirtying it.
func hashDataSubtrees(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		sub := filepath.Join(dir, name)
		isPkg, ok := holdsGoFiles(sub)
		if !ok {
			return "", false
		}
		if isPkg {
			continue
		}
		sum, subOK := hashDirectory(sub)
		if !subOK {
			return "", false
		}
		nested, nestedOK := hashDataSubtrees(sub)
		if !nestedOK {
			return "", false
		}
		parts = append(parts, name+"\x00"+sum+"\x00"+nested)
	}

	return hashOf([]byte(strings.Join(parts, "\x00"))), true
}

func holdsGoFiles(dir string) (bool, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, false
	}
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".go") {
			return true, true
		}
	}

	return false, true
}

// readPackageFiles reads every regular file of a package directory, parsing the
// Go ones. A file that does not parse keeps a nil syntax tree, which is what
// puts the whole of it into Whole.
func readPackageFiles(dir string) ([]goFile, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	files := make([]goFile, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: the directory is one `go list` reported
		if err != nil {
			return nil, false
		}
		f := goFile{name: name, data: data}
		if strings.HasSuffix(name, ".go") {
			f.fset = token.NewFileSet()
			if parsed, perr := parser.ParseFile(f.fset, name, data, parser.ParseComments); perr == nil {
				f.ast = parsed
			}
		}
		files = append(files, f)
	}

	return files, true
}

// declKey names a declaration within its file: methods of different types share
// a name, and the receiver is what tells them apart.
func declKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	var recv strings.Builder
	ast.Inspect(fn.Recv.List[0].Type, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			recv.WriteString(id.Name)
		}

		return true
	})

	return recv.String() + "." + fn.Name.Name
}

func isTestFuncName(name string) bool {
	for _, prefix := range testFuncPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// profileFileName is the name a coverage profile gives one of a package's
// files. It mirrors removeModuleFromPath, which does the same translation from
// the other direction, and is module-relative for the same reason.
func (c *Coverage) profileFileName(importPath, base string) string {
	return strings.ReplaceAll(importPath+"/"+base, c.mod.Name+"/", "")
}
