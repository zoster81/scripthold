# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `3.0.0`**, published on 2026-08-17.
- Public surface: **36 tools**, **3 guided prompts**, **168 registered text encodings**, and **101 active source-intelligence providers** over stdio and Streamable HTTP.
- The `3.0.0` GitHub Release, GitHub-only MCPB publication, and MCP Registry publication are complete; publication remains separate from operator deployment.
- R21-R27 are complete, and R23-R27 shipped together in `3.0.0`.
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
| R28 | PLANNED | Engine hygiene: reduce proven complexity and duplication, retire justified legacy/deprecated paths, reorganize maintainability hotspots, and improve measured performance without changing behavior gratuitously. |
| R29 | PLANNED | Central logging and diagnostics lifecycle controlled only by startup/operator policy, including bounded file rotation, compression, retention, and storage limits. |
| R30 | PLANNED | Documentation intelligence built on existing source-intelligence and verified-mutation primitives, beginning with Markdown structure and explicit preview/apply document changes. |
| R31 | PLANNED | Unified single/multi-file edit UX for LLMs over one shared verified edit planner rather than overlapping mutation engines. |
| R32 | PLANNED | Verified self-update with asset/version verification, staging, known-good rollback, and failure-safe installation boundaries. |
| R33 | PLANNED | Source-intelligence completion focused on analysis quality, trustworthy missing relations, scale, detection, and query usefulness without automatic semantic refactoring. |

Detailed outcomes for R1–R27 are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The completed R23 contract is [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md); the completed R24 contract is [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md); the completed R25 contract is [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md); the completed R26 contract is [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md); and the completed R27 contract is [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md).

## Approved road to 4.0

R28-R33 define the approved **3.x planning sequence** after `3.0.0`. The intended minor-release mapping is R28 -> 3.1, R29 -> 3.2, R30 -> 3.3, R31 -> 3.4, R32 -> 3.5, and R33 -> 3.6. That mapping may be revised before activation if a milestone's scope materially changes, but the architectural order and boundaries below are the approved direction. No R28-R33 milestone is active until maintainers explicitly activate it.

A cross-cutting product rule for this sequence is that MCP functions must remain **clear to an LLM, compact in number, explicit about effects, and difficult to misuse**. Prefer one coherent capability over overlapping near-duplicate tools, and preserve read-only preparation plus approval-bound mutation where changes can affect user data.

### R28 — Engine hygiene

Refactor only where repository evidence justifies it. Reduce real complexity, dead code, unused state, redundant implementations, and historical organization that obscures current responsibilities. Review deprecated pre-R23 bridges, older compatibility paths, obsolete configuration fallbacks, and phase-named source-intelligence organization individually rather than deleting compatibility wholesale. Do not treat legacy text encodings as technical debt; broad encoding support remains a core product capability.

Performance work must be benchmark/profile driven. DRY refactors are appropriate only when the participating components truly share invariants; security, durability, encoding, and platform-specific boundaries must not be collapsed merely because code looks similar.

Completion requires preserved public behavior unless an explicitly documented compatibility change is approved, focused regression coverage for removed/merged paths, measured evidence for claimed performance improvements, complete applicable repository verification, and no unexplained legacy/deprecated surface remaining in the touched scope.

### R29 — Logging and diagnostics lifecycle

Centralize server logging behind one process-wide logging subsystem while keeping **server diagnostics, HTTP/security access logging, and durable-task stdout/stderr (`task_logs`) conceptually separate**.

Logging policy is operator authority only. Level, destination, rotation, compression, retention, and storage limits are configured at startup; no MCP tool may enable, disable, or reconfigure logging at runtime. The planned levels are `off`, `error`, `warn`, `info`, and `debug`.

File logging must be storage-bounded. The planned lifecycle is active log -> rotation -> compression -> retention -> deletion, with size-based rotation, compressed archives (gzip is the preferred initial format), age-based retention, and an aggregate archive-size ceiling so a pathological logging day cannot consume unbounded disk. Cleanup must operate only on Scripthold-owned log artifacts, must not follow path indirection outside the configured log location, and must degrade safely if rotation/compression/cleanup fails. Multi-process server/supervisor/worker topology must not introduce unsafe concurrent rotation or interleaved ownership.

Completion requires deterministic lifecycle tests, disk/permission/rename/compression failure coverage, concurrency/process-ownership tests, security-redaction regressions, bounded-retention evidence, and proof that logging failure cannot corrupt normal product state.

### R30 — Documentation intelligence

Extend the existing Markdown/source-intelligence foundation into coherent documentation understanding rather than a collection of narrow Markdown-specific tools. Initial read-only capabilities may include heading/section structure, anchors, local links/fragments, front matter, fenced code, table-of-contents structure, references, and bounded document relationships where evidence is trustworthy.

Document mutation must remain explicit and verified. Candidate operations include inserting/replacing/moving a section, regenerating a table of contents, updating front matter, or repairing local links after an explicitly requested move. They must reuse the existing verified edit/mutation path rather than create an independent filesystem editor, and must preserve encoding, BOM, line endings, exact preview/apply approval, backup, and partial-state guarantees.

Completion requires a compact LLM-oriented public surface, deterministic Markdown/document parsing boundaries, malformed/ambiguous input behavior, cross-encoding/line-ending coverage, and preview/apply regression gates for every mutating capability.

### R31 — Unified single/multi-file edit

Evolve editing toward **one clear edit concept covering one or many existing files**. Do not add a second independent multi-file mutation engine beside the existing `edit_file` and `patch_package` foundations. Reuse one planner/capability model for exact preconditions, retained result bytes, backup preflight, deterministic apply, and truthful partial-state evidence.

The target UX is conceptually `edit` -> `previewId` -> `edit_apply(previewId)`, with the preparation request able to contain one or multiple file edits. Exact public naming/schema is decided during R31 design, with compatibility preserved during the 3.x line. Removal of overlapping historical public edit concepts, if desirable, is reserved for an explicitly reviewed major-version compatibility boundary.

This milestone does **not** authorize semantic refactoring. Scripthold applies explicit edits requested by the client; it does not autonomously rename symbols, move classes/types, rewrite APIs, or infer project-wide transformations.

Completion requires single- and multi-file equivalence tests, capability lifetime/staleness/replay coverage, conflict/TOCTOU handling, encoding/BOM/EOL preservation, backup behavior, deterministic commit ordering, crash/partial-commit evidence, and connector acceptance proving the surface is simpler for an LLM without weakening mutation guarantees.

### R32 — Verified self-update

Evolve update checking into a failure-safe update lifecycle: discover -> select the correct OS/architecture asset -> download -> verify release/version/checksum identity -> stage -> retain a known-good previous binary -> switch -> verify -> rollback when necessary.

The updater must treat installation state as a separate authority from the persistent user-file backup store. It must never overwrite the only known-good executable before the candidate is verified, and platform-specific replacement semantics must be designed explicitly, especially for a running Windows executable. Update failure must leave a usable installation or an explicit recoverable state rather than silently producing a half-installed runtime.

Completion requires tampered/mismatched asset rejection, interrupted download/staging/switch tests, rollback tests, platform-specific replacement coverage, no secret leakage, and reproducible verification of the installed version after a successful switch.

### R33 — Source-intelligence completion

Continue improving source intelligence as **analysis and navigation**, not automatic project transformation. Candidate work includes currently unsupported relations only where analyzers can prove them truthfully, accuracy improvements, detector/provider hardening, large-repository/index performance, and query/context improvements that materially help an LLM understand source.

Fail closed where evidence is insufficient. `callers`, `callees`, `overrides`, or any future relation must remain unsupported rather than guessed until the relevant analyzers expose trustworthy facts. Persistent on-disk indexing, external parser/compiler/LSP dependencies, or other architectural changes require their own explicit decision rather than being introduced incidentally.

Completion requires provider/capability truthfulness, regression corpora, bounded resource/concurrency behavior, deterministic indexing/query results, and no mutation authority added to Source Intelligence.

## Reserved 4.0 boundary

Version 4.0 is reserved for an intentionally reviewed **major compatibility or capability boundary** after the 3.x groundwork. Its exact release scope is not frozen by this roadmap. In particular, the two research tracks below are promising directions but are **not approved implementation milestones**, are not required to ship together, and may move to later 4.x releases or be rejected after study.

Automatic semantic refactoring / project-wide semantic transformation is explicitly **out of scope** for the approved road to 4.0 because its failure modes can create broad, difficult-to-detect project damage. Future edits remain explicit client-requested changes unless maintainers deliberately reopen that architectural decision.

### Experimental research track — Visual desktop interaction

**Concept only. Mandatory product-philosophy, structural-architecture, UX, privacy, and threat-model review is required before any implementation milestone may be activated.** The discussion to date records research hypotheses, not a frozen API.

The concept is a visual interaction subsystem for screen/window/area capture plus bounded mouse/keyboard input. WebP is the preferred initial capture format. The core safety hypothesis is that an LLM acts on coordinates from an image it actually observed rather than calculating abstract desktop coordinates itself.

A capture target would have an expiring `screenshotId`/visual-session identity. Refreshing the same target should preserve that identity and renew its expiry while producing a newer observed frame/generation. Actions should be bound to the observed visual state, and the interaction loop should force fresh visual evidence after every action. Because UI effects may be delayed, a post-action image must not be interpreted as proof that the requested application operation has completed; observation/refresh semantics for asynchronous UI changes require dedicated design.

Research must resolve at least screenshot/frame identity, expiry and stale-action rules, screen/window/area ownership, focus binding, DPI/scaling and multi-monitor coordinates, delayed UI transitions, hover effects, keyboard text/shortcut semantics, privacy/redaction, local versus remote authorization, OS accessibility/permission models, Windows/macOS/Linux/Wayland capability differences, cancellation, and abuse boundaries. The feature must be disabled by default and any eventual activation policy must remain operator-controlled rather than LLM-controlled.

### Experimental research track — MCP federation / gateway

**Concept only. Mandatory product-philosophy, structural-architecture, protocol, UX, and threat-model review is required before any implementation milestone may be activated.** The discussed function names and schemas are preliminary and may change or be discarded.

The concept is for Scripthold to act as an MCP client toward operator-configured downstream MCP servers while remaining an MCP server toward its own client. Downstream peers are optional and hot-pluggable: their absence must not block Scripthold startup, they may appear/disappear repeatedly during normal use, and reconnection must not require restarting Scripthold. A local application such as a browser may therefore expose an MCP peer only while that application is running.

Remote/downstream catalogs must **not** be merged into Scripthold's native tool catalog. The current research model is a compact discovery/call surface such as `list_mcp` -> `mcp_functions` -> `mcp_call`: first learn configured-peer availability, then inspect the selected peer's current functions/schema, then invoke an explicitly selected function. Catalog generation/fingerprint concepts should be investigated so a call cannot silently target a schema different from the one the LLM inspected.

Research must resolve peer configuration/identity, authentication/TLS, allowlists and authority boundaries, downstream descriptions/schema as untrusted prompt-injection-bearing content, protocol-version compatibility, reconnect/backoff, catalog changes, cancellation/timeouts, non-idempotent calls with unknown execution outcome, retry rules, loop/hop prevention, fault isolation, logging/audit redaction, local-versus-remote peers, and whether process launching is ever in scope. A disconnected optional peer is a normal state, not a server failure.

## Latest completed milestones

R26 and R27 are complete. R26 added deterministic offline evidence-preserving backup-store recovery into a separate audited destination. R27 completed the broad native source-intelligence program: 101 active approved providers, registry-derived capability reporting, bounded project resolution and graph queries, fingerprint-verified task context, and coherent process-local incremental generations.

Detailed milestone outcomes belong in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The authoritative R26 and R27 contracts are [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md) and [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md), with the generated provider matrix in [LANGUAGE_CAPABILITIES.md](LANGUAGE_CAPABILITIES.md). No milestone after R27 is active; future release-scoped work requires explicit activation and a documented completion gate.

## Reprioritization rule

Urgent reliability or security work may temporarily preempt an active milestone, but the interruption, completion evidence, and resume point must be explicit. Completed milestones remain historical contracts; future release-scoped work starts only after maintainers explicitly activate a new milestone in this roadmap.
