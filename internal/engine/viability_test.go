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

package engine_test

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/gomodule"
	"github.com/go-gremlins/gremlins/internal/mutator"

	"github.com/go-gremlins/gremlins/internal/configuration"
)

// operatorZoo exercises every Go operator gremlins has a mapping for, in both
// the prefix and the infix spelling wherever Go admits both, and then every
// shape in which a mapped mutation produces source that is not a program.
//
// It is a whole compilable package so that the mutants generated from it can
// be handed to the real compiler. The `// illegal:` and `// legal:` markers on
// its lines are read back by the tests below — see zooSites — so that the
// expected verdicts live next to the code that produces them rather than in a
// separate table of line numbers nobody can check by eye.
const operatorZoo = `package zoo

type point struct{ x, y int }

func Address(i int) *int      { return &i }
func Composite() *point       { return &point{x: 1, y: 2} }
func Complement(u uint) uint  { return ^u }
func Negate(i int) int        { return -i }
func Identity(i int) int      { return +i }
func Receive(c chan int) int  { return <-c }
func Negation(b bool) bool    { return !b }
func And(a, b int) int        { return a & b }
func Or(a, b int) int         { return a | b }
func Xor(a, b int) int        { return a ^ b }
func AndNot(a, b int) int     { return a &^ b }
func Shl(a int) int           { return a << 1 }
func Shr(a int) int           { return a >> 1 }
func Add(a, b int) int        { return a + b }
func Sub(a, b int) int        { return a - b }
func Mul(a, b int) int        { return a * b }
func Quo(a, b int) int        { return a / b }
func Rem(a, b int) int        { return a % b }
func Greater(a, b int) bool   { return a > b }
func Equal(a, b int) bool     { return a == b }
func Both(a, b bool) bool     { return a && b }  // legal: INVERT_LOGICAL

// Go defines + on strings and defines nothing else on them, so ARITHMETIC_BASE
// and INVERT_ASSIGNMENTS both have an entry for a token whose operands forbid
// the mutation. Nothing about the token says so; only the operand types do.
func Join(a, b string) string     { return a + "-" + b } // illegal: ARITHMETIC_BASE
func AppendTo(a, b string) string { a += b; return a }   // illegal: INVERT_ASSIGNMENTS  legal: REMOVE_SELF_ASSIGNMENTS

// The mutated expression is legal in isolation and wrong only where its value
// is used: 7 * 24 rewritten to 7 / 24 is the untyped constant 0, and it is the
// division three lines down that the compiler rejects. Anything narrower than
// a whole-package check cannot see this.
const hoursPerWeek = 7 * 24 // illegal: ARITHMETIC_BASE

func Weeks(hours int) int { return hours / hoursPerWeek } // legal: ARITHMETIC_BASE

// A ` + "`for`" + ` with no reachable break is a terminating statement, so this
// function needs no return after it. INVERT_LOOPCTRL turns the continue into a
// break, the loop stops terminating, and the function is missing a return.
func FirstPositive(xs []int) int {
	i := 0
	for {
		if xs[i] <= 0 { // legal: CONDITIONALS_BOUNDARY
			i++ // legal: INCREMENT_DECREMENT
			continue // illegal: INVERT_LOOPCTRL
		}

		return xs[i]
	}
}

// The same mutation the other way round: a break in a switch that no loop
// encloses has a meaning, and continue has none there.
func Classify(a int) string {
	switch a {
	case 1:
		break // illegal: INVERT_LOOPCTRL
	}

	return "one"
}

func Assigns(a int) int {
	a += 1
	a -= 1
	a *= 2
	a /= 2
	a %= 2
	a &= 3
	a |= 3
	a ^= 3
	a &^= 3
	a <<= 1
	a >>= 1
	a++
	a--

	for {
		if a > 0 {
			break
		}

		continue
	}

	return a
}
`

const zooModule = "example.com"

// zooSite is one mutation the zoo's markers make a claim about: the line it
// sits on and the mutator.Type that reaches it.
type zooSite struct {
	line  int
	mType string
}

func (s zooSite) String() string {
	return fmt.Sprintf("%s at line %d", s.mType, s.line)
}

// zooSites reads the `// <marker>:` annotations out of operatorZoo. A marker
// names one or more mutator types, all of which are claimed to be illegal (or
// legal) at that line.
func zooSites(marker string) map[zooSite]bool {
	sites := make(map[zooSite]bool)
	for i, line := range strings.Split(operatorZoo, "\n") {
		_, comment, found := strings.Cut(line, "// ")
		if !found {
			continue
		}
		for _, claim := range strings.Split(comment, "  ") {
			name, types, ok := strings.Cut(claim, ": ")
			if !ok || name != marker {
				continue
			}
			for _, mType := range strings.Fields(types) {
				// operatorZoo is embedded in a Go string that starts on the
				// line after the backquote, so the zoo's own line 1 is this
				// slice's index 0.
				sites[zooSite{line: i + 1, mType: mType}] = true
			}
		}
	}

	return sites
}

// generateZoo writes the zoo into a fresh module and runs the engine over it,
// returning the mutants and the directory they can be applied to.
//
// The engine reads the directory itself rather than a testing/fstest.MapFS,
// because that is the only configuration in which the default Viability has a
// package to load: the checker's whole subject is source that a compiler could
// be asked about, and a map in memory is not that.
func generateZoo(t *testing.T, opts ...engine.Option) ([]mutator.Mutator, string) {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module "+zooModule+"\n\ngo 1.25\n")
	writeFile(t, dir, "zoo.go", operatorZoo)

	viperSet(map[string]any{configuration.UnleashDryRunKey: true})
	defer viperReset()

	mod := gomodule.GoModule{Name: zooModule, Root: dir, CallingDir: "."}
	mut := engine.New(mod, engine.CodeData{}, newJobDealerStub(t), opts...)
	mutants := mut.Run(context.Background()).Mutants

	// A silent zero here would make every assertion built on it vacuous.
	if len(mutants) == 0 {
		t.Fatal("the operator zoo produced no mutants at all")
	}
	if out, err := goBuild(dir); err != nil {
		t.Fatalf("the unmutated operator zoo does not compile:\n%s", out)
	}

	return mutants, dir
}

// rejectedByCompiler applies each mutant in turn, hands the result to the real
// compiler, and returns the sites the compiler refused along with what it said
// about each.
func rejectedByCompiler(t *testing.T, mutants []mutator.Mutator, dir string) (map[zooSite]bool, []string) {
	t.Helper()

	rejected := make(map[zooSite]bool)
	var diagnostics []string
	for _, m := range mutants {
		m.SetWorkdir(dir)
		if err := m.Apply(); err != nil {
			t.Fatalf("applying the %s mutant at %s: %v", m.Type(), m.Position(), err)
		}
		out, buildErr := goBuild(dir)
		mutated := string(m.MutatedSnippet())
		if err := m.Rollback(); err != nil {
			t.Fatalf("rolling back the %s mutant at %s: %v", m.Type(), m.Position(), err)
		}
		if buildErr == nil {
			continue
		}
		rejected[zooSite{line: m.Position().Line, mType: m.Type().String()}] = true
		diagnostics = append(diagnostics,
			fmt.Sprintf("%s at %s:\n%s\nmutated source:\n%s", m.Type(), m.Position(), out, mutated))
	}

	return rejected, diagnostics
}

// TestEveryGeneratedMutantCompiles is the gate on the whole mutation table.
//
// A mutant the compiler rejects is not evidence about a test suite in either
// direction: it cannot be detected, so crediting a KILL for it pays the suite
// for the compiler's work, and recording it NOT VIABLE leaves a package's
// mutant set padded with entries that mean nothing. So the invariant is that
// gremlins does not emit one at all, and the only witness that settles it is
// the real compiler.
func TestEveryGeneratedMutantCompiles(t *testing.T) {
	t.Parallel()

	mutants, dir := generateZoo(t)
	_, diagnostics := rejectedByCompiler(t, mutants, dir)
	for _, d := range diagnostics {
		t.Errorf("a generated mutant the compiler rejects:\n%s", d)
	}
}

// TestTheViabilityCheckIsWhatKeepsTheZooCompiling is the break-proof for the
// gate above: it runs the identical generation with the Viability removed and
// requires the compiler to reject EXACTLY the sites operatorZoo marks illegal.
//
// Without it TestEveryGeneratedMutantCompiles is a test that cannot fail — it
// would pass just as well against a fixture that never exercised the shapes,
// or against a checker that had been quietly disabled. Requiring the exact set
// pins the guard from both sides: a shape the fixture claims to cover but does
// not is a missing rejection here, and a mutation the checker suppresses
// although the compiler accepts it is an extra one.
func TestTheViabilityCheckIsWhatKeepsTheZooCompiling(t *testing.T) {
	t.Parallel()

	mutants, dir := generateZoo(t, engine.WithViability(nil))
	rejected, _ := rejectedByCompiler(t, mutants, dir)

	want := zooSites("illegal")
	if len(want) == 0 {
		t.Fatal("operatorZoo marks no site illegal, so this test asserts nothing")
	}
	for site := range want {
		if !rejected[site] {
			t.Errorf("operatorZoo marks %s illegal, but the compiler accepted it with the "+
				"viability check off: the fixture no longer exercises that shape, so the gate "+
				"passes without evidence", site)
		}
	}
	for site := range rejected {
		if !want[site] {
			t.Errorf("the compiler rejected %s, which operatorZoo does not mark illegal: either "+
				"the fixture gained a shape nobody accounted for, or the mutation table did", site)
		}
	}
}

// TestViabilityKeepsTheLegalMutationsAtTheSameSites is the guard in the other
// direction. The check removes mutants, and removing mutants is exactly how a
// mutation score can be inflated, so it has to be shown to remove only what
// the compiler would have rejected anyway. Every site the zoo marks legal
// carries a mutation whose token sits on the same line as an illegal one, or
// is the same token under a different mutator; all of them must survive.
func TestViabilityKeepsTheLegalMutationsAtTheSameSites(t *testing.T) {
	t.Parallel()

	mutants, _ := generateZoo(t)

	generated := make(map[zooSite]bool)
	for _, m := range mutants {
		generated[zooSite{line: m.Position().Line, mType: m.Type().String()}] = true
	}

	want := zooSites("legal")
	if len(want) == 0 {
		t.Fatal("operatorZoo marks no site legal, so this test asserts nothing")
	}
	for site := range want {
		if !generated[site] {
			t.Errorf("the viability check suppressed %s, which the compiler accepts: it is "+
				"removing honest mutants, which shrinks the denominator a mutation score is "+
				"measured against.\ngenerated: %s", site, describeSites(generated))
		}
	}
}

// describe renders a mutant list as the type and position of each entry,
// which is what a failure here needs to name.
func describe(mutants []mutator.Mutator) string {
	described := make([]string, 0, len(mutants))
	for _, m := range mutants {
		described = append(described, fmt.Sprintf("%s at %s", m.Type(), m.Position()))
	}

	return strings.Join(described, ", ")
}

func describeSites(sites map[zooSite]bool) string {
	described := make([]string, 0, len(sites))
	for site := range sites {
		described = append(described, site.String())
	}
	sort.Strings(described)

	return strings.Join(described, ", ")
}

func goBuild(dir string) (string, error) {
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	return string(out), err
}
