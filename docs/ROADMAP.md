# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `2.2.0`**.
- Public surface: **30 tools**, **3 guided prompts**, and **168 registered text encodings** over stdio and Streamable HTTP.
- R21 and R22 are complete.
- **R23 is ACTIVE:** truthful MCP mutation boundaries and backup UX, governed by [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md).
- R24-R27 are approved future milestones and remain `PLANNED`; implementation must not begin before the active milestone is completed or explicitly reprioritized.
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
| R23 | **ACTIVE** | Separate read-only preparation from mutation, preserve capability-bound preview/apply, and improve persistent-backup history/usability. |
| R24 | PLANNED | Typed safe filesystem packages for coordinated create/copy/move/delete and directory operations without arbitrary shell fallback. |
| R25 | PLANNED | Language-neutral source-symbol architecture and initial production-quality parser providers, using native Go AST as the first reference implementation rather than the final language scope. |
| R26 | PLANNED | Separately reviewed offline backup-store repair/salvage beyond the current diagnostic-only surface. |
| R27 | PLANNED | Broad multi-language code intelligence with production-quality symbol providers plus references, implementations, dependency/call relationships, and incremental indexing. |

Detailed outcomes for R1–R22 are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). Approved design baselines for current/future work are [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md) (R23), [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md) (R24), [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md) (R25), [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md) (R26), and [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md) (R27).

## Latest completed milestone — R22

R22 delivered the release-scoped encoding expansion documented in [GLOBAL_ENCODING_COVERAGE.md](GLOBAL_ENCODING_COVERAGE.md):

- 168 canonical read/write encodings under one authoritative capability registry;
- full UTF-32 LE/BE support across the text-operation pipeline;
- the complete applicable repository-pinned `golang.org/x/text` surface plus verified pure-Go mappings and stateful/multibyte codecs derived from pinned GNU libiconv evidence;
- conservative automatic detection with explicit ambiguity when evidence is insufficient;
- bounded visible partial-failure reporting for grep and batch operations;
- registry-driven public-operation coverage, adversarial/resource verification, deterministic release packaging, and the complete cross-platform release-candidate gate.

Scripthold `2.2.0` was published on 2026-08-11 through the exact-commit workflow in [PUBLISHING.md](PUBLISHING.md). Actual MCPB bundles, their checksum manifest, the final MCPB-backed Registry manifest, and Registry publication were produced only by GitHub after tagging.

## Active milestone — R23: MCP mutation surface and backup UX

R23 addresses a concrete interoperability and safety problem: several current tools combine read-only preview/inspection actions with mutating actions, while MCP annotations are static for the whole tool. A host can therefore classify harmless preparation as destructive and encourage shell/script fallback that bypasses Scripthold's safer mutation pipeline.

The approved design in [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md) requires:

- distinct public capability boundaries for read-only preparation and actual mutation;
- truthful static MCP annotations rather than weakening metadata to avoid approval;
- one-shot `previewId` binding: preview returns the exact proposed result and apply accepts only the identifier;
- conflict on stale identity/fingerprint/state instead of recomputing a different mutation;
- retained encoding, BOM, line-ending, path-security, durable staging, backup, and verification guarantees;
- explicit backup version-history/comparison UX plus an operator-configurable default persistent-backup policy for eligible approved mutations;
- an explicit compatibility/migration decision for legacy mixed tools and direct editing before implementation changes public schemas;
- connector smoke verification that read-only previews are no longer classified as destructive solely because an apply capability exists.

R23 must not claim that true apply operations will bypass legitimate client approval, and it must not solve classification problems by marking mutating tools as read-only.

### R23 completion gate

R23 remains `ACTIVE` until the design's compatibility decision is finalized, all in-scope mixed read/write surfaces have truthful boundaries or justified exceptions, backup UX changes are implemented, catalog/runtime/docs remain synchronized, and focused plus repository-wide verification passes at the level required by [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md).

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
