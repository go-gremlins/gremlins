---
title: Increment decrement
---

# Increment decrement <small>:material-sign-direction: default</small>

_Increment decrement_ will invert the sign of the increment or decrement.

## Mutation table

|  Original  |  Mutated  |
|:----------:|:---------:|
|     ++     |    --     |
|     --     |    ++     |

## Examples

=== "Original"
    ```go
    for i := 0; i < 10; i++ {
      // Do something
    }
    ```

=== "Mutated"

    ```go
    for i := 0; i < 10; i-- {
      // Do something
    }
    ```
