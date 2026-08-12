# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `2.2.0`**.
- Public surface: **30 tools**, **3 guided prompts**, and **168 registered text encodings** over stdio and Streamable HTTP.
- R21-R23 are complete.
- No release-scoped milestone is currently `ACTIVE`.
- R24-R27 are approved future milestones and remain `PLANNED`; implementation begins only after explicit activation or reprioritization.
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
| R24 | PLANNED | Typed safe filesystem packages for coordinated create/copy/move/delete and directory operations without arbitrary shell fallback. |
| R25 | PLANNED | Language-neutral source-symbol architecture and initial production-quality parser providers, using native Go AST as the first reference implementation rather than the final language scope. |
| R26 | PLANNED | Separately reviewed offline backup-store repair/salvage beyond the current diagnostic-only surface. |
| R27 | PLANNED | Broad multi-language code intelligence with production-quality symbol providers plus references, implementations, dependency/call relationships, and incremental indexing. |

Detailed outcomes for R1–R23 are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The completed R23 contract is [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md); approved future design baselines are [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md) (R24), [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md) (R25), [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md) (R26), and [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md) (R27).

## Latest completed milestone — R23: MCP mutation surface and backup UX

R23 completed on 2026-08-12 under the contract in [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md). It delivered:

- truthful separation of the five historical mixed preparation/review names from six dedicated mutating apply tools;
- one-shot `previewId`-only apply schemas that cannot resubmit path, content, encoding, permission, or backup-policy intent;
- exact-result binding and revalidation across edit, patch-package, BOM, encoding conversion, restore, and backup GC flows;
- persistent-backup history/comparison plus the operator default `MCP_BACKUP_DEFAULT_POLICY=disabled|required`;
- backup-before-staging ordering for package/restore safety-sensitive paths and exact-byte BOM/encoding capabilities;
- registry-driven mutation-integrity verification across all 168 encodings plus normal/race/static/vulnerability/cross-platform gates;
- connector-level acceptance confirming the separated schemas and side-effect-free edit preview, exact apply, replay rejection, and rejection of the removed direct-edit form.

The 36-tool R23 source tree remains an Unreleased next-major candidate; Scripthold `2.2.0` remains the current public release. Publication, tagging, and deployment are separate governed actions. R24-R27 remain `PLANNED`, and none becomes active automatically.

## Approved future milestones

### R24 — Safe filesystem operations

The approved baseline is [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md). R24 must eliminate routine shell/script fallback for common workspace namespace changes through a strict typed preview/apply package. Its required scope includes coordinated create/copy/move/rename/delete operations, **recursive real-directory copy and delete**, deterministic destructive-scope enumeration, source/destination identity and fingerprint binding, required persistent backup before irreversible content loss, durable staging, deterministic commit ordering, and truthful partial-state evidence. It must never accept arbitrary command strings or claim unsupported whole-package atomicity/rollback.

### R25 — Source intelligence foundation

The approved baseline is [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md). R25 must introduce a read-only language-neutral `source_symbols`-class capability with structured declarations, hierarchy/ownership, qualified names, bounded signatures, source ranges, explicit partial coverage, deterministic traversal, and strict resource limits. The internal provider interface must be designed for non-Go constructs from the beginning. Native Go `go/parser`/`go/ast`/`go/token` is only the first reference provider; regex pseudo-parsing is forbidden as an unsupported-language fallback.

### R26 — Backup recovery

The approved baseline is [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md). R26 must add an offline evidence-preserving recovery/salvage workflow beyond R19 diagnosis while keeping normal startup fail closed and R19 mutation-free. The source store remains immutable evidence during planning and recovery; the baseline recovery path reconstructs only fully verified authoritative records into a separate destination, never fabricates missing bytes/metadata, never promotes orphan objects into backups without trustworthy manifests, requires explicit stale-plan validation and a final full audit, and leaves adoption/deployment of the recovered store as a separate operator action.

### R27 — Broad multi-language code intelligence

The approved baseline is [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md). R27 is explicitly **not Go-centric**. Production-quality declaration/symbol coverage is mandatory for C, C++, C#, Java, Kotlin, JavaScript, TypeScript, Python, Rust, Go, PHP, Ruby, Swift, and Pascal/Object Pascal/Delphi. The common capability model must distinguish declarations, structural relationships, semantic references/definitions, implementations/overrides, dependencies, call relationships, and incremental-index support. Advanced semantic intelligence must work across several distinct language ecosystems rather than only Go; unsupported semantics must remain explicit instead of being approximated by regex/name matching. Pascal/Delphi is a mandatory baseline family with units, interface/implementation sections, classes/records/interfaces, procedures/functions/methods, properties, `uses`, overloads, and representative legacy-encoding coverage.

## Reprioritization rule

R24-R27 are approved roadmap items, not active implementation authorization. At most one release-scoped milestone remains `ACTIVE`; changing that order requires an explicit maintainer reprioritization and corresponding roadmap update.
