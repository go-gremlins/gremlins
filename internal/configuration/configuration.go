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

package configuration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// This is the list of the keys available in config files and as flags.
const (
	GremlinsSilentKey            = "silent"
	UnleashDryRunKey             = "unleash.dry-run"
	UnleashOutputKey             = "unleash.output"
	UnleashTagsKey               = "unleash.tags"
	UnleashCoverPkgKey           = "unleash.coverpkg"
	UnleashWorkersKey            = "unleash.workers"
	UnleashTestCPUKey            = "unleash.test-cpu"
	UnleashTimeoutCoefficientKey = "unleash.timeout-coefficient"
	UnleashIntegrationMode       = "unleash.integration"
	UnleashThresholdEfficacyKey  = "unleash.threshold.efficacy"
	UnleashThresholdMCoverageKey = "unleash.threshold.mutant-coverage"
)

const (
	gremlinsCfgName      = ".gremlins"
	gremlinsEnvVarPrefix = "GREMLINS"

	xdgConfigHomeKey = "XDG_CONFIG_HOME"

	windowsOs = "windows"
)

// Init initializes the viper configuration for Gremlins.
//
// It sets the configuration file name as .gremlins.yaml, adds the passed paths as ConfigPaths
// AutomaticEnv configuration having GREMLINS as prefix.
// The environment variables take precedence over the configuration file and must be set in the
// format:
//
//	GREMLINS_<COMMAND NAME>_<FLAG NAME>
func Init(cPaths []string) error {
	replacer := strings.NewReplacer(".", "_", "-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix(gremlinsEnvVarPrefix)
	viper.AutomaticEnv()
	viper.SetConfigName(gremlinsCfgName)
	viper.SetConfigType("yaml")

	if isSpecificFile(cPaths) {
		viper.SetConfigFile(cPaths[0])
		err := viper.ReadInConfig()
		if err != nil {
			return err
		}
	} else if arePathsNotSet(cPaths) {
		cPaths = defaultConfigPaths()
	}

	for _, p := range cPaths {
		viper.AddConfigPath(p)
	}

	_ = viper.ReadInConfig() // ignoring error if file not present

	return nil
}

// MutantTypeEnabledKey returns the configuration key for a mutant.
// The generated key will have the format 'mutants.mutant-name.enabled",
// which corresponds to the Yaml:
//
//		mutants:
//	 		mutant-name:
//	 			enabled: [bool]
func MutantTypeEnabledKey(mt mutator.Type) string {
	m := mt.String()
	m = strings.ReplaceAll(m, "_", "-")
	m = strings.ToLower(m)

	return fmt.Sprintf("mutants.%s.enabled", m)
}

func isSpecificFile(cPaths []string) bool {
	return len(cPaths) == 1 && filepath.Ext(cPaths[0]) != ""
}

func arePathsNotSet(cPaths []string) bool {
	return len(cPaths) == 0 || len(cPaths) == 1 && cPaths[0] == ""
}

func defaultConfigPaths() []string {
	result := make([]string, 0, 4)

	// First global config
	if runtime.GOOS != windowsOs {
		result = append(result, "/etc/gremlins")
	}

	// Then $XDG_CONFIG_HOME
	xchLocation, _ := homedir.Expand("~/.config")
	if x := os.Getenv(xdgConfigHomeKey); x != "" {
		xchLocation = x
	}
	xchLocation = filepath.Join(xchLocation, "gremlins", "gremlins")
	result = append(result, xchLocation)

	// Then $HOME
	homeLocation, err := homedir.Expand("~/.gremlins")
	if err != nil {
		return result
	}
	result = append(result, homeLocation)

	// Then the Go module root
	if root := findModuleRoot(); root != "" {
		result = append(result, root)
	}

	// Finally the current directory
	result = append(result, ".")

	return result
}

func findModuleRoot() string {
	// This function is duplicated from internal/gomodule. We should find a way
	// to use here gomodule. The problem is the point of initialization, because
	// configuration is initialised before gomodule.
	path, _ := os.Getwd()
	for {
		if fi, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && !fi.IsDir() {
			return path
		}
		d := filepath.Dir(path)
		if d == path {
			break
		}
		path = d
	}

	return ""
}

var mutex sync.RWMutex

// Set offers synchronised access to Viper.
func Set[T any](k string, v T) {
	mutex.Lock()
	defer mutex.Unlock()
	viper.Set(k, v)
}

// Get offers synchronised access to Viper.
func Get[T any](k string) T {
	var r T
	mutex.RLock()
	defer mutex.RUnlock()
	r, _ = viper.Get(k).(T)

	return r
}

// Reset is used mainly for testing purposes, in order to clean up the Viper
// instance.
func Reset() {
	mutex.Lock()
	defer mutex.Unlock()
	viper.Reset()
}
