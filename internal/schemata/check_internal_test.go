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
	"slices"
	"strings"
	"testing"
)

// TestLayoutSpansWithRepeatedText holds layoutSpans to what it can know: a
// child of a rewritten site is found by its text, searched forward from the
// end of the previous child. Siblings with equal text are told apart by the
// cursor; a child whose text the rewrite repeats (as an op= switch repeats
// its operands in every arm) is found at the first copy, so an error in a
// later copy is attributed to the parent. That costs the parent a drop
// (restoreChildren gives the child its retry), never a broken build.
func TestLayoutSpansWithRepeatedText(t *testing.T) {
	t.Parallel()
	const file = "p.go"
	child := func(text string) *siteNode { return &siteNode{site: Site{Node: &ast.Ident{}}, rendered: text} }
	cases := map[string]struct {
		parent    string
		children  []string
		wantStart []int // each child's span start, -1 for no span
		errAt     int   // an error offset, relative to the parent's start
		wantBlame int   // index in children of the site blamed, -1 for the parent
	}{
		"siblings_with_equal_text_each_found_in_order": {
			parent: "f(X, X)", children: []string{"X", "X"}, wantStart: []int{2, 5}, errAt: 5, wantBlame: 1,
		},
		"text_repeated_in_the_parent_is_found_at_the_first_copy": {
			parent: "case 1: X; default: X", children: []string{"X"}, wantStart: []int{8}, errAt: 20, wantBlame: -1,
		},
		"error_in_the_first_copy_blames_the_child": {
			parent: "case 1: X; default: X", children: []string{"X"}, wantStart: []int{8}, errAt: 8, wantBlame: 0,
		},
		"child_text_absent_from_the_parent_has_no_span": {
			parent: "f(Y)", children: []string{"X"}, wantStart: []int{-1}, errAt: 2, wantBlame: -1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := &siteNode{site: Site{Node: &ast.Ident{}}, rendered: tc.parent}
			for _, c := range tc.children {
				root.children = append(root.children, child(c))
			}
			const base = 10 // the parent's offset in its file
			var spans []renderedSpan
			layoutChildren(root.children, base, 0, true, root.rendered, &spans)
			spans = append(spans, renderedSpan{site: root.site, start: base, end: base + len(tc.parent)})

			for i, c := range root.children {
				start := -1
				for _, s := range spans {
					if s.site.Node == c.site.Node {
						start = s.start - base
					}
				}
				if start != tc.wantStart[i] {
					t.Errorf("child %d (%q) starts at %d, want %d", i, c.rendered, start, tc.wantStart[i])
				}
			}
			overlay := map[string][]byte{file: []byte(strings.Repeat(" ", base) + tc.parent)}
			bad, unattributable := attribute([]typeError{{file: file, offset: base + tc.errAt, msg: "boom"}}, overlay, map[string][]renderedSpan{file: spans})
			if unattributable != "" {
				t.Fatalf("unattributable: %s", unattributable)
			}
			blamed := root.site.Node
			if tc.wantBlame >= 0 {
				blamed = root.children[tc.wantBlame].site.Node
			}
			if _, ok := bad[mutantKey{blamed, 0}]; !ok || len(bad) != 1 {
				t.Errorf("blamed %v, want only %v", bad, blamed)
			}
		})
	}
}

// TestUniquePackages checks that BuildAll's list of packages to build holds
// each path once, in first-seen order: a repeat would start two
// `go test -c` writing the one binary.
func TestUniquePackages(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ in, want []string }{
		"none":        {in: nil, want: []string{}},
		"distinct":    {in: []string{"m/b", "m/a"}, want: []string{"m/b", "m/a"}},
		"repeats":     {in: []string{"m/b", "m/a", "m/b", "m/a", "m/b"}, want: []string{"m/b", "m/a"}},
		"only_repeat": {in: []string{"m/a", "m/a"}, want: []string{"m/a"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := uniquePackages(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("uniquePackages(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestRemoveSite checks that a SiteError removes what it names: the whole
// site when it holds all of the site's mutants (or none), only its own
// mutants from a site keeping others, and, of a site listed twice, the entry
// holding its mutants.
func TestRemoveSite(t *testing.T) {
	t.Parallel()
	n, other := &ast.BasicLit{}, &ast.BasicLit{}
	muts := func(ids ...int) []Mutant {
		out := make([]Mutant, len(ids))
		for i, id := range ids {
			out[i] = Mutant{ID: id}
		}

		return out
	}
	group := Site{Node: n, Muts: muts(1, 2, 3), Members: []Site{{Node: other, Muts: muts(1, 2)}, {Node: other, Muts: muts(3)}}}
	cases := map[string]struct {
		sites []Site
		drop  Site
		want  [][]int // each remaining site's mutant ids
		// members are the remaining first site's members' mutant ids.
		members [][]int
	}{
		"whole":       {sites: []Site{{Node: n, Muts: muts(1, 2)}}, drop: Site{Node: n, Muts: muts(1, 2)}, want: [][]int{}},
		"no_mutants":  {sites: []Site{{Node: n, Muts: muts(1, 2)}}, drop: Site{Node: n}, want: [][]int{}},
		"partial":     {sites: []Site{group}, drop: Site{Node: n, Muts: muts(2)}, want: [][]int{{1, 3}}, members: [][]int{{1}, {3}}},
		"member_gone": {sites: []Site{group}, drop: Site{Node: n, Muts: muts(3)}, want: [][]int{{1, 2}}, members: [][]int{{1, 2}}},
		"listed_twice": {
			sites: []Site{{Node: n, Muts: muts(1)}, {Node: n, Muts: muts(2)}},
			drop:  Site{Node: n, Muts: muts(2)}, want: [][]int{{1}},
		},
		"absent": {sites: []Site{{Node: n, Muts: muts(1)}}, drop: Site{Node: other, Muts: muts(1)}, want: [][]int{{1}}},
	}
	ids := func(ms []Mutant) []int {
		out := []int{}
		for _, m := range ms {
			out = append(out, m.ID)
		}

		return out
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			before := len(tc.sites[0].Muts)
			got := removeSite(tc.sites, tc.drop)
			if len(tc.sites[0].Muts) != before {
				t.Error("removeSite changed its input")
			}
			var gotIDs [][]int
			for _, s := range got {
				gotIDs = append(gotIDs, ids(s.Muts))
			}
			if !slices.EqualFunc(gotIDs, tc.want, slices.Equal[[]int]) {
				t.Errorf("remaining %v, want %v", gotIDs, tc.want)
			}
			if tc.members == nil {
				return
			}
			var gotMembers [][]int
			for _, m := range got[0].Members {
				gotMembers = append(gotMembers, ids(m.Muts))
			}
			if !slices.EqualFunc(gotMembers, tc.members, slices.Equal[[]int]) {
				t.Errorf("members %v, want %v", gotMembers, tc.members)
			}
		})
	}
}

// TestUnusedImport holds the unused-import match to exactly the type
// checker's error at an import spec's position in a rendered file.
func TestUnusedImport(t *testing.T) {
	t.Parallel()
	const src = "package p\n\nimport (\n\t\"math\"\n\tm \"math\"\n\t_ \"os\"\n\t\"C\"\n)\n"
	const file = "/p.go"
	at := func(s string) int { return strings.Index(src, s) }
	cases := map[string]struct {
		e        typeError
		rendered bool
		want     string
	}{
		"unnamed":        {e: typeError{file: file, offset: at(`"math"`), msg: `p.go:4:2: "math" imported and not used`}, rendered: true, want: ` "math"`},
		"named":          {e: typeError{file: file, offset: at(`m "math"`), msg: `p.go:5:2: "math" imported as m and not used`}, rendered: true, want: `m "math"`},
		"blank":          {e: typeError{file: file, offset: at(`_ "os"`), msg: `"os" imported and not used`}, rendered: true},
		"cgo":            {e: typeError{file: file, offset: at(`"C"`), msg: `"C" imported and not used`}, rendered: true},
		"other_message":  {e: typeError{file: file, offset: at(`"math"`), msg: `undefined: math`}, rendered: true},
		"not_at_a_spec":  {e: typeError{file: file, offset: at(`import`), msg: `"math" imported and not used`}, rendered: true},
		"not_rendered":   {e: typeError{file: file, offset: at(`"math"`), msg: `"math" imported and not used`}},
		"unparsable":     {e: typeError{file: "/q.go", offset: 0, msg: `"math" imported and not used`}, rendered: true},
		"not_in_overlay": {e: typeError{file: "/r.go", offset: 0, msg: `"math" imported and not used`}, rendered: true},
	}
	overlay := map[string][]byte{file: []byte(src), "/q.go": []byte("package")}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, ok := unusedImport(tc.e, overlay, map[string]bool{tc.e.file: tc.rendered})
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("unusedImport = %q, %v; want %q", got, ok, tc.want)
			}
		})
	}
}

// TestBlankImports checks the in-place edit: the named import's name is
// replaced, the unnamed import gets "_ " before its path, other imports and
// the line count are untouched, and each span after an edit moves by its
// change in length.
func TestBlankImports(t *testing.T) {
	t.Parallel()
	src := []byte("package p\n\nimport (\n\t\"fmt\"\n\t\"math\"\n\tstr \"strings\"\n)\n\nvar x = 1\n")
	site := strings.Index(string(src), "1\n")
	spans := []renderedSpan{{start: site, end: site + 1}}
	out, moved := blankImports(src, spans, map[string]bool{` "math"`: true, `str "strings"`: true})
	want := "package p\n\nimport (\n\t\"fmt\"\n\t_ \"math\"\n\t_ \"strings\"\n)\n\nvar x = 1\n"
	if string(out) != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
	if got := string(out[moved[0].start:moved[0].end]); got != "1" || spans[0].start != site {
		t.Errorf("span moved to %q (the input span at %d, was %d)", got, spans[0].start, site)
	}
	if got, _ := blankImports([]byte("package"), spans, map[string]bool{"x": true}); string(got) != "package" {
		t.Errorf("unparsable source changed: %q", got)
	}
	if got, _ := blankImports(src, spans, nil); !slices.Equal(got, src) {
		t.Errorf("no keys changed the source: %q", got)
	}
}

// TestLearn checks that learn reports progress only for an import not yet
// blanked: one reported unused again although blanked ends the repair.
func TestLearn(t *testing.T) {
	t.Parallel()
	blank := map[string]map[string]bool{}
	if !learn(blank, map[string]map[string]bool{"/p.go": {` "math"`: true}}) {
		t.Error("a new import is not progress")
	}
	if learn(blank, map[string]map[string]bool{"/p.go": {` "math"`: true}}) {
		t.Error("an import already blanked is progress")
	}
	if !learn(blank, map[string]map[string]bool{"/p.go": {` "math"`: true, ` "os"`: true}}) {
		t.Error("a new import beside a blanked one is not progress")
	}
}
