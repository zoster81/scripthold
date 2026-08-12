# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `2.2.0`**.
- Public surface: **30 tools**, **3 guided prompts**, and **168 registered text encodings** over stdio and Streamable HTTP.
- R21-R23 are complete.
- R24 is `ACTIVE`: the source changes are implemented and all available local gates pass, but required native Linux/macOS namespace tests and connector-level preview/apply acceptance against an activated R24 candidate remain pending. R25-R27 remain `PLANNED` and must not be implemented incidentally.
- The authoritative R24 contract and Phase 0–12 implementation/gate record are in [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md).
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
| R24 | ACTIVE | Typed safe filesystem packages for coordinated create/copy/move/delete and directory operations without arbitrary shell fallback; final native Linux/macOS and activated-candidate connector verification pending. |
| R25 | PLANNED | Language-neutral source-symbol architecture and initial production-quality parser providers, using native Go AST as the first reference implementation rather than the final language scope. |
| R26 | PLANNED | Separately reviewed offline backup-store repair/salvage beyond the current diagnostic-only surface. |
| R27 | PLANNED | Broad multi-language code intelligence with production-quality symbol providers plus references, implementations, dependency/call relationships, and incremental indexing. |

Detailed outcomes for R1–R23 are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The completed R23 contract is [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md); the active R24 contract is [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md). Approved future design baselines are [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md) (R25), [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md) (R26), and [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md) (R27).

## Active milestone — R24: Safe filesystem operations

R24 implementation reached its final verification gate on 2026-08-12 under the contract in [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md). The implemented source delivers:

- read-only `filesystem_package` plus `previewId`-only `filesystem_package_apply`, replacing the four overlapping public `create_directory`, `copy_file`, `move_file`, and `delete_file` tools without compatibility aliases;
- strict `filesystem-package-v1` with `mkdir`, raw-byte `createFile`, `copyFile`, `copyDirectory`, native same-volume `move`, `deleteFile`, and exact-scope `deleteDirectory`, all no-replace;
- bounded exact recursive enumeration that includes hidden names and `.git`, ignores `.gitignore`, rejects link-like/special/nested-volume entries, and fails rather than truncating mutation scope;
- path, parent, object, content/tree, authorization, and missing-destination evidence retained in one-shot bounded capabilities and fully revalidated before mutation;
- mandatory persistent backup admission and verified capture before irreversible deletion of existing regular-file bytes, with all feasible creation/copy staging completed before the first target commit;
- deterministic manifest-order commits, native cross-volume rejection through `UNSUPPORTED`, and bounded `PARTIAL_COMMIT` evidence without shell fallback, copy-delete move emulation, or automatic rollback claims;
- focused adversarial/failure-injection coverage, in-process MCP preview/apply smoke, Windows-native tests, full normal and race suites, static/vulnerability checks, source smoke, and six-target Windows/Linux/macOS compilation.

The active R24 source tree exposes a 34-tool Unreleased next-major surface; Scripthold `2.2.0` remains the current public release. R24 is not `COMPLETE` until the required actual namespace suite also passes natively on Linux and macOS and connector-level preview/apply acceptance passes against an activated R24 candidate; cross-compilation and in-process MCP tests do not substitute for those gates. Publication, tagging, deployment, and runtime changes remain separate governed actions. R25-R27 remain `PLANNED`.

## Approved future milestones

### R25 — Source intelligence foundation

The approved baseline is [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md). R25 must introduce a read-only language-neutral `source_symbols`-class capability with structured declarations, hierarchy/ownership, qualified names, bounded signatures, source ranges, explicit partial coverage, deterministic traversal, and strict resource limits. The internal provider interface must be designed for non-Go constructs from the beginning. Native Go `go/parser`/`go/ast`/`go/token` is only the first reference provider; regex pseudo-parsing is forbidden as an unsupported-language fallback.

### R26 — Backup recovery

The approved baseline is [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md). R26 must add an offline evidence-preserving recovery/salvage workflow beyond R19 diagnosis while keeping normal startup fail closed and R19 mutation-free. The source store remains immutable evidence during planning and recovery; the baseline recovery path reconstructs only fully verified authoritative records into a separate destination, never fabricates missing bytes/metadata, never promotes orphan objects into backups without trustworthy manifests, requires explicit stale-plan validation and a final full audit, and leaves adoption/deployment of the recovered store as a separate operator action.

### R27 — Broad multi-language code intelligence

The approved baseline is [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md). R27 is explicitly **not Go-centric**. Production-quality declaration/symbol coverage is mandatory for C, C++, C#, Java, Kotlin, JavaScript, TypeScript, Python, Rust, Go, PHP, Ruby, Swift, and Pascal/Object Pascal/Delphi. The common capability model must distinguish declarations, structural relationships, semantic references/definitions, implementations/overrides, dependencies, call relationships, and incremental-index support. Advanced semantic intelligence must work across several distinct language ecosystems rather than only Go; unsupported semantics must remain explicit instead of being approximated by regex/name matching. Pascal/Delphi is a mandatory baseline family with units, interface/implementation sections, classes/records/interfaces, procedures/functions/methods, properties, `uses`, overloads, and representative legacy-encoding coverage.

## Reprioritization rule

R24 is the only active milestone. R25-R27 are approved roadmap items, not active implementation authorization; activation or reprioritization requires an explicit maintainer decision and corresponding roadmap update.
