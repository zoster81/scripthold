# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Public releases: see [GitHub Releases](https://github.com/zoster81/scripthold/releases/latest); **4.0.1** was released through the protected immutable release process on 2026-10-26; post-release semantic fixes remain unreleased.
- Current public capabilities: **36 tools**, **3 guided prompts**, **168 registered text encodings**, and **100 active source-intelligence providers**.
- Completed work is summarized in [`CHANGELOG.md`](../CHANGELOG.md) and implemented history remains in Git.
- The 4.0 unified confirmation milestone is completed and publicly released. See the [changelog](../CHANGELOG.md#400---2026-10-25) and [migration guide](MIGRATION_4.0.md). The 4.0.1 Source Intelligence implementation is released; the remaining work concerns post-release semantic reliability and measured client usefulness. Publication and private deployment remain separate; installing or updating a running server requires its own controlled action.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## Active milestone — Post-release Source Intelligence reliability and client evaluation

**Released in 4.0.1:** Source Intelligence offers bounded, declaration-identifying navigation and compact responses. `source_symbols outline` defaults to `tree`; clients requiring historical top-level `files` and `symbols` must request `presentation: "detailed"`. Direct ChatGPT tool-choice and comparative usability experiments remain post-release work and require a controlled connector update.

**Remaining work after 4.0.1:**

- **Semantic reliability gate (priority before further optimization):** independently review real-code declarations for every active source provider across multiple projects and relevant dialects. Assert both expected and forbidden symbols, kinds, parents, positions, encoding/line-ending behavior, malformed-input bounds, and honest incomplete-coverage evidence. Maintain a portable positive/negative oracle matrix under `internal/sourceintelligence/testdata/` and opt-in real-corpus checks. Report how many providers actually have independent checks; neither 100 active providers nor `CoverageComplete=true` proves complete semantic accuracy. Do not claim all-language reliability while gaps remain.
- Run independent ChatGPT-as-client navigation tasks over authorized Go, Python and C# corpora; compare spontaneous `source_*` choice, correctness, calls, bytes, relevant evidence, latency, allocations and coverage against grep/read baselines. The earlier 30% median call-reduction goal remains unproven.
- Compare compact project `digest` → bounded `outline` tree → verified `show` workflows with direct text search and selected reads; preserve cases where grep/read performs better.
- Reassess project-scale context relevance, traversal cost and bounded memory only where empirical results reveal concrete shortcomings.
- Expand caller/reference semantics per language only when analyzers provide evidence. Indexed relations cannot prove the absence of unindexed references.
- Plan any later optimization or semantic expansion from those measurements; avoid new tools or persistent indexes without a separate evidence-backed decision.

**Goal:** make structural Source Intelligence demonstrably preferable to ordinary text search and file reads for the code-navigation tasks it can solve better. Measure accurate answers, actual client tool choice, total retrieval effort, and irrelevant output without weakening security, source freshness, or coverage reporting. Fewer MCP calls is a supporting outcome, not sufficient evidence of usefulness.

1. Establish a reproducible multilingual baseline using read-only real-world projects. Give ChatGPT natural source-navigation questions without requiring a particular tool, then compare actual `source_*` use with `grep_text_files`, file search, and selected reads. Record answer correctness, relevant evidence, coverage, MCP calls, response bytes, latency, and allocations. Keep guided examples separate from independent tool choice; do not execute third-party code.
2. Verify declaration names, kinds, ownership, and source positions against independent real-code positive and negative oracles across supported providers, projects, and dialects. Distinguish complete analysis from semantic correctness, and retain explicit evidence when coverage is partial.
3. Compare the already released compact `digest` → selected `outline` tree → fingerprint-verified `show` workflow against grep/read baselines. Preserve cases where direct text retrieval is more reliable or efficient; investigate measured shortcomings before optimizing.
4. Extend caller/reference semantics only where an analyzer can prove the relation, with explicit evidence and ambiguity states. Qualify any change using focused TDD, bounded regression and relevant security/platform checks. The earlier 30% median call-reduction goal remains unproven.

**Released functionality:** Version 4.0.1 provides declaration-identifying structural search, explicit coverage and truncation evidence, bounded cursor continuation, compact project digests, and hierarchical source outlines with the explicit `presentation: "detailed"` compatibility path. See the [4.0.1 changelog](../CHANGELOG.md#401---2026-10-26), [tool reference](../TOOLS.md#source_symbols), and [language capability matrix](LANGUAGE_CAPABILITIES.md) for the published contract. Selected Go, Python, and C# corpus probes verified navigation, but did not establish better spontaneous ChatGPT tool selection or semantic accuracy for every provider. Cross-platform CI verifies the released implementation, not universal parser correctness.

Avoid new public tools, breaking requests, persistent indexing, and external language servers without a separate evidence-backed decision.

## Release engineering

The active publication flow is a protected, draft-first, single-writer pipeline. It authenticates and publishes the complete 22-asset set as an immutable GitHub Release before publishing matching MCPB metadata to the MCP Registry. Its first end-to-end public qualification was **v3.3.3**; ongoing release requirements and recovery safeguards are documented in [PUBLISHING.md](PUBLISHING.md). Version-specific completed work belongs in [CHANGELOG.md](../CHANGELOG.md).
