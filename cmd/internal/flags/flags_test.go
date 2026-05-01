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
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/configuration"
)

type cliCase struct {
	want      any
	getResult func(string) any
	flag      Flag
	args      []string
}

func TestSetBindsTypedValueFromCLI(t *testing.T) {
	testCases := []cliCase{
		{
			flag: Flag{
				Name:     "threshold-efficacy",
				CfgKey:   "unleash.threshold.efficacy",
				DefaultV: float64(0),
				Usage:    "test usage",
			},
			args:      []string{"--threshold-efficacy", "50"},
			getResult: func(k string) any { return configuration.Get[float64](k) },
			want:      float64(50),
		},
		{
			flag: Flag{
				Name:     "workers",
				CfgKey:   "unleash.workers",
				DefaultV: 0,
				Usage:    "test usage",
			},
			args:      []string{"--workers", "4"},
			getResult: func(k string) any { return configuration.Get[int](k) },
			want:      4,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.flag.Name, func(t *testing.T) {
			defer viper.Reset()

			cmd := &cobra.Command{Use: "test", Run: func(_ *cobra.Command, _ []string) {}}
			// #nosec G601 - We are in tests, we don't care
			if err := Set(cmd, &tc.flag); err != nil {
				t.Fatal("Set should not fail")
			}
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatal("Execute should not fail")
			}

			if got := tc.getResult(tc.flag.CfgKey); got != tc.want {
				t.Errorf("expected configuration.Get(%q) to be %T(%v), got %T(%v)", tc.flag.CfgKey, tc.want, tc.want, got, got)
			}
		})
	}
}

type unsupportedType int

type testCase struct {
	flag        Flag
	expectError bool
}

func TestSet(t *testing.T) {
	testCases := []testCase{
		{
			flag: Flag{
				Name:      "bool-flag-no-sh",
				CfgKey:    "test.cfg",
				Shorthand: "",
				DefaultV:  true,
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "bool-flag-sh",
				CfgKey:    "test.cfg",
				Shorthand: "t",
				DefaultV:  true,
				Usage:     "test usage",
			},
		},

		{
			flag: Flag{
				Name:      "string-flag-no-sh",
				CfgKey:    "test.cfg",
				Shorthand: "",
				DefaultV:  "test",
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "string-flag-sh",
				CfgKey:    "test.cfg",
				Shorthand: "t",
				DefaultV:  "test",
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "int-flag-no-sh",
				CfgKey:    "test.cfg",
				Shorthand: "",
				DefaultV:  0,
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "int-flag-sh",
				CfgKey:    "test.cfg",
				Shorthand: "t",
				DefaultV:  0,
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "float64-flag-no-sh",
				CfgKey:    "test.cfg",
				Shorthand: "",
				DefaultV:  float64(0),
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "float64-flag-sh",
				CfgKey:    "test.cfg",
				Shorthand: "t",
				DefaultV:  float64(0),
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "[]string-flag-no-sh",
				CfgKey:    "test.cfg",
				Shorthand: "",
				DefaultV:  []string{},
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "[]string-flag-sh",
				CfgKey:    "test.cfg",
				Shorthand: "t",
				DefaultV:  []string{},
				Usage:     "test usage",
			},
		},
		{
			flag: Flag{
				Name:      "not-supported-type",
				CfgKey:    "test.cfg",
				Shorthand: "t",
				DefaultV:  unsupportedType(0),
				Usage:     "test usage",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.flag.Name, func(t *testing.T) {
			defer viper.Reset()

			cmd := &cobra.Command{}

			// #nosec G601 - We are in tests, we don't care
			err := Set(cmd, &tc.flag)
			checkFlag(t, err, cmd, tc)

			tc.flag.Name += "_persistent"
			// #nosec G601 - We are in tests, we don't care
			err = SetPersistent(cmd, &tc.flag)
			checkFlag(t, err, cmd, tc)
		})
	}
}

func checkFlag(t *testing.T, err error, cmd *cobra.Command, tc testCase) {
	t.Helper()

	if (tc.expectError && err == nil) || (!tc.expectError && err != nil) {
		t.Fatal("error not expected")
	}
	if !tc.expectError {
		flag := cmd.Flag(tc.flag.Name)

		if flag == nil {
			t.Errorf("expected flag to be present")

			return
		}

		if tc.flag.Shorthand != flag.Shorthand {
			t.Errorf("expected configured shorthand")
		}
	}
}
