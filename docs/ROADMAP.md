# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `2.2.0`**.
- Public surface: **30 tools**, **3 guided prompts**, and **168 registered text encodings** over stdio and Streamable HTTP.
- R21-R26 are complete.
- **R27 is `ACTIVE`.** The explicitly reprioritized connector reliability gate completed on 2026-08-15 and R27 implementation resumed. Phases 0-8 are complete; Phase 9 scientific, legacy and functional breadth is the first incomplete R27 implementation phase.
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
| R27 | ACTIVE | Broad native multi-language/source-format intelligence with the approved expanded catalog, evidence-qualified project relations, structural search, bounded context, graphs, and incremental indexing. |

Detailed outcomes for R1–R26 are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The completed R23 contract is [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md); the completed R24 contract is [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md); the completed R25 contract is [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md); the completed R26 contract is [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md); and the active R27 baseline is [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md).

## Latest completed and active milestones

### R26 — Backup recovery

The completed contract and verification record are in [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md). R26 completed on 2026-08-14 with offline evidence-preserving recovery/salvage beyond R19 diagnosis while keeping normal startup fail closed and R19 mutation-free. The workflow uses explicit `backup-store recover-plan` / `recover-apply` commands, a persisted deterministic plan that is fully recomputed under lock before apply, an immutable source store, a new staged destination with fresh store identity, fully verified objects, manifest identity preserved except destination `StoreID`/checksum, rebuilt derived state, mandatory full audit, and a path-free provenance sidecar. The exact pushed implementation commit passed native Windows/Ubuntu/macOS CI plus the aggregate `Release candidate` gate. Adoption/deployment remains separate.

### R27 — Broad multi-language code intelligence

The approved baseline and complete sequential handoff are [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md). R27 is explicitly **not Go-centric** and extends the native R25 architecture rather than introducing a second parser/runtime stack. C, C++, C#, Java, Kotlin, JavaScript, TypeScript, Python, Rust, Go, PHP, Ruby, Swift, and Pascal/Object Pascal/Delphi remain non-negotiable minimum production gates, but they are not the product horizon: the approved catalog also covers the documented Basic/.NET, Classic ASP/composite, MQL4/MQL5, modern, legacy, scientific, functional, scripting, hardware, data/infra and document/template families. Phase 1 froze `source_query` as the compact additive read-only search/relations/context contract. Phase 2 completed the reusable profile-driven scanner/recognizer foundation and established the mechanically checked registry-derived [language capability matrix](LANGUAGE_CAPABILITIES.md). Phase 3 activated C, C++, Java, and Kotlin as production declaration/navigation providers with structural includes/imports and inheritance/supertype evidence while explicitly withholding macro/classpath/build/type/semantic claims. Phase 4 activated JavaScript/JSX, TypeScript/TSX, and Rust with literal module dependencies, dynamic-language/type declarations, Rust trait/impl structure, conservative regex/raw-string/macro boundaries, and structural-only relationship evidence while withholding dynamic/type/Cargo/project/semantic claims. Phase 5 activated PHP, Ruby, Swift, Pascal, and Delphi as distinct production providers, including literal dynamic-language dependencies, structural reopen/extension/type relationships, Pascal/Delphi sections/forwards/nested routines/generics/helpers, ambiguity-safe legacy routing, and legacy decoded-source coverage while withholding runtime/compiler/project/semantic claims. Phase 6 added the distinct Basic/.NET/composite providers for VB6, VBA, VBScript, QBasic, classic BASIC, FreeBASIC, PureBasic, F#, C++/CLI, JScript.NET, CIL, PowerShell, ASP.NET Web Forms, Razor, Blazor, and XAML while extending Classic ASP to VBScript/JScript delegation. Phase 7 added distinct MQL4/MQL5, Objective-C/Objective-C++, Dart, D, Zig, Nim, Solidity, Apex, AL, and Arduino providers while preserving `.mqh` and Objective-C `.m` ambiguity boundaries. Phase 8 added distinct Perl, Lua, Luau, Elixir, Erlang, Gleam, Groovy, POSIX shell, Bash, Tcl, and AutoHotkey providers, raising the generated matrix to 56 active production rows with explicit dynamic/metaprogramming limits, hardened opaque-data boundaries, static dependency extraction, and conservative routing. Planned entries remain unimplemented until their provider waves. R27 additionally requires cross-ecosystem resolved project relationships, bounded graph operations, and fingerprint/coherent-generation incremental indexing.

## Reprioritization rule

R25 and R26 are complete. R27 was explicitly activated on 2026-08-14. Maintainers temporarily reprioritized implementation on 2026-08-15 for a high-priority connector reliability hardening gate; that gate completed and R27 resumed without changing the approved milestone scope. Phases 0-8 are now complete and Phase 9 scientific, legacy and functional breadth is the first incomplete phase.
