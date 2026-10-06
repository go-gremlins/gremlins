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

// Package dup is the fixture for TestDupBehave: compile-time sites in
// function bodies -- const declarations, array lengths, constant composite
// literal keys -- which schemata places by duplicating the function.
package dup

// LocalConstArray sizes an array with a local constant.
func LocalConstArray() int {
	const n = 2 * 3
	var b [n]byte
	return len(b) + n
}

// LocalConstIota repeats a shifted iota.
func LocalConstIota() []int {
	const (
		A = 1 << iota
		B
		C
	)
	return []int{A, B, C}
}

// LocalConstTypedFloat declares a typed float constant.
func LocalConstTypedFloat() float32 {
	const r float32 = 1.5 * 2
	return r
}

// ArrayLenLiteral sizes an array with a constant expression.
func ArrayLenLiteral() int {
	var b [4 * 2]int
	return len(b)
}

// ArrayIndexKey indexes an array literal with a constant key.
func ArrayIndexKey() []int {
	a := [...]int{2 + 1: 9}
	return a[:]
}

// MapKey keys a map literal with a constant expression.
func MapKey() map[int]string {
	return map[int]string{3 - 1: "a"}
}

// T has a method with an unnamed receiver and parameter.
type T struct{}

// M is a method whose receiver and parameter are unnamed.
func (T) M(int) int {
	const k = 3 - 1
	return k
}

// G sizes an array of its type parameter, and is variadic.
func G[E any](xs ...E) int {
	var a [2 + 2]E
	return len(a) + len(xs)
}

// RecoverInCopy recovers, in a deferred closure, the panic of an index its
// constant puts out of range.
func RecoverInCopy(s []int) (r int) {
	defer func() {
		if e := recover(); e != nil {
			r = len(s)
		}
	}()
	const i = 2 - 1
	return s[i]
}

// PanicLine panics, on a line of its own, when its constant is mutated.
func PanicLine(s []int) int {
	const i = 2 - 1
	return s[i]
}

// TwoMutants has two mutants at one constant operator.
func TwoMutants() int {
	const k = 6 - 2
	return k
}
