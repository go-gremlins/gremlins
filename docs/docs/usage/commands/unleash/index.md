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

[//]: # (@formatter:off)
!!! tip
    If what you want is the cross-package tests and not the whole suite,
    [test selection](#test-selection) runs exactly the tests that execute the mutated line,
    wherever they live, instead of every test in the module.
[//]: # (@formatter:on)

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

### Test selection

:material-flag: `--test-selection` · :material-sign-direction: Default: `false`

By default Gremlins runs every test in the package where the mutant is. Package membership is a
guess at which tests could notice a mutation; coverage is the answer. With this flag Gremlins
runs only the tests **of that package** that actually execute the mutated line.

This cannot cost more than not using it. The package is the same one, so its test binary was
being built and its fixtures paid for either way; the only difference is that fewer of its tests
run. Measured on a module of 450 mutants, it performed 24% of the test executions the whole-suite
behaviour did.

When a mutant survives, the report names the tests that ran against it:

```
       LIVED CONDITIONALS_BOUNDARY at vm/vm.go:6:10
         not caught by: example.com/vm.TestSize, example.com/vm.TestClamp
```

The map that makes this possible cannot be read out of a coverage profile, because Go's profile
does not record which test executed a block. Gremlins builds it by running each test on its own
with coverage over the whole module, which costs one process per test — but only once. The map is
cached between runs, under `gremlins/testmap` in your user cache directory, keyed per module and
per checkout. A package is re-mapped only when the build ID of its test binary changes, which is
Go's own hash over that package's source *and* every dependency's. On an unchanged tree the map
costs one compile per package and no test runs at all.

```shell
gremlins unleash --test-selection
```

[//]: # (@formatter:off)
!!! warning
    The build ID cannot see state outside the build. A test whose coverage depends on a database,
    a clock, or the network can map differently on two runs of the same binary, and the cache
    holds that for longer than a single run would. Delete the cache directory to force a full
    re-map.
[//]: # (@formatter:on)

[//]: # (@formatter:off)
!!! note
    Selection is only ever used where the map is complete. If a package's tests cannot all be
    mapped — one of them fails to run, for instance — that package runs its whole suite, which is
    the behaviour without the flag. The same happens for a mutant the map has nothing to say
    about, and in `--integration` mode, where the whole module runs for every mutant by design.
[//]: # (@formatter:on)

### Cross package

:material-flag: `--cross-package` · :material-sign-direction: Default: `false`

Tests a mutant against the packages that **depend on** the one it is in, not only that one. A
mutation can only break a package that uses the mutated code, so those are the packages worth
running — and package scoping never runs them, which is how a mutant your suite does catch gets
reported as surviving. That is
[go-gremlins/gremlins#224](https://github.com/go-gremlins/gremlins/issues/224), where a
maintainer filed a bug against a `LIVED` verdict that was correct for what it had measured.

It needs no coverage map and no mapping phase. Which packages a change could break is a question
about imports, and one `go list` answers it — including packages whose *tests* import the mutated
one, which exercise it even when their own code does not.

```shell
gremlins unleash --cross-package
```

It is off by default because it is not free: the packages it adds are recompiled by the mutation,
and each pays its own test fixtures. On a module of 450 mutants the mutant phase went from 15.4 to
22.4 minutes, and the widest runs exhausted a 3 GB memory cap.

### Combining the two

The flags are independent and compose:

| flags | what runs for a mutant in P |
|---|---|
| neither | P's whole test suite |
| `--test-selection` | P's covering tests |
| `--cross-package` | whole suites of P and of every package that depends on P |
| both | covering tests in P and in P's dependents |

`--test-selection` alone maps only the packages being mutated, and records each test against its
own package's code. Adding `--cross-package` widens the map to the whole module, because a test
that kills the mutant may be anywhere — which is what makes that combination the expensive one.

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
