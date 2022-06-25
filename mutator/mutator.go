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
	"github.com/k3rn31/gremlins/coverage"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

type MutationStatus int

const (
	NotCovered MutationStatus = iota
	Runnable
	Lived
	Killed
)

func (ms MutationStatus) String() string {
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
	Position   token.Position
	MutantType MutantType
	Status     MutationStatus
	Token      token.Token
	Mutation   token.Token
}

type Mutator struct {
	covProfile coverage.Profile
	fs         fs.FS
}

func New(fs fs.FS, p coverage.Profile) *Mutator {
	return &Mutator{covProfile: p, fs: fs}
}

func (mu Mutator) Run() []Mutant {
	var result []Mutant
	_ = fs.WalkDir(mu.fs, ".", func(path string, d fs.DirEntry, err error) error {
		if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
			src, _ := mu.fs.Open(path)
			r := mu.runOnFile(path, src)
			result = append(result, r...)
		}
		return nil
	})

	return result
}

func (mu Mutator) runOnFile(fileName string, src io.Reader) []Mutant {
	var result []Mutant
	set := token.NewFileSet()
	file, _ := parser.ParseFile(set, fileName, src, parser.ParseComments)
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.BinaryExpr:
			r, ok := mu.inspectBinaryExpr(set, node)
			if !ok {
				return true
			}
			result = append(result, r...)
		case *ast.UnaryExpr:
			// Do something
		}
		return true
	})
	return result
}

func (mu Mutator) inspectBinaryExpr(set *token.FileSet, be *ast.BinaryExpr) ([]Mutant, bool) {
	var result []Mutant
	mutantTypes, ok := tokenMutantType[be.Op]
	if !ok {
		return nil, false
	}
	for _, mt := range mutantTypes {
		pos := set.Position(be.OpPos)
		mutant := Mutant{
			MutantType: mt,
			Token:      be.Op,
			Mutation:   mutations[mt][be.Op],
			Status:     mutationStatus(mu, pos),
			Position:   pos,
		}
		result = append(result, mutant)
	}

	return result, true
}

func mutationStatus(mu Mutator, pos token.Position) MutationStatus {
	var status MutationStatus
	if mu.covProfile.IsCovered(pos) {
		status = Runnable
	}

	return status
}
