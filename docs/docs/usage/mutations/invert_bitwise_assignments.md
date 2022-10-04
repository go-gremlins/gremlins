---
title: Invert bitwise assignments
---

# Invert bitwise assignments

_Invert bitwise assignments_ will perform inversions on basic bit operations, and it assigns the result of the two left
and right operands to the left operand.

## Mutation table

| Original | Mutated |
|:--------:|:-------:|
|    &=    |   \|=   |
|    \|=   |    &=   |
|    ^=    |   &=    |
|   &^=    |   &=    |
|   >>=    |   <<=   |
|   <<=    |   >>=   |

## Examples

=== "Original"

    ```go
    a := 1
    a &= 1
    ```

=== "Mutated"

    ```go
    a := 1
    a |= 1
    ```
