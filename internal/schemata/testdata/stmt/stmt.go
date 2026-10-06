// Package stmt is the fixture for TestStmtFormsBehave: one function per
// schemata statement form. Every plain token mutant the engine's type check
// admits compiles.
package stmt

type counts map[string]int

// Block position: the one-line switch form.

func AddAssign(x, y int) int { x += y; return x }

func SubAssign(x, y int) int { x -= y; return x }

func MulAssign(x, y int) int { x *= y; return x }

func QuoAssign(x, y int) int { x /= y; return x }

func RemAssign(x, y int) int { x %= y; return x }

func QuoAssignFloat(x, y float64) float64 { x /= y; return x }

func AndAssignTwoMutants(x, y int) int { x &= y; return x }

func OrAssign(x, y int) int { x |= y; return x }

func XorAssign(x, y uint8) uint8 { x ^= y; return x }

func AndNotAssign(x, y int) int { x &^= y; return x }

func ShlAssignSameType(x, n uint) uint { x <<= n; return x }

func ShrAssignSameType(x, n int) int { x >>= n; return x }

// ShlAssignMixedType has no REMOVE_SELF_ASSIGNMENTS mutant: x = n does not
// compile.
func ShlAssignMixedType(x uint64, n uint8) uint64 { x <<= n; return x }

// AddAssignString has no INVERT_ASSIGNMENTS mutant: s -= t does not compile.
func AddAssignString(s, t string) string { s += t; return s }

// AddAssignSideEffectLHS evaluates the index once, whichever arm runs.
func AddAssignSideEffectLHS() ([]int, int) {
	a := []int{1, 2, 3}
	n := 0
	next := func() int { n++; return n }
	a[next()] += 2
	return a, n
}

// AddAssignSideEffectRHS evaluates the value once, whichever arm runs.
func AddAssignSideEffectRHS() (int, int) {
	n := 0
	f := func() int { n++; return 5 }
	x := 1
	x += f()
	return x, n
}

// InCase has its sites in case and comm clause bodies.
func InCase(x, y int) int {
	switch x {
	case 1:
		x *= y
	default:
		x -= y
	}
	select {
	default:
		x |= 1
	}
	return x
}

// LabelledAssign keeps its label on the switch.
func LabelledAssign(skip bool) int {
	x := 1
	if skip {
		goto L
	}
	x = 10
L:
	x += 3
	return x
}

// NestedSiteInLHS has the + site inside every arm.
func NestedSiteInLHS(i int) []int {
	a := []int{0, 0, 0, 0}
	a[i+1] += 2
	return a
}

// MultiLine cannot take the one-line switch: the helper form keeps its lines.
func MultiLine(x, y int) int {
	x +=
		y
	return x
}

// MapAssignNamedMap has a side-effecting key, in a block and in an if
// statement's init.
func MapAssignNamedMap() (counts, int) {
	m := counts{"a": 1}
	n := 0
	key := func() string { n++; return "a" }
	m[key()] += 2
	ok := true
	if m[key()] *= 3; ok {
		m["b"] = m["a"]
	}
	return m, n
}

// Simple-statement position: the helper forms.

// ForPostAddAssign captures the Go 1.22 per-iteration loop variable; the
// length guard ends the loop when a mutant stops it counting up to 10.
func ForPostAddAssign() []int {
	var fns []func() int
	for i := 0; i < 10; i += 3 {
		if len(fns) > 5 {
			break
		}
		fns = append(fns, func() int { return i })
	}
	var out []int
	for _, f := range fns {
		out = append(out, f())
	}
	return out
}

func IfInitAssign(x int) int {
	if x += 1; x > 2 {
		return x
	}
	return 0
}

// SimpleInt has every integer op-assign in an if statement's init, on a
// variable and on a map entry.
func SimpleInt(x, y int) ([]int, map[int]int) {
	ok := true
	v := [10]int{x, x, x, x, x, x, x, x, x, x}
	m := map[int]int{}
	for i := range 10 {
		m[i] = x
	}
	if v[0] += y; ok {
	}
	if v[1] -= y; ok {
	}
	if v[2] *= y; ok {
	}
	if v[3] /= y; ok {
	}
	if v[4] %= y; ok {
	}
	if v[5] &= y; ok {
	}
	if v[6] |= y; ok {
	}
	if v[7] ^= y; ok {
	}
	if v[8] &^= y; ok {
	}
	if m[0] += y; ok {
	}
	if m[1] -= y; ok {
	}
	if m[2] *= y; ok {
	}
	if m[3] /= y; ok {
	}
	if m[4] %= y; ok {
	}
	if m[5] &= y; ok {
	}
	if m[6] |= y; ok {
	}
	if m[7] ^= y; ok {
	}
	if m[8] &^= y; ok {
	}
	return v[:], m
}

// SimpleShift has the shifts in a switch statement's init, with a count of
// the value's type and of another.
func SimpleShift(x, n uint, c uint8) ([]uint, map[int]uint) {
	v := [4]uint{x, x, x, x}
	m := map[int]uint{0: x, 1: x, 2: x, 3: x}
	switch v[0] <<= n; {
	}
	switch v[1] >>= n; {
	}
	switch v[2] <<= c; {
	}
	switch v[3] >>= c; {
	}
	switch m[0] <<= n; {
	}
	switch m[1] >>= n; {
	}
	switch m[2] <<= c; {
	}
	switch m[3] >>= c; {
	}
	return v[:], m
}

// SimpleShiftConst has the shifts in a switch statement's init and over
// several lines with a constant count, which the helper takes as the value's
// type: untyped, typed, and a rune. Each has its REMOVE_SELF_ASSIGNMENTS
// mutant, x = count, the constant converted to the value's type.
func SimpleShiftConst(x uint, b uint8) ([]uint, map[int]uint, uint8, uint8) {
	const k uint = 3
	v := [4]uint{x, x, x, x}
	m := map[int]uint{0: x, 1: x, 2: x}
	switch v[0] <<= 2; {
	}
	switch v[1] >>= 1; {
	}
	switch v[2] <<= k; {
	}
	switch v[3] >>= '\x01'; {
	}
	switch m[0] <<= 2; {
	}
	switch m[1] >>= k; {
	}
	m[2] <<=
		1
	switch b <<= 7; {
	}
	c := b + 5
	c >>=
		2
	return v[:], m, b, c
}

// SimpleFloat and SimpleString cover the other element types.
func SimpleFloat(x, y float64) (float64, map[string]float64) {
	m := map[string]float64{"k": x}
	for x /= y; ; {
		break
	}
	for done := false; !done; m["k"] *= y {
		done = true
	}
	return x, m
}

func SimpleString(s, t string) (string, counts2) {
	m := counts2{"k": s}
	for s += t; ; {
		break
	}
	for m["k"] += t; ; {
		break
	}
	return s, m
}

type counts2 map[string]string
