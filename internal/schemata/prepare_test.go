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
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

const prepareMod = "example.com/prepare"

// prepareModule is the prepare fixture as the engine sees it, run from the
// module root.
func prepareModule(t *testing.T) gomodule.GoModule {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "prepare"))
	if err != nil {
		t.Fatal(err)
	}

	return gomodule.GoModule{Name: prepareMod, Root: root, CallingDir: "."}
}

// streamMutants discovers the mutants of every non-test file of mod the way
// the engine's walk does -- file paths relative to the calling directory,
// every type the token table maps -- in walk order, all RUNNABLE.
func streamMutants(t *testing.T, mod gomodule.GoModule) []mutator.Mutator {
	t.Helper()
	var out []mutator.Mutator
	dir := filepath.Join(mod.Root, mod.CallingDir)
	err := fs.WalkDir(os.DirFS(dir), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
			return err
		}
		src, err := os.ReadFile(filepath.Join(dir, p)) //nolint:gosec // G304: a fixture file
		if err != nil {
			return err
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, p, src, parser.ParseComments)
		if err != nil {
			return err
		}
		pkg := mod.Name + "/" + path.Dir(p)
		ast.Inspect(file, func(n ast.Node) bool {
			node, ok := engine.NewTokenNode(n)
			if !ok {
				return true
			}
			types, ok := engine.MutantTypesFor(node)
			if !ok {
				return true
			}
			for _, mt := range types {
				m := engine.NewTokenMutant(pkg, set, file, node)
				m.SetType(mt)
				m.SetStatus(mutator.Runnable)
				out = append(out, m)
			}

			return true
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

// realNullRun runs bin in pkgDir the way a worker would, without
// GREMLINS_MUTANT, and fails with its output.
func realNullRun(bin, pkgDir string) error {
	cmd := exec.Command(bin, "-test.count=1")
	cmd.Dir = pkgDir
	cmd.Env = slicesWithout(os.Environ(), "GREMLINS_MUTANT=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out)
	}

	return nil
}

func slicesWithout(env []string, prefix string) []string {
	var out []string
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}

	return out
}

// nullRuns records the null runs Prepare makes and fails them on demand.
type nullRuns struct {
	mu    sync.Mutex
	calls map[string]int // by package directory
	dirs  map[string]string
	// builds is the build each binary's null run was handed.
	builds map[string]*schemata.Build
	fail   func(pkgDir string, call int) error
}

func (n *nullRuns) run(b *schemata.Build, bin, pkgDir string) error {
	n.mu.Lock()
	if n.calls == nil {
		n.calls = map[string]int{}
		n.dirs = map[string]string{}
		n.builds = map[string]*schemata.Build{}
	}
	n.calls[pkgDir]++
	call := n.calls[pkgDir]
	n.dirs[bin] = pkgDir
	n.builds[bin] = b
	n.mu.Unlock()
	if n.fail != nil {
		if err := n.fail(pkgDir, call); err != nil {
			return err
		}
	}

	return realNullRun(bin, pkgDir)
}

func ownPackage(pkg string) []string { return []string{pkg} }

// checkAccounting asserts every input mutant is in exactly one of placed and
// netted, that placed ids are the 1-based stream positions, and that every
// netted mutant has a reason.
func checkAccounting(t *testing.T, in []mutator.Mutator, plan schemata.Plan) {
	t.Helper()
	index := map[mutator.Mutator]int{}
	for i, m := range in {
		index[m] = i + 1
	}
	seen := map[mutator.Mutator]bool{}
	for _, p := range plan.Placed {
		if seen[p.Mutator] {
			t.Errorf("%s at %s accounted twice", p.Mutator.Type(), p.Mutator.Position())
		}
		seen[p.Mutator] = true
		if p.ID != index[p.Mutator] {
			t.Errorf("%s at %s: id %d, want its stream position %d", p.Mutator.Type(), p.Mutator.Position(), p.ID, index[p.Mutator])
		}
	}
	for _, n := range plan.Netted {
		if seen[n.Mutator] {
			t.Errorf("%s at %s accounted twice", n.Mutator.Type(), n.Mutator.Position())
		}
		seen[n.Mutator] = true
		if n.Reason == "" {
			t.Errorf("%s at %s netted without a reason", n.Mutator.Type(), n.Mutator.Position())
		}
	}
	if len(plan.Placed)+len(plan.Netted) != len(in) {
		t.Errorf("placed %d + netted %d != %d runnable", len(plan.Placed), len(plan.Netted), len(in))
	}
	for _, m := range in {
		if !seen[m] {
			t.Errorf("%s at %s is neither placed nor netted", m.Type(), m.Position())
		}
	}
}

// planByPkg is a plan's mutant types, and netted reasons, by package.
type planByPkg struct {
	placed, netted, reasons map[string][]string
}

func byPkg(plan schemata.Plan) planByPkg {
	g := planByPkg{placed: map[string][]string{}, netted: map[string][]string{}, reasons: map[string][]string{}}
	for _, p := range plan.Placed {
		g.placed[p.Mutator.Pkg()] = append(g.placed[p.Mutator.Pkg()], p.Mutator.Type().String())
	}
	for _, n := range plan.Netted {
		g.netted[n.Mutator.Pkg()] = append(g.netted[n.Mutator.Pkg()], n.Mutator.Type().String())
		g.reasons[n.Mutator.Pkg()] = append(g.reasons[n.Mutator.Pkg()], n.Reason)
	}

	return g
}

// TestPrepare is not parallel: it captures the process-wide log.
func TestPrepare(t *testing.T) {
	mod := prepareModule(t)
	in := streamMutants(t, mod)
	workDir := t.TempDir()
	runs := &nullRuns{fail: func(pkgDir string, call int) error {
		switch filepath.Base(pkgDir) {
		case "a":
			if call == 1 {
				return errors.New("flaky once")
			}
		case "c":
			return errors.New("boom first line\nsecond line")
		}

		return nil
	}}

	var out, eOut bytes.Buffer
	log.Init(&out, &eOut)
	defer log.Reset()

	plan, err := schemata.Prepare(context.Background(), mod, workDir, "", in, ownPackage, 2*time.Minute, 0, runs.run)
	if err != nil {
		t.Fatal(err)
	}
	checkAccounting(t, in, plan)

	g := byPkg(plan)
	placed, netted, reasons := g.placed, g.netted, g.reasons
	a, b, c := prepareMod+"/a", prepareMod+"/b", prepareMod+"/c"
	if got := strings.Join(placed[a], ","); got != "ARITHMETIC_BASE" {
		t.Errorf("placed in a = %s, want ARITHMETIC_BASE (retried null run passes)", got)
	}
	if got := strings.Join(netted[a], ","); got != "INVERT_ASSIGNMENTS,REMOVE_SELF_ASSIGNMENTS" {
		t.Errorf("netted in a = %s, want the unsupported += mutants", got)
	}
	for _, r := range reasons[a] {
		if !strings.Contains(r, schemata.ErrUnsupported.Error()) {
			t.Errorf("a's netted reason %q does not name the unsupported site", r)
		}
	}
	if got := strings.Join(placed[b], ","); got != "INVERT_NEGATIVES,ARITHMETIC_BASE" {
		t.Errorf("placed in b = %s, want both mutants of its '-' (its test reads the original source)", got)
	}
	if len(placed[c]) != 0 || len(netted[c]) != 2 {
		t.Errorf("c: placed %v netted %v, want both mutants netted", placed[c], netted[c])
	}
	for _, r := range reasons[c] {
		if r != "null-mutant run failed: boom first line" {
			t.Errorf("c's netted reason = %q", r)
		}
	}

	// One null run per built binary, in the original package directory; a
	// second only after a failure.
	for _, tc := range []struct {
		dir   string
		calls int
	}{{"a", 2}, {"b", 1}, {"c", 2}} {
		dir := filepath.Join(mod.Root, tc.dir)
		if runs.calls[dir] != tc.calls {
			t.Errorf("null runs in %s = %d, want %d", dir, runs.calls[dir], tc.calls)
		}
	}
	for _, p := range []string{a, b, c} {
		bin, ok := plan.Build.Binaries[p]
		if !ok {
			t.Fatalf("no binary for %s", p)
		}
		if !strings.HasPrefix(bin, workDir) || !strings.HasPrefix(plan.Build.Dir, workDir) {
			t.Errorf("build output %s / %s is outside workDir %s", bin, plan.Build.Dir, workDir)
		}
		// The null run gets the build, for the overlay its go commands need.
		got := runs.builds[bin]
		if got == nil || got.Dir != plan.Build.Dir || got.Src != plan.Build.Src || got.Binaries[p] != bin || !slices.Equal(got.Rewritten, plan.Build.Rewritten) {
			t.Errorf("null run of %s was handed build %+v, want %+v", p, got, plan.Build)
		}
	}
	checkBesideRewrittenFails(t, mod, plan, b)

	if !strings.Contains(out.String(), "schemata: built 3 packages in ") {
		t.Errorf("info log lacks the build line:\n%s", out.String())
	}
	if !strings.Contains(out.String(), fmt.Sprintf("schemata: %d mutants placed, %d netted", len(plan.Placed), len(plan.Netted))) {
		t.Errorf("info log lacks the counts:\n%s", out.String())
	}
	checkPlacedLogged(t, out.String(), plan)
	for _, n := range plan.Netted {
		line := fmt.Sprintf("schemata: %s at %s goes through the per-mutant path: %s\n", n.Mutator.Type(), n.Mutator.Position(), n.Reason)
		if !strings.Contains(eOut.String(), line) {
			t.Errorf("error log lacks %q", line)
		}
	}
}

// checkBesideRewrittenFails requires the binary of pkg, b's, to fail in a
// directory holding b's rewritten source, where the helper file sits beside
// it: the fixture's original-source check is then not vacuous.
func checkBesideRewrittenFails(t *testing.T, mod gomodule.GoModule, plan schemata.Plan, pkg string) {
	t.Helper()
	dst := t.TempDir()
	for _, dir := range []string{filepath.Join(mod.Root, "b"), filepath.Join(plan.Build.Src, "b")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304: a fixture or build file
			if err == nil {
				err = os.WriteFile(filepath.Join(dst, e.Name()), data, 0o600) //nolint:gosec // G703: a test temp dir
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := realNullRun(plan.Build.Binaries[pkg], dst); err == nil {
		t.Error("b's binary passes beside the rewritten source: the original-source check proves nothing")
	}
}

// checkPlacedLogged requires the info log out to name each placed mutant's id.
func checkPlacedLogged(t *testing.T, out string, plan schemata.Plan) {
	t.Helper()
	for _, p := range plan.Placed {
		line := fmt.Sprintf("schemata: id %d = %s %s\n", p.ID, p.Mutator.Position(), p.Mutator.Type())
		if !strings.Contains(out, line) {
			t.Errorf("info log lacks %q", line)
		}
	}
}

func TestPrepareNets(t *testing.T) {
	t.Parallel()
	a, b := prepareMod+"/a", prepareMod+"/b"
	testCases := map[string]struct {
		testPkgs func(string) []string
		mutate   func([]mutator.Mutator) []mutator.Mutator
		// wantNetted maps a package to a substring every netted reason of
		// it contains; its placeable mutants must all be netted.
		wantNetted map[string]string
		wantPlaced []string
	}{
		"a mutant whose test package fails to build": {
			testPkgs: func(p string) []string {
				if p == a {
					return []string{p, prepareMod + "/nosuch"}
				}

				return []string{p}
			},
			wantNetted: map[string]string{a: "nosuch"},
			wantPlaced: []string{b},
		},
		"a mutant at no operator of the loaded source": {
			mutate: func(in []mutator.Mutator) []mutator.Mutator {
				for i, m := range in {
					if m.Pkg() == b {
						in[i] = movedMutant{Mutator: m, line: 999}
					}
				}

				return in
			},
			wantNetted: map[string]string{b: "no mutation site"},
			wantPlaced: []string{a},
		},
		// Only that package's mutants are netted: the other's are placed.
		"a mutant in a directory outside the module root": {
			mutate: func(in []mutator.Mutator) []mutator.Mutator {
				outside := filepath.Join(os.TempDir(), "gremlins-outside-the-module", "x.go")
				for i, m := range in {
					if m.Pkg() == b {
						in[i] = movedMutant{Mutator: m, file: outside}
					}
				}

				return in
			},
			wantNetted: map[string]string{b: "is not under the module root"},
			wantPlaced: []string{a},
		},
		"a mutant whose import path is not its directory's": {
			mutate: func(in []mutator.Mutator) []mutator.Mutator {
				for i, m := range in {
					if m.Pkg() == b {
						in[i] = movedMutant{Mutator: m, pkg: prepareMod + "/elsewhere"}
					}
				}

				return in
			},
			wantNetted: map[string]string{prepareMod + "/elsewhere": "elsewhere"},
			wantPlaced: []string{a},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mod := prepareModule(t)
			in := streamMutants(t, mod)
			var kept []mutator.Mutator
			for _, m := range in {
				if m.Pkg() != prepareMod+"/c" && m.Type() != mutator.InvertAssignments && m.Type() != mutator.RemoveSelfAssignments {
					kept = append(kept, m)
				}
			}
			if tc.mutate != nil {
				kept = tc.mutate(kept)
			}
			testPkgs := tc.testPkgs
			if testPkgs == nil {
				testPkgs = ownPackage
			}
			runs := &nullRuns{}
			plan, err := schemata.Prepare(context.Background(), mod, t.TempDir(), "", kept, testPkgs, 2*time.Minute, 0, runs.run)
			if err != nil {
				t.Fatal(err)
			}
			checkAccounting(t, kept, plan)
			g := byPkg(plan)
			placed, netted, reasons := g.placed, g.netted, g.reasons
			for p, want := range tc.wantNetted {
				if len(placed[p]) != 0 || len(netted[p]) == 0 {
					t.Errorf("%s: placed %v netted %v, want all netted", p, placed[p], netted[p])
				}
				for _, r := range reasons[p] {
					if !strings.Contains(r, want) {
						t.Errorf("%s netted with %q, want it to contain %q", p, r, want)
					}
				}
			}
			for _, p := range tc.wantPlaced {
				if len(netted[p]) != 0 || len(placed[p]) == 0 {
					t.Errorf("%s: placed %v netted %v (%v), want all placed", p, placed[p], netted[p], reasons[p])
				}
			}
		})
	}
}

func TestPrepareWithNothingToPlace(t *testing.T) {
	t.Parallel()
	runs := &nullRuns{}
	plan, err := schemata.Prepare(context.Background(), prepareModule(t), t.TempDir(), "", nil, ownPackage, time.Minute, 0, runs.run)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Placed) != 0 || len(plan.Netted) != 0 || len(plan.Build.Binaries) != 0 || len(runs.calls) != 0 {
		t.Errorf("empty input gave %+v and %d null runs", plan, len(runs.calls))
	}
}

func TestPrepareStopsWhenCancelled(t *testing.T) {
	t.Parallel()
	mod := prepareModule(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runs := &nullRuns{}
	if _, err := schemata.Prepare(ctx, mod, t.TempDir(), "", streamMutants(t, mod), ownPackage, time.Minute, 0, runs.run); !errors.Is(err, context.Canceled) {
		t.Errorf("Prepare on a cancelled context: err = %v, want context.Canceled", err)
	}
}

// TestPrepareBuildSet checks that Prepare builds exactly the packages the
// mutants select: an unselected package of a mutant is not built, a mutant
// whose selection is empty is netted, and a selected package with no test
// files is no failure -- its run passes without reach -- and no binary.
func TestPrepareBuildSet(t *testing.T) {
	t.Parallel()
	mod := prepareModule(t)
	b, c, d := prepareMod+"/b", prepareMod+"/c", prepareMod+"/d"
	var in []mutator.Mutator
	for _, m := range streamMutants(t, mod) {
		if m.Pkg() == b || m.Pkg() == c || m.Pkg() == d {
			in = append(in, m)
		}
	}
	testPkgs := func(p string) []string {
		switch p {
		case b:
			return nil
		case c:
			return []string{c, d}
		default:
			return []string{prepareMod + "/a"}
		}
	}
	runs := &nullRuns{}
	plan, err := schemata.Prepare(context.Background(), mod, t.TempDir(), "", in, testPkgs, 2*time.Minute, 0, runs.run)
	if err != nil {
		t.Fatal(err)
	}
	checkAccounting(t, in, plan)
	g := byPkg(plan)
	if len(g.placed[b]) != 0 || len(g.netted[b]) == 0 {
		t.Errorf("b: placed %v netted %v, want all netted", g.placed[b], g.netted[b])
	}
	for _, r := range g.reasons[b] {
		if r != "no test package selected" {
			t.Errorf("b netted with %q, want %q", r, "no test package selected")
		}
	}
	for _, p := range []string{c, d} {
		if len(g.netted[p]) != 0 || len(g.placed[p]) == 0 {
			t.Errorf("%s: placed %v netted %v (%v), want all placed", p, g.placed[p], g.netted[p], g.reasons[p])
		}
	}
	got := slices.Sorted(maps.Keys(plan.Build.Binaries))
	if want := []string{prepareMod + "/a", c}; !slices.Equal(got, want) {
		t.Errorf("binaries for %v, want %v: only the selected packages with tests", got, want)
	}
	if !plan.Build.NoTests[d] || len(plan.Build.NoTests) != 1 {
		t.Errorf("NoTests = %v, want d alone", plan.Build.NoTests)
	}

	// With every mutant's selection empty, nothing is built at all.
	var onlyB []mutator.Mutator
	for _, m := range in {
		if m.Pkg() == b {
			onlyB = append(onlyB, m)
		}
	}
	plan, err = schemata.Prepare(context.Background(), mod, t.TempDir(), "", onlyB, testPkgs, 2*time.Minute, 0, runs.run)
	if err != nil {
		t.Fatal(err)
	}
	checkAccounting(t, onlyB, plan)
	if len(plan.Placed) != 0 || plan.Build.Dir != "" {
		t.Errorf("placed %d, build in %q; want nothing placed and nothing built", len(plan.Placed), plan.Build.Dir)
	}
}

// TestPrepareLogsShortReasons nets twopkgs' mutants with long reasons -- a
// null run of bad failing with a first line past 200 bytes, a build failing
// with many lines for ok -- and requires each mutant's log line to carry the
// reason's first line, cut to 200 bytes, and the full reason to be logged
// once per package.
//
// It is not parallel: it captures the process-wide log.
func TestPrepareLogsShortReasons(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("testdata", "twopkgs"))
	if err != nil {
		t.Fatal(err)
	}
	mod := gomodule.GoModule{Name: "twopkgs", Root: root, CallingDir: "."}
	in := streamMutants(t, mod)
	bad, ok := "twopkgs/bad", "twopkgs/ok"
	long := strings.Repeat("é", 150) // 300 bytes, two to a rune
	runs := &nullRuns{fail: func(pkgDir string, _ int) error {
		if filepath.Base(pkgDir) == "bad" {
			return errors.New(long + "\nsecond line")
		}

		return nil
	}}
	testPkgs := func(p string) []string {
		if p == ok {
			return []string{ok, "twopkgs/nosuch"}
		}

		return []string{p}
	}

	var out, eOut bytes.Buffer
	log.Init(&out, &eOut)
	defer log.Reset()
	plan, err := schemata.Prepare(context.Background(), mod, t.TempDir(), "", in, testPkgs, 2*time.Minute, 0, runs.run)
	if err != nil {
		t.Fatal(err)
	}
	checkAccounting(t, in, plan)
	g := byPkg(plan)
	for _, p := range []string{bad, ok} {
		if len(g.placed[p]) != 0 || len(g.netted[p]) < 1 {
			t.Fatalf("%s: placed %v netted %v, want all netted", p, g.placed[p], g.netted[p])
		}
	}
	if r := g.reasons[bad][0]; r != "null-mutant run failed: "+long {
		t.Errorf("bad's reason = %q, want the null run's first line whole", r)
	}
	if r := g.reasons[ok][0]; !strings.Contains(r, "\n") {
		t.Errorf("ok's reason %q is one line: the build failure proves nothing", r)
	}
	logged := eOut.String()
	for _, n := range plan.Netted {
		short, _, _ := strings.Cut(n.Reason, "\n")
		if len(short) > 200 {
			short = short[:200]
			for !utf8.ValidString(short) {
				short = short[:len(short)-1]
			}
		}
		line := fmt.Sprintf("schemata: %s at %s goes through the per-mutant path: %s\n", n.Mutator.Type(), n.Mutator.Position(), short)
		if !strings.Contains(logged, line) {
			t.Errorf("error log lacks %q", line)
		}
	}
	for _, p := range []string{bad, ok} {
		full := fmt.Sprintf("schemata: per-mutant path for %s: %s\n", p, g.reasons[p][0])
		if c := strings.Count(logged, full); c != 1 {
			t.Errorf("full reason of %s logged %d times, want once:\n%s", p, c, logged)
		}
	}
	// The build output past the first line is in the full reason alone.
	_, rest, _ := strings.Cut(g.reasons[ok][0], "\n")
	if c := strings.Count(logged, rest); c != 1 {
		t.Errorf("ok's build output logged %d times, want once", c)
	}
}

// TestPrepareStopsLoadingWhenCancelled cancels Prepare while it loads and
// type-checks the fixture's packages and requires it to return the
// context's error within two seconds: the loads must honour the context.
func TestPrepareStopsLoadingWhenCancelled(t *testing.T) {
	t.Parallel()
	mod := prepareModule(t)
	in := streamMutants(t, mod)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	runs := &nullRuns{}
	_, err := schemata.Prepare(ctx, mod, t.TempDir(), "", in, ownPackage, time.Minute, 0, runs.run)
	if el := time.Since(start); !errors.Is(err, context.Canceled) || el > 2*time.Second {
		t.Errorf("Prepare cancelled during its loads: err = %v after %s, want context.Canceled within 2s", err, el)
	}
}

// movedMutant reports a different line, package or file than the mutant it
// wraps.
type movedMutant struct {
	mutator.Mutator
	line int
	pkg  string
	file string
}

func (m movedMutant) Position() token.Position {
	p := m.Mutator.Position()
	if m.line != 0 {
		p.Line = m.line
	}
	if m.file != "" {
		p.Filename = m.file
	}

	return p
}

func (m movedMutant) Pkg() string {
	if m.pkg != "" {
		return m.pkg
	}

	return m.Mutator.Pkg()
}

// TestPrepareLineDirective holds a mutant whose position a line directive
// moves onto another operator of its own file to the per-mutant path: the
// engine names mutants by adjusted positions, and placing one at the operator
// the adjusted position names would switch the wrong operator.
func TestPrepareLineDirective(t *testing.T) {
	t.Parallel()
	src := "package l\n\nfunc Add(a, b int) int { return a + b }\nfunc Mul(a, b int) int { return a * b }\nfunc Sub(a, b int) int { return a /*line l.go:3:35*/- b }\n"
	dir := writeModule(t, map[string][]byte{
		"l/l.go": []byte(src),
		"l/l_test.go": []byte("package l\n\nimport \"testing\"\n\nfunc TestL(t *testing.T) {\n" +
			"\tif Add(2, 3) != 5 || Mul(2, 3) != 6 || Sub(3, 1) != 2 {\n\t\tt.Error(\"wrong\")\n\t}\n}\n"),
	})
	mod := gomodule.GoModule{Name: "fixture", Root: dir, CallingDir: "."}

	// Mul's and Sub's mutants, not Add's, as when Add is not covered: the
	// directive gives Sub's '-' the position of Add's '+', where
	// ARITHMETIC_BASE has a form; Mul, before the directive, is not moved.
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "l/l.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var in []mutator.Mutator
	ast.Inspect(file, func(n ast.Node) bool {
		node, ok := engine.NewTokenNode(n)
		if !ok || set.PositionFor(node.TokPos, false).Line < 4 {
			return true
		}
		mts, _ := engine.MutantTypesFor(node)
		for _, mt := range mts {
			if mt != mutator.ArithmeticBase {
				// INVERT_NEGATIVES has no form at '+': it would drop the
				// site, hiding the misplacement.
				continue
			}
			m := engine.NewTokenMutant("fixture/l", set, file, node)
			m.SetType(mt)
			m.SetStatus(mutator.Runnable)
			in = append(in, m)
		}

		return true
	})
	if len(in) == 0 {
		t.Fatal("no mutant on Sub's line")
	}
	if len(in) != 2 {
		t.Fatalf("%d mutants, want Mul's and Sub's", len(in))
	}
	if got, want := in[1].Position(), set.PositionFor(file.Pos()+token.Pos(strings.Index(src, "+")), false); got.Line != want.Line || got.Column != want.Column {
		t.Fatalf("Sub's mutant is at %v, want the position of Add's '+' %v: the fixture moves nothing", got, want)
	}

	runs := &nullRuns{}
	plan, err := schemata.Prepare(context.Background(), mod, t.TempDir(), "", in, ownPackage, 2*time.Minute, 0, runs.run)
	if err != nil {
		t.Fatal(err)
	}
	checkAccounting(t, in, plan)
	if len(plan.Placed) != 1 || plan.Placed[0].Mutator != in[0] {
		for _, p := range plan.Placed {
			t.Errorf("%s at %s placed", p.Mutator.Type(), p.Mutator.Position())
		}
		t.Errorf("want Mul's mutant alone placed: Sub's position names another operator")
	}
	for _, n := range plan.Netted {
		if n.Mutator != in[1] || !strings.Contains(n.Reason, "line directive") {
			t.Errorf("%s at %s netted with %q, want only Sub's, the line directive named", n.Mutator.Type(), n.Mutator.Position(), n.Reason)
		}
	}
}

// TestPrepareRelativeRoot runs Prepare over a module whose root is relative,
// as gomodule.Init gives it for a relative target: go/packages reports
// absolute directories, which must still find the mutants' packages.
//
// It is not parallel: it changes the working directory.
func TestPrepareRelativeRoot(t *testing.T) {
	t.Chdir(filepath.Join("testdata", "prepare"))
	mod := gomodule.GoModule{Name: prepareMod, Root: ".", CallingDir: "."}
	in := streamMutants(t, mod)
	runs := &nullRuns{}
	plan, err := schemata.Prepare(context.Background(), mod, t.TempDir(), "", in, ownPackage, 2*time.Minute, 0, runs.run)
	if err != nil {
		t.Fatal(err)
	}
	checkAccounting(t, in, plan)
	if len(plan.Placed) == 0 {
		t.Error("nothing placed under a relative module root")
	}
	for _, n := range plan.Netted {
		if strings.Contains(n.Reason, "no package loaded") {
			t.Errorf("%s at %s netted: %s", n.Mutator.Type(), n.Mutator.Position(), n.Reason)
		}
	}
	for bin, dir := range runs.dirs {
		if !filepath.IsAbs(bin) || !filepath.IsAbs(dir) {
			t.Errorf("null run of %s in %s: want absolute paths", bin, dir)
		}
	}
}
