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

package schemata

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"slices"
	"strings"
	"text/template"

	"github.com/go-gremlins/gremlins/internal/mutator"
)

// ErrUnknownHelper is the panic value of HelperSet.Use for a name that is not
// one of the fixed helpers.
var ErrUnknownHelper = errors.New("schemata: unknown helper")

// helperDef is one generated declaration. Its template is executed with the
// identifier prefix as dot, standing for "_g" in the names it declares.
type helperDef struct {
	name     string
	public   bool     // a name Use accepts; false for shared constraint types
	needs    []string // other definitions it refers to
	imports  []string
	template *template.Template
}

func def(name string, public bool, needs, imports []string, src string) helperDef {
	return helperDef{
		name:     name,
		public:   public,
		needs:    needs,
		imports:  imports,
		template: template.Must(template.New(name).Parse(src)),
	}
}

// Every helper that can switch tests its id against 0 first: an id of 0 means
// "this mutant is absent at the site", and GREMLINS_MUTANT unset (or not a
// number) also parses to 0, so without the test an absent mutant would be
// switched on in every run that selects none.
var helperDefs = slices.Concat([]helperDef{
	def("Active", true, nil, []string{"os", "strconv"}, `
// {{.}}Active is the mutant this process runs: GREMLINS_MUTANT, or 0 for none.
var {{.}}Active = func() int { n, _ := {{.}}strconv.Atoi({{.}}os.Getenv("GREMLINS_MUTANT")); return n }()
`),
	def("Reached", true, nil, []string{"os", "sync"}, `
var {{.}}ReachedOnce {{.}}sync.Once

// {{.}}Reached records, once per process, that the active mutant's site ran.
// A sync.Once, not a bool: user code calls helpers from many goroutines, and a
// data race in generated code would be reported against the user's tests.
func {{.}}Reached() {
	{{.}}ReachedOnce.Do(func() {
		if p := {{.}}os.Getenv("GREMLINS_REACHED"); p != "" {
			if f, err := {{.}}os.OpenFile(p, {{.}}os.O_CREATE|{{.}}os.O_EXCL|{{.}}os.O_WRONLY, 0o600); err == nil {
				_ = f.Close()
			}
		}
	})
}
`),
	def("Bit", true, nil, nil, `
func {{.}}Bit(id int) uint {
	if id != 0 && {{.}}Active == id {
		{{.}}Reached()
		return 1
	}
	return 0
}
`),
	def("Xor", true, nil, nil, `
func {{.}}Xor[B ~bool](id int, b B) B {
	if id != 0 && {{.}}Active == id {
		{{.}}Reached()
		return !b
	}
	return b
}
`),
	ordered("LSS", "<", "<=", ">="),
	ordered("LEQ", "<=", "<", ">"),
	ordered("GTR", ">", ">=", "<="),
	ordered("GEQ", ">=", ">", "<"),
	def("Number", false, nil, nil, `
type {{.}}Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr | ~float32 | ~float64 | ~complex64 | ~complex128
}
`),
	def("Integer", false, nil, nil, `
type {{.}}Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}
`),
	arith("ADD", "Number", "+", "-"),
	def("SUB", true, []string{"Number"}, nil, `
// {{.}}SUB is binary minus. The engine discovers both ARITHMETIC_BASE (idA)
// and INVERT_NEGATIVES (idN) at a binary -, and both rewrite l - r to l + r.
func {{.}}SUB[T {{.}}Number](idA, idN int, l, r T) T {
	if {{.}}Active != 0 && ({{.}}Active == idA || {{.}}Active == idN) {
		{{.}}Reached()
		return l + r
	}
	return l - r
}
`),
	arith("MUL", "Number", "*", "/"),
	arith("QUO", "Number", "/", "*"),
	arith("REM", "Integer", "%", "*"),
	def("NEG", true, []string{"Number"}, nil, `
// {{.}}NEG is unary minus; ARITHMETIC_BASE (idA) and INVERT_NEGATIVES (idN)
// both rewrite -x to +x. Every numeric type, unsigned included: -u is legal
// Go and wraps.
func {{.}}NEG[T {{.}}Number](idA, idN int, x T) T {
	if {{.}}Active != 0 && ({{.}}Active == idA || {{.}}Active == idN) {
		{{.}}Reached()
		return +x
	}
	return -x
}
`),
	def("POS", true, []string{"Number"}, nil, `
// {{.}}POS is unary plus; ARITHMETIC_BASE rewrites +x to -x.
func {{.}}POS[T {{.}}Number](id int, x T) T {
	if id != 0 && {{.}}Active == id {
		{{.}}Reached()
		return -x
	}
	return +x
}
`),
	def("IncDec", true, []string{"Number"}, nil, `
func {{.}}IncDec[T {{.}}Number](id int, p *T, inc bool) {
	if id != 0 && {{.}}Active == id {
		{{.}}Reached()
		inc = !inc
	}
	if inc {
		*p++
	} else {
		*p--
	}
}
`),
	def("IncDecMap", true, []string{"Number"}, nil, `
func {{.}}IncDecMap[M ~map[K]V, K comparable, V {{.}}Number](id int, m M, k K, inc bool) {
	if id != 0 && {{.}}Active == id {
		{{.}}Reached()
		inc = !inc
	}
	if inc {
		m[k]++
	} else {
		m[k]--
	}
}
`),
	arith("AND", "Integer", "&", "|"),
	arith("OR", "Integer", "|", "&"),
	arith("XOR", "Integer", "^", "&"),
	arith("ANDNOT", "Integer", "&^", "&"),
	shift("SHL", "<<", ">>"),
	shift("SHR", ">>", "<<"),
}, assignDefs())

// assignDefs returns the op= statement helpers: for each operator, a helper
// on *p and one on m[k], a map entry not being addressable; for += also the
// string helpers, which have no INVERT_ASSIGNMENTS arm (s -= t is never
// viable); for the shifts also the helpers whose count has a type of its
// own, which have no REMOVE_SELF_ASSIGNMENTS arm (x = n is not viable then).
// Their arms are written from assignMutations, one per mutator, in the
// order of the helper's id parameters.
func assignDefs() []helperDef {
	var defs []helperDef
	for _, tok := range assignTokens {
		f := assignForms[tok]
		specs := []assignSpec{{name: f.name + "Assign", tok: tok, mts: f.mts, elem: f.elem}}
		switch tok { //nolint:exhaustive // only += and the shifts have extra helpers
		case token.ADD_ASSIGN:
			specs = append(specs, assignSpec{name: "ADDAssignStr", tok: tok, mts: f.mts[1:], elem: "~string"})
		case token.SHL_ASSIGN, token.SHR_ASSIGN:
			specs = append(specs, assignSpec{name: f.name + "AssignX", tok: tok, mts: f.mts[1:], elem: f.elem, mixed: true})
		}
		for _, sp := range specs {
			defs = append(defs, sp.def(false), sp.def(true))
		}
	}

	return defs
}

// assignSpec is one op= helper, on *p or, as <name>Map, on m[k].
type assignSpec struct {
	name  string
	tok   token.Token
	mts   []mutator.Type
	elem  string // the operand's constraint: a helper constraint's name, or a literal ~T
	mixed bool   // a shift whose count has a type parameter of its own
}

func (s assignSpec) def(onMap bool) helperDef {
	var needs []string
	elem := s.elem
	if !strings.HasPrefix(elem, "~") {
		needs = append(needs, elem)
		elem = "{{.}}" + elem
	}
	tparams, vt := "T "+elem, "T"
	if s.mixed {
		needs = append(needs, "Integer")
		tparams += ", U {{.}}Integer"
		vt = "U"
	}
	name, target, params := s.name, "*p", "p *T, v "+vt
	if onMap {
		name += "Map"
		tparams = "M ~map[K]T, K comparable, " + tparams
		target, params = "m[k]", "m M, k K, v "+vt
	}
	idNames := make([]string, len(s.mts))
	var arms strings.Builder
	for i, mt := range s.mts {
		idNames[i] = assignIDNames[mt]
		to, _ := assignMutation(mt, s.tok)
		fmt.Fprintf(&arms, "\tcase %[1]s != 0 && {{.}}Active == %[1]s:\n\t\t{{.}}Reached()\n\t\t%[2]s %[3]s v\n", idNames[i], target, to)
	}
	src := fmt.Sprintf("\n// {{.}}%[1]s is %[2]s %[3]s v.\nfunc {{.}}%[1]s[%[4]s](%[5]s int, %[6]s) {\n\tswitch {\n%[7]s\tdefault:\n\t\t%[2]s %[3]s v\n\t}\n}\n",
		name, target, s.tok, tparams, strings.Join(idNames, ", "), params, arms.String())

	return def(name, true, needs, nil, src)
}

// ordered is a comparison helper: idB is CONDITIONALS_BOUNDARY, idN is
// CONDITIONALS_NEGATION, either 0 when that mutant is absent.
func ordered(name, op, boundary, negation string) helperDef {
	return def(name, true, nil, []string{"cmp"}, `
func {{.}}`+name+`[T {{.}}cmp.Ordered](idB, idN int, l, r T) bool {
	switch {
	case {{.}}Active != 0 && {{.}}Active == idB:
		{{.}}Reached()
		return l `+boundary+` r
	case {{.}}Active != 0 && {{.}}Active == idN:
		{{.}}Reached()
		return l `+negation+` r
	}
	return l `+op+` r
}
`)
}

// arith is the helper of a binary operator with one mutant at its sites:
// ARITHMETIC_BASE, or INVERT_BITWISE for a bitwise operator.
func arith(name, constraint, op, mutated string) helperDef {
	return def(name, true, []string{constraint}, nil, `
func {{.}}`+name+`[T {{.}}`+constraint+`](id int, l, r T) T {
	if id != 0 && {{.}}Active == id {
		{{.}}Reached()
		return l `+mutated+` r
	}
	return l `+op+` r
}
`)
}

// shift is an INVERT_BITWISE helper for a shift, whose count may be of any
// integer type.
func shift(name, op, mutated string) helperDef {
	return def(name, true, []string{"Integer"}, nil, `
func {{.}}`+name+`[T {{.}}Integer, U {{.}}Integer](id int, l T, r U) T {
	if id != 0 && {{.}}Active == id {
		{{.}}Reached()
		return l `+mutated+` r
	}
	return l `+op+` r
}
`)
}

// HelperSet collects the runtime helpers one package's rewritten files call,
// and generates the file that declares them. The zero value is ready to use.
type HelperSet struct {
	used map[string]bool
	raw  []string
}

// Use marks the fixed helper name (the suffix after the prefix, e.g. "LSS")
// as needed. It panics with ErrUnknownHelper for any other name: a misspelt
// name is a bug in the rewriter, better caught here than as an undefined
// identifier in a package that then fails to build.
func (h *HelperSet) Use(name string) {
	i := slices.IndexFunc(helperDefs, func(d helperDef) bool { return d.public && d.name == name })
	if i < 0 {
		panic(fmt.Errorf("%w: %q (want one of %s)", ErrUnknownHelper, name, helperNames()))
	}
	if h.used == nil {
		h.used = map[string]bool{}
	}
	h.used[name] = true
	for _, n := range helperDefs[i].needs {
		h.used[n] = true
	}
}

// AddRaw appends a per-site helper declaration, already carrying the prefix.
// It may refer to the Active and Reached helpers, which every file declares,
// and to helpers passed to Use, but not to any import: the file imports only
// what the fixed helpers need. Code identical to code already added is
// dropped, so that sites sharing a helper may each add it.
func (h *HelperSet) AddRaw(code string) {
	if !slices.Contains(h.raw, code) {
		h.raw = append(h.raw, code)
	}
}

// File returns the helper file for package pkgName with every generated
// identifier starting with prefix -- import names included, so that a
// package-level cmp, os, sync or strconv in the user's package cannot
// collide with the file's imports. It declares Active and Reached, the
// helpers passed to Use (in a fixed order, whatever the order of the calls)
// and the AddRaw code, and imports only what those need. The result is
// gofmt-formatted. If it does not format -- AddRaw code that does not parse
// -- the error is returned with the unformatted source, for the message.
func (h *HelperSet) File(pkgName, prefix string) ([]byte, error) {
	var body bytes.Buffer
	var imports []string
	for _, d := range helperDefs {
		if d.name != "Active" && d.name != "Reached" && !h.used[d.name] {
			continue
		}
		if err := d.template.Execute(&body, prefix); err != nil {
			panic(err) // the templates are fixed and dot is a string: unreachable
		}
		imports = append(imports, d.imports...)
	}
	for _, r := range h.raw {
		body.WriteString("\n" + r + "\n")
	}
	slices.Sort(imports)
	imports = slices.Compact(imports)

	var out bytes.Buffer
	out.WriteString("// Code generated by gremlins schemata. DO NOT EDIT.\n\npackage " + pkgName + "\n\nimport (\n")
	for _, imp := range imports {
		out.WriteString("\t" + prefix + imp + " \"" + imp + "\"\n")
	}
	out.WriteString(")\n")
	out.Write(body.Bytes())

	src, err := format.Source(out.Bytes())
	if err != nil {
		return out.Bytes(), fmt.Errorf("schemata: helper file for %s: %w", pkgName, err)
	}

	return src, nil
}

// helperNames lists the names Use accepts, for messages.
func helperNames() string {
	var names []string
	for _, d := range helperDefs {
		if d.public {
			names = append(names, d.name)
		}
	}

	return strings.Join(names, ", ")
}
