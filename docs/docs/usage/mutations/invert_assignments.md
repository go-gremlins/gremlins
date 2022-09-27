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
