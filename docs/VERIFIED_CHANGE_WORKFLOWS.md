# Verified Change Workflows

## Status

**COMPLETE.** This document is the stable R16 contract for deterministic fingerprints, one-shot edit approval, declared patch packages, and structured verification. Persistent backup storage is governed separately by [PERSISTENT_BACKUP_LIFECYCLE.md](PERSISTENT_BACKUP_LIFECYCLE.md).

## Goals

The verified-change surface lets an MCP client:

- identify the exact approved state of files or directory trees;
- preview a mutation and later apply exactly that prepared mutation;
- declare, preflight, apply, and verify a bounded ordered set of existing-file edits;
- run selected repository/file checks through typed inputs instead of arbitrary shell strings;
- preserve encoding, BOM, line endings, allowed-root security, durable mutation, bounded resources, stable errors, and transport equivalence.

## Non-goals

The R16 contract does not claim:

- atomic multi-file commit where the operating system cannot provide it;
- automatic rollback after a partial package commit;
- generic create/delete/move/rename actions inside patch packages;
- arbitrary command execution through `verify_state`;
- per-session filesystem ACLs or HTTP mutation of process roots;
- backup-store lifecycle policy inside the R16 design itself.

## Shared security and compatibility contract

All verified-change operations:

- use the existing allowed-root, symlink, junction, reparse-point, hard-link, and missing-ancestor validation;
- use stable typed errors and bounded diagnostics;
- reject oversized input before expensive parsing, hashing, diffing, or staging;
- honor cancellation during preparation, staging, verification, and cleanup;
- keep capability tokens and private operational state out of ordinary logs/results;
- preserve encoding, BOM, and line-ending behavior promised by the underlying edit/mutation path;
- expose equivalent schemas and behavior over stdio and Streamable HTTP.

R16 introduced `fingerprint_paths`, `patch_package`, and `verify_state`; later milestones expanded the catalog independently. Current tool counts belong in the runtime catalog/README, not in this historical design contract.

## Deterministic fingerprints

### `content-v1`

`fingerprint_paths` computes one deterministic aggregate SHA-256 over explicit files and/or directory roots. Canonical records are ordered lexically and include:

- Unicode-NFC slash-separated relative path;
- entry type;
- byte length;
- file-content SHA-256.

Modification times, ownership, and platform-specific permission bits are excluded from the default content fingerprint so equivalent copies remain stable across platforms. Metadata-sensitive modes must identify any additional fields explicitly.

Directory traversal reuses the secure deterministic walker. `.gitignore` handling is explicit and default-on with opt-out; VCS-internal `.git` directories remain excluded. Real directories and regular files participate in `content-v1`; in-root symlinks/junctions/reparse points are not followed or fingerprinted, while escaping entries fail closed.

### Complexity and limits

- Time: `O(total bytes read + entries)`.
- Memory: bounded by traversal depth, hashing buffers, and explicitly bounded returned entry details.
- File bytes are streamed rather than loaded in full.
- Path count, inspected entries, entry details, and response output are separately bounded.
- Success requires stable aggregate state across the complete fingerprint pass; concurrent changes return conflict evidence rather than a misleading hash.

## Edit preview/apply

`edit_file` supports direct editing plus an explicit one-shot preview/apply workflow through the same preparation pipeline used for exact, whitespace-flexible, fuzzy, and strict unified-patch editing.

### Preview

Preview returns a bounded approval result including:

- unguessable `previewId`;
- creation/expiration metadata;
- target and proposed-result fingerprints;
- bounded unified diff;
- encoding/BOM/line-ending metadata relevant to the prepared result;
- logical no-op status;
- retained backup policy when explicitly required.

The process-local preview cache is independently bounded by count, retained bytes, and TTL. Identifiers contain at least 256 bits of cryptographic randomness, are never listable, and are not written to ordinary logs. Process restart, eviction, expiration, response-limit failure, or apply claim invalidates/releases the capability and retained identity state.

### Apply

Apply accepts only `previewId`. The capability is atomically consumed before validation so replay, conflict, cancellation, write failure, and success are all terminal.

Before commit the server revalidates:

- capability validity and expiry;
- authorized resolved target identity;
- current target fingerprint;
- prepared result fingerprint;
- retained read-only/backup authorization;
- final path/state at the mutation boundary.

The shared durable mutation layer performs the commit. A successful logical no-op remains distinguishable from a byte-changing commit. The consumed capability is never re-emitted.

Because filesystem authorization is process-wide, preview tokens are not session-owned. Possession of the unguessable token plus normal server access authorizes only the exact prepared mutation bound to that token.

## Declared patch packages

`patch-package-v1` coordinates bounded edits to several existing regular files while detecting package-wide conflicts before the first commit.

### Manifest

The strict JSON manifest contains:

- `formatVersion: "patch-package-v1"`;
- optional bounded `label`;
- `fingerprintAlgorithm: "sha256"`;
- `fingerprintMode: "content-v1"`;
- ordered bounded `targets`;
- one unique normalized declared path per target;
- required `expectedFingerprint` and optional `expectedResultFingerprint`;
- exactly one of `edits` or one strict single-file unified `patch`;
- optional encoding/read-only authorization;
- optional exact `backupPolicy: "required"`.

Unknown fields are rejected throughout the manifest. The package rejects creation/deletion/movement/renaming, `/dev/null` patches, duplicate spellings, duplicate resolved targets, hard-link aliases, escaping links, unsupported versions/algorithms, oversized arrays, and oversized embedded data.

### Actions

- `inspect` validates structure, bounds, algorithms, paths, file types, duplicate/alias conditions, edit shapes, and patch syntax without reading full target contents.
- `dryRun` retains stable target identities, establishes a coherent package pre-state, verifies declared fingerprints, prepares every result, enforces aggregate limits, performs final coherent state verification, and returns ordered diffs plus aggregate evidence. When persistent backup is required it also performs side-effect-free conservative quota preflight. No source or backup-store state is changed.
- `apply` consumes the exact dry-run capability, rejects a resubmitted manifest, revalidates targets/prepared bytes, stages every changed result before the first target commit, performs required all-target backup capture before mutation, and commits in deterministic manifest order.
- `verify` compares current fingerprints with every `expectedResultFingerprint` and returns ordered per-file plus aggregate evidence.

Package preview state is process-local and independently bounded by count, retained bytes, TTL, target count, manifest bytes, prepared bytes, per-file source/result size, and total response output.

### Partial-commit contract

The package does **not** claim multi-file atomicity or automatic rollback.

Every target must pass parse, path, fingerprint, preparation, and staging gates before the first commit. If a later failure occurs after one or more targets may have changed, processing stops and the server performs bounded best-effort classification:

- `committed` — current bytes match the prepared result;
- `unchanged` — current bytes match the approved pre-state;
- `unknown` — neither state can be proven.

Any committed or unknown target produces stable `PARTIAL_COMMIT` evidence with the failed target/index, underlying error, counts, and available fingerprints. Post-replacement filesystem errors are classified from actual bytes rather than from the assumption that an error implies no mutation.

## Structured verification

`verify_state` provides one bounded ordered batch of allowlisted checks:

- JSON parsing for an explicit decoded file;
- text-format checks for encoding/BOM/line endings/trailing whitespace;
- fixed direct `git diff --check` for an explicit repository root and optional literal relative paths;
- shared fingerprint comparison.

A failed expectation returns `passed=false`; an operational failure carries a stable per-check error code.

Structured verification never accepts arbitrary executable names or user-supplied command strings. Any fixed executable is invoked directly without a shell, from a validated working directory, with constructed arguments, bounded stdout/stderr, timeout, cancellation, and filtered environment state.

## Persistent backup integration

Persistent backup behavior was deliberately designed after R16 and is authoritative in [PERSISTENT_BACKUP_LIFECYCLE.md](PERSISTENT_BACKUP_LIFECYCLE.md).

The integration point is intentionally narrow:

- `edit_file` creates a persistent pre-state backup only when the consumed preview retained `backupPolicy: "required"`;
- `patch_package` creates persistent backups only when the consumed package dry-run capability retained the same exact policy;
- dry run remains side-effect-free;
- every required backup is durable before its associated mutation boundary;
- package backup capture still does not create automatic rollback or a multi-file atomicity claim;
- existing operation-specific `.bak` behavior is separate and unchanged.

Restore, audit, quotas, pinning, retention, and garbage collection are not redefined here.

## Required verification

The completed implementation is protected by focused and regression coverage for:

- deterministic fingerprint ordering and repeated-run stability;
- escaping links, aliases, hard links, missing paths, and concurrent changes;
- preview expiry/eviction/replay/concurrent apply and restart invalidation;
- encoding/BOM/line-ending preservation and byte-identical no-op behavior;
- strict patch parsing and fuzzy ambiguity;
- malformed/oversized package manifests and aggregate preparation/output limits;
- coherent all-target preflight/staging and deterministic commit order;
- injected failures at each package commit position and post-replacement classification;
- structured JSON/text/fingerprint/Git checks, missing executables, timeout, cancellation, environment filtering, and path literals with spaces/metacharacters;
- stdio/HTTP equivalence;
- complete repository race/static/vulnerability and consistency gates when the subsystem changes.

## Completion record

R16 completed on 2026-08-04. Deterministic fingerprints, bounded one-shot `edit_file` preview/apply, strict `patch-package-v1` inspect/dry-run/apply/verify, and typed `verify_state` are implemented and remain part of the current Scripthold surface. Later persistent-backup work integrates through the explicit approval policy above without changing R16's no-false-atomicity and no-automatic-rollback guarantees.
