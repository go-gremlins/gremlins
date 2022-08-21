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
