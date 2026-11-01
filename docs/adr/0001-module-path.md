# ADR 0001: Use the module path of the fork

Status: Accepted when #64 merges.

## Context

The module path was `github.com/go-gremlins/gremlins`. Go installs and pins a module by its path. With the upstream path,
`go install github.com/OpenRAE/go-rae-gremlins/cmd/gremlins@<version>` fails, and the upstream path installs the
upstream code.

## Decision

The module path is `github.com/OpenRAE/go-rae-gremlins`.

## Consequences

- `go install github.com/OpenRAE/go-rae-gremlins/cmd/gremlins@<version>` installs the fork.
- Each upstream change needs the path rewrite that ADR 0003 describes.
- The copied upstream PR branches use the old path. A kept PR needs the rewrite.
