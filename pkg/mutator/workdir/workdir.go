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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-gremlins/gremlins/pkg/log"
)

// Dealer is the responsible for creating and returning the reference
// to a workdir to use during mutation testing instead of the actual
// source code.
//
// It has two methods:
//
//		Get that returns a folder name that will be used by Gremlins as workdir.
//	    Clean that must be called to remove all the created folders.
type Dealer interface {
	Get(idf string) (string, error)
	Clean()
}

// CachedDealer is the implementation of the Dealer interface, responsible
// for creating a working directory of a source directory. It allows
// Gremlins not to work in the actual source directory messing up
// with the source code files.
type CachedDealer struct {
	mutex            *sync.RWMutex
	cache            map[string]string
	workDir          string
	srcDir           string
	dockerRootFolder string
	withinDocker     bool
}

// Option for the CachedDealer initialization.
type Option func(d *CachedDealer) *CachedDealer

// NewCachedDealer instantiates a new Dealer that keeps a cache of the
// instantiated folders. Every time a new working directory is requested
// with the same identifier, the same folder reference is returned.
// It also verifies whether it is running inside a Docker container or not,
// and makes copies instead of hard links if it is.
func NewCachedDealer(workDir, srcDir string, opts ...Option) *CachedDealer {
	dealer := &CachedDealer{
		mutex:            &sync.RWMutex{},
		cache:            make(map[string]string),
		workDir:          workDir,
		srcDir:           srcDir,
		dockerRootFolder: "/",
	}

	for _, opt := range opts {
		dealer = opt(dealer)
	}

	if isRunningInDockerContainer(dealer.dockerRootFolder) {
		dealer.withinDocker = true

		return dealer
	}

	return dealer
}

// WithDockerRootFolder overrides the default root folder where to look for .dockerenv file.
func WithDockerRootFolder(rootFolder string) Option {
	return func(d *CachedDealer) *CachedDealer {
		d.dockerRootFolder = rootFolder

		return d
	}
}

// Get provides a working directory where all the files are hard links
// to the original files in the source directory. It makes full copies
// in case Gremlins is running inside a Docker container.
func (cd *CachedDealer) Get(idf string) (string, error) {
	dstDir, ok := cd.getFromCache(idf)
	if ok {
		return dstDir, nil
	}

	dstDir, err := os.MkdirTemp(cd.workDir, "wd-*")
	if err != nil {
		return "", err
	}
	err = filepath.Walk(cd.srcDir, cd.copyTo(dstDir))
	if err != nil {
		return "", err
	}

	cd.setCache(idf, dstDir)

	return dstDir, nil
}

// Clean frees all the cached folders and removes all of them from disk.
func (cd *CachedDealer) Clean() {
	for _, v := range cd.cache {
		err := os.RemoveAll(v)
		if err != nil {
			log.Errorf("impossible to remove temporary folder %s: %s\n", v, err)
		}
	}
	cd.cache = make(map[string]string)
}

func (cd *CachedDealer) getFromCache(idf string) (string, bool) {
	cd.mutex.RLock()
	defer cd.mutex.RUnlock()
	dstDir, ok := cd.cache[idf]
	if ok {
		return dstDir, true
	}

	return "", false
}

func (cd *CachedDealer) setCache(idf, folder string) {
	cd.mutex.Lock()
	defer cd.mutex.Unlock()
	cd.cache[idf] = folder
}

func (cd *CachedDealer) copyTo(dstDir string) func(srcPath string, info fs.FileInfo, err error) error {
	return func(srcPath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(cd.srcDir, srcPath)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		dstPath := filepath.Join(dstDir, relPath)

		return cd.copyPath(srcPath, dstPath, info)
	}
}

func (cd *CachedDealer) copyPath(srcPath, dstPath string, info fs.FileInfo) error {
	switch mode := info.Mode(); {
	case mode.IsDir():
		if err := os.Mkdir(dstPath, mode); err != nil && !os.IsExist(err) {
			return err
		}
	case mode.IsRegular():
		if cd.withinDocker {
			// When gremlins is running within a docker container, hard link doesn't work, so we do a copy of the file
			if err := doCopy(srcPath, dstPath, mode); err != nil {
				return err
			}
		} else {
			if err := os.Link(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func doCopy(srcPath, dstPath string, fileMode fs.FileMode) error {
	s, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	//nolint:nosnakecase
	d, err := os.OpenFile(dstPath, os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return err
	}

	if _, err = io.Copy(d, s); err != nil {
		return err
	}

	return nil
}

func isRunningInDockerContainer(dockerRootFolder string) bool {
	f := strings.TrimSuffix(dockerRootFolder, "/") + "/" + ".dockerenv"
	if _, err := os.Stat(f); err == nil {
		return true
	}

	return false
}
