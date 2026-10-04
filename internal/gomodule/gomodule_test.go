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

package gomodule_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-gremlins/gremlins/internal/gomodule"
)

func TestDetectsModule(t *testing.T) {
	t.Run("does not return error if it can retrieve module", func(t *testing.T) {
		const modName = "example.com"
		rootDir := t.TempDir()
		pkgDir := "pkgDir"
		absPkgDir := filepath.Join(rootDir, pkgDir)
		_ = os.MkdirAll(absPkgDir, 0600)
		goMod := filepath.Join(rootDir, "go.mod")
		err := os.WriteFile(goMod, []byte("module "+modName), 0600)
		if err != nil {
			t.Fatal(err)
		}

		mod, err := gomodule.Init(absPkgDir)
		if err != nil {
			t.Fatal(err)
		}

		if mod.Name != modName {
			t.Errorf("expected Go module to be %q, got %q", modName, mod.Name)
		}
		if mod.Root != rootDir {
			t.Errorf("expected Go root to be %q, got %q", rootDir, mod.Root)
		}
		if mod.CallingDir != pkgDir {
			t.Errorf("expected Go package dir to be %q, got %q", pkgDir, mod.CallingDir)
		}
	})

	t.Run("returns error if go.mod is invalid", func(t *testing.T) {
		path := t.TempDir()
		goMod := path + "/go.mod"
		err := os.WriteFile(goMod, []byte(""), 0600)
		if err != nil {
			t.Fatal(err)
		}

		_, err = gomodule.Init(path)
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("returns error if it cannot find module", func(t *testing.T) {
		_, err := gomodule.Init(t.TempDir())
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("returns error if path is empty", func(t *testing.T) {
		_, err := gomodule.Init("")
		if err == nil {
			t.Errorf("expected an error")
		}
	})
}

// TestPackagePatternsBecomeDirectories pins the reason sanitizeCallingDir
// exists: a `/...` suffix is a Go package pattern, and every consumer of
// CallingDir wants a directory. Stored verbatim, `./...` made the walk look for
// a directory named "...", find nothing, and report a passing run that examined
// no mutants at all.
func TestPackagePatternsBecomeDirectories(t *testing.T) {
	const modName = "example.com"

	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte("module "+modName), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rootDir, "pkgDir"), 0750); err != nil {
		t.Fatal(err)
	}

	testCases := map[string]struct {
		path string
		want string
	}{
		"a bare pattern at the module root is the root": {
			path: filepath.Join(rootDir, "..."),
			want: ".",
		},
		"a pattern under a package is that package": {
			path: filepath.Join(rootDir, "pkgDir", "..."),
			want: "pkgDir",
		},
		"a plain package directory is unchanged": {
			path: filepath.Join(rootDir, "pkgDir"),
			want: "pkgDir",
		},
		"the module root itself is unchanged": {
			path: rootDir,
			want: ".",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			mod, err := gomodule.Init(tc.path)
			if err != nil {
				t.Fatalf("Init(%q): %v", tc.path, err)
			}
			if mod.CallingDir != tc.want {
				t.Errorf("Init(%q).CallingDir = %q, want %q", tc.path, mod.CallingDir, tc.want)
			}
		})
	}
}
