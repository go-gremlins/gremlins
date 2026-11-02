# Upstream build and release

This page describes how upstream Gremlins (`go-gremlins/gremlins`) builds and releases. ADR 0002 records what the
fork publishes, and `release.md` describes the release of the fork.

## Build

`.goreleaser.yaml` builds `./cmd/gremlins` with `CGO_ENABLED=0` for Linux, macOS, and Windows on 386, amd64, and
arm64 (no macOS 386). `make build` builds one local binary in `dist/bin/`, and `make snap` builds a goreleaser
snapshot.

## Release

The upstream `release.yml` workflow ran on each `v*` tag. It ran goreleaser, which published to these channels:

| Channel | Upstream target | Credential |
| --- | --- | --- |
| GitHub release | draft release with archives, checksums, and source | `WORKFLOW_PAT` |
| Docker Hub | `docker.io/gogremlins/gremlins` images and manifests for 386, amd64, and arm64 | `DOCKER_HUB_TOKEN` |
| Homebrew | formula in `go-gremlins/homebrew-tap` | `WORKFLOW_PAT` |
| Linux packages | deb and rpm assets | none |
| Documentation | `docs.yml` deployed the MkDocs site with mike to `gh-pages`, served at gremlins.dev | `GITHUB_TOKEN` |

`go-gremlins/gremlins-action` is a separate upstream repository. It downloads a released binary and runs
`gremlins unleash`.

All of these channels belong to upstream. The fork removed `release.yml` and `docs.yml`, and it removed the Docker,
Homebrew, and Linux package sections from `.goreleaser.yaml`.
