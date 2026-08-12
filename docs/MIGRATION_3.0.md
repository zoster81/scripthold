# R23-R24 Next-Major MCP Surface Migration

## Status

This guide documents the intentional breaking MCP API transition implemented by completed R23 and the active R24 Unreleased source candidate. R24 source changes are implemented, all available local gates pass, and connector-level preview/apply acceptance against the activated R24 candidate passes; only required native Linux/macOS namespace verification remains pending. It is migration guidance for the next major release line; it does **not** state that Scripthold `3.0.0` has been released, tagged, or published. Scripthold `2.2.0` remains the current public release until a later release is explicitly authorized and published.

The authoritative R23 design and completion gate are in [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md). The authoritative R24 filesystem-package contract is in [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md). Tool schemas and examples are in [`TOOLS.md`](../TOOLS.md).

## Why the surface changes

Scripthold `2.2.0` combines read-only preparation and mutation in several MCP tool definitions. MCP annotations describe a complete tool rather than one action, so harmless preview/review requests can inherit destructive classification from another action in the same tool.

R23 removes that ambiguity. Read-only preparation/review remains on the historical tool names where practical, while actual mutation moves to dedicated apply tools. An apply request accepts only the unguessable one-shot `previewId` returned by preparation; path, content, edits, patches, encoding, permission intent, backup policy, and other mutation payload cannot be resubmitted or changed at apply time.

R24 applies the same capability model to coordinated filesystem mutations and removes four overlapping simple mutation tools from the next-major catalog. Instead of exposing separate public create/copy/move/delete entry points with different safety envelopes, callers prepare one bounded `filesystem-package-v1` manifest through `filesystem_package` and apply that exact plan through `filesystem_package_apply {previewId}`.

## Required caller changes

| Scripthold 2.2.0 request | Next-major request |
|---|---|
| `edit_file` omitted action or `action=direct` | `edit_file action=preview`, then `edit_file_apply {previewId}` |
| `edit_file action=apply` | `edit_file_apply {previewId}` |
| `patch_package action=apply` | `patch_package_apply {previewId}` after `patch_package action=dryRun` |
| `backup_store action=restoreApply` | `backup_restore_apply {previewId}` after `backup_store action=restorePreview` |
| `backup_store action=gcApply` | `backup_gc_apply {previewId}` after `backup_store action=gcDryRun` |
| `manage_bom action=add` | `manage_bom action=addPreview`, then `manage_bom_apply {previewId}` |
| `manage_bom action=strip` | `manage_bom action=stripPreview`, then `manage_bom_apply {previewId}` |
| `convert_encoding` with omitted/false `dryRun` | `convert_encoding dryRun=true`, then `convert_encoding_apply {previewId}` |
| `create_directory {path}` | `filesystem_package` with ordered `mkdir` operation(s), then `filesystem_package_apply {previewId}` |
| `copy_file {source,destination}` | `filesystem_package` with `copyFile`, then `filesystem_package_apply {previewId}` |
| `move_file {source,destination}` | `filesystem_package` with same-volume `move`, then `filesystem_package_apply {previewId}` |
| `delete_file {path}` | `filesystem_package` with `deleteFile`, then `filesystem_package_apply {previewId}` |

The read-only `patch_package` actions `inspect`, `dryRun`, and `verify` remain on `patch_package`. `backup_store` remains the read-only review/preparation surface for status, list/history, inspect/compare, audit, restore preview, and GC dry run. `manage_bom detect` remains read-only.

R24 additionally introduces `createFile`, `copyDirectory`, and `deleteDirectory` inside `filesystem-package-v1`. `createFile` accepts exact raw bytes as strict standard `contentBase64`; it is not a text-writing alias and performs no encoding, BOM, or line-ending conversion. Recursive copy/delete uses a complete exact scope that includes hidden entries and `.git` and does not apply `.gitignore` filtering.

The old `create_directory` behaved like recursive `mkdir -p`; R24 `mkdir` intentionally does not. To create multiple missing levels, declare each directory explicitly and in parent-before-child order. A later operation may rely on an earlier `mkdir` only as its immediate destination parent; no other generated output is consumable inside the same package.

## Apply contract

Every next-major mutation apply tool accepts exactly one required field:

```json
{"previewId":"<64-hex-character capability>"}
```

Unknown fields are rejected. A capability is process-local, bounded, expiring, unguessable, and one-shot. Apply consumes it before revalidation; success, conflict, cancellation, write failure, and replay are terminal outcomes. If current authorization, target identity, pre-state fingerprint, retained exact result, or another operation-specific precondition changed, the server fails instead of recalculating a different mutation.

Callers must therefore treat preview output as the complete approval evidence and must not expect to resend mutation parameters during apply.

For `filesystem_package_apply`, the complete package is revalidated before backup, after backup, and after pre-commit staging. Feasible create/copy content is staged before the first target commit. Operations then commit in manifest order with no-replace semantics and per-operation verification. The package deliberately does not provide transactional rollback: a failure after a target may have advanced returns `PARTIAL_COMMIT` with bounded post-state evidence rather than pretending the package was atomic.

`move` is deliberately narrower than a generic cross-filesystem move helper. The source and destination must be provably on the same filesystem volume, and the platform must provide a native no-replace rename primitive. Cross-filesystem copy-and-delete emulation is not performed and returns `UNSUPPORTED`.

## Persistent backup policy

R23 adds the operator setting `MCP_BACKUP_DEFAULT_POLICY=disabled|required`, defaulting to `disabled`. Eligible edit/package/BOM/encoding previews may omit `backupPolicy` to inherit the operator default or explicitly request `required`; a request cannot weaken an operator default of `required`.

Preview remains side-effect-free. Logical no-ops create no persistent backup and perform no write. A changed operation whose effective policy is `required` fails before mutation when backup admission is unavailable. Required persistent capture is durable and verified before the associated mutation boundary. Restore keeps its separate mandatory safety-backup rule for an existing target.

R24 filesystem deletion is stricter than the optional R23 mutation policy: every regular-file pre-state that would be irreversibly lost by `deleteFile` or `deleteDirectory` **must** be admitted, durably captured, and verified in the persistent backup store before the first target mutation. The operator default cannot weaken this R24 safety requirement. Deleting an empty directory requires no backup object because no regular-file bytes are lost.

`convert_encoding backup=true` remains the separate adjacent `.bak` behavior retained inside the conversion capability; it is not an alias for the persistent backup-store policy.

## Backup review additions

R23 adds read-only backup usability without exposing store object bytes or internal paths:

- `backup_store action=history` provides target-scoped version history;
- `backup_store action=compare` compares one verified backup with the current authorized target or two backups of the same target;
- comparison always returns fingerprint/equality evidence and adds a bounded text diff only when content is safely decodable within limits.

These review features do not imply transactional undo or automatic rollback.

## Compatibility expectations

There is no legacy `edit_file direct` alias and no mixed compatibility wrapper that combines preparation and apply under one destructive tool definition. Likewise, the R24 next-major catalog does not retain `create_directory`, `copy_file`, `move_file`, or `delete_file` alongside `filesystem_package`; callers using those public tool names must migrate.

Stdio and Streamable HTTP expose the same next-major catalog and schemas. A host may still require approval for a genuinely mutating apply tool; the split makes read-only preparation truthfully distinguishable from mutation and does not attempt to bypass client policy.

## Deployment boundary

Source migration, release publication, and operator deployment are separate actions. Updating a client configuration for this API should be coordinated with installation of the corresponding future major-release binary. Do not point a migrated client at Scripthold `2.2.0` and expect the R23/R24 next-major tool names or schemas to exist.
