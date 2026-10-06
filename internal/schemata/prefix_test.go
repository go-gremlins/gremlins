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
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestChoosePrefix(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		srcs []string
		want string
	}{
		"default_free":     {srcs: []string{"package p\nfunc f() { x := 1; _ = x }\n"}, want: "_gremlins"},
		"prefix_taken":     {srcs: []string{"package p\nvar _gremlinsX = 1\n"}, want: "_gremlins2"},
		"local_taken":      {srcs: []string{"package p\nfunc f() { _gremlins := 1; _ = _gremlins }\n"}, want: "_gremlins2"},
		"digits_continue":  {srcs: []string{"package p\nvar _gremlins23x = 1\n"}, want: "_gremlins3"},
		"base_only_digits": {srcs: []string{"package p\nvar _gremlins1 = 1\n"}, want: "_gremlins2"},
		"leading_zero":     {srcs: []string{"package p\nvar _gremlins02 = 1\n"}, want: "_gremlins2"},
		"other_prefix":     {srcs: []string{"package p\nvar x_gremlins, _gremlin = 1, 2\n"}, want: "_gremlins"},
		"empty_files":      {srcs: nil, want: "_gremlins"},
		"multi_file": {srcs: []string{
			"package p\nvar _gremlinsA = 1\n",
			"package p\nvar _gremlins2B = 1\n",
			"package p\nvar _gremlins3C = 1\n",
		}, want: "_gremlins4"},
		"taken_in_later_file": {srcs: []string{"package p\nvar a = 1\n", "package p\nvar _gremlins = 1\n"}, want: "_gremlins2"},
		"two_taken":           {srcs: []string{"package p\nvar _gremlinsA, _gremlins2B = 1, 2\n"}, want: "_gremlins3"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var files []*ast.File
			fset := token.NewFileSet()
			for i, src := range tc.srcs {
				f, err := parser.ParseFile(fset, fmt.Sprintf("p%d.go", i), src, 0)
				if err != nil {
					t.Fatal(err)
				}
				files = append(files, f)
			}
			if got := ChoosePrefix(files); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
