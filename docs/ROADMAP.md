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

**Goal:** make structural Source Intelligence demonstrably preferable to ordinary text search and file reads for the code-navigation tasks it can solve better. Measure accurate answers, actual client tool choice, total retrieval effort, and irrelevant output without weakening security, source freshness, or coverage reporting. Fewer MCP calls is a supporting outcome, not sufficient evidence of usefulness.

1. Establish a reproducible multilingual baseline on existing read-only real-world corpora. Give ChatGPT natural code questions without imposing a tool, then compare its selected `source_*`, `grep_text_files`, file search, and text-read workflows. Record correct answers, tool choice, MCP calls, response bytes, relevant evidence, coverage, latency, and allocations; do not execute third-party code. Distinguish guided or previously exposed questions from independent choices.
2. Make declaration search matches self-identifying (`name`, `kind`, `qualifiedName`), keeping non-symbol matches free of invented identity, and explicitly distinguish file coverage, analyzer coverage, and result truncation.
3. Prove useful structural navigation before optimizing response shape. Reduce excessive per-symbol output in `source_symbols outline`, documenting the explicitly approved tree-default response-shape change and the `presentation: "detailed"` compatibility path; keep `maxSymbols` as a matching-result limit for filtered outlines, with bounded analyzer discovery and explicit coverage. Preserve explicit incomplete-coverage evidence. The optional compact `find` file details and `digest` `presentation: "compact"` project map are implemented; evaluate file-map → selected tree → verified `show` against grep/read tasks before further public API expansion or one-call search-to-context.
4. Evaluate safe cursor-based continuation with stable ordering, bound generation and query, expiry, stale/tampered cursor rejection, and bounded temporary memory. Keep independent result and file-selection limits.
5. Extend reference semantics selectively by language, with explicit evidence and ambiguity states; never infer unproven callers or callees.
6. Qualify each increment with focused TDD, regressions, relevant security and platform checks, and comparative LLM tasks that include grep/read baselines. Prioritize demonstrable usefulness and accurate tool selection; the earlier 30% median call-reduction target remains a secondary target, not a measured achievement.

**Current implementation:** declaration-identifying search, independent coverage-reason flags, targeted reverse-dependency context relevance, request-local reuse of context symbol lists, and bounded Top-K structural search selection are implemented on the unreleased development branch. Bounded keyset continuation is now exposed through an authenticated, expiring, query- and generation-bound `search.nextCursor`/`cursor` pair. Opt-in read-only checks on selected real Go, Python, and C# project scopes verify that paged search reconstructs the same ordered matches as a complete search, and measure response size and per-request allocations. Opt-in real-project probes check known-file declaration evidence, project-scope discovery, and source-context correctness in Go, Python, and C#, while reporting file selection and analyzer coverage separately. The handler probes do not execute an LLM client. A manual ChatGPT Web walkthrough against the public connector and matching opt-in search/context probes on the unreleased branch resolved three source questions with two calls each, demonstrating no call-count reduction in this initial sample. The branch exposed clearer declaration identity and coverage, while response-size changes differed by task. Exploratory, non-blind task comparisons found that grep/read workflows often suit behavior tracing, while structural search can be more compact for known declarations. `source_symbols find` supports opt-in `fileDetails: "relevant"`: it omits fully analyzed nonmatching file details while retaining matching and incomplete/error evidence. `source_symbols outline` now defaults to `tree`, with optional `compact` and explicit legacy `presentation: "detailed"`: it projects existing normalized declarations as minimal file-grouped lists or parent-child hierarchies with full symbol IDs and file fingerprints. Focused tests and opt-in Go, Python, and C# corpus checks verify the same symbol counts and aggregate coverage with smaller outputs. These changes have not yet demonstrated better spontaneous LLM tool choice. Actual unbiased client tool-selection rates and end-to-end advantages over grep/read remain unmeasured; those ChatGPT-as-client comparisons are deferred to release qualification. Source file selection now retains only the bounded lexicographic `maxFiles` prefix while scanning the complete authorized traversal for omission evidence, rather than retaining every candidate path. Full-repository qualification, broader context relevance, and semantic expansion remain to be verified.

Avoid new public tools, breaking requests, persistent indexing, and external language servers without a separate evidence-backed decision.

## Release engineering

The active publication flow is a protected, draft-first, single-writer pipeline. It authenticates and publishes the complete 22-asset set as an immutable GitHub Release before publishing matching MCPB metadata to the MCP Registry. Its first end-to-end public qualification was **v3.3.3**; ongoing release requirements and recovery safeguards are documented in [PUBLISHING.md](PUBLISHING.md). Version-specific completed work belongs in [CHANGELOG.md](../CHANGELOG.md).

## Completed milestones

The 4.0 unified confirmation milestone was implemented, qualified on Linux, macOS, and Windows, and released as `v4.0.0` on 2026-10-25. Its eight-to-one MCP API migration, compatibility break, and retained security boundaries are documented in the [4.0 changelog](../CHANGELOG.md#400---2026-10-25), [migration guide](MIGRATION_4.0.md), and [architecture](ARCHITECTURE.md). The detailed implementation plan remains in Git history rather than this forward-looking roadmap.
