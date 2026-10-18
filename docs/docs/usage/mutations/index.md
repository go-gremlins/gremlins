---
title: Mutations
---

# About Mutations

Mutations are the core of Gremlins' activity. Each mutation belongs to a group that defines its _flavour_. These
groups are called _mutation types_. Gremlins supports various _mutation types_, each comprising one or more mutations.

When Gremlins scans the source code under test, it looks for mutations and for each found mutation creates a _mutant_.
A _mutant_ is the "gremlin" that actually changes the source code.

Each _mutant type_ can be enabled or disabled, and only a subset of mutations is enabled by default.

| MutationType                                           | Default |
|--------------------------------------------------------|:-------:|
| [ARITHMETIC BASE](arithmetic_base.md)                  |   YES   |
| [CONDITIONALS BOUNDARY](conditionals_boundary.md)      |   YES   |
| [CONDITIONALS NEGATION](conditionals_negation.md)      |   YES   |
| [INCREMENT DECREMENT](increment_decrement.md)          |   YES   |
| [INVERT NEGATIVES ](invert_negatives.md)               |   YES   |
| [INVERT LOGICAL ](invert_logical.md)                   |  FALSE  |
| [INVERT LOOP CTRL ](invert_loop.md)                    |  FALSE  |
| [INVERT ASSIGNMENTS ](invert_assignments.md)           |  FALSE  |
| [INVERT BITWISE ](invert_bitwise.md)                   |  FALSE  |
| [INVERT BWASSIGN ](invert_bitwise_assignments.md)      |  FALSE  |
| [REMOVE_SELF_ASSIGNMENTS ](remove_self_assignments.md) |  FALSE  |

## Mutants Gremlins does not generate

A mutant the Go compiler rejects is not a fault a test suite failed to catch. It cannot be
detected, so no verdict describes the suite: recording it as killed pays the suite for the
compiler's work, and recording it as not viable leaves the package's mutant set padded with
entries that describe the compiler. So Gremlins does not generate one.

The mutation tables in this section say which rewrites are _meaningful_ for a token read on its
own, which is all a table can say. Whether the result is a program depends on things the token
does not carry:

- **The operand types.** Go defines `+` on strings and defines nothing else on them, so
  [arithmetic base](arithmetic_base.md) and [invert assignments](invert_assignments.md) both map a
  token whose operands forbid the mutation.
- **The constant values around it.** `7 * 24 * time.Hour` rewritten to `7 / 24 * time.Hour` is a
  legal constant expression; it is the `d / week` further down the file that becomes a division by
  zero.
- **The statements the enclosing function admits.** [invert loop ctrl](invert_loop.md) turns a
  `break` inside a `switch` into a `continue` that is not in a loop, and turns the `continue` that
  keeps a `for` from terminating into a `break` that leaves the function missing a return.
- **The position the token was read in.** `&x` is address-of, not bitwise AND — see
  [invert bitwise](invert_bitwise.md).

Gremlins answers the first three by type-checking the candidate mutant's whole package before
emitting it, which is the only oracle precise enough and the only unit wide enough to see an error
that surfaces away from the mutation. A package that cannot be loaded and type-checked as it stands
is not used as an oracle at all: its mutants are generated and left to the compiler, exactly as
before, and a line naming the package says so. Dropping a mutant nobody proved illegal would shrink
the set a mutation score is measured against, which is worse than the padding it removes.

[//]: # (@formatter:off)
!!! note "`go vet` is not the compiler"
    `go test` runs a subset of `go vet` before it builds anything and reports a finding as
    `FAIL pkg [build failed]` — from the outside, indistinguishable from source that does not
    compile. One of those analyzers, `bools`, rejects exactly what
    [invert logical](invert_logical.md) produces from `name == a || name == b`: a conjunction of
    equalities against distinct constants, which it calls "suspect and". Those mutants are legal Go
    and a real change of behaviour, so Gremlins keeps generating them and runs the mutated tests
    with `-vet=off` instead. A style analyzer does not get to decide whether a mutant may be
    adjudicated.
[//]: # (@formatter:on)
