# Unleash

The main command used in Gremlins is `unleash`, that _unleashes_ the _gremlins_ and start a mutation test of your code.
If `unleash` is too long to type for you, you can use its aliases `run` and `r`.

!!! warning
    At this time, this only works in the root of a Go module (where the `go.mod` file resides).

To execute a mut
ation testing run just type

```shell
gremlins unleash
```

If the module build requires tags

```shell
gremlins unleash --tags "tag1,tag2"
```

## Flags

`unleash` supports several flags to fine tune its behaviour.

### `--dry-run`

Just performs the analysis but not the mutation testing.

```shell
gremlins unleash --dry-run
```

### `--tags`

### `--threshold-efficacy`

### `--threshold-mcover`

### `--arithmetic-base`

### `--conditionals-boundary`

### `--conditionals-negation`

### `--increment-decrement`

### `--invert-negatives`