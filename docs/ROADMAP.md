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

## Planned publication hardening (not active)

Future release engineering may introduce GitHub-enforced immutable releases, but **the current 3.3.0 publication pipeline remains the supported, functioning flow**. This is a release-infrastructure change, not a new MCP capability or an approved 4.0 scope.

- **Outcome:** prepare and verify the complete normal/provenance/MCPB asset set before public publication, using least-privileged jobs, exact source/run identities, and fail-closed draft recovery. Preserve the existing GoReleaser archives, genuine signed evidence, GitHub-only MCPB packaging, and Registry output.
- **Design boundary:** prevent recurrence of historical CI failures involving Windows 8.3 paths, macOS temporary-directory aliases, link/permission races, tool-version mismatches, Scorecard execution on tags, incorrect package types, and post-publication asset mutations. The durable requirements and negative-test gate are in [PUBLISHING.md](PUBLISHING.md#future-immutable-release-pipeline-prevention-requirements-not-active), with reusable verification in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md#future-immutable-release-workflow-redesign-not-the-current-release-flow).
- **Completion gates:** the eight milestones below measure the proposed **end-to-end immutable publication flow**, not the number of implemented helpers or passing unit tests. A partially implemented gate is not release-ready.

| Gate | Required outcome | Current status |
| --- | --- | --- |
| 1. Raw transfer contract | Transfer all 22 original-name assets by explicit GitHub artifact ID and verify size/SHA-256 | **Complete for all 22 synthetic GitHub-hosted original-name transfers**; a separate genuine 13-file normal transfer is qualified, but the authentic 22-file release set is not |
| 2. Private normal staging | Restore and verify exactly 15 original normal files on Linux before MCPB preparation | **Partial**; hosted Linux verified the genuine 13 GoReleaser binary/archive/checksum bytes across producer and consumer, and all 15 synthetic normal originals in a separate private staging test; genuine signed 15-file staging and MCPB consumption are not connected |
| 3. Authenticated normal producer | Build genuine GoReleaser originals and preserve the 12-subject signed Sigstore/DSSE evidence with exact workflow/job/run/attempt | **Partial**; genuine 13-file producer/consumer proof checks exact GitHub workflow, source SHA, run/attempt, successful producer job and artifact IDs/digests; genuine historical 12-subject signatures were separately verified, but the same-run 15-file signed chain is not qualified |
| 4. Authentic MCPB producer | Create the six genuine GitHub-only MCPB packages from independently verified normal binaries | **Not connected** |
| 5. Single draft finalizer | Verify all 22 authentic assets, create or safely resume one unpublished draft, and recover failed publication without mutation | **Partial (future-only)**; separate read-only `verify-complete` authenticates 15+7 original files and the tagged GitHub job context; the separately approved `finalize` independently re-downloads and authenticates the 22 original bytes at its own runner boundary, yet retains read-only permissions and a deliberate nonpublishing exit. No genuine tagged-run job approval, live writer, draft write, recovery or public publish is wired or qualified |
| 6. Registry handoff | Publish Registry metadata through OIDC only after the verified release and retire competing publication paths | **Not connected** |
| 7. Integrated qualification | Run complete cross-platform CI, hostile-input tests, real raw-transfer proof and release-state failure/recovery tests on the new flow | **Partial**; full cross-platform CI, synthetic 22-file transfer and genuine 13-file cross-job verification are qualified; signed 15-file/MCPB/finalizer integration and failure recovery remain unqualified |
| 8. Controlled promotion | Perform the first genuine immutable release and enable optional GitHub immutability only with explicit maintainer authorization | **Not started** |

### Minimal execution plan and check ownership

Use the gate numbers above for status, but execute them in dependency order: **reuse gate 1 → gate 3 → gate 2 → gate 4 → gate 5 → gate 7 → gate 8 → gate 6**. A passing local unit test does not complete a hosted gate. Each check belongs to the stage that can prove it; do not repeat successful evidence in a downstream job unless the files cross another trust boundary.

| Gate | Operation | Essential checks, performed at this boundary | Completion evidence |
| --- | --- | --- | --- |
| **1** | Reuse the raw-file transfer method | Confirm the existing GitHub-hosted 22-name/ID, unchanged-size/SHA-256 and preserved ZIP-byte proof still applies to the pinned upload/download actions. | **Synthetic transport qualified; reuse it** unless the action, transfer contract or observed behavior changes. Actual release bytes are checked at gates 2 and 5. |
| **3** | Produce **13 genuine GoReleaser originals and 2 authentic signed companions in one tagged Release job** | Correct annotated tag/source commit; expected 13 originals and checksum closure; genuine Sigstore bundle and unchanged signed DSSE envelope; exact job/run/attempt and 15 upload IDs. Do not substitute a PR/manual signer for the tagged certificate identity. | One genuine GitHub-run producer with 15 bound originals and valid signed evidence; **not yet qualified**. |
| **2** | Restore the 15 originals in the GitHub MCPB consumer | Download only those 15 numeric IDs into one private directory; verify exact original names, sizes and SHA-256 at this **new job boundary**; run genuine GitHub CLI signature verification for the 12 signed subjects **once here**. | Successful same-run 15-file signed Linux handoff; **not yet qualified**. |
| **4** | Build the six real MCPB packages on GitHub and stage their checksum file | Use only the verified gate-2 binaries; run existing MCPB package validation and check the six output checksums; bind all 7 original package/checksum files to their upload IDs. | Six genuine GitHub-only packages plus checksum file, from the authenticated 15-file input; **not yet qualified**. |
| **5** | Assemble the 22 files and safely prepare one draft | Recheck producer jobs/run/attempt; verify the **22 downloaded bytes at the final job boundary**, with no second 12-signature CLI pass; match tag and original release notes; create or resume one unpublished draft, upload missing exact originals once and confirm the remote 22-asset inventory. Conflict or uncertain mutation stops without blind retry. | Complete, verified **unpublished** GitHub draft and exercised recovery behavior; writer/workflow integration still pending. |
| **7** | Qualify the candidate as one coherent change | Once the full workflow is assembled, run pinned workflow syntax/static checks, required full-tier cross-platform CI, signed raw-transfer integration and focused draft/retry failure cases on the **same source revision**. | Required GitHub CI and relevant hosted evidence green. Re-run only affected checks after a fix, plus required full-tier promotion gates; **partial**. |
| **8** | Authorize and publish the first immutable release | Maintainer approval; retire competing post-publication writers; enable and independently confirm GitHub immutable-release enforcement; verify exact annotated tag, main SHA and successful Test Suite/Scorecard authority. Publish the already complete draft once, then read back **immutable public state and all 22 remote files**. | One genuine release proven immutable on GitHub; **not started**. No public transition while the setting is off. |
| **6** | Hand off the finished release to MCP Registry | Only after gate 8: use the existing Registry OIDC publisher, check the published metadata points to the immutable release and do not modify release assets. | Successful Registry publication after the verified release; **not yet qualified**. |

**Avoid repeated work:** preserve accepted evidence for the same unchanged source, action versions and inputs. Verify SHA-256 again only when bytes cross a producer/consumer boundary, not several times in the same job. Verify genuine Sigstore signatures once at gate 2. Run full CI for a coherent candidate and at the required promotion boundary, not after every small edit. If a concrete test or hosted job fails, inspect that failure and correct only the affected stage; retain already qualified unrelated stages. The first genuine tag-bound signed execution is inseparable from the carefully authorized release path, so local/mocked or PR-only proofs must never be marked as that final hosted qualification.

**Next:** before creating the first genuine tag, finish and qualify the protected single-writer workflow on one reviewed commit, as required by the [controlled cutover procedure](PUBLISHING.md#controlled-cutover-to-the-first-immutable-release-future-only). The current proposed finalizer still fails closed and must not replace the supported publisher yet. Once that controlled transition is authorized, the first authentic tagged run qualifies the genuine 15-file signed producer and GitHub-hosted consumer (gates 3 then 2), followed by real GitHub MCPB packaging and the draft finalizer. Creating a tag under the disabled finalizer and only later changing `main` would break the exact tag-to-current-main release authority; do not move the tag or relax this rule. Reuse qualified synthetic transport and prior CI evidence where unchanged, never substitute simulated signing or local MCPB outputs for real hosted proof.

## Reserved 4.0 boundary

Version 4.0 is reserved for a deliberately reviewed major compatibility or capability boundary after the 3.x work. Its scope is not frozen.

Automatic project-wide semantic refactoring remains out of scope unless maintainers explicitly reopen that decision.

Research topics such as visual desktop interaction or MCP federation are not approved milestones or APIs. They require separate product, architecture, privacy, security, and UX review before implementation.
