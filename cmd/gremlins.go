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
	"os"

	"github.com/spf13/cobra"

	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/pkg/log"
)

const paramConfigFile = "config"

// Execute initialises a new Cobra root command (gremlins) with a custom version
// string used in the `-v` flag results.
func Execute(version string) error {
	rootCmd, err := newRootCmd(version)
	if err != nil {
		return err
	}

	return rootCmd.execute()
}

type gremlinsCmd struct {
	cmd *cobra.Command
}

func (gc gremlinsCmd) execute() error {
	var cfgFile string
	cobra.OnInitialize(func() {
		err := configuration.Init([]string{cfgFile})
		if err != nil {
			log.Errorf("initialization error: %s\n", err)
			os.Exit(1)
		}
	})
	gc.cmd.PersistentFlags().StringVar(&cfgFile, paramConfigFile, "", "override config file")

	return gc.cmd.Execute()
}

func newRootCmd(version string) (*gremlinsCmd, error) {
	cmd := &cobra.Command{
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		Use:           "gremlins <command> [arguments]",
		Short: `Gremlins is a mutation testing tool for Go projects, made with love by go-gremlins 
and friends.
`,
		Version: version,
	}

	uc, err := newUnleashCmd()
	if err != nil {
		return nil, err

	}
	cmd.AddCommand(uc.cmd)

	return &gremlinsCmd{
		cmd: cmd,
	}, nil
}
