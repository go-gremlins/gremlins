This file provides guidance to Claude Code (claude.ai/code) when
working with code in this repository.

## Project Overview

Gremlins is a mutation testing tool for Go that validates test
effectiveness by introducing mutations to source code and verifying if
tests catch them. It works best on small to medium-sized Go modules
(microservices).

## Essential Commands

### Build & Test

```bash
# Build the binary
make build                    # Creates dist/bin/gremlins

# Run all tests with race detector
make test                     # go test -race ./...

# Run specific package tests
go test -race ./internal/engine/...

# Run single test
go test -race ./internal/engine/ -run TestEngineFindMutations

# Generate coverage report
make cover                    # Creates coverage.out
```

### Linting & Formatting

```bash
# Run linter (golangci-lint)
make lint

# Format imports (with local package priority)
make goimports

# Fix struct field alignment
make fieldalignment
```

### Running Gremlins

```bash
# Basic mutation testing
gremlins unleash              # Or: gremlins run, gremlins r

# Dry-run mode (find mutations without testing)
gremlins unleash --dry-run    # Or: -d

# Configuration file
# Uses .gremlins.yaml in project root
```

### Development Workflow

```bash
# Install required tools
make requirements

# Full development cycle
make all                      # lint + test + build

# Create snapshot release
make snap                     # lint + test + snapshot

# Clean build artifacts
make clean
```

## Architecture

### Core Components

**Engine (`internal/engine/`)**

- Main orchestrator for mutation testing
- Walks AST to find mutation opportunities
- Coordinates mutant execution via worker pool
- Entry point: `Engine.Run(ctx)` traverses files, finds mutants,
  executes tests

**Mutator (`internal/mutator/`)**

- Defines mutation types and their interface
- Types: `ArithmeticBase`, `ConditionalsBoundary`,
  `ConditionalsNegation`, `IncrementDecrement`, `InvertAssignments`,
  `InvertBitwise`, `InvertBitwiseAssignments`, `InvertLogical`,
  `InvertLoopCtrl`, `InvertNegatives`, `RemoveSelfAssignments`
- Status values: `NotCovered`, `Runnable`, `Skipped`, `Lived`,
  `Killed`, `NotViable`, `TimedOut`
- Each mutator implements: `Apply()` (mutate code), `Rollback()`
  (restore original)

**Coverage (`internal/coverage/`)**

- Determines which mutations are covered by tests
- Only covered mutations are tested (uncovered mutations can't be
  caught)

**Executor (`internal/engine/executor.go`)**

- Applies single mutation, runs tests, captures results
- Handles test timeouts and viability checks
- Works with temporary directories to isolate mutations

**Report (`internal/report/`)**

- Formats and outputs mutation testing results
- Supports console output and machine-readable formats
- Calculates efficacy and mutation coverage metrics

**Configuration (`internal/configuration/`)**

- Manages settings via flags, config file (.gremlins.yaml), and
  environment
- Uses Viper for configuration handling
- Controls which mutation types are enabled

### Key Flows

**Mutation Testing Flow:**

1. `cmd/unleash.go` → `run()` initializes components
2. Coverage analysis determines testable code
3. `Engine.Run()` walks file tree, parses Go AST
4. For each token, check `TokenMutantType` mapping for applicable
   mutations
5. Create mutators with status (`Runnable`, `NotCovered`, `Skipped`)
6. Worker pool processes runnable mutants:
   - Copy code to temp workdir
   - Apply mutation via `Mutator.Apply()`
   - Run tests
   - Set status: `Killed` (tests failed), `Lived` (tests passed),
     `TimedOut`, `NotViable` (build failed)
   - Rollback mutation
7. Report results with efficacy/coverage metrics

**Adding New Mutation Type:**

1. Add new constant to `mutator.Type` in `internal/mutator/mutator.go`
2. Update `Types` slice and `String()` method
3. Add token mappings in `internal/engine/mappings.go`
   (`TokenMutantType`)
4. Implement mutation logic in `internal/engine/tokenmutator.go`
   (`ApplyMutation`)
5. Add configuration key in `internal/configuration/`
6. Update flag registration in `cmd/unleash.go`

### Important Patterns

- **Token-based mutations**: Uses Go's AST and token positions to
  identify and apply mutations
- **Workdir isolation**: Each mutation runs in temporary directory
  copy to avoid interference
- **Worker pool**: Concurrent mutation execution with configurable
  parallelism
- **Configuration cascade**: Flags override config file, which
  overrides defaults
- **Context cancellation**: Graceful shutdown on SIGINT/SIGTERM via
  `ctxDoneOnSignal()`

## Testing Conventions

- All test files use `_test.go` suffix
- Tests use table-driven approach where applicable
- Race detector always enabled: `-race` flag
- Test names follow pattern: `Test<ComponentName><Behavior>`
- Use `testdata/` directories for test fixtures

## Module Structure

```text
cmd/                    - CLI commands (main.go, unleash.go, cobra setup)
internal/
  configuration/        - Config handling (Viper-based)
  coverage/            - Test coverage analysis
  diff/                - Git diff integration for incremental testing
  engine/              - Core mutation engine and AST traversal
  exclusion/           - File/pattern exclusion rules
  execution/           - Test execution primitives
  gomodule/            - Go module detection and info
  log/                 - Logging utilities
  mutator/             - Mutation definitions and interface
  report/              - Result formatting and output
```

## Quality Standards

- Go 1.21+ required
- Comprehensive linter configuration (`.golangci.yml`) with 30+
  enabled rules
- Test coverage tracked (Codecov integration)
- All commits must pass: lint, tests, build
- Mutation testing runs on itself (`.gremlins.yaml` configured)
