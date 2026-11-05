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

package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenRAE/go-rae-gremlins/internal/configuration"
	"github.com/OpenRAE/go-rae-gremlins/internal/coverage"
	"github.com/OpenRAE/go-rae-gremlins/internal/engine"
	"github.com/OpenRAE/go-rae-gremlins/internal/engine/workdir"
	"github.com/OpenRAE/go-rae-gremlins/internal/gomodule"
	"github.com/OpenRAE/go-rae-gremlins/internal/mutator"
)

const routingModule = "example.org/demo"

// routingModuleFiles is a module in which only the tests of a file's own
// directory detect a boundary mutation of that file. A mutant that runs the
// tests of another package lives.
var routingModuleFiles = map[string]string{
	"go.mod": "module " + routingModule + "\n\ngo 1.25\n",
	"demo.go": `package demo

func Root(n int) bool { return n > 0 }
`,
	"demo_test.go": `package demo

import "testing"

func TestRoot(t *testing.T) {
	if !Root(1) || Root(0) {
		t.Fatal("wrong result")
	}
}
`,
	"cmd/app/main.go": `package main

func positive(n int) bool { return n > 0 }

func main() { _ = positive(1) }
`,
	"cmd/app/main_test.go": `package main

import "testing"

func TestPositive(t *testing.T) {
	if !positive(1) || positive(0) {
		t.Fatal("wrong result")
	}
}
`,
	"different/named.go": `package named

func Positive(n int) bool { return n > 0 }
`,
	"different/named_test.go": `package named

import "testing"

func TestPositive(t *testing.T) {
	if !Positive(1) || Positive(0) {
		t.Fatal("wrong result")
	}
}
`,
}

func TestMutantRunsTestsOfItsDirectory(t *testing.T) {
	testCases := []struct {
		wantPkgs   map[string]string
		name       string
		callingDir string
	}{
		{
			name:       "from module root",
			callingDir: ".",
			wantPkgs: map[string]string{
				"demo.go":            routingModule,
				"cmd/app/main.go":    routingModule + "/cmd/app",
				"different/named.go": routingModule + "/different",
			},
		},
		{
			name:       "from main package directory",
			callingDir: "cmd/app",
			wantPkgs:   map[string]string{"main.go": routingModule + "/cmd/app"},
		},
		{
			name:       "from directory with a different package name",
			callingDir: "different",
			wantPkgs:   map[string]string{"named.go": routingModule + "/different"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRoutingModule(t)
			mod, err := gomodule.Init(filepath.Join(root, filepath.FromSlash(tc.callingDir)))
			if err != nil {
				t.Fatalf("gomodule.Init(%q) error = %v", tc.callingDir, err)
			}

			viperSet(map[string]any{
				configuration.MutantTypeEnabledKey(mutator.ConditionalsBoundary): true,
				configuration.MutantTypeEnabledKey(mutator.ConditionalsNegation): true,
			})
			defer viperReset()

			wdDealer := workdir.NewCachedDealer(t.TempDir(), mod.Root)
			defer wdDealer.Clean()
			jDealer := engine.NewExecutorDealer(mod, wdDealer, 30*time.Second)
			codeData := engine.CodeData{Cov: wholeFilesCovered(tc.wantPkgs)}
			eng := engine.New(mod, codeData, jDealer)

			res := eng.Run(context.Background())

			got := make(map[string]int)
			for _, m := range res.Mutants {
				file := m.Position().Filename
				wantPkg, ok := tc.wantPkgs[file]
				if !ok {
					continue
				}
				got[file]++
				if m.Pkg() != wantPkg {
					t.Errorf("%s:%d %s package = %q, want %q", file, m.Position().Line, m.Type(), m.Pkg(), wantPkg)
				}
				if m.Status() != mutator.Killed {
					t.Errorf("%s:%d %s status = %s, want %s", file, m.Position().Line, m.Type(), m.Status(), mutator.Killed)
				}
			}
			for file := range tc.wantPkgs {
				if got[file] == 0 {
					t.Errorf("got no mutants of %s, want mutants", file)
				}
			}
		})
	}
}

func writeRoutingModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range routingModuleFiles {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("create directory of %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return root
}

// wholeFilesCovered returns a coverage profile that covers every line of the
// given files.
func wholeFilesCovered(files map[string]string) coverage.Profile {
	p := make(coverage.Profile)
	for file := range files {
		p[file] = []coverage.Block{{StartLine: 0, StartCol: 1, EndLine: 1 << 20, EndCol: 1}}
	}

	return p
}
