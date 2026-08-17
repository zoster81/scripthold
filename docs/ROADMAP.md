# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `2.2.0`**.
- Public surface: **30 tools**, **3 guided prompts**, and **168 registered text encodings** over stdio and Streamable HTTP.
- R21-R27 are complete.
- **R27 completed on 2026-08-16.** Phases 0-18 are complete; no later release-scoped milestone is activated by this completion.
- The completed R24, R25, and R26 contracts and verification records are in [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md), [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md), and [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md).
- Publication does not imply deployment. Operator-specific deployment, rollback, and runtime state are governed separately by [PUBLISHING.md](PUBLISHING.md) and private operational procedures.

## Operating rules

- At most one release-scoped milestone may be `ACTIVE` at a time.
- Define the user-visible outcome, compatibility boundary, security implications, and completion gate before implementation begins.
- Keep changes scoped to the active milestone unless maintainers explicitly reprioritize them.
- Use content bytes and decoded-text evidence for encoding detection; filenames and extensions never select an encoding.
- Preserve stdio behavior while transport work is in progress.
- Keep both `task_run` execution kinds disabled by default on every transport.
- Public releases require an exact clean commit, a dated changelog entry, the complete release-candidate gate, and the immutable publication procedure in [PUBLISHING.md](PUBLISHING.md).
- Every milestone uses the reusable engineering checks in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md).
- Completed implementation detail moves to the appropriate design/history document instead of accumulating indefinitely in this roadmap.

## Milestone overview

| Milestone | Status | Outcome |
|---|---|---|
| R1 | COMPLETE | Shared encoding/BOM-aware text-document core. |
| R2 | COMPLETE | Secure deterministic recursive traversal across Unix and Windows path semantics. |
| R3 | COMPLETE | Durable staged mutation layer with conflict detection and rollback-aware backup handling. |
| R4 | COMPLETE | Transport-independent typed operation errors and stable public error categories. |
| R5 | COMPLETE | Bounded ordered concurrency for batch and search operations. |
| R6 | COMPLETE | Shared execution preparation and authoritative tool metadata. |
| R7 | COMPLETE | Documentation, roadmap, and contributor-workflow reset with private/public state separation. |
| R8 | COMPLETE | Conservative content-based encoding detection independent of filenames. |
| R9 | COMPLETE | Bounded-memory streaming for large text operations and explicit full-document limits. |
| R10 | COMPLETE | 2.0 public API and compatibility cleanup. |
| R11 | COMPLETE | Transport-independent server construction and process-wide root policy. |
| R12 | COMPLETE | Approved Streamable HTTP threat model and fail-closed security contract. |
| R13 | COMPLETE | Native authenticated Streamable HTTP while preserving stdio. |
| R14 | COMPLETE | Hardening, publication, deployment verification, rollback, and restoration for `2.0.0`. |
| R15 | COMPLETE | Agent-oriented read/search/edit workflows and three guided encoding prompts. |
| R16 | COMPLETE | Deterministic fingerprints, one-shot edit approval, patch packages, and structured verification. |
| R17 | COMPLETE | Approved persistent-backup lifecycle and security design. |
| R18 | COMPLETE | Persistent backup capture, review, restore, audit, and explicit garbage collection. |
| R19 | COMPLETE | Mutation-free offline diagnostics for an existing backup store. |
| R20 | COMPLETE | MCP `2026-07-28` adoption with retained legacy behavior and shared security boundaries. |
| R21 | COMPLETE | Durable asynchronous task execution and exact-commit release gating in the `2.1.x` line. |
| R22 | COMPLETE | Global portable encoding coverage, full UTF-32 support, detector hardening, and published `2.2.0`. |
| R23 | COMPLETE | Separate read-only preparation from mutation, preserve capability-bound preview/apply, and improve persistent-backup history/usability. |
| R24 | COMPLETE | Typed safe filesystem packages for coordinated create/copy/move/delete and directory operations without arbitrary shell fallback, verified through connector acceptance and native Windows/Linux/macOS CI. |
| R25 | COMPLETE | Native language-neutral source navigation with evidence-qualified analysis, shared scanner/detector/composite architecture, Go/C#/VB.NET/Python canaries, and Classic ASP segmentation. |
| R26 | COMPLETE | Offline evidence-preserving backup-store recovery/salvage through deterministic persisted plan/apply into a separate fully audited destination. |
| R27 | COMPLETE | Broad native multi-language/source-format intelligence with the approved expanded catalog, evidence-qualified project relations, structural search, bounded context, graphs, and incremental indexing. |

Detailed outcomes for R1–R27 are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The completed R23 contract is [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md); the completed R24 contract is [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md); the completed R25 contract is [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md); the completed R26 contract is [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md); and the completed R27 contract is [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md).

## Latest completed milestones

R26 and R27 are complete. R26 added deterministic offline evidence-preserving backup-store recovery into a separate audited destination. R27 completed the broad native source-intelligence program: 101 active approved providers, registry-derived capability reporting, bounded project resolution and graph queries, fingerprint-verified task context, and coherent process-local incremental generations.

Detailed milestone outcomes belong in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The authoritative R26 and R27 contracts are [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md) and [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md), with the generated provider matrix in [LANGUAGE_CAPABILITIES.md](LANGUAGE_CAPABILITIES.md). No milestone after R27 is active; future release-scoped work requires explicit activation and a documented completion gate.

## Reprioritization rule

Urgent reliability or security work may temporarily preempt an active milestone, but the interruption, completion evidence, and resume point must be explicit. Completed milestones remain historical contracts; future release-scoped work starts only after maintainers explicitly activate a new milestone in this roadmap.
