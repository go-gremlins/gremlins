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

## Stage 1: Add Node Context Awareness ✅
**Goal**: Fix SUB token ambiguity (UnaryExpr vs BinaryExpr)
**Status**: Complete

### Success Criteria
- [x] NodeToken stores original AST node for context
- [x] GetMutantTypesForToken() filters by node type
- [x] SUB in UnaryExpr only generates InvertNegatives
- [x] SUB in BinaryExpr only generates ArithmeticBase
- [x] All existing tests pass
- [x] No new linter warnings

### Tests
- TestGetMutantTypesForToken_SUB_UnaryExpr ✅
- TestGetMutantTypesForToken_SUB_BinaryExpr ✅
- TestGetMutantTypesForToken_NonAmbiguousToken ✅
- TestGetMutantTypesForToken_UnsupportedToken ✅

---

## Stage 2: Create Expression Mutation Infrastructure ✅
**Goal**: Build foundation for expression-level mutations (parallel to token system)
**Status**: Complete

### Success Criteria
- [x] NodeExpr struct for expression mutation points
- [x] ExprMutator implementation (parallel to TokenMutator)
- [x] Expression discovery integrated into engine
- [x] Expression mutations stream through same worker pool
- [x] Dry-run mode works for expression mutations
- [x] File locking works for both token and expression mutations

### Tests
- TestExprMutatorApplyAndRollback ✅
- TestExprMutatorTypeAndStatus ✅
- findParentAndReplacer implementation with 9+ parent node types ✅

---

## Stage 3: Implement ! → !! Mutation ✅
**Goal**: Implement first expression-level mutation using AST reconstruction
**Status**: Complete

### Success Criteria
- [x] InvertLogicalNot mutation type added
- [x] UnaryExpr with NOT operator detected
- [x] AST reconstruction creates !!x from !x
- [x] Parent-child relationship handling works
- [x] Mutation appears in results
- [x] Tests catch the mutation (if test coverage exists)
- [x] Comprehensive test coverage for various contexts
- [x] Documentation created for new mutation
- [x] Mutations index updated

### Tests
- TestExprMutatorApplyAndRollback (covers if, assignment, function call) ✅
- TestExprMutatorInvalidMutationType ✅
- TestGetExprMutantTypes_UnaryNotExpression ✅
- TestGetExprMutantTypes_UnaryOtherOperator ✅
- TestGetExprMutantTypes_NonUnaryExpression ✅
- TestGetExprMutantTypes_NilExpression ✅
- TestTypeString (mutator.go - InvertLogicalNot case) ✅
- TestReportToFile (report.go - InvertLogicalNot statistics) ✅

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
