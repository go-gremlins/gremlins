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
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// binarySites returns one Site per binary expression in f whose operator is
// in ops, in source order.
func binarySites(f *ast.File, ops ...token.Token) []Site {
	var sites []Site
	ast.Inspect(f, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		for _, op := range ops {
			if be.Op == op {
				sites = append(sites, Site{Node: be, Tok: be.Op})
			}
		}

		return true
	})

	return sites
}

// wrap is the toy Rewriter: it wraps the site as W(<inner text>).
func wrap(s Site, inner func(ast.Node) string) (string, error) {
	return "W(" + inner(s.Node) + ")", nil
}

func TestRender(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		src     string
		ops     []token.Token
		dup     bool        // pass the first site twice
		rw      Rewriter    // nil means wrap
		want    string      // expected output
		wantErr []error     // expected SiteError causes, in order
		errTok  token.Token // token of the site each SiteError must name
	}{
		"single": {
			src:  "package p\nfunc f(a, b int) { x := a + b; _ = x }\n",
			ops:  []token.Token{token.ADD},
			want: "package p\nfunc f(a, b int) { x := W(a + b); _ = x }\n",
		},
		"nested_sites": {
			src:  "package p\nfunc f(a, b, c int) {\n\tif a+b < c {\n\t}\n}\n",
			ops:  []token.Token{token.ADD, token.LSS},
			want: "package p\nfunc f(a, b, c int) {\n\tif W(W(a+b) < c) {\n\t}\n}\n",
		},
		"siblings": {
			src:  "package p\nfunc f(a, b, c, d int) { g(a+b, c-d) }\n",
			ops:  []token.Token{token.ADD, token.SUB},
			want: "package p\nfunc f(a, b, c, d int) { g(W(a+b), W(c-d)) }\n",
		},
		"inner_on_operand": {
			src: "package p\nfunc f(a, b, c int) bool { return a+b < c }\n",
			ops: []token.Token{token.ADD, token.LSS},
			rw: func(s Site, inner func(ast.Node) string) (string, error) {
				be, ok := s.Node.(*ast.BinaryExpr)
				if !ok {
					return "", ErrUnsupported
				}

				return "L(" + inner(be.X) + "," + inner(be.Y) + ")", nil
			},
			want: "package p\nfunc f(a, b, c int) bool { return L(L(a,b),c) }\n",
		},
		"inner_reaches_into_nested_site": {
			src: "package p\nfunc f(a, b, c int) bool { return a+b < c }\n",
			ops: []token.Token{token.ADD, token.LSS},
			rw: func(s Site, inner func(ast.Node) string) (string, error) {
				if s.Tok == token.ADD {
					return wrap(s, inner)
				}
				be, ok := s.Node.(*ast.BinaryExpr)
				if !ok {
					return "", ErrUnsupported
				}
				x, ok := be.X.(*ast.BinaryExpr)
				if !ok {
					return "", ErrUnsupported
				}
				// x.Y lies strictly inside the nested a+b site; nil has no range.
				return "R(" + inner(be.X) + "|" + inner(x.Y) + "|" + inner(nil) + ")", nil
			},
			want: "package p\nfunc f(a, b, c int) bool { return R(W(a+b)|b|) }\n",
		},
		"multi_line_site_kept_on_its_lines": {
			src:  "package p\nfunc f(a, b int) int {\n\treturn a +\n\t\tb\n}\n",
			ops:  []token.Token{token.ADD},
			want: "package p\nfunc f(a, b int) int {\n\treturn W(a +\n\t\tb)\n}\n",
		},
		"rewriter_error_leaves_original": {
			src: "package p\nfunc f(a, b, c int) {\n\tif a+b < c {\n\t}\n}\n",
			ops: []token.Token{token.ADD, token.LSS},
			rw: func(s Site, inner func(ast.Node) string) (string, error) {
				if s.Tok == token.LSS {
					return "", ErrUnsupported
				}

				return wrap(s, inner)
			},
			want:    "package p\nfunc f(a, b, c int) {\n\tif W(a+b) < c {\n\t}\n}\n",
			wantErr: []error{ErrUnsupported},
			errTok:  token.LSS,
		},
		"inserted_newline_rejected": {
			src: "package p\nfunc f(a, b, c int) { g(a+b < c) }\n",
			ops: []token.Token{token.ADD, token.LSS},
			rw: func(s Site, inner func(ast.Node) string) (string, error) {
				if s.Tok == token.LSS {
					return "W(\n" + inner(s.Node) + ")", nil
				}

				return wrap(s, inner)
			},
			want:    "package p\nfunc f(a, b, c int) { g(W(a+b) < c) }\n",
			wantErr: []error{errNewlineChanged},
			errTok:  token.LSS,
		},
		"duplicate_range_reported": {
			src:     "package p\nfunc f(a, b int) { x := a + b; _ = x }\n",
			ops:     []token.Token{token.ADD},
			dup:     true,
			want:    "package p\nfunc f(a, b int) { x := W(a + b); _ = x }\n",
			wantErr: []error{errDuplicateSite},
			errTok:  token.ADD,
		},
		"no_sites": {
			src:  "package p\nfunc f(a, b int) int { return a * b }\n",
			ops:  []token.Token{token.ADD},
			want: "package p\nfunc f(a, b int) int { return a * b }\n",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", tc.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			sites := binarySites(f, tc.ops...)
			if tc.dup {
				sites = append(sites, Site{Node: sites[0].Node, Tok: sites[0].Tok})
			}
			rw := tc.rw
			if rw == nil {
				rw = wrap
			}
			out, errs := Render(fset, fset.File(f.Pos()), []byte(tc.src), sites, rw)
			if string(out) != tc.want {
				t.Errorf("output mismatch\n got: %q\nwant: %q", out, tc.want)
			}
			if len(errs) != len(tc.wantErr) {
				t.Fatalf("got %d SiteErrors %v, want %d", len(errs), errs, len(tc.wantErr))
			}
			for i, se := range errs {
				if !errors.Is(se.Err, tc.wantErr[i]) {
					t.Errorf("SiteError %d: got %v, want %v", i, se.Err, tc.wantErr[i])
				}
				if se.Site.Tok != tc.errTok {
					t.Errorf("SiteError %d names site %v, want %v", i, se.Site.Tok, tc.errTok)
				}
			}
		})
	}
}

func TestRenderPreservesLineNumbers(t *testing.T) {
	t.Parallel()
	src := "package p\n\n" +
		"// keep me\n" +
		"func f(a, b, c int) int {\n" +
		"\tx := a + b\n" +
		"\tif x < c &&\n" +
		"\t\ta-c > 0 {\n" +
		"\t\treturn x * c\n" +
		"\t}\n" +
		"\treturn 0 // tail\n" +
		"}\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	sites := binarySites(f, token.ADD, token.LSS, token.LAND, token.SUB, token.GTR, token.MUL)
	out, errs := Render(fset, fset.File(f.Pos()), []byte(src), sites, wrap)
	if len(errs) != 0 {
		t.Fatalf("unexpected SiteErrors: %v", errs)
	}
	if got, want := bytes.Count(out, []byte("\n")), bytes.Count([]byte(src), []byte("\n")); got != want {
		t.Fatalf("newline count %d, want %d\n%s", got, want, out)
	}
	wantLines := map[string]struct {
		idx  int
		line string
	}{
		"package":    {0, "package p"},
		"blank":      {1, ""},
		"comment":    {2, "// keep me"},
		"func":       {3, "func f(a, b, c int) int {"},
		"assign":     {4, "\tx := W(a + b)"},
		"cond_head":  {5, "\tif W(W(x < c) &&"},
		"cond_tail":  {6, "\t\tW(W(a-c) > 0)) {"},
		"return_mul": {7, "\t\treturn W(x * c)"},
		"close_if":   {8, "\t}"},
		"tail":       {9, "\treturn 0 // tail"},
		"close_func": {10, "}"},
	}
	lines := bytes.Split(out, []byte("\n"))
	for name, wl := range wantLines {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := string(lines[wl.idx]); got != wl.line {
				t.Errorf("line %d: got %q, want %q", wl.idx+1, got, wl.line)
			}
		})
	}
}

// span is an ast.Node with an arbitrary range, for site shapes no parser
// produces.
type span struct{ pos, end token.Pos }

func (s span) Pos() token.Pos { return s.pos }
func (s span) End() token.Pos { return s.end }

func TestRenderRejectsMalformedSites(t *testing.T) {
	t.Parallel()
	const src = "abcdefghij"
	cases := map[string]struct {
		spans   [][2]int // byte offsets [start, end)
		want    string
		wantErr error
		blamed  int // index in spans of the site the SiteError must name
	}{
		"partial_overlap":  {spans: [][2]int{{1, 5}, {3, 8}}, want: "aW(bcde)fghij", wantErr: errOverlap, blamed: 1},
		"outside_source":   {spans: [][2]int{{2, 4}, {8, 20}}, want: "abW(cd)efghij", wantErr: errForeignFile, blamed: 1},
		"invalid_position": {spans: [][2]int{{-1, 3}, {2, 4}}, want: "abW(cd)efghij", wantErr: errBadRange, blamed: 0},
		"end_before_pos":   {spans: [][2]int{{2, 4}, {6, 3}}, want: "abW(cd)efghij", wantErr: errBadRange, blamed: 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			file := fset.AddFile("p.go", -1, len(src))
			base := file.Base()
			var sites []Site
			for _, sp := range tc.spans {
				var n span
				if sp[0] >= 0 {
					n = span{pos: token.Pos(base + sp[0]), end: token.Pos(base + sp[1])}
				}
				sites = append(sites, Site{Node: n})
			}
			out, errs := Render(fset, file, []byte(src), sites, wrap)
			if string(out) != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if len(errs) != 1 || !errors.Is(errs[0].Err, tc.wantErr) {
				t.Fatalf("got SiteErrors %v, want one %v", errs, tc.wantErr)
			}
			if errs[0].Site.Node != sites[tc.blamed].Node {
				t.Errorf("blamed site %v, want sites[%d] = %v", errs[0].Site, tc.blamed, sites[tc.blamed])
			}
		})
	}
}

// TestRewriterInnerOutsideTheSite checks that inner, asked for a node
// outside the site being rewritten, returns that node's original text, even
// where another site lies inside it.
func TestRewriterInnerOutsideTheSite(t *testing.T) {
	t.Parallel()
	const src = "package p\nfunc f(a, b, c, d int) { g(a+b, c-d) }\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	sites := binarySites(f, token.ADD, token.SUB)
	sibling := sites[1].Node // c-d, a site of its own
	rw := func(s Site, inner func(ast.Node) string) (string, error) {
		if s.Tok == token.ADD {
			return "A(" + inner(sibling) + ")", nil
		}

		return wrap(s, inner)
	}
	out, errs := Render(fset, fset.File(f.Pos()), []byte(src), sites, rw)
	if len(errs) != 0 {
		t.Fatalf("SiteErrors: %v", errs)
	}
	// The sibling reads c-d, not its rendering W(c-d).
	if want := "package p\nfunc f(a, b, c, d int) { g(A(c-d), W(c-d)) }\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestRenderRejectsSitesFromAnotherFile(t *testing.T) {
	t.Parallel()
	const aSrc = "package p\nvar x = 1 + 2\n"
	cases := map[string]struct {
		bSrc    string   // the other file in the FileSet
		src     string   // the bytes passed to Render as a.go's
		order   []string // which file's site comes first: "a" or "b"
		want    string
		wantErr []error
	}{
		// b.go is longer, but its site's offsets [16,19) fall inside a.go:
		// unchecked, they would splice a.go's "= 1".
		"foreign_site_reported": {
			bSrc: "package p\nvar y=3+4\n// padding padding\n", src: aSrc, order: []string{"a", "b"},
			want: "package p\nvar x = W(1 + 2)\n", wantErr: []error{errForeignFile},
		},
		"only_foreign_site": {
			bSrc: "package p\nvar y=3+4\n// padding padding\n", src: aSrc, order: []string{"b"},
			want: aSrc, wantErr: []error{errForeignFile},
		},
		// b.go is exactly a.go's length and its site has a.go's offsets.
		"same_size_foreign_site_first": {
			bSrc: "package p\nvar y = 3 + 4\n", src: aSrc, order: []string{"b", "a"},
			want: "package p\nvar x = W(1 + 2)\n", wantErr: []error{errForeignFile},
		},
		"same_size_only_foreign_site": {
			bSrc: "package p\nvar y = 3 + 4\n", src: aSrc, order: []string{"b"},
			want: aSrc, wantErr: []error{errForeignFile},
		},
		"size_mismatch": {
			bSrc: "package p\nvar y = 3 + 4\n", src: aSrc + "// extra\n", order: []string{"a"},
			want: aSrc + "// extra\n", wantErr: []error{errSourceSize},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			fa, err := parser.ParseFile(fset, "a.go", aSrc, 0)
			if err != nil {
				t.Fatal(err)
			}
			fb, err := parser.ParseFile(fset, "b.go", tc.bSrc, 0)
			if err != nil {
				t.Fatal(err)
			}
			var sites []Site
			for _, which := range tc.order {
				f := fa
				if which == "b" {
					f = fb
				}
				sites = append(sites, binarySites(f, token.ADD)...)
			}
			out, errs := Render(fset, fset.File(fa.Pos()), []byte(tc.src), sites, wrap)
			if string(out) != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if len(errs) != len(tc.wantErr) {
				t.Fatalf("got SiteErrors %v, want %v", errs, tc.wantErr)
			}
			for i, se := range errs {
				if !errors.Is(se.Err, tc.wantErr[i]) {
					t.Errorf("SiteError %d: got %v, want %v", i, se.Err, tc.wantErr[i])
				}
				// A foreign-file error must name b.go's site, never a.go's.
				if got := fset.File(se.Site.Node.Pos()).Name(); errors.Is(se.Err, errForeignFile) && got != "b.go" {
					t.Errorf("SiteError %d names a site in %s, want b.go", i, got)
				}
			}
		})
	}
}
