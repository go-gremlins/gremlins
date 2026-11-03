# Go coding standard and upstream comparison

Use Google's canonical Go Guide and normative Decisions.
Use Best Practices as advice where the context fits.
The Guide takes precedence when these documents differ.
Review clarity, intent, and documented exceptions in addition to running tools.
Passing lint does not prove full compliance.

`make lint-test` exercises the pinned tool with invalid Go fixtures and permitted alternatives.
The fixtures assert 64 checker predicates, formatter simplification, Windows-only source coverage, and documented exceptions.
They use local stand-ins for blocked assertion packages. They download no test dependency and execute no Go fixture.
Baseline tests cover growth, substitutions, occurrences, stale entries, history, initial adoption, and malformed input.
CI uses Coverage.py 7.16.0 for the Python adapter, with line and branch reporting through the existing Sonar pipeline.
The admission check requires both coverage reports and rejects unexpected files, links, and XML entity declarations.
Use `make lint-cover` when the pinned Coverage.py is installed; `make lint-test` needs only the standard library.

Reference snapshots reviewed for issue #77:

- [Google style sources](https://github.com/google/styleguide/tree/70d70c93c9cc6dd18c66a37ed72de3c7642722bd/go), commit `70d70c93c9cc6dd18c66a37ed72de3c7642722bd`.
- [Upstream sources](https://github.com/go-gremlins/gremlins/tree/b48a4aad17eff33dc7262e0d1487b1a4d6eec322), commit `b48a4aad17eff33dc7262e0d1487b1a4d6eec322`.
- [golangci-lint configuration](https://github.com/golangci/golangci-lint/blob/v2.15.0/.golangci.reference.yml), version 2.15.0 pinned by `mise.toml`.

The fork and upstream lint configurations were identical before this change.
The executable authority is [`.golangci.yml`](../../.golangci.yml).
The [lint design](lint-design.md) describes identities, security controls, and data flow.

## Google rule coverage

Each row names Guide or Decisions sections and the corresponding automatic checks.
The last column names the part that still needs review.
Checker settings apply to production code, tests, and development tools.
Use `make lint` for the authoritative gate, including format checks and all supported platform contexts.

| Source section or rule | Deterministic check | Review boundary |
| --- | --- | --- |
| Guide: formatting | `gofmt` with simplification and `goimports`, checked separately with `fmt --diff`. | None for formatter output. |
| Guide: line length | No golangci length limit. The existing Sonar profile checks 120 characters. | Split lines when readability improves. Google sets no fixed maximum; Sonar is a separate fork quality check. |
| Guide: MixedCaps; Decisions: underscores, constant names, initialisms | Revive `var-naming`; Staticcheck `ST1003`. | Domain abbreviations, generated names, and required external names. |
| Package names | Revive `package-naming` and Staticcheck `ST1003`. | Concision, shadowing risk, and useful package meaning. |
| Receiver names | Revive `receiver-naming`, `recvcheck`, Staticcheck `ST1016`. | Choose a useful short name. |
| Getters; repetition; variable and single-letter names | Revive `exported` detects package stuttering. | Getter semantics, scope, context, and name quality need judgment. |
| Commentary; doc comments; package comments; comment sentences | Revive `exported`, `package-comments`; Staticcheck `ST1000`, `ST1020`–`ST1022`; `godot`; Gocritic `commentFormatting`. | Comment accuracy, purpose, and useful detail. |
| Comment line length | No numeric limit. | Readability and paragraph wrapping. |
| Examples | `testableexamples` and `govet`'s `tests` analyzer. | Example usefulness and API explanation. |
| Named result parameters | `nakedret` flags naked returns in functions longer than 30 lines. | Useful result names and deferred result changes are permitted. Small naked returns require review. |
| Imports: grouping | `goimports` separates standard, third-party, and fork imports. | Additional logical groups are optional. |
| Import renaming | Revive `redundant-import-alias`, `import-alias-naming`; `importas`. | Resolve real naming collisions and use consistent meaningful aliases. |
| Blank imports | Revive `blank-imports`. | Necessary side effects and documentation. |
| Dot imports | Revive `dot-imports`; Staticcheck `ST1001`. | Rare permitted integration cases need a narrow documented exception. |
| Returning errors | Revive `error-return`, `error-naming`, `errorf`; `errname`; `errorlint`. | Which conditions constitute errors and error API design. |
| Error strings | Revive `error-strings`; Staticcheck `ST1005`. | Proper names and messages from external systems. |
| Handle errors | `errcheck` checks discarded errors and type assertions, including blank assignments and normally excluded functions. | Document an intentional ignored error; do not hide it with a baseline refresh. |
| In-band errors | Staticcheck and `nilerr` detect supported incorrect error patterns. | Choosing a separate error result or comma-ok API needs semantic review. |
| Indent error flow | Revive `indent-error-flow`, `superfluous-else`, `if-return`. | Equivalent branch choices and scope-sensitive early returns. |
| Literal formatting: field names | `govet` composites detects unkeyed external struct literals. | Same-package literals are permitted; review whether names improve clarity. |
| Literal formatting: matching and cuddled braces | `gofmt`. | Composite literal layout choices that the formatter accepts. |
| Literal formatting: repeated type names | `gofmt` simplification. | None for supported simplifications. |
| Literal formatting: zero-value fields | No unconditional ban; explicit zero values can document intent. | Review intent rather than stripping all zero values. |
| Nil slices | Revive `enforce-slice-style` with `nil`. | Non-nil empty slices needed for encoding or an API require a narrow reason. |
| Indentation confusion; function formatting | `gofmt` handles syntax layout. | Simplify complex signatures and expressions. |
| Conditionals and loops | Revive `increment-decrement`, `range`, `var-declaration`, `unnecessary-if`; Staticcheck simplification checks. | Choose clear conditions and control flow. |
| Copying | `govet` copylocks and atomic checks; Staticcheck; Revive `waitgroup-by-value`. | Aliasing and whether a copy preserves application semantics. |
| Don't panic; Must functions | Staticcheck checks supported panic/recover defects. | Panic is permitted in specified fatal or invariant cases; an unconditional ban would conflict with Google. |
| Goroutine lifetimes; synchronous functions | `govet` lostcancel, loopclosure, testinggoroutine; `contextcheck`; `bodyclose`; `rowserrcheck`; `sqlclosecheck`. | Shutdown ownership, cancellation, leaks, and synchronous API design. |
| Interfaces; generics | Go type checking and Staticcheck's applicable diagnostics. | Interface ownership, appropriate abstraction, and constraint design. |
| Pass values; receiver type | `govet` copylocks; Gocritic `ptrToRefParam`; `recvcheck`. | Size, mutation, consistency, and pointer semantics. |
| Switch and break | Revive `useless-break`; Gocritic `defaultCaseOrder`, `singleCaseSwitch`. | Intentional fallthrough and clarity. No mandatory default is imposed. |
| Type aliases | Compiler and Staticcheck check misuse. | Aliases are permitted for API migration; review their purpose. |
| Use `%q` | Gocritic `sprintfQuotedString` detects manually quoted `%s`. | Whether other values need quoting depends on their diagnostic meaning. |
| Use `any` | Revive `use-any`. | Compatibility with an explicitly older language contract. |
| Flags | Cobra and the existing configuration bindings remain authoritative. | Flag naming, registration ownership, and scope. Google-internal flag libraries do not apply. |
| Logging | Existing Logger remains authoritative for application output. The isolated parser uses standard-library logging under ADR 0005. `forbidigo` retains upstream's prohibited-print checks. | Log level, useful context, and sensitive values. Google-internal logging libraries do not apply. |
| Contexts; custom contexts | Revive `context-as-argument`, `context-keys-type`; `containedctx`; `contextcheck`; `noctx`. | API lifetime and necessary request-scoped data; no custom context type is categorically forbidden. |
| crypto/rand | `gosec` detects weak randomness in supported security-sensitive operations. | Identify security-sensitive uses; ordinary random sampling can use math/rand. |
| Useful test failures: function/input identification, got before want, full structures, stable results | `govet` printf and tests analyzers check malformed reporting calls. | Failure message semantics and correct comparison order require review. |
| Assertion libraries; testing package | Strict `depguard` allows only approved imports; Revive `imports-blocklist` supplies named-framework diagnostics; `thelper`; `usetesting`. | Keep tests in the standard testing framework; select direct checks and go-cmp where useful. |
| Keep going; equality comparisons, diffs, detail; test error semantics | `errorlint` and `govet` check supported error comparisons and reporting defects. | Decide fatal versus nonfatal failure, useful diffs, and semantic error assertions. |
| Test structure; subtests and names; table-driven and data-driven cases | Compiler and `govet` tests analyzer. | Case independence, row identity, coverage, and clear organization. No forced parallelism. |
| Test helpers | `thelper`. | Helper usefulness and correct ownership. |
| Test package: same or different package | Both are analyzed. | Choose package visibility to fit the behavior under test. |
| Non-decisions | No additional automatic rule. | Follow the Guide's clarity and consistency principles. |

The checks above enforce the predicates their implementations support.
Partial coverage is named explicitly; it does not certify the remaining semantics.
Package-local unkeyed literals remain permitted by Google's rule.
Likewise, source names and test failure messages cannot be judged by string shape alone.
Reviewers must apply these context-dependent requirements.
The declaration parser prevents identical nearby source in separate functions from sharing a baseline identity.
No deterministic check is replaced by an arbitrary complexity or line-length threshold.

Best Practices are advisory: naming and test doubles, error handling, concurrency, configuration, and test design require context.
Use their supported checks from the table where applicable.
Do not treat an advisory example as a blanket ban on an otherwise valid Go construct.

## Decisions against upstream

This table accounts for every inherited enabled linter and each changed setting or invocation.
Unlisted default settings of retained linters stay unchanged.

| Upstream standard or behavior | Decision in this fork | Reason |
| --- | --- | --- |
| `asasalint`, `asciicheck`, `containedctx`, `decorder`, `dogsled`, `durationcheck` | Keep. | Existing correctness and consistency rules remain useful. |
| `errname`, `errorlint`, `exhaustive`, `forbidigo`, `forcetypeassert` | Keep. | Preserve error, switch, output, and assertion safeguards. |
| `goconst`, `godot`, `gosec`, `gocritic`, `gocyclo` | Keep; extend Gocritic with `ptrToRefParam` and `sprintfQuotedString`. | Add pointer-to-interface and quoted-format checks. Complexity thresholds are fork preferences, not Google rules. |
| `importas`, `maintidx`, `makezero`, `nilerr`, `nlreturn` | Keep. | Preserve import consistency, metrics, allocation, error, and whitespace checks. |
| `nolintlint` | Adapt: require checker names and explanations; reject unused directives. | Suppressions need a narrow, visible reason. Review still judges the reason. |
| `nonamedreturns` | Drop. Use `nakedret` with 30 lines. | Google permits useful named results, especially for deferred updates. |
| `predeclared`, `unconvert`, `usestdlibvars` | Keep. | Preserve naming and simplification checks. |
| `tagliatelle` with JSON snake case | Keep unchanged. | Runtime JSON compatibility is a separate contract. |
| Revive's 25 explicit rules and confidence 0.8 | Keep rules; lower confidence to zero; extend with documented Google predicates. | Do not silently discard lower-confidence deterministic findings. |
| Revive additions | `package-naming`, `redundant-import-alias`, `import-alias-naming`, `enforce-slice-style` (`nil`), `use-any`, `useless-break`, `unnecessary-if`, `unnecessary-format`, `waitgroup-by-value`, `imports-blocklist`. | Cover the table above. Block testify, gomega, and goconvey assertion packages; depguard also rejects all unapproved external imports. |
| Revive exported comments | Check public interfaces and private receivers too. | Exported methods still need meaningful documentation. |
| Staticcheck default style exclusions | Enable `all`. | Include package, naming, receiver, and documentation checks normally disabled by default. |
| Missing standard correctness checks | Add `errcheck`, `govet`, `ineffassign`, `unused`, `recvcheck`, `nakedret`. | Complete the available error, copy, naming, and return checks. |
| Third-party test libraries | Add strict `depguard`, with standard/fork imports and exact existing runtime/go-cmp packages allowed. | Reject every unapproved assertion or test framework, including names absent from a blocklist. New dependencies require policy review. |
| Resource and context checks | Add `bodyclose`, `contextcheck`, `noctx`, `rowserrcheck`, `sqlclosecheck`. | Detect supported resource and lifetime mistakes. |
| Test and source directives | Add `testableexamples`, `thelper`, `usetesting`, `gocheckcompilerdirectives`, `bidichk`. | Check examples, helpers, standard testing APIs, directives, and hidden bidi controls. |
| Test exclusion for disabled `cyclop` | Drop. | It has no effect and does not exempt the enabled `gocyclo`. Tests remain in scope. |
| Implicit exclusion behavior | Explicit strict generated markers and fixture/vendor/output paths; no presets. | Keep scope visible. Do not change fixture bytes. |
| `goimports` formatter only | Add simplifying `gofmt`; keep `goimports` with the fork local prefix. | Enforce formatting separately and group local imports consistently. |
| Implicit issue caps and per-line collapse | Disable limits and collapse. | A baseline needs the complete findings. |
| Linux analysis only | Analyze Linux, Darwin, and Windows with tests, on amd64 without cgo or custom tags. | Cover the repository's supported platform files in valid contexts. |
| Latest linter action and legacy latest installer | Use the mise v2.15.0 pin throughout. | Reproducible configuration and finding identities. |
| Direct lint failure on all existing findings | Add a reviewed identity-and-count baseline with an immutable target comparison. | Admit current debt once and reject all additions or replacements. |
| Existing justified suppressions | Keep; require explicit names and reasons. | No new broad suppressions to obtain a passing baseline. |
| Formatter and alignment convenience commands | Use the pinned formatter; keep fieldalignment as an explicit optional rewrite. | Alignment is not a Google compliance check. |
| Upstream contribution and release process | Keep the fork decisions in [process review](process-review.md). | This record changes coding standards, not branch or release policy. |

## Working with lint debt

Provision the pinned tool and Go modules separately before analysis:

```shell
mise install aqua:golangci/golangci-lint
go mod download
make lint LINT_BASE=origin/dev
```

Fix a finding, then remove only the stale allowance with:

```shell
make lint-shrink LINT_BASE=origin/dev
```

The shrink command refuses new identities and higher counts before writing.
It cannot create a new baseline or refresh changed fingerprints.
Normal checks are read-only.
The initial baseline records 152 findings in 151 identities with Go 1.26.10 and golangci-lint 2.15.0.
The existing Go source and dependency inputs match comparison commit `bb2a9b44a8237ce0db2c6d78cafa030b8fd71b60`.
Do not edit allowances, add generated markers, weaken rules, or add suppressions to conceal new debt.
Policy changes need explicit review and updated coverage documentation.
See the design for first-adoption restrictions and comparison failures.
