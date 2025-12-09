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

package engine

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// ExprMutator is a mutator.Mutator for expression-level mutations.
//
// Unlike TokenMutator which swaps tokens, ExprMutator performs AST
// reconstruction to create new expression structures. This enables
// mutations like wrapping (!x → !!x) that cannot be done by token swapping.
//
// ExprMutator uses the same file locking mechanism as TokenMutator to
// ensure safe concurrent mutations.
type ExprMutator struct {
	pkg        string
	fs         *token.FileSet
	file       *ast.File
	exprNode   *NodeExpr
	workDir    string
	origFile   []byte
	status     mutator.Status
	mutantType mutator.Type

	// origExpr stores a reference to the original expression for AST restoration
	origExpr ast.Expr

	// parentNode and replaceFunc handle the mutation application
	parentNode  ast.Node
	replaceFunc func(newExpr ast.Expr) error
}

// NewExprMutant initializes an ExprMutator with parent tracking.
func NewExprMutant(
	pkg string,
	set *token.FileSet,
	file *ast.File,
	node *NodeExpr,
	parentNode ast.Node,
	replaceFunc func(newExpr ast.Expr) error,
) *ExprMutator {
	return &ExprMutator{
		pkg:         pkg,
		fs:          set,
		file:        file,
		exprNode:    node,
		origExpr:    node.Expr(),
		parentNode:  parentNode,
		replaceFunc: replaceFunc,
	}
}

// Type returns the mutator.Type of the mutant.Mutator.
func (m *ExprMutator) Type() mutator.Type {
	return m.mutantType
}

// SetType sets the mutator.Type of the mutant.Mutator.
func (m *ExprMutator) SetType(mt mutator.Type) {
	m.mutantType = mt
}

// Status returns the mutator.Status of the mutant.Mutator.
func (m *ExprMutator) Status() mutator.Status {
	return m.status
}

// SetStatus sets the mutator.Status of the mutant.Mutator.
func (m *ExprMutator) SetStatus(s mutator.Status) {
	m.status = s
}

// Position returns the token.Position where the ExprMutator resides.
func (m *ExprMutator) Position() token.Position {
	return m.fs.Position(m.exprNode.Pos())
}

// Pos returns the token.Pos where the ExprMutator resides.
func (m *ExprMutator) Pos() token.Pos {
	return m.exprNode.Pos()
}

// Pkg returns the package name to which the mutant belongs.
func (m *ExprMutator) Pkg() string {
	return m.pkg
}

// Apply performs the expression mutation by reconstructing the AST.
//
// The process:
// 1. Acquire file lock (prevents concurrent mutations on same file)
// 2. Read original file content
// 3. Apply mutation by creating new expression in AST
// 4. Write mutated file
// 5. Restore original expression in AST
// 6. Release file lock
//
// Like TokenMutator, the AST is immediately restored after file writing
// to keep the shared AST clean for subsequent mutations.
func (m *ExprMutator) Apply() error {
	fileLock(m.Position().Filename).Lock()
	defer fileLock(m.Position().Filename).Unlock()

	filename := filepath.Join(m.workDir, m.Position().Filename)

	var err error
	//nolint:gosec // filename is internally constructed, not user input
	m.origFile, err = os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Get the mutated expression based on mutation type
	mutatedExpr, err := m.getMutatedExpr()
	if err != nil {
		return err
	}

	// Replace expression in AST
	if err = m.replaceFunc(mutatedExpr); err != nil {
		return err
	}

	// Write mutated file
	if err = m.writeMutatedFile(filename); err != nil {
		// Restore original on write failure
		_ = m.replaceFunc(m.origExpr)

		return err
	}

	// Restore AST immediately (file is already written with mutation)
	return m.replaceFunc(m.origExpr)
}

// getMutatedExpr creates the mutated expression based on the mutation type.
func (m *ExprMutator) getMutatedExpr() (ast.Expr, error) {
	//nolint:exhaustive // Only expression-level mutations handled here; token mutations use TokenMutator
	switch m.mutantType {
	case mutator.InvertLogicalNot:
		return m.invertLogicalNot()
	default:
		return nil, fmt.Errorf("expression mutation type %s not yet implemented", m.mutantType)
	}
}

// invertLogicalNot transforms !x into !!x by wrapping the original UnaryExpr
// with another NOT operator.
func (m *ExprMutator) invertLogicalNot() (ast.Expr, error) {
	unaryExpr, ok := m.origExpr.(*ast.UnaryExpr)
	if !ok {
		return nil, fmt.Errorf("InvertLogicalNot requires UnaryExpr, got %T", m.origExpr)
	}

	if unaryExpr.Op != token.NOT {
		return nil, fmt.Errorf("InvertLogicalNot requires NOT operator, got %s", unaryExpr.Op)
	}

	// Create a new UnaryExpr that wraps the original !x expression
	// Result: !!x (NOT of NOT of x)
	mutated := &ast.UnaryExpr{
		OpPos: unaryExpr.OpPos, // Use same position as original
		Op:    token.NOT,       // Outer NOT operator
		X:     unaryExpr,       // The entire original !x expression
	}

	return mutated, nil
}

func (m *ExprMutator) writeMutatedFile(filename string) error {
	w := &bytes.Buffer{}
	err := printer.Fprint(w, m.fs, m.file)
	if err != nil {
		return err
	}

	err = os.WriteFile(filename, w.Bytes(), 0600)
	if err != nil {
		return err
	}

	return nil
}

// Rollback puts back the original file after the test and cleans up the
// ExprMutator to free memory.
func (m *ExprMutator) Rollback() error {
	defer m.resetOrigFile()
	filename := filepath.Join(m.workDir, m.Position().Filename)

	return os.WriteFile(filename, m.origFile, 0600)
}

// SetWorkdir sets the base path on which to Apply and Rollback operations.
func (m *ExprMutator) SetWorkdir(path string) {
	m.workDir = path
}

// Workdir returns the current working dir in which the Mutator will apply its mutations.
func (m *ExprMutator) Workdir() string {
	return m.workDir
}

func (m *ExprMutator) resetOrigFile() {
	var zeroByte []byte
	m.origFile = zeroByte
}
