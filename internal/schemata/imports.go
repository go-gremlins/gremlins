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
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// A constant form replaces a constant with its value, which can remove a
// file's last use of an import: the rendered file then fails "imported and
// not used" at the import, outside every site, which would drop the whole
// package. Such an import is rewritten in place to the blank identifier --
// `"math"` or `m "math"` becomes `_ "math"` -- which keeps the package's
// init side effects, all the original's import still guarantees, and the
// file's lines. Imports are file-scoped, and so is the error: what other
// files of the package import or use plays no part. Only an import every
// use of which, in the original file, lies inside a site placed with a
// constant form is repaired: that form drops the constant's text, which has
// no effect to lose. Any other lost use -- a form that dropped an operand --
// stays a type error.

// importKey identifies an import spec of a file by its name, empty when it
// has none, and its path literal, as written: a file can import one path
// once under each name.
func importKey(s *ast.ImportSpec) string {
	name := ""
	if s.Name != nil {
		name = s.Name.Name
	}

	return name + " " + s.Path.Value
}

// fileImports parses the import specs of the Go source src.
func fileImports(src []byte) (*token.File, []*ast.ImportSpec, bool) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ImportsOnly)
	if err != nil {
		return nil, nil, false
	}

	return fset.File(f.Pos()), f.Imports, true
}

// unusedImports splits errs into the imports of rendered files they report
// unused -- by file, by importKey -- and the other errors. An error counts
// only if it is the type checker's unused-import error at the position of
// an import spec of a file in rendered, as the type checker reports it: at
// the spec's name if it has one, else at its path. A blank import is never
// unused, and cgo's "C" is left alone.
func unusedImports(errs []typeError, overlay map[string][]byte, rendered map[string]bool, repairable func(file, key string) bool) (map[string]map[string]bool, []typeError) {
	found := map[string]map[string]bool{}
	var rest []typeError
	for _, e := range errs {
		if key, ok := unusedImport(e, overlay, rendered); ok && repairable(e.file, key) {
			if found[e.file] == nil {
				found[e.file] = map[string]bool{}
			}
			found[e.file][key] = true

			continue
		}
		rest = append(rest, e)
	}

	return found, rest
}

// unusedImport returns the importKey of the import spec e reports unused.
func unusedImport(e typeError, overlay map[string][]byte, rendered map[string]bool) (string, bool) {
	if !rendered[e.file] || !strings.Contains(e.msg, " imported") || !strings.HasSuffix(e.msg, " and not used") {
		return "", false
	}
	file, specs, ok := fileImports(overlay[e.file])
	if !ok {
		return "", false
	}
	for _, s := range specs {
		if file.Offset(s.Pos()) != e.offset {
			continue
		}
		if s.Name != nil && s.Name.Name == "_" || s.Path.Value == strconv.Quote("C") {
			return "", false
		}

		return importKey(s), true
	}

	return "", false
}

// blankImports rewrites each import spec of the rendered file out whose
// importKey is in keys to the blank identifier, and moves the spans after
// each edit by its change in length. The name is replaced, or "_ " put
// before the path: nothing else on the line moves to another.
func blankImports(out []byte, spans []renderedSpan, keys map[string]bool) ([]byte, []renderedSpan) {
	if len(keys) == 0 {
		return out, spans
	}
	file, specs, ok := fileImports(out)
	if !ok {
		return out, spans
	}
	moved := append([]renderedSpan(nil), spans...)
	// From the last spec back, so each edit's offsets hold.
	for i := len(specs) - 1; i >= 0; i-- {
		s := specs[i]
		if !keys[importKey(s)] {
			continue
		}
		start, end, repl := file.Offset(s.Path.Pos()), file.Offset(s.Path.Pos()), "_ "
		if s.Name != nil {
			start, end, repl = file.Offset(s.Name.Pos()), file.Offset(s.Name.End()), "_"
		}
		out = append(out[:start:start], append([]byte(repl), out[end:]...)...)
		delta := len(repl) - (end - start)
		for j := range moved {
			if moved[j].start >= start {
				moved[j].start += delta
				moved[j].end += delta
			}
		}
	}

	return out, moved
}

// learn adds the imports of unused to blank, reporting whether any was not
// there yet: one that was is still reported unused although blanked, which
// another blank would not mend.
func learn(blank, unused map[string]map[string]bool) bool {
	added := false
	for path, keys := range unused {
		if blank[path] == nil {
			blank[path] = map[string]bool{}
		}
		for k := range keys {
			added = added || !blank[path][k]
			blank[path][k] = true
		}
	}

	return added
}

// constantUsesOnly reports whether every use, in the original syntax syn of
// a file, of its import whose importKey is key lies inside one of sites
// that is placed with a constant form: a constant-valued expression, or a
// constant group. A use of a dot import is a use of any package-level object
// of the imported package. An import the type information does not hold is
// not repairable.
func constantUsesOnly(info *types.Info, syn *ast.File, sites []Site, key string) bool {
	var pn *types.PkgName
	for _, spec := range syn.Imports {
		if importKey(spec) != key {
			continue
		}
		var obj types.Object
		if spec.Name != nil {
			obj = info.Defs[spec.Name]
		} else {
			obj = info.Implicits[spec]
		}
		pn, _ = obj.(*types.PkgName)
	}
	if pn == nil {
		return false
	}
	dot := pn.Name() == "."
	for id, obj := range info.Uses {
		if id.Pos() < syn.FileStart || id.Pos() > syn.FileEnd {
			continue
		}
		used := obj == pn
		if dot && obj.Pkg() == pn.Imported() && obj.Parent() == pn.Imported().Scope() {
			used = true
		}
		if used && !slices.ContainsFunc(sites, func(s Site) bool {
			return constantSite(info, s) && s.Node.Pos() <= id.Pos() && id.End() <= s.Node.End()
		}) {
			return false
		}
	}

	return true
}

// constantSite reports whether s is placed with a constant form.
func constantSite(info *types.Info, s Site) bool {
	if len(s.Members) > 0 {
		return true
	}
	e, ok := s.Node.(ast.Expr)

	return ok && info.Types[e].Value != nil
}

// repairableIn returns, for the package's files as rendered this round,
// whether the import key of the file at path is repairable: every use of it
// lies inside a site of the file placed with a constant form.
func repairableIn(pkg *packages.Package, files []*sourceFile) func(path, key string) bool {
	return func(path, key string) bool {
		for _, f := range files {
			if f.path != path {
				continue
			}
			for _, syn := range pkg.Syntax {
				if pkg.Fset.File(syn.Pos()) == f.file {
					return constantUsesOnly(pkg.TypesInfo, syn, f.sites, key)
				}
			}
		}

		return false
	}
}
