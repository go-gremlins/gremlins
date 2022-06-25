# Gremlins

**WARNING: Gremlins is in an early stage of development, and it can be unstable or do anything at all.**

_Gremlins_ is a mutation testing tool for Go projects.

## What is Mutation Testing

Code coverage isn't a reliable measure of test quality. It is too easy to have tests that exercise a piece of code
but don't test anything at all.
_Mutation testing_ works by mutating the code exercised by the tests and verifying if the mutation is caught by
the test suite. Imagine _gremlins_ going into your code and messing around: will your test suit catch their damage?

Here is a nice [intro to mutation testing](https://pedrorijo.com/blog/intro-mutation/)nothing.

## How to use Gremlins

To execute a mutation testing run, from the root of a Go main module execute:

```shell
$ gremlins unleash
```

Gremlins only tests mutations of parts of the code covered by tests. If a mutant is not covered, why bother testing, you
already know will not be caught. In any case, Gremlins will report which mutations aren't covered.

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

### Current limitations

There are some limitations on how Gremlins works right now, but rest assured we'll try to make things better.

- Gremlins can be run only from the root of a Go module and will run all the test suite. This is a problem if the tests
  are especially slow.
- For each mutation, Gremlins will run all the test suite. It would be better to only run the test cases that actually
  cover the mutation.
- There is no way (yet) to implement custom mutations.

## What inspired Gremlins

Mutation testing exists since the early days of computer science, so loads of papers and articles do exists.

Among the existing tools, Gremlins is inspired especially by [PITest](https://pitest.org/).