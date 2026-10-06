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

package schemata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-gremlins/gremlins/internal/engine/workdir"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/procgroup"
)

// ErrNoTestBinary reports a package whose `go test -c` succeeded without
// writing a binary: it has no test files.
var ErrNoTestBinary = errors.New("schemata: package has no test binary (no test files)")

// schemaDirID is the dealer identifier of the schema copy.
const schemaDirID = "schemata"

// waitDelay bounds how long a killed `go test -c` may hold its output open
// through the compiler processes it started.
const waitDelay = 5 * time.Second

// Build is the schema build of a module: a pristine copy of it, the
// rewritten files laid over the copy by the go command's -overlay, and the
// test binaries built that way. Building through the overlay keeps every
// path the binaries embed -- what runtime.Caller reports -- naming a file of
// the copy, which holds the original source, as it does without schemata.
type Build struct {
	// Dir is the pristine copy of the module the binaries were built in.
	Dir string
	// Src holds each file Rewritten lists, at its path relative to the
	// module root: the content the overlay lays over the module's file.
	Src string
	// Binaries maps the import path of each package that built to its test
	// binary.
	Binaries map[string]string
	// Rewritten lists, sorted and relative to the module root, every file
	// the copy rewrites or adds.
	Rewritten []string
	// NoTests holds each package that built without test files: it has no
	// binary, and its run passes without reaching any mutant.
	NoTests map[string]bool
}

// BuildAll copies the module at modRoot into workDir, writes the rewritten
// files beside the copy -- rewritten maps a package's import path to the
// files RewritePackage returned for it, keyed by absolute path under modRoot
// -- and compiles the test binary of every package in testPkgs in the copy
// with `go test -c -overlay`, which lays the rewritten files over the copy's,
// at most runtime.NumCPU() at a time, all within timeout (a path listed
// twice is built once). A package whose build was cut off -- by timeout, a
// signal or running out of memory -- is built once more, at most half as
// many at a time, within a fresh timeout; a package whose source does not
// compile is not. A package whose files cannot be written or whose binary
// does not build has its error in the returned map and no binary; the other
// packages keep theirs. A build error's first line says whether the package
// does not compile, timed out or was killed, and whether that was after a
// retry. A package that builds without test files is in Build.NoTests, with
// neither. If the module cannot be copied, every package has that error.
func BuildAll(ctx context.Context, modRoot, workDir, tags string, rewritten map[string]map[string][]byte,
	testPkgs []string, timeout time.Duration,
) (Build, map[string]error) {
	return buildAll(ctx, modRoot, workDir, tags, rewritten, testPkgs, timeout, buildTest)
}

// buildFunc compiles the test binary of pkg, in the module copy dir with the
// overlay file laid over it, to bin, with goTmp as the go command's GOTMPDIR:
// buildTest's signature, and the seam the tests replace it through.
type buildFunc func(ctx context.Context, dir, goTmp, overlay, bin, tags, pkg string) error

// buildAll is BuildAll with the build of each package done by build.
func buildAll(ctx context.Context, modRoot, workDir, tags string, rewritten map[string]map[string][]byte,
	testPkgs []string, timeout time.Duration, build buildFunc,
) (Build, map[string]error) {
	// A path listed twice would start two `go test -c` writing one binary.
	testPkgs = uniquePackages(testPkgs)
	errs := map[string]error{}
	failAll := func(err error) (Build, map[string]error) {
		for _, p := range testPkgs {
			errs[p] = err
		}

		return Build{}, errs
	}
	// Absolute: the overlay's keys and values must not depend on the
	// directory the go command runs in.
	workDir, err := filepath.Abs(workDir)
	if err != nil {
		return failAll(fmt.Errorf("schemata: work dir: %w", err))
	}
	dir, err := workdir.NewCachedDealer(workDir, modRoot).Get(schemaDirID)
	if err != nil {
		return failAll(fmt.Errorf("schemata: copy %s: %w", modRoot, err))
	}
	binDir, err := os.MkdirTemp(workDir, "schemata-bin-*")
	if err != nil {
		return failAll(fmt.Errorf("schemata: binary dir: %w", err))
	}

	// The go command's work directories go here rather than the default
	// TMPDIR, so that a build killed at the deadline -- which cannot clean
	// up after itself -- leaves nothing outside workDir, and this function
	// can remove what it left.
	goTmp, err := os.MkdirTemp(workDir, "schemata-gotmp-*")
	if err != nil {
		return failAll(fmt.Errorf("schemata: build temp dir: %w", err))
	}
	defer func() { _ = os.RemoveAll(goTmp) }()

	src, err := os.MkdirTemp(workDir, "schemata-src-*")
	if err != nil {
		return failAll(fmt.Errorf("schemata: source dir: %w", err))
	}
	b := Build{Dir: dir, Src: src, Binaries: map[string]string{}, NoTests: map[string]bool{}}
	for _, p := range slices.Sorted(maps.Keys(rewritten)) {
		rels, err := writeFiles(modRoot, src, rewritten[p])
		b.Rewritten = append(b.Rewritten, rels...)
		if err != nil {
			errs[p] = err
		}
	}
	slices.Sort(b.Rewritten)
	overlay, err := writeOverlay(workDir, dir, src, b.Rewritten)
	if err != nil {
		return failAll(err)
	}

	names := binaryNames(testPkgs)
	var mu sync.Mutex
	// round builds pkgs, at most parallel at a time, all within timeout,
	// recording each binary in b and returning each failure.
	round := func(pkgs []string, parallel int) map[string]error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		failed := map[string]error{}
		var wg sync.WaitGroup
		sem := make(chan struct{}, parallel)
		for _, p := range pkgs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				bin := filepath.Join(binDir, names[p])
				err := build(ctx, dir, goTmp, overlay, bin, tags, p)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case errors.Is(err, ErrNoTestBinary):
					b.NoTests[p] = true
				case err != nil:
					failed[p] = err
				default:
					b.Binaries[p] = bin
				}
			}()
		}
		wg.Wait()

		return failed
	}

	var todo []string
	for _, p := range testPkgs {
		if errs[p] == nil {
			todo = append(todo, p)
		}
	}
	// A build cut off -- at the deadline, by a signal, by running out of
	// memory -- says nothing about the source, only about the machine: it
	// gets one more go, at half the parallelism, with a fresh bound. A
	// compile error would only recur. Once the run itself has ended, nothing
	// built would be used, so nothing is retried and the error stays the
	// run's.
	first := round(todo, runtime.NumCPU())
	var retry []string
	for _, p := range slices.Sorted(maps.Keys(first)) {
		err := first[p]
		switch kind := classifyBuild(err); {
		case ctx.Err() != nil:
			errs[p] = err
		case kind == buildDoesNotCompile:
			errs[p] = &buildError{pkg: p, kind: kind, err: err}
		default:
			retry = append(retry, p)
		}
	}
	if len(retry) == 0 {
		return b, errs
	}
	log.Infof("schemata: rebuilding %d packages whose build was cut off\n", len(retry))
	for p, err := range round(retry, max(1, runtime.NumCPU()/2)) {
		if ctx.Err() != nil {
			errs[p] = err

			continue
		}
		errs[p] = &buildError{pkg: p, kind: classifyBuild(err), retried: true, err: err}
	}

	return b, errs
}

// buildFailure is why a package's test binary did not build.
type buildFailure int

const (
	// buildDoesNotCompile is a failure the source caused: retrying it
	// would only fail again.
	buildDoesNotCompile buildFailure = iota
	// buildTimedOut is a build its bound cut off.
	buildTimedOut
	// buildKilled is a build a signal stopped, or one that ran out of
	// memory.
	buildKilled
)

func (k buildFailure) String() string {
	switch k {
	case buildTimedOut:
		return "timed out"
	case buildKilled:
		return "was killed"
	case buildDoesNotCompile:
	}

	return "does not compile"
}

// classifyBuild says why the build that returned err failed: its bound
// ended (the context's error), or the go command or a compiler or linker it
// started was stopped -- by a signal, which the go command reports as
// "signal: killed" for a child, or by running out of memory, which shows in
// the output -- or else the source does not compile.
func classifyBuild(err error) buildFailure {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return buildTimedOut
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == -1 {
		return buildKilled
	}
	if msg := err.Error(); strings.Contains(msg, "signal: killed") || showsOutOfMemory(msg) {
		return buildKilled
	}

	return buildDoesNotCompile
}

// outOfMemoryMarkers are what a process prints when it runs out of memory:
// the Go runtime when an allocation fails ("out of memory", "cannot allocate
// memory") or when it cannot reserve its start-up address space ("failed to
// reserve ...", "cannot reserve ..."), and the dynamic loader when it cannot
// map a program at all ("failed to map segment").
//
// They live here, with the build that reads them, rather than with any cap
// on memory: a build runs out of memory without one, on a machine that a
// parallel build of many packages has filled, and such a build says nothing
// about the source.
var outOfMemoryMarkers = []string{
	"out of memory",
	"cannot allocate memory",
	"failed to reserve",
	"cannot reserve",
	"failed to map segment",
}

// showsOutOfMemory reports whether s holds one of outOfMemoryMarkers.
func showsOutOfMemory(s string) bool {
	for _, m := range outOfMemoryMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}

	return false
}

// buildError is the failure of a package's build, with why it failed on its
// first line -- the line a netted mutant's log line carries -- and the
// build's own error, holding its output, below.
type buildError struct {
	pkg     string
	kind    buildFailure
	retried bool
	err     error
}

func (e *buildError) Error() string {
	retried := ""
	if e.retried {
		retried = " after a retry"
	}

	return fmt.Sprintf("schemata: build %s %s%s\n%v", e.pkg, e.kind, retried, e.err)
}

func (e *buildError) Unwrap() error { return e.err }

// uniquePackages returns pkgs without repeats, each at its first position.
func uniquePackages(pkgs []string) []string {
	seen := make(map[string]bool, len(pkgs))
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}

	return out
}

// writeFiles writes files, keyed by absolute path under modRoot, to the same
// places under dir, and returns their paths relative to the module root.
func writeFiles(modRoot, dir string, files map[string][]byte) ([]string, error) {
	var rels []string
	for _, path := range slices.Sorted(maps.Keys(files)) {
		rel, err := filepath.Rel(modRoot, path)
		if err != nil || !filepath.IsAbs(path) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rels, fmt.Errorf("schemata: %s is not a file under %s", path, modRoot)
		}
		dst := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return rels, fmt.Errorf("schemata: write %s: %w", rel, err)
		}
		if err := os.WriteFile(dst, files[path], 0o600); err != nil {
			return rels, fmt.Errorf("schemata: write %s: %w", rel, err)
		}
		rels = append(rels, rel)
	}

	return rels, nil
}

// writeOverlay writes, into workDir, the go command's overlay file that lays
// each file rels names in src over the same file in the module copy dir, and
// returns its path.
func writeOverlay(workDir, dir, src string, rels []string) (string, error) {
	replace := make(map[string]string, len(rels))
	for _, rel := range rels {
		replace[filepath.Join(dir, rel)] = filepath.Join(src, rel)
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err != nil {
		return "", fmt.Errorf("schemata: overlay: %w", err)
	}
	f, err := os.CreateTemp(workDir, "schemata-build-overlay-*.json")
	if err != nil {
		return "", fmt.Errorf("schemata: overlay: %w", err)
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("schemata: overlay: %w", err)
	}

	return f.Name(), nil
}

// buildTest compiles the test binary of pkg, in the module copy dir with the
// overlay file laid over it, to bin, with goTmp as the go command's GOTMPDIR. The go command runs in its own
// process group, and the deadline kills the whole group: killing only the go
// command would leave its compile and link processes running, competing with
// whatever runs next.
func buildTest(ctx context.Context, dir, goTmp, overlay, bin, tags, pkg string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("schemata: build %s: %w", pkg, err)
	}
	args := []string{"test", "-c", "-vet=off", "-overlay=" + overlay, "-o", bin}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, pkg)
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // G204: a fixed tool, the package list is the caller's
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTMPDIR="+goTmp)
	procgroup.Setup(cmd)
	cmd.Cancel = func() error { return procgroup.Kill(cmd) }
	cmd.WaitDelay = waitDelay
	out, err := cmd.CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("schemata: build %s: %w\n%s", pkg, ctxErr, out)
	}
	if err != nil {
		return fmt.Errorf("schemata: build %s: %w\n%s", pkg, err, out)
	}
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("%w: %s", ErrNoTestBinary, pkg)
	}

	return nil
}

// binaryNames gives each import path a distinct file name for its test
// binary: the path with every character outside [A-Za-z0-9._-] replaced by
// '_', suffixed with a counter when two paths sanitise alike.
func binaryNames(pkgs []string) map[string]string {
	sorted := slices.Clone(pkgs)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	names := make(map[string]string, len(sorted))
	used := map[string]bool{}
	for _, p := range sorted {
		base := strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
				return r
			}

			return '_'
		}, p)
		name := base + ".test"
		for i := 2; used[name]; i++ {
			name = base + "-" + strconv.Itoa(i) + ".test"
		}
		used[name] = true
		names[p] = name
	}

	return names
}

// BuildTimeout is the bound on the whole schema build of pkgs packages:
// configured when it is positive, otherwise allowance, the per-mutant
// compile allowance, for each package, but never under minBuildTimeout.
func BuildTimeout(configured, allowance time.Duration, pkgs int) time.Duration {
	if configured > 0 {
		return configured
	}

	return max(allowance*time.Duration(pkgs), minBuildTimeout)
}

// minBuildTimeout is the least bound BuildTimeout derives.
const minBuildTimeout = 10 * time.Minute
