# Release

ADR 0002 records the release decisions. This page describes the steps and the differences from upstream Gremlins.

## Steps

1. Merge changes into `dev` through pull requests. Each PR title is a Conventional Commit, and the type sets the
   version change: `feat` gives a minor release, `fix` or `perf` gives a patch release, and a breaking change (`!` or
   `BREAKING CHANGE:`) gives a major release. The other types do not make a release.
2. Open a pull request from `dev` to `main`, and merge it.
3. The push to `main` starts the Release workflow (`.github/workflows/release-please.yml`). release-please opens or
   updates the release pull request, `chore(main): release X.Y.Z`. It sets the version and writes `CHANGELOG.md`.
4. Merge the release pull request. release-please creates the tag `vX.Y.Z` and the GitHub release with the release
   notes. In the same run, goreleaser builds the binaries and adds the archives, the source archive, and
   `checksums.txt` to the release.
5. The Sync main -> dev workflow opens a pull request from `main` to `dev`. Merge it, so that `dev` has the release
   commit.

`GITHUB_TOKEN` opens the release pull request and the back-merge pull request, so CI does not run on them. Do not edit
the version or `CHANGELOG.md` by hand.

The manifest starts at 0.6.0, and `bootstrap-sha` in `release-please-config.json` is the commit of upstream v0.6.0. The
first release is v0.7.0 and lists the changes since upstream v0.6.0.

## Assets

Each release has `gremlins` archives for Linux (386, amd64, arm64), macOS (amd64, arm64), and Windows (386, amd64,
arm64), a source archive, and `checksums.txt`. `gremlins --version` prints the release version.

A dependent repository can also pin a module version, for example
`go install github.com/OpenRAE/go-rae-gremlins/cmd/gremlins@v0.7.0`. That binary also prints the release version.

## Differences from upstream

`docs/development/upstream-release.md` describes the upstream process.

| | Upstream Gremlins | go-rae-gremlins |
| --- | --- | --- |
| Start | A maintainer pushes a `v*` tag. | A maintainer merges the release pull request into `main`. |
| Version | The maintainer chooses the tag. | release-please sets it from the Conventional Commits. |
| Release notes | goreleaser writes them from GitHub. | release-please writes `CHANGELOG.md` and the notes. |
| GitHub release | goreleaser makes a draft, and a maintainer publishes it. | release-please publishes it, and goreleaser adds the assets. |
| Assets | Archives, source, checksums, deb, and rpm | Archives, source, and checksums |
| Docker image | `docker.io/gogremlins/gremlins` | None |
| Homebrew | `go-gremlins/homebrew-tap` | None |
| Documentation site | gremlins.dev, deployed with mike | None |
| Credentials | `WORKFLOW_PAT` and `DOCKER_HUB_TOKEN` | `GITHUB_TOKEN` only |
| goreleaser | Any v2 release | v2.19.0 |
