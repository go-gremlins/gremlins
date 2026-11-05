# Quick start

To execute a mutation test run, from the root of a Go module execute:

```sh
$ gremlins unleash #(1)
```

1. If `unleash` is too long to type for you, you can use `run` or `r` which will do the same.

Gremlins only tests mutations of parts of the code already covered by test cases. If a mutant is not covered, why bother
testing? You already know it will not be caught. In any case, Gremlins will report which mutations aren't covered.

Go coverage profiles do not record the expressions of `switch` and `select` clauses. Gremlins finds these expressions
from the clause bodies that the tests ran:

- For each case body that ran, Gremlins finds the entry case. It moves back while the previous case body ran and ends
  with `fallthrough`.
- The first expression of the entry case is covered. All expressions of the cases before it are covered.
- All case expressions are covered when the `default` body ran and the previous case body did not fall through to it.
- The channel and value expressions of a `select` statement are covered when one of its clause bodies ran.

Gremlins will report each mutation as:

- `RUNNABLE`: In _dry-run_ mode, a mutation that can be tested.
- `NOT COVERED`: A mutation not covered by tests; it will not be tested.
- `KILLED`: The mutation has been caught by the test suite.
- `LIVED`: The mutation hasn't been caught by the test suite.
- `TIMED OUT`: The tests timed out while testing the mutation: the mutation actually made the tests fail, but not
  explicitly.
- `NOT VIABLE`: The mutation makes the build fail.
