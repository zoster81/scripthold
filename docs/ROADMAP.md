# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Current public release: **Scripthold 3.3.0**.
- Public 3.3.0 surface: **43 tools**, **3 guided prompts**, **168 registered text encodings**, and **100 active source-intelligence providers**.
- Completed work is summarized in [`CHANGELOG.md`](../CHANGELOG.md) and implemented history remains in Git.
- No additional 3.x capability milestone is currently active; verified self-update is implemented on the development branch, while release and promotion qualification remain separate.
- Publication, installation, and private deployment are separate actions.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## Immutable publication cutover (first release pending)

The single-writer draft-first release pipeline is installed on `main`; both former public asset writers were retired together. **Scripthold 3.3.0 remains the latest published release.** GitHub release immutability is enabled. Genuine tagged runs have qualified the signed 15-file producer/consumer, all seven MCPB outputs and independent 22-file staging verification. The protected writer was approved and minted separate scoped App tokens, but halted safely after creating an empty, unpublished draft when immediate release-list discovery did not confirm the new release. A corrected unused version must qualify final publication and Registry handoff; never move or reuse consumed tags.

- **Outcome:** prepare and verify the complete normal/provenance/MCPB asset set before public publication, using least-privileged jobs, exact source/run identities, and fail-closed draft recovery. Preserve the existing GoReleaser archives, genuine signed evidence, GitHub-only MCPB packaging, and Registry output.
- **Design boundary:** prevent recurrence of historical CI failures involving Windows 8.3 paths, macOS temporary-directory aliases, link/permission races, tool-version mismatches, Scorecard execution on tags, incorrect package types, and post-publication asset mutations. The durable requirements and negative-test gate are in [PUBLISHING.md](PUBLISHING.md#immutable-release-pipeline-prevention-requirements-first-publication-pending), with reusable verification in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md#immutable-release-workflow-qualification-before-first-publication).
- **Completion gates:** the eight milestones below measure the proposed **end-to-end immutable publication flow**, not the number of implemented helpers or passing unit tests. A partially implemented gate is not release-ready.

| Gate | Required outcome | Current status |
| --- | --- | --- |
| 1. Raw transfer contract | Transfer all 22 original-name assets by explicit GitHub artifact ID and verify size/SHA-256 | **Qualified through the genuine read-only complete-22 verification boundary**; synthetic transport and real signed-15 staging were also qualified. Protected draft uploads and immutable public readback remain pending. |
| 2. Private normal staging | Restore and verify exactly 15 original normal files on Linux before MCPB preparation | **Qualified in genuine tagged runs:** all 15 original bytes and 12 Sigstore subjects verified by the Linux MCPB consumer. |
| 3. Authenticated normal producer | Build genuine GoReleaser originals and preserve the 12-subject signed Sigstore/DSSE evidence with exact workflow/job/run/attempt | **Qualified in genuine annotated-tag production:** 13 original GoReleaser assets and two signed provenance companions were transferred and authenticated together. |
| 4. Authentic MCPB producer | Create the six genuine GitHub-only MCPB packages from independently verified normal binaries | **Qualified in a genuine tagged run:** six platform MCPB packages plus checksum were produced and their seven originals authenticated at the downstream complete-asset verifier. |
| 5. Single draft finalizer | Verify all 22 authentic assets, create or safely resume one unpublished draft, and recover failed publication without mutation | **Partial:** separate unprivileged and protected boundaries verified all 22 originals, a real environment reviewer approved, and both narrowly scoped App tokens were minted. The finalizer created one empty unpublished draft but halted when immediate release-list discovery did not confirm it. No asset upload or public transition occurred; stable-ID confirmation fix awaits qualification. |
| 6. Registry handoff | Publish Registry metadata through OIDC only after the verified release and retire competing publication paths | **Partial (merged, no real OIDC handoff)**; the callable and manual Registry entrypoints now share a read-only GitHub gate requiring the exact tagged source commit, a published immutable release and all 22 original names with nonduplicate IDs, sizes and SHA-256 digests before OIDC. Trusted workflow code authenticates the release before tag code executes; after that gate, manifest generation uses the exact tag checkout instead of a potentially different `main` revision. No genuine immutable publication or Registry handoff has been executed; the Registry publisher now waits for a successful immutable release from the protected finalizer |
| 7. Integrated qualification | Run complete cross-platform CI, hostile-input tests, real raw-transfer proof and release-state failure/recovery tests on the new flow | **Partial:** exact merged-main full 24-job Test Suite, Scorecard and CodeQL passed; genuine signed 15-file handoff, seven MCPB originals and independent complete-22 staging succeeded. Protected draft creation stopped safely on visibility, so real complete immutable publication and recovery remain unqualified. |
| 8. Controlled promotion | Perform the first genuine immutable release and enable optional GitHub immutability only with explicit maintainer authorization | **Partial:** GitHub release immutability enabled; distinct annotated version tags were used for fail-closed genuine attempts. Neither produced a public release. One empty unpublished draft exists for a consumed tag; do not overwrite, move the tag or blindly retry. The corrected writer requires a new reviewed and qualified version. |

### Minimal execution plan and check ownership

Use the gate numbers above for status, but execute them in dependency order: **reuse gate 1 → gate 3 → gate 2 → gate 4 → gate 5 → gate 7 → gate 8 → gate 6**. A passing local unit test does not complete a hosted gate. Each check belongs to the stage that can prove it; do not repeat successful evidence in a downstream job unless the files cross another trust boundary.

| Gate | Operation | Essential checks, performed at this boundary | Completion evidence |
| --- | --- | --- | --- |
| **1** | Reuse the raw-file transfer method | Confirm the existing GitHub-hosted 22-name/ID, unchanged-size/SHA-256 and preserved ZIP-byte proof still applies to the pinned upload/download actions. | **Synthetic transport qualified; reuse it** unless the action, transfer contract or observed behavior changes. Actual release bytes are checked at gates 2 and 5. |
| **3** | Produce **13 genuine GoReleaser originals and 2 authentic signed companions in one tagged Release job** | Correct annotated tag/source commit; expected 13 originals and checksum closure; genuine Sigstore bundle and unchanged signed DSSE envelope; exact job/run/attempt and 15 upload IDs. Do not substitute a PR/manual signer for the tagged certificate identity. | **Qualified on the genuine annotated-tag producer/consumer run**: 13+2 originals and all 12 signed subjects. Retain the evidence for unchanged source only. |
| **2** | Restore the 15 originals in the GitHub MCPB consumer | Download only those 15 numeric IDs into one private directory; verify exact original names, sizes and SHA-256 at this **new job boundary**; run genuine GitHub CLI signature verification for the 12 signed subjects **once here**. | **Qualified**: same-run 15-file signed Linux handoff and all 12 subjects independently verified; subsequent MCPB packaging not yet qualified. |
| **4** | Build the six real MCPB packages on GitHub and stage their checksum file | Use only the verified gate-2 binaries; run existing MCPB package validation and check the six output checksums; bind all 7 original package/checksum files to their upload IDs. | **Qualified in a genuine tagged run:** six bundles and checksum, seven original uploads, and downstream 22-file stage verification. |
| **5** | Assemble the 22 files and safely prepare one draft | Recheck producer jobs/run/attempt; verify the **22 downloaded bytes at the final job boundary**, with no second 12-signature CLI pass; match tag and original release notes; create or resume one unpublished draft, upload missing exact originals once and confirm the remote 22-asset inventory. Conflict or uncertain mutation stops without blind retry. | **Partially qualified:** protected 22-byte check, reviewer approval and App tokens passed. An empty draft was created but immediate listing confirmation failed. Complete draft and recovery remain unqualified. |
| **7** | Qualify the candidate as one coherent change | Once the full workflow is assembled, run pinned workflow syntax/static checks, required full-tier cross-platform CI, signed raw-transfer integration and focused draft/retry failure cases on the **same source revision**. | Required GitHub CI and relevant hosted evidence green. Re-run only affected checks after a fix, plus required full-tier promotion gates; **partial**. |
| **8** | Authorize and publish the first immutable release | Maintainer approval; retire competing post-publication writers; enable and independently confirm GitHub immutable-release enforcement; verify exact annotated tag, main SHA and successful Test Suite/Scorecard authority. Publish the already complete draft once, then read back **immutable public state and all 22 remote files**. | **In progress**: repository immutability enabled and first annotated tag consumed safely without a public release. Require an unused tag and complete verified draft for the next attempt. |
| **6** | Hand off the finished release to MCP Registry | Only after gate 8: use the existing Registry OIDC publisher, check the published metadata points to the immutable release and do not modify release assets. | Successful Registry publication after the verified release; **not yet qualified**. |

**Avoid repeated work:** preserve accepted evidence for the same unchanged source, action versions and inputs. Verify SHA-256 again only when bytes cross a producer/consumer boundary, not several times in the same job. Verify genuine Sigstore signatures once at gate 2. Run full CI for a coherent candidate and at the required promotion boundary, not after every small edit. If a concrete test or hosted job fails, inspect that failure and correct only the affected stage; retain already qualified unrelated stages. The first genuine tag-bound signed execution is inseparable from the carefully authorized release path, so local/mocked or PR-only proofs must never be marked as that final hosted qualification.

**Next:** review and qualify the stable release-ID verification fix for draft creation, then use a new unused annotated version after exact-main full CI. Preserve the empty draft and consumed tags without manual mutation. The next genuine run must verify all 22 remote draft uploads before one immutable public transition and subsequent Registry OIDC handoff. Reuse unchanged source and transfer evidence; never substitute mocked publication or local MCPB outputs.

## Reserved 4.0 boundary

Version 4.0 is reserved for a deliberately reviewed major compatibility or capability boundary after the 3.x work. Its scope is not frozen.

Automatic project-wide semantic refactoring remains out of scope unless maintainers explicitly reopen that decision.

Research topics such as visual desktop interaction or MCP federation are not approved milestones or APIs. They require separate product, architecture, privacy, security, and UX review before implementation.
