# Release readiness architecture preflight — issue #84

Issue #84 requires repository readiness for a release that dependent repositories can pin after `dev` reaches `main`.
It does not require publishing the release or completing remediation.
Reuse the existing release architecture under ADRs [0001](../adr/0001-module-path.md) and [0002](../adr/0002-binary-name-and-versions.md).
No new ADR, release controller, or workflow engine is needed.

## Ownership and boundaries

| Concern | Canonical owner and guardrail |
| --- | --- |
| Version and release state | `release-please-config.json` and `.release-please-manifest.json` own the single root package. release-please owns version changes, tags, release notes, and `CHANGELOG.md`. Do not add a second version file or hand-maintained release state. |
| Publication | `.github/workflows/release-please.yml` runs on `main`. Its release-please outputs select the tag for GoReleaser. Keep asset publication in that workflow; token-created tag/release events cannot be assumed to start another workflow. |
| Build and assets | `.goreleaser.yaml` owns the platform matrix, `CGO_ENABLED=0`, source archive, checksums, and existing-release integration. GoReleaser remains the builder and uploader. `go.mod` owns module identity and Go requirements. |
| Release operation | [Release](release.md), [Contributing](../CONTRIBUTING.md#release), and `.github/workflows/sync-main-to-dev.yml` own promotion, reviewed merges, and synchronization. Readiness does not grant merge or publication authority. |
| Remediation | [Maintenance plan](maintenance-plan.md) and [documentation review](documentation-review.md) distinguish release readiness from repair work. Neither ordering language nor references to critical-path defects may make #84 wait for remediation. |
| Delivery checks | `.github/workflows/ci.yml`, `.pre-commit-config.yaml`, `Makefile`, `.ground-control.yaml`, and `.gc/plan-rules.md` own existing checks. Preserve `PR Gate`, security checks, and shrink-only baselines. Release independence does not waive failing checks. |

Module installation pins `github.com/OpenRAE/go-rae-gremlins/cmd/gremlins@vX.Y.Z`; binary installation pins the fork's release asset and checksum.
The binary stays `gremlins`, and tags stay unprefixed `vX.Y.Z`.
Neither `@latest` nor a moving branch is a reproducible pin.
The existing install guide owns installation instructions; avoid competing instructions or unverified downstream commands.
OpenRAE/env-packs pins its Go tools through the mise `go:` backend, which runs `go install` at a module version.

## Version reporting boundary

`cmd/gremlins/main.go` defaults its version to `dev`; release binaries obtain the version through GoReleaser's linker settings.
`go install ...@vX.Y.Z` does not set that linker version.
The binary then reads the main module version from its embedded build information.
The fallback has these bounds:

- Reuse standard-library `runtime/debug.ReadBuildInfo` and the existing `buildVersion` composition.
  Read the main module's version, not a dependency's version or a reconstructed Git tag.
  Do not execute Git, invoke Go, read the network, or inspect host files or environment variables at startup.
- Preserve an explicitly supplied linker version. Consult build information only for the existing `dev` sentinel.
  Missing information, empty versions, and `(devel)` must preserve the development fallback without causing startup failure.
  Remove only the leading `v`, so the output matches GoReleaser's `{{.Version}}`. Report a pseudo-version as it is.
- Preserve the current OS/architecture suffix and Cobra's version behavior.
  Keep the fallback at the binary entry point; `cmd.Execute` already accepts a supplied version.
  No configuration key, report field, public API, Logger initialization, or Exit error mapping needs to change.
- Distinguish independent verification of linker precedence, development fallback, and tagged-install metadata.
  Use existing table-driven testing conventions where useful; avoid a build-information service or general provider interface.
  A local build from a checkout cannot by itself establish tagged `go install` behavior.

## Release-state gotchas

- The bootstrap SHA resolves locally to upstream `v0.6.0`; the manifest starts at `0.6.0`.
  Local history contains release-producing `feat` and `fix` commits after that point.
  Preserve this relationship and verify the generated proposal against ADR 0002's expected first version, `v0.7.0`.
  Do not force a permanent version override or manufacture a feature commit to trigger release-please.
- A push filter for `main` and release-please's target branch are separate controls.
  The current action omits `target-branch`; confirm the detected branch is `main` or use that existing action input.
  Do not confuse Ground Control's integration branch, `dev`, with the release branch.
- GitHub settings must permit Actions to create release PRs and grant the required job permissions.
  Required checks, bot-PR workflow approval, and release immutability also affect the existing flow.
  These hosted settings were not inspected and cannot be proved by checkout configuration.
  Do not broaden credentials, change branch protection, or bypass checks to make publication work.
- The release-please job can publish a tag before GoReleaser finishes uploading assets.
  A tag or release page alone does not prove binary-pin readiness.
  Confirm append-to-release behavior is compatible with hosted immutability settings before claiming readiness.
  Recover partial failures at the same tag through existing tools, without moving tags or creating another version owner.
  The current workflow gates GoReleaser on `release_created`; a full workflow rerun may not reproduce that output.
- GoReleaser's platform list is the asset authority; do not infer archive names or formats from documentation examples.
  Source installation requires the Go version/toolchain in `go.mod` and available dependencies.
  Local snapshot generation runs hooks and builds; it is not syntax-only validation.

## Cross-cutting controls

| Layer | Required treatment |
| --- | --- |
| Configuration shapes | Use release-please's supported manifest/config schema and the pinned GoReleaser version's native validation. JSON/YAML hooks prove syntax only. Validate `.ground-control.yaml` changes through the owning Ground Control parser; do not invent fields or duplicate schemas. |
| Architecture admission | Reuse `.architecture/model.json`, `.architecture/design.json`, `scripts/check-design.sh`, and `tools/architecture` under ADR 0004. Their strict decoder checks fields, duplicate keys, references, paths, and declared trust changes. Declare affected canonical policies and packages. This model checks neither release-tool schemas nor GitHub permissions or publication success. |
| Authorization and secrets | Preserve pinned actions, job-scoped write grants, checkout without persisted credentials, and the existing `GITHUB_TOKEN` publication boundary. Keep tokens out of command arguments, URLs, logs, artifacts, and repository files. Keep candidate checks outside token-bearing Sonar and finalization boundaries. No new service, credential, or destination is needed. |
| Filesystem and host | Confine inspection and validation inputs to this checkout. Keep generated assets in the existing `dist` location. Use disposable local output for nonpublishing checks and inspect archive contents before publication. Do not read secret files or depend on host configuration. Embedded build information adds no filesystem or process access. |
| Application configuration | Reuse Configuration keys in `internal/configuration/configuration.go`, Cobra/Viper binding in `cmd/internal/flags/flags.go`, and `docs/docs/schema/configuration.json`. Version discovery and release tooling need no `configuration.Init`, host config discovery, or `GREMLINS_*` bindings. The configuration JSON schema is editor guidance, not a runtime validation gate. |
| Errors and observability | Native action/tool failures and hosted logs own release diagnostics. Preserve failure propagation and distinguish skipped, unrun, and successful jobs. Application Logger and Exit errors remain in `internal/log/log.go` and `internal/execution/execution.go`. Missing build metadata is a normal fallback, not a new logged error. No exception hierarchy or HTTP envelope is needed. |
| Persistence and consumer data | Git holds release configuration and generated release state; the existing GitHub release holds assets. `internal/report/internal/structure.go` owns mutation JSON. Release readiness needs no database, DTO, service/repository layer, new report schema, or external evidence store. |
| Verification | Reuse targeted local tests for changed runtime behavior and hosted CI for full runs. Preserve existing mutation checks without making open remediation a prerequisite. A build, snapshot, or design-check pass does not prove that either dependent repository successfully pinned a published release. |

GitHub's current documentation permits some token-created PR events to produce runs that require approval.
The repository's blanket statements that bot PRs never trigger checks need comparison with actual hosted behavior.
Keep maintainer approval and required checks; do not add a PAT or another workflow merely to avoid that approval.
See [GitHub workflow triggering](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow).

## Extension seam and non-goals

Future platforms belong in GoReleaser's existing matrix; release branch selection belongs in the action's existing input.
Future releases advance the same manifest and generated changelog without re-editing a hard-coded first-release version.
Build metadata is the standard-library seam for source-install identity; the linker remains authoritative for release assets.
Keep policy in canonical configuration and procedures linked to it, rather than copying policy into scripts or review records.

Preserve downstream CLI flags, Configuration keys and schema, JSON report fields, and Exit errors.
No Mutation type or Work directory copy change is needed.
Do not repair engine defects, install a validation service, add distribution channels, or adopt upstream workflows under this issue.
Any later upstream import remains governed by ADR 0003; byte-exact `testdata` stays untouched.
ADRs 0004–0005 continue to bind architecture and static-lint tooling; no model widening or baseline growth is proposed.
Workflow metadata removal needs evidence from its owning parser; parser acceptance and absence of local references are different claims.

## Evidence limits

Inspected source revision: `3d24c2ec478602f6a52b9ac9ea9c1cbe15c8332a`, with pre-existing working-tree edits.
Those edits initially affected `.ground-control.yaml`, `maintenance-plan.md`, and `documentation-review.md`.
The shared `.architecture/design.json` changed during inspection; this preflight preserves all those edits.
The model's trust declarations do not certify release publication or hosted settings.
`make architecture-design` passed for design #84 under ARCH001–ARCH004, with network access disabled through Go settings.
Model SHA256: `f50ccf387b1870da4a12fa781ffc2ba38db3dad4ed6d0ced9b70e19f3e1c41f0`.
Document whitespace and six local links passed inspection. This preflight produced no test or mutation results.

External behavior was checked against the [release-please action v5 source](https://github.com/googleapis/release-please-action/blob/v5.0.0/src/index.ts),
[manifest reference](https://github.com/googleapis/release-please/blob/main/docs/manifest-releaser.md),
and GoReleaser's [release](https://goreleaser.com/customization/publish/scm/) and [Go build](https://goreleaser.com/customization/builds/builders/go/) references.
Current GoReleaser documentation cannot replace validation using the repository's pinned v2.19.0 tool.
No implementation, release, upload, merge, downstream edit, or live installation was performed by this preflight.
