---
title: Remove self-assignments
---

# Remove self-assignments

_Remove self-assignments_ will perform a trivial assignment, instead of assigning the result of the two left
and right operands to the left operand.

## Mutation table

|  Original  | Mutated |
|:----------:|:-------:|
|     +=     |    =    |
|     -=     |    =    |
|     *=     |    =    |
|     /=     |    =    |
|     %=     |    =    |
|     &=     |    =    |
|  &#124;=   |    =    |
|     ^=     |    =    |
|    <<=     |    =    |
|    \>>=    |    =    |
|    &^=     |    =    |

## Examples

=== "Original"

    ```go
    a := 1
    a += 2
    ```

=== "Mutated"

    ```go
    a := 1
    a = 2
    ```
