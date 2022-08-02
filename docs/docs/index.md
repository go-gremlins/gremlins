# Welcome to Gremlins

Gremlins is a mutation testing tool for Go. It has been made to work well on _smallish_ Go modules, for example
_microservices_, on which it helps validate the test suite, aids the TDD process and can be used as a CI quality gate.

As of now, Gremlins doesn't work very well on very big Go modules, mainly because a run can take hours to complete.

## What is Mutation Testing

Code coverage is unreliable as a measure of test quality. It is too easy to have tests that exercise a piece of code but
don't test anything at all. Mutation testing works by mutating the code exercised by the tests and verifying if the
mutation is caught by the test suite. Imagine gremlins going into your code and messing around: will your test suit
catch their damage?

## Features

- Discovers mutant candidates and tests them
- Only tests mutants covered by tests
- Supports five mutant types
- Yaml-based configuration
- Can run as quality gate on CI