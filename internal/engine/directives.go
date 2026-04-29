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

package engine

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

const directivePrefix = "//nomutant"

// directiveScope describes the set of mutator types a //nomutant directive
// suppresses. all=true means "every type"; otherwise types is the explicit
// allow-list parsed from the ":type1,type2,..." suffix.
type directiveScope struct {
	types map[mutator.Type]struct{}
	all   bool
}

func (s directiveScope) matches(mt mutator.Type) bool {
	if s.all {
		return true
	}
	_, ok := s.types[mt]

	return ok
}

// blockDirective binds a directiveScope to a byte-offset range
// [startOffset, endOffset]. Smaller ranges win over larger when they
// overlap. We store byte offsets (not token.Pos) so containment checks
// in isSuppressed need only the token.Position the caller already has.
type blockDirective struct {
	scope       directiveScope
	startOffset int
	endOffset   int
}

// directiveIndex resolves whether a (position, mutator type) pair is
// suppressed by an inline //nomutant directive. It is built once per file
// before the AST walk.
type directiveIndex struct {
	fileScope *directiveScope
	byLine    map[int]directiveScope
	blocks    []blockDirective
}

func (idx *directiveIndex) isSuppressed(pos token.Position, mt mutator.Type) bool {
	if idx == nil {
		return false
	}
	if idx.fileScope != nil && idx.fileScope.matches(mt) {
		return true
	}
	// Block scopes compose additively: if any enclosing block suppresses
	// the type, the mutant is suppressed. This lets an inner directive
	// add to (rather than replace) an outer one, which is the natural
	// reading of nested //nomutant comments.
	for _, b := range idx.blocks {
		if pos.Offset >= b.startOffset && pos.Offset <= b.endOffset && b.scope.matches(mt) {
			return true
		}
	}
	if s, ok := idx.byLine[pos.Line]; ok && s.matches(mt) {
		return true
	}

	return false
}

// buildDirectiveIndex scans file.Comments and decl-attached doc comments
// for //nomutant directives and classifies each by scope (file / block /
// end-of-line). Malformed directives are logged and ignored.
func buildDirectiveIndex(set *token.FileSet, file *ast.File) *directiveIndex {
	idx := &directiveIndex{byLine: map[int]directiveScope{}}

	tokenLines := collectTokenLines(set, file)
	packageLine := set.Position(file.Package).Line

	// All comment groups: the parser attaches some to file.Doc / decl.Doc
	// rather than leaving them in file.Comments, so we need both sources.
	groups := collectAllCommentGroups(file)

	for _, cg := range groups {
		for _, c := range cg.List {
			scope, ok := parseDirective(set, c)
			if !ok {
				continue
			}
			line := set.Position(c.Pos()).Line

			// File-scope: directive is on, or above, the package clause.
			// We treat any directive at or before the package line as file-scope.
			if line <= packageLine {
				fs := scope
				idx.fileScope = &fs

				continue
			}

			// End-of-line: directive shares its line with a non-comment token.
			if tokenLines[line] {
				idx.byLine[line] = scope

				continue
			}

			// Block-scope: directive is on its own line; bind it to the
			// smallest AST node starting on line+1.
			if node, found := largestNodeStartingAtLine(set, file, line+1); found {
				idx.blocks = append(idx.blocks, blockDirective{
					scope:       scope,
					startOffset: set.Position(node.Pos()).Offset,
					endOffset:   set.Position(node.End()).Offset,
				})

				continue
			}
			// Otherwise: no-op directive (no following AST node, no token on line).
		}
	}

	return idx
}

// parseDirective recognizes "//nomutant" and "//nomutant:t1,t2,..." comments.
// It returns ok=false if the comment is not a directive at all, and logs +
// returns ok=false if the directive is malformed (empty type list or all
// types unknown).
func parseDirective(set *token.FileSet, c *ast.Comment) (directiveScope, bool) {
	text := strings.TrimSpace(c.Text)
	if text == directivePrefix {
		return directiveScope{all: true}, true
	}
	if !strings.HasPrefix(text, directivePrefix+":") {
		return directiveScope{}, false
	}
	rest := strings.TrimPrefix(text, directivePrefix+":")
	rest = strings.TrimSpace(rest)
	pos := set.Position(c.Pos())
	if rest == "" {
		log.Errorf("ignoring malformed //nomutant directive at %s: empty type list\n", pos)

		return directiveScope{}, false
	}
	types := map[mutator.Type]struct{}{}
	for _, name := range strings.Split(rest, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		mt, ok := mutatorTypeByConfigKey(name)
		if !ok {
			log.Errorf("ignoring unknown mutator type %q in //nomutant directive at %s\n", name, pos)

			continue
		}
		types[mt] = struct{}{}
	}
	if len(types) == 0 {
		return directiveScope{}, false
	}

	return directiveScope{types: types}, true
}

// mutatorTypeByConfigKey maps a config-style mutator key (e.g.
// "arithmetic-base") to its mutator.Type. It uses the same derivation as
// configuration.MutantTypeEnabledKey: lowercase Type.String() with "_"→"-".
func mutatorTypeByConfigKey(name string) (mutator.Type, bool) {
	for _, mt := range mutator.Types {
		if configKeyForType(mt) == name {
			return mt, true
		}
	}

	return 0, false
}

func configKeyForType(mt mutator.Type) string {
	s := strings.ReplaceAll(mt.String(), "_", "-")

	return strings.ToLower(s)
}

// collectTokenLines returns the set of source lines that contain at least
// one non-comment AST node, used to distinguish end-of-line directives from
// own-line ones. Returning false on *ast.CommentGroup stops the walk before
// it reaches the *ast.Comment children, so no Comment guard is needed.
func collectTokenLines(set *token.FileSet, file *ast.File) map[int]bool {
	lines := map[int]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, isCG := n.(*ast.CommentGroup); isCG {
			return false
		}
		lines[set.Position(n.Pos()).Line] = true

		return true
	})

	return lines
}

// collectAllCommentGroups gathers every *ast.CommentGroup reachable from
// the file: free-floating ones in file.Comments, plus those attached as
// Doc fields on declarations and the file itself.
func collectAllCommentGroups(file *ast.File) []*ast.CommentGroup {
	groups := append([]*ast.CommentGroup(nil), file.Comments...)
	if file.Doc != nil {
		groups = append(groups, file.Doc)
	}
	// file.Comments already includes all comment groups in the file in
	// position order — including those the parser also referenced as
	// .Doc on decls. Dedup by pointer.
	seen := map[*ast.CommentGroup]bool{}
	out := groups[:0]
	for _, g := range groups {
		if g == nil || seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, g)
	}

	return out
}

// largestNodeStartingAtLine finds the AST node with the widest range
// (Pos..End) whose start position is on the given line. The directive
// attaches to "the AST node and everything inside it," so we want the
// enclosing decl/stmt (e.g. FuncDecl), not a child Ident at the same line.
func largestNodeStartingAtLine(set *token.FileSet, file *ast.File, line int) (ast.Node, bool) {
	var (
		best     ast.Node
		bestSpan token.Pos
	)
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, isCG := n.(*ast.CommentGroup); isCG {
			return false
		}
		if set.Position(n.Pos()).Line != line {
			return true
		}
		span := n.End() - n.Pos()
		if best == nil || span > bestSpan {
			best = n
			bestSpan = span
		}

		return true
	})

	return best, best != nil
}
