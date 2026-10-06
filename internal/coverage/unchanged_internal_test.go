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

package coverage

import (
	"testing"
	"time"
)

// unchanged decides when a package is served without compiling, so every one
// of its refusals is a mapping that would otherwise be trusted unchecked.
func TestUnchanged(t *testing.T) {
	usable := fingerprint{pkgPrint: pkgPrint{Whole: "w", Typed: true}, Inputs: "i"}
	entry := func(fp fingerprint) cachedPackage {
		return cachedPackage{
			Fingerprint: fp,
			Tests:       map[string]Profile{"TestA": {}},
			Durations:   map[string]time.Duration{"TestA": time.Second},
			Compiled:    time.Second,
		}
	}

	if !unchanged(entry(usable), usable) {
		t.Fatal("an equal, usable fingerprint with every timing recorded should be served")
	}
	cases := map[string]struct {
		cached cachedPackage
		now    fingerprint
	}{
		// An entry whose mappings could not be attributed is stored under an
		// empty fingerprint, and a run that cannot read a dependency or its
		// build inputs takes one: equal, and saying nothing.
		"both fingerprints empty": {entry(fingerprint{}), fingerprint{}},
		// Each emptiness on its own, typed, so that no other guard covers it.
		"no whole": {entry(fingerprint{pkgPrint: pkgPrint{Typed: true}, Inputs: "i"}),
			fingerprint{pkgPrint: pkgPrint{Typed: true}, Inputs: "i"}},
		"no build inputs": {entry(fingerprint{pkgPrint: pkgPrint{Whole: "w", Typed: true}}),
			fingerprint{pkgPrint: pkgPrint{Whole: "w", Typed: true}}},
		"a different fingerprint": {entry(usable), fingerprint{pkgPrint: pkgPrint{Whole: "x", Typed: true}, Inputs: "i"}},
		// reusable re-maps a package the type-checker could not read rather
		// than compare it; serving it unread would be the one case that got
		// less checking than a moved build ID.
		"an untyped print": {entry(fingerprint{pkgPrint: pkgPrint{Whole: "w"}, Inputs: "i"}),
			fingerprint{pkgPrint: pkgPrint{Whole: "w"}, Inputs: "i"}},
		"no compile time": {func() cachedPackage {
			e := entry(usable)
			e.Compiled = 0

			return e
		}(), usable},
		// A test without a duration would drop the package out of the suite
		// baseline, and the gather would run it after all.
		"a test without a duration": {func() cachedPackage {
			e := entry(usable)
			e.Tests["TestB"] = Profile{}

			return e
		}(), usable},
	}
	for name, tc := range cases {
		if unchanged(tc.cached, tc.now) {
			t.Errorf("%s: served without compiling, want compiled", name)
		}
	}
}
