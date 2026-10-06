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
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// inputsHash stands for everything the test binary is built from besides the
// package and its instrumented dependencies: unchanged in every case that
// narrows, because a change there is one no record of this package could
// have seen.
const inputsHash = "inputs"

func idents(names ...string) map[string]bool {
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}

	return out
}

// fn is a function of a.go over the given lines. Its header mentions its own
// name; its text that and whatever else it names.
func fn(name string, start, end int, hash string, mentions ...string) declPrint {
	return declPrint{
		Hash: hash, Sig: name + "-sig", Names: []string{name}, File: "a.go", Start: start, End: end,
		header: idents(name), text: idents(append(mentions, name)...),
	}
}

// spec is a type, var or const entity of a.go, whose header is all of it.
func spec(kind, name string, line int, hash string, mentions ...string) declPrint {
	m := idents(append(mentions, name)...)

	return declPrint{Hash: hash, Kind: kind, Names: []string{name}, File: "a.go", Start: line, End: line, header: m, text: m}
}

// testFn is a test, or a helper when test is empty, of a_test.go. A test
// file's entity is followed through all of it.
func testFn(name, test, hash string, mentions ...string) declPrint {
	m := idents(append(mentions, name)...)

	return declPrint{Hash: hash, Names: []string{name}, File: "a_test.go", Test: test, header: m, text: m}
}

// pkg is a package as the cases below map it:
//
//   - F over lines 3-7 of a.go, naming the constant limit in its body;
//   - G over lines 9-11, naming nothing;
//   - limit, a constant on line 13;
//   - TestF, which executed F and, in the dependency example.com/dep, Clamp;
//   - TestG, which executed G and nothing in the dependency.
//
// The dependency declares Clamp, Size and an exported type Box. Every call
// returns fresh maps, so a case can change what it likes.
func pkg() (fingerprint, cachedPackage) {
	printed := func() fingerprint {
		return fingerprint{
			pkgPrint: pkgPrint{Whole: "whole", Typed: true, Decls: map[string]declPrint{
				"a.go:F":           fn("F", 3, 7, "f", "limit"),
				"a.go:G":           fn("G", 9, 11, "g"),
				"a.go:const limit": spec(kindConst, "limit", 13, "limit"),
				"a_test.go:TestF":  testFn("TestF", "TestF", "tf", "F"),
				"a_test.go:TestG":  testFn("TestG", "TestG", "tg", "G"),
				"a_test.go:helper": testFn("helper", "", "h"),
			}},
			Inputs: inputsHash,
			Deps: map[string]pkgPrint{"example.com/dep": {Whole: "dep-whole", Typed: true, Decls: map[string]declPrint{
				"dep/dep.go:Clamp":     {Hash: "clamp", Sig: "clamp-sig", Names: []string{"Clamp"}, header: idents("Clamp"), text: idents("Clamp")},
				"dep/dep.go:Size":      {Hash: "size", Sig: "size-sig", Names: []string{"Size"}, header: idents("Size"), text: idents("Size", "count")},
				"dep/dep.go:type Box":  {Hash: "box", Kind: kindType, Names: []string{"Box"}, header: idents("Box"), text: idents("Box")},
				"dep/dep.go:var count": {Hash: "count", Kind: kindVar, Names: []string{"count"}, header: idents("count"), text: idents("count")},
			}}},
		}
	}

	return printed(), cachedPackage{
		Fingerprint: printed(),
		Tests: map[string]Profile{
			"TestF": {"a.go": {{StartLine: 4, StartCol: 1, EndLine: 6, EndCol: 2}}},
			"TestG": {"a.go": {{StartLine: 10, StartCol: 1, EndLine: 10, EndCol: 2}}},
		},
		Deps: map[string][]string{"TestF": {"dep/dep.go:Clamp"}},
	}
}

func keptNames(kept map[string]Profile) []string {
	var names []string
	for name := range kept {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// reachesReg stands for a package's reach once it imports reg.
const reachesReg = "reaches reg"

// Each change here is attributable, and the mappings it could not have
// reached are kept: by body to the tests that executed it, by name to whatever
// names it.
func TestReusableKeepsWhatTheChangeCannotReach(t *testing.T) {
	t.Parallel()

	testCases := map[string]keepCase{
		"nothing": {
			change: func(*fingerprint, *cachedPackage) {},
			kept:   []string{"TestF", "TestG"},
		},
		"a body only one test executed": {
			change: func(now *fingerprint, _ *cachedPackage) {
				now.Decls["a.go:F"] = fn("F", 3, 7, "f2", "limit")
			},
			kept: []string{"TestG"},
		},
		// A variable added or removed is a changed name, and the others kept
		// their order around it.
		"a variable added among others that kept their order": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.InitOrder = []string{"a", "gone", "b"}
				now.InitOrder = []string{"new", "a", "b"}
			},
			kept: []string{"TestF", "TestG"},
		},
		// G names F without executing it — it passes F as a value — so F's
		// body cannot reach TestG, and its signature can.
		"a body of a function another passes as a value": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a.go:G"] = fn("G", 9, 11, "g", "F")
				now.Decls["a.go:G"] = fn("G", 9, 11, "g", "F")
				now.Decls["a.go:F"] = fn("F", 3, 7, "f2", "limit")
			},
			kept: []string{"TestG"},
		},
		"a signature of a function another passes as a value": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a.go:G"] = fn("G", 9, 11, "g", "F")
				now.Decls["a.go:G"] = fn("G", 9, 11, "g", "F")
				f := fn("F", 3, 7, "f2", "limit")
				f.Sig = "f2-sig"
				now.Decls["a.go:F"] = f
			},
			kept: []string{},
		},
		"a constant only one function names": {
			change: func(now *fingerprint, _ *cachedPackage) {
				now.Decls["a.go:const limit"] = spec(kindConst, "limit", 13, "limit2")
			},
			kept: []string{"TestG"},
		},
		// U's header names T, and G's signature names U: a changed T reaches
		// G through two headers, though G's text never names T.
		"a type reached through headers": {
			change: func(now *fingerprint, cached *cachedPackage) {
				g := fn("G", 9, 11, "g", "U")
				g.header["U"] = true
				for _, p := range []*fingerprint{now, &cached.Fingerprint} {
					p.Decls["a.go:G"] = g
					p.Decls["a.go:type U"] = spec(kindType, "U", 15, "u", "T")
					p.Decls["a.go:type T"] = spec(kindType, "T", 16, "t")
				}
				now.Decls["a.go:type T"] = spec(kindType, "T", 16, "t2")
			},
			kept: []string{"TestF"},
		},
		// A new name can rebind an existing mention of it: a predeclared one.
		"an added name": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a.go:G"] = fn("G", 9, 11, "g", "min")
				now.Decls["a.go:G"] = fn("G", 9, 11, "g", "min")
				now.Decls["a.go:min"] = fn("min", 20, 22, "min")
			},
			kept: []string{"TestF"},
		},
		"a removed name": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a.go:min"] = fn("min", 20, 22, "min")
				cached.Fingerprint.Decls["a.go:G"] = fn("G", 9, 11, "g", "min")
				now.Decls["a.go:G"] = fn("G", 9, 11, "g", "min")
			},
			kept: []string{"TestF"},
		},
		// A method changes the interfaces its receiver satisfies, so it is a
		// change to the receiver's name too.
		"an added method": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a.go:F"] = fn("F", 3, 7, "f", "limit", "T")
				now.Decls["a.go:F"] = fn("F", 3, 7, "f", "limit", "T")
				now.Decls["a.go:T.String"] = declPrint{Hash: "s", Sig: "s", Kind: kindMethod, Names: []string{"String", "T"},
					File: "a.go", Start: 20, End: 22, header: idents("String"), text: idents("String")}
			},
			kept: []string{"TestG"},
		},
		"a changed test": {
			change: func(now *fingerprint, _ *cachedPackage) {
				now.Decls["a_test.go:TestF"] = testFn("TestF", "TestF", "tf2", "F")
			},
			kept: []string{"TestG"},
		},
		// A test file is not instrumented, so a helper's body is followed by
		// name, from the test that names it.
		"a helper's body": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a_test.go:TestG"] = testFn("TestG", "TestG", "tg", "G", "helper")
				now.Decls["a_test.go:TestG"] = testFn("TestG", "TestG", "tg", "G", "helper")
				now.Decls["a_test.go:helper"] = testFn("helper", "", "h2")
			},
			kept: []string{"TestF"},
		},
		// Every mention of st is text that did not change, calling into
		// another package now.
		"an import bound to another path": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Imports = map[string]map[string]string{"a.go": {"st": "m/pg"}}
				now.Imports = map[string]map[string]string{"a.go": {"st": "m/mem"}}
				cached.Fingerprint.Decls["a.go:F"] = fn("F", 3, 7, "f", "st")
				now.Decls["a.go:F"] = fn("F", 3, 7, "f", "st")
			},
			kept: []string{"TestG"},
		},
		// An unnamed import whose package name is unknown binds a name nobody
		// can say, so the whole file is rebound.
		"an unnamed import of an unknown package": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Imports = map[string]map[string]string{"a.go": {"?m/x": "m/x"}}
				now.Imports = map[string]map[string]string{"a.go": {"?m/y": "m/y"}}
			},
			kept: []string{},
		},
		"a dependency body one test executed": {
			change: func(now *fingerprint, _ *cachedPackage) {
				d := now.Deps["example.com/dep"].Decls["dep/dep.go:Clamp"]
				d.Hash = "clamp2"
				now.Deps["example.com/dep"].Decls["dep/dep.go:Clamp"] = d
			},
			kept: []string{"TestG"},
		},
		// Box is exported, so its change crosses into the package.
		"a dependency type the package names": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a.go:G"] = fn("G", 9, 11, "g", "Box")
				now.Decls["a.go:G"] = fn("G", 9, 11, "g", "Box")
				d := now.Deps["example.com/dep"].Decls["dep/dep.go:type Box"]
				d.Hash = "box2"
				now.Deps["example.com/dep"].Decls["dep/dep.go:type Box"] = d
			},
			kept: []string{"TestF"},
		},
		// count is not, so it reaches only the dependency's own entities —
		// Size, which nobody executed — and not G's local of that name.
		"a dependency's unexported name": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a.go:G"] = fn("G", 9, 11, "g", "count")
				now.Decls["a.go:G"] = fn("G", 9, 11, "g", "count")
				d := now.Deps["example.com/dep"].Decls["dep/dep.go:var count"]
				d.Hash = "count2"
				now.Deps["example.com/dep"].Decls["dep/dep.go:var count"] = d
			},
			kept: []string{"TestF", "TestG"},
		},
		// TestG executed the dependency somewhere no entity could be named.
		"a change anywhere in a dependency recorded whole": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Deps["TestG"] = []string{depWhole("example.com/dep")}
				d := now.Deps["example.com/dep"].Decls["dep/dep.go:Size"]
				d.Hash = "size2"
				now.Deps["example.com/dep"].Decls["dep/dep.go:Size"] = d
			},
			kept: []string{"TestF"},
		},
		// No declaration says what it is, so nothing says what reaches it.
		"a mapping no test declaration accounts for": {
			change: func(_ *fingerprint, cached *cachedPackage) {
				cached.Tests["TestGone"] = Profile{"a.go": {{StartLine: 10, EndLine: 10}}}
			},
			kept: []string{"TestF", "TestG"},
		},
	}

	runKeepCases(t, testCases)
}

// keepCase is a change and the mappings reuse must keep across it.
type keepCase struct {
	change func(now *fingerprint, cached *cachedPackage)
	kept   []string
}

func runKeepCases(t *testing.T, testCases map[string]keepCase) {
	t.Helper()

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			now, cached := pkg()
			tc.change(&now, &cached)
			kept, why := reusable(cached, now)
			if why != "" {
				t.Fatalf("want a narrowed re-map, got a whole-package one: %s", why)
			}
			got := keptNames(kept)
			if got == nil {
				got = []string{}
			}
			if diff := cmp.Diff(tc.kept, got); diff != "" {
				t.Errorf("kept the wrong mappings (-want +got):\n%s", diff)
			}
		})
	}
}

// The order packages are initialised in moves only with what a package
// reaches by import, and only an initialiser can see it.
func TestReusableKeepsWhatTheOrderCannotReach(t *testing.T) {
	t.Parallel()

	runKeepCases(t, map[string]keepCase{
		// The order packages are initialised in may have moved, and nothing
		// instrumented runs at initialisation to see it.
		"an import added where nothing observes initialisation": {
			change: func(now *fingerprint, _ *cachedPackage) {
				now.Imports = map[string]map[string]string{"a.go": {"reg": "example.com/reg"}}
				now.Reach = reachesReg
			},
			kept: []string{"TestF", "TestG"},
		},
		// The package reached reg already, so it is initialised where it
		// was, and the init sees what it saw.
		"an import added that the package already reached, beside an init": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a.go:init#0"] = declPrint{Hash: "i", Kind: kindInit, File: "a.go", Start: 20, End: 22}
				now.Decls["a.go:init#0"] = declPrint{Hash: "i", Kind: kindInit, File: "a.go", Start: 20, End: 22}
				now.Imports = map[string]map[string]string{"a.go": {"reg": "example.com/reg"}}
			},
			kept: []string{"TestF", "TestG"},
		},
		// TestMain runs once every package is initialised.
		"a package reaching more beside TestMain": {
			change: func(now *fingerprint, cached *cachedPackage) {
				cached.Fingerprint.Decls["a_test.go:TestMain#0"] = declPrint{Hash: "m", Kind: kindMain, File: "a_test.go"}
				now.Decls["a_test.go:TestMain#0"] = declPrint{Hash: "m", Kind: kindMain, File: "a_test.go"}
				now.Reach = reachesReg
			},
			kept: []string{"TestF", "TestG"},
		},
	})
}

// Everything here re-maps the whole package. The direction matters: a mapping
// wrongly kept means a test that could kill a mutant is never run, so the
// mutant reports LIVED and the gate goes red on something nobody can reproduce.
func TestReusableRefusesWhatItCannotAttribute(t *testing.T) {
	t.Parallel()

	initAt := func(hash string, mentions ...string) declPrint {
		m := idents(mentions...)

		return declPrint{Hash: hash, Kind: kindInit, File: "a.go", Start: 20, End: 22, header: m, text: m}
	}
	setDep := func(now *fingerprint, key string, d declPrint) {
		now.Deps["example.com/dep"].Decls[key] = d
	}

	testCases := map[string]func(now *fingerprint, cached *cachedPackage){
		// An entry from before the fingerprint existed, or one whose package
		// could not be read, says nothing about what changed.
		"no fingerprint at all": func(_ *fingerprint, cached *cachedPackage) {
			cached.Fingerprint = fingerprint{}
		},
		"nothing recorded what it was built from": func(_ *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Inputs = ""
		},
		// What it is built from besides the package and its instrumented
		// dependencies moved, and no record here holds its lines.
		"the inputs moved": func(now *fingerprint, _ *cachedPackage) {
			now.Inputs = "other"
		},
		"a dependency was added": func(now *fingerprint, _ *cachedPackage) {
			now.Deps["example.com/new"] = pkgPrint{Whole: "new"}
		},
		"a dependency was replaced": func(now *fingerprint, _ *cachedPackage) {
			now.Deps["example.com/other"] = now.Deps["example.com/dep"]
			delete(now.Deps, "example.com/dep")
		},
		"the package's whole print moved": func(now *fingerprint, _ *cachedPackage) {
			now.Whole = "other"
		},
		// The order is a fact only the type-checker has, so a print it could
		// not read is one nothing can be narrowed from.
		"the package could not be type-checked": func(now *fingerprint, _ *cachedPackage) {
			now.Typed = false
		},
		"the cached package was not type-checked": func(_ *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Typed = false
		},
		"a dependency could not be type-checked": func(now *fingerprint, _ *cachedPackage) {
			dep := now.Deps["example.com/dep"]
			dep.Typed = false
			now.Deps["example.com/dep"] = dep
		},
		// Two variables both prints initialise traded places, which no
		// entity's print shows.
		"two variables initialised in the other order": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.InitOrder = []string{"a", "b", "c"}
			now.InitOrder = []string{"b", "a"}
		},
		"a dependency's variables initialised in the other order": func(now *fingerprint, cached *cachedPackage) {
			was, dep := cached.Fingerprint.Deps["example.com/dep"], now.Deps["example.com/dep"]
			was.InitOrder, dep.InitOrder = []string{"a", "b"}, []string{"b", "a"}
			cached.Fingerprint.Deps["example.com/dep"], now.Deps["example.com/dep"] = was, dep
		},
		"a dependency's whole print moved": func(now *fingerprint, _ *cachedPackage) {
			now.Deps["example.com/dep"] = pkgPrint{Whole: "other", Decls: now.Deps["example.com/dep"].Decls}
		},
		// An init runs before every test in the binary.
		"an init added": func(now *fingerprint, _ *cachedPackage) {
			now.Decls["a.go:init#0"] = initAt("i")
		},
		"an init removed": func(_ *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:init#0"] = initAt("i")
		},
		"an init changed": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:init#0"] = initAt("i")
			now.Decls["a.go:init#0"] = initAt("i2")
		},
		"an init naming a changed name": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:init#0"] = initAt("i", "limit")
			now.Decls["a.go:init#0"] = initAt("i", "limit")
			now.Decls["a.go:const limit"] = spec(kindConst, "limit", 13, "limit2")
		},
		"an init in a dependency": func(now *fingerprint, _ *cachedPackage) {
			setDep(now, "dep/dep.go:init#0", declPrint{Hash: "i", Kind: kindInit})
		},
		// A var that calls something at initialisation runs it for every
		// test, and so does TestMain.
		"a var run at initialisation changed": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:run start"] = spec(kindRun, "start", 15, "s")
			now.Decls["a.go:run start"] = spec(kindRun, "start", 15, "s2")
		},
		"a var run at initialisation naming a changed name": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:run start"] = spec(kindRun, "start", 15, "s", "limit")
			now.Decls["a.go:run start"] = spec(kindRun, "start", 15, "s", "limit")
			now.Decls["a.go:const limit"] = spec(kindConst, "limit", 13, "limit2")
		},
		"a var run at initialisation in a dependency naming a changed name": func(now *fingerprint, cached *cachedPackage) {
			run := declPrint{Hash: "r", Kind: kindRun, Names: []string{"Start"}, header: idents("Box"), text: idents("Box")}
			cached.Fingerprint.Deps["example.com/dep"].Decls["dep/dep.go:run Start"] = run
			setDep(now, "dep/dep.go:run Start", run)
			setDep(now, "dep/dep.go:type Box", declPrint{Hash: "box2", Kind: kindType, Names: []string{"Box"}})
		},
		"a var becoming one run at initialisation": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:var start"] = spec(kindVar, "start", 15, "s")
			now.Decls["a.go:run start"] = spec(kindRun, "start", 15, "s2")
		},
		// A package's imports are its place in the order packages are
		// initialised in, and something initialised sees that order.
		"a package reaching more beside an init": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:init#0"] = initAt("i")
			now.Decls["a.go:init#0"] = initAt("i")
			now.Reach = reachesReg
		},
		"a package reaching other packages beside a var run at initialisation": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Reach = reachesReg
			cached.Fingerprint.Decls["a.go:run start"] = spec(kindRun, "start", 15, "s")
			now.Decls["a.go:run start"] = spec(kindRun, "start", 15, "s")
			now.Reach = "reaches reg2"
		},
		"a dependency reaching more beside an init in the package": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:init#0"] = initAt("i")
			now.Decls["a.go:init#0"] = initAt("i")
			dep := now.Deps["example.com/dep"]
			dep.Reach = reachesReg
			now.Deps["example.com/dep"] = dep
		},
		"a package reaching less where a dependency's var reads another package": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Reach = reachesReg
			observes := declPrint{Hash: "o", Kind: kindVar, Names: []string{"Snapshot"}, Observes: true}
			cached.Fingerprint.Deps["example.com/dep"].Decls["dep/dep.go:var Snapshot"] = observes
			setDep(now, "dep/dep.go:var Snapshot", observes)
		},
		"a TestMain": func(now *fingerprint, _ *cachedPackage) {
			now.Decls["a_test.go:TestMain#0"] = declPrint{Hash: "m", Kind: kindMain, File: "a_test.go"}
		},
		// Nothing to pair it by.
		"a blank declaration changed": func(now *fingerprint, cached *cachedPackage) {
			cached.Fingerprint.Decls["a.go:blank #0"] = spec(kindBlank, "", 15, "b")
			now.Decls["a.go:blank #0"] = spec(kindBlank, "", 15, "b2")
		},
		"a linknamed declaration changed": func(now *fingerprint, _ *cachedPackage) {
			f := fn("F", 3, 7, "f2", "limit")
			f.Linked = true
			now.Decls["a.go:F"] = f
		},
		// The profile and the print disagree about the package: the mapping
		// covers a file no entity accounts for, so where its blocks have moved
		// to cannot be worked out.
		"a kept block in no entity": func(_ *fingerprint, cached *cachedPackage) {
			cached.Tests["TestG"] = Profile{"elsewhere.go": {{StartLine: 4, EndLine: 6}}}
		},
	}

	for name, change := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			now, cached := pkg()
			change(&now, &cached)
			if kept, why := reusable(cached, now); why == "" {
				t.Errorf("want the whole package re-mapped, got %v kept", keptNames(kept))
			}
		})
	}
}

// A kept mapping has to answer about where its code is now: each block moves
// by the distance its entity moved, which may differ from entity to entity.
func TestReusableMovesKeptMappingsWithTheirEntities(t *testing.T) {
	t.Parallel()

	now, cached := pkg()
	// F grew by two lines and changed; G moved down with it; a var holding a
	// function literal, which instrumented code can live in, moved by one.
	cached.Fingerprint.Decls["a.go:var hook"] = declPrint{Hash: "hook", Kind: kindVar, Names: []string{"hook"},
		File: "a.go", Start: 15, End: 17, header: idents("hook"), text: idents("hook")}
	now.Decls["a.go:var hook"] = declPrint{Hash: "hook", Kind: kindVar, Names: []string{"hook"},
		File: "a.go", Start: 16, End: 18, header: idents("hook"), text: idents("hook")}
	now.Decls["a.go:F"] = fn("F", 3, 9, "f2", "limit")
	now.Decls["a.go:G"] = fn("G", 11, 13, "g")
	cached.Tests["TestHook"] = Profile{"a.go": {{StartLine: 16, StartCol: 3, EndLine: 16, EndCol: 9}}}
	now.Decls["a_test.go:TestHook"] = testFn("TestHook", "TestHook", "th")
	cached.Fingerprint.Decls["a_test.go:TestHook"] = testFn("TestHook", "TestHook", "th")

	kept, why := reusable(cached, now)
	if why != "" {
		t.Fatalf("want a narrowed re-map, got a whole-package one: %s", why)
	}
	want := map[string]Profile{
		"TestG":    {"a.go": {{StartLine: 12, StartCol: 1, EndLine: 12, EndCol: 2}}},
		"TestHook": {"a.go": {{StartLine: 17, StartCol: 3, EndLine: 17, EndCol: 9}}},
	}
	if diff := cmp.Diff(want, kept); diff != "" {
		t.Errorf("kept the wrong mappings, or in the wrong place (-want +got):\n%s", diff)
	}
}

func TestReboundNamesComparesImportTablesByLocalName(t *testing.T) {
	t.Parallel()

	was := map[string]map[string]string{
		"a.go": {"st": "m/pg", "fmt": "fmt", "gone": "m/gone"},
		"b.go": {"?m/x": "m/x"},
		"c.go": {"os": "os"},
	}
	now := map[string]map[string]string{
		"a.go": {"st": "m/mem", "fmt": "fmt", "added": "m/added"},
		"b.go": {"?m/y": "m/y"},
		"c.go": {"os": "os"},
	}
	got := reboundNames(was, now)
	if a := got["a.go"]; a.all || !a.names["st"] || !a.names["gone"] || !a.names["added"] || a.names["fmt"] {
		t.Errorf("want st, gone and added rebound in a.go and fmt not, got %+v", a)
	}
	if !got["b.go"].all {
		t.Errorf("want b.go rebound whole, got %+v", got["b.go"])
	}
	if _, found := got["c.go"]; found {
		t.Errorf("want nothing rebound in c.go, got %+v", got["c.go"])
	}
}
