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

package deps

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		out  string
		pkg  string
		want []string
	}{
		// A -> B -> C. Mutating B can break A, so A is B's dependent; C has no
		// dependents through B because the arrow points the other way.
		"a package is the dependent of everything it imports": {
			out: "m/a|m/b,m/c||\n" +
				"m/b|m/c||\n" +
				"m/c|||\n",
			pkg:  "m/b",
			want: []string{"m/a"},
		},
		"transitive dependents come through Deps, which go list already flattens": {
			out: "m/a|m/b,m/c||\n" +
				"m/b|m/c||\n" +
				"m/c|||\n",
			pkg:  "m/c",
			want: []string{"m/a", "m/b"},
		},
		// The one that is easy to miss: nothing in m/a's own code imports m/b,
		// but its tests do, so a mutation in m/b can break m/a's tests.
		"a package whose TESTS import it is a dependent too": {
			out: "m/a||m/b|\n" +
				"m/b|||\n",
			pkg:  "m/b",
			want: []string{"m/a"},
		},
		"an external test package counts the same way": {
			out: "m/a|||m/b\n" +
				"m/b|||\n",
			pkg:  "m/b",
			want: []string{"m/a"},
		},
		// A test import drags in its own dependencies: m/a's tests import m/b,
		// and m/b uses m/c, so mutating m/c can break m/a's tests.
		"a test import brings its own dependencies with it": {
			out: "m/a||m/b|\n" +
				"m/b|m/c||\n" +
				"m/c|||\n",
			pkg:  "m/c",
			want: []string{"m/a", "m/b"},
		},
		"a package is never its own dependent": {
			out:  "m/a|m/a||\n",
			pkg:  "m/a",
			want: nil,
		},
		// Only module packages are reported: the standard library and the module
		// cache are not things this run can mutate.
		"packages outside the listing are not reported": {
			out:  "m/a|fmt,strings||\n",
			pkg:  "fmt",
			want: nil,
		},
		"a package nothing depends on has no dependents": {
			out:  "m/a|||\nm/b|||\n",
			pkg:  "m/a",
			want: nil,
		},
		"lines that are not package lines are skipped": {
			out:  "go: downloading example.com v1.0.0\nm/a|m/b||\nm/b|||\n",
			pkg:  "m/b",
			want: []string{"m/a"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tc.want, parse(tc.out).Dependents(tc.pkg)); diff != "" {
				t.Errorf("Dependents(%s) mismatch (-want +got):\n%s", tc.pkg, diff)
			}
		})
	}
}

func TestNilGraphAnswersNothing(t *testing.T) {
	t.Parallel()

	var g *Graph
	if got := g.Dependents("m/a"); got != nil {
		t.Errorf("want nil from a nil graph, got %v", got)
	}
	if got := g.Len(); got != 0 {
		t.Errorf("want 0 from a nil graph, got %d", got)
	}
}
