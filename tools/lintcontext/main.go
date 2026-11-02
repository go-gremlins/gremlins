// Command lintcontext supplies declaration identities for the lint baseline.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"strings"
)

type declaration struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Name  string `json:"name"`
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		log.Print("cannot parse lint declaration context; fix the source or parser input")
		os.Exit(1)
	}
}

func run(input io.Reader, output io.Writer) error {
	var sources map[string]string
	if err := json.NewDecoder(io.LimitReader(input, 32*1024*1024)).Decode(&sources); err != nil {
		return fmt.Errorf("decode source inputs: %w", err)
	}
	result := make(map[string][]declaration, len(sources))
	for name, source := range sources {
		items, err := declarations(source)
		if err != nil {
			return err
		}
		result[name] = items
	}

	return json.NewEncoder(output).Encode(result)
}

func declarations(source string) ([]declaration, error) {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, "", source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse declaration context: %w", err)
	}
	result := make([]declaration, 0, len(file.Decls))
	for _, node := range file.Decls {
		items, err := describe(positions, node)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}

	return result, nil
}

func describe(positions *token.FileSet, node ast.Decl) ([]declaration, error) {
	switch decl := node.(type) {
	case *ast.FuncDecl:
		name := "func:" + decl.Name.Name
		if decl.Recv != nil && len(decl.Recv.List) == 1 {
			var receiver bytes.Buffer
			if err := format.Node(&receiver, positions, decl.Recv.List[0].Type); err != nil {
				return nil, fmt.Errorf("format receiver identity: %w", err)
			}
			name = "method:" + receiver.String() + "." + decl.Name.Name
		}
		start := decl.Pos()
		if decl.Doc != nil {
			start = decl.Doc.Pos()
		}

		return []declaration{{Start: positions.Position(start).Line, End: positions.Position(decl.End()).Line, Name: name}}, nil
	case *ast.GenDecl:
		result := make([]declaration, 0, len(decl.Specs))
		for index, spec := range decl.Specs {
			start := spec.Pos()
			if doc := specDoc(spec); doc != nil {
				start = doc.Pos()
			}
			if index == 0 && decl.Doc != nil {
				start = decl.Doc.Pos()
			}
			result = append(result, declaration{
				Start: positions.Position(start).Line,
				End:   positions.Position(spec.End()).Line,
				Name:  decl.Tok.String() + ":" + specName(spec),
			})
		}

		return result, nil
	default:
		return nil, errors.New("unsupported declaration")
	}
}

func specDoc(spec ast.Spec) *ast.CommentGroup {
	switch value := spec.(type) {
	case *ast.ValueSpec:
		return value.Doc
	case *ast.TypeSpec:
		return value.Doc
	case *ast.ImportSpec:
		return value.Doc
	default:
		return nil
	}
}

func specName(spec ast.Spec) string {
	switch value := spec.(type) {
	case *ast.ValueSpec:
		names := make([]string, 0, len(value.Names))
		for _, name := range value.Names {
			names = append(names, name.Name)
		}

		return strings.Join(names, ",")
	case *ast.TypeSpec:
		return value.Name.Name
	case *ast.ImportSpec:
		return value.Path.Value
	default:
		return "unknown"
	}
}
