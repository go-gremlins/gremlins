# Repository code quality review

Issue: [#80](https://github.com/OpenRAE/go-rae-gremlins/issues/80). Review date: 2026-11-04.
Inspected revision: `17ebec658fdd89f925138d5bf0b5be7b4c02054f`.

## Result and scope

The package design is coherent and has no dependency cycles. However, the current implementation can test the wrong
package, miss changed lines, misclassify failed builds, discard configuration, and report success after an incomplete
operation. Correct these result-integrity and resource-lifetime defects before relying on the tool as a downstream
merge gate. Passing architecture checks or existing tests does not resolve these findings.

This record covers the entire tracked repository: 213 paths and 18 Go packages at the inspected revision.
Separate agent reviews covered architecture, runtime code quality, maintainability/extensibility, and test quality.
The primary reviewer performed the security review and consolidated the evidence. No implementation workflow was used.
This change delivers the review and remediation decisions; it does not repair the implementation.

| Area | Reviewed material and assessment |
| --- | --- |
| Entry points | `cmd`, `cmd/gremlins`, and `cmd/internal/flags`: command composition, flags, exits, signals, configuration binding, and tests. |
| Input and selection | `internal/configuration`, `internal/gomodule`, `internal/coverage`, `internal/diff`, and `internal/exclusion`: conversion, discovery, coverage coordinates, diff parsing, and exclusion rules. |
| Mutation execution | `internal/engine`, `internal/engine/workdir`, and `internal/engine/workerpool`: AST mappings, apply/restore, copying, scheduling, platform helpers, and tests. |
| Shared concerns and output | `internal/log`, `internal/execution`, `internal/mutator`, `internal/report`, and `internal/report/internal`: logging, errors, finite types, thresholds, JSON, and tests. |
| Static tools | All implementation files of `tools/architecture`, `tools/lintcontext`, and `scripts/lint.py`; their tests, model, design, baselines, and ADRs. |
| Automation and distribution | All eight workflows, Phase E transport/fixtures/manifests, hook scripts/configuration, Makefile, Dockerfile, release configuration, dependency pins, and service configuration. |
| Documentation and fixtures | Contributor/development/usage documentation, configuration schema, fixed test inputs, and tracked evidence. Image assets were inventoried; no image-content or license-provenance audit was performed. |

The review follows the [coding standard](coding-standards.md), [test standard](test-standard.md), and
[architecture decisions](architecture.md). It builds on the [test assessment](test-assessment.md) rather than
replacing its G01–G12 findings. Vendored dependency implementations, private external utility internals, live service
permissions, Git history secret scanning, and actual Windows/macOS execution were outside this checkout review.

## Decision rules

**High** means incorrect gate results, incomplete results presented as success, or material host-resource exposure.
**Medium** means a bounded correctness, lifecycle, contract, or verification defect. **Low** means maintenance debt
without a demonstrated current result failure. These are engineering priorities, not CVSS scores.

**Fix** means a remediation is required; it is not a claim that this review fixed it.
**Defer** names the next work or change that must address the debt and explains why it is not immediate.
The [maintenance plan](maintenance-plan.md) assigns remediation issue creation and overlap resolution to #83.
Existing #1 items remain there for triage; this review creates no duplicate issues and closes no existing findings.
Verification below each finding is the closure requirement, unless explicitly identified as already measured.

## Runtime findings

### R01 — High — Environment configuration becomes zero values

Evidence: [configuration.Get](../../internal/configuration/configuration.go), lines 183–189, asserts the type of
`viper.Get` and ignores failure. Actual environment strings for dry run, workers, and efficacy threshold read back
as `false`, `0`, and `0`. An intended quality threshold can silently disappear.

**Decision: Fix.** Use Viper's supported typed conversions through the existing configuration boundary and report
invalid values. Reuse that boundary for exclusion string slices instead of the direct Viper bypass in
[exclusion.New](../../internal/exclusion/rules.go), lines 24–26. Verify consuming types and flag/file/environment precedence.

### R02 — High — Discovered configuration errors are discarded

Evidence: [configuration.Init](../../internal/configuration/configuration.go), line 92, ignores every discovered-file
read error. A `.gremlins.yaml` containing `unleash: [malformed` returns no initialization error when found by directory
search. An explicit file path does return the error, so equivalent input has inconsistent failure behavior.

**Decision: Fix.** Ignore only the supported missing-file error. Return malformed and unreadable configuration errors
with context. Verify both explicit-file and directory-discovery paths before any test process starts.

### R03 — High — Module copying does not close files

Evidence: [workdir.doCopy](../../internal/engine/workdir/workdir.go), lines 157–173, closes neither source nor destination,
including failure paths. A 150-file copy increased open descriptors from 7 to 307 in a controlled Linux probe with
GC disabled. Finalizers do not provide timely resource ownership.

**Decision: Fix.** Close both files on every path and handle write/close errors. Keep standard `io.Copy`.
Verify a many-file copy under a controlled descriptor limit and cleanup after failed copying; do not raise host limits
to hide the leak.

### R04 — High — Failed builds and launch errors get mutation outcomes

Evidence: [executor.runTests and getTestFailedStatus](../../internal/engine/executor.go), lines 216–226 and 290–298,
report an executable launch failure as `LIVED` and exit code 1 as `KILLED`. A real compile-invalid `go test` package
exits 1, whereas the current test double assigns a build failure exit 2. Compile failures can falsely improve efficacy.

**Decision: Fix.** Return infrastructure errors and distinguish failed compilation from failed tests using supported
Go test events or an explicit build step. Verify tiny real packages for passing tests, failing assertions, failed
compilation, and a missing executable. Keep ordinary wrapped Go errors and shared exit handling.

### R05 — High — Failed mutations leave missing results or changed state

Evidence: [executor.Start](../../internal/engine/executor.go), lines 165–198, panics on workdir failure, drops a mutant
after failed apply, and only logs failed rollback before reusing that worker directory. In
[TokenMutator.Apply](../../internal/engine/tokenmutator.go), lines 124–132, a write failure returns before restoring the
shared AST token. Subsequent results can use contaminated source or AST state.

**Decision: Fix.** Own apply/run/restore as one operation, defer AST restoration, and propagate failures. Stop or
discard/recreate a worker directory after failed source restoration. Verify injected read/write/restore failures,
that every scheduled mutant has a result or run error, and that later mutants start from original source.

### R06 — High — Discovery can silently return partial results

Evidence: [Engine.Run and runOnFile](../../internal/engine/engine.go), lines 104–112 and 123–137, discard walk, open, and
parse errors. A failed interface-valued open also leaves an unsafe close path. `Run` has no error result through which
to stop a successful partial report. Directory entries named with a `.go` suffix are not excluded as directories.

**Decision: Fix.** Propagate discovery errors through the engine and command boundary. Inspect directory-entry type
before parsing. Verify failed walks/opens, malformed source, and a directory named `something.go`; no case may report
a successful incomplete assessment.

### R07 — High — Package routing depends on declaration spelling

Evidence: [Engine.pkgName](../../internal/engine/engine.go), lines 160–179, searches ancestor names using the source
package identifier. A file `different/file.go` declaring legal `package named` is routed to `example.org/demo` instead
of `example.org/demo/different`. The existing main-package defect in [#1](https://github.com/OpenRAE/go-rae-gremlins/issues/1)
is the same class of error.

**Decision: Fix through #1's package-routing remediation.** Derive the import path from the module-relative directory
or supported Go package metadata. Verify main packages, differently named packages, versioned directories, and invocation
from a subdirectory. Do not add another package-name heuristic.

### R08 — High — Diff selection loses separated edits

Evidence: [newChanges](../../internal/diff/diff.go), lines 34–47, represents every hunk's additions as one contiguous
range. A valid hunk changing lines 2 and 6 becomes `{2,3}`: changed line 6 is missed and unchanged line 3 is selected.
Lines 55–57 also treat disabled diff and an enabled empty diff identically, selecting everything.

**Decision: Fix through #1's changed-lines remediation.** Consume the established parser's individual hunk lines,
tracking actual new-file positions. Distinguish disabled selection from an empty result. Verify separated edits,
additions, deletions, renames, and no changes against independently expected line sets.

### R09 — High — Source discovery includes fixture and unrelated trees

Evidence: [Engine.Run](../../internal/engine/engine.go), lines 104–112, descends every directory and excludes only
non-Go and `_test.go` filenames plus configured regex matches. It has no automatic `testdata` or nested-module boundary.
The fixture mutation defect is already recorded in #1.

**Decision: Fix through #1's source-selection remediation.** Respect Go package discovery and fixture/module boundaries;
do not solve this with a growing set of user-supplied regex defaults. Verify that ordinary package code is found and
fixture, vendor, and separate-module trees are not scheduled as part of the current module.

### R10 — High — Covered case expressions remain an unresolved result defect

Evidence: #1 records executed `case` expressions reported as uncovered. The current
[coverage lookup](../../internal/coverage/profile.go), lines 27–41, still bases selection only on coverage-block token
coordinates; there is no case-expression association in that lookup. This review did not independently replay the
original downstream case, so its observed failure remains evidence from #1, not a new measured result.

**Decision: Fix through #1's coverage remediation.** First reproduce with a tiny switch whose tested case expression
contains a supported operator. Establish the expected coverage relationship from Go instrumentation, then verify the
mutant is tested. Do not mark an entire file covered to compensate for the missing association.

### R11 — High — Cancellation and shutdown do not own the whole run

Evidence: [runWithCancel](../../cmd/unleash.go), lines 122–152, leaves a watcher after normal completion and shares
`cancelled` without synchronization with that watcher. [Coverage](../../internal/coverage/coverage.go), lines 61–66
and 97–157, has no run context; [mutant tests](../../internal/engine/executor.go), line 202, derive from
`context.Background()`. [Engine.executeTests](../../internal/engine/engine.go), lines 202–234, stops its pool only on
cancellation; normal runs leave workers waiting. Cancellation can also strand discovery at the unchecked send on
line 156. Finally, [signal handling](../../cmd/gremlins/main.go), lines 56–64, closes a still-registered signal channel.
A controlled replay of that exact function with SIGINT followed by SIGTERM panicked with `send on closed channel`.

**Decision: Fix as a bounded lifecycle change.** Pass one run context through coverage, queueing, and execution;
derive timeouts from it; close and join owned producers/workers on every terminal path. Prefer synchronous command
composition and standard [signal.NotifyContext](https://pkg.go.dev/os/signal#NotifyContext) with its stop function.
Verify normal/no-mutant/repeated runs, cancellation at each stage, full-queue cancellation, and two termination signals
during slow shutdown under the race detector. Preserve cancellation as an explicit command result.

### R12 — Medium — Coverage changes the caller's working directory

Evidence: [Coverage.Run](../../internal/coverage/coverage.go), line 99, ignores `os.Chdir` failure and never restores
the directory. Subprocesses rely on that global change. Later relative report paths resolve from the module root,
and relative module roots can acquire a second meaning after the directory change.

**Decision: Fix.** Resolve paths at command composition, use `exec.Cmd.Dir`, and remove the global directory change.
Verify explicit relative/absolute module paths, subdirectory invocation, relative output destinations, and unchanged
caller CWD. Avoid changing existing public path semantics without a documented compatibility decision.

### R13 — Medium — Custom parsing misreads valid module files

Evidence: [gomodule.modPkg](../../internal/gomodule/gomodule.go), lines 68–75, assumes the first line contains the
module declaration. A valid file beginning `// license` returns that comment as the module name without error.
[Configuration root discovery](../../internal/configuration/configuration.go), lines 154–170, also duplicates the
module-root search despite `gomodule` having no configuration dependency.

**Decision: Fix the parser with supported Go module metadata/parsing**, such as
[modfile.Parse](https://pkg.go.dev/golang.org/x/mod/modfile#Parse), rather than extending a custom grammar.
Verify comments, blanks, quoted paths, malformed declarations, and normalized paths. Defer deduplicating root discovery
until this repair; reuse a small function or pass the root, with a reviewed model change if an import direction changes.

### R14 — Medium — Requested JSON output can fail while the command succeeds

Evidence: [fileReport](../../internal/report/report.go), lines 166–203, discards marshal/close errors and only logs
create/write failures. A missing parent directory for `result.json` returns no error from `report.Do`.

**Decision: Fix.** Return output errors through `fileReport`, `reportFindings`, and `Do`. Verify invalid destinations
and failed writes/closes while preserving the existing JSON fields. No custom exception hierarchy is needed.

### R15 — Medium — Exact threshold equality fails

Evidence: [reportStatus.assess](../../internal/report/report.go), lines 238 and 245, uses `<=` for a documented minimum.
A 100% killed result fails a threshold of 100. The successful branches of
[TestAssessment](../../internal/report/report_test.go), around line 280, return without checking for an unexpected error.
`Do(Results{})` also returns before assessment or file output; that empty-run policy is currently implicit.

**Decision: Fix.** Accept equality for both minimum thresholds and assert successful as well as failing cases.
Define empty-run behavior explicitly before changing it; test that decision without inventing a new status or exit code
solely for this review.

### R16 — Medium — An enabled mutation leaves its operator unchanged

Evidence: [tokenMutations](../../internal/engine/mappings.go), line 94, maps `REM_ASSIGN` to itself for
`InvertAssignments`. This produces a behaviorally identical candidate that correct tests cannot kill.

**Decision: Fix.** Remove the candidate unless a meaningful documented replacement is selected. Verify independent
expected operator changes and that supported mappings are not identities; retain intentional fixture bytes.

### R17 — Medium — Numeric controls lack runtime validation

Evidence: [flag registration](../../cmd/unleash.go), lines 220–222, accepts signed integers without range validation.
[workerpool.size](../../internal/engine/workerpool/workerpool.go), lines 95–103, preserves a negative worker count,
creating no workers; queue submission can then block. A negative timeout coefficient creates an already-expired deadline
in [NewExecutorDealer](../../internal/engine/executor.go), lines 88–121. No hang was deliberately executed for this review.

**Decision: Fix.** Validate worker/CPU counts, timeout coefficients, and percentage bounds once after configuration
precedence is resolved. Preserve documented zero-as-default behavior. Reject invalid values before spawning children;
verify flags, file settings, and environment values through the same path.

## Test-quality findings

| ID / severity | Evidence and effect | Remediation decision and closure check |
| --- | --- | --- |
| T01 / Medium | `internal/diff/parse_test.go:19` uses `&&` and misses independently wrong results/errors. `internal/log/log_test.go:28–40` checks an unconnected buffer. `cmd/gremlins_test.go:38` discards execution errors. R15 adds an unchecked successful assessment. Three controlled mutants survived selected tests. Existing G01/G10 applies. | **Fix.** Check each return independently, observe actual output destinations, and assert successful command/assessment errors are nil. Require the three demonstrated mutants to fail after repair. |
| T02 / Medium | `internal/engine/executor_test.go:130–161` has no running-child timeout/descendant case; `cmd/gremlins/main.go:38–66` has no binary exit/signal test. Fake build exits also mask R04. Existing G07/G11 applies. | **Fix with R04/R11/S02.** Use controlled helper children and tiny packages to verify actual exit codes, timeout status, interruption, and descendant cleanup. Execute platform-specific cases on their supported OS. |
| T03 / Medium | `workerpool_test.go:54–57,87–89`, `executor_test.go:75,116,200`, and `workdir_test.go:173` contain unbounded receives/joins. Dropped work consumes a package timeout with poor context. Existing G08 applies. | **Fix with lifecycle changes.** Bound completion, own cancellation/join cleanup, and verify an intentionally withheld result fails the named operation promptly. |
| T04 / Medium | Global configuration writes lack reliable cleanup in configuration/diff/exclusion/report tests. `engine/stubs_test.go:57–72` ignores fixture errors; `report_test.go:469–471` ignores golden read/decode errors; workdir permission tests assume denied access. Existing G04/G05/G09 applies. | **Fix in affected packages.** Register cleanup before mutation, check setup, and verify permission capability or skip precisely. Use focused independent/shuffled/repeated cases and relevant OS runs. |
| T05 / Low | `flags_test.go:161–179` does not prove value/Viper binding; `coverage/profile_test.go:194–203` only uses one matching file/block; `mutator_test.go:27–63` omits direct `Skipped` mapping. Rendering checks cover the latter indirectly. Existing G03/G06/G12 applies. | **Defer to changes to those contracts.** Add independent binding, absent-file/multiple-block, and finite-status expectations then. No generic test framework is needed. |
| T06 / Low | Several tests emit bare diffs, reversed expected/actual labels, or generic failures. `cmd/unleash_test.go:177` prints the actual type for both fields. Existing G02 applies across runtime and tool tests. | **Defer to edits of the affected tests.** Require operation/case, actual result, expected result, and a correctly labeled diff. Avoid an unrelated diagnostic-only sweep before result-integrity repairs. |

## Tooling and maintenance findings

| ID / severity | Evidence and effect | Remediation decision and closure check |
| --- | --- | --- |
| D01 / Medium | `Makefile:18–22,28–32` gives binary/coverage output targets no prerequisites. With outputs already present, a dry run selects neither recipe; source files cannot invalidate those targets. Local build/coverage can be stale. | **Fix.** Always invoke the requested build/coverage recipe; Go already caches compilation. Verify existing outputs cannot suppress a requested refresh. |
| D02 / Medium | `.pre-commit-config.yaml:2,30–35,48–53,61–66` assigns full three-platform lint, vulnerability analysis, and the full race suite to commit time. The installer enables pre-push, but no hooks select that stage. This conflicts with the documented fast-local/full-hosted split. | **Fix.** Keep quick file/secret checks at commit time and place broad verification in established hosted/publication checks. Verify actual stage selection and activation; do not bypass current hooks during remediation. |
| D03 / Medium | `docs/docs/schema/configuration.json:10` nests `silent` under `unleash`, while the runtime key is top-level. Supported worker/CPU/timeout/integration/diff/filter/exclusion settings and `invert-logical` are absent. | **Fix.** Align the existing public schema with runtime keys, types, defaults, and mutation choices. Add a small independent contract check; avoid a schema-generation framework. |
| D04 / Medium | `.github/ground-control/phase-e.integration.test.mjs:16–38` needs a supplied installed utility. Its invocation is documented in `phase-e.md:67–85`, but no workflow/Make target runs it. Ordinary CI does not exercise transport changes through this harness. | **Defer to the next transport or utility change.** Require the existing sixteen-case harness as change evidence, then wire targeted hosted execution using the approved pinned release. Keep candidate execution separate from private credentials. Existing recorded results are not a fresh run. |
| D05 / Low | `Makefile:37` installs `fieldalignment@latest`, although normal tools and `x/tools` are pinned. Provisioning can change source-rewriting behavior without a repository edit. | **Defer to developer provisioning maintenance.** Pin the reviewed version or use a standard versioned tool mechanism. Verify repeated provisioning resolves the same version. |
| D06 / Low | `docs/mkdocs.yml` still names upstream repository/distribution destinations. The symlinked `docs/docs/CONTRIBUTING.md` exists, but links such as `development/process-review.md` have no corresponding site-docs path. CI usage pages also describe upstream distribution. | **Defer to #82.** Check built-site links and fork-specific installation destinations without publishing. Do not misclassify the existing contributor symlink as a missing navigation target. |

## Security review

The primary reviewer inspected subprocess construction/environment, mutation/workdir boundaries, input paths,
logging and output, static analysis confinement, dependency pins, and credential-bearing automation. Tests execute
with the invoking user's authority. A temporary copy protects original source from ordinary mutation; it is not a
sandbox, resource limit, or restriction on test network/filesystem access.

| ID / severity | Evidence and effect | Remediation decision and closure check |
| --- | --- | --- |
| S01 / High | `internal/engine/executor.go:201–283` applies a time limit but no memory budget to mutant test processes. Fast allocation can exhaust the host before timeout. The original downstream exhaustion is recorded in #1; no OOM experiment was run here. Coverage also executes target tests without such a budget. | **Fix through #1's resource-control remediation.** Design a documented supported resource budget for the child tree using established OS/runner mechanisms, including an explicit failure/result policy. Verify a controlled allocating child within an isolated, bounded environment. Do not reproduce host exhaustion or add an unapproved service. |
| S02 / Medium | `internal/engine/process_windows.go:29–47` creates a process group but kills only the direct process. Descendant tests can outlive timeout/cancellation and keep using files/resources. This is source-established; Windows runtime behavior was not measured here. | **Fix with lifecycle remediation.** Use established Windows process-tree controls, such as [job objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects), through the existing platform boundary and reviewed dependencies. Verify descendant termination on Windows CI, including a deliberately slow child, and preserve Unix group cleanup tests. |

R03, R05, R06, R11, and R17 also affect availability or integrity. They are recorded once above, not counted as
additional security findings. No remote-exploitability claim is made for trusted-project execution.

Retained controls and explicit decisions:

- **Accept trusted target execution as the current boundary.** Coverage downloads modules and executes tests even
  with `--dry-run`; only mutant execution is skipped. Correct the misleading no-tests description in the schema/usage
  review under D03/#82. Do not present dry run as safe inspection of an untrusted checkout.
- **Retain argument-array subprocess calls.** There is no shell interpolation in the Go runtime command construction.
  Runtime Git execution still uses the target repository's Git configuration; this is part of the trusted-project boundary.
- **Retain local static analysis confinement.** Architecture inputs use clean paths, link checks, `os.OpenRoot`, strict
  JSON, and fixed Git commands. Lint analysis strips ambient credentials and disables network/build overrides.
  These checks do not make executing arbitrary project tests safe.
- **Retain credential separation and supply-chain pins.** Workflows pin actions, disable persisted checkout credentials,
  isolate token-bearing Sonar execution, and use a checksum/lockfile with disabled npm lifecycle scripts for the finalizer.
  The fork dispatcher checks no candidate code out. No new service or data destination is needed for remediation.
- **Accept the Gitleaks test-key match as a false positive.** Gitleaks 8.30.1 reported one `generic-api-key` match at
  `internal/configuration/configuration_test.go:268`. Inspection showed a dotted configuration-key name in a type table,
  not a credential. No secret value was printed, and no blanket suppression or fixture rewrite was added.

The [Go vulnerability tooling](https://go.dev/doc/security/vuln/) result below is a dated, platform-specific check,
not a guarantee about every dependency or external automation utility. Process cleanup recommendations also follow
the standard [os/exec contract](https://pkg.go.dev/os/exec#Cmd).

## Architecture, shared concerns, and extensibility decisions

| Concern | Decision and reason |
| --- | --- |
| Package layout and dependency directions | **Retain.** All 18 packages load without a cycle. `cmd` composes runtime components; the documented `engine → report` and `log → configuration → mutator` directions are intentional. No layer rewrite is justified by the findings. |
| Logging and errors | **Retain shared output and ordinary Go errors; repair propagation.** Application output already uses `internal/log`, and thresholds use shared exit errors. Fix swallowed errors and lifecycle ownership before adding a logger framework, custom exception system, or service locator. Keep the isolated parser's standard-library logging exception under ADR 0005. |
| Standard/trusted libraries | **Retain Go AST/parser/printer, `os/exec`, `io/fs`, `io.Copy`, Cobra/Viper, coverage and diff parsers.** Correct their use. Replace the custom first-line module grammar in R13 with supported metadata/parsing; do not replace working libraries with new custom implementations. |
| Pragmatic DRY | **Retain separate architecture/lint tools.** Their language and identity requirements differ; a shared policy framework would add coupling. Reuse module discovery during R13 if the small extraction preserves dependency direction. Minor local repetition does not justify generic utilities. |
| Mutation extensibility | **Retain explicit mappings for eleven types.** R16 and D03 show the need for mapping/schema consistency checks when adding a type. Keep names, defaults, token pairs, reporting, flags, and documentation aligned. A plugin registry or generated system is not warranted. |
| Broad mutator/executor interfaces | **Defer interface splitting until a concrete consumer/second implementation needs it.** R04–R06 require an error path; add that with small local APIs rather than speculative abstractions. |
| Baselines | **Retain shrink-only architecture/lint baselines.** Architecture currently has zero findings. The committed lint baseline has 152 occurrences across 151 identities, including 61 `errcheck` occurrences. Those are debt, not semantic approval. Repair error/resource/result defects before a blanket style sweep. |

An initial possible shared-AST discovery race was investigated and rejected: the inspected walker does not reread the
mutated operator field after that node's discovery. It is not a finding. Any future concurrency change still requires
targeted race evidence. No architecture-model change is required for this documentation-only review; implementation
repairs must follow the existing design check for their actual affected packages and contracts.

## Measured evidence and limits

| Check | Observed result and limit |
| --- | --- |
| `go list -deps -e -json ./cmd/... ./internal/... ./tools/...` | Cached Go 1.26.10, offline: 18 local packages; no dependency errors or cycles. Package loading does not prove behavioral correctness. |
| `go run ./tools/architecture -base HEAD` | Passed against the inspected revision with zero baseline findings. No design or baseline was modified. |
| Targeted race tests | `go test -race ./internal/configuration ./internal/diff ./internal/report ./internal/gomodule -run '^(TestConfiguration|TestNewWithCmd|TestAssessment|TestDetectsModule)$' -count=1` passed with `GOPROXY=off`. These cases do not detect the new boundary defects. |
| Runtime probes using actual package APIs in a temporary local replacement module | Measured R01: `false/0/0` for environment values `true/4/90`; R02: malformed discovered configuration accepted; R03: 7 → 307 descriptors for 150 files with GC disabled; R04: missing executable `LIVED`, real compile failure exit 1; R07: `different` routed to module root; R08: edits 2/6 mapped to 2/3; R13: comment parsed as module name; R14: missing output parent returned nil; R15: 100% failed threshold 100. No checkout source edits. |
| Signal probe | Executed the exact `ctxDoneOnSignal` function in a temporary helper; SIGINT, wait for cancellation, then SIGTERM caused `panic: send on closed channel` (exit 2). Signals targeted only that helper. |
| Focused mutation checks | Temporary `go test -overlay` changes left the checkout untouched. Two independently broken empty-diff returns and one unintended uninitialized-logger output survived their selected tests. Details follow. No aggregate mutation score is claimed. |
| Build/coverage freshness | A Make dry run with existing temporary output files selected neither recipe. No full coverage/test run was executed locally. |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | `No vulnerabilities found.` Linux/amd64, Go 1.26.10, review date above. This static analysis did not execute the test suite or audit the private finalizer's dependencies. |
| `gitleaks dir <temporary tracked-HEAD snapshot> --redact --no-banner` | Gitleaks 8.30.1 scanned approximately 2.89 MB and returned exit 1 for the single reviewed test-key false positive. This is not a clean scanner exit or a history/untracked-file scan. |

The diff mutation command selected only the affected subtest:

```shell
go test -overlay <temporary-overlay.json> ./internal/diff \
  -run '^TestNewWithCmd$/^must_return_nil_on_empty_flag$' -count=1 -timeout=20s
```

Baseline `return nil, nil` passed. Replacing that return independently with `return Diff{}, nil` or
`return nil, fmt.Errorf("unexpected empty-ref error")` also passed. The assertion must reject either wrong field.
For logging, adding `fmt.Printf(f, args...)` to the uninitialized `Infof` branch printed `test` and still passed:

```shell
go test -overlay <temporary-overlay.json> ./internal/log \
  -run '^TestUninitialised$' -count=1 -timeout=20s -v
```

These focused checks used offline cached tools and temporary overlays. They demonstrate three survivors, not a full
mutation run. Full suites, fresh aggregate coverage/lint, cross-platform runtime checks, the private transport harness,
and live hosted gates were not run for this review. No source, evidence, or derived artifacts were sent to a new service.

## Remediation order and closure

1. Restore trustworthy results: R01–R10, R14–R16, and their T01/T02 regression cases. Reuse #1 for its existing items.
2. Repair bounded ownership and availability: R03/R05/R11/R12/R17, S01/S02, and T02–T04. Work may be grouped where the
   same lifecycle change owns the fix, without creating a broad rewrite.
3. Align maintenance contracts: R13, D01–D03, then address deferred T05/T06/D04–D06 at their named change boundaries.

Every finding above has a remediation decision. #83 should map required repairs to bounded issues, preserving these
IDs and existing #1/test-assessment links. Close a finding only with a repair revision, failing-before/passing-after
regression evidence, relevant targeted mutation evidence, and required hosted platform results. Completion of #80
means this record is delivered; it does not mean the repository's recorded defects are repaired.
