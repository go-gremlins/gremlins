---
title: Mutations
---

# About Mutations

Mutations are the core of Gremlins' activity. Each mutation belongs to a group that defines its _flavour_. These
groups are called _mutation types_. Gremlins supports various _mutation types_, each comprising one or more mutations.

When Gremlins scans the source code under test, it looks for mutations and for each found mutation creates a _mutant_.
A _mutant_ is the "gremlin" that actually changes the source code.

Each _mutant type_ can be enabled or disabled, and only a subset of mutations is enabled by default.

| MutationType                                      | Default  |
|---------------------------------------------------|:--------:|
| [ARITHMETIC BASE](arithmetic_base.md)             |   YES    |
| [CONDITIONALS BOUNDARY](conditionals_boundary.md) |   YES    |
| [CONDITIONALS NEGATION](conditionals_negation.md) |   YES    |
| [INCREMENT DECREMENT](increment_decrement.md)     |   YES    |
| [INVERT NEGATIVES ](invert_negatives.md)          |   YES    |
| [INVERT LOOP CTRL ](invert_loop.md)               |   YES    |
| [INVERT LOGICAL ](invert_logical.md)              |  FALSE   |
