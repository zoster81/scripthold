# R23 Next-Major MCP Surface Migration

## Status

This guide documents the intentional breaking MCP API transition implemented by the Unreleased R23 source tree. It is migration guidance for the next major release line; it does **not** state that Scripthold `3.0.0` has been released, tagged, or published. Scripthold `2.2.0` remains the current public release until a later release is explicitly authorized and published.

The authoritative R23 design and completion gate are in [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md). Tool schemas and examples are in [`TOOLS.md`](../TOOLS.md).

## Why the surface changes

Scripthold `2.2.0` combines read-only preparation and mutation in several MCP tool definitions. MCP annotations describe a complete tool rather than one action, so harmless preview/review requests can inherit destructive classification from another action in the same tool.

R23 removes that ambiguity. Read-only preparation/review remains on the historical tool names where practical, while actual mutation moves to dedicated apply tools. An apply request accepts only the unguessable one-shot `previewId` returned by preparation; path, content, edits, patches, encoding, permission intent, backup policy, and other mutation payload cannot be resubmitted or changed at apply time.

## Required caller changes

| Scripthold 2.2.0 request | R23 next-major request |
|---|---|
| `edit_file` omitted action or `action=direct` | `edit_file action=preview`, then `edit_file_apply {previewId}` |
| `edit_file action=apply` | `edit_file_apply {previewId}` |
| `patch_package action=apply` | `patch_package_apply {previewId}` after `patch_package action=dryRun` |
| `backup_store action=restoreApply` | `backup_restore_apply {previewId}` after `backup_store action=restorePreview` |
| `backup_store action=gcApply` | `backup_gc_apply {previewId}` after `backup_store action=gcDryRun` |
| `manage_bom action=add` | `manage_bom action=addPreview`, then `manage_bom_apply {previewId}` |
| `manage_bom action=strip` | `manage_bom action=stripPreview`, then `manage_bom_apply {previewId}` |
| `convert_encoding` with omitted/false `dryRun` | `convert_encoding dryRun=true`, then `convert_encoding_apply {previewId}` |

The read-only `patch_package` actions `inspect`, `dryRun`, and `verify` remain on `patch_package`. `backup_store` remains the read-only review/preparation surface for status, list/history, inspect/compare, audit, restore preview, and GC dry run. `manage_bom detect` remains read-only.

## Apply contract

Every R23 apply tool accepts exactly one required field:

```json
{"previewId":"<64-hex-character capability>"}
```

Unknown fields are rejected. A capability is process-local, bounded, expiring, unguessable, and one-shot. Apply consumes it before revalidation; success, conflict, cancellation, write failure, and replay are terminal outcomes. If current authorization, target identity, pre-state fingerprint, retained exact result, or another operation-specific precondition changed, the server fails instead of recalculating a different mutation.

Callers must therefore treat preview output as the complete approval evidence and must not expect to resend mutation parameters during apply.

## Persistent backup policy

R23 adds the operator setting `MCP_BACKUP_DEFAULT_POLICY=disabled|required`, defaulting to `disabled`. Eligible edit/package/BOM/encoding previews may omit `backupPolicy` to inherit the operator default or explicitly request `required`; a request cannot weaken an operator default of `required`.

Preview remains side-effect-free. Logical no-ops create no persistent backup and perform no write. A changed operation whose effective policy is `required` fails before mutation when backup admission is unavailable. Required persistent capture is durable and verified before the associated mutation boundary. Restore keeps its separate mandatory safety-backup rule for an existing target.

`convert_encoding backup=true` remains the separate adjacent `.bak` behavior retained inside the conversion capability; it is not an alias for the persistent backup-store policy.

## Backup review additions

R23 adds read-only backup usability without exposing store object bytes or internal paths:

- `backup_store action=history` provides target-scoped version history;
- `backup_store action=compare` compares one verified backup with the current authorized target or two backups of the same target;
- comparison always returns fingerprint/equality evidence and adds a bounded text diff only when content is safely decodable within limits.

These review features do not imply transactional undo or automatic rollback.

## Compatibility expectations

There is no legacy `edit_file direct` alias and no mixed compatibility wrapper that combines preparation and apply under one destructive tool definition. Callers using removed mutating request forms must migrate to the corresponding preparation plus dedicated apply tool.

Stdio and Streamable HTTP expose the same R23 catalog and schemas. A host may still require approval for a genuinely mutating apply tool; R23 only makes read-only preparation truthfully distinguishable from mutation and does not attempt to bypass client policy.

## Deployment boundary

Source migration, release publication, and operator deployment are separate actions. Updating a client configuration for this API should be coordinated with installation of the corresponding future major-release binary. Do not point a migrated client at Scripthold `2.2.0` and expect the R23 apply tool names to exist.
