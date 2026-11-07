# Documentation review

Issue: [#82](https://github.com/OpenRAE/go-rae-gremlins/issues/82). Review date: 2026-11-05.

## Baseline and scope

The reviewed commit is
[`b272babf582a9d551915a2a06a49b6eca1fb58ad`](https://github.com/OpenRAE/go-rae-gremlins/tree/b272babf582a9d551915a2a06a49b6eca1fb58ad).
All source locations below refer to that commit. Links open the current checkout; use the pinned tree for historical evidence.

This record covers the 50 tracked Markdown paths listed by `git ls-files '*.md' '*.mdx'` at that commit.
It includes hidden contributor templates and the documentation alias.
The inventory is:

| Group | Paths reviewed | Count |
| --- | --- | --- |
| Entry points and guidance | `README.md`, `CLAUDE.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `.gc/plan-rules.md`, `docs/CONTRIBUTING.md` | 6 |
| Contributor templates | `.github/ISSUE_TEMPLATE/bug_report.md`, `.github/ISSUE_TEMPLATE/feature_request.md`, `.github/PULL_REQUEST_TEMPLATE.md` | 3 |
| Decisions | `docs/adr/README.md` and ADRs 0001–0005 | 6 |
| Development | `architecture.md`, `coding-standards.md`, `lint-design.md`, `lint-preflight.md`, `maintenance-plan.md`, `phase-e-design.md`, `phase-e.md`, `process-review.md`, `release.md`, `test-assessment.md`, `test-standard-preflight.md`, `test-standard.md`, `upstream-release.md` under `docs/development/` | 13 |
| User documentation | `index.md`, `install.md`, `quick-start.md`, `usage/configuration.md`, two command pages, `workers.md`, two CI pages, and twelve mutation pages under `docs/docs/` | 21 |
| Alias | `docs/docs/CONTRIBUTING.md` links to `../CONTRIBUTING.md`; the prose was reviewed once | 1 |

Code, the configuration schema, workflow files, and MkDocs settings supplied evidence for technical claims.
This is not a code or security audit.
License files, copyright notices, source comments, generated artifacts, images, and fixture prose are outside the prose assessment.
The Code of Conduct was reviewed for applicability; its adopted policy text is not subject to an editorial rewrite here.

The [maintenance plan](maintenance-plan.md) separates review, triage, and remediation.
This delivery records decisions. It does not claim that the findings are repaired.
Issue #83 triages the repair work before the release under #84.
No new service, publication destination, prose checker, or runtime behavior is introduced.

## ASD-STE100 method and limits

The reference is ASD-STE100 Issue 9, dated 2025-01-15.
The official [FAQ](https://www.asd-ste100.org/STE_faq.html) describes controlled vocabulary, direct instructions, and technical terminology.
The official [standard](https://www.asd-ste100.org/assets/files/ASD-STE100_ISSUE9.pdf) supplies the writing rules and dictionary.
Search-index excerpts from those official sources were available on the review date.
Direct retrieval returned HTTP 403. The complete standard and dictionary were not available for this review.

The review checks these aspects separately:

- Vocabulary and meaning: identify informal expressions and ambiguous terms; retain necessary technical names.
- Verbs and instructions: prefer active statements and direct commands. State prerequisites before the action.
- Sentences and structure: use short sentences, one topic at a time, and separate procedural actions.
- Consistency: use the same term for the same concept, as required by rule 9.4.
- Technical accuracy: compare commands, defaults, output descriptions, and fork instructions with their repository owners.

These are a limited assessment against STE principles, not a declaration of full compliance.
No word is classified as dictionary-approved without its dictionary entry.
Code tokens, paths, flags, status names, and quotations retain their exact spelling.
The proposed rewrites below are editorial examples, not certified STE text.
Manual review supplied the prose findings; there is no configured Vale or equivalent prose gate.
No external link crawl, site deployment, example execution, or fresh product test result is claimed.

## Findings and remediation decisions

Priority **high** means that the text can cause an incorrect command, configuration, or product choice.
Priority **medium** means that the text can mislead readers about behavior or procedure.
Priority **low** means an editorial improvement.
Each decision states a specific repair or a reason to retain the text.
All repair decisions remain open for #83; retained text does not imply repository-wide STE compliance.

### D01: Fork entry points lead to upstream resources — high

Evidence: [README](../../README.md), badges and “How to use Gremlins”; `docs/mkdocs.yml`, repository and site settings.
The badges describe upstream results, and the main documentation link opens `gremlins.dev`.
[ADR 0002](../adr/0002-binary-name-and-versions.md) states that this fork publishes no documentation site.
Readers can mistake upstream health and instructions for fork evidence.

Decision: **repair**. Link fork usage instructions to this repository. Remove misleading badges or identify their upstream scope.
Keep historical upstream links where the surrounding text identifies them.
Do not deploy a site or register another service as part of the repair.
Verification: follow each fork entry point and compare badge targets with the repository's configured checks.

### D02: Release support promises lack a fork decision — medium

Evidence: [README](../../README.md), “Gremlins version”; [site introduction](../docs/index.md), warning.
Both promise a particular minor-release maintenance policy and discuss a future 1.x policy.
The [fork release procedure](release.md) records a different release process but no corresponding support commitment.

Decision: **repair**. Separate version compatibility guidance from support commitments.
Link to ADR 0002 and the fork release procedure; remove inherited promises unless a maintainer adopts them.
Verification: compare the resulting support statements with the recorded fork decision and release metadata.
Do not infer a released version from a future release plan.

### D03: Status-filter example uses an invalid flag — high

Evidence: [unleash reference](../docs/usage/commands/unleash/index.md), “Filter out out”; [`cmd/unleash.go`](../../cmd/unleash.go), `setFlagsOnCmd`.
The example uses `--S lctv`. `S` is the shorthand for `--output-statuses` and requires one leading hyphen.
The underscore variants elsewhere are accepted by the existing flag normalizer; they are not rejected flags.

Decision: **repair**. Use `-S lctv` or `--output-statuses lctv`; use canonical hyphenated long names consistently.
Verification: compare every command example with flag registration and normalization, then exercise corrected examples in a controlled fixture.

### D04: Environment example omits part of the key — high

Evidence: [configuration](../docs/usage/configuration.md), environment example;
[`internal/configuration/configuration.go`](../../internal/configuration/configuration.go), `Init` and `MutantTypeEnabledKey`.
The YAML key is `mutants.arithmetic-base.enabled`; the example uses `GREMLINS_MUTANTS_ARITHMETIC_BASE`.
The configured replacement produces `GREMLINS_MUTANTS_ARITHMETIC_BASE_ENABLED`.

Decision: **repair**. Include `_ENABLED` and explain the complete key-to-variable mapping.
Verification: use isolated configuration inputs to demonstrate that the documented variable overrides the corresponding YAML value.

### D05: Configuration search locations and order disagree with code — high

Evidence: [configuration](../docs/usage/configuration.md), “Location”;
[`defaultConfigPaths`](../../internal/configuration/configuration.go).
The page lists the current directory first and `$HOME/.gremlins.yaml` as a file.
The implementation registers global locations first, a repeated `gremlins` XDG directory component, and `$HOME/.gremlins` as a directory.
The displayed list does not describe those inputs.

Decision: **repair after behavior review**. Establish intended precedence and paths before rewriting the instructions.
Keep any required runtime repair distinct from the editorial change; do not make the prose silently approve a defect.
Verification: test competing files and explicit `--config` input in isolated directories on applicable operating systems.

### D06: “Complete” configuration example omits supported keys — medium

Evidence: [configuration](../docs/usage/configuration.md), reference example;
[`cmd/unleash.go`](../../cmd/unleash.go), `paramCoverPackages` and `paramOutputDiffStatuses`;
[schema](../docs/schema/configuration.json).
The example claims to list all properties but omits `unleash.coverpkg` and `unleash.output-diff-statuses`.

Decision: **repair**. Add the supported keys with verified defaults, or identify the example as a subset.
Verification: compare the example with flag bindings and the schema without changing either contract to match an incomplete example.

### D07: Worker and timeout guidance conflicts with implementation — high

Evidence: [workers](../docs/usage/commands/unleash/workers.md), timeout and integration sections;
[`executor.go`](../../internal/engine/executor.go), `DefaultTimeoutCoefficient` and `NewExecutorDealer`;
[`workerpool.go`](../../internal/engine/workerpool/workerpool.go), `size`.
The page gives a default coefficient of 3; the code and unleash page use 5.
It says integration mode halves test CPU values; the executor currently divides a nonzero value by itself.
The proposed sum of workers and test CPU counts is a heuristic, not a demonstrated resource bound.

Decision: **repair after behavior review**. Correct the coefficient and remove the unsupported resource rule.
Resolve the intended test CPU behavior before documenting it as a guarantee.
Verification: compare both pages with worker sizing, test arguments, and timeout calculation in controlled cases.

### D08: Threshold prose overstates success and omits equality — high

Evidence: [unleash reference](../docs/usage/commands/unleash/index.md), both threshold sections;
[`report.go`](../../internal/report/report.go), `assess`; [`main.go`](../../cmd/gremlins/main.go).
The page says zero means Gremlins never exits with an error. Other errors still return a nonzero status.
The implementation rejects values equal to a positive threshold as well as values below it.
The displayed ratio formulas also omit the multiplication by 100 used for percentage values.

Decision: **repair**. Say that zero disables that threshold check only.
State the observed comparison and percentage units; confirm equality behavior against the intended contract before changing code.
Verification: cover below, equal, above, disabled, and ordinary-error cases; retain exit codes 10 and 11 unless separately authorized.

### D09: Status descriptions imply that a timeout proves detection — medium

Evidence: [README](../../README.md) and [quick start](../docs/quick-start.md), `TIMED OUT` descriptions;
[test standard T10](test-standard.md#t10-keep-measurement-evidence-honest).
Both descriptions equate a timeout with a mutation making tests fail.
These lists also omit the supported `SKIPPED` status.

Decision: **repair**. Describe a timeout as exceeding the allowed execution time, with no claim about which assertion detected a defect.
Explain `SKIPPED` and distinguish it from `NOT COVERED` using the status owners.
Verification: compare each description with [`mutator.go`](../../internal/mutator/mutator.go) and executor/report transitions.

### D10: Silent-mode prose names the wrong error destination — medium

Evidence: [command reference](../docs/usage/commands/index.md), “Silent”;
[`main.go`](../../cmd/gremlins/main.go), Logger initialization; [`log.go`](../../internal/log/log.go).
The page promises errors on STDOUT. The binary supplies the error writer separately through `color.Error`.
The long explanation also mixes output suppression, report files, and exit checks.

Decision: **repair**. Explain normal-output suppression and error output separately, with the actual stream verified at the binary boundary.
Use short active statements rather than the passive “will be reported”.
Verification: capture stdout, stderr, and exit status for silent success and failure in a controlled process.

### D11: GitHub Actions example has an invalid job structure — high

Evidence: [GitHub Action guide](../docs/usage/ci/github-action.md), example;
[candidate-source workflow](../../.github/workflows/gremlins.yml).
The sample puts a sequence directly under the job and omits `runs-on` and `steps`.
It selects the upstream action with a floating product version, without explaining that it runs upstream Gremlins.

Decision: **repair**. Supply a valid job and identify fork versus upstream execution explicitly.
Reuse the repository's established candidate-source approach or an approved pinned fork binary.
Verification: validate workflow syntax and verify which module/version the example executes. Do not add an integration service.

### D12: Docker instructions do not identify the upstream boundary — high

Evidence: [Docker guide](../docs/usage/ci/docker.md); [release procedure](release.md), distribution comparison.
The guide presents `gogremlins/gremlins` as a general installation channel.
The fork publishes no Docker image, as ADR 0002 records.

Decision: **repair**. Label the images as upstream-only or replace the page with supported fork CI instructions.
Do not publish a fork image to make the inherited instructions true.
Verification: compare all distribution claims with the fork release configuration and ADR 0002.

### D13: Report example is incomplete and not valid standalone JSON — medium

Evidence: [unleash reference](../docs/usage/commands/unleash/index.md), “Output”;
[`structure.go`](../../internal/report/internal/structure.go).
The `json` block includes comments, omits `mutator_statistics`, and gives component counts whose sum exceeds `mutants_total`.
Readers cannot copy it as a complete valid report.

Decision: **repair**. Use a valid, internally consistent illustrative report and move annotations outside the JSON block.
Describe all current fields or state the example's limited scope.
Verification: parse the example and compare its fields and counts with the canonical report structure and controlled report output.

### D14: Figurative prose obscures the technical explanation — low

Evidence: [README](../../README.md), mutation introduction; [quick start](../docs/quick-start.md), “why bother”;
[mutation overview](../docs/usage/mutations/index.md), “flavour”; [workers](../docs/usage/commands/unleash/workers.md), “play a little”.
These expressions require interpretation and do not name a technical action.

Decision: **repair**. Use literal statements, for example: “Gremlins changes covered source code and runs the tests.”
For tuning, name the setting and the observation that warrants a change.
Verification: review the affected prose for idioms and rhetorical questions; preserve the technical meaning.

### D15: Dense sentences combine instructions and explanations — medium

Evidence: [README](../../README.md), opening paragraph; [Contributing](../CONTRIBUTING.md), “Ground Control delivery” step 3;
[Phase E guide](phase-e.md), paragraph beginning “They verify both events”; [process review](process-review.md), comparison table.
Several sentences combine multiple actions or long lists of concepts.
This conflicts with the STE principles of short sentences and one topic at a time.

Decision: **repair**. Split commands into ordered actions and move descriptive detail into separate sentences or lists.
For example: “The workflow stages the diff and scans for sensitive content. It then runs the configured pre-commit checks.”
Verification: review sentence boundaries and action order manually, including table cells. Do not count line wrapping as sentence splitting.

### D16: Technical names and editorial forms are inconsistent — low

Evidence: [mutation overview](../docs/usage/mutations/index.md), mixed `YES`/`FALSE` defaults and “mutant type”/“mutation type”;
[configuration](../docs/usage/configuration.md), “behaviours” and “Yaml”; [README](../../README.md), “test suit”;
[unleash reference](../docs/usage/commands/unleash/index.md), “give output file” and repeated words.
These differences hinder consistent interpretation.

Decision: **repair**. Select one term per concept, use `true`/`false` for Boolean defaults, and retain exact flag/status spellings.
Correct grammar and use consistent American English spelling in new prose.
Verification: search all active prose for the selected terms and repeat the manual grammar review.
Do not change code identifiers, report keys, quotations, or license language for spelling consistency.

### D17: Feature count is incorrect — medium

Evidence: [site introduction](../docs/index.md), “Supports five mutant types”;
[mutation overview](../docs/usage/mutations/index.md); [`mutator.go`](../../internal/mutator/mutator.go), `Types`.
The code supports eleven types. Five are enabled by default.

Decision: **repair**. Distinguish supported types from default-enabled types and link to the existing table.
Verification: compare the statement with `Types` and mutation flag defaults; avoid a second independent type inventory.

### D18: Contributor templates retain upstream routing — medium

Evidence: [bug template](../../.github/ISSUE_TEMPLATE/bug_report.md) and
[feature template](../../.github/ISSUE_TEMPLATE/feature_request.md), `assignees: k3rn31`;
[Code of Conduct](../../CODE_OF_CONDUCT.md), enforcement contact `contact@gremlins.dev`.
The fork guide directs contributors to OpenRAE, but these entry points retain upstream contacts.
Their suitability for this fork is not established by the repository.

Decision: **repair with a maintainer-selected contact**. Remove inherited automatic assignment or use an approved fork owner.
Confirm a private conduct-reporting channel before replacing its address. Do not send a test report or invent an address.
Verification: inspect template metadata and the recorded contact decision; keep conduct reporting separate from vulnerability reporting in `SECURITY.md`.

### D19: Build guidance can be mistaken for the daily verification procedure — medium

Evidence: [CLAUDE.md](../../CLAUDE.md), “Development Workflow”, describes `make all` as the full development cycle;
[Contributing](../CONTRIBUTING.md), “Verify the change”, assigns full runs to CI.
The command exists, but its suggested purpose is unclear across the two pages.

Decision: **repair**. Label broad commands as repository capabilities and link daily verification to the contributor guide.
Verification: compare command descriptions across both pages with the targeted-local/full-hosted rule.
Keep the executable publication hooks unchanged.

### D20: Historical and adopted text needs explicit treatment — low

Evidence: [upstream release comparison](upstream-release.md), upstream publication channels;
ADRs 0001–0005, conditional acceptance status and historical context;
[Code of Conduct](../../CODE_OF_CONDUCT.md), adopted policy and attribution.
These texts can contain terminology or historical statements that differ from current instructions.

Decision: **retain with context**. Keep upstream channels in the explicitly historical comparison and preserve adopted policy and attribution.
When updating ADR status, record the acceptance event; do not erase the historical rationale or rewrite legal text for STE style.
Verification: ensure active guides link to current decisions and clearly distinguish historical descriptions from operator instructions.
No formal STE conformance claim is made for retained text.

## Acceptance check and follow-up

ManualFindingDecisionAudit: all 20 findings have source evidence, a remediation decision, and verification needed for closure.
The scope inventory accounts for all 50 baseline Markdown paths, including the alias.
The ASD reference, method, and access limits are stated separately from technical accuracy findings.
This satisfies the issue's requirement for a review record and a decision for each finding.

No finding is closed by this record. The retained-text decision in D20 is contextual treatment, not a suppression allowance.
The existing install guide and private vulnerability-reporting instructions need no separate accuracy finding from this review.
That observation is not a full STE or security certification.

Triage #83 should group related repairs and carry these IDs into its decisions.
Address incorrect examples and fork routing in the remediation issues. The release under #84 does not wait for them.
Resolve behavior questions in D05, D07, and D08 before prose changes imply an approved runtime contract.
For each repair, record the revision, affected IDs, verification result, and remaining limits.
Preserve this commit as the assessment baseline.
