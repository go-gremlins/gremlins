# Quick start

To execute a mutation test run, from the root of a Go module execute:

```sh
$ gremlins unleash #(1)
```

1. If `unleash` is too long to type for you, you can use `run` or `r` which will do the same.

Gremlins uses Go's coverage profile to avoid testing mutations in code that the test suite does not execute. It also
tests mutations in constant declarations, which Go coverage cannot instrument, and switch case conditions whose case
body is covered. Gremlins reports the remaining mutations as not covered.

Gremlins will report each mutation as:

- `RUNNABLE`: In _dry-run_ mode, a mutation that can be tested.
- `NOT COVERED`: A mutation not covered by tests; it will not be tested.
- `KILLED`: The mutation has been caught by the test suite.
- `LIVED`: The mutation hasn't been caught by the test suite.
- `TIMED OUT`: The tests timed out while testing the mutation: the mutation actually made the tests fail, but not
  explicitly.
- `NOT VIABLE`: The mutation makes the build fail.
