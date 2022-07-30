# Welcome to Gremlins

Gremlins is a mutation testing tool for Go. It has been made to work well on _smallish_ Go modules, for example
_microservices_, on which it helps validate the test suite, aids the TDD process and can be used as a CI quality gate.

As of now, Gremlins doesn't work very well on very big Go modules, mainly because a run can take hours to complete.

## Features

- Discovers mutant candidates and tests them
- Only tests mutants covered by tests
- Supports five mutant types
- Can run in _dry run_ mode
- Yaml-based configuration