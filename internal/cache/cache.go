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

// Package cache stores the verdict of a mutant under a key derived from the
// content of everything that verdict can depend on, so a later run over the
// same content reuses it instead of running the mutant's tests again.
//
// # What the key covers
//
// A mutant's verdict is what `go test <pkg>` reports for the mutated source.
// The key (KeyInputs) holds:
//
//   - the gremlins binary itself (its content hash), which fixes how a mutant
//     is applied and how the output is classified;
//   - the toolchain: the Go environment that changes what a test binary is;
//   - the environment every test binary inherits, every variable of it but
//     the shell's bookkeeping, GOTMPDIR and the ones --cache-env-ignore names:
//     an initialiser, a TestMain before testing.M.Run, the Go runtime and a
//     child process all read variables no test log records;
//   - a consumer-supplied salt for anything outside gremlins' view (--cache-salt);
//   - the test command's shape: build tags and -cpu (the run bound is not in
//     the key; see below);
//   - the mutated package's import path and the content of its `go test`
//     dependency closure, below;
//   - the mutation: the file, the mutator type, the position, and the content
//     of the mutated file as gremlins wrote it.
//
// The closure is every package `go list -deps -test <pkg>` lists outside the
// standard library. A package outside the module contributes the files that
// build it. A package inside the module contributes every file of its
// directory tree, stopping at a nested module, whatever the build reads of
// them, so data a package-level initialiser or a TestMain reads there is
// covered: the test log below starts only once testing.M.Run does. The test
// files of a package other than the mutated one are left out, because they
// are not part of the test binary, so editing them leaves the key of every
// mutant of the packages that import it alone. The module's go.mod and go.sum, and the
// workspace's go.work and go.work.sum when there is one, complete it.
//
// # What the entry covers
//
// A test can read files and environment variables the build never sees. The
// mutant's test binary is run with -test.testlogfile, the log `go test` itself
// uses for its result cache, which records every file the tests open or stat
// and every variable they read. The entry stores a digest of each of those
// inputs, and a lookup recomputes every one and misses on any difference. A
// path inside the module is recorded relative to it, wherever the test
// reached it from (the copy it ran in, or the module itself, through any
// symbolic link). A path under GOROOT, the module cache, the build cache or
// gremlins' own working directory is not an input: the key fixes the first
// three, and the last holds the copies and the build's scratch files. Any
// other path is recorded as it is, a missing one included, so a test's check
// that a path does not exist is an input too. A test's own scratch files
// under the temporary directory are recorded as well; their names are fresh
// on every run, so they are missing at every later lookup, as they were when
// the entry was stored.
//
// The digests an entry stores are taken from the copy of the module the test
// ran in, after the mutation was rolled back, and the entry is stored only
// when that copy's closure still has the key's digest: a module edited while
// the run was going cannot have its new content credited with a verdict
// reached on the old.
//
// What neither the key nor a log records stays outside the cache's view, and a
// consumer that depends on it must put it in the salt: files a package-level
// initialiser or a TestMain reads outside the closure's package trees, files
// a child process started by a test reads, anything reached over a network,
// and an ignored variable read before the log starts.
//
// # Which verdicts are stored
//
// The test binary also runs with -test.memprofile. testing.M writes that
// profile after it has flushed and closed the test log, in the same function,
// so a non-empty profile proves the log is complete. A binary that died before
// that point (a panic outside a test, os.Exit, a signal) may have lost the end
// of its log, and a partial input list could hide a changed input.
//
//   - KILLED, only with a complete log.
//   - LIVED, only with a complete log and a `go test` that exited 0.
//   - NOT VIABLE, only when `go test` reported a build failure with a
//     compiler diagnostic naming the mutated file: no test binary ran, and the
//     failure is the mutation's, so the key alone determines it. A setup
//     failure, or a build that failed without naming the file (a killed
//     compiler, a full disk), is the environment's and is not stored.
//   - TIMED OUT and RUN TIMED OUT are never stored. Both are a fact about the
//     producing run's clock as much as about the mutant, so every run asks again.
//     A shutdown status is never stored either: nothing adjudicated the mutant.
//
// The run bound is not in the key, because a consumer that measures it per run
// would otherwise never hit. A stored KILLED or LIVED verdict is therefore the
// verdict reached under the producing run's bound; a run with a much tighter
// bound could have timed the same mutant out. A run against the same content
// without the cache carries the same exposure between two machines.
//
// # Validation
//
// An entry is believed only when it parses, its schema and key match, its own
// digest matches its content, its status is one this package stores, and every
// recorded runtime input still has the recorded digest. Anything else is a
// miss, never an error: a corrupt or mismatched entry costs one test run. The
// digest detects corruption, not forgery; the cache directory is as trusted as
// the module it serves.
package cache

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// Schema names the layout of a key and an entry. A change to either changes it.
const Schema = "gremlins-mutant-cache/v2"

// testlogHeader is the first line of a `-test.testlogfile` log.
const testlogHeader = "# test log"

// The two ways an entry records a path: its content was read, or only its
// metadata was looked at.
const (
	opOpen = "open"
	opStat = "stat"
)

// toolchainEnv is the Go environment that changes what a test binary is.
var toolchainEnv = []string{
	"GOVERSION", "GOROOT", "GOOS", "GOARCH", "GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64",
	"GOPPC64", "GORISCV64", "GOWASM", "GOFIPS140", "GOEXPERIMENT", "GOFLAGS", "GOWORK",
	"CGO_ENABLED", "CC", "CXX", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_LDFLAGS",
}

// unkeyedEnv are the variables left out of the key whatever the consumer
// says: the shell's own bookkeeping, and GOTMPDIR, which gremlins sets for
// each run itself.
var unkeyedEnv = []string{"PWD", "OLDPWD", "SHLVL", "_", "GOTMPDIR"}

// cacheEnv are the Go environment's own locations, whose content the key
// already fixes; a test's reads under them are not inputs.
var cacheEnv = []string{"GOMODCACHE", "GOCACHE"}

// storable are the statuses an entry may carry.
var storable = []mutator.Status{mutator.Killed, mutator.Lived, mutator.NotViable}

// KeyInputs is everything a key is derived from. Every field enters the key.
type KeyInputs struct {
	Schema    string
	Gremlins  string
	Toolchain string
	Env       string
	Salt      string
	Tags      string
	TestCPU   int
	Package   string
	Closure   string
	File      string
	Mutator   string
	Line      int
	Column    int
	Mutated   string
}

// Key returns the hex SHA-256 of the canonical encoding of k.
func (k KeyInputs) Key() string {
	b, _ := json.Marshal(k)

	return digest(b)
}

// Input is one runtime input of an entry: a path, relative to the module root
// when it lies inside the module and absolute otherwise, how the test touched
// it, and the digest of what it found.
type Input struct {
	Op     string `json:"op"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// EnvInput is one environment variable a test read. The value is stored as a
// digest, so an entry never carries a secret the test happened to read.
type EnvInput struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// Entry is what the cache stores for one mutant.
type Entry struct {
	Schema string     `json:"schema"`
	Key    string     `json:"key"`
	Status string     `json:"status"`
	Inputs []Input    `json:"inputs"`
	Env    []EnvInput `json:"env"`
	Digest string     `json:"digest"`
}

func (e Entry) contentDigest() string {
	e.Digest = ""
	b, _ := json.Marshal(e)

	return digest(b)
}

// Evidence is what a finished test run tells the cache about its verdict.
type Evidence struct {
	// Testlog is the -test.testlogfile the binary wrote, if any.
	Testlog string
	// LogComplete is true when the binary got past flushing and closing the
	// test log, which the -test.memprofile it then writes proves.
	LogComplete bool
	// WorkRoot is the copy of the module the mutant ran in, and PkgDir the
	// package directory inside it that the test binary started in.
	WorkRoot string
	PkgDir   string
	// ScratchRoot is gremlins' own working directory, which holds the copies
	// and the build's temporary files; nothing under it is an input.
	ScratchRoot string
	// Env is the environment the test binary was given.
	Env []string
	// Adjudicated is true when the status came from the test run itself, not
	// from a deadline or a shutdown.
	Adjudicated bool
	// ExitedZero is true when `go test` exited 0.
	ExitedZero bool
	// BuildFailed is true when `go test` reported a build failure with a
	// compiler diagnostic naming the mutated file.
	BuildFailed bool
}

// Cache is a verdict cache rooted at a directory.
type Cache struct {
	dir     string
	modRoot string
	base    KeyInputs
	// modRoots is modRoot in its literal and resolved forms.
	modRoots []string
	// gowork is the workspace file the go command uses, if any.
	gowork string
	// notInputs are directories whose content the key fixes, each in its
	// literal and resolved forms.
	notInputs []string

	mu       sync.Mutex
	closures map[string]closureResult

	hits   atomic.Int64
	stores atomic.Int64
}

type closureResult struct {
	files  []closureFile
	digest string
	err    error
}

// closureFile is one file of a closure: its name in the digest, and where it
// is. rel is set for a file inside the module, relative to its root, so the
// same closure can be read from a copy of the module.
type closureFile struct {
	name string
	rel  string
	abs  string
}

// Options configures New.
type Options struct {
	// Dir is where entries are kept.
	Dir string
	// ModRoot is the root of the module under test.
	ModRoot string
	// Salt is anything the consumer wants to enter every key.
	Salt string
	// Tags and TestCPU are the test command's shape.
	Tags    string
	TestCPU int
	// Env is the environment every test binary inherits.
	Env []string
	// EnvIgnore names variables left out of the key: a name, or a prefix
	// ending in `*`. A test's own reads of them are still recorded.
	EnvIgnore []string
}

// New returns a Cache. It fails when the toolchain or the gremlins binary
// cannot be identified, because a key without them could not tell two
// toolchains or two gremlins builds apart.
func New(o Options) (*Cache, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot locate the gremlins binary: %w", err)
	}
	self, err := fileDigest(exe)
	if err != nil {
		return nil, fmt.Errorf("cannot hash the gremlins binary: %w", err)
	}
	toolchain, err := goEnv(o.ModRoot, toolchainEnv)
	if err != nil {
		return nil, err
	}
	locations, err := goEnv(o.ModRoot, cacheEnv)
	if err != nil {
		return nil, err
	}
	var where map[string]string
	if err := json.Unmarshal(locations, &where); err != nil {
		return nil, fmt.Errorf("cannot read the Go environment: %w", err)
	}
	var tc map[string]string
	if err := json.Unmarshal(toolchain, &tc); err != nil {
		return nil, fmt.Errorf("cannot read the Go environment: %w", err)
	}
	if err := os.MkdirAll(o.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("cannot create the cache directory: %w", err)
	}
	var notInputs []string
	for _, d := range []string{tc["GOROOT"], where["GOMODCACHE"], where["GOCACHE"]} {
		if d != "" {
			notInputs = append(notInputs, pathForms(d)...)
		}
	}

	return &Cache{
		dir:     o.Dir,
		modRoot: o.ModRoot,
		base: KeyInputs{
			Schema:    Schema,
			Gremlins:  self,
			Toolchain: digest(toolchain),
			Env:       envKey(o.Env, append(append([]string{}, unkeyedEnv...), o.EnvIgnore...)),
			Salt:      o.Salt,
			Tags:      o.Tags,
			TestCPU:   o.TestCPU,
		},
		modRoots:  pathForms(o.ModRoot),
		gowork:    workspaceFile(tc["GOWORK"]),
		notInputs: notInputs,
		closures:  map[string]closureResult{},
	}, nil
}

// envKey digests every variable of env that ignore does not name. The
// variables an initialiser, a TestMain, the Go runtime or a child process
// reads leave no trace in a test log, so the key holds them all.
func envKey(env, ignore []string) string {
	vals := map[string]string{}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && !envIgnored(k, ignore) {
			vals[k] = v
		}
	}
	names := make([]string, 0, len(vals))
	for n := range vals {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s %s\n", n, digest([]byte(vals[n])))
	}

	return digest([]byte(b.String()))
}

func envIgnored(name string, ignore []string) bool {
	for _, p := range ignore {
		if prefix, ok := strings.CutSuffix(p, "*"); (ok && strings.HasPrefix(name, prefix)) || p == name {
			return true
		}
	}

	return false
}

func goEnv(dir string, names []string) ([]byte, error) {
	//nolint:gosec // the go command with a fixed argument list
	cmd := exec.Command("go", append([]string{"env", "-json"}, names...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot read the Go environment: %w", err)
	}

	return out, nil
}

// Hits is the number of verdicts served from the cache so far.
func (c *Cache) Hits() int64 { return c.hits.Load() }

// Stores is the number of verdicts written to the cache so far.
func (c *Cache) Stores() int64 { return c.stores.Load() }

// Key derives the key of a mutant. relFile is the mutated file's path relative
// to the module root, and mutated is its content as gremlins wrote it.
func (c *Cache) Key(pkg, relFile string, mt mutator.Type, line, column int, mutated []byte) (string, error) {
	cl := c.closure(pkg)
	if cl.err != nil {
		return "", cl.err
	}
	k := c.base
	k.Package = pkg
	k.Closure = cl.digest
	k.File = filepath.ToSlash(relFile)
	k.Mutator = mt.String()
	k.Line = line
	k.Column = column
	k.Mutated = digest(mutated)

	return k.Key(), nil
}

// Lookup returns the stored verdict for key when the entry validates against
// the current module and env, the environment the test binary would be given.
func (c *Cache) Lookup(key string, env []string) (mutator.Status, bool) {
	raw, err := os.ReadFile(c.path(key))
	if err != nil {
		return 0, false
	}
	status, ok := c.validate(raw, key, env)
	if ok {
		c.hits.Add(1)
	}

	return status, ok
}

func (c *Cache) validate(raw []byte, key string, env []string) (mutator.Status, bool) {
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		return 0, false
	}
	if e.Schema != Schema || e.Key != key || e.Digest != e.contentDigest() {
		return 0, false
	}
	status, ok := storedStatus(e.Status)
	if !ok {
		return 0, false
	}
	for _, in := range e.Inputs {
		if inputDigest(c.modRoot, in.Op, in.Path) != in.Digest {
			return 0, false
		}
	}
	for _, v := range e.Env {
		if envDigest(env, v.Name) != v.Digest {
			return 0, false
		}
	}

	return status, true
}

// Store records status for key, the key of a mutant of pkg, when the rules in
// the package documentation allow it, and reports whether it did. It is called
// once the mutation has been rolled back in ev.WorkRoot.
func (c *Cache) Store(key, pkg string, status mutator.Status, ev Evidence) (bool, error) {
	if !ev.Adjudicated {
		return false, nil
	}
	e := Entry{Schema: Schema, Key: key, Status: status.String(), Inputs: []Input{}, Env: []EnvInput{}}
	switch status {
	case mutator.NotViable:
		if !ev.BuildFailed {
			return false, nil
		}
	case mutator.Killed, mutator.Lived:
		if !ev.LogComplete || ev.ExitedZero != (status == mutator.Lived) {
			return false, nil
		}
		inputs, env, ok := c.runtimeInputs(ev)
		if !ok {
			return false, nil
		}
		e.Inputs, e.Env = inputs, env
	case mutator.NotCovered, mutator.Runnable, mutator.Skipped, mutator.TimedOut, mutator.RunTimedOut:
		return false, nil
	default:
		// A status added to mutator after this switch was written is not
		// stored until a rule here says it may be.
		return false, nil
	}
	if !c.ranAgainstKey(pkg, ev.WorkRoot) {
		return false, nil
	}
	e.Digest = e.contentDigest()
	b, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	if err := writeAtomic(c.path(key), b); err != nil {
		return false, err
	}
	c.stores.Add(1)

	return true, nil
}

// ranAgainstKey reports whether the copy the test ran in holds the closure
// content the key was derived from.
func (c *Cache) ranAgainstKey(pkg, workRoot string) bool {
	cl := c.closure(pkg)
	if cl.err != nil {
		return false
	}
	d, err := closureDigest(cl.files, workRoot)

	return err == nil && d == cl.digest
}

// storedStatus maps an entry's status back, accepting only the ones Store writes.
func storedStatus(s string) (mutator.Status, bool) {
	for _, st := range storable {
		if st.String() == s {
			return st, true
		}
	}

	return 0, false
}

// runtimeInputs reads the test log into the inputs an entry records, with
// their digests taken from ev.WorkRoot. It reports false when there is no
// complete log to read.
func (c *Cache) runtimeInputs(ev Evidence) ([]Input, []EnvInput, bool) {
	if ev.Testlog == "" {
		return nil, nil, false
	}
	f, err := os.Open(ev.Testlog)
	if err != nil {
		return nil, nil, false
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() || sc.Text() != testlogHeader {
		return nil, nil, false
	}
	paths := map[Input]bool{}
	names := map[string]bool{}
	pwd := ev.PkgDir
	copyRoots := pathForms(ev.WorkRoot)
	var scratch []string
	if ev.ScratchRoot != "" {
		scratch = pathForms(ev.ScratchRoot)
	}
	for sc.Scan() {
		op, name, ok := strings.Cut(sc.Text(), " ")
		if !ok {
			return nil, nil, false
		}
		switch op {
		case "getenv":
			names[name] = true

			continue
		case opOpen, opStat, "chdir":
		default:
			return nil, nil, false
		}
		abs := name
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(pwd, abs)
		}
		if op == "chdir" {
			pwd = abs
			op = opStat
		}
		if p, ok := c.inputPath(abs, copyRoots, scratch); ok {
			paths[Input{Op: op, Path: p}] = true
		}
	}
	if sc.Err() != nil {
		return nil, nil, false
	}

	inputs := make([]Input, 0, len(paths))
	for in := range paths {
		in.Digest = inputDigest(ev.WorkRoot, in.Op, in.Path)
		inputs = append(inputs, in)
	}
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Path != inputs[j].Path {
			return inputs[i].Path < inputs[j].Path
		}

		return inputs[i].Op < inputs[j].Op
	})
	env := make([]EnvInput, 0, len(names))
	for n := range names {
		env = append(env, EnvInput{Name: n, Digest: envDigest(ev.Env, n)})
	}
	sort.Slice(env, func(i, j int) bool { return env[i].Name < env[j].Name })

	return inputs, env, true
}

// inputPath names the input at abs: relative to the module when abs lies in
// the copy the test ran in or in the module itself, in any of their forms;
// nothing when it lies under a directory the key fixes or under gremlins' own
// scratch directory; and abs itself, resolved, otherwise.
func (c *Cache) inputPath(abs string, copyRoots, scratch []string) (string, bool) {
	for _, roots := range [][]string{copyRoots, c.modRoots} {
		if rel, ok := relUnder(roots, abs); ok {
			return filepath.ToSlash(rel), true
		}
	}
	for _, roots := range [][]string{c.notInputs, scratch} {
		if _, ok := relUnder(roots, abs); ok {
			return "", false
		}
	}
	forms := pathForms(abs)

	return forms[len(forms)-1], true
}

// relUnder reports whether p, in any of its forms, lies under one of roots,
// and p relative to that root.
func relUnder(roots []string, p string) (string, bool) {
	forms := pathForms(p)
	for _, root := range roots {
		for _, f := range forms {
			if rel, ok := under(root, f); ok {
				return rel, true
			}
		}
	}

	return "", false
}

// pathForms is p cleaned and, when it differs, p with its symbolic links
// resolved: the path itself when it exists, else its nearest existing parent
// with the rest appended. The resolved form is last.
func pathForms(p string) []string {
	p = filepath.Clean(p)
	rest := ""
	for dir := p; ; {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			resolved := filepath.Join(r, rest)
			if resolved == p {
				return []string{p}
			}

			return []string{p, resolved}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return []string{p}
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

// under reports whether p is root or lies below it, and p relative to root.
func under(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return rel, true
}

// inputDigest describes what a test would find at path: an absolute path as
// it is, a relative one under root. An open file is its content, an open
// directory its listing, and a stat the kind, size and permissions; a missing
// path, and any file that is not regular, are values of their own, so a device
// or a pipe is never read.
func inputDigest(root, op, path string) string {
	p := filepath.FromSlash(path)
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	info, err := os.Stat(p)
	if err != nil {
		return "missing"
	}
	switch {
	case info.IsDir() && op == opStat:
		return fmt.Sprintf("dir %v", info.Mode())
	case info.IsDir():
		entries, err := os.ReadDir(p)
		if err != nil {
			return "unreadable"
		}
		var b strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&b, "%s %v\n", e.Name(), e.Type())
		}

		return "dir " + digest([]byte(b.String()))
	case !info.Mode().IsRegular():
		return fmt.Sprintf("special %v", info.Mode())
	case op == opStat:
		return fmt.Sprintf("file %d %v", info.Size(), info.Mode())
	}
	d, err := fileDigest(p)
	if err != nil {
		return "unreadable"
	}

	return "file " + d
}

// envDigest is the digest of name's value in env, the last assignment winning
// as it does for a child process; an unset variable is a value of its own.
func envDigest(env []string, name string) string {
	val, set := "", false
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			val, set = v, true
		}
	}
	if !set {
		return "unset"
	}

	return digest([]byte(val))
}

// listedPackage is the part of `go list -json` the closure reads.
type listedPackage struct {
	ImportPath string
	Dir        string
	Standard   bool

	GoFiles, CgoFiles, CFiles, CXXFiles, MFiles, HFiles, FFiles, SFiles []string
	SwigFiles, SwigCXXFiles, SysoFiles, EmbedFiles                      []string
}

// buildFiles are the files that build the package into a test binary.
func (p listedPackage) buildFiles() []string {
	var out []string
	for _, l := range [][]string{
		p.GoFiles, p.CgoFiles, p.CFiles, p.CXXFiles, p.MFiles, p.HFiles, p.FFiles, p.SFiles,
		p.SwigFiles, p.SwigCXXFiles, p.SysoFiles, p.EmbedFiles,
	} {
		out = append(out, l...)
	}

	return out
}

const listFields = "ImportPath,Dir,Standard,GoFiles,CgoFiles,CFiles,CXXFiles,MFiles,HFiles,FFiles,SFiles," +
	"SwigFiles,SwigCXXFiles,SysoFiles,EmbedFiles"

func (c *Cache) closure(pkg string) closureResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.closures[pkg]; ok {
		return r
	}
	r := closureResult{}
	r.files, r.err = c.listClosure(pkg)
	if r.err == nil {
		r.digest, r.err = closureDigest(r.files, c.modRoot)
	}
	c.closures[pkg] = r

	return r
}

// listClosure lists the files of pkg's `go test` dependency closure, as the
// package documentation describes it. Each file is named by its package's
// import path, never by its directory, so the same content in a module cache
// at a different path gives one key.
func (c *Cache) listClosure(pkg string) ([]closureFile, error) {
	args := []string{"list", "-deps", "-test", "-json=" + listFields}
	if c.base.Tags != "" {
		args = append(args, "-tags", c.base.Tags)
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	cmd.Dir = c.modRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list %s: %w", pkg, err)
	}

	var pkgDir string
	var listed []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listedPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list %s: %w", pkg, err)
		}
		if p.ImportPath == pkg {
			pkgDir = p.Dir
		}
		listed = append(listed, p)
	}
	if pkgDir == "" {
		return nil, fmt.Errorf("go list %s: the package itself is not listed", pkg)
	}

	files := map[string]closureFile{}
	add := func(f closureFile) {
		if _, seen := files[f.name]; !seen {
			files[f.name] = f
		}
	}
	for _, p := range listed {
		if p.Standard {
			continue
		}
		name, _, _ := strings.Cut(p.ImportPath, " ")
		relDir, inModule := relUnder(c.modRoots, p.Dir)
		if !inModule {
			for _, f := range p.buildFiles() {
				// An absolute path is a file the go command generated itself,
				// such as the test main in its build cache; its content follows
				// from the files listed beside it.
				if !filepath.IsAbs(f) {
					add(closureFile{name: name + "/" + f, abs: filepath.Join(p.Dir, f)})
				}
			}

			continue
		}
		dirFiles, err := packageDirFiles(p.Dir, p.Dir == pkgDir)
		if err != nil {
			return nil, err
		}
		for _, f := range dirFiles {
			add(closureFile{name: name + "/" + filepath.ToSlash(f), rel: filepath.Join(relDir, f)})
		}
	}
	for _, f := range []string{"go.mod", "go.sum"} {
		add(closureFile{name: "module/" + f, rel: f})
	}
	if c.gowork != "" {
		add(closureFile{name: "workspace/go.work", abs: c.gowork})
		add(closureFile{name: "workspace/go.work.sum", abs: c.gowork + ".sum"})
	}

	out2 := make([]closureFile, 0, len(files))
	for _, f := range files {
		out2 = append(out2, f)
	}
	sort.Slice(out2, func(i, j int) bool { return out2[i].name < out2[j].name })

	return out2, nil
}

// packageDirFiles lists, relative to dir, every regular file in dir's tree,
// stopping at a nested module. Test files are left out unless withTests is
// set.
func packageDirFiles(dir string, withTests bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir {
				if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
					return fs.SkipDir
				}
			}

			return nil
		}
		if !d.Type().IsRegular() || (!withTests && strings.HasSuffix(d.Name(), "_test.go")) {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, rel)

		return nil
	})

	return out, err
}

// workspaceFile is the workspace file named by GOWORK, if there is one.
func workspaceFile(gowork string) string {
	if gowork == "off" {
		return ""
	}

	return gowork
}

// closureDigest hashes the content of files, reading a file inside the module
// under root.
func closureDigest(files []closureFile, root string) (string, error) {
	var b strings.Builder
	for _, f := range files {
		p := f.abs
		if f.rel != "" {
			p = filepath.Join(root, f.rel)
		}
		d, err := fileDigest(p)
		if errors.Is(err, fs.ErrNotExist) {
			d = "missing"
		} else if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s %s\n", f.name, d)
	}

	return digest([]byte(b.String())), nil
}

func (c *Cache) path(key string) string {
	return filepath.Join(c.dir, key[:2], key+".json")
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".entry-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())

		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())

		return err
	}

	return os.Rename(tmp.Name(), path)
}

func digest(b []byte) string {
	s := sha256.Sum256(b)

	return hex.EncodeToString(s[:])
}

func fileDigest(path string) (string, error) {
	//nolint:gosec // paths come from go list, the test log and the module tree
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
