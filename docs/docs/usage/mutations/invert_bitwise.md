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
