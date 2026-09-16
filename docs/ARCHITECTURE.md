# Scripthold Architecture

This document describes the **current** architectural boundaries of Scripthold. It is intentionally not a milestone diary or verification log. Public schemas and exact tool behavior remain authoritative in [`TOOLS.md`](../TOOLS.md) and `internal/toolcatalog/catalog.json`.

## Server and transports

Scripthold builds one MCP server and exposes the same tool catalog over stdio and Streamable HTTP.

- **stdio** is the default for client-managed local processes and secure bridge/tunnel topologies.
- **Streamable HTTP** is intended for persistent services, containers, and explicitly secured remote deployments. It is loopback-only by default and requires bearer authentication.
- Allowed directories are process-wide policy. Sessions are protocol/lifecycle units, not per-client filesystem ACLs.
- Modern MCP traffic and retained legacy behavior share the same outer authorization, resource, logging, and execution boundaries.

The HTTP threat model and deployment requirements are in [`HTTP_SECURITY.md`](HTTP_SECURITY.md).

## Filesystem boundary

Every filesystem operation is confined to explicitly authorized roots after canonical path resolution.

- Symlinks, junctions, Windows reparse points, aliases, and missing-path ancestors are validated before access.
- Recursive traversal is deterministic and does not follow escaping directory links.
- Missing destinations are authorized through their nearest existing ancestor.
- Existing objects are bound to stable identity where an operation requires later revalidation.
- Paths, directory entries, environment values, file contents, and process output are treated as untrusted input.

Mutating operations stage data before commit, detect practical concurrent changes, and use no-replace semantics for destinations that were missing at preparation time. Multi-file operations report partial progress truthfully rather than claiming transactionality that the operating system cannot provide.

## Text and encoding model

Scripthold presents decoded text to MCP clients as UTF-8 while preserving the source encoding unless conversion is explicitly requested.

- Encoding detection is based on BOM and content evidence, never filename extensions.
- Unicode BOM evidence is authoritative.
- Ambiguous non-empty data fails closed unless the caller supplies an explicit encoding.
- New files default to UTF-8 unless configuration or the request selects another encoding.
- Encoding, BOM, and line-ending promises are preserved by the shared text pipeline.
- Streaming operations independently bound input, line, result, and aggregate output memory.
- Full-document edits reject files above configured hard limits before unbounded preparation.

`list_encodings` is the runtime authority for supported codecs and aliases. The user-facing reference is in [`TOOLS.md`](../TOOLS.md#supported-encodings).

## Verified mutation model

Potentially destructive workflows separate **preparation** from **application**.

- Read-only preparation validates inputs and current state, computes exact result evidence, and returns an expiring high-entropy capability such as `previewId`.
- Apply tools accept only the prepared capability identifier; mutation parameters cannot be changed at apply time.
- Capabilities are bounded, process-local, one-shot, and consumed before final revalidation.
- Target authorization, identity, fingerprints, retained result bytes, backup requirements, and operation-specific preconditions are revalidated before mutation.
- Post-commit failures are classified from bounded observed state. Scripthold does not report a predicted preview state as fact after an uncertain commit boundary.

`edit_file`/`edit_file_apply`, patch packages, encoding/BOM previews, restore/GC previews, and filesystem packages follow this model where applicable.

## Persistent backup store

The optional persistent backup store is a separate internal authority from public workspace roots.

- Its root must not overlap public allowed roots and is inaccessible to ordinary file tools.
- Store ownership and permissions are fail-closed; one writer owns the store lifecycle.
- Objects are content-addressed and manifests are immutable records.
- Approval-bound mutations may require or explicitly request persistent pre-state capture.
- `pinned` backups are protected from automatic per-target retention and ordinary GC but can be explicitly removed by `backup_delete`.
- Restore captures a safety backup of an existing target before replacement.
- GC is explicit and preview/apply based; there is no background GC.
- After a newer eligible unpinned backup is durable, synchronous per-target retention may remove the oldest eligible non-pinned versions.
- Backup durability does not imply automatic rollback of a later failed target mutation.

Offline diagnosis/recovery commands operate on existing evidence without silently repairing or mutating the source store. Recovery, when requested, reconstructs into a separate destination and keeps source evidence unchanged.

## Durable work and deferred operations

Long-running shell/script execution and long-running read-only MCP operations use separate durable subsystems.

### Durable tasks

`task_run` is the only public execution entry point. Shell and script execution retain distinct authorization gates and are disabled by default. Accepted work is stored independently from the frontend connection and owned by the supervisor/worker/executor topology. Idempotency, logical locks, bounded queueing/logs, cancellation, retention, and at-most-once recovery semantics are part of the contract.

See [`DURABLE_TASKS.md`](DURABLE_TASKS.md).

### Deferred read-only operations

When a deferred-operation store is configured, eligible expensive read-only calls can be admitted durably before expensive execution and continue independently of the frontend wait window. Work that never crossed the durable started boundary may be recovered; work that lost its executor after durable start is not automatically replayed. An unreadable individual operation record is preserved, excluded from replay, and reported through bounded path-free diagnostics without blocking recovery of other readable operations; store-wide policy, lock, scan, or write failures remain fail-closed for the recovery cycle. Oversized completed MCP responses use the same compact retrieval surface but do not cause re-execution.

`source_query` remains synchronous because its returned index binding is process-local and intended for follow-up requests in the same runtime.

## Source intelligence

Source Intelligence is read-only and fail-closed.

- The registry contains 101 active approved source-analysis providers; [`LANGUAGE_CAPABILITIES.md`](LANGUAGE_CAPABILITIES.md) is the generated capability projection.
- Go uses the standard library AST; other providers use native bounded scanners/recognizers or offset-preserving adapters.
- Language/dialect routing uses source evidence and reports ambiguity rather than guessing.
- Public coordinates are based on decoded source, independent of the original byte encoding.
- Project relationships and evidence levels are reported only where the provider proves them.
- Process-local incremental indexing is bounded and retains normalized analysis facts, not a persistent complete-source cache.
- Returned source context is re-opened and fingerprint-verified before text is materialized.
- Source Intelligence does not execute project code and does not require external parser/compiler/LSP processes.

## Diagnostics

Diagnostics are intentionally separated by responsibility.

- Process/server diagnostics are redacted and may use stderr or bounded optional files.
- Deferred-operation recovery failures expose only a bounded path-free failure signature; repeated identical signatures are suppressed until the condition changes or clears, while raw store errors, paths, and operation identifiers remain private.
- HTTP access/security logging is a separate channel.
- Durable task stdout/stderr belongs to task logs, not server diagnostics.
- Human-readable tool failure text, raw panic values, stacks, secrets, and clear filesystem paths must not leak into category-only lifecycle logs.

Logging must degrade safely when an optional file sink fails; it must not change the result of the underlying MCP operation.

## Configuration and limits

Configuration is startup authority. Invalid values fail closed or fall back only where the specific configuration contract defines a safe default. Client-provided bounds may narrow server ceilings but must not enlarge them.

The main configuration families are:

- `MCP_MAX_*` for text, batch, traversal, output, and preview limits;
- `MCP_SOURCE_MAX_*` for source analysis/query limits;
- `MCP_BACKUP_*` for the optional backup store;
- `MCP_TASK_*` for durable tasks;
- `MCP_DEFERRED_*` and response-size limits for deferred operations;
- `MCP_HTTP_*` for Streamable HTTP;
- `MCP_LOG_*` for diagnostics;
- `MCP_ENABLE_*` for execution authorization.

Exact public behavior and relevant defaults belong in [`TOOLS.md`](../TOOLS.md), [`DURABLE_TASKS.md`](DURABLE_TASKS.md), and [`HTTP_SECURITY.md`](HTTP_SECURITY.md), not in historical design records.

## Change rule

Architecture changes should update this document only when a **current boundary or invariant** changes. Release history belongs in [`CHANGELOG.md`](../CHANGELOG.md), future work in [`ROADMAP.md`](ROADMAP.md), migration actions in the migration guides, and implementation history in Git.
