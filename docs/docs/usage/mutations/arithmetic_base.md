---
title: Arithmetic base
---

# Arithmetic base <small>:material-sign-direction: default</small>

_Arithmetic base_ will perform inversions on basic arithmetic operations.

## Mutation table

|  Original  |  Mutated  |
|:----------:|:---------:|
|     +      |     -     |
|     -      |     +     |
|     *      |     /     |
|     /      |     *     |
|     %      |     *     |

## Examples

=== "Original"

    ```go
    a := 1 + 2
    ```

=== "Mutated"

    ```go
    a := 1 - 2
    ```

[//]: # (@formatter:off)
!!! note "Only where the operands allow it"
    Go's `+` is also string concatenation, and no other arithmetic operator is defined on strings.
    Gremlins type-checks a candidate mutant before emitting it, so `a + "-" + b` yields no
    `ADD -> SUB` mutant, and a `MUL -> QUO` that turns a constant into zero yields none either when
    something divides by that constant. See [about mutations](index.md#mutants-gremlins-does-not-generate).
[//]: # (@formatter:on)
