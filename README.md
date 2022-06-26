# Gremlins

[![Tests](https://github.com/k3rn31/gremlins/actions/workflows/ci.yml/badge.svg)](https://github.com/k3rn31/gremlins/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/k3rn31/gremlins)](https://goreportcard.com/report/github.com/k3rn31/gremlins)
[![codecov](https://codecov.io/gh/k3rn31/gremlins/branch/main/graph/badge.svg?token=MICF9A6U3J)](https://codecov.io/gh/k3rn31/gremlins)

**WARNING: Gremlins is in an early stage of development, and it can be unstable or do anything at all. As of now, it
only works in dry-run mode, which means it identifies mutation candidates, but still doesn't perform the mutation
testing.**

Gremlins is a mutation testing tool for Go.

## What is Mutation Testing

Code coverage unreliable to measure test quality. It is too easy to have tests that exercise a piece of code but don't
test anything at all.
_Mutation testing_ works by mutating the code exercised by the tests and verifying if the mutation is caught by
the test suite. Imagine _gremlins_ going into your code and messing around: will your test suit catch their damage?

Here is a nice [intro to mutation testing](https://pedrorijo.com/blog/intro-mutation/)nothing.

## How to use Gremlins

To execute a mutation testing run, from the root of a Go main module execute:

```shell
$ gremlins unleash
```

Gremlins only tests mutations of parts of the code already covered by test cases. If a mutant is not covered, why bother
testing? You already know it will not be caught. In any case, Gremlins will report which mutations aren't covered.

### Supported mutations

- [x] Conditionals boundary
- [ ] Increments
- [ ] Invert negatives
- [ ] Math
- [ ] Negate conditionals

#### Conditional Boundaries

| Original | Mutated |
|----------|---------|
| \>       | \>=     |
| \>=      | \>      |
| <        | <=      |
| <=       | <       |

Example:

```go
if a > b {
// Do something
}
```

will be changed to

```go
if a < b {
// Do something
}
```

### Current limitations

There are some limitations on how Gremlins works right now, but rest assured we'll try to make things better.

- Gremlins can be run only from the root of a Go module and will run all the test suite. This is a problem if the tests
  are especially slow.
- For each mutation, Gremlins will run all the test suite. It would be better to only run the test cases that actually
  cover the mutation.
- Gremlins doesn't support custom test commands; if you have to do anything different from `go test ./...` to run your
  test suite, most probably it will not work with Gremlins.
- There is no way to implement custom mutations.

## What inspired Gremlins

Mutation testing exists since the early days of computer science, so loads of papers and articles do exists. Gremlins is
inspired from those.

Among the existing mutation testing tools, Gremlins is inspired especially by [PITest](https://pitest.org/).

### Other Mutation Testing tools for Go

There is not much around, except from:

- [go-mutesting](https://github.com/avito-tech/go-mutesting#list-of-mutators)
