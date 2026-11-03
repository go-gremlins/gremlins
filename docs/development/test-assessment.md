# Go test assessment

Assessment date: 2026-11-03. Issue: [#78](https://github.com/OpenRAE/go-rae-gremlins/issues/78).
Inspected revision: `67008e1c54c758f0bd300ea26a21e3a0f4e2cde8`.
Requirements: [Go test standard](test-standard.md), T01–T10, introduced by this change.

## Method and limits

Compared the [architecture model](../../.architecture/model.json) with Go source directories in the checkout.
The inventory contains 18 packages, 29 test files, and 102 top-level `Test` functions.
Counts include subprocess helper entry points; they do not count subtests or measure test quality.
Reviewed test structure, representative assertions and helpers in each tested package, platform files, and existing check configuration.
Source search identified cleanup, subprocess, configuration, concurrency, and failure-reporting sites for closer review.
Fixture modules under `testdata/`, dependencies, generated outputs, and Python tests are excluded from these counts.

This is a source assessment, not a complete proof of conformance.
Each gap below is a concrete observation; absence of a reported gap is not a package pass.
No full test, coverage, mutation, cross-platform, or shuffle run was performed for this record.
Current percentages, race results, mutation survivors, and order independence are therefore unverified.
Configured checks are described separately from measured results.
The inventory and source observations can be reproduced from the named revision without running its code.

## Package inventory

Paths below are relative to the repository root. Gap IDs refer to the detailed record below.
Each package was considered against T01–T10; runtime measurements remain unverified for every package.

| Package | Test files / top-level tests | Source evidence and assessment |
| --- | --- | --- |
| `cmd` | 2 / 3 | `gremlins_test.go`: `TestExecute`; `unleash_test.go`: `TestUnleash`. Root/version and flag cases exist. G01, G02. |
| `cmd/gremlins` | 0 / 0 | `main.go` owns exit mapping, signal cancellation, and version decoration. `cmd.TestExecute` and `execution.TestExitErr` cover parts indirectly. G11. |
| `cmd/internal/flags` | 1 / 1 | `flags_test.go`: `TestSet` covers supported flag types and an unsupported type. `checkFlag` verifies presence and shorthand only. G03. |
| `internal/configuration` | 2 / 6 | `TestConfiguration`, `TestConfigPaths`, and `TestMutantDefaultStatus` cover file/env loading, discovery, and defaults. G04, G05. |
| `internal/coverage` | 2 / 7 | `TestCoverageRunFails` covers child failures; `TestCoverageParsesOutput` covers profile paths. Two tests are child entry points. G02, G06. |
| `internal/diff` | 2 / 4 | `TestDiff_IsChanged` covers line limits; `TestNewWithCmd` uses a local command double. G01, G04. |
| `internal/engine` | 7 / 20 | Mutation discovery, apply/rollback, snippets, arguments, and pre-cancelled context have tests. Three functions are child entry points. G02, G05, G07, G08. |
| `internal/engine/workdir` | 1 / 4 | Copy/cache/error tests use temporary directories and Windows ACL branches. G08, G09. |
| `internal/engine/workerpool` | 1 / 2 | `TestWorker` and `TestPool` check execution and worker counts. Configuration resets are present. G08. |
| `internal/exclusion` | 1 / 1 | `TestRules_IsFileExcluded` covers valid, invalid, and empty rules. G04. |
| `internal/execution` | 1 / 1 | `TestExitErr` checks both supported threshold error codes and messages. G02 applies to diagnostics; process exit behavior is G11. |
| `internal/gomodule` | 1 / 1 | `TestDetectsModule` covers detection, invalid/missing module, and empty path. G09. |
| `internal/log` | 1 / 4 | Info/error/silent tests use buffers and Logger resets. G02, G10. |
| `internal/mutator` | 1 / 2 | `TestTypeString` checks all 11 supported type mappings. `TestStatusString` checks six of seven statuses. G02, G12. |
| `internal/report` | 2 / 11 | Summary, assessment, JSON, status filtering, and diff tests exist. G02, G04, G09. |
| `internal/report/internal` | 0 / 0 | Data-only JSON structures are compared indirectly by `report.TestReportToFile` against `testdata/normal_output.json`. A local test file is not required by T01. Fixture setup weakness is G09. |
| `tools/architecture` | 3 / 29 | Boundary, baseline, strict JSON, Git history, design, path escape, and diagnostic-leakage cases exist. G02. Capability-dependent skips exist; their successful execution is unverified. |
| `tools/lintcontext` | 1 / 6 | Declaration scopes, malformed input, documentation ranges, and writer failure have cases. `TestRunRejectsBadInputAndWriteFailure` does not identify the failing input. G02. No other gap established by source review. |

## Gap record

These are open findings at the inspected revision, not approved exceptions or a suppression baseline.
Closure checks describe evidence needed for later repairs; they were not executed here.

| ID / requirement | Location and observation | Consequence and closure check |
| --- | --- | --- |
| G01 / [T03](test-standard.md#t03-make-assertions-detect-the-defect) | `internal/diff/parse_test.go`, `TestNewWithCmd/must return nil on empty flag`: `d != nil && err != nil` rejects only two simultaneous failures. `cmd/gremlins_test.go`, `TestGremlins` discards `c.execute()`'s error. | One invalid result or a command failure can escape the intended check. Verify each contract result independently; demonstrate failure when either result is wrong. |
| G02 / [T04](test-standard.md#t04-make-failures-useful) | Examples include bare `cmp.Diff` in coverage, mutator, report, and token-mutator tests; want-before-got messages in execution, node, and worker tests; generic errors in command, flags, exclusion, and gomodule tests. `tools/architecture/main_test.go`, `TestInvalidModels` reports only a case index. `tools/lintcontext/main_test.go`, `TestRunRejectsBadInputAndWriteFailure` reports no failing input. | Failures need source inspection to identify the actual error, comparison direction, or input. Check the diagnostic category across these files; each repaired assertion must identify its operation/case and actual versus expected result. These examples are not an exhaustive count. |
| G03 / [T01](test-standard.md#t01-test-observable-contracts), [T03](test-standard.md#t03-make-assertions-detect-the-defect) | `cmd/internal/flags/flags_test.go`, `checkFlag`: the successful rows check presence and shorthand, but not the registered value/type or Viper binding. | The table's default values and configuration keys are not verified at the binding boundary. Test a changed flag value through the configured Viper key, for ordinary and persistent flags. |
| G04 / [T06](test-standard.md#t06-isolate-shared-state) | `internal/diff/parse_test.go`, `TestNewWithCmd`, and `internal/exclusion/rules_test.go`, `TestRules_IsFileExcluded`, set global configuration without restoration. `internal/report/logger_test.go`, `TestLogger` resets Logger but leaves its configuration writes. `internal/configuration/configuration_test.go`, `TestConfiguration` resets Viper only at the successful end of a row. | State can survive a test or early failure. Register cleanup before mutation, establish inputs per case, and verify selected tests independently and with shuffled/repeated order. Source review establishes missing cleanup, not an observed flaky run. |
| G05 / [T05](test-standard.md#t05-own-fixtures-and-resources), [T06](test-standard.md#t06-isolate-shared-state) | `internal/configuration/configuration_test.go`, `TestConfigPaths`, ignores cwd lookup/change/restore errors. `internal/engine/stubs_test.go`, `loadFixture`, ignores open/read errors. `engine_test.go`, `TestSkipTestAndNonGoFiles` and `TestSkipNotDiffMutants`, ignore fixture reads and do not close their opened file. | Incorrect setup can test a different directory or empty input and leak resources. Verify setup failure is reported at the calling test and cleanup runs after early failure. Preserve fixture bytes. |
| G06 / [T02](test-standard.md#t02-select-meaningful-cases) | `internal/coverage/profile_test.go`, `TestIsCovered`, always constructs one matching filename with one block. It has no nil/empty profile, absent filename, or multiple-block case. | Missing-file and block traversal behavior lack direct cases. Add independent expected results for those inputs and verify that skipping a later matching block is detected. |
| G07 / [T02](test-standard.md#t02-select-meaningful-cases), [T08](test-standard.md#t08-verify-process-and-platform-boundaries) | `internal/engine/executor_test.go`, `TestMutatorTestExecution`, selects success, test failure, build failure, and uncovered cases. No test drives a timed-out child or descendant cleanup. Neither platform process helper has a direct test. `TestStopsOnCancel` only starts with an already cancelled context. | Timeout arguments and pre-cancellation do not prove running-child cancellation or cleanup. Verify status and bounded termination of controlled children in supported Unix and Windows contexts. Do not assume identical descendant behavior across platforms. |
| G08 / [T07](test-standard.md#t07-bound-concurrent-tests) | `workerpool_test.go`, `TestWorker`/`TestPool`, wait directly on channels. `executor_test.go` uses channel receives and `wg.Wait()` without a test-specific failure bound. `workdir_test.go`, `TestCachesFolder/it works in parallel`, waits on a group without a bound. | A missing result can consume the package timeout with little case context. Add bounded completion and failure cleanup; verify a deliberately missing result reports the operation and leaves no worker behind. |
| G09 / [T05](test-standard.md#t05-own-fixtures-and-resources), [T08](test-standard.md#t08-verify-process-and-platform-boundaries) | `internal/report/report_test.go`, `TestReportToFile`, ignores the golden read/decode errors. `notWriteableDir` ignores temporary-directory and permission errors. `internal/gomodule/gomodule_test.go`, `TestDetectsModule`, ignores `MkdirAll`'s error. `workdir_test.go`, `TestErrors`, assumes mode/ACL changes make access fail without checking host privilege. | A setup problem can appear as a product failure or invalidate a negative case. Check setup and decode errors; verify denied access or explicitly skip unsupported privilege/capability contexts. Then execute the selected permission cases on each relevant OS. |
| G10 / [T03](test-standard.md#t03-make-assertions-detect-the-defect) | `internal/log/log_test.go`, `TestUninitialised`, checks a buffer never passed to Logger. | The empty-buffer assertion does not observe output routing. Verify the uninitialized behavior through observable destinations or a controlled process, and show that unintended output is detected. |
| G11 / [T01](test-standard.md#t01-test-observable-contracts), [T08](test-standard.md#t08-verify-process-and-platform-boundaries) | `cmd/gremlins/main.go` has no adjacent test. `cmd.TestExecute` checks returned errors, and `execution.TestExitErr` checks error values. No Go test invokes the binary to verify its exit mapping or signal handling. | These indirect tests do not prove actual process exit or cancellation. Add controlled process evidence for normal/error/threshold exits and supported termination signals without inheriting private host configuration. |
| G12 / [T02](test-standard.md#t02-select-meaningful-cases) | `internal/mutator/mutator_test.go`, `TestStatusString`, omits `Skipped`. `TestTypeString` already covers all 11 supported mutation types. | The direct status mapping test covers six of seven supported statuses. Add an independent expectation that `Skipped.String()` returns `SKIPPED`, retaining the existing complete type mapping cases. |

## Existing execution and evidence

| Owner | Source observation | Limit of the evidence |
| --- | --- | --- |
| [Makefile](../../Makefile) and [CI](../../.github/workflows/ci.yml) | `make cover` uses race detection and atomic coverage on Linux, macOS, and Windows. Linux uploads the Go coverage profile. | Configuration does not establish current passing results or package percentages. The file target can reuse an existing profile locally. |
| [Lint policy](coding-standards.md) | Test files are analyzed, with helper, testing API, error, and approved-import checks. Existing debt uses an identity baseline. | Lint does not establish semantic conformance to T01–T10. |
| [Mutation workflow](../../.github/workflows/gremlins.yml) | Runs the candidate Gremlins source using [.gremlins.yaml](../../.gremlins.yaml). | No fresh mutation result was reviewed. This workflow is separate from `PR Gate`; its existence is not a passing result. |
| [Executor](../../internal/engine/executor.go) and [coverage runner](../../internal/coverage/coverage.go) | Construct nested Go test commands. | The outer race flag is not inherited automatically by these commands. Their scope differs from the repository's outer tests. |
| [Coverage admission](../../scripts/check-coverage.sh) | Existing CI validates the Go/Python reports before the separate Sonar scan. | Valid report shape does not prove freshness, assertion quality, or test isolation. |

T09's targeted-local/full-hosted split is documented and configured.
T10's current runtime measurements remain unknown in this source-only assessment; no numeric gap or conformance score is claimed.
There is no blanket requirement for fuzzing, benchmarks, parallel tests, or a local test file in each package.
Those absences alone are not findings.

## Updating the record

Keep this revision as the reference for the findings above.
When a gap is repaired, record the new revision, affected requirement, test name, command or hosted run, result, and remaining limits.
Refresh the package inventory when packages change.
Do not turn an open gap into an exception or remove it solely because an aggregate check passes.
Issue #78 delivers the standard and this record; it does not claim that all test debt has been repaired.
