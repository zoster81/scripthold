# MCP Mutation Surface and Backup UX Design

## Status

**APPROVED — R23 ACTIVE DESIGN BASELINE.** This document records the approved direction for separating read-only preparation from filesystem mutation, reducing avoidable MCP-client blocking, and making persistent backup history easier to use. Implementation has not started; the current `2.2.0` tool surface remains authoritative in [`TOOLS.md`](../TOOLS.md).

R23 may intentionally change public tool schemas or names. Any compatibility transition must be specified and tested before implementation; this document does not silently redefine the current API.

## Problem

Several current tools combine read-only actions and mutating actions behind one MCP tool definition. MCP annotations are static per tool, so a tool that can mutate must be advertised conservatively even when a particular request is only a preview or inspection.

The clearest examples are:

- `edit_file`: `preview` is read-only while `direct` and `apply` can mutate;
- `patch_package`: `inspect`, `dryRun`, and `verify` are read-only while `apply` mutates;
- `backup_store`: status/review/audit/restore-preview/GC-preview actions are read-only while restore/GC apply actions mutate;
- `manage_bom`: detection is read-only while add/strip mutate;
- `convert_encoding`: dry-run is read-only while normal conversion mutates.

This mixed surface can cause an MCP host or connector to treat a harmless preparation request as destructive. When callers then fall back to shell or scripts merely to edit files, they can bypass Scripthold's encoding preservation, path validation, fingerprints, conflict detection, durable staging, backup integration, and post-commit verification.

R23 addresses the avoidable classification problem. It does **not** attempt to bypass legitimate approval or safety policy for an actual mutation.

## Goals

R23 will:

- expose read-only preparation through tools whose handlers are physically incapable of filesystem mutation;
- expose mutation through separately annotated apply tools;
- preserve the one-shot capability model: preview prepares exact result bytes and returns an unguessable `previewId`, while apply accepts only that identifier;
- ensure an apply cannot resubmit or alter path, patch text, replacement text, encoding, writable authorization, or backup policy;
- revalidate authorization, file identity, fingerprints, prepared bytes, and current state immediately before mutation;
- retain the existing fail-closed conflict/replay/expiry/restart behavior;
- make persistent backup history and comparison easier to inspect without starting a restore;
- provide an operator-configurable default persistent-backup policy for eligible approved mutations, with the exact public configuration name finalized during implementation design;
- reduce script fallback for ordinary file mutation without weakening MCP annotations or filesystem security;
- preserve stdio and Streamable HTTP equivalence.

## Non-goals

R23 will not:

- mark a genuinely mutating tool as read-only or non-destructive merely to avoid client approval;
- guarantee that a client will never request confirmation or reject a true apply operation;
- turn `task_run` into a preferred file-edit path;
- provide arbitrary command execution through a file-operation tool;
- claim automatic rollback for multi-file partial commits;
- make backup history equivalent to transactional undo;
- implement recursive directory mutation or general filesystem refactoring; that belongs to R24;
- implement source-code symbol indexing; that belongs to R25.

## Approved preview/apply contract

The canonical flow is:

```text
read-only preview
    -> exact proposed result + diff + fingerprints + metadata
    -> unguessable previewId
    -> explicit mutating apply(previewId)
    -> revalidation
    -> required backup when policy demands it
    -> durable commit
    -> final verification
```

### Preview

A preview tool may accept the complete preparation request, including target path, exact edits or patch, encoding choices, writable authorization, and backup policy where applicable.

It must:

- perform current allowed-root and path-security validation;
- prepare the exact final bytes without modifying public targets or persistent backup state;
- return a bounded diff or equivalent final-state evidence;
- return target and result fingerprints plus relevant encoding/BOM/line-ending metadata;
- retain exact prepared bytes, target identity, authorization-sensitive options, and backup policy behind a bounded expiring capability;
- be annotated truthfully as read-only only when no code path from that public tool can mutate filesystem or persistent state.

### Apply

The corresponding apply tool accepts **only `previewId`**.

It must not accept a path, patch, edit list, content, encoding, force-writable option, backup-policy override, or other mutation payload. Apply atomically consumes the capability before validation so success, conflict, cancellation, write failure, and replay are terminal outcomes.

Before the first mutation boundary it revalidates:

- capability validity and expiry;
- current allowed-root authorization;
- target path and stable file identity or approved missing state;
- target fingerprint;
- retained prepared-result fingerprint;
- retained permission and backup policy;
- source/target state required by the specific operation.

If any approved precondition changed, apply fails with `CONFLICT` rather than recalculating a different result.

## MCP annotation contract

Tool annotations must describe the complete capability of the public tool, not the intended action of one request.

R23 therefore requires:

- read-only tools to have `readOnlyHint: true` and no reachable mutation path;
- true mutation tools to retain `readOnlyHint: false` and appropriate `destructiveHint` semantics;
- `openWorldHint` to continue reflecting external-world access rather than being altered to influence approval UX;
- catalog, runtime registration, generated projections, README, and `TOOLS.md` to remain synchronized;
- automated drift/consistency tests that fail when a read-only annotation can reach mutation logic or when a newly mixed read/write action surface is introduced without explicit review.

The implementation must not solve connector blocking by lying in metadata.

## Surface restructuring

Exact final names are an implementation-time compatibility decision, but the public capability boundaries are approved:

- edit preparation and edit apply become distinct MCP tools;
- patch-package inspection/preview/verification are separated from package apply;
- backup review/audit/restore-preview/GC-preview capability is separated from restore/GC mutation capability where required to make annotations truthful;
- BOM detection is separated from BOM mutation;
- encoding-conversion dry-run/preparation is separated from conversion apply where the same static-annotation problem exists.

The current direct-edit path is a compatibility liability because it bypasses approval-bound preview/apply. R23 must explicitly decide whether to remove it or retain it only through a clearly documented migration path; it must not remain accidentally privileged.

## Persistent backup UX

The existing backup store remains a separate protected internal authority and keeps its immutable-object/manifest, quota, explicit-GC, no-background-GC, and no-false-rollback invariants from [`PERSISTENT_BACKUP_LIFECYCLE.md`](PERSISTENT_BACKUP_LIFECYCLE.md).

R23 improves usability without exposing raw object bytes or internal store paths:

- provide an explicit version-history view for an authorized target;
- provide read-only comparison of a backup with the current target and, where safely bounded, one backup with another backup;
- retain restore as preview/apply with mandatory safety backup of an existing target;
- allow an operator-configurable default persistent-backup policy for eligible approved mutations so callers do not have to remember `backupPolicy: "required"` on every operation;
- preview remains side-effect-free and logical no-ops create no backup;
- required-backup admission failure prevents the target mutation;
- exact configuration names, migration behavior, and default value must be finalized before code changes and must remain disabled or compatibility-safe unless the milestone explicitly approves a breaking default.

History and comparison are review capabilities, not automatic rollback. Multi-file undo requires explicit operation-level recovery semantics and is not implied by the presence of backups.

## Compatibility strategy

R23 changes a public MCP surface and therefore requires an explicit migration decision before implementation. At minimum the design review must resolve:

- final tool names and schemas;
- whether legacy mixed tools are removed in one breaking release or retained temporarily;
- how retained legacy tools are annotated without recreating the original preview-blocking problem;
- whether direct edit is removed, deprecated, or isolated;
- how guided prompts migrate to the separated tools;
- release-version implications and changelog/migration documentation.

A compatibility wrapper that still combines read-only and mutating actions under one destructive annotation does not satisfy the primary R23 goal.

## Required tests

R23 implementation must include focused and regression coverage for:

- every read-only public tool being mutation-free under all valid and invalid inputs;
- annotation/catalog/runtime/schema consistency;
- preview diff/result fingerprints and exact prepared bytes;
- apply accepting only a capability identifier;
- stale target, same-content path replacement, expiry, eviction, restart, replay, and concurrent apply;
- required backup creation before mutation and no backup on preview/no-op;
- backup history filtering, authorization, pagination, and bounded comparison output;
- encoding/BOM/line-ending preservation across representative Unicode and legacy codecs;
- read-only and permission-change targets;
- cancellation and injected failures before/after backup, staging, replacement, directory sync, and verification;
- stdio/HTTP tool metadata and representative-result equivalence;
- connector-level smoke evidence that read-only preparation is exposed as read-only and no longer requires shell/script fallback solely because it shares metadata with an apply action.

True apply operations may still require client approval; that is expected and must not be treated as a failed test.

## Completion gate

R23 is complete only when:

1. the final public split and compatibility strategy are documented and approved;
2. every mixed read/write tool in scope has a truthful capability boundary or an explicitly justified exception;
3. preview/apply retains exact-result capability binding and current security/durability invariants;
4. backup history/comparison and the approved default-policy mechanism are implemented with bounded read-only behavior;
5. catalog/runtime/docs/generated projections agree;
6. focused, regression, race where available, static-analysis, vulnerability, cross-platform build, and repository consistency checks pass as applicable;
7. connector smoke testing confirms that read-only preview operations are no longer classified as destructive merely because their corresponding apply capability exists;
8. any intentional public API break has migration and changelog documentation before release.

## Follow-on milestones

R23 deliberately establishes the capability pattern reused by later work:

- **R24 — Safe filesystem operations:** governed by [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md), with typed preview/apply packages for coordinated create/copy/move/delete/directory operations without arbitrary shell commands;
- **R25 — Source intelligence foundation:** governed by [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md), with language-neutral bounded symbol extraction/indexing and Go native AST only as the first reference provider;
- **R26 — Backup recovery:** governed by [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md), with separately reviewed offline evidence-preserving repair/salvage beyond the current diagnostic-only command;
- **R27 — Broad multi-language code intelligence:** governed by [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md), with mandatory broad language coverage plus references, implementations, dependency/call relationships, and incremental indexing after the common symbol model is stable.
