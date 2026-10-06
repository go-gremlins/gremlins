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
	"errors"
	"go/ast"
	"go/token"
	"slices"
	"sort"
	"strings"
)

var (
	// errDuplicateSite reports a site whose range equals an earlier site's.
	errDuplicateSite = errors.New("schemata: duplicate site range")
	// errOverlap reports a site that partially overlaps another site.
	errOverlap = errors.New("schemata: site partially overlaps another site")
	// errBadRange reports a site whose node range lies outside the source.
	errBadRange = errors.New("schemata: site range outside source")
	// errForeignFile reports a site whose node is not in the file src holds.
	errForeignFile = errors.New("schemata: site is not in the rendered file")
	// errSourceSize reports a site not rendered because src is not the
	// length of the file it is said to hold.
	errSourceSize = errors.New("schemata: src length differs from the file size")
	// errNewlineChanged reports a rewrite whose text holds a different
	// number of newlines than the site's original text, which would move
	// every line after it. Only the count is compared: a site that ends on
	// the line it ended on is all the lines outside it need.
	errNewlineChanged = errors.New("schemata: rewrite changes the line count")
)

// Rewriter returns the replacement text for site s. inner(n) returns the
// source text of n with every site nested inside n already rendered. For a
// node outside s's own range it returns n's original text, no site rendered,
// and for a node with no valid range (nil) the empty string.
type Rewriter func(s Site, inner func(ast.Node) string) (string, error)

// SiteError is a site that was left as its original text, and why.
type SiteError struct {
	Site Site
	Err  error
}

// siteNode is a site in the containment forest.
type siteNode struct {
	site       Site
	start, end int
	children   []*siteNode
	rendered   string
	// fallback is set when the rewrite failed and rendered is the site's
	// original text, with its nested sites rendered.
	fallback bool
	// dup is set when the site keeps its text and its mutants are placed by
	// duplicating its function; fallback is then set too.
	dup *dupError
}

// Render returns src, the bytes of file, with every site replaced by its
// rewrite. Edits are spliced into the original bytes: text outside the sites
// is byte-identical and every line outside a site keeps its number, since a
// rewrite holds as many newlines as its site (a site's own lines can hold
// other text when its rewrite moves an operand across a newline). Sites nested in another
// site are rendered first and reach the outer rewrite through inner. A site
// whose rewrite fails keeps its original text (with its nested sites still
// rendered) and is reported as a SiteError, as is every site that does not
// lie in file. If src is not file.Size() bytes, every site is reported and
// src is returned unchanged.
//
// A constant group's rewrite can place some of its mutants and refuse the
// others: each refused mutant is reported as a SiteError of the group
// holding that one mutant, and the rewrite is spliced in for the rest.
//
// A site whose rewrite asks for duplication -- a compile-time site in a
// function body, see NewRewriter -- keeps its text too, and is placed: the
// function's duplicates are appended after the last line, and a jump into
// each, and names for its unnamed parameters, are spliced into the
// function's signature line and opening-brace line, so that still no line
// moves.
//
// Caller contract: sites must come from fset, and the sites inside a constant
// expression must have been grouped by GroupConstantSites, as RewritePackage
// does; ungrouped, such a site is refused as an operand of a constant
// expression. A token.Pos is a plain offset
// into fset, so a node from a different FileSet whose position happens to
// fall inside file cannot be detected and is spliced as if it were file's.
func Render(fset *token.FileSet, file *token.File, src []byte, sites []Site, rw Rewriter) ([]byte, []SiteError) {
	out, _, errs := render(fset, file, src, sites, rw)

	return out, errs
}

// render is Render, also returning the rendered byte range of every placed
// site, and of each duplicate's text, attributed to its mutant.
func render(fset *token.FileSet, file *token.File, src []byte, sites []Site, rw Rewriter) ([]byte, []renderedSpan, []SiteError) {
	if file == nil || file.Size() != len(src) {
		errs := make([]SiteError, 0, len(sites))
		for _, s := range sites {
			errs = append(errs, SiteError{Site: s, Err: errSourceSize})
		}

		return src, nil, errs
	}
	roots, errs := buildForest(fset, file, sites)
	for _, r := range roots {
		errs = renderNode(fset, file, src, r, rw, errs)
	}
	out, spans, dupErrs := placeDups(fset, src, []byte(splice(src, 0, len(src), roots)), roots, layoutSpans(roots))

	return out, spans, append(errs, dupErrs...)
}

// buildForest nests each site under the smallest site containing it.
func buildForest(fset *token.FileSet, file *token.File, sites []Site) ([]*siteNode, []SiteError) {
	var errs []SiteError
	nodes := make([]*siteNode, 0, len(sites))
	for _, s := range sites {
		start, end, err := nodeRange(fset, file, s.Node)
		if err != nil {
			errs = append(errs, SiteError{Site: s, Err: err})

			continue
		}
		nodes = append(nodes, &siteNode{site: s, start: start, end: end})
	}
	// Outer sites before the sites they contain; stable keeps caller order
	// among identical ranges so the later duplicate is the one reported.
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].start != nodes[j].start {
			return nodes[i].start < nodes[j].start
		}

		return nodes[i].end > nodes[j].end
	})

	var roots, stack []*siteNode
	for _, n := range nodes {
		for len(stack) > 0 && stack[len(stack)-1].end <= n.start {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
			stack = append(stack, n)

			continue
		}
		top := stack[len(stack)-1]
		switch {
		case top.start == n.start && top.end == n.end:
			errs = append(errs, SiteError{Site: n.site, Err: errDuplicateSite})
		case n.end > top.end:
			errs = append(errs, SiteError{Site: n.site, Err: errOverlap})
		default:
			top.children = append(top.children, n)
			stack = append(stack, n)
		}
	}

	return roots, errs
}

// renderNode renders n's children, then n itself, into n.rendered.
func renderNode(fset *token.FileSet, file *token.File, src []byte, n *siteNode, rw Rewriter, errs []SiteError) []SiteError {
	for _, c := range n.children {
		errs = renderNode(fset, file, src, c, rw, errs)
	}
	inner := func(x ast.Node) string {
		start, end, err := nodeRange(fset, file, x)
		if err != nil {
			return ""
		}

		return splice(src, start, end, n.children)
	}
	original := splice(src, n.start, n.end, n.children)
	text, err := rw(n.site, inner)
	var dup *dupError
	var some *refusedMutantsError
	switch {
	case errors.As(err, &some) && !some.all && strings.Count(text, "\n") != strings.Count(string(src[n.start:n.end]), "\n"):
		errs = append(errs, SiteError{Site: n.site, Err: errNewlineChanged})
		text = original
		n.fallback = true
	case errors.As(err, &some) && some.all:
		errs = append(errs, some.refused...)
		text = original
		n.fallback = true
	case errors.As(err, &some):
		// The rewrite places the group's other mutants.
		errs = append(errs, some.refused...)
		n.site.Muts = withoutMutants(n.site.Muts, some.refused)
	case errors.As(err, &dup):
		text = original
		n.fallback = true
		n.dup = dup
	case err != nil:
		errs = append(errs, SiteError{Site: n.site, Err: err})
		text = original
		n.fallback = true
	case strings.Count(text, "\n") != strings.Count(string(src[n.start:n.end]), "\n"):
		errs = append(errs, SiteError{Site: n.site, Err: errNewlineChanged})
		text = original
		n.fallback = true
	}
	n.rendered = text

	return errs
}

// withoutMutants is muts without the mutants of the sites in refused.
func withoutMutants(muts []Mutant, refused []SiteError) []Mutant {
	gone := map[int]bool{}
	for _, e := range refused {
		for _, m := range e.Site.Muts {
			gone[m.ID] = true
		}
	}

	return slices.DeleteFunc(slices.Clone(muts), func(m Mutant) bool { return gone[m.ID] })
}

// splice returns src[start:end] with every site in sites (sorted, disjoint)
// that lies within the range replaced by its rendered text. A range that lies
// strictly inside a site is spliced from that site's children instead.
func splice(src []byte, start, end int, sites []*siteNode) string {
	var b strings.Builder
	pos := start
	for _, s := range sites {
		switch {
		case s.end <= start || s.start >= end:
			continue
		case s.start >= start && s.end <= end:
			b.Write(src[pos:s.start])
			b.WriteString(s.rendered)
			pos = s.end
		case s.start <= start && s.end >= end:
			return splice(src, start, end, s.children)
		}
	}
	b.Write(src[pos:end])

	return b.String()
}

// nodeRange returns n's byte range within file, unadjusted by //line
// directives. A node whose start or end is not in file is foreign.
func nodeRange(fset *token.FileSet, file *token.File, n ast.Node) (int, int, error) {
	if n == nil || !n.Pos().IsValid() || !n.End().IsValid() || n.End() < n.Pos() {
		return 0, 0, errBadRange
	}
	if fset.File(n.Pos()) != file || int(n.End()) > file.Base()+file.Size() {
		return 0, 0, errForeignFile
	}

	return file.Offset(n.Pos()), file.Offset(n.End()), nil
}
