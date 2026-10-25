# Scripthold 4.0 MCP Tool Migration

Scripthold 4.0 consolidates approval of prepared filesystem operations into one MCP tool. This guide explains how existing 3.x clients switch to the new command without changing the way previews are prepared or reviewed.

**Status:** Released in Scripthold 4.0.0. Existing 3.x installations retain their original apply commands until explicitly upgraded; refresh tool discovery and migrate clients after installing 4.0.0.

## Change for callers

The eight prior apply tool names are removed from the 4.0 catalog. Use `confirm_verified_operation` for every approved preview:

| In Scripthold 3.x | In Scripthold 4.0 |
|---|---|
| `edit_file_apply` | `confirm_verified_operation` |
| `patch_package_apply` | `confirm_verified_operation` |
| `filesystem_package_apply` | `confirm_verified_operation` |
| `markdown_apply` | `confirm_verified_operation` |
| `convert_encoding_apply` | `confirm_verified_operation` |
| `manage_bom_apply` | `confirm_verified_operation` |
| `backup_restore_apply` | `confirm_verified_operation` |
| `backup_gc_apply` | `confirm_verified_operation` |

The corresponding preparation tools and preview outputs are unchanged. The backup deletion command `backup_delete` is independent of this workflow and remains available.

## Confirming an operation

First run the appropriate preview command, review its changes, fingerprints, backup policy, targets and expiry, and obtain approval. Then call:

```json
{"previewId":"<the exact lowercase 64-character token from the preview>"}
```

An optional `note` can describe the approval:

```json
{"previewId":"<the exact lowercase 64-character token from the preview>","note":"Approved"}
```

The request is a closed object: no path, manifest, content, encoding or backup override is accepted. The note is limited to 256 UTF-8 bytes and cannot affect routing, authorization or mutation.

The response includes `operationKind` in addition to the original operation-specific structured fields. The operation's existing error code and partial or uncertain state are preserved.

A preview remains process-local, one-shot and subject to its original expiry and eviction limits. Replays, expired previews and IDs from a previous process receive a conflict. Clients must request a **fresh preview and fresh approval**, not retry an unknown or partially completed mutation. A restart or rollback to a 3.x executable does not preserve outstanding previews.

## Deployment and compatibility

Update client tool discovery after installing 4.0. Calls using removed tool names receive the normal MCP unknown-tool error; 4.0 does not register compatibility aliases. This is an intentional major-version breaking change. Do not assume an older client's cached tool list will continue working.

See [TOOLS.md](../TOOLS.md#confirm_verified_operation) for the command, [ARCHITECTURE.md](ARCHITECTURE.md) for mutation boundaries, and [ROADMAP.md](ROADMAP.md) for the remaining qualification before release.
