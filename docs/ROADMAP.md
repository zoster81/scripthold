# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Current public release: **Scripthold 4.0.0**, published through the protected immutable GitHub release process.
- Current public capabilities: **36 tools**, **3 guided prompts**, **168 registered text encodings**, and **100 active source-intelligence providers**.
- Completed work is summarized in [`CHANGELOG.md`](../CHANGELOG.md) and implemented history remains in Git.
- The 4.0 unified confirmation milestone is completed and publicly released. See the [changelog](../CHANGELOG.md#400---2026-10-25) and [migration guide](MIGRATION_4.0.md). The Source Intelligence efficiency milestone below is approved but unreleased. Publication and private deployment remain separate; installing or updating a running server requires its own controlled action.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## Active milestone — Source Intelligence for LLM Efficiency

**Goal:** help LLM clients answer real code questions accurately with fewer MCP calls and less irrelevant response data, without weakening security, source freshness, or coverage reporting.

1. Establish a reproducible multilingual baseline on existing read-only real-world corpora. Track task correctness, MCP calls, response bytes, coverage, latency, and allocations; do not execute third-party code.
2. Make declaration search matches self-identifying (`name`, `kind`, `qualifiedName`), keeping non-symbol matches free of invented identity, and explicitly distinguish file coverage, analyzer coverage, and result truncation.
3. Improve the relevance and size of returned context; evaluate a direct bounded search-to-context workflow only when it reduces verified client round trips.
4. Evaluate safe cursor-based continuation with stable ordering, bound generation and query, expiry, stale/tampered cursor rejection, and bounded temporary memory. Keep independent result and file-selection limits.
5. Extend reference semantics selectively by language, with explicit evidence and ambiguity states; never infer unproven callers or callees.
6. Qualify each increment with focused TDD, regressions, relevant security and platform checks, and paired LLM task comparisons. Aim for a 30% reduction in median calls without correctness loss; this is a target, not a measured achievement.

**Current implementation:** declaration-identifying search, independent coverage-reason flags, targeted reverse-dependency context relevance, request-local reuse of context symbol lists, and bounded Top-K structural search selection are implemented on the unreleased development branch. Bounded keyset continuation is now exposed through an authenticated, expiring, query- and generation-bound `search.nextCursor`/`cursor` pair. Opt-in read-only checks on selected real Go, Python, and C# project scopes verify that paged search reconstructs the same ordered matches as a complete search, and measure response size and per-request allocations. Three opt-in declaration task oracles on known real source files establish name, kind, location, and fingerprint correctness targets for later paired-client trials; they do not simulate LLM behavior. Full-repository coverage, paired real-client LLM task comparisons, broader context relevance, and semantic expansion remain to be qualified.

Avoid new public tools, breaking requests, persistent indexing, and external language servers without a separate evidence-backed decision.

## Release engineering

The active publication flow is a protected, draft-first, single-writer pipeline. It authenticates and publishes the complete 22-asset set as an immutable GitHub Release before publishing matching MCPB metadata to the MCP Registry. Its first end-to-end public qualification was **v3.3.3**; ongoing release requirements and recovery safeguards are documented in [PUBLISHING.md](PUBLISHING.md). Version-specific completed work belongs in [CHANGELOG.md](../CHANGELOG.md).

## Completed milestones

The 4.0 unified confirmation milestone was implemented, qualified on Linux, macOS, and Windows, and released as `v4.0.0` on 2026-10-25. Its eight-to-one MCP API migration, compatibility break, and retained security boundaries are documented in the [4.0 changelog](../CHANGELOG.md#400---2026-10-25), [migration guide](MIGRATION_4.0.md), and [architecture](ARCHITECTURE.md). The detailed implementation plan remains in Git history rather than this forward-looking roadmap.
