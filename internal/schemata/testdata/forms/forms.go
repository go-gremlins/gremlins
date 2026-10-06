// Package forms is the fixture for TestFormsBehave: one function per
// schemata rewrite form. Every plain token mutant of every site compiles.
package forms

type celsius int

type ord interface{ ~int | ~float64 | ~string }

type num interface{ ~int | ~float64 }

type box struct{ n int }

type myErr struct{}

func (*myErr) Error() string { return "myErr" }

func Eq(a, b int) bool { return a == b }

func Neq(s, t string) bool { return s != t }

// TypedNil compares an interface holding a typed nil pointer with nil.
func TypedNil() bool {
	var p *myErr
	var err error = p
	return err != nil
}

// Recovered needs recover() to stay a direct call of the deferred function.
func Recovered() (caught bool) {
	defer func() {
		caught = recover() != nil
	}()
	panic("boom")
}

func Less(a, b int) bool { return a < b }

func LessEq(a, b float64) bool { return a <= b }

// Hot compares an unexported named type with an untyped constant.
func Hot(c celsius) bool { return c > 30 }

func GreaterEq(s, t string) bool { return s >= t }

func GMax[T ord](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func GSum[T num](xs []T) T {
	var s T
	for _, x := range xs {
		s = s + x
	}
	return s
}

func Add(a, b int) int { return a + b }

func Sub(a, b float64) float64 { return a - b }

func Mul(a, b int) int { return a * b }

func Quo(a, b int) int { return a / b }

func Rem(a, b int) int { return a % b }

func Neg(x int) int { return -x }

func NegU(u uint8) uint8 { return -u }

func Pos(x float64) float64 { return +x }

func MultiLine(a, b int) int {
	return a +
		b
}

func Commented(a, b int) bool { return a < /* gap */ b }

func Nested(a, b, c int) bool { return a+b < c }

// NestedEq has a site (a+1) inside an operand of another (==).
func NestedEq(a, b int) bool { return a+1 == b }

// GAddOne adds an untyped constant to a type-parameter operand.
func GAddOne[T num](x T) T { return x + 1 }

func bump(p *int) int {
	*p = *p + 5
	return 3
}

// Order reads x in the same expression as a call that changes it.
func Order() (bool, int) {
	x := 1
	r := x < bump(&x)
	return r, x
}

func IncIdent() int {
	n := 1
	n++
	return n
}

func DecField() int {
	b := box{n: 5}
	b.n--
	return b.n
}

func IncSlice(xs []int, i int) []int {
	xs[i]++
	return xs
}

func IncArray() [3]int {
	var a [3]int
	a[1]++
	return a
}

func IncPtrArray(pa *[2]int) *[2]int {
	pa[0]++
	return pa
}

func IncDeref() int {
	v := 7
	p := &v
	*p++
	return v
}

func DecParen() float64 {
	f := 1.5
	(f)--
	return f
}

var keyCalls int

func key() string {
	keyCalls++
	return "k"
}

// IncMap increments a map entry whose key has a side effect.
func IncMap() (map[string]int, int) {
	keyCalls = 0
	m := map[string]int{}
	m[key()]++
	return m, keyCalls
}

func IncNilMap() {
	var m map[string]int
	m["a"]++
}

func GInc[S ~[]E, E num](s S) S {
	s[1]++
	return s
}

// Closures captures the Go 1.22 per-iteration loop variable; the length
// guard ends the loop when a mutant stops it counting up to 3.
func Closures() []int {
	var fns []func() int
	for i := 0; i < 3; i++ {
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

type mask uint16

type flag bool

func BitInt(a, b int) []int { return []int{a & b, a | b, a ^ b, a &^ b} }

func BitU8(a, b uint8) []uint8 { return []uint8{a & b, a | b, a ^ b, a &^ b} }

func BitMask(a, b mask) []mask { return []mask{a & b, a | b, a ^ b, a &^ b} }

func ShiftU64(x uint64, n int) []uint64 { return []uint64{x << n, x >> n} }

func ShiftI8(x int8, n uint) []int8 { return []int8{x << n, x >> n} }

// ShlUntypedLeft shifts an untyped constant, which takes the type int64
// from its context.
func ShlUntypedLeft(n uint) int64 {
	var x int64 = 1 << n
	return x
}

var ticks int

func tick(b bool) bool {
	ticks++
	return b
}

// LandShort and LorShort count whether the right operand was evaluated.
func LandShort(a, b bool) (bool, int) {
	ticks = 0
	r := a && tick(b)
	return r, ticks
}

func LorShort(a, b bool) (bool, int) {
	ticks = 0
	r := a || tick(b)
	return r, ticks
}

func LandNamed(a, b flag) flag { return a && b }

func MaskTest(a, b int, c bool) bool { return a&b == 0 && c }

// The comparisons below take the named type flag from their contexts, as
// cel-go's types.Bool(v == id) does; FlagConv is that site.
func FlagConv(v, id string) any { return flag(v == id) }

func FlagConvLess(a, b int) any { return flag((a < b)) }

func FlagReturn(a, b float64) flag { return a <= b }

func FlagAssign(a, b int) flag {
	var f flag
	f = a != b
	return f
}

func FlagVar(a, b int) flag {
	var f flag = a > b
	return f
}

func flagID(f flag) flag { return f }

func FlagArg(a, b int) flag { return flagID(a >= b) }

type flagBox struct{ f, g flag }

func FlagField(a, b int) flagBox { return flagBox{f: a == b, g: a < b} }

func FlagSlice(a, b int) []flag { return []flag{a != b, a <= b} }

func FlagMap(a, b int) map[flag]flag { return map[flag]flag{a > b: a == b} }

func FlagSend(a, b int) flag {
	c := make(chan flag, 1)
	c <- a >= b
	return <-c
}

// FlagOperand compares a flag with a comparison, itself in a flag context.
func FlagOperand(f flag, a, b int) flag { return f == (a < b) }

func FlagParam[P ~bool](a, b int) P { return a == b }

func FlagLocal(a, b string) any {
	type local bool
	var x local = a < b
	return x
}
