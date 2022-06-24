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
	"bufio"
	"bytes"
	"fmt"
	"golang.org/x/tools/cover"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Coverage is responsible for executing a Go test with coverage via the Run() method,
// then parsing the result coverage report file.
type Coverage struct {
	cmdContext execContext
	workDir    string
	fileName   string
	mod        string
}

type execContext = func(name string, args ...string) *exec.Cmd

// New instantiates a Coverage element using exec.Command as execContext,
// actually running the command on the OS.
func New(workdir string) *Coverage {
	return NewWithCmdAndPackage(exec.Command, getMod(), workdir)
}

func getMod() string {
	file, err := os.Open("go.mod")
	if err != nil {
		fmt.Printf("not in a go module root folder: %v\n", err)
		os.Exit(-1)
	}
	r := bufio.NewReader(file)
	line, _, err := r.ReadLine()
	if err != nil {
		fmt.Printf("not in a go module root folder %v\n\n", err)
		os.Exit(-1)
	}
	packageName := bytes.TrimPrefix(line, []byte("module "))
	return string(packageName)
}

// NewWithCmdAndPackage instantiates a Coverage element given a custom execContext.
func NewWithCmdAndPackage(cmdContext execContext, mod, workdir string) *Coverage {
	return &Coverage{
		cmdContext: cmdContext,
		workDir:    workdir,
		fileName:   "coverage",
		mod:        mod,
	}
}

// Run executes the coverage command and parses the results, returning a *Profile
// object.
func (c Coverage) Run() (Profile, error) {
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

func (c Coverage) getProfile(err error) (Profile, error) {
	cf, err := os.Open(c.filePath())
	if err != nil {
		return nil, err
	}
	profile, err := c.parse(cf)
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

func (c Coverage) parse(data io.Reader) (Profile, error) {
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
			fn := removeModuleFromPath(p, c)
			status[fn] = append(status[fn], block)
		}
	}
	return status, nil
}

func removeModuleFromPath(p *cover.Profile, c Coverage) string {
	return strings.ReplaceAll(p.FileName, c.mod+"/", "")
}
