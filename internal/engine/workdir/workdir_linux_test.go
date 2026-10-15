//go:build linux

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

package workdir

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestCopyClosesFiles(t *testing.T) {
	previous := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previous)

	root := t.TempDir()
	source, work := filepath.Join(root, "source"), filepath.Join(root, "work")
	for _, dir := range []string{source, work} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 50; i++ {
		if err := os.WriteFile(filepath.Join(source, fmt.Sprintf("file%d.txt", i)), []byte("copied data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dealer := NewCachedDealer(work, source)
	defer dealer.Clean()
	for i := 0; i < 8; i++ {
		if _, err := dealer.Get(fmt.Sprintf("worker%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	assertNoOpenFiles(t, root)
	dealer.Clean()
	assertNoOpenFiles(t, root)
}

func TestCopyClosesFilesOnError(t *testing.T) {
	previous := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previous)

	for _, name := range []string{"destination open", "copy"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
			if name == "copy" {
				// Opening a directory succeeds, but copying its contents as a file fails.
				if err := os.Mkdir(source, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(source, []byte("data"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(destination, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := doCopy(source, destination, 0600); err == nil {
				t.Fatal("expected copy error")
			}
			assertNoOpenFiles(t, root)
		})
	}
}

func assertNoOpenFiles(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	var opened []string
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err == nil && strings.HasPrefix(target, root+string(os.PathSeparator)) {
			opened = append(opened, target)
		}
	}
	if len(opened) != 0 {
		t.Errorf("copy left %d source/destination files open", len(opened))
	}
}
