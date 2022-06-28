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

package main

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"runtime"
	"testing"
)

func TestVersionString(t *testing.T) {
	platform := fmt.Sprintf("\n\tGOOS: %s\n\tGOARCH: %s", runtime.GOOS, runtime.GOARCH)
	testCases := []struct {
		name    string
		version string
		date    string
		builtBy string
		want    string
	}{
		{
			name: "all empty",
			want: platform,
		},
		{
			name:    "version only",
			version: "1.2.3",
			want:    "1.2.3" + platform,
		},
		{
			name:    "version and date",
			version: "1.2.3",
			date:    "12/12/12",
			want:    "1.2.3\n\tbuilt at 12/12/12" + platform,
		},
		{
			name:    "complete",
			version: "1.2.3",
			date:    "12/12/12",
			builtBy: "test builder",
			want:    "1.2.3\n\tbuilt at 12/12/12 by test builder" + platform,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := buildVersion(tc.version, tc.date, tc.builtBy)
			if got != tc.want {
				t.Errorf(cmp.Diff(got, tc.want))
			}
		})
	}
}
