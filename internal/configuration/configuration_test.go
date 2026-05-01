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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

func TestGetFromBoundPFlag(t *testing.T) {
	testCases := []struct {
		register func(fs *pflag.FlagSet, name string)
		check    func(t *testing.T, cfgKey string)
		name     string
		flagName string
		cfgKey   string
		args     []string
	}{
		{
			name:     "float64",
			flagName: "efficacy",
			cfgKey:   "unleash.threshold.efficacy",
			args:     []string{"--efficacy", "50"},
			register: func(fs *pflag.FlagSet, name string) { fs.Float64(name, 0, "") },
			check: func(t *testing.T, cfgKey string) {
				t.Helper()
				if got := Get[float64](cfgKey); got != 50.0 {
					t.Errorf("Get[float64](%q) = %v, want 50.0", cfgKey, got)
				}
			},
		},
		{
			name:     "int",
			flagName: "workers",
			cfgKey:   "unleash.workers",
			args:     []string{"--workers", "4"},
			register: func(fs *pflag.FlagSet, name string) { fs.Int(name, 0, "") },
			check: func(t *testing.T, cfgKey string) {
				t.Helper()
				if got := Get[int](cfgKey); got != 4 {
					t.Errorf("Get[int](%q) = %v, want 4", cfgKey, got)
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			tc.register(fs, tc.flagName)
			if err := viper.BindPFlag(tc.cfgKey, fs.Lookup(tc.flagName)); err != nil {
				t.Fatal("BindPFlag should not fail")
			}
			if err := fs.Parse(tc.args); err != nil {
				t.Fatal("Parse should not fail")
			}

			tc.check(t, tc.cfgKey)
			viper.Reset()
		})
	}
}

type envEntry struct {
	name  string
	value string
}

func TestConfiguration(t *testing.T) {
	testCases := []struct {
		wantedConfig map[string]interface{}
		name         string
		configPaths  []string
		envEntries   []envEntry
		expectErr    bool
	}{
		{
			name:        "from single file",
			configPaths: []string{"testdata/config1/.gremlins.yaml"},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": true,
				"unleash.tags":    "tag1,tag2,tag3",
			},
		},
		{
			name:        "from returns error if unreadable",
			configPaths: []string{"testdata/config1/.gremlin"},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": true,
				"unleash.tags":    "tag1,tag2,tag3",
			},
			expectErr: true,
		},
		{
			name:        "from cfg",
			configPaths: []string{"./testdata/config1"},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": true,
				"unleash.tags":    "tag1,tag2,tag3",
			},
		},
		{
			name:        "from cfg multi",
			configPaths: []string{"./testdata/config2", "./testdata/config1"},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": true,
				"unleash.tags":    "tag1.2,tag2.2,tag3.2",
			},
		},
		{
			name: "from env",
			envEntries: []envEntry{
				{name: "GREMLINS_UNLEASH_DRY_RUN", value: "true"},
				{name: "GREMLINS_UNLEASH_TAGS", value: "tag1,tag2,tag3"},
			},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": "true",
				"unleash.tags":    "tag1,tag2,tag3",
			},
		},
		{
			name: "from cfg override with env",
			envEntries: []envEntry{
				{name: "GREMLINS_UNLEASH_TAGS", value: "tag1env,tag2env,tag3env"},
			},
			configPaths: []string{"./testdata/config1"},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": true,
				"unleash.tags":    "tag1env,tag2env,tag3env",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.envEntries != nil {
				for _, e := range tc.envEntries {
					t.Setenv(e.name, e.value)
				}
			}
			err := Init(tc.configPaths)
			if tc.expectErr && err == nil {
				t.Fatal("expected error")
			}
			if tc.expectErr {
				return
			}

			for key, wanted := range tc.wantedConfig {
				got := Get[any](key)
				if got != wanted {
					t.Error(cmp.Diff(got, wanted))
				}
			}
			viper.Reset()
		})
	}
}

func TestConfigPaths(t *testing.T) {
	home, _ := homedir.Dir()

	t.Run("it lookups in default locations", func(t *testing.T) {
		oldDir, _ := os.Getwd()
		_ = os.Chdir("testdata/config1")
		defer func(dir string) {
			_ = os.Chdir(dir)
		}(oldDir)

		var want []string

		// First global
		if runtime.GOOS != windowsOs {
			want = append(want, "/etc/gremlins")
		}

		// Then $XDG_CONFIG_HOME and $HOME
		want = append(want,
			filepath.Join(home, ".config", "gremlins", "gremlins"),
			filepath.Join(home, ".gremlins"),
		)

		// Then module root, then current folder
		moduleRoot, _ := os.Getwd()
		want = append(want, moduleRoot, ".")

		got := defaultConfigPaths()

		if !cmp.Equal(got, want) {
			t.Error(cmp.Diff(got, want))
		}
	})

	t.Run("no module root if not in go module", func(t *testing.T) {
		oldDir, _ := os.Getwd()
		_ = os.Chdir(t.TempDir())
		defer func(dir string) {
			_ = os.Chdir(dir)
		}(oldDir)

		var want []string

		// First global
		if runtime.GOOS != windowsOs {
			want = append(want, "/etc/gremlins")
		}

		// Then $XDG_CONFIG_HOME and $HOME, then current folder
		want = append(want,
			filepath.Join(home, ".config", "gremlins", "gremlins"),
			filepath.Join(home, ".gremlins"),
			".",
		)

		got := defaultConfigPaths()

		if !cmp.Equal(got, want) {
			t.Error(cmp.Diff(got, want))
		}
	})

	t.Run("when XDG_CONFIG_HOME is set, it lookups in that locations", func(t *testing.T) {
		oldDir, _ := os.Getwd()
		_ = os.Chdir("testdata/config1")
		defer func(dir string) {
			_ = os.Chdir(dir)
		}(oldDir)

		customPath := filepath.Join("my", "custom", "path")
		t.Setenv("XDG_CONFIG_HOME", customPath)

		var want []string

		// First global
		if runtime.GOOS != windowsOs {
			want = append(want, "/etc/gremlins")
		}

		// Then $XDG_CONFIG_HOME and $HOME
		want = append(want,
			filepath.Join(customPath, "gremlins", "gremlins"),
			filepath.Join(home, ".gremlins"))

		// Then Go module root, then current directory
		moduleRoot, _ := os.Getwd()
		want = append(want, moduleRoot, ".")

		got := defaultConfigPaths()

		if !cmp.Equal(got, want) {
			t.Error(cmp.Diff(got, want))
		}
	})
}

func TestGeneratesMutantTypeEnabledKey(t *testing.T) {
	mt := mutator.ArithmeticBase
	want := "mutants.arithmetic-base.enabled"

	got := MutantTypeEnabledKey(mt)

	if got != want {
		t.Errorf("expected %q, got %q", mt, want)
	}
}

func TestViperSynchronisedAccess(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		value any
		name  string
		key   string
	}{
		{
			name:  "bool",
			key:   "tvsa.bool.key",
			value: true,
		},
		{
			name:  "int",
			key:   "tvsa.int.key",
			value: 10,
		},
		{
			name:  "float64",
			key:   "tvsa.float64.key",
			value: float64(10),
		},
		{
			name:  "string",
			key:   "tvsa.string.key",
			value: "test string",
		},
		{
			name:  "char",
			key:   "tvsa.char.key",
			value: 'a',
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			Set(tc.key, tc.value)

			got := Get[any](tc.key)

			if !cmp.Equal(got, tc.value) {
				t.Errorf("expected %v, got %v", tc.value, got)
			}
		})
	}
}

func TestReset(t *testing.T) {
	Set("test.key", true)

	Reset()

	got := Get[bool]("test.key")

	if got != false {
		t.Errorf("expected config to be reset")
	}
}
