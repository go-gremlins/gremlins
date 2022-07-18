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
	"github.com/k3rn31/gremlins/log"
	"io/fs"
	"os"
	"path/filepath"
)

// Dealer is the responsible for creating and returning the reference
// to a workdir to use during mutation testing instead of the actual
// source code.
type Dealer interface {
	Get() (string, func(), error)
}

// CDealer is the implementation of the Dealer interface, responsible
// for creating a working directory of a source directory. It allows
// Gremlins not to work in the actual source directory messing up
// with the source code files.
type CDealer struct {
	workDir string
	srcDir  string
}

// NewDealer instantiates a new CDealer.
func NewDealer(workDir, srcDir string) CDealer {
	return CDealer{workDir: workDir, srcDir: srcDir}
}

// Get provides a working directory where all the files are hard links
// to the original files in the source directory. It also returns a
// closer function that cleans up the directory.
//
// The idea is to make this a sort of workdir pool when Gremlins will
// support parallel execution.
func (fm CDealer) Get() (string, func(), error) {
	dstDir, err := os.MkdirTemp(fm.workDir, "wd-*")
	if err != nil {
		return "", nil, err
	}
	err = filepath.Walk(fm.srcDir, func(srcPath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(fm.srcDir, srcPath)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		dstPath := filepath.Join(dstDir, relPath)

		switch mode := info.Mode(); {
		case mode.IsDir():
			if err := os.Mkdir(dstPath, info.Mode()); err != nil && !os.IsExist(err) {
				return err
			}
		case mode.IsRegular():
			if err := os.Link(srcPath, dstPath); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return "", nil, err
	}
	return dstDir, func() {
		err := os.RemoveAll(dstDir)
		if err != nil {
			log.Errorln("impossible to remove temporary folder")
		}
	}, nil
}
