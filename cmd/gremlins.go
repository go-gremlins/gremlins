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
	"errors"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/go-gremlins/gremlins/cmd/internal/flags"
	"github.com/go-gremlins/gremlins/configuration"
	"github.com/go-gremlins/gremlins/pkg/log"
)

const paramConfigFile = "config"

// Execute initialises a new Cobra root command (gremlins) with a custom version
// string used in the `-v` flag results.
func Execute(ctx context.Context, version string) error {
	rootCmd, err := newRootCmd(ctx, version)
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

func newRootCmd(ctx context.Context, version string) (*gremlinsCmd, error) {
	if version == "" {
		return nil, errors.New("expected a version string")
	}

	cmd := &cobra.Command{
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		Use:           "gremlins",
		Short:         shortExplainer(),
		Version:       version,
	}

	uc, err := newUnleashCmd(ctx)
	if err != nil {
		return nil, err

	}
	cmd.AddCommand(uc.cmd)

	flag := &flags.Flag{Name: "silent", CfgKey: configuration.GremlinsSilentKey, Shorthand: "s", DefaultV: false, Usage: "suppress output and run in silent mode"}
	if err := flags.SetPersistent(cmd, flag); err != nil {
		return nil, err
	}

	return &gremlinsCmd{
		cmd: cmd,
	}, nil
}

func shortExplainer() string {
	return heredoc.Doc(`
		Gremlins is a mutation testing tool for Go projects, made with love by k3rn31 
		and friends.
	`)
}
