# ADR 0003: Take upstream changes by cherry-pick

Status: Accepted when #64 merges.

## Context

The maintainer's pre-push guard permits only merges of `dev` into a feature branch, and it blocks Claude and Codex
attribution trailers. The module path of the fork differs from upstream (ADR 0001).

## Decision

To take an upstream change:

1. Make a branch from `dev`.
2. Cherry-pick the upstream commits with `git cherry-pick -x`, so that each commit names its upstream commit.
3. Change `github.com/go-gremlins/gremlins` to `github.com/OpenRAE/go-rae-gremlins` in the changed files.
4. Remove Claude and Codex attribution trailers from the commit messages.
5. Open a PR to `dev`.

## Consequences

- The fork never merges an upstream branch.
- Each commit keeps its original author.
