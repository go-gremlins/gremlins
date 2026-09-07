---
title: Invert loop
---

# Invert loop control

_Invert loop control_ will perform inversions on control operations, which means a `continue` will become a `break`.

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

[//]: # (@formatter:off)
!!! note "Only where the statement is legal"
    `break` and `continue` are not interchangeable everywhere they both parse. A `break` inside a
    `switch` that no loop encloses becomes a `continue` that is not in a loop; a `continue` that is
    what keeps a `for {}` from terminating becomes a `break` that leaves the enclosing function
    missing a return. Gremlins type-checks a candidate mutant before emitting it, so neither is
    generated. See [about mutations](index.md#mutants-gremlins-does-not-generate).
[//]: # (@formatter:on)
