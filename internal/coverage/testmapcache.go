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
	"time"
)

// cacheVersion is the shape of a cache file. A file written by a different
// version is discarded rather than migrated: it costs one rebuild, and the
// alternative is reading a map whose meaning has changed.
//
// Version 12 records how long each test's mapping run took, which is what a
// run that skips the coverage gather derives its timeout baseline from.
// Version 11 asks the type-checker which vars read another package at
// initialisation, which a dot import or a var holding another's address hides
// from the syntax.
// Version 10 names a method by its receiver type alone.
// Version 9 records what each print's package reaches by import, which is
// what moves the order packages are initialised in, in place of comparing
// each file's imports; and prints TestMain as its own kind, which does not
// count as seeing that order.
// Version 8 carries each print's initialisation order as the type-checker
// derives it, and refuses to narrow from a print it could not derive it for.
// Version 7 folds every linked package and every main module's go.mod into
// Inputs, records which vars read another package at initialisation, names a
// local type's aliases among its names, lets fmt.Errorf through only over
// operands with no method of the package's, and reads a test file by its
// entities rather than its name.
// Version 6 prints every top-level entity of a package and its instrumented
// dependencies on its own — functions, methods, type and var specs, const
// groups, each file's import table — in place of one all-or-nothing shell, and
// follows a changed name through the headers that mention it (see reusable).
// Version 5 instruments a package's in-module dependencies alongside it: each
// test records the dependency functions it executed, the fingerprint carries
// each dependency's print, and Inputs no longer hashes those directories.
// Version 4 is shared between checkouts: its directory is named from the
// module alone, its Inputs name nothing by absolute path and fold in the build
// environment and compile flags, and an entry whose fingerprint and Inputs both
// agree is reused whole across a moved build ID. Version 3 records the
// package's source fingerprint beside its mappings, so that a changed package
// can keep the mappings the change could not have touched. Version 2 was one
// file per package. Version 1 was one file per module, which meant a run had
// to write back every package it had not looked at or lose them — and a
// scoped run, which is the recommended workflow, looks at one.
const cacheVersion = 12

// cachedPackage is one package's mapping, and the build ID of the test binary
// it was produced from.
//
// The build ID is the key, and Go computes it over the package's own source AND
// every dependency's, transitively. So an unchanged build ID means nothing the
// package's tests execute has changed, and the coverage they produced cannot
// have changed either. Touching a package three levels down invalidates exactly
// the binaries that link it, without Gremlins doing any dependency analysis of
// its own.
//
// What the build ID cannot see is state outside the build: a test whose
// coverage depends on a database, a clock, or the network can map differently
// on two runs of the same binary. That is the same non-determinism the map has
// without a cache, held for longer.
//
// The build ID being coarse is why Fingerprint is here as well: it records what
// the package's source looked like when the mappings were made, so that a run
// whose build ID has moved can still ask which of them the change reached. An
// entry may have none — a package whose directory could not be read — in which
// case a changed build ID re-maps the whole package, as it always did.
//
// Deps is, per test, the dependency entities it executed (see
// splitCoverage). It is apart from Tests on purpose: Tests is coverage, and
// read as coverage everywhere, while Deps is only ever asked "did this test
// reach an entity that changed". A test that reached none has no entry.
//
// Durations is, per test, how long its mapping run took: the whole process,
// from start to exit, as profileForTest ran it. A reused mapping keeps the
// duration it was recorded with. See TestMap.SuiteBaseline for what reads it.
//
// ImportPath is stored as well as hashed into the file name, so that a file
// found under the wrong name is a miss rather than another package's answer.
type cachedPackage struct {
	Tests     map[string]Profile       `json:"tests"`
	Deps      map[string][]string      `json:"deps,omitempty"`
	Durations map[string]time.Duration `json:"durations,omitempty"`
	// Compiled is how long the package's test binary took to compile when it
	// was mapped. A hit that skips the compile reports it in its place, so the
	// package stays in the suite baseline; an entry without one is compiled.
	Compiled    time.Duration `json:"compiled,omitempty"`
	ImportPath  string        `json:"import_path"`
	BuildID     string        `json:"build_id"`
	Fingerprint fingerprint   `json:"fingerprint"`
	Version     int           `json:"version"`
}

// cacheKey covers what changes the meaning of every entry at once rather than
// per package: what the mappings were measured against, and the build tags that
// decide which files exist at all.
//
// It names a directory rather than living inside the files, so that a
// --cross-package map and a narrow one cannot be mistaken for one another even
// though they describe the same packages.
func cacheKey(scope, buildTags string) string {
	sum := sha256.Sum256([]byte(scope + "\x00" + buildTags))

	return hex.EncodeToString(sum[:])
}

// perPackageScope stands for "each test binary was instrumented over its own
// package", which is what happens without --cross-package. It is not a package
// pattern and cannot collide with one: a --coverpkg value carrying a NUL cannot
// be typed on a command line or written in a config file.
const perPackageScope = "\x00per-package"

// cacheScope is what the mappings under one key were measured against, and so
// what an entry has to agree with to be readable.
//
// It is exactly the -coverpkg each test binary was compiled with, which is what
// decides whose lines can appear in a profile — hence the delegation, so the two
// cannot drift apart. Without --cross-package that is the package's own import
// path, and the answer here collapses to a constant: every package's mapping is
// then a function of that package alone.
//
// What is deliberately NOT here is how much of the module was scanned. It
// decides which packages get mapped and nothing about what a mapping says, so a
// run scoped to one package reads the file a whole-module run wrote for it
// rather than starting empty.
func (c *Coverage) cacheScope() string {
	return c.testMapCoverPkg(perPackageScope)
}

// cacheDirPath is where this module's per-package map files live. It is outside
// the module, under the user's cache directory unless a caller names another,
// so that a checkout stays clean.
//
// It is named from the module name alone, so that every checkout of a module
// shares one map. A CI fleet checks a module out under a different work
// directory on every runner, and a map named after the checkout path could only
// ever be read back by the runner that wrote it. Sharing is safe because no
// entry is trusted for where it was found: an entry is used whole only while
// its package's build ID agrees, and in part only while its fingerprint and
// Inputs do — and neither of those names a path. Two checkouts writing at once
// cannot tear a file, because every save is a rename; at worst one overwrites
// the other's entry, and the loser re-maps next time.
func (c *Coverage) cacheDirPath(key string) (string, error) {
	base := c.cacheDir
	if base == "" {
		var err error
		base, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256([]byte(c.mod.Name))

	return filepath.Join(base, "gremlins", "testmap", hex.EncodeToString(sum[:]), key), nil
}

// cacheFilePath is the file holding one package's mapping. The import path is
// hashed rather than used directly: it contains separators, and on a
// case-insensitive filesystem two distinct import paths can name one file.
func cacheFilePath(dir, importPath string) string {
	sum := sha256.Sum256([]byte(importPath))

	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
}

// loadCachedPackage reads one package's mapping, reporting whether there is a
// usable one.
//
// Every failure reads as a miss rather than an error: a missing, unreadable,
// corrupt, or stale-versioned file costs a re-map of that package, which is
// exactly what happens without a cache at all. There is no failure here worth
// stopping a run for.
//
// A package that has gone away is never asked about — reads are keyed by an
// import path taken from the current `go list` — so its file is dead disk
// rather than a stale answer, and reclaiming it is housekeeping, not
// correctness.
func loadCachedPackage(dir, importPath string) (cachedPackage, bool) {
	data, err := os.ReadFile(cacheFilePath(dir, importPath))
	if err != nil {
		return cachedPackage{}, false
	}
	var p cachedPackage
	if err := json.Unmarshal(data, &p); err != nil {
		return cachedPackage{}, false
	}
	if p.Version != cacheVersion || p.ImportPath != importPath || p.Tests == nil || p.BuildID == "" {
		return cachedPackage{}, false
	}

	return p, true
}

// save writes one package's mapping through a temporary file in the same
// directory, so that an interrupted run leaves the previous file rather than a
// half-written one.
//
// A run writes only the packages it mapped. It has no reason to touch another
// package's file, so a scoped run cannot evict the rest of the module's map,
// and two runs scoped to different packages cannot clobber each other.
func (p cachedPackage) save(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "testmap-*.json")
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

	return os.Rename(name, cacheFilePath(dir, p.ImportPath))
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
