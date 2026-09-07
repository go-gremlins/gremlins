# Unleash

The main command used in Gremlins is `unleash`, that _unleashes_ the _gremlins_ and starts a mutation test of your code.
If `unleash` is too long to type for you, you can use its aliases `run` and `r`.

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

### Arithmetic base

:material-flag: `--arithmetic-base` · :material-sign-direction: Default: `true`

Enables/disables the [ARITHMETIC BASE](../../mutations/arithmetic_base.md) mutant type.

```shell
gremlins unleash --arithmetic-base=false
```

### Conditionals-boundary

:material-flag: `--conditionals-boundary` · :material-sign-direction: Default: `true`

Enables/disables the [CONDITIONALS BOUNDARY](../../mutations/conditionals_boundary.md) mutant type.

```shell
gremlins unleash --conditionals_boundary=false
```

### Conditionals negation

:material-flag: `--conditionals-negation` · :material-sign-direction: Default: `true`

Enables/disables the [CONDITIONALS NEGATION](../../mutations/conditionals_negation.md) mutant type.

```shell
gremlins unleash --conditionals_negation=false
```

### Cover packages

:material-flag: `--coverpkg` · :material-sign-direction: Default: empty

Apply coverage analysis in each test to packages matching the patterns.
The default is for each test to analyze only the package being tested.

```shell
gremlins unleash --coverpkg "./internal/...,./pkg/..."
```

### Exclude files

:material-flag: `--exclude-files/-E` · :material-sign-direction: Default: empty

Allows to exclude generated or not important files.

If a file path matches a regular expression, it is skipped from execution and threshold calculation.

The default is to skip only test files.

```shell
gremlins unleash --exclude-files "_(gen|wrap).go"
```

You can provide a few rules. File is skipped if matches any regexp.

```shell
gremlins unleash -E "_(gen|wrap).go$" -E "^(generate|wrap)/" -E "internal/super_old/"
```

### Diff

:material-flag: `--diff`/`-D` · :material-sign-direction: Default: empty

Run tests only for mutants inside code changes between current state and git reference (branch or commit).
The default is each mutant covered by tests.

#### Branch merge base

```shell
gremlins unleash --diff "origin/main"
```

#### Commit

```shell
gremlins unleash --diff "b62af323"
```

#### PR

```shell
gremlins unleash --diff "origin/$GITHUB_BASE_REF"
```

Use `actions/checkout@v4` with `fetch-depth: 0` to fetch all history.

### Dry run

:material-flag:`--dry-run`/`-d` · :material-sign-direction: Default: false

Just performs the analysis but not the mutation testing.

```shell
gremlins unleash --dry-run
```

### Statuses output

:material-flag: `--output-statuses`/`-S` · :material-sign-direction: Default: empty - show all

Filters stdout to print only statuses from flag. Useful to filter important findings in big project output.
Alternative to `gremlins r | grep LIVED` configured from file.

Flag do not change json file and stats report content.

### Examples

#### Show only `LIVED` and `NOT COVERED`

```shell
gremlins unleash --output-statuses "lc"
```

Output

```
       LIVED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3
 NOT COVERED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3
```

#### Filter out out `SKIPPED`, `KILLED`.

```shell
gremlins unleash --S lctv
```

Output

```
       LIVED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3
 NOT COVERED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3
  NOT VIABLE CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3
   TIMED OUT CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3
```

### Filter letters

- `l` - LIVED
- `c` - NOT COVERED
- `t` - TIMED OUT
- `k` - KILLED
- `v` - NOT VIABLE
- `s` - SKIPPED
- `r` - RUNNABLE

### Diff statuses output

:material-flag: `--output-diff-statuses` · :material-sign-direction: Default: empty - no diffs shown

Prints a unified diff of the original vs mutated code snippet for mutants whose status matches
the filter. Only `l` (LIVED) and `k` (KILLED) are accepted — other statuses are not valid because
diffs are only meaningful for mutants that were actually executed and had their source changed.

This flag works in combination with `--output-statuses`: a mutant that is hidden by
`--output-statuses` will produce no output at all — no status line and no diff.

#### Examples

##### Show diff for survived mutants

```shell
gremlins unleash --output-diff-statuses l
```

Output

```
       LIVED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3
       -x > y
       +x >= y
```

##### Show diff for both lived and killed mutants

```shell
gremlins unleash --output-diff-statuses lk
```

### Filter letters

- `l` - LIVED
- `k` - KILLED

### Increment decrement

:material-flag: `--increment-decrement` · :material-sign-direction: Default: `true`

Enables/disables the [INCREMENT DECREMENT](../../mutations/increment_decrement.md) mutant type.

```shell
gremlins unleash --increment-decrement=false
```

### Integration mode

:material-flag:`--integration`/`-i` · :material-sign-direction: Default: false

In _normal mode_, Gremlins executes only the tests of the packages where the mutant is found.
This is done to optimize the performance, running less test cases for each mutation.

The drawback of this approach lies in the fact that if a mutation in a package influences the tests
of another package, this is not caught by Gremlins. In general, this is an acceptable drawback
because packages should rely on their own tests, not on the tests of other packages.

Nonetheless, there may be cases where you may want to run all the test suite for each mutation, for
example if you are analysing integration or E2E tests. In this scenario, you can enable _integration mode_.
However, you should be aware that integration mode is generally much slower, and you can also get
slightly different results depending on your test suite.

```shell
gremlins unleash --integration
```

### Invert assignments

:material-flag: `--invert-assignments` · :material-sign-direction: Default: `false`

Enables/disables the [INVERT ASSIGNMENTS](../../mutations/invert_assignments.md) mutant type.

```shell
gremlins unleash --invert-assignments
```

### Invert bitwise

:material-flag: `--invert-bitwise` · :material-sign-direction: Default: `false`

Enables/disables the [INVERT BITWISE](../../mutations/invert_bitwise.md) mutant type.

```shell
gremlins unleash --invert-bitwise
```

### Invert bitwise assignments

:material-flag: `--invert-bwassign` · :material-sign-direction: Default: `false`

Enables/disables the [INVERT BWASSIGN](../../mutations/invert_bitwise_assignments.md) mutant type.

```shell
gremlins unleash --invert-bwassign
```

### Invert logical operators

:material-flag: `--invert-logical` · :material-sign-direction: Default: `false`

Enables/disables the [INVERT LOGICAL](../../mutations/invert_logical.md) mutant type.

```shell
gremlins unleash --invert_logical
```

### Invert loop control

:material-flag: `--invert-loopctrl` · :material-sign-direction: Default: `false`

Enables/disables the [INVERT LOOP](../../mutations/invert_loop.md) mutant type.

```shell
gremlins unleash --invert-loopctrl
```

### Invert negatives

:material-flag: `--invert-negatives` · :material-sign-direction: Default: `true`

Enables/disables the [INVERT NEGATIVES](../../mutations/invert_negatives.md) mutant type.

```shell
gremlins unleash --invert_negatives=false
```

### Output

:material-flag: `--output`/`-o` · :material-sign-direction: Default: empty

When set, Gremlins will write the give output file with machine readable results.

```shell
gremlins unleash --output=output.json
```

The output file in JSON format and has the following structure:

[//]: # (@formatter:off)

```json
{
  "go_module": "github.com/go-gremlins/gremlins",
  "test_efficacy": 82.00,
  //(1)
  "mutations_coverage": 80.00,
  //(2)
  "mutants_total": 100,
  "mutants_killed": 82,
  "mutants_lived": 8,
  "mutants_not_viable": 2,
  //(3)
  "mutants_not_covered": 10,
  "elapsed_time": 123.456,
  //(4)
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

[//]: # (@formatter:on)

1. This is a percentage expressed as floating point number.
2. This is a percentage expressed as floating point number.
3. NOT VIABLE mutants are excluded from all the calculations.
4. The elapsed time is expressed in seconds, expressed as floating point number.

[//]: # (@formatter:off)
!!! warning
    The JSON output file is not _pretty printed_; it is optimised for machine reading.
[//]: # (@formatter:on)

### Remove self-assignments

:material-flag: `--remove-self-assignments` · :material-sign-direction: Default: `false`

Enables/disables the [REMOVE_SELF ASSIGNMENTS](../../mutations/remove_self_assignments.md) mutant type.

```shell
gremlins unleash --remove-self-assignments
```

### Tags

:material-flag: `--tags`/`-t` · :material-sign-direction: Default: empty

Sets the `go` command build tags.

```shell
gremlins unleash --tags "tag1,tag2"
```

### Test CPU

:material-flag: `--test-cpu` · :material-sign-direction: Default: `0`

[//]: # (@formatter:off)
!!! tip
    To understand better the use of these flag, check [workers](workers.md)
[//]: # (@formatter:on)

This flag overrides the number of CPUs the Go test tool will utilize. By default, Gremlins doesn't set this value.

```shell
gremlins unleash --test-cpu=1
```

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

### Timeout coefficient

:material-flag: `--timeout-coefficient` · :material-sign-direction:
Default: `0` (uses default value of `5`)

[//]: # (@formatter:off)
!!! tip
    To understand better the use of these flag, check [workers](workers.md)
[//]: # (@formatter:on)

Gremlins determines the timeout for each Go test run by multiplying by a
coefficient the time it took to perform the coverage run. The default
coefficient is `5`, which can be overridden with this flag (`0` means use
the default).

To ensure reasonable timeouts even when the coverage run is very fast,
Gremlins enforces a minimum base timeout of 1 second before applying the
coefficient. For example:

- Coverage run takes 500ms → timeout = max(500ms, 1s) × 5 = 5 seconds
- Coverage run takes 2s → timeout = 2s × 5 = 10 seconds

```shell
gremlins unleash --timeout-coefficient=10
```

### Timeout max

:material-flag: `--timeout-max` · :material-sign-direction:
Default: `""` (no ceiling)

An absolute ceiling on a single mutant's test RUN, as a Go duration. It caps
the coefficient-derived timeout; it never raises it. It bounds only the run:
compiling the mutated package is charged to `--compile-allowance` instead.

The coefficient scales the timeout by how long the package's own tests take,
which is unrelated to how much damage a mutant can do in that time. A mutant
that inverts a loop-advance statement (`i++` → `i--`) inside a scanning loop
that appends on every iteration never terminates and allocates until the
machine runs out of memory. On a CI runner the OOM killer then reaps the
runner agent itself and the job dies with no verdict at all. Because the
coefficient hands the longest timeouts to the slowest-testing packages, those
packages are the most exposed — which is backwards.

This flag bounds that exposure independently of the baseline. Past the
ceiling the mutant is killed and recorded as `TIMED OUT`, the honest verdict
for a mutant that did not terminate.

```shell
gremlins unleash --timeout-max=15s
```

A malformed or non-positive value is reported on stderr and ignored, leaving
the run with no ceiling.

### Compile allowance

:material-flag: `--compile-allowance` · :material-sign-direction:
Default: `2m`

How long a mutant is allowed to COMPILE, as a Go duration, on top of the bound
on its test run.

Compile time and run time scale with unrelated things: compile time with the
size of the package and its dependency graph, run time with the test that
adjudicates the mutant. Charging both to one number makes the slow-compiling
packages the ones whose mutants time out — including mutants whose tests would
have reached a verdict in milliseconds. So the two are bounded separately:
`go test -timeout` gets the run bound, and a deadline of `run bound + compile
allowance` covers compilation and the run together.

That deadline is a backstop, not a second leash. No `-timeout` can bound a
compile that has hung, because there is no test binary yet to enforce one, so
something has to. A mutant killed by either bound is recorded `TIMED OUT`,
which stays in the efficacy denominator and credits no test with a detection.

```shell
gremlins unleash --timeout-max=5s --compile-allowance=3m
```

A malformed or non-positive value is reported on stderr and ignored, leaving
the default allowance in place.

!!! note "Verdicts are read from the output, not the exit status"
    `go test` reports a failing test, a package that does not build and a test
    that ran past its `-timeout` all as exit status 1 — only the test *binary*
    exits 2, and what Gremlins spawns is `go`. Gremlins therefore scans the
    child's output to tell the three apart, so a timed-out mutant is recorded
    `TIMED OUT` rather than credited as a detection, and a mutant that does not
    compile is recorded `NOT VIABLE`. The output is scanned as it streams and
    then discarded, so a mutant that prints without bound cannot exhaust memory.

[//]: # (@formatter:off)
!!! note "Result Consistency"
    You may observe small fluctuations in the number of
    killed/lived/timed-out mutants between runs (typically ±2-4 mutants).
    This is normal and can be caused by:
    - **Race conditions**: Mutations may introduce or remove race
      conditions that behave differently each run
    - **Timing-sensitive tests**: Tests involving timeouts, concurrency,
      or I/O timing
    - **System variations**: CPU scheduling, system load, and other
      OS-level factors
    These minor variations do not indicate a problem with the mutation
    testing process. Large variations or progressive degradation would
    indicate an issue.
[//]: # (@formatter:on)

### On-shutdown status

:material-flag: `--on-shutdown-status` · :material-sign-direction: Default: `not-run`

Controls the `Status` recorded for mutants that were still in-flight when the run was cancelled
(e.g. `SIGTERM` from a CI runner that hit its job timeout, or Ctrl-C from the terminal).

Allowed values:

- `not-run` (default) — record cancelled-in-flight mutants as `NOT COVERED`. Truthful: they were
  never observed.
- `timed-out` — record them as `TIMED OUT`. Useful if you want test-efficacy
  (`KILLED / (KILLED + LIVED)`) to ignore them.
- `lived` — legacy behaviour: record them as `LIVED`. Available for backwards compatibility,
  but misleading — these mutants were not tested, so they should not be counted as survivors.

```shell
gremlins unleash --on-shutdown-status=timed-out
```

### Workers

:material-flag: `--workers` · :material-sign-direction: Default: `0`

[//]: # (@formatter:off)
!!! tip
    To understand better the use of these flag, check [workers](workers.md)
[//]: # (@formatter:on)

Gremlins runs in parallel mode, which means that more than one test at a time will be performed, based on the number of
CPU cores available.

By default, Gremlins will use all the available CPU cores of, and , in _integration mode_, it will use half of the
available CPU cores.

The `--workers` flag allows to override the number of CPUs to use (`0` means use the default).

```shell
gremlins unleash --workers=4
```
