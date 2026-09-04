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

func TestInitSanitizesCallingDir(t *testing.T) {
	const modName = "example.com"
	rootDir := t.TempDir()
	pkgDir := filepath.Join("internal", "somepkg")
	absPkgDir := filepath.Join(rootDir, pkgDir)
	if err := os.MkdirAll(absPkgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte("module "+modName), 0o600); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "clean subdir",
			path: absPkgDir,
			want: pkgDir,
		},
		{
			name: "subdir with package pattern",
			path: absPkgDir + string(filepath.Separator) + "...",
			want: pkgDir,
		},
		{
			name: "nested pattern is stripped",
			path: filepath.Join(rootDir, "internal", "somepkg", "..."),
			want: pkgDir,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mod, err := gomodule.Init(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if mod.CallingDir != tc.want {
				t.Errorf("expected CallingDir to be %q, got %q", tc.want, mod.CallingDir)
			}
		})
	}

	t.Run("name ending with dots is preserved", func(t *testing.T) {
		dotDir := filepath.Join(rootDir, "foo...")
		if err := os.MkdirAll(dotDir, 0o750); err != nil {
			t.Fatal(err)
		}
		mod, err := gomodule.Init(dotDir)
		if err != nil {
			t.Fatal(err)
		}
		if mod.CallingDir != "foo..." {
			t.Errorf("expected CallingDir to be %q, got %q", "foo...", mod.CallingDir)
		}
	})
}
