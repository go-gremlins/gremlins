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
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/pkg/coverage"
	"github.com/go-gremlins/gremlins/pkg/log"
	"github.com/go-gremlins/gremlins/pkg/mutator"
	"github.com/go-gremlins/gremlins/pkg/mutator/workdir"
	"github.com/go-gremlins/gremlins/pkg/report"
)

type unleashCmd struct {
	cmd *cobra.Command
}

const (
	commandName             = "unleash"
	paramBuildTags          = "tags"
	paramDryRun             = "dry-run"
	paramThresholdEfficacy  = "threshold-efficacy"
	paramThresholdMCoverage = "threshold-mcover"
)

func newUnleashCmd() (*unleashCmd, error) {
	cmd := &cobra.Command{
		Use:     fmt.Sprintf("%s [path]", commandName),
		Aliases: []string{"run", "r"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Executes the mutation testing process",
		Long:    `Unleashes the gremlins and performs mutation testing on a Go module.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Infoln("Starting...")
			runDir, err := os.Getwd()
			if err != nil {
				return err
			}

			currPath, err := currentPath(args)
			if err != nil {
				return err
			}

			workDir, err := ioutil.TempDir(os.TempDir(), "gremlins-")
			if err != nil {
				return fmt.Errorf("impossible to create the workdir: %w", err)
			}
			defer func(wd string, rd string) {
				_ = os.Chdir(rd)
				err := os.RemoveAll(wd)
				if err != nil {
					log.Errorf("impossible to remove temporary folder: %s\n\t%s", err, wd)
				}
			}(workDir, runDir)

			c, err := coverage.New(workDir, currPath)
			if err != nil {
				return fmt.Errorf("directory %q does not contain main module: %w", currPath, err)
			}

			p, err := c.Run()
			if err != nil {
				return err
			}

			d := workdir.NewDealer(workDir, currPath)
			mut := mutator.New(os.DirFS(currPath), p, d)
			results := mut.Run()

			err = report.Do(results)
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolP(paramDryRun, "d", false, "find mutations but do not executes tests")
	err := viper.BindPFlag(configuration.UnleashDryRunKey, cmd.Flags().Lookup(paramDryRun))
	if err != nil {
		return nil, err
	}

	cmd.Flags().StringP(paramBuildTags, "t", "", "a comma-separated list of build tags")
	err = viper.BindPFlag(configuration.UnleashTagsKey, cmd.Flags().Lookup(paramBuildTags))
	if err != nil {
		return nil, err
	}

	cmd.Flags().Float64(paramThresholdEfficacy, 0, "threshold for code-efficacy percent")
	err = viper.BindPFlag(configuration.UnleashThresholdEfficacyKey, cmd.Flags().Lookup(paramThresholdEfficacy))
	if err != nil {
		return nil, err
	}

	cmd.Flags().Float64(paramThresholdMCoverage, 0, "threshold for mutant-coverage percent")
	err = viper.BindPFlag(configuration.UnleashThresholdMCoverageKey, cmd.Flags().Lookup(paramThresholdMCoverage))
	if err != nil {
		return nil, err
	}

	return &unleashCmd{
		cmd: cmd,
	}, nil
}

func currentPath(args []string) (string, error) {
	cp := "."
	if len(args) > 0 {
		cp = args[0]
	}
	if cp != "." {
		err := os.Chdir(cp)
		if err != nil {
			return "", err
		}
		cp = "."
	}

	return cp, nil
}
