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
	"fmt"
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
	workdir string
	fs      *token.FileSet
	file    *ast.File
	node    ast.Node

	// ApplyF is the function that modifies the ast.Node so that the mutation
	// is effective.
	ApplyF func()
	// RollbackF is the function that modifies the ast.Node so that the mutation
	// is removed.
	RollbackF func()
	// TokPos is the position of the token.Token that can be mutated.
	TokPos token.Pos
	// Type is the MutantType categorizing the Mutant.
	Type MutantType
	// Status is the current Status of the Mutant.
	Status MutantStatus
}

// NewMutant initialises a Mutant.
func NewMutant(set *token.FileSet, file *ast.File, node ast.Node) Mutant {
	return Mutant{
		fs:   set,
		file: file,
		node: node,
	}
}

// Apply applies the Mutant to the source file.
//
// It works by executing the ApplyF function of the Mutant, then opening the
// file where the Mutant was found, and overriding it with the mutated version.
func (m *Mutant) Apply() error {
	m.ApplyF()
	f, err := openFile(m.fs, m.TokPos, m.workdir)
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
	m.RollbackF()
	f, err := openFile(m.fs, m.TokPos, m.workdir)
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
	m.workdir = path
}

// Pos returns the token.Position where the Mutant resides.
func (m *Mutant) Pos() token.Position {
	return m.fs.Position(m.TokPos)
}

func openFile(fs *token.FileSet, tokPos token.Pos, workdir string) (*os.File, error) {
	file := fs.File(tokPos)
	if workdir != "" {
		workdir += "/"
	}
	f, err := os.OpenFile(workdir+file.Name(), os.O_TRUNC|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		return nil, err
	}
	return f, err
}

func closeFile(f *os.File) {
	err := f.Close()
	if err != nil {
		fmt.Println("an error occurred while closing the mutated file")
	}
}
