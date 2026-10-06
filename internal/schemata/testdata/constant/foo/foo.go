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

// Package foo declares unexported types that the app package can only
// reach through foo's functions: a rewrite that had to name them would not
// compile.
package foo

type level int

type small uint8

type ratio float64

// Unit is a typed constant: an expression of it keeps the type level.
const Unit level = 2

func Set(l level) level { return l }

func Small() small { return 7 }

func SetR(r ratio) ratio { return r }

func R() ratio { return 0 }

// Mix takes its float second and returns two results.
func Mix(l level, r ratio) (level, ratio) { return l, r }

type Box struct{ R ratio }

func (b *Box) Put(r ratio) { b.R = r }

func Sum(rs ...ratio) ratio {
	var s ratio
	for _, r := range rs {
		s += r
	}
	return s
}

func Pick[T ~float64](x T) T { return x }

// Level is an exported float type, which the app package names as foo.Level.
type Level float64
