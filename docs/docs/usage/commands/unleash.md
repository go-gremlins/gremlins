# Unleash

The main command used in Gremlins is `unleash`, that _unleashes_ the _gremlins_ and starts a mutation test of your code.
If `unleash` is too long to type for you, you can use its aliases `run` and `r`.

!!! warning
    At this time, this only works in the root of a Go module (where the `go.mod` file resides).

To execute a mutation testing run just type

```shell
gremlins unleash
```

If the module build requires tags

```shell
gremlins unleash --tags "tag1,tag2"
```

## Flags

`unleash` supports several flags to fine tune its behaviour.

### Dry run
:material-flag:`--dry-run` · :material-sign-direction: Default: false
  
Just performs the analysis but not the mutation testing.

```shell
gremlins unleash --dry-run
```

### Tags
:material-flag: `--tags`/`-t` · :material-sign-direction: Default: empty


Sets the `go` command build tags.

```shell
gremlins unleash --tags "tag1,tag2"
```

### Output

:material-flag: `--output`/`-o` · :material-sign-direction: Default: empty

When set, Gremlins will write the give output file with machine readable results.

```shell
gremlins unleash --output=output.json
```

The output file in in JSON format and has the following structure:

```json
{
  "go_module": "github.com/go-gremlins/gremlins",
  "test_efficacy": 82.00, //(1)
  "mutations_coverage": 80.00, //(2)
  "mutants_total": 100,
  "mutants_killed": 82,
  "mutants_lived": 8,
  "mutants_not_viable": 2, //(3)
  "mutants_not_covered": 10,
  "elapsed_time": 123.456, //(4)
  "files": [
    {
      "file_name": "myFile.go",
      "mutations": [
        {
          "line": 10,
          "column": 8,
          "type": "CONDITIONALS_NEGATION",
          "status": "KILLED"
        }
      ]
    }
  ]
}
```

1. This is a percentage expressed as floating point number.
2. This is a percentage expressed as floating point number.
3. NOT VIABLE mutants are excluded from all the calculations.
4. The elapsed time is expressed in seconds, expressed as floating point number.

!!! warning
    The JSON output file is not _pretty printed_; it is optimised for machine reading.

### Threshold efficacy
:material-flag: `--threshold-efficacy` · :material-sign-direction: Default: 0

When set, it makes Gremlins exit with an error (code 10) if the _test efficacy_ threshold is not met. By default it is
zero, which
means Gremlins never exits with an error.

The _test efficacy_ is calculated as `KILLED / (KILLED + LIVED)` and assesses how effective are the tests.

```shell
gremlins unleash --threshold-efficacy 80
```

### Threshold mutant coverage
:material-flag: `--threshold-mcover` · :material-sign-direction: Default: 0

When set, it makes Gremlins exit with an error (code 11) if the _mutant coverage_ threshold is not met. By default
it is zero, which means Gremlins never exits with an error.

The _mutant coverage_ is calculated as `(KILLED + LIVED) / (KILLED + LIVED + NOT_COVERED)` and assesses how many mutants
are covered by tests.

```shell
gremlins unleash --threshold-mcover 80
```

### Arithmetic base
:material-flag: `--arithmetic-base` · :material-sign-direction: Default: `true`

Enables/disables the [ARITHMETIC BASE](../mutations/arithmetic_base.md) mutant type.

```shell
gremlins unleash --arithmetic-base=false
```

### Conditionals-boundary
:material-flag: `--conditionals-boundary` · :material-sign-direction: Default: `true`

Enables/disables the [CONDITIONALS BOUNDARY](../mutations/conditionals_boundary.md) mutant type.

```shell
gremlins unleash --conditionals_boundary=false
```

### Conditionals negation
:material-flag: `--conditionals-negation` · :material-sign-direction: Default: `true`

Enables/disables the [CONDITIONALS NEGATION](../mutations/conditionals_negation.md) mutant type.

```shell
gremlins unleash --conditionals_negation=false
```

### Increment decrement
:material-flag: `--increment-decrement` · :material-sign-direction: Default: `true`

Enables/disables the [INCREMENT DECREMENT](../mutations/increment_decrement.md) mutant type.

```shell
gremlins unleash --increment-decrement=false
```

### Invert negatives
:material-flag: `--invert-negatives` · :material-sign-direction: Default: `true`

Enables/disables the [INVERT NEGATIVES](../mutations/invert_negatives.md) mutant type.

```shell
gremlins unleash --invert_negatives=false
```
