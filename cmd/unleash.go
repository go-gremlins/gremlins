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

	"github.com/go-gremlins/gremlins/pkg/coverage"
	"github.com/go-gremlins/gremlins/pkg/log"
	"github.com/go-gremlins/gremlins/pkg/mutator"
	"github.com/go-gremlins/gremlins/pkg/mutator/workdir"
	"github.com/go-gremlins/gremlins/pkg/report"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type unleashCmd struct {
	cmd *cobra.Command
}

const (
	CommandName    = "unleash"
	ParamDryRun    = "dry-run"
	ParamBuildTags = "tags"
)

func newUnleashCmd(v *viper.Viper) (*unleashCmd, error) {
	dryRun := v.GetBool(fmt.Sprintf("%s.%s", CommandName, ParamDryRun))
	buildTags := v.GetString(fmt.Sprintf("%s.%s", CommandName, ParamBuildTags))

	cmd := &cobra.Command{
		Use:     fmt.Sprintf("%s [path of the Go module]", CommandName),
		Aliases: []string{"run", "r"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Executes the mutation testing process",
		Long:    `Unleashes the gremlins and performs mutation testing on a Go module.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Infoln("Starting...")
			currentPath := "."
			if len(args) > 0 {
				currentPath = args[0]
			}
			if currentPath != "." {
				err := os.Chdir(currentPath)
				if err != nil {
					return err
				}
				currentPath = "."
			}

			workDir, err := ioutil.TempDir(os.TempDir(), "gremlins-")
			if err != nil {
				return fmt.Errorf("impossible to create the workdir: %w", err)
			}
			defer func(n string) {
				err := os.RemoveAll(n)
				if err != nil {
					log.Errorf("impossible to remove temporary folder: %s\n\t%s", err, workDir)
				}
			}(workDir)

			c, err := coverage.New(workDir, currentPath, coverage.WithBuildTags(buildTags))
			if err != nil {
				return fmt.Errorf("directory %q does not contain main module: %w", currentPath, err)
			}

			p, err := c.Run()
			if err != nil {
				return err
			}

			d := workdir.NewDealer(workDir, currentPath)
			mut := mutator.New(os.DirFS(currentPath), p, d,
				mutator.WithDryRun(dryRun),
				mutator.WithBuildTags(buildTags))
			results := mut.Run()

			report.Do(results)

			return nil
		},
	}

	cmd.Flags().BoolVarP(&dryRun, ParamDryRun, "d", false, "find mutations but do not executes tests")
	err := viper.BindPFlag(fmt.Sprintf("%s.%s", CommandName, ParamDryRun), cmd.Flags().Lookup(ParamDryRun))
	if err != nil {
		return nil, err
	}

	cmd.Flags().StringVarP(&buildTags, ParamBuildTags, "t", "", "a comma-separated list of build tags")
	err = viper.BindPFlag(fmt.Sprintf("%s.%s", CommandName, ParamBuildTags), cmd.Flags().Lookup(ParamBuildTags))
	if err != nil {
		return nil, err
	}

	return &unleashCmd{
		cmd: cmd,
	}, nil
}
