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

package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// initStrict is a variable an initialiser might read.
const initStrict = "INIT_MODE=strict"

// testPkg is the package of the module newTestCache writes.
const testPkg = "example.com/m/p"

// testEnv is a sandbox: a module, a directory outside it, and a temporary
// directory of its own, so that a path outside the module is not under the
// temporary directory the cache ignores.
type testEnv struct {
	mod, outside, scratch string
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()

	base := t.TempDir()
	e := testEnv{
		mod:     filepath.Join(base, "mod"),
		outside: filepath.Join(base, "outside"),
		scratch: filepath.Join(base, "scratch"),
	}
	for _, d := range []string{e.mod, e.outside, e.scratch} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TMPDIR", e.scratch)
	writeTestFile(t, e.mod, "go.mod", "module example.com/m\n\ngo 1.25\n")
	writeTestFile(t, e.mod, "p/p.go", "package p\n\nfunc F() int { return 1 }\n")
	writeTestFile(t, e.mod, "p/p_test.go", "package p\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) {}\n")

	return e
}

func (e testEnv) cache(t *testing.T, o Options) *Cache {
	t.Helper()

	o.ModRoot = e.mod
	if o.Dir == "" {
		o.Dir = filepath.Join(e.scratch, "cache")
	}
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func writeTestFile(t *testing.T, dir, rel, content string) string {
	t.Helper()

	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // a file of the test's own sandbox
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

func key(t *testing.T, c *Cache) string {
	t.Helper()

	k, err := c.Key(testPkg, "p/p.go", mutator.ConditionalsBoundary, 3, 1, []byte("mutated"))
	if err != nil {
		t.Fatal(err)
	}

	return k
}

// TestEveryKeyInputChangesTheKey derives keys through Cache.Key and New, so
// it checks that each input is both collected and encoded: changing any one of
// them must change the key.
func TestEveryKeyInputChangesTheKey(t *testing.T) {
	e := newTestEnv(t)
	c := e.cache(t, Options{})
	base := key(t, c)
	if again := key(t, e.cache(t, Options{})); again != base {
		t.Fatal("the same inputs give two keys")
	}

	type keyArgs struct {
		pkg, file string
		mt        mutator.Type
		line, col int
		mutated   string
	}
	b := keyArgs{testPkg, "p/p.go", mutator.ConditionalsBoundary, 3, 1, "mutated"}
	for name, a := range map[string]keyArgs{
		"package": {"example.com/m", b.file, b.mt, b.line, b.col, b.mutated},
		"file":    {b.pkg, "p/q.go", b.mt, b.line, b.col, b.mutated},
		"mutator": {b.pkg, b.file, mutator.ConditionalsNegation, b.line, b.col, b.mutated},
		"line":    {b.pkg, b.file, b.mt, 4, b.col, b.mutated},
		"column":  {b.pkg, b.file, b.mt, b.line, 2, b.mutated},
		"mutated": {b.pkg, b.file, b.mt, b.line, b.col, "other"},
	} {
		if name == "package" {
			writeTestFile(t, e.mod, "root.go", "package m\n")
		}
		k, err := c.Key(a.pkg, a.file, a.mt, a.line, a.col, []byte(a.mutated))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if k == base {
			t.Errorf("changing the %s does not change the key", name)
		}
	}
	for name, o := range map[string]Options{
		"salt": {Salt: "s"}, "tags": {Tags: "integration"}, "-cpu": {TestCPU: 2},
	} {
		if key(t, e.cache(t, o)) == base {
			t.Errorf("changing the %s does not change the key", name)
		}
	}
	t.Run("GOFLAGS", func(t *testing.T) {
		t.Setenv("GOFLAGS", "-mod=mod")
		if key(t, e.cache(t, Options{})) == base {
			t.Error("setting GOFLAGS does not change the key")
		}
	})
	withEnv := key(t, e.cache(t, Options{Env: []string{initStrict}}))
	if withEnv == base {
		t.Error("a variable of the environment does not change the key")
	}
	for name, o := range map[string]Options{
		"an ignored name":   {Env: []string{initStrict, "RUN_ID=1"}, EnvIgnore: []string{"RUN_ID"}},
		"an ignored prefix": {Env: []string{initStrict, "RUN_ID=1", "RUN_URL=u"}, EnvIgnore: []string{"RUN_*"}},
		"GOTMPDIR":          {Env: []string{initStrict, "GOTMPDIR=/per/run"}},
	} {
		if key(t, e.cache(t, o)) != withEnv {
			t.Errorf("%s changes the key", name)
		}
	}
	writeTestFile(t, e.mod, "p/nested/go.mod", "module example.com/nested\n")
	writeTestFile(t, e.mod, "p/nested/data.txt", "one")
	base = key(t, e.cache(t, Options{}))
	t.Run("a nested module", func(t *testing.T) {
		writeTestFile(t, e.mod, "p/nested/data.txt", "two")
		if key(t, e.cache(t, Options{})) != base {
			t.Error("a file of a nested module changes the key")
		}
	})
	for name, rel := range map[string]string{
		"a source file": "p/p.go", "a non-Go file": "p/notes.txt", "testdata": "p/testdata/deep/case.txt",
		"a subdirectory": "p/fixtures/want", "go.mod": "go.mod",
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(e.mod, filepath.FromSlash(rel))
			old, readErr := os.ReadFile(p) //nolint:gosec // a file of the test's own sandbox
			writeTestFile(t, e.mod, rel, string(old)+"\n// changed\n")
			defer func() {
				if readErr != nil {
					_ = os.Remove(p)
				} else {
					writeTestFile(t, e.mod, rel, string(old))
				}
			}()
			if key(t, e.cache(t, Options{})) == base {
				t.Errorf("changing %s does not change the key", rel)
			}
		})
	}
}

// TestStoreRules pins which verdicts may be stored, one case per rule in the
// package documentation.
func TestStoreRules(t *testing.T) {
	e := newTestEnv(t)
	c := e.cache(t, Options{})
	completeLog := writeTestFile(t, e.scratch, "log", testlogHeader+"\n")
	killed := Evidence{Testlog: completeLog, LogComplete: true, WorkRoot: e.mod, PkgDir: filepath.Join(e.mod, "p"), Adjudicated: true}
	lived := killed
	lived.ExitedZero = true
	notViable := Evidence{Adjudicated: true, BuildFailed: true}

	testCases := []struct {
		name   string
		status mutator.Status
		ev     Evidence
		edit   func(*Evidence)
		stored bool
	}{
		{name: "KILLED with a complete log", status: mutator.Killed, ev: killed, stored: true},
		{name: "KILLED with an incomplete log", status: mutator.Killed, ev: killed, edit: func(e *Evidence) { e.LogComplete = false }},
		{name: "KILLED without a test log", status: mutator.Killed, ev: killed, edit: func(e *Evidence) { e.Testlog = "" }},
		{name: "KILLED from a go test that exited 0", status: mutator.Killed, ev: lived},
		{name: "LIVED with a complete log", status: mutator.Lived, ev: lived, stored: true},
		{name: "LIVED with an incomplete log", status: mutator.Lived, ev: lived, edit: func(e *Evidence) { e.LogComplete = false }},
		{name: "LIVED from a go test that did not exit 0", status: mutator.Lived, ev: killed},
		{name: "LIVED from a shutdown", status: mutator.Lived, ev: lived, edit: func(e *Evidence) { e.Adjudicated = false }},
		{name: "NOT VIABLE from a failed build", status: mutator.NotViable, ev: notViable, stored: true},
		{name: "NOT VIABLE without a failed build", status: mutator.NotViable, ev: notViable, edit: func(e *Evidence) { e.BuildFailed = false }},
		{name: "TIMED OUT", status: mutator.TimedOut, ev: killed},
		{name: "RUN TIMED OUT", status: mutator.RunTimedOut, ev: killed},
		{name: "NOT COVERED", status: mutator.NotCovered, ev: killed},
	}
	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k := fmt.Sprintf("%064d", i)
			ev := tc.ev
			if ev.WorkRoot == "" {
				ev.WorkRoot = e.mod
			}
			if tc.edit != nil {
				tc.edit(&ev)
			}
			stored, err := c.Store(k, testPkg, tc.status, ev)
			if err != nil {
				t.Fatal(err)
			}
			if stored != tc.stored {
				t.Fatalf("stored = %v, want %v", stored, tc.stored)
			}
			got, hit := c.Lookup(k, nil)
			if hit != tc.stored || (hit && got != tc.status) {
				t.Fatalf("lookup = %v, %v; want %v, %v", got, hit, tc.status, tc.stored)
			}
		})
	}

	// The copy the test ran in must hold the content the key describes: a
	// verdict reached on other content is never stored under this key.
	t.Run("a copy whose closure differs from the key's", func(t *testing.T) {
		k := key(t, c)
		stale := filepath.Join(e.outside, "copy")
		writeTestFile(t, stale, "go.mod", "module example.com/m\n\ngo 1.25\n")
		writeTestFile(t, stale, "p/p.go", "package p\n\nfunc F() int { return 2 }\n")
		writeTestFile(t, stale, "p/p_test.go", "package p\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) {}\n")
		ev := killed
		ev.WorkRoot, ev.PkgDir = stale, filepath.Join(stale, "p")
		if stored, err := c.Store(k, testPkg, mutator.Killed, ev); err != nil || stored {
			t.Fatalf("stored = %v, %v; want false", stored, err)
		}
		writeTestFile(t, stale, "p/p.go", "package p\n\nfunc F() int { return 1 }\n")
		if stored, err := c.Store(k, testPkg, mutator.Killed, ev); err != nil || !stored {
			t.Fatalf("an identical copy: stored = %v, %v; want true", stored, err)
		}
	})
}

// TestAnEntryIsBelievedOnlyWhenItValidates tampers with a stored entry in
// every way validation checks, and with the inputs it recorded.
func TestAnEntryIsBelievedOnlyWhenItValidates(t *testing.T) {
	e := newTestEnv(t)
	c := e.cache(t, Options{})
	writeTestFile(t, e.mod, "p/data/in.txt", "one")
	outside := writeTestFile(t, e.outside, "fixture.txt", "one")
	log := writeTestFile(t, e.scratch, "log", strings.Join([]string{
		testlogHeader,
		"open data/in.txt",
		"getenv PROBE",
		"open " + outside,
		"open " + filepath.Join(e.scratch, "own-scratch-file"),
	}, "\n")+"\n")
	env := []string{"PROBE=a"}
	ev := Evidence{Testlog: log, LogComplete: true, WorkRoot: e.mod, PkgDir: filepath.Join(e.mod, "p"), Env: env, Adjudicated: true}
	k := key(t, c)
	if stored, err := c.Store(k, testPkg, mutator.Killed, ev); err != nil || !stored {
		t.Fatalf("store: %v %v", stored, err)
	}
	raw, err := os.ReadFile(c.path(k))
	if err != nil {
		t.Fatal(err)
	}
	var entry Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, in := range entry.Inputs {
		paths = append(paths, in.Path)
	}
	resolvedOutside := pathForms(outside)[len(pathForms(outside))-1]
	scratchFile := pathForms(filepath.Join(e.scratch, "own-scratch-file"))
	want := []string{resolvedOutside, scratchFile[len(scratchFile)-1], "p/data/in.txt"}
	sort.Strings(want)
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("inputs = %v, want %v", paths, want)
	}
	if strings.Contains(string(raw), "PROBE=a") || strings.Contains(string(raw), `"a"`) {
		t.Fatalf("the entry carries the variable's value: %s", raw)
	}
	if _, hit := c.Lookup(k, env); !hit {
		t.Fatal("the untouched entry is a miss")
	}

	// encode re-encodes the stored entry after edit; resigned recomputes its
	// digest, so that check alone cannot be what refuses it.
	encode := func(edit func(*Entry), resigned bool) []byte {
		x := entry
		edit(&x)
		if resigned {
			x.Digest = x.contentDigest()
		}
		b, _ := json.Marshal(x)

		return b
	}
	misses := []struct {
		name string
		raw  []byte
		env  []string
	}{
		{name: "not JSON", raw: []byte("{"), env: env},
		{name: "truncated", raw: raw[:len(raw)/2], env: env},
		{name: "a digest that does not match", raw: encode(func(x *Entry) { x.Status = mutator.Lived.String() }, false), env: env},
		{name: "another schema", raw: encode(func(x *Entry) { x.Schema = "v0" }, true), env: env},
		{name: "another key", raw: encode(func(x *Entry) { x.Key = "other" }, true), env: env},
		{name: "a status this cache never stores", raw: encode(func(x *Entry) { x.Status = mutator.TimedOut.String() }, true), env: env},
		{name: "a changed variable", raw: raw, env: []string{"PROBE=b"}},
		{name: "an unset variable", raw: raw, env: nil},
	}
	for _, m := range misses {
		t.Run(m.name, func(t *testing.T) {
			if err := os.WriteFile(c.path(k), m.raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, hit := c.Lookup(k, m.env); hit {
				t.Fatal("expected a miss")
			}
		})
	}

	for name, file := range map[string]string{
		"a changed input inside the module":  filepath.Join(e.mod, "p", "data", "in.txt"),
		"a changed input outside the module": outside,
	} {
		t.Run(name, func(t *testing.T) {
			//nolint:gosec // the test's own cache directory
			if err := os.WriteFile(c.path(k), raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, hit := c.Lookup(k, env); !hit {
				t.Fatal("the restored entry is a miss")
			}
			old, _ := os.ReadFile(file) //nolint:gosec // a file of the test's own sandbox
			writeTestFile(t, filepath.Dir(file), filepath.Base(file), "two")
			defer writeTestFile(t, filepath.Dir(file), filepath.Base(file), string(old))
			if _, hit := c.Lookup(k, env); hit {
				t.Fatal("expected a miss")
			}
		})
	}
}

// TestTheTestLogIsReadRelativeToWhereTheTestRan pins the path arithmetic: a
// relative name is resolved against the package directory, a chdir moves
// that base and is itself an input, a path inside the copy or the module is
// mapped onto the module tree through any symbolic link, a path under
// gremlins' own directory is dropped, and any other path, missing or not, is
// kept as it is.
func TestTheTestLogIsReadRelativeToWhereTheTestRan(t *testing.T) {
	e := newTestEnv(t)
	c := e.cache(t, Options{})
	gremlinsDir := filepath.Join(e.scratch, "gremlins")
	work := filepath.Join(gremlinsDir, "work")
	if err := os.MkdirAll(filepath.Join(work, "a", "pkg"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, gremlinsDir, "testlog-1", "")
	// Paths under the temporary directory are inputs, a missing one
	// included: a test may check that a flag file does not exist.
	kept := writeTestFile(t, e.scratch, "fixture", "loose")
	gone := filepath.Join(e.scratch, "flag")
	// The copy is reached through a link, as it is when the temporary
	// directory is one; the test binary sees the resolved path.
	link := filepath.Join(e.outside, "work-link")
	if err := os.Symlink(work, link); err != nil {
		t.Fatal(err)
	}
	resolvedWork, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	log := writeTestFile(t, e.scratch, "log", strings.Join([]string{
		testlogHeader,
		"open data.txt",
		"chdir ../shared",
		"stat more.txt",
		"open " + filepath.Join(resolvedWork, "abs.txt"),
		"open " + filepath.Join(e.mod, "go.mod"),
		"stat " + gone,
		"open " + kept,
		"open " + filepath.Join(gremlinsDir, "testlog-1"),
		"open " + filepath.Join(e.outside, "elsewhere.txt"),
	}, "\n")+"\n")
	inputs, _, ok := c.runtimeInputs(Evidence{Testlog: log, WorkRoot: link, PkgDir: filepath.Join(link, "a", "pkg"), ScratchRoot: gremlinsDir})
	if !ok {
		t.Fatal("the log was not read")
	}
	var got []string
	for _, in := range inputs {
		got = append(got, in.Op+" "+in.Path)
	}
	elsewhere := pathForms(filepath.Join(e.outside, "elsewhere.txt"))
	keptForms, goneForms := pathForms(kept), pathForms(gone)
	want := []string{
		"stat a/shared", "stat a/shared/more.txt", "open a/pkg/data.txt", "open abs.txt", "open go.mod",
		"open " + elsewhere[len(elsewhere)-1], "open " + keptForms[len(keptForms)-1],
		"stat " + goneForms[len(goneForms)-1],
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}

	for _, bad := range []string{"", "not a test log\nopen x\n", testlogHeader + "\nunknown-op x\n", testlogHeader + "\nnospace\n"} {
		p := writeTestFile(t, e.scratch, "bad-log", bad)
		if _, _, ok := c.runtimeInputs(Evidence{Testlog: p, WorkRoot: work, PkgDir: work}); ok {
			t.Errorf("a malformed log was accepted: %q", bad)
		}
	}
}

// TestASpecialFileIsNeverRead pins that a device in the test log is described,
// not read: reading one such as /dev/zero would never finish.
func TestASpecialFileIsNeverRead(t *testing.T) {
	if got := inputDigest("/", opOpen, "/dev/zero"); !strings.HasPrefix(got, "special ") {
		t.Fatalf("inputDigest(/dev/zero) = %q", got)
	}
}
