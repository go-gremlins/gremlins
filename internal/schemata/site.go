/*
 * Copyright 2026 The Gremlins Authors
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

// Package schemata rewrites Go source so that every mutant is compiled into
// one binary behind a runtime switch (mutant schemata).
package schemata

import (
	"errors"
	"go/ast"
	"go/token"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// ErrUnsupported reports a mutation site the schemata rewrite cannot handle.
var ErrUnsupported = errors.New("schemata: unsupported site")

// Mutant is one mutation at a Site, identified by a binary-wide ID.
type Mutant struct {
	ID   int
	Type mutator.Type
}

// Site is an AST node with the mutants that apply to it.
//
// A constant group (see GroupConstantSites) is one Site for the sites inside
// a maximal constant-valued expression: Node is that expression, Tok is
// token.ILLEGAL, Members are the operator sites inside it, and Muts are all
// their mutants.
type Site struct {
	Node    ast.Node
	Tok     token.Token
	Muts    []Mutant
	Members []Site
}
