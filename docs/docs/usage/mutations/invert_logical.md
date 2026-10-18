---
title: Invert logical
---

# Invert logical operators

_Invert logical operators_ will perform inversions on logical operators.

## Mutation table

[//]: # (@formatter:off)

| Orig | Mutation |
|:----:|:--------:|
| &&   | \|\|     |
| \|\| | &&       |


[//]: # (@formatter:on)

## Examples

=== "Original"

    ```go
    a := true && false
    ```

=== "Mutated"

    ```go
    a := true || false
    ```

[//]: # (@formatter:off)
!!! note "These mutants trip `go vet`, and are generated anyway"
    `name == a || name == b` inverted to `&&` is what `go vet`'s `bools` analyzer calls
    "suspect and", and `go test` turns that into `FAIL pkg [build failed]` before any test runs.
    The mutant is legal Go and a real change of behaviour, so Gremlins generates it and runs the
    mutated tests with `-vet=off`. See [about mutations](index.md#mutants-gremlins-does-not-generate).
[//]: # (@formatter:on)
