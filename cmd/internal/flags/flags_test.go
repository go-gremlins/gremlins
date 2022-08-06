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
)

type unsupportedType int

func TestSet(t *testing.T) {
	testCases := []struct {
		flag        Flag
		expectError bool
	}{
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

			err := Set(cmd, &tc.flag)
			if (tc.expectError && err == nil) || (!tc.expectError && err != nil) {
				t.Fatal("error not expected")
			}
			if !tc.expectError {
				if cmd.Flags().Lookup(tc.flag.Name) == nil {
					t.Errorf("expected flag to be present")
				}
			}

			tc.flag.Name += "_persistent"
			err = SetPersistent(cmd, &tc.flag)
			if (tc.expectError && err == nil) || (!tc.expectError && err != nil) {
				t.Fatal("error not expected")
			}
			if !tc.expectError {
				if cmd.Flag(tc.flag.Name) == nil {
					t.Errorf("expected flag to be present")
				}
			}

		})
	}
}
