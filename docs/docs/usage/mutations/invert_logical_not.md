---
title: Invert logical not
---

# Invert logical not operator

_Invert logical not_ will double-negate boolean expressions by wrapping a NOT operator with another NOT operator.

This mutation tests whether your tests can detect when a boolean negation is effectively neutralized by a double negation.

## Mutation table

[//]: # (@formatter:off)

| Orig | Mutation |
|:----:|:--------:|
| !x   | !!x      |

[//]: # (@formatter:on)

## Examples

=== "Original"

    ```go
    if !condition {
        return
    }
    ```

=== "Mutated"

    ```go
    if !!condition {
        return
    }
    ```

---

=== "Original"

    ```go
    result := !isValid()
    ```

=== "Mutated"

    ```go
    result := !!isValid()
    ```

## Why this mutation matters

Double negation (`!!x`) is logically equivalent to the original value (`x`), effectively canceling out the NOT operator. If your tests pass with this mutation, it indicates:

- The negation might not be necessary in the first place
- Your tests may not be properly validating the boolean logic
- The condition's true/false behavior isn't being tested adequately
