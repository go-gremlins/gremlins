# Conditionals negation

_Conditionals negation_ inverts the conditional direction, which means a `==` will become a `!=`.

## Mutation table

|  Original  |  Mutated  |
|:----------:|:---------:|
|     ==     |    !=     |
|     !=     |    ==     |
|     \>     |    \<=    |
|     <=     |    \>     |
|     <      |    \>=    |
|    \>=     |     <     |

## Examples

=== "Original"
    ```go
    if a == b {
      // Do something
    }
    ```

=== "Mutated"

    ```go
    if a != b {
      // Do something
    }
    ```
