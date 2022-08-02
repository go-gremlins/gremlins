---
title: Invert negatives
---

# Invert negatives <small>:material-sign-direction: default</small>

_Invert negatives_ will invert the sign of negative numbers, making them positive.

## Mutation table

|  Original  |  Mutated  |
|:----------:|:---------:|
|     -      |     +     |

## Examples

=== "Original"

    ```go
    func makeNegative(i int) int {
      return -i
    }
    ```

=== "Mutated"

    ```go
    func makeNegative(i int) int {
      return +i
    }
    ```
