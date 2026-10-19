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
| 5. Single draft finalizer | Verify all 22 authentic assets, create or safely resume one unpublished draft, and recover failed publication without mutation | **Not connected** |
| 6. Registry handoff | Publish Registry metadata through OIDC only after the verified release and retire competing publication paths | **Not connected** |
| 7. Integrated qualification | Run complete cross-platform CI, hostile-input tests, real raw-transfer proof and release-state failure/recovery tests on the new flow | **Partial**; full cross-platform CI, synthetic 22-file transfer and genuine 13-file cross-job verification are qualified; signed 15-file/MCPB/finalizer integration and failure recovery remain unqualified |
| 8. Controlled promotion | Perform the first genuine immutable release and enable optional GitHub immutability only with explicit maintainer authorization | **Not started** |

The **next engineering boundary is gates 2–3**: qualify one trusted future producer that delivers 13 genuine GoReleaser originals **together with** two authentic signed provenance companions, preserving signed subject identity and the complete 15-file byte inventory on a GitHub-hosted Linux consumer. GitHub artifact metadata does not independently identify the uploading job: the existing proof corroborates successful exact-job evidence with its reviewed step outputs, without claiming an atomic snapshot or full signed-release authority. The active release pipeline is unchanged.
- **Promotion gate:** review the workflows, trust validators, CI policy/evidence map, tests, and documentation as one coherent change; qualify the exact GitHub cross-job transfer using harmless synthetic files and the full supported platform matrix before any genuine release. Do not create artificial public releases or locally produce MCPB bundles to obtain evidence.

## Reserved 4.0 boundary

Version 4.0 is reserved for a deliberately reviewed major compatibility or capability boundary after the 3.x work. Its scope is not frozen.

Automatic project-wide semantic refactoring remains out of scope unless maintainers explicitly reopen that decision.

Research topics such as visual desktop interaction or MCP federation are not approved milestones or APIs. They require separate product, architecture, privacy, security, and UX review before implementation.
