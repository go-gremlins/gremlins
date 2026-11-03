# Development process review

Issue: [#75](https://github.com/OpenRAE/go-rae-gremlins/issues/75). Reviewed on 2026-11-02.

## Scope and baseline

This record compares upstream Gremlins with the process established for the OpenRAE fork and its Ground Control
workflow. It records the merged process in [CONTRIBUTING.md](../CONTRIBUTING.md). It changes documentation, with no
new automation, services, or runtime behavior.

The upstream baseline is `go-gremlins/gremlins` at
[`b48a4aad17eff33dc7262e0d1487b1a4d6eec322`](https://github.com/go-gremlins/gremlins/tree/b48a4aad17eff33dc7262e0d1487b1a4d6eec322).
The fork baseline is `OpenRAE/go-rae-gremlins` at
[`b72ab35ca67bb36f8b2051619980abb6eaaf89ed`](https://github.com/OpenRAE/go-rae-gremlins/tree/b72ab35ca67bb36f8b2051619980abb6eaaf89ed),
after the [development setup PR #64](https://github.com/OpenRAE/go-rae-gremlins/pull/64).
The OpenRAE comparison uses this fork's recorded decisions and configuration; it does not assume undocumented
organization-wide rules.

Sources checked:

- Upstream [contributor guide](https://github.com/go-gremlins/gremlins/blob/b48a4aad17eff33dc7262e0d1487b1a4d6eec322/docs/CONTRIBUTING.md)
  and [PR template](https://github.com/go-gremlins/gremlins/blob/b48a4aad17eff33dc7262e0d1487b1a4d6eec322/.github/PULL_REQUEST_TEMPLATE.md).
- Upstream [CI](https://github.com/go-gremlins/gremlins/blob/b48a4aad17eff33dc7262e0d1487b1a4d6eec322/.github/workflows/ci.yml),
  [mutation](https://github.com/go-gremlins/gremlins/blob/b48a4aad17eff33dc7262e0d1487b1a4d6eec322/.github/workflows/gremlins.yml),
  [release](https://github.com/go-gremlins/gremlins/blob/b48a4aad17eff33dc7262e0d1487b1a4d6eec322/.github/workflows/release.yml),
  and [documentation](https://github.com/go-gremlins/gremlins/blob/b48a4aad17eff33dc7262e0d1487b1a4d6eec322/.github/workflows/docs.yml)
  workflows and [GoReleaser configuration](https://github.com/go-gremlins/gremlins/blob/b48a4aad17eff33dc7262e0d1487b1a4d6eec322/.goreleaser.yaml).
- Fork [Ground Control configuration](../../.ground-control.yaml), [plan rules](../../.gc/plan-rules.md),
  [ADR 0001](../adr/0001-module-path.md), [ADR 0002](../adr/0002-binary-name-and-versions.md),
  [ADR 0003](../adr/0003-upstream-changes.md), [PR template](../../.github/PULL_REQUEST_TEMPLATE.md), and
  [pre-commit hooks](../../.pre-commit-config.yaml) with their [installer](../../scripts/install-hooks.sh).
- Fork [CI](../../.github/workflows/ci.yml), [mutation](../../.github/workflows/gremlins.yml),
  [PR-title](../../.github/workflows/pr-title.yml), [release](../../.github/workflows/release-please.yml), and
  [branch synchronization](../../.github/workflows/sync-main-to-dev.yml) workflows, plus the
  [release procedure](release.md) and [upstream release comparison](upstream-release.md).
- Installed Ground Control `/implement` and `/quickfix` skill contracts, read on the review date. These live outside
  this repository. Their SHA-256 digests at review were
  `a4ff93ddc9cc1b21a5779b3c7220b4e7a9924b8fe883e714718652983c08abd0` (`implement/SKILL.md`) and
  `09c50b8acf64c7885b35a9a96bbc1e264a6be2492b81aef9287fb1c534c0ad01` (`quickfix/SKILL.md`).
  Use the installed contracts for execution details and the repository configuration for repository-specific values.

## Comparison and decisions

| Topic | Upstream baseline | OpenRAE and Ground Control baseline | Merged process and reason |
| --- | --- | --- | --- |
| Intake | Search existing reports, use templates, coordinate significant work, and link nontrivial changes to issues. | An issue supplies the outcome and acceptance criteria; Ground Control records pickup and decisions there. Copied upstream items await triage. | Retain upstream's reporting discipline and use the fork's issue tracker and acceptance criteria. Requirements and decisions remain visible to reviewers. |
| Branches | GitHub flow, with special branches for maintenance releases. | Issue branches target `dev`; `main` receives reviewed promotions. Publication merges current `dev` into the issue branch. | Use `<issue>-<slug>` branches and PRs to `dev`. Preserve real synchronization merges and resolve conflicts without discarding work. |
| Commits and PRs | `area: description` subjects, issue references, multiple commits allowed, and a documented squash policy. | Conventional Commit titles drive release-please; the PR template asks for summary, related issues, ADR impact, and test evidence. | Use Conventional Commits, truthful check results, and issue links. Maintainers choose the merge method; a blanket squash rule would conflict with established branch maintenance. |
| Verification | The PR template asks for local `make all`, regression tests, and documentation updates. CI runs cross-platform coverage. | Targeted local checks, mandatory publication hooks, and hosted CI/Sonar gates. The hooks also run race tests for matching Go changes. | Retain regression proof, documentation, and cross-platform coverage. Use targeted checks while developing and let configured publication hooks and CI own broader gates. |
| Mutation testing | The Gremlins workflow runs a released upstream action with `version: latest`. | The fork runs `go run ./cmd/gremlins unleash --silent` from the candidate commit. | Keep the fork's candidate-source execution so verification exercises the implementation under review. The separate changed-line mutation standard remains under #79. |
| Quality and security | CI reports coverage to Codecov, Code Climate, and Codacy. | Pinned actions, restricted permissions, secret and vulnerability checks, and the configured SonarCloud project. Foreign PRs skip token-dependent Sonar jobs. | Use the fork's configured checks and inspect all checks on the current head. Upstream integrations are not authorization to send fork data to additional services. |
| Review and delivery | Automatic checks and maintainer review precede merge. | Normal Ground Control delivery adds planning and pre-push review. Quickfix omits default review; both lanes synchronize, publish, monitor, and record readiness before a human merge. | Retain maintainer review. Choose the appropriate Ground Control command and follow its review and stop rules; quickfix retains publication and hosted gates. |
| Completion | PRs can reference issues with closing keywords. | The PR body links the issue with `Closes`; Ground Control verifies the merge before its final report and issue close. | The [Phase E workflow](../../.github/workflows/ground-control-phase-e.yml) invokes the existing finalizer after merge. See [setup and recovery](phase-e.md) for the first installation and fallback paths. |
| Upstream imports | Work is developed in the upstream repository. | ADR 0003 requires cherry-picks with `-x`, original authorship, and the fork module path. | Preserve traceability and adapt imports through a PR to `dev`; never merge upstream branches. |
| Release | Semantic versions, a release freeze, tag-triggered GoReleaser, and upstream-owned distribution channels. | ADR 0002 and the release workflow use release-please on `main`, followed by GoReleaser and a reviewed synchronization PR to `dev`. | Keep semantic versioning and the fork's release procedure. Do not adopt upstream's freeze, maintenance branches, manual version edits, or publication channels. |

## Result and boundaries

[CONTRIBUTING.md](../CONTRIBUTING.md) is the contributor-facing process from issue intake through verification,
PR review, post-merge closure, and release. It replaces the inherited guide's upstream issue destination,
commit convention, branch assumptions, local full-suite instruction, and mandatory squash/release-freeze rules.
Existing ADRs 0001–0003 remain binding; this review reconciles their application and requires no new ADR.

The guide distinguishes configured checks from a successful delivery: `PR Gate` summarizes part of CI, while
mutation, title, and any installed hosted security checks must also be inspected. Ground Control readiness
requires its configured Sonar result even though foreign-fork CI permits a Sonar skip. The user owns any
`GitGuardian Security Checks` investigation and resolution.

The review records current practice without changing workflows or branch protection. Architecture modeling (#76),
coding standards (#77), test standards (#78), changed-line mutation gates (#79), code/security/documentation reviews
(#80–#82), and upstream triage (#83) remain separate items in the [maintenance plan](maintenance-plan.md).
This record does not claim those gates or reviews already exist.
