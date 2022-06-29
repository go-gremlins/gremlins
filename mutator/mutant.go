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

type MutantStatus int

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

type Mutant struct {
	workdir string
	fs      *token.FileSet
	file    *ast.File
	node    ast.Node

	ApplyF    func()
	RollbackF func()
	TokPos    token.Pos
	Type      MutantType
	Status    MutantStatus
}

func NewMutant(set *token.FileSet, file *ast.File, node ast.Node) Mutant {
	return Mutant{
		fs:   set,
		file: file,
		node: node,
	}
}

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

func (m *Mutant) SetWorkdir(path string) {
	m.workdir = path
}

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
