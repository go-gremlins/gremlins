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
	"github.com/k3rn31/gremlins/coverage"
	"github.com/k3rn31/gremlins/mutator"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
)

type unleashCmd struct {
	cmd *cobra.Command
}

func newUnleashCmd() *unleashCmd {
	cmd := &cobra.Command{
		Use:     "unleash [path of the Go module]",
		Aliases: []string{"run", "r"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Executes the mutation testing process",
		Long:    `Unleashes the gremlins and performs mutation testing on a Go module.`,
		Run: func(cmd *cobra.Command, args []string) {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			tmpdir, _ := ioutil.TempDir(os.TempDir(), "unleash-")
			defer func(name string) {
				_ = os.Remove(name)
			}(tmpdir)
			cov, err := coverage.New(tmpdir, path)
			if err != nil {
				fmt.Printf("directory %s does not contain main module\n", path)
				os.Exit(1)
			}
			pro, err := cov.Run()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			mut := mutator.New(os.DirFS(path), pro)
			rec := mut.Run()

			// Temporary report
			for _, r := range rec {
				fmt.Printf("found possible mutant at %s - %s\n", r.Position, r.Status)
			}
		},
	}

	return &unleashCmd{
		cmd: cmd,
	}
}
