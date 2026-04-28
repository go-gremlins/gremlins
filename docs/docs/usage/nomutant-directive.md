---
title: Nomutant directive
---

# Nomutant directive

The `//nomutant` comment directive lets you suppress mutations on individual
lines, blocks, or whole files directly from the source code. It is a
finer-grained alternative to the `exclude-files` configuration option, and is
useful when only a few mutations in a file are noisy or known to be unkillable
(for example, defensive checks for unreachable code).

A suppressed mutation is reported with the `SKIPPED` status, the same status
used by diff-mode for unchanged code, so you can still audit which directives
took effect by reading the report.

## Forms

There are four scope variants, all written as `//`-style line comments.

### 1. End-of-line

The directive shares a line with a statement and applies to that statement
only.

```go
a := b + c //nomutant
```

Every applicable mutator on that line is suppressed.

### 2. End-of-line, typed filter

When the directive is followed by `:` and a comma-separated list of mutator
type names, only the listed types are suppressed; other mutators on that line
still produce mutants.

```go
a := b + c //nomutant:arithmetic-base,invert-bitwise
```

The names are the same as the configuration keys for each mutator type — see
[Configuration](configuration.md) for the full list (e.g. `arithmetic-base`,
`conditionals-boundary`, `invert-bwassign`).

### 3. Block scope

A `//nomutant` on its own line, immediately above a function declaration or
a single statement, suppresses every mutation inside that AST node.

```go
//nomutant
func myFunc() {
    a := b + c
    return a * d
}
```

The typed filter is supported here too:

```go
//nomutant:arithmetic-base
func myFunc() {
    // only arithmetic-base mutators inside myFunc are suppressed
}
```

Block scope also works above a single statement:

```go
func myFunc() {
    //nomutant
    a := b + c // suppressed
    d := e * f // not suppressed
}
```

### 4. File scope

A `//nomutant` placed immediately before the package clause suppresses every
mutation in the file. It is equivalent to adding the file to the
`unleash.exclude-files` configuration list, but lives next to the code.

```go
//nomutant
package apackage
```

The typed filter applies at file scope as well:

```go
//nomutant:arithmetic-base
package apackage
```

## Nesting

Directives compose **additively**: a mutation is suppressed if any
enclosing scope (file, block, or end-of-line) suppresses its type. An
inner directive adds to outer ones rather than replacing them.

```go
//nomutant:invert-bitwise
func F() {
    //nomutant:arithmetic-base
    a := 1 + 2 // BOTH arithmetic-base AND invert-bitwise are suppressed
    b := 3 * 4 // only invert-bitwise is suppressed (outer scope still applies)
}
```

This means an untyped outer directive (`//nomutant`, suppressing every
type) cannot be narrowed by an inner typed directive — the outer one
already covers everything. To opt back in to a specific mutator inside
a broadly-suppressed region, restructure the code rather than relying
on directive composition.

## Malformed directives

A directive whose typed filter is empty (`//nomutant:`) or names only unknown
mutator types (`//nomutant:bogus-type`) is treated as a no-op and a warning
is logged. This makes it safe to add or rename directives without breaking
your build.

## Interaction with other settings

- A directive-suppressed mutant is reported with status `SKIPPED`. The mutant
  is still emitted to the report so you can confirm the directive took
  effect; it is not silently dropped.
- File-level `exclude-files` rules and `//nomutant` directives are
  independent. Either one is sufficient to suppress a mutation.
- Disabling a mutator type via configuration takes effect before the
  directive is evaluated; if a mutator type is disabled globally, the
  directive has nothing to suppress.
