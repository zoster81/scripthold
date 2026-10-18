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
- **Prepared local components:** the 13-output GoReleaser selector, canonical 15-original source binding, rooted Linux original-byte copier, and fail-closed private 15-file stage are available as future-only building blocks. They do not independently establish genuine Sigstore identity or authorize GitHub artifact transfer or publication, and they are **not part of the active release workflow**.
- **Next boundary:** authenticate original provenance and the exact producer repository, source commit, workflow, run and attempt; bridge the two signed original sidecars into the same trusted producer source area without changing their bytes; verify numeric GitHub upload artifact IDs and an unchanged 15-file raw transfer before joining the complete 22-asset finalizer.
- **Promotion gate:** review the workflows, trust validators, CI policy/evidence map, tests, and documentation as one coherent change; qualify the exact GitHub cross-job transfer using harmless synthetic files and the full supported platform matrix before any genuine release. Do not create artificial public releases or locally produce MCPB bundles to obtain evidence.

## Reserved 4.0 boundary

Version 4.0 is reserved for a deliberately reviewed major compatibility or capability boundary after the 3.x work. Its scope is not frozen.

Automatic project-wide semantic refactoring remains out of scope unless maintainers explicitly reopen that decision.

Research topics such as visual desktop interaction or MCP federation are not approved milestones or APIs. They require separate product, architecture, privacy, security, and UX review before implementation.
