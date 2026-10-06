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

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/go-gremlins/gremlins/cmd/internal/flags"
	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/coverage"
	"github.com/go-gremlins/gremlins/internal/deps"
	"github.com/go-gremlins/gremlins/internal/diff"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/engine/workdir"
	"github.com/go-gremlins/gremlins/internal/exclusion"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/report"
)

type unleashCmd struct {
	cmd *cobra.Command
}

const (
	commandName = "unleash"

	paramDiff               = "diff"
	paramBuildTags          = "tags"
	paramCoverPackages      = "coverpkg"
	paramDryRun             = "dry-run"
	paramOutputStatuses     = "output-statuses"
	paramOutputDiffStatuses = "output-diff-statuses"
	paramOutput             = "output"
	paramIntegrationMode    = "integration"
	paramExcludeFiles       = "exclude-files"
	paramTestCPU            = "test-cpu"
	paramWorkers            = "workers"
	paramTimeoutCoefficient = "timeout-coefficient"
	paramTestSelection      = "test-selection"
	paramCrossPackage       = "cross-package"

	// Thresholds.
	paramThresholdEfficacy  = "threshold-efficacy"
	paramThresholdMCoverage = "threshold-mcover"
)

func newUnleashCmd(ctx context.Context) (*unleashCmd, error) {
	cmd := &cobra.Command{
		Use:     fmt.Sprintf("%s [path]", commandName),
		Aliases: []string{"run", "r"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Unleash the gremlins",
		Long:    longExplainer(),
		RunE:    runUnleash(ctx),
	}

	if err := setFlagsOnCmd(cmd); err != nil {
		return nil, err
	}

	return &unleashCmd{cmd: cmd}, nil
}

func longExplainer() string {
	return heredoc.Doc(`
		Unleashes the gremlins and performs mutation testing on a Go module. It works by
		first gathering the coverage of the test suite and then analysing the source
		code to look for supported mutants.

		Unleash only tests covered mutants, since it doesn't make sense to test mutants 
		that no test case is able to catch.

		In 'dry-run' mode, unleash only performs the analysis of the source code, but it
		doesn't actually perform the test.

		Thresholds are configurable quality gates that make gremlins exit with an error 
		if those values are not met. Efficacy is the percent of KILLED mutants over
		the total KILLED and LIVED mutants. Mutant coverage is the percent of total
		KILLED + LIVED mutants, over the total mutants.
	`)
}

func runUnleash(ctx context.Context) func(cmd *cobra.Command, args []string) error {
	return func(_ *cobra.Command, args []string) error {
		log.Infoln("Starting...")
		path, _ := os.Getwd()
		if len(args) > 0 {
			path = args[0]
		}
		mod, err := gomodule.Init(path)
		if err != nil {
			return fmt.Errorf("not in a Go module: %w", err)
		}

		workDir, err := os.MkdirTemp(os.TempDir(), "gremlins-")
		if err != nil {
			return fmt.Errorf("impossible to create the workdir: %w", err)
		}
		defer cleanUp(workDir)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		cancelled := false
		var results report.Results
		go runWithCancel(ctx, wg, func(c context.Context) {
			results, err = run(c, mod, workDir)
		}, func() {
			cancelled = true
		})
		wg.Wait()
		if err != nil {
			return err
		}
		if cancelled {
			return nil
		}

		return report.Do(results)
	}
}

func runWithCancel(ctx context.Context, wg *sync.WaitGroup, runner func(c context.Context), onCancel func()) {
	c, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		log.Infof("\nShutting down gracefully...\n")
		cancel()
		onCancel()
	}()
	runner(c)
	wg.Done()
}

func cleanUp(wd string) {
	if err := os.RemoveAll(wd); err != nil {
		log.Errorf("impossible to remove temporary folder: %s\n\t%s", err, wd)
	}
}

func run(ctx context.Context, mod gomodule.GoModule, workDir string) (report.Results, error) {
	fDiff, err := diff.New()
	if err != nil {
		return report.Results{}, err
	}

	c := coverage.New(workDir, mod)

	exclude, err := exclusion.New()
	if err != nil {
		return report.Results{}, err
	}

	var opts []engine.ExecutorDealerOption

	// Which packages a mutation could break is a question about imports, so it
	// costs one `go list` and no test runs at all.
	var graph *deps.Graph
	if configuration.Get[bool](configuration.UnleashCrossPackageKey) {
		graph, err = deps.New(exec.Command, c.ScanPath())
		if err != nil {
			return report.Results{}, fmt.Errorf("failed to resolve the module's dependents: %w", err)
		}
		opts = append(opts, engine.WithDependents(graph))
	}

	// Which tests within those packages execute the mutated line is a question
	// about coverage, and that is the expensive one. With selection the map
	// that answers it is built inside Gather, and stands in for the coverage
	// gather wherever it can. The map scope is set first: a package outside it
	// holds no mutant in this run, so it is neither mapped nor gathered.
	selection := testSelectionRequested()
	if selection {
		if scope := mapScope(mod, fDiff, graph.Dependents); scope != nil {
			coverage.WithMapScope(scope)(c)
		}
	}
	cProfile, testMap, err := c.Gather(selection)
	if err != nil {
		return report.Results{}, err
	}

	if selection {
		opts = append(opts, engine.WithTestSelection(testMap))
	}

	wdDealer := workdir.NewCachedDealer(workDir, mod.Root)
	defer wdDealer.Clean()

	jDealer := engine.NewExecutorDealer(mod, wdDealer, cProfile.Elapsed, opts...)

	codeData := engine.CodeData{
		Cov:       cProfile.Profile,
		Diff:      fDiff,
		Exclusion: exclude,
	}

	mut := engine.New(mod, codeData, jDealer)
	results := mut.Run(ctx)

	return results, nil
}

// mapScope is the test map's scope for a --diff run: the packages of the
// changed production files, which are the only ones that hold mutants, and,
// through dependents, the packages whose tests --cross-package selects for
// them. Files under testdata or vendor are never mutated, so they bring no
// package in. Without a diff every package can hold a mutant: no scope. The
// diff's paths are relative to the calling directory, as mutant positions are,
// and a file's import path is its directory under the module, whatever its
// package clause says.
func mapScope(mod gomodule.GoModule, changes diff.Diff, dependents func(string) []string) []string {
	if len(changes) == 0 {
		return nil
	}
	in := map[string]bool{}
	for file := range changes {
		name := filepath.ToSlash(string(file))
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || underIgnoredDir(name) {
			continue
		}
		pkg := mod.Name
		if dir := filepath.ToSlash(filepath.Dir(filepath.Join(mod.CallingDir, name))); dir != "." {
			pkg += "/" + dir
		}
		in[pkg] = true
		for _, dep := range dependents(pkg) {
			in[dep] = true
		}
	}
	scope := make([]string, 0, len(in))
	for pkg := range in {
		scope = append(scope, pkg)
	}
	sort.Strings(scope)

	return scope
}

// underIgnoredDir reports whether a slash-separated path runs through a
// testdata or vendor directory, which the engine never mutates.
func underIgnoredDir(name string) bool {
	for _, part := range strings.Split(path.Dir(name), "/") {
		if part == "testdata" || part == "vendor" {
			return true
		}
	}

	return false
}

// testSelectionRequested reports whether to build the test map.
//
// Integration mode runs the whole module for every mutant by design, so there is
// nothing for selection to narrow and the map would be paid for nothing.
func testSelectionRequested() bool {
	if !configuration.Get[bool](configuration.UnleashTestSelectionKey) {
		return false
	}
	if configuration.Get[bool](configuration.UnleashIntegrationMode) {
		log.Infoln("test-selection has no effect in integration mode: every mutant runs the whole module")

		return false
	}

	return true
}

func setFlagsOnCmd(cmd *cobra.Command) error {
	cmd.Flags().SortFlags = false
	cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		from := []string{".", "_"}
		to := "-"
		for _, sep := range from {
			name = strings.ReplaceAll(name, sep, to)
		}

		return pflag.NormalizedName(name)
	})

	fls := []*flags.Flag{
		{Name: paramDryRun, CfgKey: configuration.UnleashDryRunKey, Shorthand: "d", DefaultV: false, Usage: "find mutations but do not executes tests"},
		{Name: paramOutputStatuses, CfgKey: configuration.UnleashOutputStatusesKey, Shorthand: "S", DefaultV: "", Usage: "print only statuses from this flag, allowed values - 'lctkvsr'"},
		{Name: paramOutputDiffStatuses, CfgKey: configuration.UnleashOutputDiffStatusesKey, DefaultV: "", Usage: "print diff for mutants with these statuses, allowed values - 'lk'"},
		{Name: paramBuildTags, CfgKey: configuration.UnleashTagsKey, Shorthand: "t", DefaultV: "", Usage: "a comma-separated list of build tags"},
		{Name: paramCoverPackages, CfgKey: configuration.UnleashCoverPkgKey, DefaultV: "", Usage: "a comma-separated list of package patterns"},
		{Name: paramDiff, CfgKey: configuration.UnleashDiffRef, Shorthand: "D", DefaultV: "", Usage: "diff branch or commit"},
		{Name: paramOutput, CfgKey: configuration.UnleashOutputKey, Shorthand: "o", DefaultV: "", Usage: "set the output file for machine readable results"},
		{Name: paramIntegrationMode, CfgKey: configuration.UnleashIntegrationMode, Shorthand: "i", DefaultV: false, Usage: "makes Gremlins run the complete test suite for each mutation"},
		{Name: paramExcludeFiles, CfgKey: configuration.UnleashExcludeFiles, Shorthand: "E", DefaultV: []string{}, Usage: "exclude files from Gremlins run by filepath regexp"},
		{Name: paramThresholdEfficacy, CfgKey: configuration.UnleashThresholdEfficacyKey, DefaultV: float64(0), Usage: "threshold for code-efficacy percent"},
		{Name: paramThresholdMCoverage, CfgKey: configuration.UnleashThresholdMCoverageKey, DefaultV: float64(0), Usage: "threshold for mutant-coverage percent"},
		{Name: paramWorkers, CfgKey: configuration.UnleashWorkersKey, DefaultV: 0, Usage: "the number of workers to use in mutation testing"},
		{Name: paramTestCPU, CfgKey: configuration.UnleashTestCPUKey, DefaultV: 0, Usage: "the number of CPUs to allow each test run to use"},
		{Name: paramTimeoutCoefficient, CfgKey: configuration.UnleashTimeoutCoefficientKey, DefaultV: 0, Usage: "the coefficient by which the timeout is increased"},
		{Name: paramTestSelection, CfgKey: configuration.UnleashTestSelectionKey, DefaultV: false, Usage: "run only the tests of the mutated package that execute the mutated line"},
		{Name: paramCrossPackage, CfgKey: configuration.UnleashCrossPackageKey, DefaultV: false, Usage: "also test the mutated package's dependents, so a mutation is judged by what it could break"},
	}

	for _, f := range fls {
		err := flags.Set(cmd, f)
		if err != nil {
			return err
		}
	}

	return setMutantTypeFlags(cmd)
}

func setMutantTypeFlags(cmd *cobra.Command) error {
	for _, mt := range mutator.Types {
		name := mt.String()
		usage := fmt.Sprintf("enable %q mutants", name)
		param := strings.ReplaceAll(name, "_", "-")
		param = strings.ToLower(param)
		confKey := configuration.MutantTypeEnabledKey(mt)

		err := flags.Set(cmd, &flags.Flag{
			Name:     param,
			CfgKey:   confKey,
			DefaultV: configuration.IsDefaultEnabled(mt),
			Usage:    usage,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
