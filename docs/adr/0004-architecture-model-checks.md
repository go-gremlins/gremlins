# ADR 0004: One architecture model for repository checks and design review

Status: Accepted when #76 merges.

Implementation: `.architecture/model.json` and `tools/architecture`.
See [the architecture design](../development/architecture.md) for rules, data flow, security controls, and invocation.

## Context

Issue #76 requires whole-repository checks compiled from an architecture model, actionable findings,
shrink-only baselines, and a design check before coding. The existing architecture vocabulary in
`.ground-control.yaml` names patterns and helpers. It is not an executable policy or a model schema.

`CLAUDE.md` describes the runtime architecture. `.gc/plan-rules.md` and `docs/CONTRIBUTING.md` supply the
repository workflow. There is no architecture checker or baseline today. The installed Ground Control
design step requires design before implementation, but explicitly supplies no automated design gate.
Repository guidance must not claim that such a gate already exists.

## Decisions and guardrails

### One policy source, with a development-tool boundary

Keep one repository-owned, versioned model of components, allowed dependency directions, rule scope,
and contract ownership. Each rule needs a stable ID, an explanation, and a concrete fix-it message.
Compile that model into executable checks. Compilation can produce in-memory checks; generated files
are not required. If outputs are committed, reject drift from the model.

Keep evaluation independent of invocation. CI and the design step must consume the same rules.
Use existing Go tooling and standard parsers before adding dependencies or a policy language.
Do not introduce a plugin framework, a service, or a generic workflow engine.
The checker belongs to repository development tooling, outside the shipped Gremlins command and its
runtime dependency graph. Architecture findings must not enter the mutation JSON report.

Model intended boundaries rather than blessing every observed import. In particular, `cmd/unleash.go`
composes coverage, diff, exclusion, engine, work directory copies, and reports. `engine` already depends
on `report`; `log` depends on `configuration`, which depends on `mutator`. Neither Logger nor
Configuration is an independent leaf. Preserve Go's `internal` visibility boundaries, including
`cmd/internal/flags` and `internal/report/internal`. Do not impose invented service/repository layers
or refactor these relationships merely to make the model pass. Known violations belong in the baseline,
not broad permission rules.

### Whole-repository coverage and reuse

| Surface | Canonical incumbent and constraint |
| --- | --- |
| Runtime architecture | `CLAUDE.md`, `cmd/`, and all `internal/` packages, including engine subpackages. Classify new packages explicitly; do not silently ignore unclassified code. |
| Static analysis | `.golangci.yml`, `mise.toml`, and `make lint` already own lint, formatting, and Go security rules. Reuse those checks; do not reproduce their implementations or copy their rule lists into the model. |
| Go analysis | `go/ast`, `go/parser`, and `go/token` are already used in `internal/engine`. Reuse standard parsing, not the mutation engine's execution or its ignored parse errors. Use semantic resolution when a rule needs symbol identity; textual names alone do not handle aliases and shadowing. |
| Configuration contract | `internal/configuration/configuration.go`, `cmd/internal/flags/flags.go`, `cmd/unleash.go`, and `docs/docs/schema/configuration.json` remain authoritative. The model can reference ownership and relationships; it must not create a second key/default/schema inventory. |
| Output and errors | `internal/log/log.go` owns Go console diagnostics. Initialize Logger in a Go checker entry point. Return errors with ordinary Go wrapping (`%w`, `errors.Is`/`As`); do not create an exception hierarchy. `internal/execution` owns Gremlins exit codes, not architecture debt. A development check needs success/failure without changing those codes. |
| Reports and persistence | `internal/report/internal/structure.go` owns the mutation report shape. Model and baseline are version-controlled development data; no database, new report format for consumers, or external storage is needed. |
| Invocation and CI | `Makefile`, `.github/workflows/ci.yml`, `.pre-commit-config.yaml`, and `scripts/install-hooks.sh` own execution and hook activation. Use a common checker entry point. A hook is optional local convenience, not proof of CI or design enforcement. Keep fast local checks separate from CI's full runs. |
| Design workflow | `.gc/plan-rules.md`, `.ground-control.yaml`, `CLAUDE.md`, and `docs/CONTRIBUTING.md` are the existing repository inputs. Do not fork the installed Ground Control orchestrator or invent unsupported configuration fields. |

Enumerate the repository independently of the host's active Go build. Include production code, tests,
OS-specific files such as `process_windows.go` and `process_unix.go`, and custom-tagged files.
Type-aware rules need explicit build contexts; do not combine incompatible platform definitions.
Define separate test scopes where tests need dependencies unavailable to production code.
Keep fixtures and generated/build outputs distinct from application packages, with explicit exclusions.
Do not rewrite `testdata` bytes or treat its nested example modules as production packages.
Repository workflows and configuration remain in scope for the rules that govern them; a Go import
graph alone does not establish whole-repository workflow or contract compliance.

### Shrink-only means identity, not just a count

Let F be current finding identities, B the candidate baseline, and T the baseline at the pinned target
revision. Require F to be a subset of B, and B to be a subset of T. Require removal of stale B entries.
A fixed violation cannot fund a different violation, even when the total count stays equal.

Finding identity must include rule, repository-relative location, and semantic subject, with occurrence
counts where needed. Line numbers and message text are display data, not identity. Sorting and path
normalization must be deterministic across operating systems. Duplicate identities must not hide growth.
Missing files, malformed input, unknown rules, and analysis failures are hard errors, never baseline debt.
Show baseline-covered findings with their fix-it messages as well as new findings.

Compare against the CI event's target commit for a PR, or the preceding commit for a branch push.
Resolve an explicit local comparison ref to a commit. Do not trust a candidate-supplied baseline or
silently succeed when shallow history omits the comparison commit. First adoption must distinguish
pre-existing violations in the target tree from violations introduced by the candidate. It needs a
reviewed initial baseline; normal checks must never regenerate or enlarge it automatically.
Rule removal, renamed IDs, changed exclusions, and identity migrations require visible policy review;
they must not disguise added debt as shrinkage.

### Check the proposed design before coding

The design step must evaluate proposed components, dependencies, contract changes, and trust boundaries
against the model before implementation. Running checks on unchanged source alone does not check a design.
Use the same compiled rule IDs and semantics for facts that can be checked mechanically. Review the
remaining design assertions explicitly; do not turn prose into another policy or design language.

Record the model revision, affected rules, proposed relationships, results, and any unresolved conflict
in the existing design evidence. An incompatible design needs revision or an explicit, reviewed model
decision before coding. Recheck when the model or relevant design changes. A documentation link,
workflow phase marker, or baseline suppression is not evidence that this evaluation happened.
The implementation design must identify the actual pre-code invocation through the existing workflow;
repository notes alone do not satisfy this acceptance criterion.

### Trust boundaries and failure behavior

Source files, model data, baselines, paths, and Git refs are inputs. Findings remain local or in the
existing repository CI logs. No new upload, hosting, or external processing is authorized.

| Layer | Required boundary |
| --- | --- |
| Input validation | One canonical model decoder/compiler validates versions, unique rule IDs, references, scopes, and required fix-it messages. Reject unknown fields and duplicate keys in serialized inputs. Baseline parsing validates its identities and counts. Existing `check-json`/`check-yaml` hooks prove syntax only, not these semantics. |
| Filesystem and OS | Confine scans and configured paths to the checkout. Reject traversal, absolute external paths, and symlink escapes, including model/baseline inputs. Do not follow module discovery into parent directories. Treat filenames as data, including spaces, newlines, and leading dashes. Keep checks read-only; any explicit baseline shrink write must replace a regular file atomically without following links. |
| Processes and environment | Static checks must not execute scanned source, generators, or model-specified commands. A compiled Go model/tool is candidate code: run it only in an unprivileged, secret-free job. For necessary Git reads, pass separate arguments with fixed options and validated refs; use no shell interpolation, credentials in argv, or ambient config-driven execution. Do not reuse the publishing credential path in `sync-main-to-dev.yml`. |
| Configuration and environment binding | Do not initialize the checker through `configuration.Init`: it searches host config paths and enables `GREMLINS_*` overrides. Architecture policy must come from explicit repository inputs, not `.gremlins.yaml`, user config, build tags in the environment, or mutation exclusion settings. The existing JSON schema is editor guidance; no runtime JSON-schema validator was found. Do not assume it validates checker inputs. |
| CI authorization | Follow `ci.yml`: pinned actions, `contents: read`, and checkout with `persist-credentials: false`. Run checks for foreign PRs without secrets. Preserve the separate Sonar admission validation and token-bearing scan job; never run candidate tooling in that job or under `pull_request_target` with secrets. |
| CI failure propagation | Extend the existing `PR Gate` dependency and result checks so checker failure, cancellation, or an unexpected skip blocks it. Avoid path filters that skip architecture checks on policy-only changes. Push, PR, and manual runs need defined comparison semantics. |
| Error and log output | Report rule ID, escaped relative location, problem, and fix. Do not echo source, configuration contents, environment values, or raw subprocess output. Escape control characters and GitHub annotation delimiters. Parser, read, and subprocess failures must fail visibly, not become an empty successful result. No HTTP error envelope or application auth layer exists here. |

The candidate can modify the model, checker, and CI together. Baseline comparison does not prove those
changes are trustworthy. Maintainer review and existing branch protections remain the policy-change
boundary; #76 must not claim protection against arbitrary malicious rewrites of its own checks.

## Extensibility and verification boundaries

Put the extension seam in rule data: stable ID, component/path scope, permitted relationship, and fix-it
text. A new package or dependency variation should change that data, not CI, hook dispatch, and workflow
logic separately. Keep evaluation testable with supplied filesystem and comparison inputs, following
the repository's existing injection patterns rather than introducing a global service.

Table-driven tests must distinguish allowed relationships, genuine violations, invalid inputs, and
incomplete scans. Baseline evidence must cover equal-count substitutions, duplicate occurrences, stale
entries, target-history failures, and first adoption. Include aliasing, platform/test scopes, path
escapes, and deterministic output where the selected rules depend on them. Use targeted race tests
locally and the existing hosted mutation workflow for checker logic; mutation thresholds and changed-line
policy remain #79. These are proof obligations, not a mandate for another testing framework.

## Non-goals

No mutation type, Gremlins CLI flag, configuration key, JSON report field, or runtime exit-code change.
No runtime layering rewrite, blanket baseline refresh, generalized policy platform, new service,
release channel, branch-protection administration, or duplicate Ground Control workflow.
Coding standards (#77), test standards (#78), changed-line mutation gates (#79), broad code/security
remediation (#80–#81), and upstream triage (#83) remain separate work.
ADRs 0001–0003 still govern module identity, the binary/release boundary, and upstream imports.
