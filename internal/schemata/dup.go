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
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"
)

// A compile-time site -- in a const declaration, an array length or a
// constant composite literal key -- cannot switch at run time: its value is
// folded, and may be needed, by the compiler. Inside a function
// declaration's body its scope is that one function, so each of its mutants
// k is placed by a copy of the function with the plain mutant's operator,
// named prefix_D<k>_<name> and appended after the file's last line, and a
// jump into it spliced after the original's opening brace:
//
//	func F(x int) int {if _gremlinsActive == 3 { _gremlinsReached(); return _gremlins_D3_F(x) }; ...
//
// While mutant k is active, code inside the copy runs in one extra stack
// frame, under the copy's name; nothing else differs.

// dupError is what a rewriter returns, as its error, for a compile-time
// site in the body of the function declaration fn: the site keeps its text
// and render places each of its mutants by duplicating fn. It wraps
// ErrUnsupported, so that a caller that does not duplicate refuses the site.
type dupError struct {
	fn     *ast.FuncDecl
	prefix string
	// mutated maps each of the site's mutant ids to the plain mutant's
	// operator.
	mutated map[int]token.Token
}

func (d *dupError) Error() string {
	return fmt.Sprintf("%v: compile-time site, placed by duplicating %s", ErrUnsupported, d.fn.Name.Name)
}

func (d *dupError) Unwrap() error { return ErrUnsupported }

// dupSite returns the request to duplicate fn for the compile-time site s,
// whose node is e, or refuses it: in init, which nothing can call; in a
// function with a //go: directive other than //go:noinline, which a copy
// would lose; with a blank type parameter, which the jump cannot forward;
// and a mutant the engine does not rewrite e's operator for.
func (r *rewriter) dupSite(s Site, e ast.Expr, fn *ast.FuncDecl) error {
	if fn.Recv == nil && fn.Name.Name == "init" {
		return fmt.Errorf("%w: compile-time site in init", ErrUnsupported)
	}
	if fn.Doc != nil {
		for _, c := range fn.Doc.List {
			if d, ok := strings.CutPrefix(c.Text, "//go:"); ok && strings.TrimSpace(d) != "noinline" {
				return fmt.Errorf("%w: compile-time site in a function with the directive %s", ErrUnsupported, c.Text)
			}
		}
	}
	if r.callsRecover(fn.Body) {
		return fmt.Errorf("%w: compile-time site in a function that calls recover", ErrUnsupported)
	}
	if fn.Type.TypeParams != nil {
		for _, f := range fn.Type.TypeParams.List {
			for _, n := range f.Names {
				if n.Name == "_" {
					return fmt.Errorf("%w: compile-time site in a function with a blank type parameter", ErrUnsupported)
				}
			}
		}
	}
	var op token.Token
	switch e := e.(type) {
	case *ast.BinaryExpr:
		op = e.Op
	case *ast.UnaryExpr:
		op = e.Op
	}
	d := &dupError{fn: fn, prefix: r.prefix, mutated: map[int]token.Token{}}
	for _, m := range s.Muts {
		to, ok := constMutation(m.Type, op)
		if !ok {
			return fmt.Errorf("%w: no %s mutation of %s", ErrUnsupported, m.Type, op)
		}
		d.mutated[m.ID] = to
	}

	return d
}

// callsRecover reports whether body calls the builtin recover outside any
// nested function literal. Such a function may be deferred itself, and
// recover stops a panic only when called directly by the deferred function:
// through the jump, the copy's recover would be a frame too deep and return
// nil. A recover inside a closure the function defers is unaffected. Without
// Uses to resolve the name, any call of a function named recover counts.
func (r *rewriter) callsRecover(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			id, ok := ast.Unparen(n.Fun).(*ast.Ident)
			if ok && id.Name == "recover" && (r.info.Uses == nil || r.info.Uses[id] == types.Universe.Lookup("recover")) {
				found = true
			}
		}

		return !found
	})

	return found
}

// textEdit replaces del bytes at off with text.
type textEdit struct {
	off, del int
	text     string
}

// applyEdits returns src with edits applied. The edits must not overlap.
func applyEdits(src []byte, edits []textEdit) ([]byte, error) {
	edits = slices.Clone(edits)
	slices.SortStableFunc(edits, func(a, b textEdit) int { return a.off - b.off })
	var b bytes.Buffer
	pos := 0
	for _, e := range edits {
		if e.off < pos || e.off+e.del > len(src) {
			return nil, fmt.Errorf("%w: overlapping duplication edits", ErrUnsupported)
		}
		b.Write(src[pos:e.off])
		b.WriteString(e.text)
		pos = e.off + e.del
	}
	b.Write(src[pos:])

	return b.Bytes(), nil
}

// signature is how a duplicate's jump calls it: the receiver and argument
// names, after naming the unnamed and blank ones, and the edits to fn's
// signature that name them.
type signature struct {
	recv  string
	args  []string
	edits []textEdit
}

// paramNames names fn's receiver and parameters. An unnamed or blank one
// is named prefix_a<i>, i its index counting the receiver first; the same
// edits apply to the original and the copy, on the same line, so the
// renaming changes nothing the function's callers or body can see.
func paramNames(file *token.File, fn *ast.FuncDecl, prefix string) signature {
	var sig signature
	i := 0
	name := func(f *ast.Field) []string {
		var out []string
		if len(f.Names) == 0 {
			n := prefix + "_a" + strconv.Itoa(i)
			i++
			sig.edits = append(sig.edits, textEdit{off: file.Offset(f.Type.Pos()), text: n + " "})

			return []string{n}
		}
		for _, id := range f.Names {
			n := id.Name
			if n == "_" {
				n = prefix + "_a" + strconv.Itoa(i)
				sig.edits = append(sig.edits, textEdit{off: file.Offset(id.Pos()), del: 1, text: n})
			}
			i++
			out = append(out, n)
		}

		return out
	}
	if fn.Recv != nil {
		for _, f := range fn.Recv.List {
			sig.recv = name(f)[0]
		}
	}
	for _, f := range fn.Type.Params.List {
		names := name(f)
		if _, variadic := f.Type.(*ast.Ellipsis); variadic {
			names[len(names)-1] += "..."
		}
		sig.args = append(sig.args, names...)
	}

	return sig
}

// dupName is the name of fn's duplicate for mutant id.
func dupName(prefix string, id int, fn *ast.FuncDecl) string {
	return prefix + "_D" + strconv.Itoa(id) + "_" + fn.Name.Name
}

// duplicate returns, for the one mutant of s -- a compile-time site in fn's
// body, in the file src holds -- the jump to splice after fn's opening brace
// and the duplicate to append after the file's last line: fn's text with
// s's operator replaced by mutated, renamed, its unnamed and blank
// parameters named, after a line directive that gives it fn's position. The
// jump calls Reached, so the mutant is reached exactly when fn runs. It
// refuses a file with line directives of its own, which the copy's would
// interleave with, and a site whose source text is not its operator.
func duplicate(fset *token.FileSet, src []byte, fn *ast.FuncDecl, s Site, mutated token.Token, prefix string) (string, string, error) {
	file := fset.File(fn.Pos())
	if len(s.Muts) != 1 || file == nil || file.Size() != len(src) || fn.Body == nil {
		return "", "", fmt.Errorf("%w: duplication needs one mutant and the source of %s", ErrUnsupported, fn.Name.Name)
	}
	if bytes.HasPrefix(src, []byte("//line ")) || bytes.Contains(src, []byte("\n//line ")) || bytes.Contains(src, []byte("/*line ")) {
		return "", "", fmt.Errorf("%w: compile-time site in a file with line directives", ErrUnsupported)
	}
	var opPos token.Pos
	switch n := s.Node.(type) {
	case *ast.BinaryExpr:
		opPos = n.OpPos
	case *ast.UnaryExpr:
		opPos = n.OpPos
	}
	if opPos <= fn.Body.Lbrace || opPos >= fn.Body.Rbrace {
		return "", "", fmt.Errorf("%w: compile-time site not in the body of %s", ErrUnsupported, fn.Name.Name)
	}
	op := file.Offset(opPos)
	if !bytes.HasPrefix(src[op:], []byte(s.Tok.String())) {
		return "", "", fmt.Errorf("%w: source at the site is not its operator %s", ErrUnsupported, s.Tok)
	}

	id := s.Muts[0].ID
	name := dupName(prefix, id, fn)
	sig := paramNames(file, fn, prefix)
	start := file.Offset(fn.Pos())
	edits := append(slices.Clone(sig.edits),
		textEdit{off: file.Offset(fn.Name.Pos()), del: len(fn.Name.Name), text: name},
		textEdit{off: op, del: len(s.Tok.String()), text: mutated.String()})
	for i := range edits {
		edits[i].off -= start
	}
	text, err := applyEdits(src[start:file.Offset(fn.End())], edits)
	if err != nil {
		return "", "", err
	}
	at := fset.PositionFor(fn.Pos(), false)
	copyText := fmt.Sprintf("//line :%d:%d\n%s", at.Line, at.Column, text)

	return jump(prefix, id, name, fn, sig), copyText, nil
}

// jump is the statement that runs fn's duplicate name while mutant id is
// active, on one line.
func jump(prefix string, id int, name string, fn *ast.FuncDecl, sig signature) string {
	call := name
	if sig.recv != "" {
		call = sig.recv + "." + name
	}
	if fn.Type.TypeParams != nil {
		var tps []string
		for _, f := range fn.Type.TypeParams.List {
			for _, n := range f.Names {
				tps = append(tps, n.Name)
			}
		}
		call += "[" + strings.Join(tps, ", ") + "]"
	}
	call += "(" + strings.Join(sig.args, ", ") + ")"
	body := "return " + call
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		body = call + "; return"
	}

	return fmt.Sprintf("if %sActive == %d { %sReached(); %s };", prefix, id, prefix, body)
}

// dupMutant is one mutant placed by a duplicate.
type dupMutant struct {
	site       Site // the site, with this one mutant
	fn         *ast.FuncDecl
	jump, copy string
}

// placeDups splices the duplicates the dup nodes of the forest roots ask
// for into out, the file src rendered from roots, whose placed sites lie at
// spans. It returns the file with each duplicated function's jumps and
// parameter names spliced in -- on their lines, so no line moves -- and its
// duplicates appended, the spans moved by the splices, and a span for each
// jump, renamed parameter and duplicate, attributed to its mutant. A site
// none of whose mutants can be duplicated is reported, keeping its text.
func placeDups(fset *token.FileSet, src, out []byte, roots []*siteNode, spans []renderedSpan) ([]byte, []renderedSpan, []SiteError) {
	var nodes []*siteNode
	var walk func([]*siteNode)
	walk = func(ns []*siteNode) {
		for _, n := range ns {
			if n.dup != nil {
				nodes = append(nodes, n)
			}
			walk(n.children)
		}
	}
	walk(roots)
	if len(nodes) == 0 {
		return out, spans, nil
	}

	var errs []SiteError
	var muts []dupMutant
	var fns []*ast.FuncDecl
	for _, n := range nodes {
		var made []dupMutant
		var err error
		for _, m := range n.site.Muts {
			one := Site{Node: n.site.Node, Tok: n.site.Tok, Muts: []Mutant{m}}
			var jmp, cp string
			jmp, cp, err = duplicate(fset, src, n.dup.fn, one, n.dup.mutated[m.ID], n.dup.prefix)
			if err != nil {
				break
			}
			made = append(made, dupMutant{site: one, fn: n.dup.fn, jump: jmp, copy: cp})
		}
		if err != nil {
			errs = append(errs, SiteError{Site: n.site, Err: err})

			continue
		}
		muts = append(muts, made...)
		if !slices.Contains(fns, n.dup.fn) {
			fns = append(fns, n.dup.fn)
		}
	}
	if len(muts) == 0 {
		return out, spans, errs
	}

	file := fset.File(nodes[0].site.Node.Pos())
	var edits []ownedEdit
	for _, fn := range fns {
		var jumps strings.Builder
		var owners []Site
		for _, m := range muts {
			if m.fn == fn {
				jumps.WriteString(m.jump)
				owners = append(owners, m.site)
			}
		}
		for _, e := range paramNames(file, fn, nodes[0].dup.prefix).edits {
			e.off = renderedOffset(roots, e.off)
			edits = append(edits, ownedEdit{e, owners})
		}
		off := renderedOffset(roots, file.Offset(fn.Body.Lbrace)+1)
		edits = append(edits, ownedEdit{textEdit{off: off, text: jumps.String()}, owners})
	}
	b, moved := spliceOwned(out, edits, spans)
	if !bytes.HasSuffix(b.Bytes(), []byte("\n")) {
		b.WriteByte('\n')
	}
	for _, m := range muts {
		b.WriteByte('\n')
		start := b.Len()
		b.WriteString(m.copy)
		moved = append(moved, renderedSpan{site: m.site, id: m.site.Muts[0].ID, start: start, end: b.Len()})
		b.WriteByte('\n')
	}

	return b.Bytes(), moved, errs
}

// ownedEdit is an edit and the mutants a type error in its text is put on.
type ownedEdit struct {
	textEdit
	owners []Site
}

// spliceOwned applies edits -- none inside a span -- to out, returning the
// result, spans moved by the edits before them, and a span per owner of
// each edit's text.
func spliceOwned(out []byte, edits []ownedEdit, spans []renderedSpan) (*bytes.Buffer, []renderedSpan) {
	edits = slices.Clone(edits)
	slices.SortStableFunc(edits, func(a, b ownedEdit) int { return a.off - b.off })
	sorted := slices.Clone(spans)
	slices.SortStableFunc(sorted, func(a, b renderedSpan) int { return a.start - b.start })
	var b bytes.Buffer
	var moved []renderedSpan
	pos, shift, si := 0, 0, 0
	for _, e := range edits {
		for ; si < len(sorted) && sorted[si].start < e.off; si++ {
			moved = append(moved, shifted(sorted[si], shift))
		}
		b.Write(out[pos:e.off])
		start := b.Len()
		b.WriteString(e.text)
		for _, o := range e.owners {
			moved = append(moved, renderedSpan{site: o, id: o.Muts[0].ID, start: start, end: b.Len()})
		}
		pos = e.off + e.del
		shift += len(e.text) - e.del
	}
	for ; si < len(sorted); si++ {
		moved = append(moved, shifted(sorted[si], shift))
	}
	b.Write(out[pos:])

	return &b, moved
}

// shifted is s moved by shift bytes.
func shifted(s renderedSpan, shift int) renderedSpan {
	s.start += shift
	s.end += shift

	return s
}

// renderedOffset maps off, an offset of the original file that lies in no
// site, to the file rendered from roots: text outside the sites is
// unchanged, so it moves by the length change of the sites before it.
func renderedOffset(roots []*siteNode, off int) int {
	out := off
	for _, r := range roots {
		if r.end > off {
			break
		}
		out += len(r.rendered) - (r.end - r.start)
	}

	return out
}
