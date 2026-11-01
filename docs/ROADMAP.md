# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Public releases: see [GitHub Releases](https://github.com/zoster81/scripthold/releases/latest); **4.0.1** was released through the protected immutable release process on 2026-10-26; post-release semantic fixes remain unreleased.
- Current public capabilities: **36 tools**, **3 guided prompts**, **168 registered text encodings**, and **100 active source-intelligence providers**.
- Completed release changes belong in the [Changelog](../CHANGELOG.md), and the [4.0 migration guide](MIGRATION_4.0.md) covers the unified confirmation tool. Publishing and installing are separate; an active server does not change until it is explicitly updated.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## Active milestone — Post-release Source Intelligence reliability and client evaluation

**Released in 4.0.1:** Source Intelligence offers bounded, declaration-identifying navigation and compact responses. `source_symbols outline` defaults to `tree`; clients requiring historical top-level `files` and `symbols` must request `presentation: "detailed"`. Direct ChatGPT tool-choice and comparative usability experiments remain post-release work and require a controlled connector update.

**Remaining work after 4.0.1 — acceptance gates:**

1. **Qualify projects, not file counts.** Use the existing project/provider qualification ledger and provenance to select independent, eligible real projects for all 100 active providers (at least eight projects per provider where available). Preserve reasons for excluded, missing, unroutable, or historically different projects. The 800-file smoke set and 1,509 qualification samples support this work but do not replace project-level navigation questions. Do not execute third-party code.
2. **Prove structural correctness.** Check independently chosen source declarations against original text, including expected **and forbidden** symbols, names, kinds, ownership, and positions. Include dialects, encoding/BOM and line endings, malformed/oversized inputs, and truthful partial-coverage reports. Keep portable positive/negative oracles under `internal/sourceintelligence/testdata/` and optional original-corpus checks. A provider being enabled, or `coverageComplete: true`, does not certify its semantic accuracy.
3. **Measure usefulness against text search.** For each qualified project, run the **same independently answerable navigation question** using bounded `source_*` operations and a `grep_text_files`/targeted-read baseline. Include cross-file definitions, supported relationships, dependency navigation, ownership, and negative results. Record correct/incorrect answers, false positives, skips, coverage and truncation, and calls, response bytes, latency, and allocations **only when actually measured**. Compare compact `digest` → selected `outline` → fingerprint-verified `show` with direct search; retain cases where grep/read wins.
4. **Test real client behavior separately.** Run independent ChatGPT-as-client navigation across projects and languages, including Go, Python, and C#, and record spontaneous tool choice, answer quality, and irrelevant output. Do not conflate scripted paired tests with client-choice evidence. Reassess context selection and traversal cost only where measurements warrant it; extend caller/reference semantics only when the analyzers can substantiate each relation. Unindexed relationships do not prove absence of callers or uses.

**Goal and decision rule:** Demonstrate that Source Intelligence gives more accurate or more useful answers for appropriate code-navigation tasks without weakening workspace security, source freshness, or honest coverage reporting. Improvements and any later optimization must be based on verified paired evidence. The earlier 30% median call-reduction target remains **unproven**; fewer calls alone are not a success criterion. Avoid additional public tools, breaking schemas, persistent indexes, and external language servers without separate evidence-backed approval.

**Published baseline:** The [4.0.1 changelog](../CHANGELOG.md#401---2026-10-26), [tool reference](../TOOLS.md#source_symbols), and generated [language capability matrix](LANGUAGE_CAPABILITIES.md) define the released Source Intelligence contract. Cross-platform CI qualifies that implementation but does not prove universal parser correctness. The protected release process is documented separately in [Publishing](PUBLISHING.md).
