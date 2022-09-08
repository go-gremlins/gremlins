---
title: Invert loop
---

# Invert loop control

_Invert loop control_ will perform inversions on control operations, which means a `continue` will become a `break`..

## Mutation table

[//]: # (@formatter:off)

|   Orig   | Mutation |
|:--------:|:--------:|
| continue |  break   |
|  break   | continue |

[//]: # (@formatter:on)

## Examples

=== "Original"

    ```go
    for i := 0; i < 3; i++ {
        continue
    }
    ```

=== "Mutated"

    ```go
    for i := 0; i < 3; i++ {
        break
    }
    ```
