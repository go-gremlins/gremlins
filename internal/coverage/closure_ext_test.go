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

package coverage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// removed, as the content of a file in a change, deletes the file.
const removed = "\x00removed"

// files is a set of fixture files by path below the fixture root.
type files map[string]string

// write puts files into the fixture, creating directories as needed.
func (h *cacheHarness) write(fs files) {
	h.t.Helper()

	for rel, content := range fs {
		path := filepath.Join(h.pkgRoot, rel)
		if content == removed {
			if err := os.Remove(path); err != nil {
				h.t.Fatalf("cannot remove %s: %v", rel, err)
			}

			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			h.t.Fatalf("cannot create the directory of %s: %v", rel, err)
		}
		writeFixture(h.t, path, content)
	}
}

// calcWith is calc with Double's one-line body replaced, on line 4 as before,
// and line 2 — blank in calcSource — holding header, so that every line the
// fake profiles name stays where it is: Double over 3-5, Triple over 7-9.
func calcWith(header, doubleBody, tripleBody string) string {
	return "package calc\n" + header + "\nfunc Double(n int) int {\n\t" + doubleBody +
		"\n}\n\nfunc Triple(n int) int {\n\t" + tripleBody + "\n}\n"
}

// change is a mapped package before and after an edit, and the tests the edit
// can have reached: the ones a run after it must map again.
type change struct {
	before files
	after  files
	want   []string
}

// remapped maps calc, applies the change, maps it again, and reports what the
// second run had to execute.
func remapped(t *testing.T, c change) []string {
	t.Helper()

	h := newCacheHarness(t)
	h.write(c.before)
	first := h.buildCalc("")
	if first.Len() != 2 {
		t.Fatalf("want calc mapped before the change, got %d tests", first.Len())
	}
	h.write(c.after)
	second := h.buildCalc(changedCalc)
	if second.Len() != 2 {
		t.Errorf("want the map still complete after the change, got %d tests", second.Len())
	}

	return h.testsRun()
}

func runChanges(t *testing.T, cases map[string]change) {
	t.Helper()

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(c.want, remapped(t, c)); diff != "" {
				t.Errorf("re-mapped the wrong tests (-want +got):\n%s", diff)
			}
		})
	}
}

// Every change here is to something no coverage block holds, and the closure
// over names is what decides which tests it can have reached: those whose
// executed code, or whose own source, names something the change affected.
func TestAChangeOutsideAFunctionBodyReMapsOnlyWhatNamesIt(t *testing.T) {
	runChanges(t, map[string]change{
		// Nothing that exists names either of them.
		"an added type and function": {
			after: files{"calc/calc.go": calcSource + "\ntype unit int\n\nfunc Quadruple(n unit) unit {\n\treturn n * 4\n}\n"},
		},
		"a constant one function uses": {
			before: files{
				"calc/calc.go":   calcWith("", "return n * factor", "return n * 3"),
				"calc/consts.go": "package calc\n\nconst factor = 2\n",
			},
			after: files{"calc/consts.go": "package calc\n\nconst factor = 3\n"},
			want:  []string{"TestDouble"},
		},
		// Inserting a name moves b's value under iota without changing b's
		// text, so a const group changes as one.
		"a name inserted into a const group under iota": {
			before: files{
				"calc/calc.go":   calcWith("", "return n * (b + 1)", "return n * 3"),
				"calc/consts.go": "package calc\n\nconst (\n\ta = iota\n\tb\n)\n",
			},
			after: files{"calc/consts.go": "package calc\n\nconst (\n\tpad = iota\n\ta\n\tb\n)\n"},
			want:  []string{"TestDouble"},
		},
		// The tag is read by reflection somewhere Double reaches, through a
		// value it built: Double names the type.
		"a struct tag": {
			before: files{
				"calc/calc.go":  calcWith("", "return n*2 + size(point{})", "return n * 3"),
				"calc/types.go": "package calc\n\ntype point struct {\n\tX int `json:\"x\"`\n}\n\nfunc size(p point) int {\n\treturn p.X\n}\n",
			},
			after: files{"calc/types.go": "package calc\n\ntype point struct {\n\tX int `json:\"y\"`\n}\n\nfunc size(p point) int {\n\treturn p.X\n}\n"},
			want:  []string{"TestDouble"},
		},
		// A method added to a type changes which interfaces it satisfies,
		// and so where a type switch over a value of it goes: the change is
		// to the receiver type, and Double names it.
		"an added method that satisfies an interface": {
			before: files{
				"calc/calc.go":  calcWith("", "return describe(counter(n))", "return n * 3"),
				"calc/types.go": "package calc\n\ntype counter int\n\nfunc describe(v any) int {\n\tif _, ok := v.(interface{ String() string }); ok {\n\t\treturn 1\n\t}\n\treturn 0\n}\n",
			},
			after: files{"calc/types.go": "package calc\n\ntype counter int\n\nfunc (c counter) String() string {\n\treturn \"\"\n}\n\nfunc describe(v any) int {\n\tif _, ok := v.(interface{ String() string }); ok {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"},
			want:  []string{"TestDouble"},
		},
		"an added type with a method nothing names": {
			after: files{"calc/calc.go": calcSource + "\ntype counter int\n\nfunc (c counter) String() string {\n\treturn \"counter\"\n}\n"},
		},
		// A handler handed to something that reflects over its signature is
		// called by nothing the test executed in the module, so its
		// signature reaches the test through the name Double passes.
		"a handler's signature, the handler passed as a value": {
			before: files{
				"calc/calc.go":    calcWith("", "return register(twice)", "return n * 3"),
				"calc/handler.go": "package calc\n\nfunc twice(n int) int {\n\treturn n * 2\n}\n\nfunc register(h any) int {\n\t_ = h\n\treturn 0\n}\n",
			},
			after: files{"calc/handler.go": "package calc\n\nfunc twice(n int, _ ...int) int {\n\treturn n * 2\n}\n\nfunc register(h any) int {\n\t_ = h\n\treturn 0\n}\n"},
			want:  []string{"TestDouble"},
		},
		// Its body, by contrast, reaches only a test that executed it.
		"a handler's body, the handler passed as a value": {
			before: files{
				"calc/calc.go":    calcWith("", "return register(twice)", "return n * 3"),
				"calc/handler.go": "package calc\n\nfunc twice(n int) int {\n\treturn n * 2\n}\n\nfunc register(h any) int {\n\t_ = h\n\treturn 0\n}\n",
			},
			after: files{"calc/handler.go": "package calc\n\nfunc twice(n int) int {\n\treturn n + n\n}\n\nfunc register(h any) int {\n\t_ = h\n\treturn 0\n}\n"},
		},
		// Every call site's text is as it was, and every one of them now
		// calls into another package.
		"an import swapped under the same name": {
			before: files{"calc/calc.go": calcWith(`import st "example.com/vm"`, "return st.Size(nil) + n*2", "return n * 3")},
			after:  files{"calc/calc.go": calcWith(`import st "example.com/empty"`, "return st.Size(nil) + n*2", "return n * 3")},
			want:   []string{"TestDouble"},
		},
		// The directive is the only text that changed, and it decides what
		// the variable holds.
		"a //go:embed target": {
			before: files{
				"calc/calc.go": calcWith("", "return n * len(data)", "return n * 3"),
				"calc/data.go": "package calc\n\nimport _ \"embed\"\n\n//go:embed a.txt\nvar data string\n",
				"calc/a.txt":   "a",
				"calc/b.txt":   "bb",
			},
			after: files{"calc/data.go": "package calc\n\nimport _ \"embed\"\n\n//go:embed b.txt\nvar data string\n"},
			want:  []string{"TestDouble"},
		},
		// A prose comment decides nothing.
		"a comment": {
			before: files{
				"calc/calc.go":   calcWith("", "return n * factor", "return n * 3"),
				"calc/consts.go": "package calc\n\n// factor is two.\nconst factor = 2\n",
			},
			after: files{"calc/consts.go": "package calc\n\n// factor is what Double multiplies by.\nconst factor = 2\n"},
		},
		// A constructor known to do nothing but build its value is a plain
		// named entity, not a call run for every test: errors.New, and
		// fmt.Errorf over a literal and an error errors.New made, neither of
		// which has a method of the package's for it to call.
		"an error variable": {
			before: files{
				"calc/calc.go": calcWith("", "return n*2 + len(errBad.Error())", "return n * 3"),
				"calc/errs.go": "package calc\n\nimport \"errors\"\n\nvar errBad = errors.New(\"bad\")\n",
			},
			after: files{"calc/errs.go": "package calc\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n\t\"regexp\"\n)\n\nvar errBad = errors.New(\"worse\")\n\nvar errNew = fmt.Errorf(\"new: %w\", errBad)\n\nvar pattern = regexp.MustCompile(\"x+\")\n"},
			want:  []string{"TestDouble"},
		},
		// A function literal's body runs when it is called, not when the
		// variable holding it is initialised.
		"a variable holding a function literal": {
			before: files{
				"calc/calc.go": calcWith("", "return n * hook()", "return n * 3"),
				"calc/hook.go": "package calc\n\nvar hook = func() int {\n\treturn compute()\n}\n\nfunc compute() int {\n\treturn 2\n}\n",
			},
			after: files{"calc/hook.go": "package calc\n\nvar hook = func() int {\n\treturn compute() + 0\n}\n\nfunc compute() int {\n\treturn 2\n}\n"},
			want:  []string{"TestDouble"},
		},
	})
}

// A new top-level name that is also a predeclared identifier rebinds every use
// of the identifier without one of them changing: Double calls the new min from
// then on. It is a changed name like any other, and whatever names it is
// affected, whatever kind of declaration the new one is.
func TestAnAddedPredeclaredNameReMapsItsUses(t *testing.T) {
	before := files{"calc/calc.go": calcWith("", "return min(max(n*2, 0), cap(make([]any, 9)))", "return n * 3")}
	cases := map[string]change{}
	for name, added := range map[string]string{
		"a function": "package calc\n\nfunc min(a, b int) int {\n\treturn a\n}\n",
		"a var":      "package calc\n\nvar max = func(a, b int) int { return a }\n",
		"a const":    "package calc\n\nconst cap = 4\n",
		"a type":     "package calc\n\ntype any = int\n",
	} {
		cases[name] = change{before: before, after: files{"calc/shadow.go": added}, want: []string{"TestDouble"}}
	}
	runChanges(t, cases)
}

// A test file is never instrumented, so a test's reach through its own file is
// followed by name, bodies included.
func TestATestFileChangeReMapsOnlyTheTestsThatNameIt(t *testing.T) {
	tripleWithHelper := func(body string) string {
		return "package calc\n\nimport \"testing\"\n\nfunc want(t *testing.T, got, expected int) {\n\tt.Helper()\n\t" + body +
			"\n}\n\nfunc TestTriple(t *testing.T) {\n\twant(t, Triple(2), 6)\n}\n"
	}
	runChanges(t, map[string]change{
		"a helper's body": {
			before: files{"calc/triple_test.go": tripleWithHelper("if got != expected {\n\t\tt.Fail()\n\t}")},
			after:  files{"calc/triple_test.go": tripleWithHelper("if got != expected {\n\t\tt.Error(got)\n\t}")},
			want:   []string{"TestTriple"},
		},
		// A helper's helper is reached through the one between.
		"a helper the helper calls": {
			before: files{
				"calc/triple_test.go": tripleWithHelper("check(t, got == expected)"),
				"calc/util_test.go":   "package calc\n\nimport \"testing\"\n\nfunc check(t *testing.T, ok bool) {\n\tif !ok {\n\t\tt.Fail()\n\t}\n}\n",
			},
			after: files{"calc/util_test.go": "package calc\n\nimport \"testing\"\n\nfunc check(t *testing.T, ok bool) {\n\tif !ok {\n\t\tt.Error(\"no\")\n\t}\n}\n"},
			want:  []string{"TestTriple"},
		},
		// A test file is entities like any other: what it declares is added,
		// and nothing names it.
		"a test file added": {
			after: files{"calc/extra_test.go": "package calc\n\nfunc unused() int {\n\treturn 1\n}\n"},
		},
		// The helper is removed from one file and added in another, and
		// TestTriple names it.
		"a helper moved into a test file of its own": {
			before: files{"calc/triple_test.go": tripleWithHelper("_ = got == expected")},
			after: files{
				"calc/triple_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestTriple(t *testing.T) {\n\twant(t, Triple(2), 6)\n}\n",
				"calc/want_test.go":   "package calc\n\nimport \"testing\"\n\nfunc want(t *testing.T, got, expected int) {\n\t_ = got == expected\n}\n",
			},
			want: []string{"TestTriple"},
		},
		// A changed name of the package reaches a test that names it, though
		// no profile records a line of the test.
		"a constant only a test names": {
			before: files{
				"calc/triple_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestTriple(t *testing.T) {\n\tif Triple(2) != expected {\n\t\tt.Fail()\n\t}\n}\n",
				"calc/consts.go":      "package calc\n\nconst expected = 6\n",
			},
			after: files{"calc/consts.go": "package calc\n\nconst expected = 7\n"},
			want:  []string{"TestTriple"},
		},
	})
}

// A dependency's changes are attributed by the same rules as the package's:
// a body to the tests that executed it, a name to whatever names it, across
// the package boundary by its exported names.
func TestADependencyChangeReMapsOnlyWhatReachesIt(t *testing.T) {
	vmWithLimit := func(limit string) string {
		return strings.Replace(vmSource, "x := n\n", "x := n + limit - limit\n", 1) + "\nconst limit = " + limit + "\n"
	}
	tripleUsesSize := calcWith(`import "example.com/vm"`, "return n * 2", "return n*3 + vm.Size(nil)")
	runChanges(t, map[string]change{
		// Clamp names the constant, and TestDouble executed Clamp.
		"a constant a dependency function uses": {
			before: files{"vm/vm.go": vmWithLimit("10")},
			after:  files{"vm/vm.go": vmWithLimit("11")},
			want:   []string{"TestDouble"},
		},
		// Triple names Size; nothing executed it.
		"a dependency function's signature": {
			before: files{"calc/calc.go": tripleUsesSize},
			after:  files{"vm/vm.go": strings.Replace(vmSource, "Size(v []int) int", "Size(v []int) (n int)", 1)},
			want:   []string{"TestTriple"},
		},
		"a dependency type nothing names": {
			after: files{"vm/vm.go": vmSource + "\ntype box int\n\nfunc (b box) String() string {\n\treturn \"box\"\n}\n"},
		},
		// len is rebound in vm alone; Clamp does not name it, Size does, and
		// no test of calc executed Size.
		"a dependency function shadowing a predeclared identifier": {
			after: files{"vm/vm.go": vmSource + "\nfunc len(v []int) int {\n\treturn 0\n}\n"},
		},
		// A dependency's test files are never linked into another package's
		// test binary: nothing in them, init and TestMain included, can reach
		// a mapping made there.
		"a dependency's test file": {
			before: files{"vm/vm_test.go": "package vm\n\nimport \"testing\"\n\nfunc helper() int {\n\treturn Size(nil)\n}\n\nfunc TestSize(t *testing.T) {\n\t_ = helper()\n}\n"},
			after: files{
				"vm/vm_test.go":     "package vm\n\nimport \"testing\"\n\nfunc init() {}\n\nfunc helper() int {\n\treturn Clamp(1, 2, 3)\n}\n\nfunc TestMain(m *testing.M) {\n\tm.Run()\n}\n\nfunc TestSize(t *testing.T) {\n\t_ = helper()\n}\n",
				"vm/export_test.go": "package vm\n\nvar Exported = Size\n",
			},
		},
	})
}

// Each of these runs for every test, or decides what is compiled at all, so
// whatever changed it re-maps every package that has it in scope.
func TestAChangeEveryTestCanReachReMapsThePackage(t *testing.T) {
	both := []string{"TestDouble", "TestTriple"}
	cases := map[string]change{
		"an init added":   {after: files{"calc/init.go": "package calc\n\nfunc init() {}\n"}},
		"an init changed": {before: files{"calc/init.go": "package calc\n\nfunc init() {\n\t_ = 1\n}\n"}, after: files{"calc/init.go": "package calc\n\nfunc init() {\n\t_ = 2\n}\n"}},
		"an init removed": {before: files{"calc/init.go": "package calc\n\nfunc init() {}\n"}, after: files{"calc/init.go": "package calc\n"}},
		"a var initialised by a call": {
			after: files{"calc/vars.go": "package calc\n\nvar start = compute()\n\nfunc compute() int {\n\treturn 1\n}\n"},
		},
		// Errorf formats its operands there and then, and formatting a value
		// calls its String method.
		"a var formatting a value with fmt.Errorf": {
			after: files{"calc/errs.go": "package calc\n\nimport \"fmt\"\n\ntype item struct{}\n\nvar errX = fmt.Errorf(\"%v\", item{})\n"},
		},
		"a var initialised by an invoked function literal": {
			after: files{"calc/vars.go": "package calc\n\nvar start = func() int {\n\treturn 1\n}()\n"},
		},
		// Its text is as it was, but it calls with something new.
		"what a var initialised by a call names": {
			before: files{"calc/vars.go": "package calc\n\nvar start = compute(seed)\n\nconst seed = 1\n\nfunc compute(n int) int {\n\treturn n\n}\n"},
			after:  files{"calc/vars.go": "package calc\n\nvar start = compute(seed)\n\nconst seed = 2\n\nfunc compute(n int) int {\n\treturn n\n}\n"},
		},
		"a TestMain": {
			after: files{"calc/main_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) {\n\tm.Run()\n}\n"},
		},
		"a blank import": {after: files{"calc/side.go": "package calc\n\nimport _ \"example.com/vm\"\n"}},
		"a dot import":   {after: files{"calc/side.go": "package calc\n\nimport . \"example.com/vm\"\n\nvar s = Size\n"}},
		"a build constraint": {
			before: files{"calc/extra.go": "package calc\n"},
			after:  files{"calc/extra.go": "//go:build linux\n\npackage calc\n"},
		},
		"a file named for a platform": {after: files{"calc/extra_linux.go": "package calc\n"}},
		// A test file is entities like any other, but a platform suffix
		// before its _test still decides when it is compiled.
		"a test file added for a platform": {after: files{"calc/extra_linux_test.go": "package calc\n"}},
		"a cgo preamble": {
			before: files{"calc/cgo.go": "package calc\n\n// #define N 1\nimport \"C\"\n"},
			after:  files{"calc/cgo.go": "package calc\n\n// #define N 2\nimport \"C\"\n"},
		},
		"a non-Go file": {
			before: files{"calc/limits.json": `{"max": 1}`},
			after:  files{"calc/limits.json": `{"max": 2}`},
		},
		// No name to pair it by.
		"a blank var": {
			before: files{"calc/vars.go": "package calc\n\nvar _ = Double\n"},
			after:  files{"calc/vars.go": "package calc\n\nvar _ = Triple\n"},
		},
		// And the same in a dependency, which every package built on it has
		// in scope.
		"an init in a dependency": {after: files{"vm/init.go": "package vm\n\nfunc init() {}\n"}},
		"a var initialised by a call in a dependency": {
			after: files{"vm/vars.go": "package vm\n\nvar Start = Size(nil)\n"},
		},
		"a build constraint in a dependency": {
			before: files{"vm/extra.go": "package vm\n"},
			after:  files{"vm/extra.go": "//go:build linux\n\npackage vm\n"},
		},
	}
	for name, c := range cases {
		c.want = both
		cases[name] = c
	}
	runChanges(t, cases)
}
