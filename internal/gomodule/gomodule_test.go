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
		if mod.PkgDir != pkgDir {
			t.Errorf("expected Go package dir to be %q, got %q", pkgDir, mod.PkgDir)
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
