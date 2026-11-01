# ADR 0002: Keep the binary name and use semantic versions from release-please

Status: Accepted when #64 merges.

## Decision

- The binary name stays `gremlins`. The CLI is a contract for the dependent repositories and the documentation.
- The fork uses semantic version tags `vX.Y.Z` on its own module path. release-please sets each version from the
  Conventional Commit titles of the merged PRs. The first release of the fork is v0.7.0, after upstream v0.6.0.
- A release publishes the binaries and checksums as assets of a GitHub release. The fork publishes no Docker image,
  Homebrew formula, or documentation site.

## Consequences

- Nobody edits the version or `CHANGELOG.md` by hand. release-please owns them.
- Dependent repositories pin a GitHub release or a module version.
