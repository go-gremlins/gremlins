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
	"github.com/k3rn31/gremlins/coverage"
	"github.com/k3rn31/gremlins/log"
	"github.com/k3rn31/gremlins/mutator"
	"github.com/k3rn31/gremlins/mutator/workdir"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
)

type unleashCmd struct {
	cmd *cobra.Command
}

func newUnleashCmd() *unleashCmd {
	var dryRun bool
	var buildTags string

	cmd := &cobra.Command{
		Use:     "unleash [path of the Go module]",
		Aliases: []string{"run", "r"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Executes the mutation testing process",
		Long:    `Unleashes the gremlins and performs mutation testing on a Go module.`,
		Run: func(cmd *cobra.Command, args []string) {
			currentPath := "."
			if len(args) > 0 {
				currentPath = args[0]
			}

			workDir, err := ioutil.TempDir(os.TempDir(), "gremlins-")
			if err != nil {
				log.Errorf("impossible to create the workdir: %s", err)
				os.Exit(1)
			}
			defer func(n string) {
				err := os.RemoveAll(n)
				if err != nil {
					log.Errorf("impossible to remove temporary folder: %s\n\t%s", err, workDir)
				}
			}(workDir)

			c, err := coverage.New(workDir, currentPath, coverage.WithBuildTags(buildTags))
			if err != nil {
				log.Errorf("directory %s does not contain main module\n", currentPath)
				os.Exit(1)
			}

			p, err := c.Run()
			if err != nil {
				log.Errorln(err)
				os.Exit(1)
			}

			d := workdir.NewDealer(workDir, currentPath)
			mut := mutator.New(os.DirFS(currentPath), p, d,
				mutator.WithDryRun(dryRun),
				mutator.WithBuildTags(buildTags))
			results := mut.Run()

			// Temporary reporting
			var k int
			var l int
			var nc int
			for _, m := range results {
				if m.Status == mutator.Killed {
					k++
				}
				if m.Status == mutator.Lived {
					l++
				}
				if m.Status == mutator.NotCovered {
					nc++
				}
			}
			log.Infoln("-----")
			log.Infof("Killed: %d, Lived: %d, Not covered: %d\n", k, l, nc)
			log.Infof("Real coverage: %.2f%%\n", float64(k+l)/float64(k+l+nc)*100)
			log.Infof("Test efficacy: %.2f%%\n", float64(k)/float64(k+l)*100)
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "find mutations but do not executes tests")
	cmd.Flags().StringVarP(&buildTags, "tags", "t", "", "a comma-separated list of build tags")
	return &unleashCmd{
		cmd: cmd,
	}
}
