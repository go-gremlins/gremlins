# Implementation Plan: Expression-Level Mutations (Issue #145)

## Goal
Implement `!` → `!!` mutation and build foundation for future expression-level mutations, while fixing the SUB token context issue.

## Final Architecture
The final result will be a **Clean Architecture** with:
- Registry pattern for mutation discovery
- Separation of concerns (discovery, mutation, expression, token packages)
- Centralized file locking via FileMutation wrapper
- Factory pattern for mutator creation
- No switch statements in mutation logic
- Full support for both token and expression mutations

## Implementation Strategy
Pragmatic incremental approach to reach clean architecture faster with lower risk.

---

## Stage 1: Add Node Context Awareness
**Goal**: Fix SUB token ambiguity (UnaryExpr vs BinaryExpr)
**Status**: In Progress

### Success Criteria
- [ ] NodeToken stores original AST node for context
- [ ] GetMutantTypesForToken() filters by node type
- [ ] SUB in UnaryExpr only generates InvertNegatives
- [ ] SUB in BinaryExpr only generates ArithmeticBase
- [ ] All existing tests pass
- [ ] No new linter warnings

### Tests
- TestGetMutantTypesForToken_SUB_UnaryExpr
- TestGetMutantTypesForToken_SUB_BinaryExpr
- TestMutations_SUB_Unary_OnlyInvertNegatives
- TestMutations_SUB_Binary_OnlyArithmeticBase

---

## Stage 2: Create Expression Mutation Infrastructure
**Goal**: Build foundation for expression-level mutations (parallel to token system)
**Status**: Not Started

### Success Criteria
- [ ] NodeExpr struct for expression mutation points
- [ ] ExprMutator implementation (initially copy-paste from TokenMutator)
- [ ] Expression discovery integrated into engine
- [ ] Expression mutations stream through same worker pool
- [ ] Dry-run mode works for expression mutations
- [ ] File locking works for both token and expression mutations

### Tests
- TestNewExprNode_UnaryExpr
- TestExprMutatorApplyAndRollback
- TestExpressionDiscovery_ParallelPath

---

## Stage 3: Implement ! → !! Mutation
**Goal**: Implement first expression-level mutation using AST reconstruction
**Status**: Not Started

### Success Criteria
- [ ] InvertLogicalNot mutation type added
- [ ] UnaryExpr with NOT operator detected
- [ ] AST reconstruction creates !!x from !x
- [ ] Parent-child relationship handling works
- [ ] Mutation appears in results
- [ ] Tests catch the mutation (if test coverage exists)
- [ ] Comprehensive test coverage for various contexts

### Tests
- TestInvertLogicalNot_IfCondition
- TestInvertLogicalNot_Return
- TestInvertLogicalNot_Assignment
- TestInvertLogicalNot_BinaryExpr (e.g., `a && !b`)
- TestInvertLogicalNot_Nested (e.g., `!(!x)`)
- TestInvertLogicalNot_FunctionCall (e.g., `!isValid()`)

---

## Stage 4: Refactor to Clean Architecture
**Goal**: Eliminate technical debt, achieve final clean architecture
**Status**: Not Started

### Success Criteria
- [ ] FileOperations extracted (shared by TokenMutator and ExprMutator)
- [ ] File locking centralized in FileMutation wrapper
- [ ] Discovery registry pattern implemented
- [ ] MutationSpec interface for all mutations
- [ ] Factory pattern for mutator creation
- [ ] No code duplication between mutators
- [ ] All token mutations migrated to specs
- [ ] All tests pass
- [ ] No performance regression
- [ ] Code coverage maintained

### Tests
- TestRegistry_Registration
- TestRegistry_Discovery_TokenMutations
- TestRegistry_Discovery_ExpressionMutations
- TestFileMutation_ConcurrentLocking
- TestFactory_TokenMutatorCreation
- TestFactory_ExpressionMutatorCreation

---

## Notes
- Each stage is independently committable
- All tests must pass before moving to next stage
- Run `make test` and `make lint` after each change
- Follow existing code patterns until Stage 4 refactoring
