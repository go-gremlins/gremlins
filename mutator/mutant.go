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

package mutator

import (
	"github.com/k3rn31/gremlins/log"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
)

// MutantStatus represents the status of a given Mutant.
//
// - NotCovered means that a Mutant has been identified, but is not covered
//   by tests.
// - Runnable means that a Mutant has been identified and is covered by tests,
//   which means it can be executed.
// - Lived means that the Mutant has been tested, but the tests did pass, which
//   means the test suite is not effective in catching it.
// - Killed means that the Mutant has been tested and the tests failed, which
//   means they are effective in covering this regression.
type MutantStatus int

// Currently supported MutantStatus.
const (
	NotCovered MutantStatus = iota
	Runnable
	Lived
	Killed
)

func (ms MutantStatus) String() string {
	switch ms {
	case NotCovered:
		return "NOT COVERED"
	case Runnable:
		return "RUNNABLE"
	case Lived:
		return "LIVED"
	case Killed:
		return "KILLED"
	default:
		panic("this should not happen")
	}
}

// Mutant represents a possible mutation of the source code.
type Mutant struct {
	workDir     string
	fs          *token.FileSet
	file        *ast.File
	actualToken token.Token

	// TokenNode contains the reference to the token.Token to be mutated.
	TokenNode *NodeToken
	// Type is the MutantType categorizing the Mutant.
	Type MutantType
	// Status is the current Status of the Mutant.
	Status MutantStatus
}

// NewMutant initialises a Mutant.
func NewMutant(set *token.FileSet, file *ast.File, node *NodeToken) Mutant {
	return Mutant{
		fs:   set,
		file: file,
		//actualToken: *astNode.Tok(),
		TokenNode: node,
	}
}

// Apply applies the Mutant to the source file.
//
// It works by executing the ApplyF function of the Mutant, then opening the
// file where the Mutant was found, and overriding it with the mutated version.
func (m *Mutant) Apply() error {
	m.actualToken = m.TokenNode.Tok()
	m.TokenNode.SetTok(mutations[m.Type][m.TokenNode.Tok()])

	err := removeFile(m.fs, m.TokenNode.TokPos, m.workDir)
	if err != nil {
		return err
	}
	f, err := openFile(m.fs, m.TokenNode.TokPos, m.workDir)
	if err != nil {
		return err
	}
	defer closeFile(f)
	err = printer.Fprint(f, m.fs, m.file)
	if err != nil {
		return err
	}
	return nil
}

// Rollback removes the Mutant from the source file.
//
// It works by executing the RollbackF function of the Mutant, then opening the
// mutated file where the Mutant was applied, and overriding it with the
// un-mutated version.
func (m *Mutant) Rollback() error {
	m.TokenNode.SetTok(m.actualToken)

	f, err := openFile(m.fs, m.TokenNode.TokPos, m.workDir)
	if err != nil {
		return err
	}
	defer closeFile(f)
	err = printer.Fprint(f, m.fs, m.file)
	if err != nil {
		return err
	}

	return nil
}

// SetWorkdir sets the base path on which to Apply and Rollback operations.
//
// By default, Mutant will operate on the same source on which the analysis
// was performed. Changing the workdir will prevent the modifications of the
// original files.
func (m *Mutant) SetWorkdir(path string) {
	m.workDir = path
}

// Pos returns the token.Position where the Mutant resides.
func (m *Mutant) Pos() token.Position {
	return m.fs.Position(m.TokenNode.TokPos)
}

func removeFile(fs *token.FileSet, tokPos token.Pos, workdir string) error {
	file := fs.File(tokPos)
	file.Name()
	if workdir != "" {
		workdir += "/"
	}
	err := os.RemoveAll(workdir + file.Name())
	if err != nil {
		return err
	}
	return nil
}

func openFile(fs *token.FileSet, tokPos token.Pos, workdir string) (*os.File, error) {
	file := fs.File(tokPos)
	if workdir != "" {
		workdir += "/"
	}
	f, err := os.OpenFile(workdir+file.Name(), os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return nil, err
	}
	return f, err
}

func closeFile(f *os.File) {
	err := f.Close()
	if err != nil {
		log.Errorln("an error occurred while closing the mutated file")
	}
}
