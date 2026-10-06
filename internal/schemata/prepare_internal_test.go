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

package schemata

import (
	"testing"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// TestNetFailedChecksTheMutantsOwnPackage holds the mutants of a package
// whose rewritten files failed to write -- BuildAll records that error under
// the package itself -- to the per-mutant path even when their selection
// names only a dependent package that built: no binary compiles them in.
func TestNetFailedChecksTheMutantsOwnPackage(t *testing.T) {
	t.Parallel()
	const writeErr = "schemata: write p/p.go: permission denied"
	testCases := map[string]struct {
		need   map[string][]string
		failed map[string]string
		want   []string
	}{
		"own_package_write_failed_selection_only_dependent": {
			need:   map[string][]string{"m/p": {"m/dep"}, "m/q": {"m/q"}},
			failed: map[string]string{"m/p": writeErr},
			want:   []string{writeErr, writeErr, ""},
		},
		"selected_package_failed": {
			need:   map[string][]string{"m/p": {"m/dep"}, "m/q": {"m/q"}},
			failed: map[string]string{"m/dep": "build failed"},
			want:   []string{"build failed", "build failed", ""},
		},
		"nothing_failed": {
			need: map[string][]string{"m/p": {"m/dep"}, "m/q": {"m/q"}},
			want: []string{"", "", ""},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := &preparer{
				muts:    make([]mutator.Mutator, 3),
				reasons: make([]string, 3),
				pkgOf:   []string{"m/p", "m/p", "m/q"},
			}
			p.netFailed(tc.need, tc.failed)
			for i, want := range tc.want {
				if p.reasons[i] != want {
					t.Errorf("mutant %d (%s): reason %q, want %q", i, p.pkgOf[i], p.reasons[i], want)
				}
			}
		})
	}
}
