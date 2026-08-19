# MCP Mutation Surface and Backup UX Contract

## Status

R23 is **COMPLETE** and shipped in Scripthold `3.0.0`. This document defines the final separation between read-only preparation/review and explicitly mutating apply authority.

## Scope

R23 removed mixed MCP tools whose static annotations could not truthfully represent both preview and mutation. Read-only preparation now lives on handlers that cannot mutate public state; corresponding apply tools accept only an unguessable one-shot `previewId` and cannot resubmit or alter paths, edits, encoding, writable authorization, or backup policy.

Apply revalidates current authorization, identity/fingerprints, prepared bytes, capability state, and required backup policy before mutation. Replay, expiry, restart, conflict, cancellation, partial-state, encoding/BOM/EOL, and durable commit guarantees remain fail closed. This split improves MCP annotation truthfulness without bypassing approval for genuine mutations or turning `task_run` into an editing path.

R24 later applied the same preparation/apply discipline to namespace operations; R25/R27 source intelligence remains read-only; R26 backup recovery retains a separate offline authority.

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

If any approved precondition changed, apply fails with `CONFLICT` rather than recalculating a different result. After a write reaches a boundary where the target may already have changed, single-file apply must classify the bounded actual target state instead of reusing preview predictions: an unchanged target preserves the underlying error with `changed=false`; an observed committed result or an unclassifiable post-error state returns `PARTIAL_COMMIT` with actual-state evidence and never claims `applied=true` merely because the prepared bytes were expected to change the file.

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

The capability boundaries were approved before implementation; the final names and compatibility decision are recorded below:

- edit preparation and edit apply become distinct MCP tools;
- patch-package inspection/preview/verification are separated from package apply;
- backup review/audit/restore-preview/GC-preview capability is separated from restore/GC mutation capability where required to make annotations truthful;
- BOM detection is separated from BOM mutation;
- encoding-conversion dry-run/preparation is separated from conversion apply where the same static-annotation problem exists.

The former direct-edit path was identified as a compatibility liability because it bypassed approval-bound preview/apply. The final decision below removes direct edit from the MCP surface rather than retaining an accidentally privileged alias.

## Final R23 public-surface and compatibility decision

R23 uses an intentional breaking transition rather than a legacy mixed wrapper. The five historical mixed tool names remain registered only for read-only preparation/review behavior that can be annotated truthfully; their mutating forms are removed and rejected rather than silently reinterpreted. Mutation moves to dedicated apply tools whose complete public input is exactly `{ "previewId": "..." }`.

The final public split is:

| Read-only tool | R23 read-only contract | Mutating apply tool |
| --- | --- | --- |
| `edit_file` | `action="preview"` only; prepares exact edit result. Historical omitted-action/direct and in-tool apply forms are removed. | `edit_file_apply` |
| `patch_package` | `inspect`, `dryRun`, and `verify` only. `dryRun` remains the compatibility spelling for capability-producing package preview. | `patch_package_apply` |
| `backup_store` | `status`, `list`, `history`, `inspect`, `compare`, `audit`, `restorePreview`, and `gcDryRun` only. `history` requires an authorized target; `compare` is bounded backup/current or same-target backup/backup review. | `backup_restore_apply` and `backup_gc_apply` |
| `manage_bom` | Existing `detect` remains read-only; mutation preparation uses explicit `addPreview` or `stripPreview` so an old `add`/`strip` request cannot silently become a no-op preview. | `manage_bom_apply` |
| `convert_encoding` | Requires `dryRun=true`, prepares and retains exact converted bytes, and returns a capability. Historical `dryRun=false`/omitted mutation is removed. | `convert_encoding_apply` |

Every apply schema contains only required `previewId`; unknown fields are rejected. There is no `edit_file` direct-mutation compatibility alias. Existing callers that already use `edit_file action=preview`, `patch_package dryRun`, `backup_store restorePreview`/`gcDryRun`, `manage_bom detect`, or `convert_encoding dryRun=true` keep the read-only preparation entry point; callers of the former mutating forms must migrate to the returned capability plus the corresponding apply tool.

R23 also finalizes the operator default as `MCP_BACKUP_DEFAULT_POLICY=disabled|required`, defaulting to `disabled` for compatibility. Eligible approval-bound content mutations are edit, patch package, BOM change, and encoding conversion. A request may explicitly bind `backupPolicy="required"`; omission inherits the operator default. No request value can weaken an operator default of `required`. Restore keeps its independent mandatory safety-backup rule for an existing target, while GC has no content-backup policy. `convert_encoding.backup=true` remains the separate adjacent `.bak` request and is retained inside the preview capability; it is not reinterpreted as the persistent-store policy.

Because R23 removes previously public mutating request forms, it is a semantic-versioning breaking change and therefore ships in Scripthold `3.0.0` rather than as a silent `2.x` compatibility change. The concrete caller migration is documented in [MIGRATION_3.0.md](MIGRATION_3.0.md) and the user-visible changes are recorded in the `3.0.0` changelog; completing R23 did not itself create, tag, or publish that release.

## Persistent backup UX

The existing backup store remains a separate protected internal authority and keeps its immutable-object/manifest, quota, explicit-GC, no-background-GC, and no-false-rollback invariants from [`PERSISTENT_BACKUP_LIFECYCLE.md`](PERSISTENT_BACKUP_LIFECYCLE.md).

R23 improves usability without exposing raw object bytes or internal store paths:

- provide an explicit version-history view for an authorized target;
- provide read-only comparison of a backup with the current target and, where safely bounded, one backup with another backup;
- retain restore as preview/apply with mandatory safety backup of an existing target;
- allow an operator-configurable default persistent-backup policy for eligible approved mutations so callers do not have to remember `backupPolicy: "required"` on every operation;
- preview remains side-effect-free and logical no-ops create no backup;
- required-backup admission failure prevents the target mutation;
- configuration names, migration behavior, and the compatibility-safe default are finalized above and must remain synchronized with implementation and migration tests.

History and comparison are review capabilities, not automatic rollback. Multi-file undo requires explicit operation-level recovery semantics and is not implied by the presence of backups.

## Compatibility strategy

The migration decision was resolved before implementation and is now binding:

- the five historical mixed names remain only for their read-only preparation/review contracts;
- the former mutating forms are removed rather than wrapped behind destructive mixed definitions;
- direct `edit_file` mutation is removed from the MCP surface;
- six dedicated apply tools accept only `previewId` and reject unknown override fields;
- guided prompts and public documentation use the separated preparation/apply flow;
- the change is intentionally breaking and therefore ships in `3.0.0` rather than as a silent `2.x` compatibility change.

The legacy mixed Go handler entry points retained for package-internal regression coverage are not registered as MCP tools and are not part of the R23 public surface. A compatibility wrapper that combines read-only and mutating actions under one static tool definition does not satisfy R23 and must not be reintroduced.
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

## Subsequent evolution

R23 established the preview/apply capability pattern reused by later completed work. R24 applied it to safe filesystem packages, R25/R27 added read-only source intelligence without mutation authority, and R26 added a separate offline evidence-preserving recovery boundary for the protected backup store. Their current contracts live in [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md), [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md), [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md), and [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md).