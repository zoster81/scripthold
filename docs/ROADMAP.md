# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and stable subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `3.1.6`**, published on 2026-08-19.
- Public `3.1.6` surface: **36 tools**, **3 guided prompts**, **168 registered text encodings**, and **101 active source-intelligence providers** over stdio and Streamable HTTP.
- Current unreleased source exposes **38 tools**; maintenance added the explicit `backup_delete` capability while the published `3.1.6` surface remains unchanged.
- R1-R29 are complete. The pre-R29 test/build/CI architecture optimization is also complete and shipped in `3.1.5` without changing the public MCP surface.
- R30-R33 remain `PLANNED`. No release-scoped milestone is active until maintainers explicitly activate one.
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

## Completed maintenance gate - connector reliability

Connector reliability maintenance is complete in current unreleased source without activating a release-scoped milestone. The completed work addresses two independent request-path failure classes: synchronous tool calls that approach an external transport TTL, and completed MCP responses large enough to be rejected by an intermediary before delivery.

Current unreleased source applies a bounded cooperative synchronous deadline to every MCP tool call and retains oversized completed results behind bounded `deferred_operation` retrieval. When an operator configures the separate deferred-operation store, `fingerprint_paths`, `grep_text_files`, `search_files`, `tree`, and `source_symbols` are durably admitted before expensive execution and owned independently from the frontend request. Recovery may relaunch only work that never crossed the durable started boundary; a lost executor after that boundary becomes `interrupted` without automatic replay. Current filesystem roots are revalidated before recovered execution and result retrieval.

`source_query` remains synchronous because its returned index binding is process-local and must remain usable by follow-up requests. User-file mutations, preview/capability-producing operations, and the existing durable task API are excluded from automatic deferral. Native negotiated `io.modelcontextprotocol/tasks` support is implemented over the same Deferred Operation Engine, including `tools/call` task results, `tasks/get`, `tasks/update`, and `tasks/cancel`. Qualification covers multi-client mixed traffic, reconnect, durable-store restart retrieval, deliberately over-soft-window concurrency with an unaffected fast path, real-repository source execution, coherent state/marker publication, cancellable store-lock observation, affected race suites, and supported-target cross-compilation. Build, deployment, publication, and live connector qualification remain separate operator/release actions rather than source-completion criteria.

Source Intelligence real-world requalification is no longer blocked by this maintenance gate.

## Completed milestones

R1-R29 are complete. Their concise outcomes and release checkpoints are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md). The most recent subsystem contracts are:

- R23 — [MCP mutation surface](MCP_MUTATION_SURFACE.md)
- R24 — [safe filesystem operations](SAFE_FILESYSTEM_OPERATIONS.md)
- R25 — [source-intelligence foundation](SOURCE_INTELLIGENCE.md)
- R26 — [backup recovery](BACKUP_RECOVERY.md)
- R27 — [broad multi-language code intelligence](MULTILANGUAGE_CODE_INTELLIGENCE.md)
- R28 — [engine hygiene](ENGINE_HYGIENE.md)
- R29 — [logging and diagnostics lifecycle](LOGGING_DIAGNOSTICS.md)

## Planned 3.x milestones

The intended planning order is R30 -> R31 -> R32 -> R33. Version mapping may change before activation if scope changes materially; architectural boundaries require explicit review rather than being inferred from a version number.

### R30 — Documentation intelligence

Extend the existing Markdown/source-intelligence foundation into coherent documentation understanding: structure, anchors, local links/fragments, front matter, fenced code, references, and bounded document relationships where evidence is trustworthy.

Document mutation must use the dedicated **Marksplice Go module** for Markdown-aware transformation and integrate it behind Scripthold's verified preview/apply primitives. Preserve encoding, BOM, line endings, backup, conflict, and partial-state guarantees. Do not restore the discarded in-repository Markdown mutation prototype or introduce a second document editor.

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
