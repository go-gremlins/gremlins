# Maintenance plan

This plan makes go-rae-gremlins a well-maintained fork of Gremlins. Its issues are in the Maintenance baseline
milestone. The phases run in order.

autarchy-ai/coldlight#3 selects go-rae-gremlins for mutation testing, and OpenRAE/env-packs#557 makes mutation testing
on changed lines a merge gate. The defects in #1 are on the critical path of that work.

## 1. Capture

The open upstream issues and PRs are in the fork. Nobody merges a copied item before triage (#83).

- #66 Copy the four upstream PRs that the pre-push guard blocked

## 2. Ready for development

#64 onboards the repository to Ground Control, aligns the workflows, moves the Sonar analysis to raesystem, records
ADRs 0001 to 0003 (module path, binary name and versions, upstream changes), and sets up the release process.
`docs/development/upstream-release.md` describes the upstream build and release.

## 3. Understand

Each review has a record, a baseline, and decisions. The reviews do not fix code.

- #75 Review the development process and merge the upstream conventions with ours
- #76 Model the architecture as code and check designs against the model
- #77 Set the coding standards for strict linting and the Google Go Style rules
- #78 Define the test standard
- #79 Gate changes on mutation testing of changed lines and on regression proof
- #80 Review the code quality
- #81 Review the security
- #82 Review the documentation

## 4. Triage

Each copied item and #1 has a decision: keep, rework, or close.

- #83 Triage the copied upstream items and #1 against the review results

## 5. Remediate and develop

The kept work and the remediations are done under the new gates. Triage (#83) creates their issues.

- #84 Cut a tagged release that the dependent repositories can pin
