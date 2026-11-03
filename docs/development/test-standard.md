# Go test standard

This is the test standard for all Go packages under `cmd/`, `internal/`, and `tools/`.
It includes platform files, test helpers, and tests in the same or an external package.
Nested fixture modules under `testdata/` are test inputs, not additional application packages.
Python tooling retains its existing test policy.

Apply this standard with the [Go coding standard](coding-standards.md).
That document owns style, approved imports, and lint exceptions.
This document owns test behavior and evidence requirements.
The [assessment](test-assessment.md) records current gaps; it grants no exceptions.
Apply the requirements to new and changed tests. Record gaps found in existing tests against the same requirements.
Requirement IDs remain stable when the wording changes.

## T01: Test observable contracts

Test behavior that a caller or operator depends on.
Use expected results derived from the contract, not copied implementation logic.
A mock call alone does not prove the resulting behavior.
Check command arguments when those arguments are the boundary contract.
Use existing local doubles and injection points for dependencies.
Use real filesystem or subprocess tests when the actual boundary behavior matters.
Both same-package and external-package tests are permitted.
Do not expose production internals solely to satisfy a test layout rule.

Packages without local tests need a named indirect test or a specific explanation of why none is needed.
Data-only structures can be verified through their consumers.
Executable entry points still need evidence for their own exit and signal behavior.

## T02: Select meaningful cases

Cover normal behavior, relevant limits, empty input, invalid input, and failures where applicable.
Check each supported variant in a finite contract, such as mutation types or statuses.
For a bug fix, reproduce the defect with a failing regression test before changing the behavior.
Then verify that the test passes with the fix.
For new behavior, observe the relevant assertion fail before implementation.
Document a specific reason when a case cannot apply.

Use named table rows when cases share setup and checks.
Use separate tests when cases need different setup or behavior.
Tests must not depend on table order, package test order, or another test's side effects.
Fuzzing and benchmarks are useful for suitable input spaces or performance claims; neither is mandatory for every package.

## T03: Make assertions detect the defect

Check both error presence and returned values when the contract requires them.
Reject each independently invalid result; a conjunction can accidentally accept one wrong field.
Use `errors.Is` or `errors.As` for semantic errors where wrapping is permitted.
Check exact error text only when the text is part of the contract.
Use direct comparisons or the existing `go-cmp` dependency for structured results.
Do not introduce assertion frameworks or generic assertion helpers.

Use `t.Fatal` when a failed prerequisite makes later assertions invalid.
Use `t.Error` for independent checks that can continue.
Report failures through the test's own goroutine when using fatal methods.
Send worker results back for assertion instead of calling `t.Fatal` in a worker.

## T04: Make failures useful

Identify the operation, relevant input or named case, actual result, and expected result.
Put the actual result before the expected result in ordinary failure messages.
For a diff, use a clear legend that matches the comparison order, such as `(-want +got)`.
Do not print an unexplained diff or hide the actual error behind “unexpected error”.
Do not include secrets or raw host environment data in diagnostics.
Name tests for the component and behavior. Preserve established names when they remain clear.
Call `t.Helper` in setup and cleanup helpers that receive a testing object.

## T05: Own fixtures and resources

Keep fixed test inputs in `testdata/` and preserve their exact bytes unless the fixture itself changes.
Create writable inputs and outputs with `t.TempDir`.
Check setup, fixture reads, decoding, permission changes, and required cleanup errors.
Register cleanup as soon as the resource exists, including file handles and child processes.
Use `t.Cleanup` when resource lifetime includes subtests.
An intentional ignored cleanup error needs a narrow explanation; setup errors must not become zero-value inputs.
Golden output checks must compare the public contract, including ordering only when ordering is promised.

## T06: Isolate shared state

Set and restore all configuration, Logger, environment, and working-directory state used by a test.
Register restoration before the operation can fail.
Prefer `t.Setenv`, `t.Chdir`, and cleanup callbacks to unchecked manual restoration.
Reset or restore Viper through the existing package APIs, with ownership across the complete test operation.
Individual synchronized configuration calls do not isolate a sequence of calls between tests.
Tests that change process state must remain serial unless complete isolation is demonstrated.
Use `t.Parallel` only for independent cases; parallelism is not a compliance requirement.
Avoid dependence on user configuration or inherited `GREMLINS_*` values.

## T07: Bound concurrent tests

Coordinate with channels, contexts, or explicit completion signals instead of sleeps that assume scheduling speed.
Bound waits so a missing result reports the failing operation before the package timeout.
Stop workers, cancel contexts, and join goroutines on success and failure.
Test cancellation and shutdown behavior where the package owns them.
Race-detector success covers only the paths and schedules executed by that run.
It does not prove test isolation or absence of all races.

## T08: Verify process and platform boundaries

Use synthetic inputs, explicit arguments, controlled child environments, and temporary working directories.
Use the existing injection seams for unit tests of commands and their failures.
Select subprocess helper tests narrowly and distinguish those entry points from assertion-bearing tests.
Test timeouts, cancellation, and process cleanup when these are promised by the component.
Verify Unix and Windows behavior in their supported contexts; compilation alone does not prove cleanup behavior.
For permission tests, verify the setup and state the privilege and platform assumptions.
Use a specific skip reason when the required capability is absent. A skipped test supplies no success evidence.
Temporary module copies protect source files; they do not sandbox executed code, network access, or inherited credentials.

## T09: Use the existing verification paths

Run the narrowest relevant local tests with the race detector.
For example, select the changed behavior:

```shell
go test -race ./internal/coverage -run '^TestCoverageParsesOutput$'
```

Use a targeted repeat or shuffle run when investigating order or concurrency behavior.
CI owns full test runs. Do not repeat full suites as local completion checks.
The [contributor guide](../CONTRIBUTING.md) owns publication and review procedures.
The [Makefile](../../Makefile), [hooks](../../.pre-commit-config.yaml), and
[CI workflow](../../.github/workflows/ci.yml) own the executable commands and platform matrix.
Their configured publication gates still apply.
For prose-only changes, check accuracy, links, and the diff; do not add tests that merely repeat the prose.

## T10: Keep measurement evidence honest

Use statement coverage to locate unexecuted code, not to certify assertion quality.
Use mutation testing where practical to assess whether tests detect behavioral changes.
Inspect surviving and uncovered mutants; record the behavior gap or a concrete explanation for equivalent behavior.
Do not classify build failures or timeouts as proof that the intended assertion caught a defect.
The [mutation workflow](../../.github/workflows/gremlins.yml) and [.gremlins.yaml](../../.gremlins.yaml)
own current execution and thresholds. This standard adds no threshold or changed-line gate.

Record the revision, command or hosted run, scope, platform, result, and limitations for measured claims.
Use a fresh coverage profile; `make cover` can reuse an existing `coverage.out` file target.
An unrun, skipped, or configured check is not a passed check.
The outer race run does not imply that nested mutant test commands enable `-race`.
Keep statement coverage, mutant coverage, efficacy, regression proof, and lint findings distinct.

## Enforcement and review

| Requirements | Existing automatic checks | Review still required |
| --- | --- | --- |
| T01–T03 | Go compiler, `govet`, and applicable error checks in lint. | Contract coverage, independent expectations, and assertion strength. |
| T04–T06 | Formatting, `thelper`, `usetesting`, error checks, and approved imports in lint. | Useful messages, complete resource ownership, and isolation. |
| T07–T08 | Hosted race tests on supported operating systems. | Bounded waits, cancellation coverage, child cleanup, and justified skips. |
| T09–T10 | Existing publication hooks, hosted tests, coverage admission, and mutation workflow. | Relevant scope, fresh evidence, and interpretation of results. |

Lint baselines admit existing lint debt; they do not certify these semantic requirements.
Passing checks or source review alone does not establish full conformance.
Record an exception with its requirement, reason, affected behavior, and maintainer decision.
An assessment gap is not an approved exception.

## References

- [Google Go Decisions](https://google.github.io/styleguide/go/decisions), Tests: the normative source adopted by the coding standard.
- [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices), Testing: advice on behavior and test doubles.
- [Go testing source](https://github.com/golang/go/blob/go1.26.10/src/testing/testing.go), Go 1.26.10: subtests, failures, cleanup, environment, and working-directory APIs.
- [Go race detector](https://go.dev/doc/articles/race_detector), Usage and execution limits.

The [design note](test-standard-preflight.md) records the reference review and security boundaries for issue #78.
