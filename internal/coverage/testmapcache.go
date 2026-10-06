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

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cacheVersion is the shape of the cache file. A cache written by a different
// version is discarded rather than migrated: it costs one rebuild, and the
// alternative is reading a map whose meaning has changed.
const cacheVersion = 1

// cachedPackage is one package's mapping, and the build ID of the test binary
// it was produced from.
type cachedPackage struct {
	BuildID string             `json:"build_id"`
	Tests   map[string]Profile `json:"tests"`
}

// mapCache is the on-disk test map.
//
// The key of a package is the build ID of its test binary, which Go computes
// over the package's own source AND every dependency's, transitively. So an
// unchanged build ID means nothing that the package's tests execute has
// changed, and the coverage they produced cannot have changed either. Touching
// a package three levels down invalidates every binary that links it, without
// Gremlins doing any dependency analysis of its own.
//
// What the build ID cannot see is state outside the build: a test whose
// coverage depends on a database, a clock, or the network can map differently
// on two runs of the same binary. That is the same non-determinism the map has
// without a cache, held for longer.
type mapCache struct {
	Version  int                      `json:"version"`
	Key      string                   `json:"key"`
	Packages map[string]cachedPackage `json:"packages"`
}

// cacheKey covers what changes the meaning of every entry at once rather than
// per package: how much of the module was mapped — a map built without
// --cross-package records only each test's own package — and the build tags
// that decide which files exist at all.
func cacheKey(scope, buildTags string) string {
	sum := sha256.Sum256([]byte(scope + "\x00" + buildTags))

	return hex.EncodeToString(sum[:])
}

// cachePath is where the map for this module lives. It is outside the module,
// under the user's cache directory unless a caller names another, so that a
// checkout stays clean and two checkouts of the same module do not share a map.
func (c *Coverage) cachePath() (string, error) {
	base := c.cacheDir
	if base == "" {
		var err error
		base, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	modName, modRoot := c.mod.Name, c.mod.Root
	root, err := filepath.Abs(modRoot)
	if err != nil {
		root = modRoot
	}
	sum := sha256.Sum256([]byte(modName + "\x00" + root))

	return filepath.Join(base, "gremlins", "testmap", hex.EncodeToString(sum[:])+".json"), nil
}

// loadCache reads the cache, or returns an empty one.
//
// Every failure returns an empty cache rather than an error: a missing,
// unreadable, corrupt, or stale-versioned cache costs a rebuild, which is
// exactly what happens without a cache at all. There is no failure here worth
// stopping a run for.
func loadCache(path, key string) *mapCache {
	empty := &mapCache{Version: cacheVersion, Key: key, Packages: map[string]cachedPackage{}}

	data, err := os.ReadFile(path) //nolint:gosec // G304: the path is Gremlins' own cache directory
	if err != nil {
		return empty
	}
	var c mapCache
	if err := json.Unmarshal(data, &c); err != nil {
		return empty
	}
	if c.Version != cacheVersion || c.Key != key || c.Packages == nil {
		return empty
	}

	return &c
}

// save writes the cache through a temporary file, so that an interrupted run
// leaves the previous cache rather than a half-written one.
func (c *mapCache) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "testmap-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)

		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)

		return err
	}

	return os.Rename(name, path)
}

// buildID asks Go for the identity of a compiled binary. It is the hash Go
// itself uses to decide whether a build is up to date, which is precisely the
// question the cache needs answered.
func (c *Coverage) buildID(binary string) (string, error) {
	out, err := c.cmdContext("go", "tool", "buildid", binary).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w\n%s", err, out)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("go tool buildid returned nothing for %s", binary)
	}

	return id, nil
}
