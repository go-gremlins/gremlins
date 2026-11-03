# Contributing to go-rae-gremlins

go-rae-gremlins is the OpenRAE fork of Gremlins. Use the
[OpenRAE issue tracker](https://github.com/OpenRAE/go-rae-gremlins/issues) for work on this fork and follow the
[Code of Conduct](../CODE_OF_CONDUCT.md).

The [development process review](development/process-review.md) compares the upstream conventions with the
OpenRAE and Ground Control process and records the decisions behind this guide.

## Start with an issue

Search existing issues before reporting a bug or suggesting a feature. Use the issue templates where applicable.
For bugs, include the Gremlins version, Go version, operating system, reproduction steps, and expected and actual
results. For features, explain the problem, the proposed behavior, and alternatives.

Coordinate significant changes with a maintainer before implementation. Give the issue a clear outcome and
checkable acceptance criteria. Ground Control takes an issue number as its input; the issue body and discussion
carry the requirements and decisions.

Copied upstream issues and PRs need a keep, rework, or close decision under
[the maintenance plan](development/maintenance-plan.md) before implementation or merge.

## Prepare the clone

Install Go as specified by `go.mod`, [mise](https://mise.jdx.dev), and
[pre-commit](https://pre-commit.com). Install the pinned linter from `mise.toml`:

```shell
mise install aqua:golangci/golangci-lint
make hooks
```

`make hooks` installs and verifies the clone's hook dispatch and runs an initial check of all files. That initial
check includes the configured Go checks and race tests. Run it for each new clone; Git does not copy local hooks.
If installation reports an existing unmanaged hook or a dispatcher problem, inspect and resolve that problem
before committing. Do not bypass the hooks or overwrite a shared hook configuration.

The [pre-commit configuration](../.pre-commit-config.yaml) checks file hygiene, syntax, merge conflicts, private
keys, and secrets. Go changes also run lint, vulnerability checks, and race tests. These are publication gates;
daily development checks should target the changed behavior.

## Branch and implement

Start an issue branch from current `dev`, named `<issue>-<short-slug>`, and target its PR at `dev`. `main` receives
reviewed promotions from `dev`; maintainers approve those merges and all feature PR merges.

Keep the issue branch current by fetching and merging `origin/dev` into it. Resolve conflicts with a real merge
and verify the result. Preserve existing work and history; do not use a hard reset or force-push to discard
conflicts. Ground Control performs this synchronization at publication and rechecks it before PR creation and
readiness.

Read `CLAUDE.md`, the [plan rules](../.gc/plan-rules.md), and the [ADRs](adr/README.md) before changing code. Reuse
existing helpers and dependencies. Keep the change bounded by the issue, and update the documentation it affects.
Changes to flags or configuration keys must also update the JSON schema and usage pages. Preserve the CLI,
configuration, report, and exit-code contracts used by dependent repositories unless the issue authorizes a change.
Keep `testdata` files byte-exact unless the fixture itself needs to change.

Before coding, follow the [architecture design check](development/architecture.md). Update
`.architecture/design.json` with the proposed packages, dependencies, affected contracts, trust boundaries,
and unresolved conflicts. Run `make architecture-design` and record its model digest and result in the issue plan.
Resolve conflicts before implementation. Recheck when the design or model changes. CI requires an updated design
for package, contract, and policy changes and checks every changed package against it.

Run `make architecture ARCHITECTURE_BASE=origin/dev` to scan all packages and compare baseline identities against
a fetched target commit. Tests and platform files are included. Each finding gives a repair instruction.
Remove stale baseline entries when fixing violations. Never regenerate or enlarge the baseline. Architecture
policy changes need maintainer review. CI runs this check without secrets and requires it in `PR Gate`.

Follow the [Go coding standard and upstream comparison](development/coding-standards.md).
Provision the pinned linter with mise and download Go modules before analysis.
`make lint` checks formatting and analyzes Linux, Darwin, and Windows with tests.
It compares lint identities and counts against the fetched target baseline.
CI passes the event's comparison commit to both lint and policy checks.
The first `main` promotion adopts the reviewed `dev` baseline, and rejects any additions to it.
For local checks, use `make lint LINT_BASE=origin/dev` or another fetched target commit.
Repair findings and run `make lint-shrink` to remove stale allowances.
The shrink command cannot add or replace debt. Do not regenerate the baseline.
Changes to rules, suppressions, contexts, and tool versions need explicit review.

For upstream commits, follow [ADR 0003](adr/0003-upstream-changes.md): cherry-pick with `-x`, retain the original
author, adapt the module path, and open a PR to `dev`. Do not merge an upstream branch into this fork.

## Verify the change

Follow the [Go test standard](development/test-standard.md) for test behavior, isolation, and evidence.
The [test assessment](development/test-assessment.md) records current gaps against that standard.

Add a regression test for a bug or tests for new behavior. Run the narrowest relevant tests with the race detector,
for example:

```shell
go test -race ./internal/coverage -run '^TestCoverageParsesOutput$'
```

Choose the package and test names that exercise your change. Check mutation results where applicable to test
effectiveness. For documentation-only changes, check accuracy, links, and the diff; no new Go test is needed.
Format changed Go files with the pinned formatter:

```shell
mise exec -- golangci-lint fmt path/to/changed.go
```

CI owns the full verification runs. The repository still provides `make all` (lint, tests, build), `make cover`
(race tests and coverage), and `make policy` (all configured hooks), but do not repeat those broad runs as a daily
local completion check. Run targeted checks locally, then let the publication hooks and hosted checks enforce
their configured gates.

The current hosted checks are defined in the repository:

| Workflow | Verification |
| --- | --- |
| [CI](../.github/workflows/ci.yml) | Architecture model/design checks, shrink-only baselines, lint, Python line/branch coverage, race tests and Go coverage on Linux/macOS/Windows, policy hooks, Sonar admission, and SonarCloud. `PR Gate` combines architecture, lint, tests, policy, and Sonar results. |
| [Gremlins](../.github/workflows/gremlins.yml) | Mutation testing with the Gremlins source from the candidate commit. |
| [PR title](../.github/workflows/pr-title.yml) | A Conventional Commit PR title with a lowercase subject. |

CI deliberately skips the Sonar jobs for PRs from other repositories because they have no access to the token;
`PR Gate` accepts that specific skip. Ground Control readiness still requires the configured Sonar result.
Inspect every check on the current PR head, including installed security checks. A green `PR Gate` alone does
not establish that every hosted check passed.

Repair failures caused by or directly blocking the change and verify the new head. Report unrelated concerns with
evidence without expanding the issue. For Ground Control runs, `GitGuardian Security Checks` findings belong to
the user: report the check name, status, and URL, then stop for the user's resolution.

## Commit messages and pull requests

Use Conventional Commits for commit messages and PR titles, such as:

```text
fix(coverage): preserve slash-separated profile paths
```

Allowed types are `feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `build`, `ci`, `perf`, `style`, and `revert`.
A scope is optional; the subject starts with a lowercase letter. This replaces upstream's `area: description`
convention and supports the fork's release automation.

Keep branch names and repository metadata free of assistant or vendor attribution. Required Ground Control review
records and the required review attestation are the narrow exceptions.

Use the [PR template](../.github/PULL_REQUEST_TEMPLATE.md) to describe the resulting behavior, link the issue,
state ADR impact, and report actual checks and results. Ground Control renders its own PR body with the same
evidence and its required attestation. Include `Closes #<issue>` for the delivered issue. GitHub closes issues
automatically only on the default branch, so a merge into `dev` still needs explicit post-merge finalization.

Maintainers review the change and choose the merge method. Multiple commits are acceptable; an upstream rule to
squash every PR does not override synchronization merges or the maintainer's choice here. Merge only after the
current head has the required checks and review. A contributor or agent preparing the PR does not merge it.

## Ground Control delivery

[`.ground-control.yaml`](../.ground-control.yaml) supplies this repository's commands, `dev` base branch, PR-title
rules, Sonar project, and review cap. Ground Control's installed skills and tools supply the workflow contract.
Contributors working manually still follow the branch, hook, verification, review, and release rules above.

1. Invoke `/implement <issue>` for the normal issue workflow, or `/quickfix <issue>` for a small, settled change.
   The issue branch and pickup record identify the work. The normal workflow adds design, planning, and pre-push
   review; quickfix omits those heavier steps and runs no pre-push review unless `--review` is supplied.
2. Implement the issue's acceptance criteria and run targeted verification. Record significant decisions and any
   required review findings on the issue. A quickfix with `--review` runs one pre-push review cycle; act on its
   findings and follow the returned stop conditions.
3. Let the workflow publish the change. It stages the diff, scans for sensitive content, runs configured pre-commit
   checks, commits, pushes, and synchronizes with current `dev`. Repair a failed gate and retry through the workflow.
4. Let the workflow render and create the synchronized PR, observe CI and Sonar, and record readiness for the
   checked head. A later head change requires a new readiness check. Readiness hands the PR to the maintainer for
   merge; it does not close the issue.
5. After the maintainer merges, the [Phase E workflow](../.github/workflows/ground-control-phase-e.yml) calls the
   existing finalizer to post the final report and close the issue. Follow [setup and recovery](development/phase-e.md)
   for release access, first installation, restricted fork events, or a failed run. When readiness reports
   `finalization: reentry`, re-invoke the same `/implement <issue>` or `/quickfix <issue>` after merge.

## Release

Follow the [fork release procedure](development/release.md) and [ADR 0002](adr/0002-binary-name-and-versions.md).
Maintainers approve the `dev` to `main` promotion and the release PR. release-please manages versions, tags, release
notes, and `CHANGELOG.md`; GoReleaser adds the binaries, source archive, and checksums. Review and merge the automated
`main` to `dev` synchronization PR afterward.

The fork does not inherit upstream's release freeze or maintenance-release branches. Release and synchronization
PRs opened with `GITHUB_TOKEN` do not trigger the normal PR checks; maintainer review remains necessary. Do not edit
release versions or `CHANGELOG.md` manually, or publish to upstream Docker, Homebrew, or documentation channels.
