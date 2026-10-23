# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Current public release: **Scripthold 3.3.3**, published with GitHub-enforced immutable release assets and the official MCP Registry entry.
- Public 3.3.3 surface: **43 tools**, **3 guided prompts**, **168 registered text encodings**, and **100 active source-intelligence providers**.
- Completed work is summarized in [`CHANGELOG.md`](../CHANGELOG.md) and implemented history remains in Git.
- No additional 3.x capability or release-infrastructure migration milestone is active. Verified standalone self-update is available in the released 3.x line; release qualification and private deployment remain separate.
- Publication, installation, and private deployment are separate actions.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## Release engineering

The active publication flow is a protected, draft-first, single-writer pipeline. It authenticates and publishes the complete 22-asset set as an immutable GitHub Release before publishing matching MCPB metadata to the MCP Registry. Its first end-to-end public qualification was **v3.3.3**; ongoing release requirements and recovery safeguards are documented in [PUBLISHING.md](PUBLISHING.md). Version-specific completed work belongs in [CHANGELOG.md](../CHANGELOG.md).

## Next program: agent effectiveness (planning active; no release milestone)

The next goal is to make existing MCP tools easier for agents to select and use correctly, without weakening safety or compatibility. "R31" is a planning label, not a promised release version, new API, or authority to publish. The published 3.3.3 release remains unchanged.

### Evidence-first sequence and decision gates

1. **R31.1 — Baseline (first actionable work).** Inventory the 43-tool catalog, annotations, prompts and existing performance benchmarks. Prepare a versioned, reproducible corpus of synthetic/public-domain requests covering source navigation, mixed encodings, Markdown, reviewable edits, conflict recovery, denial and ambiguity. Every scenario needs the expected tool sequence, a legitimate outcome, prohibited side effects and scoring rules. Record exact source/client/model versions, repetitions, catalog bytes and tokenizer-specific token measurements, correct/incorrect tool choices, unnecessary calls and safety violations. Separate deterministic contract outcomes from variable model results. Record existing Source Intelligence benchmarks as ns/op and allocs/op; collect p50/p95 only through separately instrumented end-to-end latency runs. **Exit:** reproducible measurements and the highest-impact confirmed usability problem; no product change.
2. **R31.2 — Tool discovery and descriptions (first possible product slice).** Evaluate (A) improving existing tool titles, descriptions and examples without name/schema changes; (B) opt-in, startup-fixed tool profiles, only if A is insufficient and client compatibility can be proven; (C) session-dependent hidden/lazy tool registration. **Prefer A**: it reuses `internal/toolcatalog/catalog.json` and established runtime/catalog drift tests, and is reversible. B needs a separate compatibility/security decision with a stable catalog for each server instance. **Reject C for now**: it introduces discovery, authorization and compatibility ambiguity. Do not assume an MCP client implements server-controlled `tool_search` or replace narrow tools with a generic executor. **Exit:** repeated baseline comparison demonstrates improved correct selection with no safety or schema regression.
3. **R31.3 — Guided workflows.** Evaluate existing prompts before expanding the public surface. Exercise repository inspection → verified source context; file/Markdown edit → reviewable preview → explicit approval → apply → verify; and encoding diagnosis → safe conversion → verification. Preserve backup, one-shot authorization, conflict checks and truthful partial-state reporting. **Exit:** end-to-end positive and denied/stale/concurrent/partial cases are reproducible; no silent mutation.
4. **R31.4 — Source Intelligence accuracy (follow-on candidate).** Use a small bounded corpus of public repositories and synthetic malformed/ambiguous language files. Label expected declarations/relationships and unsupported cases. Measure precision/recall where labels exist, false positives, stale-context refusal, memory and latency. Favor correctness in the existing 100 providers over increasing the provider count. Reuse `internal/sourceintelligence` unit tests and benchmarks. **Exit:** grounded accuracy gain and negative regression coverage without invented semantic relationships.
5. **R31.5 — Performance/interoperability (follow-on candidate).** Profile source query, workspace traversal, Markdown, encoding, bounded output and deferred/durable observation at representative file sizes and concurrency. Compare cold/warm latency, allocations, resource budgets and transport behavior; optimize only verified hotspots. Validate annotations, tool schemas and stable `tools/list` behavior across representative clients. **Exit:** reproducible gain above measurement noise, no degraded resource or platform safety.
6. **R31.6 — Promotion decision.** Review goal metrics, reproducibility, negative security checks, API compatibility, platform/race coverage, documentation and exact-commit CI. Use a compatible 3.x release only if justified; reserve 4.0 for an explicitly approved breaking boundary. The existing immutable release procedure remains mandatory.

### Execution packages

| Package | Deliverable | Dependency | Completion evidence |
|---|---|---|---|
| P0 | Catalog/prompts inventory and fixed evaluation protocol | Ready now | Exact-revision scenario set, expected routes, denial cases, measurement definitions |
| P1 | Offline baseline harness and repeated measurements | P0 | Traces, tool-choice scoring, catalog footprint, existing performance baseline, variability |
| P2 | Minimal discoverability change (prefer metadata-only) | P1 | Focused tests and before/after client evaluation with stable public contracts |
| P3 | One guided workflow improvement | P1; independent of P2 when inputs do not overlap | Approval/preview/apply/recovery end-to-end negative and positive tests |
| P4 | Targeted Source Intelligence accuracy repair | P1 | Ground-truth comparisons, ambiguity tests, provider/benchmark regression |
| P5 | Measured performance correction | P1 identifies a real bottleneck | Repeatable benchmarks, bounds and platform/race regression |
| P6 | Release decision and qualification | A coherent verified product slice | Reviewed diff, promotion suite, exact-commit CI and publishing gate |

**Immediate frontier: P0 → P1 only.** Keep P2–P5 as contingent candidates until P1 demonstrates value. Do not preemptively change public tools, allowed roots, execution authorization, packaging or release scripts; at most one release-scoped milestone may be active.

### Evaluation safeguards and reuse

- Freeze a baseline, use identical before/after cases, disclose model variability and sample counts. Token counts must name the tokenizer; synthetic evaluations are not production telemetry. Zero unauthorized filesystem mutations is a non-negotiable safety requirement.
- Include path escape, symlink/junction, ambiguous encoding/BOM, oversized input, stale fingerprint, denied execution, missing backup, expired preview, concurrent edit, partial commit and unsupported relationship cases.
- Keep experiments confined to explicitly authorized test roots with no credentials or private user data. Maintain stable stdio/HTTP authority and all existing failure/resource contracts.
- Reuse `filetoolsserver` catalog/schema tests, `internal/sourceintelligence` benchmarks, `testarchitecture` CI evidence policies, [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md), and [PUBLISHING.md](PUBLISHING.md). Changes to test architecture or workflow files invoke their required full qualification tier.

### Deferred research and operational boundaries

R30's deferred Marksplice work remains deferred until its semantic authority supports it. MCP federation, visual desktop interaction, per-agent ACLs, automatic project-wide semantic refactoring and new 4.0 APIs are not approved implementation tasks: each requires an independent product, privacy, security, UX and compatibility decision.

Private installation state is separate from publication. Confirm actual executable version, standalone update adoption and pending transaction state before any private deployment; no launcher edit, restart or self-update is part of this planning track.

## Reserved 4.0 boundary

Version 4.0 is reserved for a deliberately reviewed major compatibility or capability boundary after the 3.x work. Its scope is not frozen.

Automatic project-wide semantic refactoring remains out of scope unless maintainers explicitly reopen that decision.

Research topics such as visual desktop interaction or MCP federation are not approved milestones or APIs. They require separate product, architecture, privacy, security, and UX review before implementation.
