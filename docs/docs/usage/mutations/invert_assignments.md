---
title: Invert assignments
---

# Invert assignments

_Invert assignments_ will perform inversions on basic arithmetic operations, and it assigns the result of the two left
and right operands to the left operand.

## Mutation table

| Original | Mutated |
|:--------:|:-------:|
|    +=    |   -=    |
|    -=    |   +=    |
|    *=    |   /=    |
|    /=    |   *=    |
|    %=    |   *=    |

## Examples

=== "Original"

    ```go
    a := 1
    a *= 2
    ```

=== "Mutated"

    ```go
    a := 1
    a /= 2
    ```

[//]: # (@formatter:off)
!!! note "Only where the operands allow it"
    `s += suffix` on a string has no `-=` counterpart: Go defines `+` on strings and nothing else.
    Gremlins type-checks a candidate mutant before emitting it, so that mutation is not generated.
    `REMOVE_SELF_ASSIGNMENTS` still applies at the same site. See
    [about mutations](index.md#mutants-gremlins-does-not-generate).
[//]: # (@formatter:on)
