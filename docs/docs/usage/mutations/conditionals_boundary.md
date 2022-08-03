---
title: Conditionals boundary
---

# Conditionals boundary <small>:material-sign-direction: default</small>

_Conditionals boundaries_ modify the boundary of a conditional, which means that exclusive/inclusive ranges will be
inverted.

## Mutation table

|  Original  |  Mutated  |
|:----------:|:---------:|
|     \>     |    \>=    |
|    \>=     |    \>     |
|     <      |    <=     |
|     <=     |     <     |

## Examples

=== "Original"
    ```go
    if a > b {
      // Do something
    }
    ```

=== "Mutated"

    ```go
    if a >= b {
      // Do something
    }
    ```
