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

package internal

import (
	"github.com/go-gremlins/gremlins/log"
	"github.com/go-gremlins/gremlins/mutant"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
)

// TokenMutant is a mutant.Mutant of a token.Token.
type TokenMutant struct {
	fs          *token.FileSet
	file        *ast.File
	status      mutant.Status
	mutantType  mutant.Type
	tokenNode   *NodeToken
	workDir     string
	actualToken token.Token
}

// NewTokenMutant initialises a TokenMutant.
func NewTokenMutant(set *token.FileSet, file *ast.File, node *NodeToken) *TokenMutant {
	return &TokenMutant{
		fs:        set,
		file:      file,
		tokenNode: node,
	}
}

// Type returns the mutant.Type of the mutant.Mutant.
func (m *TokenMutant) Type() mutant.Type {
	return m.mutantType
}

// SetType sets the mutant.Type of the mutant.Mutant.
func (m *TokenMutant) SetType(mt mutant.Type) {
	m.mutantType = mt
}

// Status returns the mutant.Status of the mutant.Mutant.
func (m *TokenMutant) Status() mutant.Status {
	return m.status
}

// SetStatus sets the mutant.Status of the mutant.Mutant.
func (m *TokenMutant) SetStatus(s mutant.Status) {
	m.status = s
}

// Position returns the token.Position where the TokenMutant resides.
func (m *TokenMutant) Position() token.Position {
	return m.fs.Position(m.tokenNode.TokPos)
}

// Pos returns the token.Pos where the TokenMutant resides.
func (m *TokenMutant) Pos() token.Pos {
	return m.tokenNode.TokPos
}

// Apply saves the original token.Token of the mutant.Mutant and sets the
// current token from the tokenMutations table.
//
// To apply the modification, it first removes the source code file which
// contains the mutant position, then it writes it back with the mutation
// applied.
func (m *TokenMutant) Apply() error {
	m.actualToken = m.tokenNode.Tok()
	m.tokenNode.SetTok(tokenMutations[m.Type()][m.tokenNode.Tok()])

	return m.writeOnFile()
}

// Rollback puts back the original token of the TokenMutant, then it writes
// it on the actual file which contains the position of the mutant.Mutant.
func (m *TokenMutant) Rollback() error {
	m.tokenNode.SetTok(m.actualToken)

	return m.writeOnFile()
}

func (m *TokenMutant) writeOnFile() error {
	err := removeFile(m.fs, m.tokenNode.TokPos, m.workDir)
	if err != nil {
		return err
	}
	f, err := openFile(m.fs, m.tokenNode.TokPos, m.workDir)
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
// By default, TokenMutant will operate on the same source on which the analysis
// was performed. Changing the workdir will prevent the modifications of the
// original files.
func (m *TokenMutant) SetWorkdir(path string) {
	m.workDir = path
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
	f, err := os.OpenFile(workdir+file.Name(), os.O_CREATE|os.O_WRONLY, 0600)
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
