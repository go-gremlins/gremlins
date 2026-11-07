/*
 * Copyright 2026 The Gremlins Authors
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
	"runtime"
	"runtime/debug"
	"testing"
)

const releaseTag = "0.7.0"

func TestBuildVersionAddsPlatform(t *testing.T) {
	want := releaseTag + " " + runtime.GOOS + "/" + runtime.GOARCH
	if got := buildVersion(releaseTag); got != want {
		t.Errorf("buildVersion(%q) = %q, want %q", releaseTag, got, want)
	}
}

func TestReleaseVersion(t *testing.T) {
	buildInfo := func(version string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/OpenRAE/go-rae-gremlins", Version: version}}
	}
	testCases := []struct {
		info   *debug.BuildInfo
		name   string
		linked string
		want   string
		ok     bool
	}{
		{
			name:   "linker version wins over module version",
			linked: releaseTag,
			info:   buildInfo("v0.6.0"),
			ok:     true,
			want:   releaseTag,
		},
		{
			name:   "go install at a release tag reports the tag",
			linked: devVersion,
			info:   buildInfo("v0.7.0"),
			ok:     true,
			want:   releaseTag,
		},
		{
			name:   "go install at a commit reports the pseudo-version",
			linked: devVersion,
			info:   buildInfo("v0.6.1-0.20261106233346-3d24c2ec4786"),
			ok:     true,
			want:   "0.6.1-0.20261106233346-3d24c2ec4786",
		},
		{
			name:   "build without a module version stays dev",
			linked: devVersion,
			info:   buildInfo("(devel)"),
			ok:     true,
			want:   devVersion,
		},
		{
			name:   "empty module version stays dev",
			linked: devVersion,
			info:   buildInfo(""),
			ok:     true,
			want:   devVersion,
		},
		{
			name:   "missing build information stays dev",
			linked: devVersion,
			info:   nil,
			ok:     false,
			want:   devVersion,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := releaseVersion(tc.linked, tc.info, tc.ok)
			if got != tc.want {
				t.Errorf("releaseVersion(linked %q) = %q, want %q", tc.linked, got, tc.want)
			}
		})
	}
}
