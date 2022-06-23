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

package coverage

import (
	"fmt"
	"golang.org/x/tools/cover"
	"io"
	"os"
	"os/exec"
)

// Block holds the start and end coordinates of a section of a source file
// covered by tests.
type Block struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Profile is implemented as a map holding a slice of Block per each filename.
type Profile map[string][]Block

// Coverage is responsible for executing a Go test with coverage via the Run() method,
// then parsing the result coverage report file.
type Coverage struct {
	cmdContext execContext
	workDir    string
	fileName   string
}

type execContext = func(name string, args ...string) *exec.Cmd

// New instantiates a Coverage element using exec.Command as execContext,
// actually running the command on the OS.
func New(workdir string) *Coverage {
	return NewWithCmd(exec.Command, workdir)

}

// NewWithCmd instantiates a Coverage element given a custom execContext.
func NewWithCmd(cmdContext execContext, workdir string) *Coverage {
	return &Coverage{
		cmdContext: cmdContext,
		workDir:    workdir,
		fileName:   "coverage",
	}
}

// Run executes the coverage command and parses the results, returning a *Profile
// object.
func (c Coverage) Run() (*Profile, error) {
	err := c.execute()
	if err != nil {
		return nil, err
	}
	profile, err := c.getProfile(err)
	if err != nil {
		return nil, err
	}

	return profile, nil
}

func (c Coverage) getProfile(err error) (*Profile, error) {
	cf, err := os.Open(c.filePath())
	if err != nil {
		return nil, err
	}
	profile, err := parse(cf)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

func (c Coverage) filePath() string {
	return fmt.Sprintf("%v/%v", c.workDir, c.fileName)
}

func (c Coverage) execute() error {
	cmd := c.cmdContext("go", "test", "-cover", "-coverprofile", c.filePath(), "./...")
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func parse(data io.Reader) (*Profile, error) {
	profiles, err := cover.ParseProfilesFromReader(data)
	if err != nil {
		return nil, err
	}
	status := make(Profile)
	for _, p := range profiles {
		for _, b := range p.Blocks {
			block := Block{
				StartLine: b.StartLine,
				StartCol:  b.StartCol,
				EndLine:   b.EndLine,
				EndCol:    b.EndCol,
			}
			status[p.FileName] = append(status[p.FileName], block)
		}
	}
	return &status, nil
}
