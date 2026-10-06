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
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
)

// The differential suite is the one that can see a mapping kept wrongly. Every
// other test of reuse drives a fake go command with canned profiles, so a kept
// mapping is checked against what the fake was told to say, never against what
// the test really executes after the change. Here the toolchain is real: each
// case maps a small module, applies an edit, maps it again from the cache, and
// maps it once more from nothing. The two maps after the edit must agree test
// for test, profile and dependency keys both — a mapping the reuse kept that a
// fresh run would not produce is exactly the false LIVED this whole mechanism
// has to avoid.
//
// Agreement alone would be satisfied by re-mapping everything, so each case
// also names the tests the cached run must have re-mapped: the narrowing it is
// there to exercise is asserted, not assumed.

// diffBase is the module every case starts from, before its own files.
var diffBase = map[string]string{
	"go.mod":            "module example.com/m\n\ngo 1.22\n",
	"calc/calc.go":      "package calc\n\nfunc Double(n int) int {\n\treturn n * 2\n}\n\nfunc Triple(n int) int {\n\treturn n * 3\n}\n",
	"calc/calc_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestDouble(t *testing.T) {\n\t_ = Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = Triple(2)\n}\n",
}

// diffRemoved, as a file's content, deletes it.
const diffRemoved = "\x00removed"

const diffPkg = "example.com/m/calc"

// diffCase is one edit, and the tests the cached run must map again after it.
//
// stale marks a known exposure (see "What it cannot see" in reusable): the
// cached run is expected to keep a mapping a fresh run would not produce. It
// is asserted rather than skipped, so that a change which closes the
// exposure is seen.
type diffCase struct {
	before   map[string]string
	after    map[string]string
	remapped []string
	stale    bool
}

// diffModule is one case's module on disk and the commands its runs issued.
type diffModule struct {
	t        *testing.T
	root     string
	ran      []string
	compiles int
}

func (m *diffModule) write(fs map[string]string) {
	m.t.Helper()

	for rel, content := range fs {
		path := filepath.Join(m.root, rel)
		if content == diffRemoved {
			if err := os.Remove(path); err != nil {
				m.t.Fatalf("cannot remove %s: %v", rel, err)
			}

			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			m.t.Fatalf("cannot create the directory of %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			m.t.Fatalf("cannot write %s: %v", rel, err)
		}
	}
}

// command runs the real toolchain, recording each test a run executes. The
// coverage directory an instrumented `go test` of this suite sets is dropped:
// the binaries built here write their own profiles.
func (m *diffModule) command(name string, args ...string) *exec.Cmd {
	if name == "go" && len(args) > 0 && args[0] == "test" && slices.Contains(args, "-c") {
		m.compiles++
	}
	for i, arg := range args {
		if arg == "-test.run" && i+1 < len(args) {
			m.ran = append(m.ran, strings.TrimSuffix(strings.TrimPrefix(args[i+1], "^"), "$"))
		}
	}
	cmd := exec.Command(name, args...) //nolint:gosec // the toolchain and the binaries it built
	cmd.Dir = m.root
	cmd.Env = append(os.Environ(), "GOCOVERDIR=", "GOWORK=off")

	return cmd
}

// diffMap is what one run left: each test's profile and its dependency keys.
type diffMap struct {
	profiles map[string]Profile
	deps     map[string][]string
	// baselined says the package counts toward the map's suite baseline
	// rather than being left to the coverage gather, and compiled is the
	// compile time it counts with.
	baselined bool
	compiled  time.Duration
}

// build maps the calc package against a cache directory and returns what the
// map holds for it, as the cache file records it after the run.
func (m *diffModule) build(cacheDir string) diffMap {
	m.t.Helper()

	m.ran = nil
	m.compiles = 0
	m.t.Chdir(m.root)
	c := NewWithCmd(m.command, m.t.TempDir(),
		gomodule.GoModule{Name: "example.com/m", Root: m.root, CallingDir: "calc"},
		WithTestMapCacheDir(cacheDir))
	tm, err := c.BuildTestMap()
	if err != nil {
		m.t.Fatalf("BuildTestMap() error: %v", err)
	}
	if !tm.Mapped(diffPkg) {
		m.t.Fatalf("%s was not mapped", diffPkg)
	}
	out := diffMap{profiles: map[string]Profile{}}
	for id, p := range tm.profiles {
		if id.Pkg == diffPkg {
			out.profiles[id.Name] = p
		}
	}
	dir, err := c.cacheDirPath(cacheKey(c.cacheScope(), c.buildTags))
	if err != nil {
		m.t.Fatalf("cannot locate the cache: %v", err)
	}
	entry, ok := loadCachedPackage(dir, diffPkg)
	if !ok {
		m.t.Fatalf("no cache entry was written for %s", diffPkg)
	}
	out.deps = entry.Deps
	_, rest := tm.SuiteBaseline()
	out.baselined = !slices.Contains(rest, diffPkg)
	out.compiled = tm.compiled[diffPkg]
	sort.Strings(m.ran)

	return out
}

func runDifferential(t *testing.T, cases map[string]diffCase) {
	t.Helper()

	if testing.Short() {
		t.Skip("builds and runs real test binaries")
	}
	log.Init(&bytes.Buffer{}, &bytes.Buffer{})
	t.Cleanup(log.Reset)

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := &diffModule{t: t, root: t.TempDir()}
			m.write(diffBase)
			m.write(c.before)
			cache := t.TempDir()
			m.build(cache)

			m.write(c.after)
			reused := m.build(cache)
			ran := m.ran
			scratch := m.build(t.TempDir())

			if c.stale {
				if cmp.Equal(scratch.profiles, reused.profiles, cmpopts.EquateEmpty()) {
					t.Errorf("a known exposure no longer keeps a stale mapping: update reusable's doc comment and this case")
				}

				return
			}
			if diff := cmp.Diff(scratch.profiles, reused.profiles, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("a mapping kept from the cache differs from a fresh one (-fresh +kept):\n%s", diff)
			}
			if diff := cmp.Diff(scratch.deps, reused.deps, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("dependency keys kept from the cache differ from fresh ones (-fresh +kept):\n%s", diff)
			}
			if diff := cmp.Diff(c.remapped, ran, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("re-mapped the wrong tests (-want +got):\n%s", diff)
			}
		})
	}
}

// diffCalc is calc.go with each function's body, and anything before them.
func diffCalc(header, double, triple string) string {
	return "package calc\n" + header + "\nfunc Double(n int) int {\n\t" + double +
		"\n}\n\nfunc Triple(n int) int {\n\t" + triple + "\n}\n"
}

var bothTests = []string{"TestDouble", "TestTriple"}

// regHead declares a registry that initialisers append to, and regDouble is a
// Double that reads which of them ran first.
const (
	regHead   = "package calc\n\nvar order []string\n\nfunc reg(s string) int {\n\torder = append(order, s)\n\treturn 0\n}\n\n"
	regDouble = "if order[0] == \"b\" {\n\t\treturn 1\n\t}\n\treturn n * 2"
)

// The changes the closure narrows: the cached run maps again only the tests a
// change could reach, and what it keeps is what a fresh run produces.
func TestDifferentialNarrowedChanges(t *testing.T) {
	vm := func(clamp string) string {
		return "package vm\n\nfunc Clamp(n int) int {\n\t" + clamp + "\n}\n\nfunc Size(v []int) int {\n\treturn len(v)\n}\n"
	}
	describer := "package calc\n\ntype counter int\n\nfunc describe(v any) int {\n\tif _, ok := v.(interface{ String() string }); ok {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"
	handler := func(twice string) string {
		return "package calc\n\nimport \"reflect\"\n\n" + twice +
			"\n\nfunc register(h any) int {\n\tif reflect.TypeOf(h).NumIn() > 1 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"
	}
	point := func(tag string) string {
		return "package calc\n\nimport \"reflect\"\n\ntype point struct {\n\tX int `json:\"" + tag +
			"\"`\n}\n\nfunc size(p point) int {\n\tif reflect.TypeOf(p).Field(0).Tag.Get(\"json\") == \"y\" {\n\t\treturn 1\n\t}\n\treturn p.X\n}\n"
	}
	helper := func(body string) string {
		return "package calc\n\nimport \"testing\"\n\nfunc want(t *testing.T, got int) {\n\tt.Helper()\n\t" + body +
			"\n}\n\nfunc TestDouble(t *testing.T) {\n\t_ = Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\twant(t, Triple(2))\n}\n"
	}
	runDifferential(t, map[string]diffCase{
		"an added type and function": {
			after: map[string]string{"calc/extra.go": "package calc\n\ntype unit int\n\nfunc Quadruple(n unit) unit {\n\treturn n * 4\n}\n"},
		},
		"a constant one function uses": {
			before: map[string]string{
				"calc/calc.go":   diffCalc("", "if factor > 2 {\n\t\treturn n * factor\n\t}\n\treturn n * 2", "return n * 3"),
				"calc/consts.go": "package calc\n\nconst factor = 2\n",
			},
			after:    map[string]string{"calc/consts.go": "package calc\n\nconst factor = 3\n"},
			remapped: []string{"TestDouble"},
		},
		"a name inserted into a const group under iota": {
			before: map[string]string{
				"calc/calc.go":   diffCalc("", "if b > 1 {\n\t\treturn n\n\t}\n\treturn n * 2", "return n * 3"),
				"calc/consts.go": "package calc\n\nconst (\n\ta = iota\n\tb\n)\n",
			},
			after:    map[string]string{"calc/consts.go": "package calc\n\nconst (\n\tpad = iota\n\ta\n\tb\n)\n"},
			remapped: []string{"TestDouble"},
		},
		"a struct tag read by reflection": {
			before: map[string]string{
				"calc/calc.go":  diffCalc("", "return n*2 + size(point{})", "return n * 3"),
				"calc/types.go": point("x"),
			},
			after:    map[string]string{"calc/types.go": point("y")},
			remapped: []string{"TestDouble"},
		},
		"an added method that satisfies an interface": {
			before: map[string]string{
				"calc/calc.go":  diffCalc("", "return describe(counter(n))", "return n * 3"),
				"calc/types.go": describer,
			},
			after:    map[string]string{"calc/types.go": describer + "\nfunc (c counter) String() string {\n\treturn \"\"\n}\n"},
			remapped: []string{"TestDouble"},
		},
		"a handler's signature, the handler passed as a value": {
			before: map[string]string{
				"calc/calc.go":    diffCalc("", "return register(twice)", "return n * 3"),
				"calc/handler.go": handler("func twice(n int) int {\n\treturn n * 2\n}"),
			},
			after:    map[string]string{"calc/handler.go": handler("func twice(n int, _ int) int {\n\treturn n * 2\n}")},
			remapped: []string{"TestDouble"},
		},
		"a handler's body, the handler passed as a value": {
			before: map[string]string{
				"calc/calc.go":    diffCalc("", "return register(twice)", "return n * 3"),
				"calc/handler.go": handler("func twice(n int) int {\n\treturn n * 2\n}"),
			},
			after: map[string]string{"calc/handler.go": handler("func twice(n int) int {\n\treturn n + n\n}")},
		},
		// Both packages are linked before and after, so only the name moved.
		"an import swapped under the same name": {
			before: map[string]string{
				"vm/vm.go":      vm("return n"),
				"vm2/vm2.go":    "package vm2\n\nfunc Size(v []int) int {\n\treturn 2 * len(v)\n}\n",
				"calc/calc.go":  diffCalc("\nimport st \"example.com/m/vm\"\n", "return st.Size(nil) + n*2", "return n * 3"),
				"calc/other.go": "package calc\n\nimport (\n\t\"example.com/m/vm\"\n\t\"example.com/m/vm2\"\n)\n\nfunc other() int {\n\treturn vm.Size(nil) + vm2.Size(nil)\n}\n",
			},
			after:    map[string]string{"calc/calc.go": diffCalc("\nimport st \"example.com/m/vm2\"\n", "return st.Size(nil) + n*2", "return n * 3")},
			remapped: []string{"TestDouble"},
		},
		"a //go:embed target": {
			before: map[string]string{
				"calc/calc.go": diffCalc("", "if len(data) > 1 {\n\t\treturn n\n\t}\n\treturn n * 2", "return n * 3"),
				"calc/data.go": "package calc\n\nimport _ \"embed\"\n\n//go:embed a.txt\nvar data string\n",
				"calc/a.txt":   "a",
				"calc/b.txt":   "bb",
			},
			after:    map[string]string{"calc/data.go": "package calc\n\nimport _ \"embed\"\n\n//go:embed b.txt\nvar data string\n"},
			remapped: []string{"TestDouble"},
		},
		"a predeclared name shadowed": {
			before:   map[string]string{"calc/calc.go": diffCalc("", "return min(n*2, 100)", "return n * 3")},
			after:    map[string]string{"calc/shadow.go": "package calc\n\nfunc min(a, b int) int {\n\tif a > b {\n\t\treturn b\n\t}\n\treturn a\n}\n"},
			remapped: []string{"TestDouble"},
		},
		"a dependency function's body": {
			before: map[string]string{
				"vm/vm.go":     vm("return n"),
				"calc/calc.go": diffCalc("\nimport \"example.com/m/vm\"\n", "return vm.Clamp(n) * 2", "return n*3 + vm.Size(nil)"),
			},
			after:    map[string]string{"vm/vm.go": vm("if n > 100 {\n\t\treturn 100\n\t}\n\treturn n")},
			remapped: []string{"TestDouble"},
		},
		"a test helper's body": {
			before:   map[string]string{"calc/calc_test.go": helper("_ = got")},
			after:    map[string]string{"calc/calc_test.go": helper("if got < 0 {\n\t\tt.Fail()\n\t}")},
			remapped: []string{"TestTriple"},
		},
		// Every operand is a literal or a stdlib error, so nothing of the
		// package runs while the variables are initialised.
		"error variables built from literals": {
			before: map[string]string{
				"calc/calc.go": diffCalc("", "return n*2 + len(errBad.Error())", "return n * 3"),
				"calc/errs.go": "package calc\n\nimport \"errors\"\n\nvar errBad = errors.New(\"bad\")\n",
			},
			after: map[string]string{"calc/errs.go": "package calc\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n\t\"regexp\"\n)\n\n" +
				"var errBad = errors.New(\"worse\")\n\nvar errNew = fmt.Errorf(\"new %d: %w\", 1, errBad)\n\nvar pattern = regexp.MustCompile(\"x+\")\n"},
			remapped: []string{"TestDouble"},
		},
	})
}

// The holes an adversarial review proved against the closure, each a change
// that reaches a test by a route no name it mentions carries.
func TestDifferentialChangesNoNameCarries(t *testing.T) {
	regFirst := func(h string) string {
		return regHead + "var a = first()\n\nvar b = reg(\"b\")\n\nfunc first() int {\n\tif len(order) > 1000 {\n\t\th()\n\t}\n\treturn reg(\"a\")\n}\n\nfunc h() {\n\t" + h + "\n}\n"
	}
	item := "\ntype item struct{}\n\nfunc (item) String() string {\n\treturn \"item\"\n}\n"
	hashing := diffCalc("\nimport \"crypto\"\n", "if crypto.MD5.Available() {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3")
	runDifferential(t, map[string]diffCase{
		// fmt.Errorf formats its operands at initialisation, calling their
		// String methods.
		"fmt.Errorf formatting a value with a String method": {
			before:   map[string]string{"calc/errs.go": "package calc\n\nimport \"fmt\"\n\nvar errX = fmt.Errorf(\"x\")\n" + item},
			after:    map[string]string{"calc/errs.go": "package calc\n\nimport \"fmt\"\n\nvar errX = fmt.Errorf(\"%v\", item{})\n" + item},
			remapped: bothTests,
		},
		// The method is declared on the alias and belongs to box, which is
		// the name Double mentions.
		"a method declared through an alias": {
			before: map[string]string{
				"calc/calc.go": diffCalc("\nimport \"fmt\"\n", "return n*2 + len(fmt.Sprint(box{}))", "return n * 3"),
				"calc/box.go":  "package calc\n\ntype box struct{}\n\ntype Alias = box\n",
			},
			after:    map[string]string{"calc/box.go": "package calc\n\ntype box struct{}\n\ntype Alias = box\n\nfunc (Alias) String() string {\n\treturn \"alias\"\n}\n"},
			remapped: []string{"TestDouble"},
		},
		// The other direction: the method is declared on box, and Double
		// names only the alias.
		"a method declared on the type an alias names": {
			before: map[string]string{
				"calc/calc.go": diffCalc("\nimport \"fmt\"\n", "return n*2 + len(fmt.Sprint(Alias{}))", "return n * 3"),
				"calc/box.go":  "package calc\n\ntype box struct{}\n\ntype Alias = box\n",
			},
			after:    map[string]string{"calc/box.go": "package calc\n\ntype box struct{}\n\ntype Alias = box\n\nfunc (box) String() string {\n\treturn \"box\"\n}\n"},
			remapped: []string{"TestDouble"},
		},
		// Across packages: the method is declared on vm.Box, and Double
		// names only calc's alias of it.
		"a method declared on another package's type a local alias names": {
			before: map[string]string{
				"vm/vm.go":     "package vm\n\ntype Box struct{}\n",
				"calc/box.go":  "package calc\n\nimport \"example.com/m/vm\"\n\ntype Alias = vm.Box\n",
				"calc/calc.go": diffCalc("\nimport \"fmt\"\n", "return n*2 + len(fmt.Sprint(Alias{}))", "return n * 3"),
			},
			after:    map[string]string{"vm/vm.go": "package vm\n\ntype Box struct{}\n\nfunc (Box) String() string {\n\treturn \"box\"\n}\n"},
			remapped: []string{"TestDouble"},
		},
		// crypto/md5 registers MD5 when it is initialised, and Double asks
		// whether it is registered; a test file linking it changes Double.
		"a test file linking a package with an init": {
			before: map[string]string{"calc/calc.go": hashing},
			after: map[string]string{"calc/calc_test.go": "package calc\n\nimport (\n\t\"crypto/md5\"\n\t\"testing\"\n)\n\n" +
				"func TestDouble(t *testing.T) {\n\t_ = Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = Triple(md5.Size)\n}\n"},
			remapped: bothTests,
		},
		"a dependency linking a package with an init": {
			before: map[string]string{
				"calc/calc.go": diffCalc("\nimport (\n\t\"crypto\"\n\n\t\"example.com/m/vm\"\n)\n",
					"if crypto.MD5.Available() {\n\t\treturn 1\n\t}\n\treturn n * 2", "return vm.Size(nil)"),
				"vm/vm.go": "package vm\n\nfunc Size(v []int) int {\n\treturn len(v)\n}\n",
			},
			after:    map[string]string{"vm/hash.go": "package vm\n\nimport \"crypto/md5\"\n\nfunc Sum() int {\n\treturn md5.Size\n}\n"},
			remapped: bothTests,
		},
		// Package-level variables are initialised in declaration order,
		// subject to their dependencies: swapping two whose initialisers have
		// effects changes what both see, and no name of either changed.
		"two initialisers with effects swapped": {
			before:   map[string]string{"calc/calc.go": diffCalc("", regDouble, "return n * 3"), "calc/reg.go": regHead + "var a = reg(\"a\")\nvar b = reg(\"b\")\n"},
			after:    map[string]string{"calc/reg.go": regHead + "var b = reg(\"b\")\nvar a = reg(\"a\")\n"},
			remapped: bothTests,
		},
		// The dependency is found through a function body that never runs:
		// h now mentions b, so a, which calls first, which mentions h, waits
		// for b. The order moved, and the only change is to a body no test
		// executed.
		"an unexecuted body that adds an initialisation dependency": {
			before:   map[string]string{"calc/calc.go": diffCalc("", regDouble, "return n * 3"), "calc/reg.go": regFirst("_ = 0")},
			after:    map[string]string{"calc/reg.go": regFirst("_ = b")},
			remapped: bothTests,
		},
		"a method declared through a parenthesised alias": {
			before: map[string]string{
				"calc/calc.go": diffCalc("\nimport \"fmt\"\n", "return n*2 + len(fmt.Sprint(box{}))", "return n * 3"),
				"calc/box.go":  "package calc\n\ntype box struct{}\n\ntype A = (box)\n",
			},
			after:    map[string]string{"calc/box.go": "package calc\n\ntype box struct{}\n\ntype A = (box)\n\nfunc (A) String() string {\n\treturn \"alias\"\n}\n"},
			remapped: []string{"TestDouble"},
		},
		// z registers itself when initialised, calc's init reads the
		// registry, and z is linked before and after: only the order the two
		// are initialised in changes, because calc now imports z.
		"an import that reorders initialisation": {
			before: map[string]string{
				"reg/reg.go":   "package reg\n\nvar Names []string\n",
				"z/z.go":       "package z\n\nimport \"example.com/m/reg\"\n\nfunc init() {\n\treg.Names = append(reg.Names, \"z\")\n}\n\nfunc Name() string {\n\treturn \"z\"\n}\n",
				"calc/init.go": "package calc\n\nimport \"example.com/m/reg\"\n\nvar seen int\n\nfunc init() {\n\tseen = len(reg.Names)\n}\n",
				"calc/calc.go": diffCalc("\n// Name is for the external test.\nfunc Name() string {\n\treturn \"calc\"\n}\n", "if seen > 0 {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3"),
				"calc/x_test.go": "package calc_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/calc\"\n\t\"example.com/m/z\"\n)\n\n" +
					"func TestDouble(t *testing.T) {\n\t_ = calc.Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = calc.Triple(2)\n}\n\n" +
					"func TestZ(t *testing.T) {\n\t_ = z.Name() + calc.Name()\n}\n",
				"calc/calc_test.go": diffRemoved,
			},
			after:    map[string]string{"calc/name.go": "package calc\n\nimport \"example.com/m/z\"\n\nfunc name() string {\n\treturn z.Name()\n}\n"},
			remapped: []string{"TestDouble", "TestTriple", "TestZ"},
		},
	})
}

// A test file is entities like any other: adding, removing or renaming one is
// attributed by what it declares, and by what it links.
func TestDifferentialTestFiles(t *testing.T) {
	runDifferential(t, map[string]diffCase{
		"a new test file holding only tests": {
			after:    map[string]string{"calc/more_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestMore(t *testing.T) {\n\t_ = Double(3)\n}\n"},
			remapped: []string{"TestMore"},
		},
		"a new test file linking a package with an init": {
			before: map[string]string{"calc/calc.go": diffCalc("\nimport \"crypto\"\n",
				"if crypto.MD5.Available() {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3")},
			after: map[string]string{"calc/hash_test.go": "package calc\n\nimport (\n\t\"crypto/md5\"\n\t\"testing\"\n)\n\n" +
				"func TestHash(t *testing.T) {\n\t_ = md5.Size\n}\n"},
			remapped: []string{"TestDouble", "TestHash", "TestTriple"},
		},
		"a file renamed to a test file": {
			before: map[string]string{
				"calc/helper.go":    "package calc\n\nfunc helper() int {\n\treturn 2\n}\n",
				"calc/calc_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestDouble(t *testing.T) {\n\t_ = Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = Triple(helper())\n}\n",
			},
			after: map[string]string{
				"calc/helper.go":      diffRemoved,
				"calc/helper_test.go": "package calc\n\nfunc helper() int {\n\treturn 2\n}\n",
			},
			remapped: []string{"TestTriple"},
		},
	})
}

// Gremlins' own build tags decide what the test binary is built from, and so
// what every listing and type-check behind reuse has to read.
func TestDifferentialBuildTags(t *testing.T) {
	viper.Set(configuration.UnleashTagsKey, "extra")
	t.Cleanup(func() { viper.Set(configuration.UnleashTagsKey, "") })
	vm := func(clamp string) string { return "package vm\n\nfunc Clamp(n int) int {\n\t" + clamp + "\n}\n" }
	tagged := "//go:build extra\n\npackage calc\n\n"
	runDifferential(t, map[string]diffCase{
		"a dependency linked only by a tagged file": {
			before: map[string]string{
				"vm/vm.go":      vm("return n"),
				"calc/extra.go": "//go:build extra\n\npackage calc\n\nimport \"example.com/m/vm\"\n\nfunc via(n int) int {\n\treturn vm.Clamp(n)\n}\n",
				"calc/calc.go":  diffCalc("", "if via(n) > 50 {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3"),
			},
			after:    map[string]string{"vm/vm.go": vm("if n > 1 {\n\t\treturn 100\n\t}\n\treturn n")},
			remapped: []string{"TestDouble"},
		},
		// The variables are compiled only under the tag, and the package
		// type-checks without them, so a type-check without the tag would
		// see no initialiser at all, before or after.
		"initialisers swapped in a tagged file": {
			before: map[string]string{
				"calc/calc.go": diffCalc("", "if len(order) > 0 && "+strings.TrimPrefix(regDouble, "if "), "return n * 3"),
				"calc/regs.go": regHead,
				"calc/vars.go": tagged + "var a = reg(\"a\")\nvar b = reg(\"b\")\n",
			},
			after:    map[string]string{"calc/vars.go": tagged + "var b = reg(\"b\")\nvar a = reg(\"a\")\n"},
			remapped: bothTests,
		},
	})
}

// The order packages are initialised in is decided by the import graph, which
// no package's type-check sees: what each package reaches by import is listed
// beside it, and a change to it re-maps when anything initialised can see it.
func TestDifferentialPackageInitOrder(t *testing.T) {
	zmod := map[string]string{
		"go.mod":            "module example.com/m\n\ngo 1.22\n\nrequire example.com/z v0.0.0\n\nreplace example.com/z => ./zmod\n",
		"zmod/go.mod":       "module example.com/z\n\ngo 1.22\n",
		"zmod/reg/reg.go":   "package reg\n\nvar Names []string\n",
		"zmod/z.go":         "package z\n\nimport \"example.com/z/reg\"\n\nfunc init() {\n\treg.Names = append(reg.Names, \"z\")\n}\n\nfunc Name() string {\n\treturn \"z\"\n}\n",
		"calc/init.go":      "package calc\n\nimport \"example.com/z/reg\"\n\nvar seen = len(reg.Names)\n",
		"calc/calc.go":      diffCalc("\n// Name is for the external test.\nfunc Name() string {\n\treturn \"calc\"\n}\n", "if seen > 0 {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3"),
		"calc/calc_test.go": diffRemoved,
		"calc/x_test.go": "package calc_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/calc\"\n\t\"example.com/z\"\n)\n\n" +
			"func TestDouble(t *testing.T) {\n\t_ = calc.Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = calc.Triple(2)\n}\n\n" +
			"func TestZ(t *testing.T) {\n\t_ = z.Name() + calc.Name()\n}\n",
	}
	runDifferential(t, map[string]diffCase{
		// The writer, z's init, is in a module that is not instrumented, so
		// only the plain var reading reg.Names says the order is observed.
		"an order-observing plain var whose writer is not instrumented": {
			before:   zmod,
			after:    map[string]string{"calc/name.go": "package calc\n\nimport \"example.com/z\"\n\nfunc name() string {\n\treturn z.Name()\n}\n"},
			remapped: []string{"TestDouble", "TestTriple", "TestZ"},
		},
		// strings.NewReplacer runs at initialisation, but calc already
		// reached vm and strings, so it is initialised where it was.
		"a test file importing what the package already reached, beside a var run at initialisation": {
			before: map[string]string{
				"vm/vm.go":     "package vm\n\nfunc Clamp(n int) int {\n\treturn n\n}\n",
				"calc/calc.go": diffCalc("\nimport \"example.com/m/vm\"\n", "return vm.Clamp(n) * 2", "return n * 3"),
				"calc/rep.go":  "package calc\n\nimport \"strings\"\n\nvar rep = strings.NewReplacer(\"a\", \"b\")\n",
			},
			after: map[string]string{"calc/calc_test.go": "package calc\n\nimport (\n\t\"strings\"\n\t\"testing\"\n\n\t\"example.com/m/vm\"\n)\n\n" +
				"func TestDouble(t *testing.T) {\n\t_ = Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = Triple(vm.Clamp(len(strings.Repeat(\"x\", 2))))\n}\n"},
			remapped: []string{"TestTriple"},
		},
		// The same shape, but the test file reaches z, which calc did not,
		// so calc is now initialised after z's init and count sees it.
		"a test file reaching a new package, beside a var run at initialisation": {
			before: map[string]string{
				"reg/reg.go":   "package reg\n\nvar Names []string\n",
				"z/z.go":       "package z\n\nimport \"example.com/m/reg\"\n\nfunc init() {\n\treg.Names = append(reg.Names, \"z\")\n}\n\nfunc Name() string {\n\treturn \"z\"\n}\n",
				"calc/run.go":  "package calc\n\nimport \"example.com/m/reg\"\n\nvar seen = count()\n\nfunc count() int {\n\treturn len(reg.Names)\n}\n",
				"calc/calc.go": diffCalc("", "if seen > 0 {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3"),
				"calc/x_test.go": "package calc_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/z\"\n)\n\n" +
					"func TestZ(t *testing.T) {\n\t_ = z.Name()\n}\n",
			},
			after: map[string]string{"calc/calc_test.go": "package calc\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/z\"\n)\n\n" +
				"func TestDouble(t *testing.T) {\n\t_ = Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = Triple(len(z.Name()))\n}\n"},
			remapped: []string{"TestDouble", "TestTriple", "TestZ"},
		},
		// Imports moved, but nothing instrumented can see when anything was
		// initialised, so nothing is re-mapped.
		"imports changed where nothing observes the order": {
			before: map[string]string{
				"vm/vm.go":     "package vm\n\nfunc Clamp(n int) int {\n\treturn n\n}\n",
				"calc/calc.go": diffCalc("\nimport \"example.com/m/vm\"\n", "return vm.Clamp(n) * 2", "return n * 3"),
			},
			after: map[string]string{"calc/other.go": "package calc\n\nimport \"example.com/m/vm\"\n\nfunc other() int {\n\treturn vm.Clamp(1)\n}\n"},
		},
	})
}

// A var reads another package's state through a name no selector carries: a
// dot import, or a same-package var holding its address. Only the
// type-checker can say what such a name is, so it is asked. With an internal
// TestMain, and z's module sorting after testing, calc is initialised before
// z only while it does not reach it — TestMain itself observes nothing.
func TestDifferentialPackageInitOrderUnqualifiedReads(t *testing.T) {
	zmod := func(zpath, initGo string, testMain bool) map[string]string {
		fs := map[string]string{
			"go.mod":            "module example.com/m\n\ngo 1.22\n\nrequire " + zpath + " v0.0.0\n\nreplace " + zpath + " => ./zmod\n",
			"zmod/go.mod":       "module " + zpath + "\n\ngo 1.22\n",
			"zmod/reg/reg.go":   "package reg\n\nvar Names []string\n",
			"zmod/z.go":         "package z\n\nimport \"" + zpath + "/reg\"\n\nfunc init() {\n\treg.Names = append(reg.Names, \"z\")\n}\n\nfunc Name() string {\n\treturn \"z\"\n}\n",
			"calc/init.go":      "package calc\n\nimport . \"" + zpath + "/reg\"\n\n" + initGo,
			"calc/calc.go":      diffCalc("\n// Name is for the external test.\nfunc Name() string {\n\treturn \"calc\"\n}\n", "if seen > 0 {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3"),
			"calc/calc_test.go": diffRemoved,
			"calc/x_test.go": "package calc_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/calc\"\n\t\"" + zpath + "\"\n)\n\n" +
				"func TestDouble(t *testing.T) {\n\t_ = calc.Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = calc.Triple(2)\n}\n\n" +
				"func TestZ(t *testing.T) {\n\t_ = z.Name() + calc.Name()\n}\n",
		}
		if testMain {
			fs["calc/main_test.go"] = "package calc\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) {\n\tos.Exit(m.Run())\n}\n"
		}

		return fs
	}
	name := func(zpath string) map[string]string {
		return map[string]string{"calc/name.go": "package calc\n\nimport \"" + zpath + "\"\n\nfunc name() string {\n\treturn z.Name()\n}\n"}
	}
	dot := "var seen = len(Names)\n"
	alias := "var names = &Names\n\nvar seen = len(*names)\n"
	all := []string{"TestDouble", "TestTriple", "TestZ"}
	runDifferential(t, map[string]diffCase{
		"a plain var reading another package through a dot import": {
			before: zmod("example.com/z", dot, false), after: name("example.com/z"), remapped: all,
		},
		"a plain var reading another package through a dot import, beside TestMain": {
			before: zmod("zz.example/z", dot, true), after: name("zz.example/z"), remapped: all,
		},
		"a plain var reading another package through a pointer to it": {
			before: zmod("example.com/z", alias, false), after: name("example.com/z"), remapped: all,
		},
		"a plain var reading another package through a pointer to it, beside TestMain": {
			before: zmod("zz.example/z", alias, true), after: name("zz.example/z"), remapped: all,
		},
	})
}

// What reuse is known not to see, asserted so that it stays documented.
func TestDifferentialKnownExposures(t *testing.T) {
	caller := "_, _, line, _ := runtime.Caller(0)\n\tif line > 7 {\n\t\treturn 1\n\t}\n\treturn n * 2"
	runDifferential(t, map[string]diffCase{
		// The constant moves Double down two lines without changing its
		// print, and Double branches on its own line number.
		"an absolute line read by runtime.Caller": {
			before: map[string]string{"calc/calc.go": diffCalc("\nimport \"runtime\"\n", caller, "return n * 3")},
			after:  map[string]string{"calc/calc.go": diffCalc("\nimport \"runtime\"\n\nconst pad = 1\n", caller, "return n * 3")},
			stale:  true,
		},
	})
}

// A whole-package re-map is the expensive outcome, so the run says which
// package it was and what forced it.
func TestDifferentialAWholeRemapLogsItsReason(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs real test binaries")
	}
	log.Reset()
	var out bytes.Buffer
	log.Init(&out, &bytes.Buffer{})
	t.Cleanup(log.Reset)

	m := &diffModule{t: t, root: t.TempDir()}
	m.write(diffBase)
	cache := t.TempDir()
	m.build(cache)
	m.write(map[string]string{"calc/extra.go": "//go:build !never\n\npackage calc\n\nfunc extra() int {\n\treturn 1\n}\n"})
	out.Reset()
	m.build(cache)

	want := "testmap: re-mapping all of " + diffPkg + ": the package: something outside every declaration changed"
	if !strings.Contains(out.String(), want) {
		t.Errorf("want the log to say %q, got:\n%s", want, out.String())
	}
}

// TestMain runs after every package is initialised, so it sees the order
// only through what the initialisers left behind, which is what the
// observers the rule counts already decide. A test file's imports moving
// beside it re-maps what the change reaches, and an initialiser that sees
// the order still re-maps everything.
func TestDifferentialTestMain(t *testing.T) {
	mainTest := "package calc\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) {\n\tos.Exit(m.Run())\n}\n"
	vm := "package vm\n\nfunc Clamp(n int) int {\n\treturn n\n}\n"
	calcVM := diffCalc("\nimport \"example.com/m/vm\"\n", "return vm.Clamp(n) * 2", "return n * 3")
	tests := func(imports, triple string) string {
		return "package calc\n\nimport (\n" + imports + "\t\"testing\"\n)\n\n" +
			"func TestDouble(t *testing.T) {\n\t_ = Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t" + triple + "\n}\n"
	}
	// z registers itself when initialised, and is linked through the
	// external test; calc's internal test newly importing it moves calc
	// after z.
	reg := map[string]string{
		"reg/reg.go":        "package reg\n\nvar Names []string\n",
		"z/z.go":            "package z\n\nimport \"example.com/m/reg\"\n\nfunc init() {\n\treg.Names = append(reg.Names, \"z\")\n}\n\nfunc Name() string {\n\treturn \"z\"\n}\n",
		"calc/main_test.go": mainTest,
		"calc/x_test.go": "package calc_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/z\"\n)\n\n" +
			"func TestZ(t *testing.T) {\n\t_ = z.Name()\n}\n",
	}
	with := func(base map[string]string, more map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range more {
			out[k] = v
		}

		return out
	}
	importZ := map[string]string{"calc/calc_test.go": tests("\t\"example.com/m/z\"\n", "_ = Triple(len(z.Name()))")}
	runDifferential(t, map[string]diffCase{
		"a test file importing an already linked package beside TestMain": {
			before:   map[string]string{"vm/vm.go": vm, "calc/calc.go": calcVM, "calc/main_test.go": mainTest},
			after:    map[string]string{"calc/calc_test.go": tests("\t\"strings\"\n", "_ = Triple(len(strings.Repeat(\"x\", 2)))")},
			remapped: []string{"TestTriple"},
		},
		"a new test file importing an instrumented package beside TestMain": {
			before: map[string]string{"vm/vm.go": vm, "calc/calc.go": calcVM, "calc/main_test.go": mainTest},
			after: map[string]string{"calc/more_test.go": "package calc\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/vm\"\n)\n\n" +
				"func TestMore(t *testing.T) {\n\t_ = vm.Clamp(1)\n}\n"},
			remapped: []string{"TestMore"},
		},
		// calc now reaches z and is initialised after it, which nothing
		// initialised sees: TestMain runs after both, and here z has no
		// init (an instrumented one would count as seeing the order).
		"a test file reaching a new package beside TestMain": {
			before:   with(reg, map[string]string{"z/z.go": "package z\n\nfunc Name() string {\n\treturn \"z\"\n}\n"}),
			after:    importZ,
			remapped: []string{"TestTriple"},
		},
		// calc's init and z's both append to reg.Names, and Double reads
		// which came first.
		"an init that sees the order, beside TestMain": {
			before: with(reg, map[string]string{
				"calc/init.go": "package calc\n\nimport \"example.com/m/reg\"\n\nfunc init() {\n\treg.Names = append(reg.Names, \"calc\")\n}\n",
				"calc/calc.go": diffCalc("\nimport \"example.com/m/reg\"\n", "if reg.Names[0] == \"z\" {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3"),
			}),
			after:    importZ,
			remapped: []string{"TestDouble", "TestTriple", "TestZ"},
		},
		// The observer is a test file's plain var, read when calc is
		// initialised: before z's init, and then after it.
		"a test file's var that sees the order, beside TestMain": {
			before: with(reg, map[string]string{
				"calc/seen_test.go": "package calc\n\nimport \"example.com/m/reg\"\n\nvar seen = len(reg.Names)\n",
				"calc/calc_test.go": "package calc\n\nimport \"testing\"\n\n" +
					"func TestDouble(t *testing.T) {\n\t_ = Double(2 + seen)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = Triple(2)\n}\n",
				"calc/calc.go": diffCalc("", "if n > 2 {\n\t\treturn 1\n\t}\n\treturn n * 2", "return n * 3"),
			}),
			after: map[string]string{"calc/calc_test.go": "package calc\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/z\"\n)\n\n" +
				"func TestDouble(t *testing.T) {\n\t_ = Double(2 + seen)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = Triple(len(z.Name()))\n}\n"},
			remapped: []string{"TestDouble", "TestTriple", "TestZ"},
		},
	})
}

// A method is a change to its receiver type, not to every call of a method
// so spelled: what reaches it is whatever names the type.
func TestDifferentialMethods(t *testing.T) {
	rec := "package calc\n\nimport \"testing\"\n\ntype rec struct {\n\ttesting.TB\n}\n\n" +
		"func TestRec(t *testing.T) {\n\tr := &rec{TB: t}\n\tr.Logf(\"%d\", Double(1))\n}\n"
	box := "package calc\n\ntype box struct{}\n\nfunc mk() any {\n\treturn box{}\n}\n"
	boxTests := "package calc\n\nimport (\n\t\"fmt\"\n\t\"testing\"\n)\n\nfunc mkT() any {\n\treturn box{}\n}\n\n" +
		"func TestDouble(t *testing.T) {\n\t_ = Double(2)\n}\n\nfunc TestTriple(t *testing.T) {\n\t_ = Triple(len(fmt.Sprint(mkT())))\n}\n\n" +
		"func TestPlain(t *testing.T) {\n\tt.Logf(\"%d\", Triple(1))\n}\n"
	runDifferential(t, map[string]diffCase{
		"a test type gaining a method every test calls by that name": {
			before: map[string]string{
				"calc/calc_test.go": "package calc\n\nimport \"testing\"\n\n" +
					"func TestDouble(t *testing.T) {\n\tt.Logf(\"%d\", Double(2))\n}\n\nfunc TestTriple(t *testing.T) {\n\tt.Logf(\"%d\", Triple(2))\n}\n",
				"calc/rec_test.go": rec,
			},
			after:    map[string]string{"calc/rec_test.go": rec + "\nfunc (r *rec) Logf(format string, args ...any) {}\n"},
			remapped: []string{"TestRec"},
		},
		// Double reaches a box only through mk, a body it executes, and
		// TestTriple only through mkT, a helper it calls; neither names box.
		"a method a value reaches through code that names its type": {
			before: map[string]string{
				"calc/calc.go":      diffCalc("\nimport \"fmt\"\n", "return n*2 + len(fmt.Sprint(mk()))", "return n * 3"),
				"calc/box.go":       box,
				"calc/calc_test.go": boxTests,
			},
			after:    map[string]string{"calc/box.go": box + "\nfunc (box) String() string {\n\treturn \"box\"\n}\n"},
			remapped: []string{"TestDouble", "TestTriple"},
		},
	})
}

// A package whose fingerprint and build inputs equal its cache entry's is
// served from the entry without compiling its tests: the build ID the compile
// would yield was the only thing left to learn, and with both equal a moved one
// means only a moved checkout (see buildInputsOf). A warm map of many packages
// otherwise spends nearly all its time compiling binaries it then discards.
func TestDifferentialAnUnchangedPackageIsNotCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs real test binaries")
	}
	log.Init(&bytes.Buffer{}, &bytes.Buffer{})
	t.Cleanup(log.Reset)

	// What was served must be what a fresh map makes, byte for byte: equal
	// fingerprints mean no line moved, so nothing may be shifted.
	sameAsFresh := func(t *testing.T, served, fresh diffMap) {
		t.Helper()
		got, err := json.Marshal(served.profiles)
		if err != nil {
			t.Fatal(err)
		}
		want, err := json.Marshal(fresh.profiles)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("served profiles differ from a fresh map:\nserved %s\nfresh  %s", got, want)
		}
		if diff := cmp.Diff(fresh.deps, served.deps, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("served dependency keys differ from fresh ones (-fresh +served):\n%s", diff)
		}
	}

	t.Run("files touched but not changed", func(t *testing.T) {
		m := &diffModule{t: t, root: t.TempDir()}
		m.write(diffBase)
		cache := t.TempDir()
		mapped := m.build(cache)

		later := time.Now().Add(time.Hour)
		for rel := range diffBase {
			if err := os.Chtimes(filepath.Join(m.root, rel), later, later); err != nil {
				t.Fatal(err)
			}
		}
		served := m.build(cache)
		if m.compiles != 0 || len(m.ran) != 0 {
			t.Fatalf("an unchanged package compiled %d times and ran %v, want neither", m.compiles, m.ran)
		}
		if !served.baselined {
			t.Error("a package served without compiling dropped out of the suite baseline, so the gather would run it")
		}
		// It reports the compile time recorded when it was mapped: the
		// baseline the per-mutant timeout is derived from must not lose it.
		if served.compiled <= 0 || served.compiled != mapped.compiled {
			t.Errorf("served compile time %v, want the %v recorded when it was mapped", served.compiled, mapped.compiled)
		}
		sameAsFresh(t, served, m.build(t.TempDir()))
	})

	// The cache is shared by runners whose checkouts live in different
	// directories, so a moved checkout must be served the same way.
	t.Run("the same module in another checkout", func(t *testing.T) {
		first := &diffModule{t: t, root: t.TempDir()}
		first.write(diffBase)
		cache := t.TempDir()
		first.build(cache)

		moved := &diffModule{t: t, root: t.TempDir()}
		moved.write(diffBase)
		served := moved.build(cache)
		if moved.compiles != 0 || len(moved.ran) != 0 {
			t.Fatalf("a moved checkout compiled %d times and ran %v, want neither", moved.compiles, moved.ran)
		}
		sameAsFresh(t, served, moved.build(t.TempDir()))
	})

	// Equal prints are not enough on their own: a print records the lines its
	// entity occupies, so a line moved without a print changing still makes
	// the fingerprint differ, and the package is compiled and its mappings
	// shifted rather than served as they stand.
	t.Run("a blank line moves code, so it is compiled", func(t *testing.T) {
		m := &diffModule{t: t, root: t.TempDir()}
		m.write(diffBase)
		cache := t.TempDir()
		m.build(cache)

		m.write(map[string]string{"calc/calc.go": strings.Replace(diffBase["calc/calc.go"],
			"func Double(n int) int {\n", "func Double(n int) int {\n\n", 1)})
		served := m.build(cache)
		if m.compiles != 1 {
			t.Fatalf("a moved line compiled %d times, want 1", m.compiles)
		}
		sameAsFresh(t, served, m.build(t.TempDir()))
	})

	// An entry from before compile times were recorded cannot keep the package
	// in the suite baseline, so it compiles as before rather than guess.
	t.Run("an entry without a compile time is compiled", func(t *testing.T) {
		m := &diffModule{t: t, root: t.TempDir()}
		m.write(diffBase)
		cache := t.TempDir()
		m.build(cache)
		c := NewWithCmd(m.command, t.TempDir(),
			gomodule.GoModule{Name: "example.com/m", Root: m.root, CallingDir: "calc"}, WithTestMapCacheDir(cache))
		dir, err := c.cacheDirPath(cacheKey(c.cacheScope(), c.buildTags))
		if err != nil {
			t.Fatal(err)
		}
		entry, ok := loadCachedPackage(dir, diffPkg)
		if !ok {
			t.Fatal("no cache entry")
		}
		entry.Compiled = 0
		if err := entry.save(dir); err != nil {
			t.Fatal(err)
		}
		m.build(cache)
		if m.compiles != 1 {
			t.Fatalf("an entry without a compile time compiled %d times, want 1", m.compiles)
		}
		// That compile is recorded, so the entry stops costing one: a cache
		// written before compile times were kept is upgraded in place by the
		// first run that compiles each package, not left to compile forever.
		m.build(cache)
		if m.compiles != 0 {
			t.Fatalf("after its compile time was recorded the entry compiled %d times, want 0", m.compiles)
		}
	})
}
