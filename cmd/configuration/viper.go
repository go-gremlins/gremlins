package configuration

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

import (
	"strings"

	"github.com/spf13/viper"
)

const (
	gremlinsCfgName      = ".gremlins"
	gremlinsEnvVarPrefix = "GREMLINS"
)

// GetViper gets the viper configuration for Gremlins
//
// It sets the configuration file name as .gremlins.yaml, adds the passed paths as ConfigPaths
// AutomaticEnv configuration having GREMLINS as prefix.
// The environment variables take precedence over the configuration file and must be set in the
// format:
//   GREMLINS_<COMMAND NAME>_<FLAG NAME>
func GetViper(configPaths []string) *viper.Viper {
	// setting viper
	v := viper.New()
	v.SetConfigName(gremlinsCfgName)
	v.SetConfigType("yaml")

	for _, p := range configPaths {
		v.AddConfigPath(p)
	}

	v.SetEnvPrefix(gremlinsEnvVarPrefix)
	replacer := strings.NewReplacer(".", "_", "-", "_")
	v.SetEnvKeyReplacer(replacer)
	v.AutomaticEnv()

	_ = v.ReadInConfig() // ignoring error if file not present

	return v

}
