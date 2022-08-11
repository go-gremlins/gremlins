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
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"

	"github.com/go-gremlins/gremlins/pkg/mutant"
)

// TokenMutant is a mutant.Mutant of a token.Token.
type TokenMutant struct {
	origFile    []byte
	fs          *token.FileSet
	file        *ast.File
	tokenNode   *NodeToken
	workDir     string
	status      mutant.Status
	mutantType  mutant.Type
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

// Apply overwrites the source code file with the mutated one. It also
// stores the original file in the TokenMutant in order to allow
// Rollback to put it back later.
//
// To apply the modification, it first removes the source code file which
// contains the mutant position, then it writes it back with the mutation
// applied.
// The removal of the file is necessary because it might be a hard link
// to the original file, and, if it was modified in place, it would modify
// the original. Removing the link and re-writing the file preserves the
// original to be modified.
//
// Apply also puts back the original Token after the mutated file write.
// This is done in order to facilitate the atomicity of the operation,
// avoiding locking in a method and unlocking in another.
func (m *TokenMutant) Apply() error {
	filename := filepath.Join(m.workDir, m.Position().Filename)
	var err error
	m.origFile, err = os.ReadFile(filename)
	if err != nil {
		return err
	}

	m.actualToken = m.tokenNode.Tok()
	m.tokenNode.SetTok(tokenMutations[m.Type()][m.tokenNode.Tok()])

	w := &bytes.Buffer{}
	err = printer.Fprint(w, m.fs, m.file)
	if err != nil {
		return err
	}

	// We need to remove the file before writing because it can be
	// a hard link to the original file.
	err = os.RemoveAll(filename)
	if err != nil {
		return err
	}

	err = os.WriteFile(filename, w.Bytes(), 0600)
	if err != nil {
		return err
	}

	// Rollback here to facilitate the atomicity of the operation.
	m.tokenNode.SetTok(m.actualToken)

	return nil
}

// Rollback puts back the original file after the test and cleans up the
// TokenMutant to free memory.
//
// It isn't necessary to remove the file before writing as it is done in
// Apply, because in this case, we can be sure the file is not a hard link,
// since Apply already made it a concrete one.
func (m *TokenMutant) Rollback() error {
	defer m.resetOrigFile()
	filename := filepath.Join(m.workDir, m.Position().Filename)

	return os.WriteFile(filename, m.origFile, 0600)
}

// SetWorkdir sets the base path on which to Apply and Rollback operations.
//
// By default, TokenMutant will operate on the same source on which the analysis
// was performed. Changing the workdir will prevent the modifications of the
// original files.
func (m *TokenMutant) SetWorkdir(path string) {
	m.workDir = path
}

func (m *TokenMutant) resetOrigFile() {
	var zeroByte []byte
	m.origFile = zeroByte
}
