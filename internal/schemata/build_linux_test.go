//go:build linux

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

package schemata_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-gremlins/gremlins/internal/schemata"
)

// slowModule writes a module whose one package takes several seconds to
// compile. The seed makes its source unique, so the build cache cannot
// answer for it.
func slowModule(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "package slow\n\nconst seed = %d\n\n", time.Now().UnixNano())
	for i := range 20000 {
		fmt.Fprintf(&b, "func F%d(a, b int) int {\n\tif a > b {\n\t\treturn a*%d + b\n\t}\n\n\treturn b - a + seed\n}\n\n", i, i)
	}
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":       "module slow\n\ngo 1.22\n",
		"slow.go":      b.String(),
		"slow_test.go": "package slow\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) { _ = F1(1, 2) }\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

// processesUnder lists the pids of running processes whose working
// directory or command line mentions dir.
func processesUnder(dir string) []string {
	var pids []string
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		pid := e.Name()
		if pid == "" || pid[0] < '0' || pid[0] > '9' {
			continue
		}
		cwd, _ := os.Readlink(filepath.Join("/proc", pid, "cwd"))
		cmdline, _ := os.ReadFile(filepath.Join("/proc", pid, "cmdline")) //nolint:gosec // G304: a /proc path
		if strings.HasPrefix(cwd, dir) || strings.Contains(string(cmdline), dir) {
			pids = append(pids, pid+" "+strings.ReplaceAll(string(cmdline), "\x00", " "))
		}
	}

	return pids
}

// TestBuildAllKillsTheWholeBuildAtTheDeadline cancels a build while the
// compiler is running -- the first build and the retry a build cut off gets
// -- and requires that no process of it survives and that it leaves no
// go-build work directory behind.
func TestBuildAllKillsTheWholeBuildAtTheDeadline(t *testing.T) {
	t.Parallel()
	modRoot := slowModule(t)
	workDir := t.TempDir()

	start := time.Now()
	_, errs := schemata.BuildAll(context.Background(), modRoot, workDir, "", nil, []string{"slow"}, time.Second)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("BuildAll returned after %s with a 1s timeout and one retry", elapsed)
	}
	if !errors.Is(errs["slow"], context.DeadlineExceeded) {
		t.Fatalf("error %v, want the deadline", errs["slow"])
	}

	// A killed process can take a moment to leave the process table.
	var left []string
	for deadline := time.Now().Add(500 * time.Millisecond); ; time.Sleep(50 * time.Millisecond) {
		if left = processesUnder(workDir); len(left) == 0 || time.Now().After(deadline) {
			break
		}
	}
	if len(left) > 0 {
		t.Errorf("processes of the cancelled build survive:\n%s", strings.Join(left, "\n"))
	}

	err := filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), "go-build") {
			t.Errorf("leftover work directory %s", path)

			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBuildAllRetriesASignalledGoCommand fails a package's first build with
// the exit of a process a signal killed, and requires it rebuilt once.
func TestBuildAllRetriesASignalledGoCommand(t *testing.T) {
	t.Parallel()
	killed := exec.Command("sh", "-c", "kill -KILL $$").Run()
	var exitErr *exec.ExitError
	if !errors.As(killed, &exitErr) || exitErr.ExitCode() != -1 {
		t.Fatalf("the killed shell returned %v, want an exit by signal", killed)
	}
	mod := twoPkgsModule(t)
	const pkg = "twopkgs/ok"
	f := &fakeBuilds{realOnNil: true, answer: func(_ context.Context, _ string, call int) error {
		if call == 1 {
			return fmt.Errorf("schemata: build %s: %w", pkg, killed)
		}

		return nil
	}}
	b, errs := schemata.BuildAllWith(context.Background(), mod.Root, t.TempDir(), "", nil, []string{pkg}, time.Minute, f.build)
	if errs[pkg] != nil || b.Binaries[pkg] == "" {
		t.Errorf("binary %q, error %v; want the retry to build it", b.Binaries[pkg], errs[pkg])
	}
	if c := f.callsOf(pkg); c != 2 {
		t.Errorf("built %d times, want 2", c)
	}
}
