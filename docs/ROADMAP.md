# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Current public release: **Scripthold 3.3.6**, published through the protected immutable GitHub release process.
- Current public capabilities: **43 tools**, **3 guided prompts**, **168 registered text encodings**, and **100 active source-intelligence providers**.
- Completed work is summarized in [`CHANGELOG.md`](../CHANGELOG.md) and implemented history remains in Git.
- The next approved development milestone is **4.0: unified confirmation of prepared operations**. The 4.0 development tree now contains the breaking 36-tool catalog and unified dispatcher; this remains unshipped pending full qualification. See [4.0 implementation plan](#40-unified-confirmation-milestone) below. Publication and private deployment remain separate actions; release 3.3.6 does not automatically upgrade existing installations.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## Release engineering

The active publication flow is a protected, draft-first, single-writer pipeline. It authenticates and publishes the complete 22-asset set as an immutable GitHub Release before publishing matching MCPB metadata to the MCP Registry. Its first end-to-end public qualification was **v3.3.3**; ongoing release requirements and recovery safeguards are documented in [PUBLISHING.md](PUBLISHING.md). Version-specific completed work belongs in [CHANGELOG.md](../CHANGELOG.md).

## 4.0 unified confirmation milestone

**Implemented in the unreleased development tree; cross-platform and release qualification outstanding:** simplify the public MCP tool catalog by replacing the eight preview-backed apply tools with one `confirm_verified_operation` tool. Keep every preview tool, prepared payload, existing apply engine, and its safety boundary intact. The goal is an easier-to-use API even if external safety-screening behavior does not change. The term *verified* means the server prepared and checked a proposed operation; it does **not** mean the action is already applied or automatically authorized.

### Proposed public contract

- Tool: `confirm_verified_operation` with required `previewId` (the existing 64-character identifier) and optional short `note`. The note is untrusted descriptive text: bound its length, do not use it for authorization, routing, parameters, audit decisions, privileged instructions, or mutation logic. Do not promise that an external platform will accept a request based on the note.
- Preserve truthful tool annotations: `readOnlyHint: false`, `destructiveHint: true`, and `openWorldHint: false`; a prepared operation can still alter or remove data.
- Replace the public tools `edit_file_apply`, `patch_package_apply`, `filesystem_package_apply`, `markdown_apply`, `convert_encoding_apply`, `manage_bom_apply`, `backup_restore_apply`, and `backup_gc_apply`. Retain their internal engines and original checks. This changes the development tool catalog from 43 to 36 entries. Version 3.3.6 remains the currently shipped 43-tool release.
- Return the original operation-specific structured result and stable errors, adding the operation kind without changing its meaning. No success claim after an uncertain commit; retain honest `PARTIAL_COMMIT` and observed-state reporting.
- Direct mutations without a preview, including whole-file replacement, line-ending changes, and explicit backup deletion, are **not** in scope.

### Implementation contract and concurrency invariants

The unified confirmation remains a mutating operation. Its input is a closed object: required `previewId` (exactly 64 lowercase hexadecimal characters) and optional `note` (at most 256 UTF-8 bytes). The note is descriptive only: it must not affect ownership lookup, authorization, the prepared operation, error classification, logging of sensitive content, or the mutation outcome. Reject unknown fields, empty IDs, malformed IDs, and oversized notes before routing.

A successful confirmation preserves the original operation-specific structured fields and MCP error/result semantics and adds a stable `operationKind` discriminator. Preserve existing operation-specific failure payloads, including partially committed and unknown states; never convert an uncertain state into success or retry a consumed capability. Missing, expired, evicted, consumed, or previous-process identifiers yield a conflict; malformed identifiers yield invalid input. Normal permission, cancellation, limit, stale-target, and filesystem failures remain the owning engine's responsibility.

One process-local routing index records only `previewId -> operation owner`, not prepared payloads or a separate approval status. **Admission and removal must be coordinated with each owning store under a documented lock order**: a preview cannot be published without its routing reservation, and rejected reservations cannot leave reachable orphan previews. A cross-store ID collision must reject or regenerate the new candidate, never replace an existing owner. The index is bounded by the aggregate live preview capacity; expiry, eviction, explicit discard, consumption, and process restart must not allow unbounded retention or revival. Routing releases its own lock before invoking the authoritative engine claim; the engine, not the index, decides whether the preview is still valid. Keep the Markdown subkind and the BOM/encoding subkind in their existing stores.

The security review must compare (1) coordinated registration/removal using existing stores, (2) store probing, and (3) duplicated payload migration. Probing is not acceptable if it can claim from the wrong store or create timing-dependent errors; payload migration is not acceptable if it replaces existing authority or duplicates retained data. Verify admission rollback, deterministic lock order, cross-store forced collisions, eviction/expiry cleanup, concurrent double confirmation, and realistic worst-case capacity before implementing the public change.

### Client migration boundary

In 4.0, clients must rediscover the MCP catalog and replace calls to all eight former `*_apply` tool names with `confirm_verified_operation({ "previewId": "<preview token>" })`. Every preview producer and its approval evidence remain unchanged. The new optional note is never necessary to confirm an operation. Do not expose public aliases for removed tools; old clients invoking a removed name receive the normal MCP unknown-tool response, because an unregistered tool cannot reliably provide its own custom migration message. The old-to-new mapping and discovery requirements are documented in [MIGRATION_4.0.md](MIGRATION_4.0.md); do not rewrite historical 2.0/3.0 guidance. Update the three guided prompts, tool descriptions, contract tests and generated catalog projections alongside the actual 4.0 tool registration. Rollback is a release/runtime choice: reverting to a 3.x binary restores its old tool catalog, but outstanding process-local preview identifiers cannot survive a restart in either direction.

### Execution sequence and mandatory gates

1. **Inventory and contract freeze (read-only first).** Map each preview producer, store, ownership, TTL, eviction, cancellation, restart behavior, claim boundary, apply handler, result type, tool-registration site, and tests. Resolve common registration and bounded cleanup without creating a second capability authority. Compare a small routing registry with probing stores or migrating payloads; choose the least invasive design only after proving every one of the eight paths. Gate: a reviewed eight-path matrix and written error/response contract.
2. **Threat and concurrency review.** A registry maps IDs to owner engines only; original stores remain authoritative. Registration must be coordinated with preview creation: prevent an orphaned preview if admission fails, avoid unbounded stale entries, and reject ID collisions across all engines. Expired, evicted, consumed, foreign, malformed, and restarted previews must fail closed. Preserve one-shot atomic claim at the owning engine and prevent two competing confirmations from applying twice. Gate: negative scenarios and resource-bound invariants accepted before implementation.
3. **Migration design.** Define the intentional major-version removal of the eight public apply names, update affected client examples and the migration guide, and keep internal handler semantics. No simultaneous public aliases in 4.0 unless a separately approved compatibility decision requires them. Plan a clear actionable error for old clients if practical without re-exposing removed tools. Gate: explicit documented mapping, with no hidden API changes.
4. **Minimal implementation using TDD.** Add focused red contract and race tests, then one bounded dispatcher and the minimal registration hooks needed in existing preview paths. Do not consolidate payload caches, alter backup semantics, or refactor unrelated code. Run focused tests first, then verify source/tool-catalog contract generation and documentation-link checks before review. Gate: all eight end-to-end paths and cross-transport behavior pass.
5. **Adversarial qualification.** Exercise success/no-op, invalid and conflicting IDs, wrong owner, expiry/eviction/restart, simultaneous claims, interruption, lost response after commit, partial multifile results, symlink/junction and root-policy changes, required/pinned backups, permissions, encoding/BOM/EOL preservation, and resource exhaustion. Test Windows, Linux, and macOS where CI runners support the relevant cases. Gate: no safety/behavior regression and no unsupported claims about external screening.
6. **Documentation and release gate.** Update `TOOLS.md`, `README.md`, architecture and migration guidance only for implemented behavior; changelog after the feature is verified. Apply the [Development Checklist](DEVELOPMENT_CHECKLIST.md) and [Publishing Procedure](PUBLISHING.md): exact clean commit, full test/race/static/security/release-candidate qualification, review, and explicit release authorization. Do not publish, tag, push, or upgrade private installations merely because this milestone is approved.

### Scope and review discipline

- Keep Source Intelligence, performance, and diagnostics unchanged unless specific regressions are proven. Simplify other tool names, descriptions, or responses only when concrete evidence justifies a separate scoped change.
- Desktop control is excluded. MCP federation and project-wide semantic refactoring require independent product and security review; neither belongs to this milestone.
- At each phase, use the `cgptw-stage-probe` specialist skill appropriate to the task when available: contract/module design; security assurance; compatibility/migration; implement-verified-change; test strategy; independent code review; release readiness. Select one primary skill per phase, rather than treating a skill as another executing agent or evidence of successful tests.
- Do not conceal mutating behavior or use notes to influence/bypass a platform's safety controls. A single confirmation tool is a usability and compatibility change, not a guarantee of fewer external blocks.

Automatic project-wide semantic refactoring remains out of scope unless maintainers explicitly reopen that decision.

Visual desktop interaction is excluded from this milestone. MCP federation is not approved as an API and requires separate product, architecture, privacy, security, and UX review before implementation.
