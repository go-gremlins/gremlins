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

package engine_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/engine"
)

func TestTimeouts(t *testing.T) {
	t.Parallel()
	coefficient := 1.6
	viperSet(map[string]any{configuration.UnleashTimeoutCoefficientKey: coefficient})
	defer viperReset()
	timeout := engine.NewTimeout()

	t.Run("test if package timeout is established", func(t *testing.T) {
		t.Parallel()
		_, ok := timeout.Of("notSetPkg")

		if ok {
			t.Errorf("expected an error")
		}
	})

	t.Run("it is adaptive", func(t *testing.T) {
		t.Parallel()
		timeout.SetTo("testPkg", 3*time.Second)
		timeout.SetTo("testPkg", 7*time.Second)
		timeout.SetTo("testPkg", 6*time.Second)
		timeout.SetTo("testPkg", 4*time.Second)

		got, _ := timeout.Of("testPkg")

		want := 7600 * time.Millisecond
		if got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	})

	t.Run("set returns set value", func(t *testing.T) {
		t.Parallel()

		got := timeout.SetTo("pkgToSet", 3*time.Second)

		want := 4800 * time.Millisecond
		if got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	})

	timeSet := 3 * time.Second
	timeWant := 4800 * time.Millisecond
	// Tests for concurrent access, this must be run with the -race flag.
	for i := 0; i < 100; i++ {
		i := i
		tName := fmt.Sprintf("sets ang gets in parallel %d", i)
		t.Run(tName, func(t *testing.T) {
			t.Parallel()
			name := fmt.Sprintf("pack-%d", i)
			timeout.SetTo(name, timeSet)
			got, _ := timeout.Of(name)
			if got != timeWant {
				t.Errorf("wanted %s, got %s", timeWant, got)
			}
		})
	}
}
