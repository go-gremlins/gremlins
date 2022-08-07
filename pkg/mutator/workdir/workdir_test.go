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

package workdir_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hectane/go-acl"

	"github.com/go-gremlins/gremlins/pkg/mutator/workdir"
)

func TestLinkFolder(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	populateSrcDir(t, srcDir, 3)
	dstDir := t.TempDir()

	mngr := workdir.NewDealer(dstDir, srcDir, workdir.WithDockerRootFolder(dstDir))

	dstDir, cl, err := mngr.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer cl()

	err = filepath.Walk(srcDir, func(path string, srcFileInfo fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			t.Fatal(err)
		}
		if relPath == "." {
			return nil
		}
		dstPath := filepath.Join(dstDir, relPath)
		dstFileInfo, err := os.Lstat(dstPath)
		if err != nil {
			t.Fatal(err)
		}

		if srcFileInfo.Mode().IsRegular() {
			sameFile := os.SameFile(dstFileInfo, srcFileInfo)
			if !sameFile {
				t.Error("expected file to be the same, got different file")
			}
		}
		if !cmp.Equal(dstFileInfo.Name(), srcFileInfo.Name()) {
			t.Errorf("expected Name to be %v, got %v", srcFileInfo.Name(), dstFileInfo.Name())
		}
		if !cmp.Equal(dstFileInfo.Mode(), srcFileInfo.Mode()) {
			t.Errorf(cmp.Diff(srcFileInfo.Mode(), dstFileInfo.Mode()))
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCopyFolder(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	populateSrcDir(t, srcDir, 3)
	dstDir := t.TempDir()

	dockerRootDir := t.TempDir()
	dockerEnv := filepath.Join(dockerRootDir, ".dockerenv")
	err := os.WriteFile(dockerEnv, []byte{}, 0400)
	if err != nil {
		t.Fatal(err)
	}

	mngr := workdir.NewDealer(dstDir, srcDir, workdir.WithDockerRootFolder(dockerRootDir))

	dstDir, cl, err := mngr.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer cl()

	err = filepath.Walk(srcDir, func(path string, srcFileInfo fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			t.Fatal(err)
		}
		if relPath == "." {
			return nil
		}
		dstPath := filepath.Join(dstDir, relPath)
		dstFileInfo, err := os.Lstat(dstPath)
		if err != nil {
			t.Fatal(err)
		}

		sameFile := os.SameFile(dstFileInfo, srcFileInfo)
		if sameFile {
			t.Error("expected file to be different, got the same file")
		}

		if !cmp.Equal(dstFileInfo.Name(), srcFileInfo.Name()) {
			t.Errorf("expected Name to be %v, got %v", srcFileInfo.Name(), dstFileInfo.Name())
		}
		if !cmp.Equal(dstFileInfo.Mode(), srcFileInfo.Mode()) {
			t.Errorf(cmp.Diff(srcFileInfo.Mode(), dstFileInfo.Mode()))
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCDealerErrors(t *testing.T) {
	t.Run("dstDir is not a path", func(t *testing.T) {
		t.Parallel()
		srcDir := "not a dir"
		dstDir := t.TempDir()

		mngr := workdir.NewDealer(dstDir, srcDir)

		_, _, err := mngr.Get()
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("srcDir is not readable", func(t *testing.T) {
		t.Parallel()
		srcDir := t.TempDir()
		err := os.Chmod(srcDir, 0000)
		clean := os.Chmod
		if runtime.GOOS == "windows" {
			err = acl.Chmod(srcDir, 0000)
			clean = acl.Chmod
		}
		if err != nil {
			t.Fatal(err)
		}
		defer func(d string) {
			_ = clean(d, 0700)
		}(srcDir)

		dstDir := t.TempDir()

		mngr := workdir.NewDealer(dstDir, srcDir)

		_, _, err = mngr.Get()
		if err == nil {
			t.Errorf("expected an error")
		}
	})

	t.Run("dstDir is not writeable", func(t *testing.T) {
		t.Parallel()
		srcDir := t.TempDir()
		dstDir := t.TempDir()
		err := os.Chmod(dstDir, 0000)
		clean := os.Chmod
		if runtime.GOOS == "windows" {
			err = acl.Chmod(dstDir, 0000)
			clean = acl.Chmod
		}
		if err != nil {
			t.Fatal(err)
		}
		defer func(d string) {
			_ = clean(d, 0700)
		}(dstDir)

		mngr := workdir.NewDealer(dstDir, srcDir)

		_, _, err = mngr.Get()
		if err == nil {
			t.Errorf("expected an error")
		}
	})
}

func populateSrcDir(t *testing.T, srcDir string, depth int) {
	t.Helper()
	if depth == 0 {
		return
	}

	for i := 0; i < 10; i++ {
		dirName := filepath.Join(srcDir, fmt.Sprintf("srcdir-%d", i))
		err := os.Mkdir(dirName, 0700)
		if err != nil {
			t.Fatal(err)
		}
		populateSrcDir(t, dirName, depth-1)
	}

	for i := 0; i < 10; i++ {
		fileName := filepath.Join(srcDir, fmt.Sprintf("srcfile-%d", i))
		err := os.WriteFile(fileName, []byte{}, 0400)
		if err != nil {
			t.Fatal(err)
		}
	}
}
