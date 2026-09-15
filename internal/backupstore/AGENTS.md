# Backup Store Subsystem Agent Guide

This guide applies to `internal/backupstore/`. Follow the repository root [`AGENTS.md`](../../AGENTS.md) first.

## Security and durability invariants

- The store is a dedicated internal authority configured only at process startup and must never overlap a public allowed directory in either requested or resolved form.
- Reject relative paths, symlinks, junctions, reparse points, hard-linked internal regular files, special files, path aliases, root-identity replacement, and uncertain component resolution.
- Hold one platform-native exclusive writer lock for the complete store lifetime.
- Treat `store.json`, objects, and manifests as immutable. Derived indexes may be rebuilt but are never authoritative.
- Create directories and files with owner-only permissions where supported and never expose internal paths or stored bytes through ordinary logs or MCP results.
- Preserve the ordering invariant that durable objects precede manifests and manifest removal precedes object removal.
- Never add background deletion, automatic rollback, or multi-process writers without a separately approved design. The approved automatic deletion exception is synchronous per-target FIFO retention after a newer unpinned manifest is already durable; it may remove only the oldest non-pinned versions above `MaxVersionsPerTarget` and must never select pinned or active-restore records.
- Offline diagnostics are existing-store-only and mutation-free. The diagnostic dependency graph must not reach descriptor/layout creation, index persistence, GC cleanup, capture, restore, or target mutation helpers.

## Implementation guidance

Keep path validation, locking, descriptor handling, object storage, manifests, indexing, restore, and garbage collection in separate small components. Use bounded streaming reads and bounded directory enumeration with explicit size and count limits. Revalidate the retained store-root identity, internal layout, file type, owner-only permissions, and single-link state around every no-replace install or immutable read.

Platform-specific locking, reparse detection, and directory synchronization belong in build-tagged files. Existing-only diagnostic lock acquisition must never use create flags and must retain/revalidate lock identity. Failure injection must cover write, sync, close, rename, cleanup, lock, corruption, and crash-recovery boundaries.

## Verification

```bash
go test ./internal/backupstore -count=1
go test ./internal/security ./internal/config ./filetoolsserver/handler ./cmd/scripthold -count=1
go test ./... -count=1
git diff --check
```

Run the race detector and six-target builds when locking, concurrency, or platform-specific files change.
