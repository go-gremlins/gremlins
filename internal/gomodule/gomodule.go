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

// Package gomodule detects and provides information about Go modules.
package gomodule

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// GoModule represents the current execution context in Gremlins.
//
//	Name is the module name of the Go module being tested by Gremlins.
//	Root is the root folder of the Go module.
//	CallingDir is the folder in which Gremlins is running.
type GoModule struct {
	Name       string
	Root       string
	CallingDir string
}

// Init initializes the current module. It finds the module name and the root
// of the module, then returns a GoModule struct.
func Init(path string) (GoModule, error) {
	if path == "" {
		return GoModule{}, fmt.Errorf("path is not set")
	}
	mod, root, err := modPkg(path)
	if err != nil {
		return GoModule{}, err
	}
	path, _ = filepath.Rel(root, path)

	return GoModule{
		Name:       mod,
		Root:       root,
		CallingDir: path,
	}, nil
}

func modPkg(path string) (string, string, error) {
	root := findModuleRoot(path)
	//nolint:gosec // root is internally determined, not user input
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", "", err
	}
	// go.mod may carry comments and blank lines above the module directive,
	// and the module path may be quoted, so parse it rather than reading
	// the first line.
	packageName := modfile.ModulePath(data)
	if packageName == "" {
		return "", "", fmt.Errorf("no module directive found in %s", filepath.Join(root, "go.mod"))
	}

	return packageName, root, nil
}

func findModuleRoot(path string) string {
	// Inspired by how Go itself finds the module root.
	path = filepath.Clean(path)
	for {
		if fi, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && !fi.IsDir() {
			return path
		}
		d := filepath.Dir(path)
		if d == path {
			break
		}
		path = d
	}

	return ""
}
