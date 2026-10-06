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
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// buildInputsOf hashes everything the package's test binary is built from
// except the package itself and the dependencies it instruments: the source of
// every other dependency somebody could edit, the module's requirements and
// every main module's go.mod, the set of packages linked, the toolchain, the
// build environment, and the flags Gremlins compiles the binary with.
//
// Per-test invalidation needs this, and the build ID cannot supply it. The
// build ID folds the package's own source together with every dependency's, so
// a moved build ID says something changed and never says where. That
// distinction is the whole of the difference between sound and nearly sound: a
// changed dependency can send a test down a path it did not take before, and if
// no profile records that dependency's lines the change looks clean to every
// mapping the cache holds. Narrowing on the package's own fingerprint while
// such a dependency also moved would keep mappings that are stale, and a stale
// mapping means a test that could kill a mutant is never selected — the mutant
// reports LIVED and the gate goes red on something nobody can reproduce.
//
// So a run narrows only while this is unchanged, and re-maps the whole package
// the moment it is not.
//
// An instrumented dependency is the exception, and the only one: its lines ARE
// recorded, per test, as the functions each test executed there, and it is
// fingerprinted beside the package (see fingerprint.Deps), so what changed in
// it can be attributed just as a change in the package can. Leaving it out of
// here is sound only because every directory it leaves out is one of those —
// both read the same listing and the same instrumented flag, and a dependency
// that cannot be fingerprinted fails the whole fingerprint rather than falling
// through the gap between the two.
//
// It has to be complete as well as cheap, because it is also what lets an
// unchanged package skip the build ID: with the fingerprint and this both equal,
// a moved build ID is taken to mean nothing but a moved checkout (see
// reusable). Anything that changes the compiled binary and is in neither would
// then be reused across, which is why the build environment and the compile
// flags are here although no source file holds them.
//
// It also has to name nothing that differs between two checkouts of the same
// module, or no cache restored on another machine could ever be used. A
// dependency is named by its path relative to the module root, or by its import
// path when it lives outside the root — a replace target or a go.work member,
// which can sit anywhere on disk. GOWORK, which is a path, is folded in the same
// way: relative to the module root.
//
// Directories are read rather than build IDs asked for: a dependency's identity
// according to Go means building it, and this has to be cheap enough to run
// before deciding whether there is work to skip. Measured at ~330ms for a
// package with 430 transitive dependencies, twelve of them in the module.
//
// What is left out is left out because something else in the hash pins it.
// GOROOT is pinned by the toolchain version. The module cache is pinned by
// go.sum and go.work.sum, whose contents are here — and a dependency vendored or
// replaced into a local directory is not in the module cache, so it is read like
// any other source.
func (c *Coverage) buildInputsOf(pkg *testPackage) (string, bool) {
	env, ok := c.goEnv()
	if !ok {
		return "", false
	}
	listing := c.listingOf(pkg)
	if !listing.ok {
		return "", false
	}
	root := c.absRoot()

	parts := []string{env.version}
	parts = append(parts, env.build...)
	parts = append(parts, "flags\x00"+strings.Join(c.testBuildFlags(pkg), "\x00"))
	parts = append(parts, "work\x00"+relativeTo(root, env.work))
	for _, f := range []struct{ name, path string }{
		{"go.mod", filepath.Join(root, "go.mod")},
		{"go.sum", filepath.Join(root, "go.sum")},
		{"go.work", workFile(env.work, "go.work")},
		{"go.work.sum", workFile(env.work, "go.work.sum")},
	} {
		parts = append(parts, f.name+"\x00"+hashFileOrAbsent(f.path))
	}
	// Every main module's go.mod, the workspace members' as well as the
	// module's own: its go and godebug lines change what the toolchain does
	// with source that did not change.
	for _, goMod := range listing.goMods {
		parts = append(parts, "gomod\x00"+relativeTo(root, goMod)+"\x00"+hashFileOrAbsent(goMod))
	}
	// Which packages are linked at all, the standard library and the module
	// cache included. A package's initialisation runs in every test, so one
	// newly linked — crypto/md5 registering MD5 with crypto — changes what
	// code nobody edited does, and no name of it appears anywhere the
	// closure reads.
	parts = append(parts, "linked\x00"+strings.Join(listing.linked, "\x00"))
	for _, dep := range listing.deps {
		if dep.instrumented {
			continue
		}
		sum, dirOK := c.hashDir(dep.dir)
		if !dirOK {
			return "", false
		}
		parts = append(parts, dep.name+"\x00"+sum)
	}

	return hashOf([]byte(strings.Join(parts, "\x00"))), true
}

// hashFileOrAbsent hashes one file's content. Absent is a fact about the
// module, not a failure: a module with no go.sum has nothing pinned, one built
// without a workspace has no go.work, and saying so consistently is enough. An
// empty path is a file there is no reason to look for.
func hashFileOrAbsent(path string) string {
	if path == "" {
		return "absent"
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: the module root, or the workspace go itself reports
	if err != nil {
		return "absent"
	}

	return hashOf(data)
}

// workFile is a file of the workspace GOWORK names: go.work is GOWORK itself,
// and go.work.sum sits beside it. Without a workspace — GOWORK empty, or "off"
// — there is no such file.
func workFile(gowork, name string) string {
	if gowork == "" || gowork == "off" {
		return ""
	}
	if name == "go.work" {
		return gowork
	}

	return filepath.Join(filepath.Dir(gowork), name)
}

// absRoot is the module root as an absolute path, which is what a dependency's
// directory is compared against.
func (c *Coverage) absRoot() string {
	root, err := filepath.Abs(c.mod.Root)
	if err != nil {
		return c.mod.Root
	}

	return root
}

// relativeTo names a path the same way in every checkout that lays the module
// out the same way. A path it cannot relate is returned as it is, which can
// only ever cost a hit.
func relativeTo(root, path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}

	return filepath.ToSlash(rel)
}

// dependency is one package directory a test binary is built from, and the
// name it goes into Inputs under.
type dependency struct {
	name       string
	dir        string
	importPath string
	// instrumented says the test binary records this dependency's lines: it
	// belongs to a main module — the module being mapped, or a member of the
	// workspace it is in. Everything else is a replace target or a vendored
	// copy, which is read into Inputs whole as before.
	instrumented bool
}

// depListing is one package's dependency listing, kept for the run: it is
// asked for by the compile flags, by Inputs and by the fingerprint, and the
// three must agree.
type depListing struct {
	deps []dependency
	// linked is the import path of every package the test binary links,
	// sorted, whichever directory it lives in.
	linked []string
	// goMods is the go.mod of every main module the binary links a package
	// of, sorted.
	goMods []string
	ok     bool
}

// depListFormat asks for each dependency's import path, its directory,
// whether its module is a main module, its package name, and its module's
// go.mod. A package of the standard library has no module and reports an
// empty third and fifth field.
//
// The name is what an unnamed import binds in the importing file, which a path
// does not say: the file's import table is compared by local name (see
// pkgPrint.Imports).
const depListFormat = "{{.ImportPath}}\t{{.Dir}}\t{{with .Module}}{{.Main}}{{end}}\t{{.Name}}\t{{with .Module}}{{.GoMod}}{{end}}"

// dependencyDirs is where the source of everything this package's tests link
// lives, minus the package's own directory and minus what the toolchain and the
// module cache already pin.
//
// The listing is of the test binary's dependencies, not the package's: a test
// file's imports are linked too, and a change in one of them moves the build ID
// exactly as any other dependency does.
//
// Each is named so that the name is the same in every checkout: by its path
// inside the module, or by its import path when it is outside the module root.
// Go lists a package recompiled for a test binary under its import path with
// the binary's name in brackets; that suffix is dropped, since the directory is
// the same one. The result is ordered by name rather than by directory, for the
// same reason: two checkouts can order the same directories differently once
// some of them are outside the root.
//
// A dependency is instrumented when Go says its module is a main module, which
// is exactly the module itself and, in a workspace, its members. That is a
// narrower test than "inside the module root": a nested module under the root,
// required through a replace, is not a main module and stays in Inputs.
func (c *Coverage) dependencyDirs(pkg *testPackage) ([]dependency, bool) {
	listing := c.listingOf(pkg)

	return listing.deps, listing.ok
}

// listingOf is the package's dependency listing, asked for once per run.
func (c *Coverage) listingOf(pkg *testPackage) depListing {
	if listing, done := c.depListings[pkg.importPath]; done {
		return listing
	}
	listing := c.listDependencies(pkg)
	if c.depListings == nil {
		c.depListings = map[string]depListing{}
	}
	c.depListings[pkg.importPath] = listing

	return listing
}

func (c *Coverage) listDependencies(pkg *testPackage) depListing {
	out, err := c.cmdContext("go", c.listArgs("-deps", "-test", "-f", depListFormat, pkg.importPath)...).CombinedOutput()
	if err != nil {
		return depListing{}
	}

	env, ok := c.goEnv()
	if !ok {
		return depListing{}
	}
	own, err := filepath.Abs(pkg.dir)
	if err != nil {
		own = pkg.dir
	}
	root := c.absRoot()

	seen := map[string]struct{}{}
	linked, goMods := map[string]bool{}, map[string]bool{}
	var deps []dependency
	if c.pkgNames == nil {
		c.pkgNames = map[string]string{}
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, "\t")
		// `go list` writes build diagnostics to the same stream, and none of
		// them has the listing's shape.
		if len(fields) < 2 {
			continue
		}
		fields = append(fields, "", "", "")
		importPath := strings.TrimSpace(strings.SplitN(fields[0], " [", 2)[0])
		dir := strings.TrimSpace(fields[1])
		main := strings.TrimSpace(fields[2]) == "true"
		if importPath != "" {
			linked[importPath] = true
		}
		// Every package's name is recorded, the toolchain's and the module
		// cache's too: a file can import any of them without a name.
		if name := strings.TrimSpace(fields[3]); name != "" && importPath != "" {
			c.pkgNames[importPath] = name
		}
		if goMod := strings.TrimSpace(fields[4]); main && goMod != "" {
			goMods[goMod] = true
		}
		// A synthesised test package can report no directory at all.
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		if dir == own || under(dir, env.root) || under(dir, env.modCache) {
			continue
		}
		if _, dup := seen[dir]; dup {
			continue
		}
		seen[dir] = struct{}{}
		label := "import\x00" + importPath
		if under(dir, root) {
			label = "dir\x00" + relativeTo(root, dir)
		}
		deps = append(deps, dependency{name: label, dir: dir, importPath: importPath, instrumented: main})
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].name < deps[j].name })

	return depListing{deps: deps, linked: sortedKeys(linked), goMods: sortedKeys(goMods), ok: true}
}

// listArgs is a `go list` invocation under the build tags the test binaries
// are compiled with. A listing without them answers about another build: a
// dependency only a tagged file imports is missing from it, so it is neither
// instrumented nor hashed into Inputs, and a change to it is seen by nothing.
// Every listing behind the map goes through here for that reason.
func (c *Coverage) listArgs(args ...string) []string {
	out := []string{"list"}
	if c.buildTags != "" {
		out = append(out, "-tags", c.buildTags)
	}

	return append(out, args...)
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)

	return out
}

// hashDir hashes every regular file of a dependency's directory, by name and
// content, and the data below it.
//
// It is the whole directory rather than the Go files a build would select,
// because a build tag decides which of them that is and the tags can change
// between runs. Reading one extra file is cheaper than being wrong about which
// ones matter.
//
// The data below it — testdata, and the trees an //go:embed pattern reaches
// into — is read by the same rule the package's own fingerprint uses (see
// hashDataSubtrees), because an embedded file is compiled into the dependency
// without a line of it changing. Leaving it out was once only a missed
// re-map behind a moved build ID; now that an unchanged fingerprint and Inputs
// reuse the whole map, it would be a kept mapping the change invalidated.
//
// Results are kept for the run: a module's packages share dependencies, and
// hashing one twice would be the cost of mapping a second package that happens
// to import the same thing.
func (c *Coverage) hashDir(dir string) (string, bool) {
	if sum, done := c.dirHashes[dir]; done {
		return sum, sum != ""
	}
	sum, ok := hashDirectory(dir)
	if ok {
		var below string
		below, ok = hashDataSubtrees(dir)
		sum = hashOf([]byte(sum + "\x00subtrees\x00" + below))
	}
	if c.dirHashes == nil {
		c.dirHashes = map[string]string{}
	}
	if !ok {
		c.dirHashes[dir] = ""

		return "", false
	}
	c.dirHashes[dir] = sum

	return sum, true
}

func hashDirectory(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: a directory `go list` reported as a dependency
		if err != nil {
			return "", false
		}
		parts = append(parts, name+"\x00"+hashOf(data))
	}

	return hashOf([]byte(strings.Join(parts, "\x00"))), true
}

// goEnvironment is the part of the Go environment that decides what a build
// produces without appearing in any source file.
type goEnvironment struct {
	// build is every variable in buildEnvVars with its value, in that order.
	build    []string
	root     string
	modCache string
	version  string
	work     string
}

// buildEnvVars are the variables that change what a test binary is compiled to
// while every source file stays the same: the target platform and its
// micro-architecture levels, cgo and the C toolchain and flags it compiles
// with (a -D in CGO_CFLAGS changes what the C code does), toolchain
// experiments, the FIPS 140 module selection, and flags applied to every go
// command.
//
// GOWORK changes it too, but it is a path, so it is asked for separately and
// folded in relative to the module root (see buildInputsOf); its value as
// written would differ between any two checkouts that use a workspace.
//
// Each one has to be here because reusing a map across a change in it is
// wrong, not merely stale. A map made for amd64 says nothing about a file only
// an arm64 build compiles; one made without a tag GOFLAGS adds says nothing
// about the files the tag selects. And because the build ID moves with each of
// them, leaving one out is not a missed hit but a narrowed run that keeps
// mappings the change invalidated.
//
// A variable an older toolchain does not know reads as empty, consistently, so
// the list can be ahead of the go in use.
var buildEnvVars = []string{
	"GOOS", "GOARCH",
	"GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM",
	"CGO_ENABLED", "CC", "CXX", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_FFLAGS", "CGO_LDFLAGS",
	"GOEXPERIMENT", "GOFIPS140", "GOFLAGS",
}

// goEnv asks the toolchain about itself, once per run.
//
// It asks for JSON and reads only standard output: a variable can be empty,
// which a line-per-value answer cannot tell apart from a missing line, and go
// writes warnings to standard error that are not part of the answer.
func (c *Coverage) goEnv() (goEnvironment, bool) {
	if c.env != nil {
		return *c.env, c.env.version != ""
	}
	env := goEnvironment{}
	args := append([]string{"env", "-json", "GOROOT", "GOMODCACHE", "GOVERSION", "GOWORK"}, buildEnvVars...)
	out, err := c.cmdContext("go", args...).Output()
	var values map[string]string
	if err == nil && json.Unmarshal(out, &values) == nil {
		env = goEnvironment{
			root:     values["GOROOT"],
			modCache: values["GOMODCACHE"],
			version:  values["GOVERSION"],
			work:     values["GOWORK"],
		}
		for _, name := range buildEnvVars {
			env.build = append(env.build, name+"="+values[name])
		}
	}
	c.env = &env

	return env, env.version != ""
}

// under reports whether a path is inside a directory, by path rather than by
// inode: this decides whether something is the toolchain's or the module
// cache's, and both answers only have to be conservative in one direction —
// a directory wrongly treated as editable is hashed, which costs nothing but
// the reading.
func under(path, dir string) bool {
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
