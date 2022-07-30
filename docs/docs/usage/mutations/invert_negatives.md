# Invert negatives

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
