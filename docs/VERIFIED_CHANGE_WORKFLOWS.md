# Verified Change Workflows

## Status

**COMPLETE for the R16 verified-change primitives.** Persistent backup storage is governed separately by [PERSISTENT_BACKUP_LIFECYCLE.md](PERSISTENT_BACKUP_LIFECYCLE.md). This revision is aligned with the completed R23 MCP split: preparation/review is exposed through truthful read-only tools and mutation through previewId-only apply tools while preserving R16 fingerprint, conflict, durability, and no-false-atomicity guarantees. R23 completed its connector acceptance gate on 2026-08-12.

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

R23 exposes the R16 edit preparation pipeline through two physically separate MCP capabilities. `edit_file` is read-only and accepts only `action=preview`; historical omitted-action/direct mutation and in-tool apply are removed. `edit_file_apply` is mutating and accepts only `previewId`.

### Preview

Preview returns bounded approval evidence including an unguessable 256-bit `previewId`, creation/expiration metadata, target/result fingerprints, bounded diff, encoding/BOM/line-ending metadata, logical no-op status, and the effective persistent backup policy. It retains exact prepared result bytes and stable target identity in a count/byte/TTL-bounded process-local cache. Preview performs no target write, permission change, persistent capture, `.bak` creation, or target-adjacent staging. When the effective backup policy is `required`, only a changed result performs read-only backup admission preflight; a no-op requires no store.

### Apply

`edit_file_apply` accepts exactly one field: `previewId`. Unknown path/content/edit/encoding/permission/backup fields are rejected. The capability is atomically consumed before validation so replay, conflict, cancellation, write failure, and success are terminal.

Before commit the server revalidates capability validity, current authorization/resolved path, stable target identity, approved pre-state fingerprint, retained exact result fingerprint, bound permission intent, and persistent backup policy. If a changed result requires persistent backup, the exact approved pre-state is durably captured/verified before permission changes or target replacement and the target is revalidated again. The shared durable mutation layer commits the retained bytes and final fingerprint is verified. A logical no-op reports `applied=false`, performs no backup/write, and leaves metadata unchanged.

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
- `patch_package_apply` consumes the exact dry-run capability and accepts no resubmitted manifest. It revalidates targets/prepared bytes, completes and verifies required all-target backup capture, revalidates again, then stages changed results and commits in deterministic manifest order.
- `verify` compares current fingerprints with every `expectedResultFingerprint` and returns ordered per-file plus aggregate evidence.

Package preview state is process-local and independently bounded by count, retained bytes, TTL, target count, manifest bytes, prepared bytes, per-file source/result size, and total response output.

### Partial-commit contract

The package does **not** claim multi-file atomicity or automatic rollback.

Every target must pass parse, path, fingerprint, preparation, required-backup, post-backup revalidation, and staging gates before the first commit. If a later failure occurs after one or more targets may have changed, processing stops and the server performs bounded best-effort classification:

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

The R23 integration point remains narrow and approval-bound:

- `MCP_BACKUP_DEFAULT_POLICY` is `disabled|required`; omitted request policy inherits it, an explicit `required` may strengthen it, and callers cannot weaken an operator-required default;
- `edit_file` preview, `patch_package` dry-run, `manage_bom` add/strip preview, and `convert_encoding dryRun=true` retain the effective policy inside their exact one-shot capabilities;
- preparation remains free of persistent backup writes; changed required operations perform only read-only admission preflight, while logical no-ops require no store and create no backup;
- the corresponding apply tool accepts only `previewId` and every required persistent pre-state is durably captured/verified before target mutation; package and conversion batches finish required backup capture before the first target write;
- package/conversion sequential commits still do not claim automatic rollback or multi-file atomicity;
- `convert_encoding.backup=true` is an independent adjacent `.bak` intent retained inside the capability and is created only by apply for changed targets;
- restore keeps its separate mandatory safety-backup contract and GC captures no public target bytes.

Restore, audit, quotas, pinning, retention, and garbage collection are otherwise defined by the backup lifecycle contract.

## Windows atomic-replacement diagnostics

Windows existing-file replacement stages the complete result in the target directory, flushes and closes the staged file, revalidates the approved target snapshot, and first commits with `MoveFileExW(MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)`. Retry remains bounded to transient Windows access/sharing/lock errors and revalidates the approved target before each repeated commit attempt. After the first failed classic commit and a successful revalidation, Scripthold may make one `SetFileInformationByHandle(FileRenameInfoEx)` attempt with `FILE_RENAME_REPLACE_IF_EXISTS | FILE_RENAME_POSIX_SEMANTICS`, but only when an independent DELETE-access probe on the current target succeeds. This handles the Windows case where a delete-sharing open handle still prevents `MoveFileExW` replacement; a target whose ACL or share mode denies DELETE is never bypassed, and an unsupported POSIX-rename class or filesystem falls back to the original bounded `MoveFileExW` retries. The normal fast path therefore retains `MOVEFILE_WRITE_THROUGH`; the fallback operates only on the already-synced same-directory staged file after fingerprint revalidation. Read sessions use delete-sharing on Windows and verify at `Finish` that the pathname still resolves to the same filesystem object, so a concurrent replacement is reported as a modification rather than certifying bytes from a superseded handle.

A retry episode is observed as one bounded event rather than one log record per attempt. A retry that eventually succeeds or aborts for a non-retryable reason is logged only when debug logging is enabled. An exhausted retry is always emitted as a warning and includes the deeper best-effort Windows evidence needed for later correlation. Diagnostic failures or panics are ignored and can never alter mutation success/failure.

The versioned `windows-atomic-replace-v1` payload records only bounded technical evidence: retry phase/Win32 codes, attempt count and elapsed time; SHA-256 identifiers for the normalized target/staged paths plus the target extension; basic attributes, size/link/delete-pending state; independent DELETE-access probes for both target and staged file; independent `FILE_ADD_FILE` and `FILE_DELETE_CHILD` probes on the target parent directory; and a bounded Restart Manager view of applications/services currently using either target or staged file. Restart Manager collection is performed only after the ordinary retry window is exhausted, and target plus staged path are registered together in one session rather than through per-file calls. Process evidence is limited to PID, application type, bounded application/service display names, restartability, and whether the reported process is the current server process.

The diagnostic payload deliberately excludes source bytes, diffs, command lines, executable paths, preview identifiers, and clear target/staged paths. A path hash exists only to correlate repeated incidents affecting the same pathname. A single `ERROR_ACCESS_DENIED` is not classified as a root cause: Windows permits delete/rename authorization through DELETE on the file or delete-child permission on the parent, while destination creation also depends on parent-directory access. A denied DELETE probe with a sharing violation plus a Restart Manager process is strong evidence of incompatible handle sharing. A granted DELETE probe plus a current-process Restart Manager match can instead indicate the classic `MoveFileExW` open-destination limitation; focused Windows tests reproduce that signature with both retained file identities and read sessions, while the guarded POSIX fallback resolves it without overriding non-delete-sharing handles. Logs remain evidence rather than permission for process termination or unbounded retries.

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

R16 completed on 2026-08-04. Deterministic fingerprints, bounded exact edit/package preparation, strict `patch-package-v1`, and typed `verify_state` remain implemented primitives. R23 changes their MCP exposure by separating read-only preparation from mutation (`edit_file_apply` and `patch_package_apply`) without changing R16's conflict, no-false-atomicity, or no-automatic-rollback guarantees.
