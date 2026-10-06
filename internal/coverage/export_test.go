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

package coverage

// CacheVersion is the current cache file version, for the external tests that
// hand-write a cache file: one that is unusable for any other reason must
// carry the current version, or it tests the version check instead.
const CacheVersion = cacheVersion

// WithStubTypes stands in for the type-checker in the tests whose go command
// is a fake, and so whose fixtures are no module go/packages can load: every
// package type-checks, and none has an initialiser.
func WithStubTypes() Option {
	return func(c *Coverage) *Coverage {
		c.typeLoader = func(_ bool, importPaths []string) map[string]typeFacts {
			out := make(map[string]typeFacts, len(importPaths))
			for _, p := range importPaths {
				out[p] = typeFacts{}
			}

			return out
		}

		return c
	}
}
