# Automatic Phase E finalization

The [Phase E workflow](../../.github/workflows/ground-control-phase-e.yml) runs after a PR merges into `dev` or `main`.
It calls Ground Control's existing merged-PR finalizer to post the delivery report and close the delivered issue.
It does not merge PRs or execute delivered Go code, tests, hooks, or workflow commands.
PRs without a trusted delivery handoff are successful no-ops.

## Configure release access

Add the Actions secret `GROUND_CONTROL_READ_TOKEN` to `OpenRAE/go-rae-gremlins`.
Use a fine-grained token restricted to `autarchy-ai/Ground-Control` with **Contents: read-only** and its mandatory metadata read access.
Authorize organization access if required. Do not reuse an operator-wide write token.
Keep the existing `SONAR_TOKEN` secret configured for finalizer settings.

The release token is available only to the download step.
The finalizer uses this repository's automatic token for evidence reads, issue comments, and closure.
No write access to repository contents or PRs is granted.
The local developer `.env` is not copied or changed.
A missing download credential fails with a setup message; it does not silently skip finalization.

## First installation and fork deliveries

Merge the workflow through the normal reviewed PR into `dev`.
The installation PR can have `finalization: reentry` because its base did not yet contain the workflow.
Re-invoke its `/implement` or `/quickfix` command after merge to finish that delivery.
Later deliveries use automatic finalization once their base includes the workflow and release access is configured.

Manual dispatch becomes available after the workflow reaches the default branch, `main`, through an approved promotion.
GitHub restricts secrets and write tokens on ordinary fork PR events.
The [fork dispatcher](../../.github/workflows/ground-control-fork-finalization.yml) handles merged fork PRs through a trusted merge-only event.
It checks out no source and reads no private secret. It can only dispatch this repository's fixed Phase E workflow on `main`.
The dispatcher has PR-read and Actions-write access; it has no issue-write access.
The finalizer runs under the supported `workflow_dispatch` event with its original permissions.
Automatic fork finalization requires the Phase E definition on `main`, just like manual dispatch.
No Ground Control source change is needed.
Merges made with `GITHUB_TOKEN` can suppress follow-up events; use the same recovery paths.

## Recover a failed run

Inspect the failed **Ground Control Phase E** run and its issue failure record.
Repair the named cause, such as expired release access or unavailable evidence.
Rerun that workflow run, or dispatch it from `main` with the merged PR number:

```shell
gh workflow run ground-control-phase-e.yml --repo OpenRAE/go-rae-gremlins --ref main -f pr=123
```

Replace `123` with the delivered PR number.
The workflow checks the live merge state and base, then checks out its immutable merge revision.
An unmerged PR or a different base is refused.
The same finalizer handles automatic execution, repair, and retries without duplicating final reports.
Local `/implement <issue>` or `/quickfix <issue>` reentry remains available when Actions cannot run.

## Upgrade the utility

The workflow pins the release version, asset name, SHA256, Node version, and external action commits.
The [installation manifest](../../.github/ground-control/package.json) and [lockfile](../../.github/ground-control/package-lock.json)
pin the utility dependency graph and integrity values.
Change the release tuple and regenerate the lockfile together in a reviewed PR.
Verify the selected release's CLI, workflow recognition, and Actions provenance compatibility.
Never commit the downloaded private tarball or installed dependencies.

Installation uses `npm ci` with lifecycle scripts, audit submission, and package-manager caching disabled.
Transport manifests come from the workflow revision, separately from the delivered revision being repaired.
The [design](phase-e-design.md) records data flow, permissions, integrity, and failure boundaries.

## Verify an upgrade locally

Install the pinned utility in a temporary directory using the committed manifest, lockfile, and checksum-verified tarball.
Use the same disabled lifecycle scripts as the workflow.
Point `GROUND_CONTROL_TEST_ROOT` at that directory and run:

```shell
node --test .github/ground-control/phase-e.integration.test.mjs
```

The sixteen cases execute the actual workflow shell steps and installed CLI against a stateful GitHub stand-in.
No finalizer, authorization, evidence-discovery, or provenance function is substituted.
They verify both events and branches, production repository authorization, trusted evidence discovery, Actions provenance, correct issue/report binding, retries, invalid deliveries, permission denial, untrusted authors, mismatched provenance, immutable revision selection, tampered-download rejection, locked installation, and automatic fork dispatch.
The stand-in enforces each job's permission map, including refusal without issue-write access.
All finalization cases use a fresh checksum-verified, locked installation; the reference package supplies only the YAML parser during setup.
They do not use a live GitHub credential or change a real issue.
For issue #97, all sixteen cases passed against the checksum-verified v1.9.0 installation.
Another 47 targeted upstream release tests passed for readiness, merge gates, reporting, and automated finalization.
These results do not replace the first configured Actions run.

Verification evidence: [TAP results](evidence/phase-e-v1.9.0.tap) and [release/source/file hashes](evidence/phase-e-v1.9.0.json).
