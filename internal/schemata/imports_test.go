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

package schemata_test

import (
	"bytes"
	"context"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

// TestRewritePackageBlanksAnImportARewriteLeftUnused checks the unused
// import repair: a constant form replaces a constant with its value, which
// can remove a file's last use of an import, and the rendered file then
// fails "imported and not used" outside every site. The import is rewritten
// in place to the blank identifier, keeping its init side effects and the
// file's lines, and the package is placed -- under each mutant the schema
// prints what the plain token mutant prints. An import still used is left
// as written, and any other error still drops what it dropped before.
func TestRewritePackageBlanksAnImportARewriteLeftUnused(t *testing.T) {
	t.Parallel()
	const driver = "package p\n\nimport (\n\t\"fmt\"\n\t\"testing\"\n)\n\nfunc TestPrint(t *testing.T) { fmt.Println(F()) }\n"
	testCases := map[string]struct {
		src     string
		factory schemata.RewriterFactory
		// import is the import line the rendered file must hold, "" when the
		// package is dropped.
		imp string
		// reason, when set, is what every dropped site's error holds; else
		// nothing is dropped.
		reason string
	}{
		"float_form": {
			src: "package p\n\nimport \"math\"\n\nfunc F() any { return []any{-math.MaxFloat64} }\n",
			imp: "import _ \"math\"\n",
		},
		"int_form": {
			src: "package p\n\nimport \"math\"\n\nfunc F() int64 { return math.MaxInt8 - 1 }\n",
			imp: "import _ \"math\"\n",
		},
		"named_import": {
			src: "package p\n\nimport m \"math\"\n\nfunc F() any { return []any{-m.MaxFloat64} }\n",
			imp: "import _ \"math\"\n",
		},
		"grouped_imports": {
			src: "package p\n\nimport (\n\t\"fmt\"\n\t\"math\"\n)\n\nfunc F() string { return fmt.Sprint(-math.MaxFloat64) }\n",
			imp: "\t_ \"math\"\n",
		},
		"still_used": {
			src: "package p\n\nimport \"math\"\n\nfunc F() any { return []any{-math.MaxFloat64, math.Pi} }\n",
			imp: "import \"math\"\n",
		},
		"site_error_beside_it": {
			src:     "package p\n\nimport \"math\"\n\nfunc F() any { return []any{-math.MaxFloat64, g(1, 2)} }\n\nfunc g(a, b int) int { return a + b }\n",
			factory: breakTok(token.ADD),
			imp:     "import _ \"math\"\n",
			reason:  "undefined: _gremlinsNope",
		},
		"dot_import": {
			src: "package p\n\nimport . \"math\"\n\nfunc F() any { return []any{-MaxFloat64} }\n",
			imp: "import _ \"math\"\n",
		},
		// A form that drops an operand holding the import's last use is a
		// defect the repair must not hide: only a constant form's dropped
		// text is repaired.
		"lost_by_another_form": {
			src:     "package p\n\nimport \"math\"\n\nfunc F() float64 { return math.Abs(2) + 1 }\n",
			factory: dropTok(token.ADD),
			reason:  "unattributable type error: ",
		},
		"dot_import_lost_by_another_form": {
			src:     "package p\n\nimport . \"math\"\n\nfunc F() float64 { return Abs(2) + 1 }\n",
			factory: dropTok(token.ADD),
			reason:  "unattributable type error: ",
		},
		"unattributable_error": {
			src:     "package p\n\nimport \"math\"\n\nfunc F() any { return []any{-math.MaxFloat64} }\n",
			factory: brokenHelper,
			reason:  "unattributable type error: ",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := writeModule(t, map[string][]byte{"p.go": []byte(tc.src), "p_test.go": []byte(driver)})
			pkg := loadPkg(t, dir, ".")
			f := pkg.Syntax[0]
			for _, sf := range pkg.Syntax {
				if strings.HasSuffix(pkg.Fset.Position(sf.Pos()).Filename, "p.go") {
					f = sf
				}
			}
			sites, plain := discover(f)
			factory := tc.factory
			if factory == nil {
				factory = schemata.NewRewriter
			}
			files, placed, dropped := schemata.RewritePackageWith(context.Background(), pkg, sites, "", factory)
			for _, d := range dropped {
				if tc.reason == "" || !strings.Contains(d.Err.Error(), tc.reason) {
					t.Errorf("site at %s dropped: %v", pkg.Fset.Position(d.Site.Node.Pos()), d.Err)
				}
			}
			if tc.reason != "" && len(dropped) == 0 {
				t.Errorf("nothing dropped, want %q", tc.reason)
			}
			if tc.imp == "" {
				if len(files) != 0 || len(placed) != 0 {
					t.Errorf("placed %d sites in %d files, want the package dropped", len(placed), len(files))
				}

				return
			}
			out, ok := files[filepath.Join(dir, "p.go")]
			if !ok {
				t.Fatalf("p.go not rendered; files %v", slices.Collect(maps.Keys(files)))
			}
			if !bytes.Contains(out, []byte(tc.imp)) {
				t.Errorf("rendered p.go lacks %q:\n%s", tc.imp, out)
			}
			if got, want := bytes.Count(out, []byte("\n")), strings.Count(tc.src, "\n"); got != want {
				t.Errorf("rendered p.go has %d lines, the source %d", got, want)
			}
			if tc.reason != "" {
				return
			}
			schema := map[string][]byte{"p_test.go": []byte(driver)}
			for path, content := range files {
				schema[filepath.Base(path)] = content
			}
			schemaBin := buildFixture(t, schema)
			orig := runFixture(t, buildFixture(t, map[string][]byte{"p.go": []byte(tc.src), "p_test.go": []byte(driver)}))
			if got := runFixture(t, schemaBin); !bytes.Equal(got, orig) {
				t.Errorf("schema without a mutant printed\n%s\nthe original printed\n%s", got, orig)
			}
			for _, p := range plain {
				to, ok := engine.TokenMutation(p.mt, p.tok)
				if !ok {
					t.Fatalf("engine has no %s mutation for %s", p.mt, p.tok)
				}
				off := pkg.Fset.Position(p.pos).Offset
				src := []byte(tc.src)
				mutated := slices.Concat(src[:off:off], []byte(to.String()), src[off+len(p.tok.String()):])
				want := runFixture(t, buildFixture(t, map[string][]byte{"p.go": mutated, "p_test.go": []byte(driver)}))
				reach := filepath.Join(t.TempDir(), "reached")
				got := runFixture(t, schemaBin, "GREMLINS_MUTANT="+strconv.Itoa(p.id), "GREMLINS_REACHED="+reach)
				if !bytes.Equal(got, want) {
					t.Errorf("schema with mutant %d printed\n%s\nthe plain mutant printed\n%s", p.id, got, want)
				}
				if _, err := os.Stat(reach); err != nil {
					t.Errorf("mutant %d: reach file missing: %v", p.id, err)
				}
			}
		})
	}
}

// dropTok returns a rewriter factory that wraps NewRewriter but rewrites
// every binary site whose token is tok to 0, dropping its operands.
func dropTok(tok token.Token) schemata.RewriterFactory {
	return func(info *types.Info, sizes types.Sizes, files []*ast.File, prefix string, h *schemata.HelperSet) schemata.Rewriter {
		rw := schemata.NewRewriter(info, sizes, files, prefix, h)

		return func(s schemata.Site, inner func(ast.Node) string) (string, error) {
			if _, ok := s.Node.(*ast.BinaryExpr); ok && s.Tok == tok {
				return "0", nil
			}

			return rw(s, inner)
		}
	}
}
