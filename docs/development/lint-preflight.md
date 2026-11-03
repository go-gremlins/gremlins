# Issue #77: lint architecture preflight

Status: guidance before implementation. This document does not establish the coding standard or deliver #77.
It identifies the boundaries for strict linting, a shrink-only baseline, and the upstream comparison record.
No implementation sequence is prescribed.

The subsequent security review found that Logger reuse executes candidate application initializers.
[ADR 0005](../adr/0005-static-lint-parser.md) supersedes that guidance for the lint parser only.
The parser uses standard-library diagnostics and a standalone build that cannot resolve application imports.

## Existing owners and boundaries

The inspected fork revision is `bb2a9b44a8237ce0db2c6d78cafa030b8fd71b60`.
The local upstream reference is `go-gremlins/gremlins` at
`b48a4aad17eff33dc7262e0d1487b1a4d6eec322`; no upstream fetch is needed for this comparison.
Its `.golangci.yml` is identical to the fork's at these revisions.

| Concern | Canonical incumbent and required reuse |
| --- | --- |
| Lint and formatting | [`.golangci.yml`](../../.golangci.yml) owns executable settings; [`mise.toml`](../../mise.toml) pins golangci-lint v2.15.0. Keep an explicit rule selection. `make lint` remains the common gate. |
| Invocation | [`Makefile`](../../Makefile), [CI](../../.github/workflows/ci.yml), [hooks](../../.pre-commit-config.yaml), and [Ground Control configuration](../../.ground-control.yaml) already delegate to `make lint`. Keep one evaluation path, including the policy job's invocation. |
| Architecture policy | [ADR 0004](../adr/0004-architecture-model-checks.md), [the model](../../.architecture/model.json), and [`tools/architecture`](../../tools/architecture) own package boundaries and design checks, not coding-style rules. Reuse the baseline invariants and input/Git safeguards; do not copy lint rules into the model. |
| Diagnostics and errors | [`internal/log`](../../internal/log/log.go) owns Go console output. Use it in application-connected tools without `configuration.Init`. ADR 0005 requires standard-library-only diagnostics in the isolated lint parser. Use ordinary wrapped Go errors and nonzero development-check status, not a new error hierarchy or mutation exit-code category. |
| Runtime contracts | Configuration keys and Viper binding, the configuration JSON schema, mutation reports, and `internal/execution.ExitError` remain runtime concerns. No lint settings or findings belong in them. |
| Workflow and evidence | [CONTRIBUTING](../CONTRIBUTING.md), [plan rules](../../.gc/plan-rules.md), and the [process comparison](process-review.md) already define development, review, and comparison-record conventions. Preserve the hook installer and existing publication gates. |

The architecture decoder, Git reader, and ratchet are currently private functions in a `package main`, not
an importable library. They are the reference implementation for these concerns. Reuse through a small shared
extraction only where actual common code warrants it; do not import a command, paste a second generic ratchet,
or turn architecture tooling into a general policy framework. A lint-specific diagnostic adapter is a separate concern.

## Standards and comparison record

Pin the Google style source revision and the upstream comparison revision in the eventual standards record.
Distinguish the canonical [Guide](https://google.github.io/styleguide/go/guide), normative
[Decisions](https://google.github.io/styleguide/go/decisions), and advisory Best Practices, as explained by
the [Google overview](https://google.github.io/styleguide/go/). Do not claim that enabling a linter proves full compliance.

The record must account for each rule: source section, deterministic predicate, selected checker/subrule and
settings, or the precise reason it requires human judgment or lacks tool support. A deterministic coverage gap
must remain explicit and be resolved before claiming the outcome; calling it subjective does not resolve it.
Prefer supported checks within golangci-lint over custom analyzers. Keep configuration as the executable authority;
the record explains coverage and decisions rather than defining another policy schema.

Compare enabled and disabled rules, options, formatter behavior, exclusions, suppressions, versions, and invocation
against upstream. Record each difference and its keep/adapt/drop decision with a reason, including rules retained
from upstream. Existing complexity and whitespace preferences are not automatically Google rules. In particular,
Google permits useful named results and has no fixed line-length limit; upstream enables `nonamedreturns`.
Resolve that conflict explicitly. Preserve the repository Logger rather than adopting Google-internal logging examples.
Keep runtime JSON `snake_case` tags stable; identifier naming guidance does not authorize report/schema changes.

Observed drift to resolve within lint integration: `make requirements` still installs the old v1 module at `@latest`,
while lint uses the mise pin. `make goimports`, Ground Control's format command, and the configured formatter must
agree on the fork import prefix and formatting policy. `fieldalignment -fix` is not evidence of Google style compliance.
Review implicit checker subrules and `revive`'s confidence filter for missing coverage. The inherited test exclusion
names `cyclop`, while the enabled complexity checker is `gocyclo`; they are different checks.
Avoid bulk fixes, exported renames, struct reordering, or fixture rewrites to make the initial run green.

## Baseline contract and complete analysis

Let F be all current lint identities, B the committed candidate lint baseline, and T the baseline at the pinned
comparison commit. Require F = B and B to be a subset of T, with occurrence counts where identities repeat.
This rejects new findings, growth, equal-count replacements, duplicate laundering, and stale allowances.
Keep lint debt distinct from `.architecture/baseline.json`; architecture subjects cannot stand in for lint identity.

Identity needs checker/subrule, normalized checkout-relative file, and a stable subject or context fingerprint.
Line/column and rendered message alone are too fragile. Preserve distinct violations within one function and
distinguish repeated real occurrences from the same diagnostic reported in multiple build contexts.
Define deterministic ordering and behavior for source movement and tool-version changes; do not hide migrations
behind a refresh. Persist minimal identities, not source lines, absolute host paths, or complete tool reports.

First adoption needs an explicit reviewed initial baseline evidenced against untouched source at a pinned target
commit under the selected strict settings. Only findings already present there may seed the candidate baseline.
The architecture check's missing-target-baseline-means-empty behavior cannot admit nonempty lint debt unchanged.
Normal checks must never regenerate or enlarge allowances; deleting the target baseline later is not adoption.
PRs compare with the event's base SHA, pushes with the preceding SHA, and local/manual runs with an explicitly
resolved target. Define initial-push behavior. Missing comparison history is an error; the lint checkout currently
lacks the architecture job's full history. Pass the same comparison to `make lint` wherever policy hooks invoke it.

Use the pinned tool's structured output, not scraped console text. Validate the emitted shape and distinguish
the documented findings exit status from crashes, timeout, package/type errors, missing tools, and malformed or
truncated output. Bound input/report sizes and fail on overflow rather than trimming findings.
A subprocess failure must never become an empty successful findings set. Severity is display
metadata, not permission to ignore a finding. Review all output filters before baselining: disable issue-count
caps and per-line collapsing; do not use changed-lines/new-only mode for the authoritative gate.
These controls are documented in [golangci-lint configuration](https://golangci-lint.run/docs/configuration/file/).

Cover production code, `*_test.go`, development tools, and supported OS-specific files in valid, separate build
contexts. Declare supported tags and generated-code policy; Linux `./...` alone does not cover Windows files.
Handle fixture modules, vendor, build output, and nested modules explicitly without importing mutation exclusions.
Linter and formatter exclusions are separate controls. Do not broadly exempt tests or use new `nolint`, generated
markers, weaker settings, or path exclusions to manufacture baseline shrinkage. Existing and future justified
exceptions need narrow scope, a reason, and visible policy review; `nolintlint` alone cannot prove the reason is valid.

## Cross-cutting gates and trust boundaries

| Layer | Required behavior |
| --- | --- |
| Configuration shape | Use the pinned tool's `config verify` and effective enabled-rule checks; YAML syntax hooks alone do not validate linter settings. Select the checkout config explicitly, preventing parent/home discovery. Validate baseline version, fields, identities, multiplicities, and duplicate keys using the existing strict-decoding approach. Do not create a second golangci-lint schema. |
| Environment binding | Do not initialize Viper, discover `.gremlins.yaml`, or accept `GREMLINS_*` overrides. Control Go build inputs (`GOFLAGS`, `GOWORK`, `GOENV`, toolchain, tags, and cgo) for reproducible supported contexts; ambient `-toolexec` or workspace settings must not introduce commands or external source. Tool installation is separate from analysis. |
| Filesystem and persistence | Confine source, config, baseline, and output paths to the checkout or a controlled temporary directory. Reject traversal, linked policy files, symlink escapes, and external module replacements that escape the declared scan. Treat paths with spaces, newlines, or leading dashes as data. Checks are read-only; any explicit shrink write must be atomic and must not follow links. |
| Process and host | Reuse fixed-argument Git reads with validated commit refs; no shell interpolation or credentials in argv. Bound subprocess time and fail on incomplete output. Do not run source, generators, repository-specified plugins, or tests as part of lint analysis. Do not reuse mutant work-directory execution to implement lint. |
| CI authorization and secrets | Keep pinned actions, `contents: read`, and `persist-credentials: false`. Candidate tooling runs without secrets, including on foreign PRs. Preserve Sonar admission and its separate token-bearing job. Do not use publishing credentials, `pull_request_target` with candidate code, new uploads, or new services. |
| Failure propagation | Preserve `PR Gate`'s required lint success. New findings, baseline growth, parse/validation errors, cancellation, and unexpected skips must block it. Policy-only, baseline-only, and tool-version changes must trigger the same gate and relevant local hook; current hook file filters omit several such inputs. Do not fork CI and hook comparison logic. |
| Diagnostics and error output | Use Logger for application diagnostics and ordinary error wrapping. The isolated lint parser uses only standard-library logging under ADR 0005. Quote input-derived locations/messages and escape workflow annotations/control characters. Show rule, location, and repair guidance without source dumps, secrets, raw environment, or raw subprocess stderr. There is no HTTP auth/controller/error-envelope layer to add. |
| Model validation | Before implementation, update `.architecture/design.json` with the actual packages, dependencies, and affected policies, then run `make architecture-design`. New tool packages and canonical lint/baseline paths need explicit ownership. Preserve ARCH001–ARCH004, local/existing-CI destinations, no analysis network use, and no scanned-source execution. Provision approved tools/dependencies separately rather than silently weakening that trust contract. |

The existing syntax, secret, vulnerability, architecture, lint, and Sonar checks have distinct jobs. Lint integration
does not replace them. The candidate can edit its own baseline, config, checker, and workflow together; maintainer
review remains the boundary for policy changes, including suppressions and weakened rule settings.

## Extension seam, proof, and non-goals

Keep rule selection/settings in `.golangci.yml`, the tool pin in mise, and build contexts and comparison revision
as explicit inputs to the common evaluation path. A new checker or supported build context should not require
separate CI, hook, and Ground Control implementations. Version the lint identity contract; tool upgrades and
new checks need reviewed coverage/identity decisions and cannot automatically buy more baseline debt.

Use existing table-driven tests and targeted race tests to prove same-count substitution, repeated occurrences,
line movement, stale entries, first adoption, invalid input/history, tool failures, and policy-only CI changes.
Demonstrate that a new violation fails even when the candidate adds it to B. Exercise shared Go comparison logic
with the existing hosted mutation workflow where applicable; do not introduce a new test framework or redefine #78–#79.

This preflight changes guidance only. #77's implementation remains development tooling and its standards record:
no new mutation type, runtime configuration/schema/report/exit-code changes, broad debt cleanup, upstream merge,
service, storage system, or replacement workflow. [ADR 0003](../adr/0003-upstream-changes.md) still governs any imported
upstream changes. Architecture checks remain under ADR 0004; wider test, mutation, quality, and security work stays
with #78–#81. This note is neither the completed standards comparison nor evidence that the new gate exists.
