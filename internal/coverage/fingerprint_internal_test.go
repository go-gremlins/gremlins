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
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/gomodule"
)

// fingerprintOfSource writes the given files into a package directory and
// fingerprints it, the way a run does before deciding what its cache still
// says. Paths may name subdirectories.
func fingerprintOfSource(t *testing.T, files map[string]string) fingerprint {
	t.Helper()

	return fingerprintWithNames(t, files, nil)
}

// fingerprintWithNames is fingerprintOfSource with the package names a build
// listing would have reported.
func fingerprintWithNames(t *testing.T, files map[string]string, names map[string]string) fingerprint {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("cannot create the directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("cannot write the source: %v", err)
		}
	}
	c := &Coverage{mod: gomodule.GoModule{Name: "example.com", Root: dir, CallingDir: "."}, pkgNames: names}

	fp, ok := c.fingerprintOf(&testPackage{importPath: "example.com/pkg", dir: dir})
	if !ok {
		t.Fatal("want the package fingerprinted, got a failure")
	}

	return fp
}

func TestFingerprintOfFailsOnADirectoryItCannotRead(t *testing.T) {
	t.Parallel()

	c := &Coverage{mod: gomodule.GoModule{Name: "example.com", Root: ".", CallingDir: "."}}
	pkg := &testPackage{importPath: "example.com/gone", dir: filepath.Join(t.TempDir(), "gone")}

	// Failing to read is not failing the run: the caller re-maps the package,
	// which is what happens without a cache at all.
	if _, ok := c.fingerprintOf(pkg); ok {
		t.Error("want a failure for a directory that is not there")
	}
}

const plainSource = "package pkg\n\nfunc F() int {\n\treturn 1\n}\n"

// Whole is everything no entity can carry a change of. Each pair here differs
// in exactly one such thing, and each must move it.
func TestFingerprintWholeHoldsWhatNoEntityCarries(t *testing.T) {
	t.Parallel()

	testCases := map[string][2]map[string]string{
		"a non-Go file": {
			{"a.go": plainSource, "data.txt": "one"},
			{"a.go": plainSource, "data.txt": "two"},
		},
		// A file that does not parse yields no entities, so all of it is here.
		"a file that does not parse": {
			{"a.go": "package pkg\n\nfunc F( {\n"},
			{"a.go": "package pkg\n\nfunc G( {\n"},
		},
		"data below the directory": {
			{"a.go": plainSource, "testdata/golden/one.json": `{"n":1}`},
			{"a.go": plainSource, "testdata/golden/one.json": `{"n":2}`},
		},
		"a build constraint": {
			{"a.go": plainSource},
			{"a.go": "//go:build linux\n\n" + plainSource},
		},
		"an old-style build constraint": {
			{"a.go": plainSource},
			{"a.go": "// +build linux\n\n" + plainSource},
		},
		"a file named for a platform": {
			{"a.go": plainSource},
			{"a.go": plainSource, "b_linux.go": "package pkg\n"},
		},
		"a file named for an architecture, as a test file": {
			{"a.go": plainSource},
			{"a.go": plainSource, "b_arm64_test.go": "package pkg\n"},
		},
		"a directive outside every declaration": {
			{"a.go": plainSource},
			{"a.go": "//go:debug panicnil=1\n\n" + plainSource},
		},
		"a blank import": {
			{"a.go": plainSource},
			{"a.go": "package pkg\n\nimport _ \"embed\"\n\nfunc F() int {\n\treturn 1\n}\n"},
		},
		"a dot import": {
			{"a.go": plainSource},
			{"a.go": "package pkg\n\nimport . \"strings\"\n\nfunc F() int {\n\treturn 1\n}\n"},
		},
		// The preamble is a comment, and it is C that gets compiled.
		"a cgo preamble": {
			{"a.go": "package pkg\n\n// #define N 1\nimport \"C\"\n"},
			{"a.go": "package pkg\n\n// #define N 2\nimport \"C\"\n"},
		},
		"the package name": {
			{"a.go": plainSource},
			{"a.go": "package other\n\nfunc F() int {\n\treturn 1\n}\n"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if fingerprintOfSource(t, tc[0]).Whole == fingerprintOfSource(t, tc[1]).Whole {
				t.Error("want Whole moved")
			}
		})
	}
}

// What an entity carries, or what decides nothing, leaves Whole where it was:
// otherwise every change would be a whole re-map again.
func TestFingerprintWholeIgnoresWhatAnEntityCarries(t *testing.T) {
	t.Parallel()

	testCases := map[string][2]map[string]string{
		"an added plain file": {
			{"a.go": plainSource},
			{"a.go": plainSource, "b.go": "package pkg\n\nconst limit = 3\n"},
		},
		// A test file's name constrains nothing but which binary it is
		// compiled into, and its declarations carry that.
		"an added test file": {
			{"a.go": plainSource},
			{"a.go": plainSource, "b_test.go": "package pkg\n\nfunc helper() int {\n\treturn 1\n}\n"},
		},
		"a file renamed to a test file": {
			{"a.go": plainSource, "b.go": "package pkg\n\nfunc helper() int {\n\treturn 1\n}\n"},
			{"a.go": plainSource, "b_test.go": "package pkg\n\nfunc helper() int {\n\treturn 1\n}\n"},
		},
		"a moved function": {
			{"a.go": plainSource + "\nfunc G() int {\n\treturn 2\n}\n"},
			{"a.go": "package pkg\n\nfunc G() int {\n\treturn 2\n}\n\nfunc F() int {\n\treturn 1\n}\n"},
		},
		"an import": {
			{"a.go": plainSource},
			{"a.go": "package pkg\n\nimport \"strings\"\n\nfunc F() int {\n\treturn len(strings.ToUpper(\"\"))\n}\n"},
		},
		"a prose comment": {
			{"a.go": "// Package pkg is a package.\n" + plainSource},
			{"a.go": "// Package pkg is another package.\n" + plainSource},
		},
		// A subdirectory holding Go files is a package of its own: one this
		// package imports is covered by its own print, and one it does not
		// import has no business dirtying it.
		"a subpackage's source": {
			{"a.go": plainSource, "sub/sub.go": "package sub\n\nfunc G() int {\n\treturn 1\n}\n"},
			{"a.go": plainSource, "sub/sub.go": "package sub\n\nfunc G() int {\n\treturn 2\n}\n"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if fingerprintOfSource(t, tc[0]).Whole != fingerprintOfSource(t, tc[1]).Whole {
				t.Error("want Whole unchanged")
			}
		})
	}
}

// entitySource declares one of each kind of entity, at known lines.
const entitySource = `package pkg

import (
	"errors"
	"time"
)

// F doubles n.
func F(n int) int {
	return n * 2
}

func (t *T[K]) M() int {
	return 1
}

func init() {
	_ = F(1)
}

func _() {}

type (
	T[K any] struct{ k K }
	U        int
)

const single = 1

const (
	a = iota
	b
)

var (
	plain    = 3
	errBad   = errors.New("bad")
	start    = F(2)
	hook     = func() int { return F(3) }
	invoked  = func() int { return 4 }()
	sized    = make([]int, len("ab"))
	asBytes  = []byte("x")
	asInt    = int64(5)
	asType   = (*T[int])(nil)
	duration = time.Duration(5)
	x, y     = 1, 2
)

var _ = 5
`

const entityTestSource = `package pkg

import "testing"

func helper() int {
	return F(1)
}

func TestF(t *testing.T) {
	_ = helper()
}

func TestMain(m *testing.M) {
	m.Run()
}
`

func TestFingerprintDescribesEveryEntity(t *testing.T) {
	t.Parallel()

	fp := fingerprintWithNames(t, map[string]string{"a.go": entitySource, "a_test.go": entityTestSource},
		map[string]string{"errors": "errors", "time": "time"})

	type shape struct {
		Kind       string
		Names      []string
		Test       string
		Start, End int
	}
	got := map[string]shape{}
	for key, d := range fp.Decls {
		got[key] = shape{Kind: d.Kind, Names: d.Names, Test: d.Test, Start: d.Start, End: d.End}
	}
	want := map[string]shape{
		// The span starts at the declaration, not at its doc comment: a doc
		// comment growing must not read as the body having moved.
		"pkg/a.go:F":            {Names: []string{"F"}, Start: 9, End: 11},
		"pkg/a.go:TK.M":         {Kind: kindMethod, Names: []string{"T"}, Start: 13, End: 15},
		"pkg/a.go:init#0":       {Kind: kindInit, Start: 17, End: 19},
		"pkg/a.go:_#0":          {Kind: kindBlank, Start: 21, End: 21},
		"pkg/a.go:type T":       {Kind: kindType, Names: []string{"T"}, Start: 24, End: 24},
		"pkg/a.go:type U":       {Kind: kindType, Names: []string{"U"}, Start: 25, End: 25},
		"pkg/a.go:const single": {Kind: kindConst, Names: []string{"single"}, Start: 28, End: 28},
		// A parenthesised const group is one entity.
		"pkg/a.go:const a,b": {Kind: kindConst, Names: []string{"a", "b"}, Start: 30, End: 33},
		"pkg/a.go:var plain": {Kind: kindVar, Names: []string{"plain"}, Start: 36, End: 36},
		// An allowlisted constructor builds its value and nothing else.
		"pkg/a.go:var errBad": {Kind: kindVar, Names: []string{"errBad"}, Start: 37, End: 37},
		"pkg/a.go:run start":  {Kind: kindRun, Names: []string{"start"}, Start: 38, End: 38},
		// A literal's body runs when it is called, not at initialisation.
		"pkg/a.go:var hook":    {Kind: kindVar, Names: []string{"hook"}, Start: 39, End: 39},
		"pkg/a.go:run invoked": {Kind: kindRun, Names: []string{"invoked"}, Start: 40, End: 40},
		"pkg/a.go:var sized":   {Kind: kindVar, Names: []string{"sized"}, Start: 41, End: 41},
		"pkg/a.go:var asBytes": {Kind: kindVar, Names: []string{"asBytes"}, Start: 42, End: 42},
		"pkg/a.go:var asInt":   {Kind: kindVar, Names: []string{"asInt"}, Start: 43, End: 43},
		"pkg/a.go:var asType":  {Kind: kindVar, Names: []string{"asType"}, Start: 44, End: 44},
		// It may be a conversion; it may be a call. Only the type checker
		// knows, and this does not ask it.
		"pkg/a.go:run duration": {Kind: kindRun, Names: []string{"duration"}, Start: 45, End: 45},
		"pkg/a.go:var x,y":      {Kind: kindVar, Names: []string{"x", "y"}, Start: 46, End: 46},
		"pkg/a.go:blank #0":     {Kind: kindBlank, Start: 49, End: 49},
		"pkg/a_test.go:helper":  {Names: []string{"helper"}, Start: 5, End: 7},
		"pkg/a_test.go:TestF":   {Names: []string{"TestF"}, Test: "TestF", Start: 9, End: 11},
		// TestMain runs for every test, and is not one.
		"pkg/a_test.go:TestMain#0": {Kind: kindMain, Start: 13, End: 15},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("entities (-want +got):\n%s", diff)
	}

	// A function's header is what a name reaches through without executing
	// it; a test file's entity is followed through all of it.
	f := fp.Decls["pkg/a.go:F"]
	if !f.header["int"] || f.header["n"] == false || f.header["F"] == false {
		t.Errorf("want F's header to hold its signature's names, got %v", f.header)
	}
	if !f.text["n"] {
		t.Errorf("want F's text to hold its body's names, got %v", f.text)
	}
	if h := fp.Decls["pkg/a_test.go:helper"]; !h.header["F"] {
		t.Errorf("want a test-file helper's header to be all of it, got %v", h.header)
	}
	if diff := cmp.Diff(map[string]string{"errors": "errors", "time": "time"}, fp.Imports["pkg/a.go"]); diff != "" {
		t.Errorf("imports (-want +got):\n%s", diff)
	}
}

// A predeclared function the package declares for itself is a call into the
// package.
func TestFingerprintTreatsAShadowedBuiltinAsACall(t *testing.T) {
	t.Parallel()

	fp := fingerprintOfSource(t, map[string]string{
		"a.go": "package pkg\n\nvar sized = make(3)\n\nfunc make(n int) int {\n\treturn n\n}\n",
	})
	if _, ok := fp.Decls["pkg/a.go:run sized"]; !ok {
		t.Errorf("want the var run at initialisation, got %v", keysOf(fp.Decls))
	}
}

// An allowlisted constructor is known by the path its package name is bound
// to, not by the name.
func TestFingerprintKnowsAnAllowlistedConstructorByItsPath(t *testing.T) {
	t.Parallel()

	src := "package pkg\n\nimport errors \"example.com/myerrors\"\n\nvar errBad = errors.New(\"bad\")\n"
	fp := fingerprintOfSource(t, map[string]string{"a.go": src})
	if _, ok := fp.Decls["pkg/a.go:run errBad"]; !ok {
		t.Errorf("want a New of another package run at initialisation, got %v", keysOf(fp.Decls))
	}
}

// Each pair differs in something that decides what the entity does, and its
// print must move.
func TestFingerprintPrintHoldsWhatDecides(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		key    string
		before string
		after  string
	}{
		"a //go:embed pattern": {
			"pkg/a.go:var data",
			"package pkg\n\nimport _ \"embed\"\n\n//go:embed a.txt\nvar data string\n",
			"package pkg\n\nimport _ \"embed\"\n\n//go:embed b.txt\nvar data string\n",
		},
		"a //go:noinline directive": {
			"pkg/a.go:F",
			plainSource,
			"package pkg\n\n//go:noinline\nfunc F() int {\n\treturn 1\n}\n",
		},
		// Inserted lines move the blocks below them inside the body, which a
		// mapping moved by the declaration's distance would get wrong.
		"a comment line inside a body": {
			"pkg/a.go:F",
			plainSource,
			"package pkg\n\nfunc F() int {\n\t// one\n\treturn 1\n}\n",
		},
		"an indentation change": {
			"pkg/a.go:F",
			plainSource,
			"package pkg\n\nfunc F() int {\n\t\treturn 1\n}\n",
		},
		// An Example is checked against its comment.
		"an Example's output comment": {
			"pkg/a_test.go:Example",
			"package pkg\n\nfunc Example() {\n\t// Output: 1\n}\n",
			"package pkg\n\nfunc Example() {\n\t// Output: 2\n}\n",
		},
		"a struct tag": {
			"pkg/a.go:type P",
			"package pkg\n\ntype P struct {\n\tX int `json:\"x\"`\n}\n",
			"package pkg\n\ntype P struct {\n\tX int `json:\"y\"`\n}\n",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file := "a.go"
			if tc.key == "pkg/a_test.go:Example" {
				file = "a_test.go"
			}
			before := fingerprintOfSource(t, map[string]string{file: tc.before}).Decls[tc.key]
			after := fingerprintOfSource(t, map[string]string{file: tc.after}).Decls[tc.key]
			if before.Hash == "" || before.Hash == after.Hash {
				t.Errorf("want %s's print moved, got %q then %q", tc.key, before.Hash, after.Hash)
			}
		})
	}
}

// What decides nothing leaves an entity's print where it was, and only moves
// its span.
func TestFingerprintPrintIgnoresWhatDecidesNothing(t *testing.T) {
	t.Parallel()

	before := fingerprintOfSource(t, map[string]string{"a.go": "package pkg\n\n// F is one.\nfunc F() int {\n\treturn 1 // one\n}\n"})
	after := fingerprintOfSource(t, map[string]string{"a.go": "package pkg\n\n\n// F returns one,\n// always.\nfunc F() int {\n\treturn 1 // just one\n}\n"})
	b, a := before.Decls["pkg/a.go:F"], after.Decls["pkg/a.go:F"]
	if b.Hash != a.Hash || b.Sig != a.Sig {
		t.Errorf("want prose and blank lines ignored, got %+v then %+v", b, a)
	}
	if a.Start-b.Start != 2 {
		t.Errorf("want the span moved by the two lines above it, got %d then %d", b.Start, a.Start)
	}
}

// Only a body change is attributable to the tests that executed the body.
func TestFingerprintPrintsASignatureApartFromItsBody(t *testing.T) {
	t.Parallel()

	was := fingerprintOfSource(t, map[string]string{"a.go": plainSource}).Decls["pkg/a.go:F"]
	body := fingerprintOfSource(t, map[string]string{"a.go": "package pkg\n\nfunc F() int {\n\treturn 2\n}\n"}).Decls["pkg/a.go:F"]
	sig := fingerprintOfSource(t, map[string]string{"a.go": "package pkg\n\nfunc F() int64 {\n\treturn 1\n}\n"}).Decls["pkg/a.go:F"]
	if body.Hash == was.Hash || body.Sig != was.Sig {
		t.Errorf("a body change: want the hash moved and the signature kept, got %+v then %+v", was, body)
	}
	if sig.Sig == was.Sig {
		t.Errorf("a signature change: want the signature moved, got %+v then %+v", was, sig)
	}
}

// An import table is by local name: what an unnamed import binds is the
// imported package's own name, when the listing gave it.
func TestFingerprintRecordsEachFilesImportTable(t *testing.T) {
	t.Parallel()

	src := "package pkg\n\nimport (\n\tst \"example.com/store\"\n\t\"example.com/v2/yaml\"\n\t\"example.com/unlisted\"\n)\n"
	fp := fingerprintWithNames(t, map[string]string{"a.go": src}, map[string]string{"example.com/v2/yaml": "yaml"})
	want := map[string]string{
		"st":                    "example.com/store",
		"yaml":                  "example.com/v2/yaml",
		"?example.com/unlisted": "example.com/unlisted",
	}
	if diff := cmp.Diff(want, fp.Imports["pkg/a.go"]); diff != "" {
		t.Errorf("imports (-want +got):\n%s", diff)
	}
}

// A declaration bound to another package's symbol by //go:linkname is marked,
// because no name in this package carries its change.
func TestFingerprintMarksALinknamedDeclaration(t *testing.T) {
	t.Parallel()

	fp := fingerprintOfSource(t, map[string]string{
		"a.go": "package pkg\n\nimport _ \"unsafe\"\n\n//go:linkname now runtime.nanotime\nfunc now() int64\n",
	})
	if d := fp.Decls["pkg/a.go:now"]; !d.Linked {
		t.Errorf("want now marked as linked, got %+v", d)
	}
}

func keysOf(decls map[string]declPrint) []string {
	keys := make([]string, 0, len(decls))
	for key := range decls {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

// The receiver's base type is what a method changes the method set of,
// however it is written.
func TestReceiverNameIsTheBaseType(t *testing.T) {
	t.Parallel()

	fp := fingerprintOfSource(t, map[string]string{"a.go": `package pkg

func (p *Pair[K, V]) A() {}

func (u (U)) B() {}

func () C() {}

func (x other.T) D() {}
`})
	want := map[string][]string{
		"pkg/a.go:PairKV.A": {"Pair"},
		"pkg/a.go:U.B":      {"U"},
		"pkg/a.go:C":        {"C"},
		"pkg/a.go:otherT.D": {"D"},
	}
	got := map[string][]string{}
	for key, d := range fp.Decls {
		got[key] = d.Names
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("method names (-want +got):\n%s", diff)
	}
}

// What is not obviously free of effects at initialisation runs there: a call
// through anything but a plain package name, a parenthesised callee, an
// instantiated generic, a blank var's call. A key a file repeats pairs nothing and so re-maps on a change.
func TestFingerprintErrsTowardsRunningAtInit(t *testing.T) {
	t.Parallel()

	fp := fingerprintOfSource(t, map[string]string{"a.go": `package pkg

var (
	chained = cfg.x.New()
	parened = (F)(1)
	generic = Apply[int](1)
)

var _ = F(2)

func F(n int) int {
	return n
}

func F(n int) int {
	return n + 1
}
`})
	for key, kind := range map[string]string{
		"pkg/a.go:run chained": kindRun,
		"pkg/a.go:run parened": kindRun,
		"pkg/a.go:run generic": kindRun,
		"pkg/a.go:run _#0":     kindRun,
		"pkg/a.go:F":           "",
		"pkg/a.go:F#1":         kindBlank,
	} {
		d, ok := fp.Decls[key]
		if !ok || d.Kind != kind {
			t.Errorf("want %s of kind %q, got %+v (present %v) among %v", key, kind, d, ok, keysOf(fp.Decls))
		}
	}
}

// fmt.Errorf formats its operands while the variable is initialised, and
// formatting a value calls its String or Error method. Only an operand that
// can have no method of the package's is let through: a literal, or a var the
// package declares once as errors.New of a literal — matched within the
// package that names it, so an external test package's var of the same name
// is not mistaken for it.
func TestFingerprintFmtErrorfFormatsOnlyWhatRunsNothing(t *testing.T) {
	t.Parallel()

	fp := fingerprintWithNames(t, map[string]string{
		"a.go": `package pkg

import (
	"errors"
	"fmt"
	e2 "errors"
)

var errBase = errors.New("base")

var errAliased = e2.New("aliased")

var errDup = errors.New("dup")

var errPair, errOther = errors.New("a"), errors.New("b")

var (
	literals = fmt.Errorf("x %d %s %v", 1, "a", 2.5)
	wrapped  = fmt.Errorf("w: %w", errBase)
	aliased  = fmt.Errorf("w: %w", errAliased)
	value    = fmt.Errorf("%v", item{})
	named    = fmt.Errorf("%v", limit)
	dup      = fmt.Errorf("%w", errDup)
	paired   = fmt.Errorf("%w", errPair)
	spread   = fmt.Errorf("%v", errBase...)
	called   = fmt.Errorf("%d", len("x"))
	parened  = (fmt.Errorf)("%v", item{})
)
`,
		"b_linux.go": "package pkg\n\nimport \"errors\"\n\nvar errDup = errors.New(\"again\")\n",
		"x_test.go": `package pkg_test

import "fmt"

var errBase = item{}

var shadowed = fmt.Errorf("%w", errBase)
`,
	}, map[string]string{"errors": "errors", "fmt": "fmt"})
	for key, kind := range map[string]string{
		"pkg/a.go:var literals":      kindVar,
		"pkg/a.go:var wrapped":       kindVar,
		"pkg/a.go:var aliased":       kindVar,
		"pkg/a.go:run value":         kindRun,
		"pkg/a.go:run named":         kindRun,
		"pkg/a.go:run dup":           kindRun,
		"pkg/a.go:run paired":        kindRun,
		"pkg/a.go:run spread":        kindRun,
		"pkg/a.go:run called":        kindRun,
		"pkg/a.go:run parened":       kindRun,
		"pkg/x_test.go:run shadowed": kindRun,
	} {
		d, ok := fp.Decls[key]
		if !ok || d.Kind != kind {
			t.Errorf("want %s of kind %q, got %+v (present %v) among %v", key, kind, d, ok, keysOf(fp.Decls))
		}
	}
}

// An alias and the local type it names have one method set, so the alias
// declares the type's name too. A type of another package, a pointer or a
// predeclared type cannot gain a method through an alias.
func TestFingerprintAnAliasDeclaresTheLocalTypeItNames(t *testing.T) {
	t.Parallel()

	fp := fingerprintOfSource(t, map[string]string{"a.go": `package pkg

type box struct{}

type Alias = box

type Ext = other.Box

type Ptr = *box

type Int = int

type Defined box
`})
	want := map[string][]string{
		"pkg/a.go:type box":     {"box"},
		"pkg/a.go:type Alias":   {"Alias", "box"},
		"pkg/a.go:type Ext":     {"Ext"},
		"pkg/a.go:type Ptr":     {"Ptr"},
		"pkg/a.go:type Int":     {"Int"},
		"pkg/a.go:type Defined": {"Defined"},
	}
	got := map[string][]string{}
	for key, d := range fp.Decls {
		got[key] = d.Names
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("type names (-want +got):\n%s", diff)
	}
}

// A var initialised by reading another package sees whatever that package
// held when this one was initialised, which is what an import changing the
// order of initialisation can move. A constructor's callee, a function
// literal's body and the package's own names read nothing of that kind.
func TestFingerprintMarksAVarThatReadsAnotherPackage(t *testing.T) {
	t.Parallel()

	fp := fingerprintWithNames(t, map[string]string{"a.go": `package pkg

import (
	"errors"
	"example.com/reg"
)

var (
	snapshot = len(reg.Names)
	errPlain = errors.New("plain")
	deferred = func() int { return len(reg.Names) }
	local    = F
	plain    int
)

func F() int {
	return 1
}
`}, map[string]string{"errors": "errors", "example.com/reg": "reg"})
	got := map[string]bool{}
	for key, d := range fp.Decls {
		if d.Kind == kindVar {
			got[key] = d.Observes
		}
	}
	want := map[string]bool{
		"pkg/a.go:var snapshot": true,
		"pkg/a.go:var errPlain": false,
		"pkg/a.go:var deferred": false,
		"pkg/a.go:var local":    false,
		"pkg/a.go:var plain":    false,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("vars that read another package (-want +got):\n%s", diff)
	}
}

// A data subtree that cannot be read is a failure to fingerprint, not a
// subtree with nothing in it.
func TestFingerprintOfFailsOnDataItCannotRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(plainSource), 0o600); err != nil {
		t.Fatalf("cannot write the source: %v", err)
	}
	locked := filepath.Join(dir, "testdata")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatalf("cannot create the directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) }) //nolint:gosec // restoring a directory this test locked
	c := &Coverage{mod: gomodule.GoModule{Name: "example.com", Root: dir, CallingDir: "."}}

	if _, ok := c.fingerprintOf(&testPackage{importPath: "example.com/pkg", dir: dir}); ok {
		t.Error("want a failure for a subtree that cannot be read")
	}
	if _, ok := c.depSourceOf(dependency{importPath: "example.com/pkg", dir: dir, instrumented: true}); ok {
		t.Error("want a dependency with a subtree that cannot be read unread")
	}
}
