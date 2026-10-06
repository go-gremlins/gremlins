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

package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/tools/cover"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/deps"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/engine/workdir"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/report"
	"github.com/go-gremlins/gremlins/internal/schemata"
)

// TestSchemataParity runs the engine over testdata/parity twice, without and
// with --schemata, and expects every mutant to get the same status both ways,
// with every runnable mutant judged against the schema binaries. Both runs
// use --cross-package and --test-selection, as the CLI wires them, so that
// parity/other's test is the one that judges calc.Twice; the whole-suite
// path is TestSchemataParityRelativeTarget's.
//
// It is not parallel: it captures the global log to read the report, and
// building the test map changes the working directory.
func TestSchemataParity(t *testing.T) {
	modRoot, err := filepath.Abs("testdata/parity")
	if err != nil {
		t.Fatal(err)
	}
	mod := gomodule.GoModule{Name: "parity", Root: modRoot, CallingDir: "."}
	selection := map[string]any{
		configuration.UnleashCrossPackageKey:  true,
		configuration.UnleashTestSelectionKey: true,
	}
	prof, opts := paritySelection(t, mod, selection)

	legacy, legacyRes, legacyCalls := runParityWith(t, mod, prof, false, selection, opts...)
	if legacyCalls != 0 {
		t.Errorf("the run without --schemata called Prepare %d times", legacyCalls)
	}
	if legacyRes.Schemata != nil {
		t.Errorf("the run without --schemata reports a schemata summary %+v", *legacyRes.Schemata)
	}

	// The fixture is only evidence if the statuses it compares are varied.
	seen := map[mutator.Status]int{}
	for _, s := range legacy {
		seen[s]++
	}
	for _, s := range []mutator.Status{mutator.Killed, mutator.Lived, mutator.NotCovered, mutator.RunTimedOut} {
		if seen[s] == 0 {
			t.Errorf("the fixture produced no %s mutant without --schemata: %v", s, legacy)
		}
	}
	runnable := len(legacy) - seen[mutator.NotCovered] - seen[mutator.Skipped]
	checkSharedLine(t, modRoot, legacy)
	checkDuplicatedVerdicts(t, modRoot, legacy)
	checkEveryMutator(t, legacy)
	checkFixtureSites(t, modRoot, legacy)

	keep := filepath.Join(t.TempDir(), "kept")
	t.Setenv("GREMLINS_SCHEMATA_KEEP", keep)
	withSchemata, res, calls := runParityWith(t, mod, prof, true, selection, opts...)
	checkKept(t, keep, runnable)
	if calls != 1 {
		t.Errorf("the run with --schemata called Prepare %d times, want 1", calls)
	}
	if !maps.Equal(withSchemata, legacy) {
		for _, k := range slices.Sorted(maps.Keys(legacy)) {
			if withSchemata[k] != legacy[k] {
				t.Errorf("%s: %s with --schemata, %s without", k, withSchemata[k], legacy[k])
			}
		}
		if len(withSchemata) != len(legacy) {
			t.Errorf("%d mutants with --schemata, %d without", len(withSchemata), len(legacy))
		}
	}
	if res.Schemata == nil {
		t.Fatal("the run with --schemata has no schemata summary")
	}
	if want := (report.SchemataSummary{Placed: runnable, PerMutant: 0}); *res.Schemata != want {
		t.Errorf("schemata summary = %+v, want %+v", *res.Schemata, want)
	}

	out := &bytes.Buffer{}
	log.Init(out, &bytes.Buffer{})
	defer log.Reset()
	_ = report.Do(res)
	if want := fmt.Sprintf("Schemata: placed %d, per-mutant path 0\n", runnable); !strings.Contains(out.String(), want) {
		t.Errorf("report does not contain %q:\n%s", want, out.String())
	}
}

// TestSchemataSaysWhyItWasNotUsed runs --schemata where no schema build can
// be made, in a dry run and in integration mode, and requires the summary
// and the report line to say why placed is 0, not just print it.
//
// It is not parallel: it captures the global log to read the report.
func TestSchemataSaysWhyItWasNotUsed(t *testing.T) {
	modRoot, err := filepath.Abs("testdata/parity")
	if err != nil {
		t.Fatal(err)
	}
	mod := gomodule.GoModule{Name: "parity", Root: modRoot, CallingDir: "."}
	prof := parityProfile(t, mod)
	testCases := map[string]struct {
		extra   map[string]any
		wantWhy string
	}{
		"dry_run":          {extra: map[string]any{configuration.UnleashDryRunKey: true}, wantWhy: "dry run"},
		"integration_mode": {extra: map[string]any{configuration.UnleashIntegrationMode: true}, wantWhy: "integration mode"},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, res, calls := runParityWith(t, mod, prof, true, tc.extra)
			if calls != 0 {
				t.Errorf("Prepare was called %d times", calls)
			}
			if res.Schemata == nil || res.Schemata.Placed != 0 || res.Schemata.NotUsed != tc.wantWhy {
				t.Fatalf("schemata summary = %+v, want nothing placed and NotUsed %q", res.Schemata, tc.wantWhy)
			}
			out := &bytes.Buffer{}
			log.Init(out, &bytes.Buffer{})
			defer log.Reset()
			_ = report.Do(res)
			if want := fmt.Sprintf("Schemata: not used (%s), per-mutant path %d\n", tc.wantWhy, res.Schemata.PerMutant); !strings.Contains(out.String(), want) {
				t.Errorf("report does not contain %q:\n%s", want, out.String())
			}
		})
	}
}

// runParity runs the engine over mod and returns each mutant's status keyed by
// position and type, the results, and how many times Prepare was called.
func runParity(t *testing.T, mod gomodule.GoModule, prof coverage.Profile, withSchemata bool) (map[string]mutator.Status, report.Results, int) {
	t.Helper()

	return runParityWith(t, mod, prof, withSchemata, nil)
}

// runParityWith is runParity with extra settings and dealer options.
func runParityWith(t *testing.T, mod gomodule.GoModule, prof coverage.Profile, withSchemata bool, extra map[string]any,
	opts ...engine.ExecutorDealerOption,
) (map[string]mutator.Status, report.Results, int) {
	t.Helper()
	settings := map[string]any{
		configuration.UnleashSchemataKey:   withSchemata,
		configuration.UnleashTimeoutMaxKey: "1s",
	}
	maps.Copy(settings, extra)
	// Every mutator, all of which the schemata engine rewrites.
	for _, mt := range mutator.Types {
		settings[configuration.MutantTypeEnabledKey(mt)] = slices.Contains(schemaMutators, mt)
	}
	viperSet(settings)
	defer viperReset()

	wdd := workdir.NewCachedDealer(t.TempDir(), mod.Root)
	defer wdd.Clean()
	d := engine.NewExecutorDealer(mod, wdd, time.Second, opts...)
	var calls atomic.Int32
	prepare := func(ctx context.Context, m gomodule.GoModule, workDir, tags string, runnable []mutator.Mutator,
		testPkgs func(string) []string, allowance, buildTimeout time.Duration, nullRun schemata.NullRunFunc,
	) (schemata.Plan, error) {
		calls.Add(1)

		return schemata.Prepare(ctx, m, workDir, tags, runnable, testPkgs, allowance, buildTimeout, nullRun)
	}
	eng := engine.New(mod, engine.CodeData{Cov: prof}, d, engine.WithPrepare(prepare))
	res := eng.Run(context.Background())

	statuses := map[string]mutator.Status{}
	for _, m := range res.Mutants {
		p := m.Position()
		statuses[fmt.Sprintf("%s:%d:%d %s", p.Filename, p.Line, p.Column, m.Type())] = m.Status()
	}

	return statuses, res, int(calls.Load())
}

// parityProfile gathers the fixture's coverage the way gremlins does, without
// the chdir coverage.Run makes, which parallel tests cannot share.
func parityProfile(t *testing.T, mod gomodule.GoModule) coverage.Profile {
	t.Helper()
	out := filepath.Join(t.TempDir(), "coverage")
	cmd := exec.Command("go", "test", "-count=1", "-cover", "-coverprofile", out, "./...") //nolint:gosec // G204: test code running go on its own fixture
	cmd.Dir = mod.Root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("coverage: %v\n%s", err, b)
	}
	f, err := os.Open(out) //nolint:gosec // G304: test code reading the profile it wrote
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	profiles, err := cover.ParseProfilesFromReader(f)
	if err != nil {
		t.Fatal(err)
	}
	prof := coverage.Profile{}
	for _, p := range profiles {
		// Named as coverage names a profile's files: relative to the
		// directory Gremlins was pointed at, the frame mutant positions use.
		name, err := filepath.Rel(mod.CallingDir, strings.TrimPrefix(p.FileName, mod.Name+"/"))
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range p.Blocks {
			if b.Count == 0 {
				continue
			}
			prof[name] = append(prof[name], coverage.Block{StartLine: b.StartLine, StartCol: b.StartCol, EndLine: b.EndLine, EndCol: b.EndCol})
		}
	}

	return prof
}

// paritySelection gathers what cmd/unleash wires for --cross-package and
// --test-selection over the fixture: the dependency graph, the test map, and
// the coverage profile widened by the map's union, which is what makes a line
// only another package's test executes covered.
func paritySelection(t *testing.T, mod gomodule.GoModule, selection map[string]any) (coverage.Profile, []engine.ExecutorDealerOption) {
	t.Helper()
	prof := parityProfile(t, mod)
	inRoot := func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command(name, args...) //nolint:gosec // G204: test code running go on its own fixture
		cmd.Dir = mod.Root

		return cmd
	}
	// BuildTestMap changes into the module root; t.Chdir puts it back.
	t.Chdir(mod.Root)
	viperSet(selection)
	c := coverage.NewWithCmd(inRoot, t.TempDir(), mod, coverage.WithTestMapCacheDir(t.TempDir()))
	viperReset()
	graph, err := deps.New(inRoot, c.ScanPath())
	if err != nil {
		t.Fatal(err)
	}
	testMap, err := c.BuildTestMap()
	if err != nil {
		t.Fatal(err)
	}

	return coverage.Merge(prof, testMap.Union()), []engine.ExecutorDealerOption{engine.WithDependents(graph), engine.WithTestSelection(testMap)}
}

// checkEveryMutator requires the legacy run to have killed one mutant of
// every mutator and let another live: parity on a mutator is only evidence
// when its schema form is seen to change a verdict and to leave one alone.
func checkEveryMutator(t *testing.T, legacy map[string]mutator.Status) {
	t.Helper()
	for _, mt := range mutator.Types {
		got := map[mutator.Status]int{}
		for k, st := range legacy {
			if strings.HasSuffix(k, " "+mt.String()) {
				got[st]++
			}
		}
		if got[mutator.Killed] == 0 || got[mutator.Lived] == 0 {
			t.Errorf("%s mutants by status = %v, want at least one KILLED and one LIVED", mt, got)
		}
	}
}

// checkFixtureSites requires the legacy statuses the fixture's special sites
// exist for: the package-level constant NOT COVERED, the function-local
// constants (placed by duplicating their functions) and the line only
// parity/other's test executes KILLED.
func checkFixtureSites(t *testing.T, modRoot string, legacy map[string]mutator.Status) {
	t.Helper()
	testCases := map[string]struct {
		file, prefix string
		want         mutator.Status
	}{
		"package_const":      {file: "consts.go", prefix: "const Limit", want: mutator.NotCovered},
		"local_const":        {file: "consts.go", prefix: "\tconst k", want: mutator.Killed},
		"array_length_const": {file: "consts.go", prefix: "\tconst n", want: mutator.Killed},
		"cross_package":      {file: "calc.go", prefix: "func Twice(", want: mutator.Killed},
	}
	for name, tc := range testCases {
		line := fixtureLine(t, modRoot, tc.file, tc.prefix)
		got := map[mutator.Status]int{}
		for k, st := range legacy {
			if strings.HasPrefix(k, fmt.Sprintf("calc/%s:%d:", tc.file, line)) {
				got[st]++
			}
		}
		if got[tc.want] == 0 || len(got) != 1 {
			t.Errorf("%s (%s:%d) mutants by status = %v, want all %s", name, tc.file, line, got, tc.want)
		}
	}
}

// fixtureLine is the line of calc/file that starts with prefix, 0 if none.
func fixtureLine(t *testing.T, modRoot, file, prefix string) int {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(modRoot, "calc", file)) //nolint:gosec // G304: the fixture
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(l, prefix) {
			return i + 1
		}
	}

	return 0
}

// checkSharedLine requires the fixture's Mix line to hold two operators whose
// mutants the legacy run judges differently, one KILLED and one LIVED: the
// parity on it then shows that each schema id switches its own operator and
// not its neighbour on the line.
func checkSharedLine(t *testing.T, modRoot string, legacy map[string]mutator.Status) {
	t.Helper()
	line := fixtureLine(t, modRoot, "calc.go", "func Mix(")
	got := map[mutator.Status]int{}
	for k, st := range legacy {
		if strings.Contains(k, fmt.Sprintf("calc.go:%d:", line)) {
			got[st]++
		}
	}
	if line == 0 || got[mutator.Killed] != 1 || got[mutator.Lived] != 1 || len(got) != 2 {
		t.Errorf("Mix (line %d) mutants by status = %v, want one KILLED and one LIVED", line, got)
	}
}

// checkDuplicatedVerdicts requires the fixture's Width constant -- an array
// length that Width also uses at run time -- to have KILLED and LIVED
// mutants, and no other status, without --schemata. Both are placed by duplicating Width, so
// the parity on it carries function duplication through both verdicts: the
// LIVED one shows that the copy, entered under its own id only, changes
// nothing a test does not observe.
func checkDuplicatedVerdicts(t *testing.T, modRoot string, legacy map[string]mutator.Status) {
	t.Helper()
	line := fixtureLine(t, modRoot, "consts.go", "\tconst w")
	got := map[mutator.Status][]string{}
	for k, st := range legacy {
		if strings.HasPrefix(k, fmt.Sprintf("calc/consts.go:%d:", line)) {
			got[st] = append(got[st], k)
		}
	}
	if line == 0 || len(got[mutator.Killed]) == 0 || len(got[mutator.Lived]) == 0 || len(got) != 2 {
		t.Errorf("Width's constant (consts.go:%d) mutants by status = %v, want KILLED and LIVED ones only", line, got)
	}
}

// checkKept requires the build GREMLINS_SCHEMATA_KEEP kept in dir to index
// runnable mutants, each with its id, position, type and package, and to hold
// the binaries of calc and other and the rewritten source the index names,
// with copies of the functions whose local constants are mutated.
func checkKept(t *testing.T, dir string, runnable int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "index.json")) //nolint:gosec // G304: the kept index
	if err != nil {
		t.Fatalf("no kept index: %v", err)
	}
	var idx struct {
		Binaries map[string]string
		Source   string
		Mutants  []struct {
			ID                      int
			Position, Type, Package string
		}
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Mutants) != runnable {
		t.Errorf("kept index has %d mutants, want %d", len(idx.Mutants), runnable)
	}
	for _, m := range idx.Mutants {
		if m.ID < 1 || !strings.HasPrefix(m.Position, "calc/") || m.Type == "" || m.Package != "parity/calc" {
			t.Errorf("kept index entry %+v is incomplete", m)
		}
	}
	bin, ok := idx.Binaries["parity/calc"]
	if !ok || !strings.HasPrefix(bin, dir) {
		t.Fatalf("kept binaries %v, want parity/calc's under %s", idx.Binaries, dir)
	}
	if fi, err := os.Stat(bin); err != nil || fi.Mode()&0o100 == 0 {
		t.Errorf("kept binary %s: %v, mode %v", bin, err, fi)
	}
	// calc.Twice's mutant selects parity/other alone, through --cross-package.
	if _, ok := idx.Binaries["parity/other"]; !ok {
		t.Errorf("kept binaries %v lack parity/other's", idx.Binaries)
	}
	if _, err := os.Stat(filepath.Join(idx.Source, "calc", "calc.go")); err != nil {
		t.Errorf("kept source lacks the rewritten calc.go: %v", err)
	}
	// The function-local constants are placed by copies of their functions.
	consts, err := os.ReadFile(filepath.Join(idx.Source, "calc", "consts.go"))
	if err != nil {
		t.Fatalf("kept source lacks the rewritten consts.go: %v", err)
	}
	for _, fn := range []string{"Scaled", "Slots"} {
		if !regexp.MustCompile(`func \w+_D\d+_` + fn + `\(`).Match(consts) {
			t.Errorf("the rewritten consts.go has no copy of %s:\n%s", fn, consts)
		}
	}
}

// TestSchemataParityRelativeTarget runs the parity fixture the way the CLI
// does from the module root, `gremlins unleash ./calc`: gomodule.Init of a
// relative target gives a relative module root, and every placed mutant must
// still run against the schema binaries with the statuses of the legacy run.
//
// It is not parallel: it changes the working directory.
func TestSchemataParityRelativeTarget(t *testing.T) {
	t.Chdir("testdata/parity")
	mod, err := gomodule.Init("./calc")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(mod.Root) {
		t.Fatalf("gomodule.Init(./calc) gave the absolute root %s: the test proves nothing", mod.Root)
	}
	prof := parityProfile(t, mod)

	legacy, _, _ := runParity(t, mod, prof, false)
	runnable := 0
	for _, s := range legacy {
		if s != mutator.NotCovered && s != mutator.Skipped {
			runnable++
		}
	}
	if runnable == 0 {
		t.Fatalf("no runnable mutant from a relative target: %v", legacy)
	}
	withSchemata, res, _ := runParity(t, mod, prof, true)
	for _, k := range slices.Sorted(maps.Keys(legacy)) {
		if withSchemata[k] != legacy[k] {
			t.Errorf("%s: %s with --schemata, %s without", k, withSchemata[k], legacy[k])
		}
	}
	if res.Schemata == nil {
		t.Fatal("the run with --schemata has no schemata summary")
	}
	if want := (report.SchemataSummary{Placed: runnable, PerMutant: 0}); *res.Schemata != want {
		t.Errorf("schemata summary = %+v, want %+v", *res.Schemata, want)
	}
}

// TestSchemataBuildTimeoutReachesPrepare runs --schemata with and without
// --schemata-build-timeout and requires Prepare handed the configured bound,
// zero (derive it) when it is unset or invalid, and the per-mutant compile
// allowance unchanged beside it.
//
// It is not parallel: it sets the process-wide configuration.
func TestSchemataBuildTimeoutReachesPrepare(t *testing.T) {
	modRoot, err := filepath.Abs("testdata/parity")
	if err != nil {
		t.Fatal(err)
	}
	mod := gomodule.GoModule{Name: "parity", Root: modRoot, CallingDir: "."}
	prof := parityProfile(t, mod)
	testCases := map[string]struct {
		settings                 map[string]any
		wantAllowance, wantBuild time.Duration
	}{
		"unset":     {settings: map[string]any{}, wantAllowance: engine.DefaultCompileAllowance},
		"set":       {settings: map[string]any{configuration.UnleashSchemataBuildTimeoutKey: "7m"}, wantAllowance: engine.DefaultCompileAllowance, wantBuild: 7 * time.Minute},
		"malformed": {settings: map[string]any{configuration.UnleashSchemataBuildTimeoutKey: "soon"}, wantAllowance: engine.DefaultCompileAllowance},
		"with_compile_allowance": {
			settings:      map[string]any{configuration.UnleashCompileAllowanceKey: "3m", configuration.UnleashSchemataBuildTimeoutKey: "1h"},
			wantAllowance: 3 * time.Minute, wantBuild: time.Hour,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			settings := map[string]any{
				configuration.UnleashSchemataKey:   true,
				configuration.UnleashTimeoutMaxKey: "1s",
			}
			maps.Copy(settings, tc.settings)
			viperSet(settings)
			defer viperReset()
			wdd := workdir.NewCachedDealer(t.TempDir(), mod.Root)
			defer wdd.Clean()
			d := engine.NewExecutorDealer(mod, wdd, time.Second)
			var gotAllowance, gotBuild time.Duration
			calls := 0
			prepare := func(ctx context.Context, m gomodule.GoModule, workDir, tags string, runnable []mutator.Mutator,
				testPkgs func(string) []string, allowance, buildTimeout time.Duration, nullRun schemata.NullRunFunc,
			) (schemata.Plan, error) {
				calls++
				gotAllowance, gotBuild = allowance, buildTimeout

				return schemata.Prepare(ctx, m, workDir, tags, runnable, testPkgs, allowance, buildTimeout, nullRun)
			}
			eng := engine.New(mod, engine.CodeData{Cov: prof}, d, engine.WithPrepare(prepare))
			eng.Run(context.Background())
			if calls != 1 {
				t.Fatalf("Prepare called %d times, want once", calls)
			}
			if gotAllowance != tc.wantAllowance || gotBuild != tc.wantBuild {
				t.Errorf("Prepare got allowance %s, build timeout %s; want %s, %s", gotAllowance, gotBuild, tc.wantAllowance, tc.wantBuild)
			}
		})
	}
}
