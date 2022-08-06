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

package flags

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Flag is the internal representation of a command flag. It is used to set
// flags in a more generic way.
type Flag struct {
	Name      string
	CfgKey    string
	Shorthand string
	DefaultV  any
	Usage     string
}

// Set is a "generic" function used to set flags on cobra.Command and bind
// them to viper.Viper.
func Set(cmd *cobra.Command, flag *Flag) error {
	flagSet := cmd.Flags()

	return setFlags(flag, flagSet)
}

// SetPersistent is a "generic" function used to set persistent flags
// on cobra.Command and bind them to viper.Viper.
func SetPersistent(cmd *cobra.Command, flag *Flag) error {
	flagSet := cmd.PersistentFlags()

	return setFlags(flag, flagSet)
}

func setFlags(flag *Flag, fs *pflag.FlagSet) error {
	switch dv := flag.DefaultV.(type) {
	// TODO: add a case for all the supported types
	case bool:
		setBool(flag, fs, dv)
	case string:
		setString(flag, fs, dv)
	case float64:
		setFloat64(flag, fs, dv)
	}
	err := viper.BindPFlag(flag.CfgKey, fs.Lookup(flag.Name))
	if err != nil {
		return err
	}

	return nil
}

func setFloat64(flag *Flag, flags *pflag.FlagSet, dv float64) {
	if flag.Shorthand != "" {
		flags.Float64P(flag.Name, flag.Shorthand, dv, flag.Usage)
	} else {
		flags.Float64(flag.Name, dv, flag.Usage)
	}
}

func setString(flag *Flag, flags *pflag.FlagSet, dv string) {
	if flag.Shorthand != "" {
		flags.StringP(flag.Name, flag.Shorthand, dv, flag.Usage)
	} else {
		flags.String(flag.Name, dv, flag.Usage)
	}
}

func setBool(flag *Flag, flags *pflag.FlagSet, dv bool) {
	if flag.Shorthand != "" {
		flags.BoolP(flag.Name, flag.Shorthand, dv, flag.Usage)
	} else {
		flags.Bool(flag.Name, dv, flag.Usage)
	}
}
