---
title: Invert logical operators
---

# Invert logical operators

_Invert logical operators_ will perform inversions on logical operators.

## Mutation table

|  Original  |  Mutated  |
|:----------:|:---------:|
|    &&      |    ||     |
|    ||      |    &&     |

## Examples

=== "Original"

    ```go
    a := true && false
    ```

=== "Mutated"

    ```go
    a := true || false
    ```
