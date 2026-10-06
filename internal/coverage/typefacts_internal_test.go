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
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/go/packages"

	"github.com/go-gremlins/gremlins/internal/gomodule"
)

// The order is the type-checker's, test files and external tests included,
// and a package with an error says nothing rather than something partial.
func TestLoadTypesFromSourceReadsTheInitialisationOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	root := t.TempDir()
	for rel, content := range map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.22\n",
		// b waits for a, which is declared after it; the blanks are named
		// by file and by place, and b.go sorts after a.go.
		"p/a.go":      "package p\n\nfunc f() int { return 1 }\n\nvar _ = f()\n\nvar b = a + 1\n\nvar a = 1\n\nvar _ = f()\n",
		"p/b.go":      "package p\n\nvar _ = f()\n",
		"p/p_test.go": "package p\n\nvar t1 = b\n",
		"p/x_test.go": "package p_test\n\nimport \"example.com/m/p\"\n\nvar x = p.Name\n",
		"p/name.go":   "package p\n\nconst Name = \"p\"\n",
		"bad/bad.go":  "package bad\n\nvar v = undefined\n",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := &Coverage{mod: gomodule.GoModule{Name: "example.com/m", Root: root}}

	got := c.loadTypesFromSource(true, []string{"example.com/m/p", "example.com/m/bad"})

	// The package as its test binary links it imports nothing; its external
	// tests import that recompiled package, by its variant's ID.
	want := map[string]typeFacts{"example.com/m/p": {
		initOrder: []string{"_@a.go#0", "a", "b", "_@a.go#1", "_@b.go#0", "t1", xtestPrefix + "x"},
		reach:     hashOf([]byte("\x00xtest\x00example.com/m/p [example.com/m/p.test]")),
		observers: map[string]bool{
			observerKey("a.go", ""): true, observerKey("b.go", ""): true, observerKey("name.go", ""): true,
			observerKey("p_test.go", ""): true, observerKey("x_test.go", ""): true,
		},
	}}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(typeFacts{})); diff != "" {
		t.Errorf("unexpected facts (-want +got):\n%s", diff)
	}
	plain := c.loadTypesFromSource(false, []string{"example.com/m/p"})
	if p := plain["example.com/m/p"]; len(p.initOrder) != 5 || p.reach != hashOf(nil) {
		t.Errorf("want the package as another binary links it, without its tests, got %v", plain)
	}
}

// A var observes the order across packages when its initialiser reads a
// variable of another package, however the name is spelled, or a variable of
// its own package that does. A constant, a field of its own value, and a
// function literal's body read nothing at initialisation.
func TestLoadTypesFromSourceFindsTheVarsReadingAnotherPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	root := t.TempDir()
	for rel, content := range map[string]string{
		"go.mod":   "module example.com/m\n\ngo 1.22\n",
		"q/q.go":   "package q\n\nvar Names []string\n\nconst Max = 3\n\ntype Box struct{ N int }\n",
		"p/dot.go": "package p\n\nimport . \"example.com/m/q\"\n\nvar dot = len(Names)\n\nvar ptr = &Names\n\nvar via, also = len(*ptr), 1\n",
		"p/sel.go": "package p\n\nimport \"example.com/m/q\"\n\nvar sel = q.Names\n\nvar max = q.Max\n\nvar box q.Box\n\n" +
			"var field = box.N\n\nvar lit = func() int { return len(q.Names) }\n\nvar local = max + field\n",
		"p/x_test.go": "package p_test\n\nimport \"example.com/m/q\"\n\nvar x = q.Box{N: len(q.Names)}\n",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := &Coverage{mod: gomodule.GoModule{Name: "example.com/m", Root: root}}

	got := c.loadTypesFromSource(true, []string{"example.com/m/p"})["example.com/m/p"].observers

	want := map[string]bool{
		observerKey("dot.go", ""): true, observerKey("sel.go", ""): true, observerKey("x_test.go", ""): true,
		observerKey("dot.go", "dot"): true, observerKey("dot.go", "ptr"): true, observerKey("dot.go", "via"): true,
		observerKey("dot.go", "also"): true, observerKey("sel.go", "sel"): true, observerKey("x_test.go", "x"): true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected observers (-want +got):\n%s", diff)
	}
}

func TestSameInitOrderComparesWhatBothInitialise(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		was, now []string
		// first and second are the variables initOrderSwap names, empty
		// when the orders agree.
		first, second string
	}{
		"the same":               {was: []string{"a", "b"}, now: []string{"a", "b"}},
		"one added and one gone": {was: []string{"a", "gone", "b"}, now: []string{"new", "a", "b"}},
		"two trading places":     {was: []string{"c", "a", "b"}, now: []string{"c", "b", "a"}, first: "a", second: "b"},
		// A name twice in one list is not something a type-checker says,
		// and it is not taken as agreement, on either side.
		"a name listed twice before": {was: []string{"a", "a"}, now: []string{"a"}, first: "a", second: "a"},
		"a name listed twice after":  {was: []string{"b"}, now: []string{"b", "b"}, first: "b", second: "b"},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			agree := tc.first == ""
			if got := sameInitOrder(tc.was, tc.now); got != agree {
				t.Errorf("sameInitOrder(%v, %v) = %v, want %v", tc.was, tc.now, got, agree)
			}
			first, second, swapped := initOrderSwap(tc.was, tc.now)
			if first != tc.first || second != tc.second || swapped == agree {
				t.Errorf("initOrderSwap(%v, %v) = %q, %q, %v, want %q, %q, %v",
					tc.was, tc.now, first, second, swapped, tc.first, tc.second, !agree)
			}
		})
	}
}

// A package the import listing did not return, or returned without its
// external tests, has no place in the order to compare, and so no facts.
func TestFactsOfLeavesOutWhatTheListingMissed(t *testing.T) {
	t.Parallel()

	typed := func() *packages.Package {
		return &packages.Package{TypesInfo: &types.Info{}, Fset: token.NewFileSet()}
	}
	listed := &packages.Package{Imports: map[string]*packages.Package{"fmt": {ID: "fmt"}}}
	got := factsOf(map[string]*variants{
		"m/whole":    {plain: typed(), xtest: typed()},
		"m/unlisted": {plain: typed()},
		"m/noxtest":  {plain: typed(), xtest: typed()},
	}, map[string]*variants{
		"m/whole":   {plain: listed, xtest: listed},
		"m/noxtest": {plain: listed},
	}, true)

	want := map[string]typeFacts{"m/whole": {reach: hashOf([]byte("fmt\x00xtest\x00fmt")), observers: map[string]bool{}}}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(typeFacts{})); diff != "" {
		t.Errorf("unexpected facts (-want +got):\n%s", diff)
	}
}

// A load that cannot run says nothing about any package.
func TestLoadTypesFromSourceSaysNothingWhenTheLoadFails(t *testing.T) {
	t.Parallel()

	c := &Coverage{mod: gomodule.GoModule{Name: "example.com/m", Root: filepath.Join(t.TempDir(), "missing")}}
	if got := c.loadTypesFromSource(true, []string{"example.com/m/p"}); got != nil {
		t.Errorf("want no facts, got %v", got)
	}
}
