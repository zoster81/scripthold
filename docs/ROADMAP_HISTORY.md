# Scripthold Engineering Milestone History

This document is the concise public history of completed milestones. Current/future state belongs in [ROADMAP.md](ROADMAP.md), user-visible release changes in [CHANGELOG.md](../CHANGELOG.md), and detailed technical contracts in the linked subsystem documents. Git history and GitHub Actions retain execution-level provenance; this file intentionally does not duplicate session logs, run IDs, local runtime state, or exhaustive test output.

## Completed milestones

| Milestone | Outcome |
|---|---|
| R1 | Unified encoding/BOM/line-ending aware text-document core. |
| R2 | Secure deterministic recursive traversal with Unix/Windows link, reparse, alias, and missing-ancestor enforcement. |
| R3 | Durable staged mutation with conflict detection, platform commit primitives, cleanup, and rollback-aware backup handling. |
| R4 | Stable transport-independent typed operation errors. |
| R5 | Bounded ordered concurrency for batch/search work. |
| R6 | Shared execution preparation plus authoritative embedded tool metadata. |
| R7 | Separated public roadmap/history/contributor guidance from private operator state. |
| R8 | Conservative content-based encoding detection independent of filenames. |
| R9 | Bounded-memory streaming text operations and explicit full-document limits. |
| R10 | 2.0 public API/compatibility cleanup; migration is documented in [MIGRATION_2.0.md](MIGRATION_2.0.md). |
| R11 | Transport-independent server construction and process-wide root policy. |
| R12 | Fail-closed Streamable HTTP threat model in [HTTP_SECURITY.md](HTTP_SECURITY.md). |
| R13 | Native authenticated Streamable HTTP while preserving stdio. |
| R14 | 2.0 hardening, publication, deployment verification, rollback, and restoration. |
| R15 | Agent-oriented read/search/edit ergonomics and guided encoding workflows. |
| R16 | Deterministic fingerprints, one-shot edit approval, patch packages, and typed verification; see [VERIFIED_CHANGE_WORKFLOWS.md](VERIFIED_CHANGE_WORKFLOWS.md). |
| R17 | Persistent-backup lifecycle/security design. |
| R18 | Persistent backup capture, review, restore, audit, pinning, and explicit garbage collection; see [PERSISTENT_BACKUP_LIFECYCLE.md](PERSISTENT_BACKUP_LIFECYCLE.md). |
| R19 | Mutation-free offline backup-store diagnostics; see [OFFLINE_BACKUP_DIAGNOSTICS.md](OFFLINE_BACKUP_DIAGNOSTICS.md). |
| R20 | MCP `2026-07-28` adoption with retained legacy behavior and shared security boundaries; see [MCP_2026_07_28_ADOPTION.md](MCP_2026_07_28_ADOPTION.md). |
| R21 | Durable asynchronous task execution plus exact-commit release gating; see [DURABLE_TASKS.md](DURABLE_TASKS.md). |
| R22 | 168 portable read/write encodings, full UTF-32 LE/BE support, and detector hardening; see [GLOBAL_ENCODING_COVERAGE.md](GLOBAL_ENCODING_COVERAGE.md). |
| R23 | Truthful read-only preparation vs mutation, `previewId`-only apply authority, and backup UX; see [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md) and [MIGRATION_3.0.md](MIGRATION_3.0.md). |
| R24 | Typed safe filesystem packages for bounded create/copy/move/delete/directory operations with backup-before-loss and truthful partial-state evidence; see [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md). |
| R25 | Native language-neutral `source_symbols` foundation with evidence-qualified detection, decoded coordinates, composites, and initial Go/C#/VB.NET/Python/Classic ASP providers; see [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md). |
| R26 | Offline evidence-preserving backup recovery into a separate fully audited destination; see [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md). |
| R27 | Broad native source intelligence: 101 active providers, `source_query`, project relations, structural search, bounded context, graphs, and coherent process-local index generations; see [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md) and [LANGUAGE_CAPABILITIES.md](LANGUAGE_CAPABILITIES.md). |
| R28 | Evidence-driven engine hygiene: compatibility consolidation, dead/test-only cleanup, responsibility-oriented source organization, and measured performance review without gratuitous public behavior changes; see [ENGINE_HYGIENE.md](ENGINE_HYGIENE.md). |

## Release checkpoints

| Release | Date | Milestone significance |
|---|---|---|
| `2.0.0` | 2026-08-02 | Published the R8-R14 architecture: conservative detection, bounded streaming, 2.0 API cleanup, shared server construction, and authenticated Streamable HTTP. |
| `2.1.1` | 2026-08-10 | Consolidated the R15-R21 line around durable tasks, verified changes/backups, MCP compatibility, and exact-SHA publication gating. |
| `2.2.0` | 2026-08-11 | Published R22 global encoding coverage and full UTF-32 text support. |
| `3.0.0` | 2026-08-17 | Published R23-R27: separated mutation authority, safe filesystem packages, backup recovery, and the completed 101-provider source-intelligence surface. |
| `3.1.0` | 2026-08-18 | Published R28 engine hygiene without changing the public MCP surface. |
| `3.1.5` | 2026-08-19 | Published the completed pre-R29 test/build/CI architecture redesign and qualification hardening, including Unicode-safe composite source offsets, again without changing the public MCP surface. |

Publication and private deployment remain separate operations. Exact commits, workflow runs, asset checksums, and release-by-release user-visible details remain available through Git, GitHub Releases/Actions, and [CHANGELOG.md](../CHANGELOG.md) rather than being duplicated here.
