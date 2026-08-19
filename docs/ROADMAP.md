# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and stable subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `3.1.6`**, published on 2026-08-19.
- Public surface: **36 tools**, **3 guided prompts**, **168 registered text encodings**, and **101 active source-intelligence providers** over stdio and Streamable HTTP.
- R1-R28 are complete. The pre-R29 test/build/CI architecture optimization is also complete and shipped in `3.1.5` without changing the public MCP surface.
- R29-R33 remain `PLANNED`. No release-scoped milestone is active until maintainers explicitly activate one.
- Publication and deployment are separate operator actions; public milestone state never implies a private runtime change.

## Operating rules

- At most one release-scoped milestone may be `ACTIVE` at a time.
- Define the user-visible outcome, compatibility boundary, security implications, and completion gate before implementation begins.
- Keep changes scoped to the active milestone unless maintainers explicitly reprioritize them.
- Preserve the established encoding, filesystem, mutation, backup, task, transport, and source-intelligence security boundaries unless a milestone explicitly changes a contract.
- Public MCP functions should remain compact, explicit about effects, clear to an LLM, and difficult to misuse.
- Public releases require an exact clean commit, a dated changelog entry, the full exact-SHA release-candidate gate, and the procedure in [PUBLISHING.md](PUBLISHING.md).
- Every milestone uses the reusable engineering checks in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md).
- Move completed implementation detail to the relevant contract or [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md); do not accumulate historical execution logs here.

## Completed milestones

R1-R28 are complete. Their concise outcomes and release checkpoints are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The most recent subsystem contracts are:

- R23 — [MCP mutation surface](MCP_MUTATION_SURFACE.md)
- R24 — [safe filesystem operations](SAFE_FILESYSTEM_OPERATIONS.md)
- R25 — [source-intelligence foundation](SOURCE_INTELLIGENCE.md)
- R26 — [backup recovery](BACKUP_RECOVERY.md)
- R27 — [broad multi-language code intelligence](MULTILANGUAGE_CODE_INTELLIGENCE.md)
- R28 — [engine hygiene](ENGINE_HYGIENE.md)

## Planned 3.x milestones

The intended planning order is R29 -> R30 -> R31 -> R32 -> R33. Version mapping may change before activation if scope changes materially; architectural boundaries require explicit review rather than being inferred from a version number.

### R29 — Logging and diagnostics lifecycle

Centralize process-wide server diagnostics while keeping server logs, HTTP/security access logging, and durable-task stdout/stderr (`task_logs`) distinct.

Logging policy remains startup/operator authority only. File logging must be storage-bounded through deterministic rotation, compression, retention, and aggregate-size limits. Multi-process ownership, redaction, permission/rename/compression failures, and cleanup must fail safely without corrupting normal product state.

Completion requires lifecycle/failure/concurrency tests, bounded-retention evidence, and security-redaction regressions.

### R30 — Documentation intelligence

Extend the existing Markdown/source-intelligence foundation into coherent documentation understanding: structure, anchors, local links/fragments, front matter, fenced code, references, and bounded document relationships where evidence is trustworthy.

Any document mutation must reuse verified preview/apply primitives and preserve encoding, BOM, line endings, backup, conflict, and partial-state guarantees. Do not introduce an independent document editor.

Completion requires deterministic malformed/ambiguous behavior, compact LLM-facing UX, cross-encoding coverage, and preview/apply regressions for mutating capabilities.

### R31 — Unified single/multi-file edit

Evolve editing toward one clear concept for one or many existing files, reusing one planner/capability model for exact preconditions, retained result bytes, backup preflight, deterministic apply, and truthful partial-state evidence.

This milestone does **not** authorize automatic semantic refactoring. Source changes remain explicit client-requested edits; removal of historical public concepts requires a separately reviewed compatibility boundary.

Completion requires single/multi-file equivalence, capability lifetime/replay, TOCTOU/conflict, encoding/BOM/EOL, backup, commit-order, crash/partial-commit, and connector-ergonomics evidence.

### R32 — Verified self-update

Add a failure-safe update lifecycle: discover, select the correct target asset, verify release/version/checksum identity, stage, retain a known-good binary, switch, verify, and roll back when necessary.

Installation state is separate from the persistent user-file backup store. Update failure must leave a usable installation or an explicit recoverable state, with platform-specific replacement semantics designed deliberately.

Completion requires tamper/mismatch rejection, interrupted-operation tests, rollback coverage, platform replacement tests, and post-switch version verification.

### R33 — Source-intelligence completion

Improve analysis quality, trustworthy missing relations, detector/provider accuracy, scale, indexing, query usefulness, and regression corpora without adding mutation authority to Source Intelligence.

Fail closed where evidence is insufficient. Callers, callees, overrides, or future relations remain unsupported until analyzers can prove them truthfully. Persistent on-disk indexing or external parser/compiler/LSP dependencies require separate architectural approval.

Completion requires provider/capability truthfulness, deterministic bounded behavior, scale evidence, and no automatic source transformation.

## Reserved 4.0 boundary

Version 4.0 is reserved for an intentionally reviewed major compatibility or capability boundary after the 3.x groundwork. Its scope is not frozen.

Automatic semantic refactoring/project-wide semantic transformation remains **out of scope** unless maintainers explicitly reopen that safety decision.

Two concepts remain research-only and are **not approved implementation milestones or APIs**:

- **Visual desktop interaction:** study observation-bound screen/window capture and bounded input, including stale-frame rules, focus/DPI/multi-monitor behavior, delayed UI transitions, privacy, OS permissions, and operator-controlled authorization.
- **MCP federation/gateway:** study optional hot-pluggable downstream MCP peers without merging remote catalogs into Scripthold's native catalog, including peer trust, authentication, schema changes, prompt-injection boundaries, reconnect behavior, retries, cancellation, loop prevention, and fault isolation.

Either concept requires a dedicated product, architecture, UX, privacy, and threat-model review before implementation is activated. Previous exploratory names or schemas are not commitments.

## Reprioritization rule

Urgent reliability or security work may preempt an active milestone, but the interruption, completion evidence, and resume point must be explicit. Completed milestones remain historical contracts; future release-scoped work starts only after explicit activation.
