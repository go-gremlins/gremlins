---
title: Invert bitwise
---

# Invert bitwise

_Invert bitwise_ will perform inversions on basic bit operations.

## Mutation table

| Original | Mutated |
|:--------:|:-------:|
|    &     |    \|   |
|    \|    |    &    |
|    ^     |    &    |
|    &^    |    &    |
|    >>    |   <<    |
|    <<    |   >>    |

## Examples

=== "Original"

    ```go
    a := 1 & 2
    ```

=== "Mutated"

    ```go
    a := 1 | 2
    ```

[//]: # (@formatter:off)
!!! note "Only the infix operators"
    Go spells `&` and `^` the same in prefix position, where they mean
    address-of and bitwise complement rather than AND and XOR. Applying the
    table above to those would produce source no compiler accepts — `&Foo{}`
    becomes `|Foo{}`, which does not parse, and `^u` becomes `&u`, which no
    longer has the operand's type. A mutant a compiler rejects is not a fault
    the tests failed to catch, so Gremlins does not generate one: `&x` and `^x`
    are left alone, while `-x` and `+x`, whose prefix meaning is the same
    arithmetic as their infix one, are still mutated by
    [arithmetic base](arithmetic_base.md) and
    [invert negatives](invert_negatives.md).
[//]: # (@formatter:on)
