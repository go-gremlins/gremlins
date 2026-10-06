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

package dup

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
)

// show prints name and f's result, or the value f panicked with and the
// dup.go line it panicked on.
func show(name string, f func() any) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %v at %s\n", name, r, panicLine())
		}
	}()
	fmt.Printf("%s: %v\n", name, f())
}

// panicLine is the file and line of the innermost dup.go frame of a
// panicking goroutine, called from the deferred function that recovers.
func panicLine() string {
	pcs := make([]uintptr, 32)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(1, pcs)])
	for {
		f, more := frames.Next()
		if filepath.Base(f.File) == "dup.go" {
			return fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line)
		}
		if !more {
			return "no dup.go frame"
		}
	}
}

func TestPrint(t *testing.T) {
	show("LocalConstArray", func() any { return LocalConstArray() })
	show("LocalConstIota", func() any { return LocalConstIota() })
	show("LocalConstTypedFloat", func() any { return LocalConstTypedFloat() })
	show("ArrayLenLiteral", func() any { return ArrayLenLiteral() })
	show("ArrayIndexKey", func() any { return ArrayIndexKey() })
	show("MapKey", func() any { return MapKey() })
	show("Method", func() any { return T{}.M(0) })
	show("Generic", func() any { return G(1, 2, 3) })
	show("RecoverInCopy", func() any { return RecoverInCopy([]int{7, 8}) })
	show("PanicLine", func() any { return PanicLine([]int{7, 8}) })
	show("TwoMutants", func() any { return TwoMutants() })
}
