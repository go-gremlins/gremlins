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
	"go/ast"
	"strconv"
	"strings"
)

const prefixBase = "_gremlins"

// ChoosePrefix returns an identifier prefix that no identifier in files
// starts with -- locals, fields and labels included -- so that nothing the
// rewrite generates can collide with, shadow or be captured by a user name.
// The candidates are "_gremlins", "_gremlins2", "_gremlins3", and so on.
func ChoosePrefix(files []*ast.File) string {
	// taken holds the n of every candidate some identifier starts with; the
	// base is candidate 1. One pass over the identifiers, none kept.
	taken := map[int]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				markTaken(id.Name, taken)
			}

			return true
		})
	}
	for i := 1; ; i++ {
		if !taken[i] {
			return candidatePrefix(i)
		}
	}
}

func candidatePrefix(i int) string {
	if i == 1 {
		return prefixBase
	}

	return prefixBase + strconv.Itoa(i)
}

// markTaken records in taken every candidate that name starts with: the base
// and, for each leading run of digits after it, the candidate that number
// names ("_gremlins23x" starts "_gremlins2" and "_gremlins23").
func markTaken(name string, taken map[int]bool) {
	rest, ok := strings.CutPrefix(name, prefixBase)
	if !ok {
		return
	}
	taken[1] = true
	if rest == "" || rest[0] < '1' || rest[0] > '9' {
		return // candidates carry no leading zero, and never 0 or 1
	}
	n := 0
	for _, c := range rest {
		if c < '0' || c > '9' || n > 1<<20 {
			break
		}
		n = n*10 + int(c-'0')
		if n >= 2 {
			taken[n] = true
		}
	}
}
