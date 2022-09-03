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

package engine

import (
	"sync"
	"time"

	"github.com/go-gremlins/gremlins/internal/configuration"
)

// DefaultTimeoutCoefficient is the default multiplier for the timeout length
// of each test run.
const DefaultTimeoutCoefficient = 4.0

// Timeout keeps track of timeouts durations per package. It is meant to be
// concurrency safe, because it is accessed by parallel workers in the Engine.
// Timeout applies a configurable coefficient to each package timeout to
// fine tune it, in order to avoid too strict timings and consequently false
// positive TIMED OUT mutations.
// The package timeout is "adaptive". It maintains an average of the computed
// test timings.
type Timeout struct {
	m           sync.RWMutex
	packages    map[string]time.Duration
	coefficient float64
}

// NewTimeout instantiates a Timeout.
func NewTimeout() *Timeout {
	coefficient := DefaultTimeoutCoefficient
	c := configuration.Get[float64](configuration.UnleashTimeoutCoefficientKey)
	if c != 0 {
		coefficient = c
	}
	// Can we find a way to have here the total number of packages?
	return &Timeout{packages: make(map[string]time.Duration), coefficient: coefficient}
}

// SetTo sets a timeout for a package.
// Before setting it, it applies the coefficient and computes the average
// with the current timeout if already present.
func (t *Timeout) SetTo(pkg string, duration time.Duration) time.Duration {
	t.m.Lock()
	defer t.m.Unlock()
	d := time.Duration(float64(duration) * t.coefficient)
	c, ok := t.packages[pkg]
	if ok {
		d += c
		d /= 2
	}
	t.packages[pkg] = d

	return d
}

// Of returns the time.Duration of the timeout for a given package.
func (t *Timeout) Of(pkg string) (time.Duration, bool) {
	t.m.RLock()
	defer t.m.RUnlock()
	to, ok := t.packages[pkg]

	return to, ok
}
