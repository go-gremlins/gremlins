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
