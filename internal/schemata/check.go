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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"go/version"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

var (
	// ErrOldGoVersion drops every site of a module whose go.mod go line is
	// below 1.21: the helpers need cmp and the generic inference rules of
	// that version.
	ErrOldGoVersion = errors.New("module go version < 1.21")
	// ErrUnattributable drops every site of a package whose rewrite has a
	// type error that lies in no placed site.
	ErrUnattributable = errors.New("unattributable type error")
	// ErrTypeCheck drops a site whose rewrite has a type error in it.
	ErrTypeCheck = errors.New("schemata: rewrite does not type-check")
	// errNoPackageFiles drops every site of a package with no directory or
	// no syntax to render.
	errNoPackageFiles = errors.New("schemata: package has no directory or syntax")
	// errNotInPackageDir drops a site in a file outside the package's
	// directory, such as a cgo-generated file.
	errNotInPackageDir = errors.New("schemata: site is not in a file of the package directory")
	// errNoConvergence drops the sites still placed when the type-check
	// rounds run out (unreachable: every round drops a site).
	errNoConvergence = errors.New("schemata: type-check rounds exhausted")
)

// minGoVersion is the lowest module go version the helpers compile under.
const minGoVersion = "go1.21"

// rewriterFactory makes the Rewriter for one rendering round of a package.
type rewriterFactory func(info *types.Info, sizes types.Sizes, files []*ast.File, prefix string, h *HelperSet) Rewriter

// HelperFileName is the name of the helper file generated into a package
// whose identifier prefix is prefix.
func HelperFileName(prefix string) string {
	return "zz_" + prefix + "_schema.go"
}

// RewritePackage renders every file of pkg holding one of sites, adds the
// helper file, and proves the result type-checks -- the package's tests
// included -- with tags, before anything is written: files maps absolute
// paths in pkg's directory to their new content, for an overlay or a copy.
//
// pkg must be loaded with at least NeedTypes, NeedTypesInfo and NeedSyntax --
// and NeedTypesSizes, or constants are held to the host's integer sizes --
// and sites must come from pkg.Fset. Each type error is attributed to the
// innermost placed site whose rendered text contains it; those sites are
// dropped and the package rendered again from its original source, until it
// type-checks. An error in a function duplicate, or in what its jump adds to
// the original function, drops only the mutant the duplicate is for. A type
// error inside no placed site drops every site of the package with
// ErrUnattributable. Every input mutant ends in exactly one of placed and
// dropped: a site some of whose mutants are dropped is in both, each with its
// own. The sites inside one maximal constant expression are rewritten, and
// returned, as one constant group (see GroupConstantSites). When no site is
// placed, files is empty. The type-checks load packages under ctx; once it
// ends, every site still placed is dropped.
func RewritePackage(ctx context.Context, pkg *packages.Package, sites []Site, tags string) (map[string][]byte, []Site, []SiteError) {
	return rewritePackage(ctx, pkg, sites, tags, NewRewriter)
}

// sourceFile is one file of the package that holds a site.
type sourceFile struct {
	path  string
	file  *token.File
	src   []byte
	sites []Site
}

// renderedSpan is a placed site's byte range in its rendered file. id is 0
// for the site's own text, which every mutant of the site shares, or the
// one mutant the text is for: a duplicate, or what its jump adds to the
// original function.
type renderedSpan struct {
	site       Site
	id         int
	start, end int
}

// mutantKey is a site, by its node, and one of its mutants, or 0 for all.
type mutantKey struct {
	node ast.Node
	id   int
}

func rewritePackage(ctx context.Context, pkg *packages.Package, sites []Site, tags string, newRW rewriterFactory) (map[string][]byte, []Site, []SiteError) {
	if len(sites) == 0 {
		return nil, nil, nil
	}
	if pkg.Dir == "" || len(pkg.Syntax) == 0 || pkg.Fset == nil || pkg.TypesInfo == nil {
		return nil, nil, dropAll(nil, sites, errNoPackageFiles)
	}
	if err := checkGoVersion(pkg); err != nil {
		return nil, nil, dropAll(nil, sites, err)
	}
	prefix := ChoosePrefix(slices.Concat(pkg.Syntax, otherFiles(pkg)))
	helperPath := filepath.Join(pkg.Dir, HelperFileName(prefix))
	if _, err := os.Stat(helperPath); err == nil {
		return nil, nil, dropAll(nil, sites, fmt.Errorf("schemata: %s already exists", helperPath))
	}

	sites = GroupConstantSites(pkg.TypesInfo, pkg.Syntax, sites)
	files, dropped := groupSites(pkg, sites)
	// Every round drops a site or a mutant, and a site dropped for a type
	// error is restored at most once (see restoreChildren): twice the work.
	rounds := 0
	for _, s := range sites {
		rounds += 2 * max(len(s.Muts), 1)
	}
	rank := map[siteID]int{}
	for i, s := range sites {
		rank[idOf(s)] = i
	}
	var typeDrops []typeDrop
	retried := map[siteID]bool{}
	// blank holds, by file, the imports the rendered sites left unused (see
	// unusedImports). It is learned only in a round whose every error is
	// such an import, and forgotten when a round drops sites, any of which
	// may hold the import's last use.
	blank := map[string]map[string]bool{}
	for round := 0; round < rounds; round++ {
		h := &HelperSet{}
		rw := newRW(pkg.TypesInfo, pkg.TypesSizes, pkg.Syntax, prefix, h)
		overlay := map[string][]byte{}
		spans := map[string][]renderedSpan{}
		for _, f := range files {
			out, fileSpans, errs := render(pkg.Fset, f.file, f.src, f.sites, rw)
			out, fileSpans = blankImports(out, fileSpans, blank[f.path])
			for _, e := range errs {
				dropped = append(dropped, e)
				f.sites = removeSite(f.sites, e.Site)
			}
			if !bytes.Equal(out, f.src) {
				overlay[f.path] = out
				spans[f.path] = fileSpans
			}
		}
		if len(liveSites(files)) == 0 {
			return nil, nil, dropped
		}
		helper, err := h.File(pkg.Syntax[0].Name.Name, prefix)
		if err != nil {
			return nil, nil, dropAll(dropped, liveSites(files), fmt.Errorf("%w: %w", ErrUnattributable, err))
		}
		overlay[helperPath] = helper

		typeErrs, err := typeCheck(ctx, pkg.Dir, overlay, tags)
		if err != nil {
			return nil, nil, dropAll(dropped, liveSites(files), fmt.Errorf("%w: %w", ErrUnattributable, err))
		}
		if len(typeErrs) == 0 {
			return overlay, liveSites(files), dropped
		}
		rendered := map[string]bool{}
		for path := range spans {
			rendered[path] = true
		}
		unused, rest := unusedImports(typeErrs, overlay, rendered, repairableIn(pkg, files))
		if len(rest) == 0 && learn(blank, unused) {
			// learn adds an import at least each time, so this repeats at
			// most once per import, each time for a round more.
			rounds++

			continue
		}
		if len(rest) > 0 {
			// The unused imports are found again once this round's drops
			// are made, if they still are unused.
			typeErrs, blank = rest, map[string]map[string]bool{}
		}
		bad, unattributable := attribute(typeErrs, overlay, spans)
		if unattributable != "" {
			return nil, nil, dropAll(dropped, liveSites(files), fmt.Errorf("%w: %s", ErrUnattributable, unattributable))
		}
		var roundDrops []typeDrop
		for _, f := range files {
			kept := f.sites[:0:0]
			for _, s := range f.sites {
				if msg, ok := bad[mutantKey{s.Node, 0}]; ok {
					se := SiteError{Site: s, Err: fmt.Errorf("%w: %s", ErrTypeCheck, msg)}
					dropped = append(dropped, se)
					roundDrops = append(roundDrops, typeDrop{f: f, err: se})

					continue
				}
				// A duplicate's error drops its own mutant only.
				var live []Mutant
				for _, m := range s.Muts {
					if msg, ok := bad[mutantKey{s.Node, m.ID}]; ok {
						one := Site{Node: s.Node, Tok: s.Tok, Muts: []Mutant{m}}
						dropped = append(dropped, SiteError{Site: one, Err: fmt.Errorf("%w: %s", ErrTypeCheck, msg)})

						continue
					}
					live = append(live, m)
				}
				if len(live) > 0 {
					s.Muts = live
					kept = append(kept, s)
				}
			}
			f.sites = kept
		}
		typeDrops = append(typeDrops, roundDrops...)
		dropped, typeDrops = restoreChildren(roundDrops, dropped, typeDrops, retried, rank)
	}

	return nil, nil, dropAll(dropped, liveSites(files), errNoConvergence)
}

// siteID identifies a site by its node and token, as removeSite does.
type siteID struct {
	node ast.Node
	tok  token.Token
}

func idOf(s Site) siteID { return siteID{s.Node, s.Tok} }

// typeDrop is a site dropped for a type error, and the file it lies in.
type typeDrop struct {
	f   *sourceFile
	err SiteError
}

// restoreChildren gives each site in typeDrops that lies inside a site of
// roundDrops -- dropped this round for a type error -- one more round, once:
// an error is attributed to the innermost site whose text holds it, which for
// an inference error of the parent's can be a child of the parent. The
// parent is dropped in turn, and the child, never at fault, would otherwise
// stay dropped. A site that fails again after its retry stays dropped. It
// returns dropped and typeDrops without the restored sites.
func restoreChildren(roundDrops []typeDrop, dropped []SiteError, typeDrops []typeDrop, retried map[siteID]bool, rank map[siteID]int) ([]SiteError, []typeDrop) {
	for _, parent := range roundDrops {
		pn := parent.err.Site.Node
		for i := 0; i < len(typeDrops); i++ {
			c := typeDrops[i]
			cs := c.err.Site
			if cs.Node == pn || retried[idOf(cs)] || cs.Node.Pos() < pn.Pos() || cs.Node.End() > pn.End() {
				continue
			}
			retried[idOf(cs)] = true
			typeDrops = slices.Delete(typeDrops, i, i+1)
			i--
			dropped = slices.DeleteFunc(dropped, func(e SiteError) bool { return errors.Is(e.Err, c.err.Err) })
			c.f.sites = append(c.f.sites, cs)
			slices.SortStableFunc(c.f.sites, func(a, b Site) int { return rank[idOf(a)] - rank[idOf(b)] })
		}
	}

	return dropped, typeDrops
}

// dropAll appends every site in sites to dropped with err.
func dropAll(dropped []SiteError, sites []Site, err error) []SiteError {
	for _, s := range sites {
		dropped = append(dropped, SiteError{Site: s, Err: err})
	}

	return dropped
}

// liveSites lists the sites still placed, in file then input order.
func liveSites(files []*sourceFile) []Site {
	var out []Site
	for _, f := range files {
		out = append(out, f.sites...)
	}

	return out
}

// removeSite returns sites without the first site with s's node and token:
// one entry per SiteError, so that a site listed twice is accounted twice.
// Of sites listed twice, the one holding s's mutants is taken. When s holds
// only some of that site's mutants -- a constant group's refused mutants,
// the rest placed -- only those are removed from it.
func removeSite(sites []Site, s Site) []Site {
	gone := map[int]bool{}
	for _, m := range s.Muts {
		gone[m.ID] = true
	}
	same := func(x Site) bool { return x.Node == s.Node && x.Tok == s.Tok }
	i := slices.IndexFunc(sites, func(x Site) bool {
		held := 0
		for _, m := range x.Muts {
			if gone[m.ID] {
				held++
			}
		}

		return same(x) && held == len(gone)
	})
	if i < 0 {
		i = slices.IndexFunc(sites, same)
	}
	if i < 0 {
		return sites
	}
	kept := slices.DeleteFunc(slices.Clone(sites[i].Muts), func(m Mutant) bool { return gone[m.ID] })
	out := slices.Clone(sites)
	if len(s.Muts) == 0 || len(kept) == 0 {
		return slices.Delete(out, i, i+1)
	}
	out[i].Muts = kept
	var members []Site
	for _, x := range out[i].Members {
		x.Muts = slices.DeleteFunc(slices.Clone(x.Muts), func(m Mutant) bool { return gone[m.ID] })
		if len(x.Muts) > 0 {
			members = append(members, x)
		}
	}
	out[i].Members = members

	return out
}

// groupSites sorts sites into the package files they lie in, reading each
// file's original source. A site in no file of the package directory, or in
// a file that cannot be read, is dropped.
func groupSites(pkg *packages.Package, sites []Site) ([]*sourceFile, []SiteError) {
	var files []*sourceFile
	byFile := map[*token.File]*sourceFile{}
	var dropped []SiteError
	for _, s := range sites {
		var tf *token.File
		if s.Node != nil && s.Node.Pos().IsValid() {
			tf = pkg.Fset.File(s.Node.Pos())
		}
		if tf == nil || filepath.Dir(tf.Name()) != filepath.Clean(pkg.Dir) {
			dropped = append(dropped, SiteError{Site: s, Err: errNotInPackageDir})

			continue
		}
		f, ok := byFile[tf]
		if !ok {
			src, err := os.ReadFile(tf.Name())
			if err != nil {
				dropped = append(dropped, SiteError{Site: s, Err: err})

				continue
			}
			f = &sourceFile{path: tf.Name(), file: tf, src: src}
			byFile[tf] = f
			files = append(files, f)
		}
		f.sites = append(f.sites, s)
	}

	return files, dropped
}

// otherFiles parses the Go files in pkg's directory that are not in its
// syntax -- its tests, and files excluded by build constraints -- so that the
// prefix avoids their identifiers too: an internal test file shares the
// package scope. Over-inclusion only lengthens the prefix.
func otherFiles(pkg *packages.Package) []*ast.File {
	have := map[string]bool{}
	for _, f := range pkg.Syntax {
		have[pkg.Fset.File(f.Pos()).Name()] = true
	}
	paths, _ := filepath.Glob(filepath.Join(pkg.Dir, "*.go"))
	var out []*ast.File
	fset := token.NewFileSet()
	for _, p := range paths {
		if have[p] {
			continue
		}
		// A file that does not parse still yields the identifiers it has.
		if f, _ := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution); f != nil {
			out = append(out, f)
		}
	}

	return out
}

// checkGoVersion refuses a module whose go version is below minGoVersion.
func checkGoVersion(pkg *packages.Package) error {
	v := ""
	if pkg.Module != nil && pkg.Module.GoVersion != "" {
		v = pkg.Module.GoVersion
	} else {
		var err error
		if v, err = goModVersion(pkg.Dir); err != nil {
			return fmt.Errorf("%w: %w", ErrOldGoVersion, err)
		}
	}
	if gv := "go" + v; !version.IsValid(gv) || version.Compare(gv, minGoVersion) < 0 {
		return fmt.Errorf("%w (go %s)", ErrOldGoVersion, v)
	}

	return nil
}

// goModVersion returns the go line of the go.mod governing dir; a go.mod
// without one means go 1.16.
func goModVersion(dir string) (string, error) {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		data, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			sc := bufio.NewScanner(bytes.NewReader(data))
			for sc.Scan() {
				line, _, _ := strings.Cut(sc.Text(), "//")
				if f := strings.Fields(line); len(f) == 2 && f[0] == "go" {
					return f[1], nil
				}
			}

			return "1.16", nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
	}
}

// typeError is one error from the type-check of the overlaid package.
type typeError struct {
	file   string // absolute path, or "" when the error has no position
	offset int    // byte offset in file, -1 when unknown
	msg    string
}

// typeCheck loads the package in dir -- with its tests -- with overlay laid
// over its files, under ctx, and returns every error of the loaded packages.
func typeCheck(ctx context.Context, dir string, overlay map[string][]byte, tags string) ([]typeError, error) {
	cfg := &packages.Config{
		// NeedDeps type-checks the dependencies from source. Without it the
		// loader asks `go list -export` for their export data, which
		// compiles the overlaid package itself for its test variants and
		// reports its type errors again, without a position.
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax,
		Context: ctx,
		Dir:     dir,
		Tests:   true,
		Overlay: overlay,
	}
	if tags != "" {
		cfg.BuildFlags = []string{"-tags", tags}
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", dir, err)
	}
	var out []typeError
	for _, p := range pkgs {
		for _, e := range p.TypeErrors {
			pos := e.Fset.PositionFor(e.Pos, false)
			out = append(out, typeError{file: pos.Filename, offset: pos.Offset, msg: e.Error()})
		}
		for _, e := range p.Errors {
			if e.Kind == packages.TypeError {
				continue // in TypeErrors, with an exact position
			}
			file, offset := errorPos(e.Pos, overlay)
			out = append(out, typeError{file: file, offset: offset, msg: e.Error()})
		}
	}

	return out, nil
}

// errorPos converts a "file:line:col" position to a byte offset in the
// overlay content of file. It returns "" and -1 when it cannot.
func errorPos(pos string, overlay map[string][]byte) (string, int) {
	rest, colStr, ok1 := cutLast(pos, ":")
	file, lineStr, ok2 := cutLast(rest, ":")
	if !ok1 || !ok2 {
		// "file:line" without a column.
		file, lineStr, colStr = rest, colStr, "1"
	}
	line, err1 := strconv.Atoi(lineStr)
	col, err2 := strconv.Atoi(colStr)
	content, ok := overlay[file]
	if err1 != nil || err2 != nil || !ok || line < 1 || col < 1 {
		return "", -1
	}
	off := 0
	for l := 1; l < line; l++ {
		i := bytes.IndexByte(content[off:], '\n')
		if i < 0 {
			return "", -1
		}
		off += i + 1
	}

	return file, off + col - 1
}

func cutLast(s, sep string) (string, string, bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}

	return s[:i], s[i+len(sep):], true
}

// attribute maps each type error to the innermost placed site whose rendered
// span contains it -- to one mutant of it, in a span of a duplicate. It
// returns the sites and mutants to drop, with their messages, or the message
// of the first error that lies in no placed site.
func attribute(errs []typeError, overlay map[string][]byte, spans map[string][]renderedSpan) (map[mutantKey]string, string) {
	bad := map[mutantKey]string{}
	for _, e := range errs {
		if _, ok := overlay[e.file]; !ok || e.offset < 0 {
			return nil, e.msg
		}
		var inner *renderedSpan
		for i, s := range spans[e.file] {
			if s.start <= e.offset && e.offset < s.end && (inner == nil || s.end-s.start < inner.end-inner.start) {
				inner = &spans[e.file][i]
			}
		}
		if inner == nil {
			return nil, e.msg
		}
		k := mutantKey{inner.site.Node, inner.id}
		if _, seen := bad[k]; !seen {
			bad[k] = e.msg
		}
	}

	return bad, ""
}

// layoutSpans returns the rendered byte range of every placed site of a
// file rendered from roots. Text outside the sites is unchanged by Render, so
// a root, or a site nested in a site whose rewrite failed (and which kept its
// original text), lies at its original offset moved by the length change of
// the sites before it. A site nested in a rewritten site lies wherever the
// rewrite put its rendered text: the rewrite forms pass operands in their
// original order, so each child is found by searching the parent's text from
// the end of the previous child. A child that is not found gets no span, nor
// do its descendants, and an error in its text falls to the parent -- which
// is then dropped and the child checked again in the next round. In the other
// direction, an error of the parent's own making that lies in a child's text
// (an inference error reported at an argument) is blamed on the child, which
// is dropped and, once its parent is, tried again (restoreChildren). Either
// way a wrong guess costs a site or a round, never a broken build.
func layoutSpans(roots []*siteNode) []renderedSpan {
	var out []renderedSpan
	layoutChildren(roots, 0, 0, false, "", &out)

	return out
}

// layoutChildren places nodes, the children of a site rendered at base whose
// original text started at origStart. rewritten says the parent's text is a
// rewrite (text), not its original text with the children spliced in.
func layoutChildren(nodes []*siteNode, base, origStart int, rewritten bool, text string, out *[]renderedSpan) {
	delta, cursor := 0, 0
	for _, n := range nodes {
		var start int
		if rewritten {
			i := strings.Index(text[cursor:], n.rendered)
			if i < 0 {
				continue
			}
			start = base + cursor + i
			cursor += i + len(n.rendered)
		} else {
			start = base + n.start - origStart + delta
			delta += len(n.rendered) - (n.end - n.start)
		}
		if !n.fallback {
			*out = append(*out, renderedSpan{site: n.site, start: start, end: start + len(n.rendered)})
		}
		layoutChildren(n.children, start, n.start, !n.fallback, n.rendered, out)
	}
}
