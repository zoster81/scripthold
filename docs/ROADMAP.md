# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Current public release: **Scripthold 4.0.0**, published through the protected immutable GitHub release process.
- Current public capabilities: **36 tools**, **3 guided prompts**, **168 registered text encodings**, and **100 active source-intelligence providers**.
- Completed work is summarized in [`CHANGELOG.md`](../CHANGELOG.md) and implemented history remains in Git.
- The 4.0 unified confirmation milestone is completed and publicly released. See the [changelog](../CHANGELOG.md#400---2026-10-25) and [migration guide](MIGRATION_4.0.md). No new release-scoped milestone is currently approved. Publication and private deployment remain separate; installing or updating a running server requires its own controlled action.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## Release engineering

The active publication flow is a protected, draft-first, single-writer pipeline. It authenticates and publishes the complete 22-asset set as an immutable GitHub Release before publishing matching MCPB metadata to the MCP Registry. Its first end-to-end public qualification was **v3.3.3**; ongoing release requirements and recovery safeguards are documented in [PUBLISHING.md](PUBLISHING.md). Version-specific completed work belongs in [CHANGELOG.md](../CHANGELOG.md).

## Completed milestones

The 4.0 unified confirmation milestone was implemented, qualified on Linux, macOS, and Windows, and released as `v4.0.0` on 2026-10-25. Its eight-to-one MCP API migration, compatibility break, and retained security boundaries are documented in the [4.0 changelog](../CHANGELOG.md#400---2026-10-25), [migration guide](MIGRATION_4.0.md), and [architecture](ARCHITECTURE.md). The detailed implementation plan remains in Git history rather than this forward-looking roadmap.
