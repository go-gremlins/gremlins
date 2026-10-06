package a

// Add sums x and y.
func Add(x, y int) int { return x + y }

// Bump adds one to m["a"] in a for statement's post: a site the rewrite does
// not support (a map entry of type-parameter type in a simple statement).
func Bump[M ~map[string]int](m M) int {
	for done := false; !done; m["a"] += 1 {
		done = true
	}

	return m["a"]
}
