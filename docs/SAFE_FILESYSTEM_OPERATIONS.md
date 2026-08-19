# Safe Filesystem Operations Contract

## Status

R24 is **COMPLETE**. This document defines the final `filesystem_package` / `filesystem_package_apply` contract shipped in Scripthold `3.0.0`.

## Scope

R24 replaces routine shell/script fallback for ordinary workspace namespace changes with one bounded declarative preview/apply workflow. `filesystem-package-v1` supports exactly seven operation types: `mkdir`, raw-byte `createFile`, `copyFile`, exact `copyDirectory`, native same-volume `move`, `deleteFile`, and exact `deleteDirectory`.

The contract is no-replace, approval-bound, deterministic, and fail closed. It preserves allowed-root/link/reparse/protected-root rules, binds prepared source/destination/parent state, requires persistent backup before irreversible regular-file-byte loss, reports actual partial state rather than claiming whole-package atomicity, and never falls back to arbitrary shell execution. Text-aware mutation remains on the dedicated encoding-aware edit/write surfaces.
## Approved workflow

The canonical R24 workflow is:

```text
read-only package preview
    -> strict typed manifest validation
    -> secure path/identity resolution
    -> complete bounded effect enumeration
    -> source/destination/pre-state fingerprints
    -> backup/quota preflight where required
    -> exact ordered plan + warnings + result evidence
    -> previewId
    -> apply(previewId)
    -> full revalidation
    -> required backup capture
    -> durable staging
    -> deterministic commit order
    -> bounded post-state classification and verification
```

The preview operation is physically read-only. The apply operation is separately annotated as mutating and accepts only the prepared capability identifier.

## Package model

The public read-only tool is `filesystem_package`. It accepts a versioned `filesystem-package-v1` manifest containing an ordered array of typed operations and returns the deterministic preview plus a one-shot `previewId`. The public mutating tool is `filesystem_package_apply`; it accepts only `previewId`.

The v1 manifest has exactly seven operation types:

- `mkdir`: `{type, path}`;
- `createFile`: `{type, path, contentBase64}`;
- `copyFile`: `{type, source, destination}`;
- `copyDirectory`: `{type, source, destination}`;
- `move`: `{type, source, destination}`;
- `deleteFile`: `{type, path}`;
- `deleteDirectory`: `{type, path}`.

Unknown operation types, unknown fields, missing required fields, and fields belonging to another operation type are rejected. There is no generic recursive flag, overwrite flag, command field, hook, environment, or caller-selected backup bypass.

`mkdir` creates exactly one missing directory; it does not perform implicit recursive `MkdirAll`. A destination parent may already exist or may be created by an earlier `mkdir` in the same package. `createFile`, copy operations, and `move` require a missing destination and never replace it. `copyFile` accepts only an existing regular-file source. `copyDirectory` accepts only an existing real-directory source. `move` accepts an existing regular file or real directory. Delete operations require the declared target type to match.

To keep v1 reviewable and avoid a transaction-language explosion, a path/object may participate in only one mutating operation in a package, except that an earlier `mkdir` may provide a parent directory for later destinations. Sources must exist in the preview pre-state; v1 does not allow a later operation to consume a file or directory created, copied, or moved by an earlier operation. More complex dependent refactors use multiple approved packages rather than hidden virtual-namespace semantics.

### Path representation

Every path is treated as untrusted input.

Preview must:

- normalize lexical spelling;
- resolve existing ancestors through the established security layer;
- reject escaping links, junctions, reparse points, aliases, and protected roots;
- canonicalize path comparison for the target platform;
- detect duplicate declared paths and duplicate resolved identities;
- detect source/destination overlap that would make recursion or move semantics ambiguous;
- distinguish an expected missing destination from an existing destination;
- retain stable identities/handles where existing primitives support them.

A package cannot use a different spelling to mutate the same object twice accidentally.

## Recursive directory operations

Recursive directory copy/delete are a core R24 requirement, not an optional later enhancement.

### Enumeration

Preview must enumerate the complete operation scope before approval under hard limits for:

- entries visited;
- files;
- directories;
- total source bytes;
- maximum depth;
- retained result details;
- output bytes;
- preparation/capability retained bytes.

Traversal must be deterministic, cancellation-aware, complete, and independent of `.gitignore`; hidden names and `.git` are part of the tree when they are beneath the approved root. Hitting any entry, depth, byte, retained-detail, or output hard limit fails the preview rather than silently truncating mutation scope. Directory symlinks, junctions, reparse points, and other link-like entries are not followed and are rejected inside recursive v1 scopes. Unsupported special files are rejected rather than silently skipped or treated as regular files. Recursive scopes must not cross a nested filesystem/volume boundary; such a tree is unsupported in v1.

### Copy directory

A recursive copy must:

- require the destination tree state expected by the preview;
- prevent copying a directory into itself or its descendant;
- reject every destination collision; R24 v1 has no replace policy;
- stage file bytes using the durable filesystem layer where practical;
- preserve only metadata explicitly promised by the API;
- create directories/files in deterministic order;
- verify copied file content and final tree evidence before success.

### Delete directory

A recursive delete is destructive and must be especially explicit.

Preview must return the bounded exact deletion set or a deterministic aggregate plus enough retained details for meaningful approval. Apply may delete only entries bound into that preview. New unexpected descendants, changed identities, replaced entries, or scope growth after preview cause `CONFLICT` rather than being swept into the deletion.

Directory deletion must proceed children-before-parent and return structured actual-state evidence after any failure.

## Move and rename semantics

Moves are always no-replace in R24 v1. There is no overwrite variant in this milestone.

Preview binds:

- source identity and type;
- source fingerprint/evidence where applicable;
- destination expected state;
- parent identities relevant to the namespace change;
- cross-device implications detected before apply where possible.

R24 v1 supports only native same-filesystem/same-volume moves. A cross-filesystem move of either a file or directory fails as `UNSUPPORTED` during preview or, if the platform cannot prove the relationship until apply, before the first namespace mutation. R24 must not emulate a move as copy-plus-delete and must not fall back to shell/script execution.

## File content creation and replacement

R24 is primarily a namespace package, not a second text editor.

`createFile` is intentionally minimal and byte-oriented. `contentBase64` represents the exact bounded bytes to create; the empty string is a valid empty file. R24 performs no encoding detection, BOM policy, newline conversion, template expansion, or text reinterpretation for this operation.

`createFile` exists so one approved namespace package can include a new exact-byte artifact alongside independent move/delete/create-directory changes. It is not a replacement for `write_whole_file`, `edit_file`, `convert_encoding`, or the R23 text preparation/apply workflow. Encoding-aware text mutation stays on those dedicated surfaces.

R24 v1 does not overwrite existing files or directories.

## Backup integration

R24 must reuse the persistent backup authority rather than inventing an unrelated backup format.

The R24 v1 rules are:

- preview performs only read-only backup/quota admission checks;
- every R24 operation that would irreversibly destroy existing regular-file bytes requires persistent capture; callers cannot disable this protection for an R24 package;
- `deleteFile` therefore requires capture of its target before deletion;
- `deleteDirectory` requires capture of every regular file in the approved recursive scope before the first deletion begins, subject to bounded aggregate quota admission;
- an empty-directory delete requires no content backup;
- create/copy operations require no backup because they are no-replace;
- a native same-filesystem move requires no duplicate content backup because it does not destroy the moved bytes;
- a failed preflight, quota check, capture, capture verification, or source-state match prevents the destructive phase;
- no logical no-op creates a backup;
- durable backups survive later package failure and are recovery evidence, not a claim of whole-package rollback.

If current backup-store schemas are insufficient to represent R24 source-operation categories or recovery evidence safely, R24 must extend them through an explicit compatible design rather than bypassing the store.

## Preparation capability

R24 uses the R23 capability contract.

A preview capability retains, under strict count/byte/TTL limits:

- normalized manifest;
- deterministic operation order;
- resolved source/destination identities;
- expected missing states;
- relevant content/tree fingerprints;
- enumerated recursive scopes;
- exact prepared file bytes where applicable;
- required backup set and read-only quota-admission evidence;
- expected final-state evidence.

The identifier must be cryptographically unguessable, process-local unless a future design explicitly changes that model, non-listable, and invalid after restart, expiry, eviction, or first apply claim.

Apply accepts only `previewId`.

## Commit ordering and partial state

A multi-operation filesystem package cannot generally be made atomic across platforms. R24 therefore requires deterministic ordering and truthful failure evidence.

Before the first mutation, apply must perform all feasible package-wide checks, including:

- capability consumption;
- current authorization;
- source/destination identity revalidation;
- complete recursive-scope revalidation;
- expected missing/existing state checks;
- required backup capture;
- staging of content that can be safely staged in advance.

The implementation must choose and document an operation ordering that minimizes irreversible state. For example, creating/staging new content before deleting old content is generally preferable when semantics permit it.

After a failure that may have changed namespace state, the result must classify every operation/target with bounded evidence such as:

- `committed`;
- `unchanged`;
- `partially_committed`;
- `missing`;
- `present`;
- `unknown`.

The exact public vocabulary is an implementation decision, but uncertain state must never be reported as successful rollback.

`PARTIAL_COMMIT` or an explicitly reviewed compatible error category must be used when durable progress occurred and complete intended state cannot be established.

## Concurrency and TOCTOU contract

R24 must assume other processes can modify the workspace between preview and apply and even during apply.

Required mitigations include:

- stable identities where available;
- content/tree fingerprints;
- nearest-existing-ancestor validation for missing destinations;
- parent revalidation immediately before namespace operations;
- no-replace primitives for expected-missing destinations;
- target reinspection before destructive deletion;
- refusal to absorb newly appeared recursive entries;
- post-operation verification based on actual filesystem state.

The design may reduce but must not misrepresent unavoidable path-based TOCTOU windows.

## Encoding and metadata behavior

Filesystem copy/move/delete operations preserve bytes rather than decode/re-encode content.

Where file creation or text replacement is supported, the schema must explicitly identify the byte/text contract and reuse the established encoding subsystem when text semantics are promised.

Permission, timestamps, ownership, ACLs, alternate data streams, extended attributes, sparse-file properties, executable bits, and platform-specific metadata must be documented individually. R24 must not claim preservation of metadata that is not implemented and cross-platform tested.

## Security boundary

R24 remains inside the existing process-wide allowed-root model.

It must not:

- add per-session roots;
- allow package manifests to change configured roots;
- expose protected backup/task-store paths;
- follow escaping links;
- use shell expansion for filesystem operations;
- delegate safety-critical namespace changes to an unrestricted command string.

A package that cannot be represented safely through dedicated primitives must fail as unsupported rather than silently invoke `task_run`.

## Complexity and resource bounds

The implementation publishes bounded defaults and hard ceilings before public activation. Dedicated R24 limits are:

| Limit | Default | Hard ceiling |
|---|---:|---:|
| operations per package | 256 | 4,096 |
| manifest bytes | 16 MiB | 64 MiB |
| recursive entries per exact scope | 100,000 | 1,000,000 |
| recursive depth | 128 | 1,024 |
| aggregate source bytes | 1 GiB | 1 TiB |
| aggregate staging bytes | 1 GiB | 1 TiB |
| retained package previews | 16 | 1,024 |
| retained preview bytes | 128 MiB | 1 GiB |
| preview lifetime | 900 seconds | 86,400 seconds |

The corresponding environment variables are `MCP_MAX_FILESYSTEM_PACKAGE_OPERATIONS`, `MCP_MAX_FILESYSTEM_PACKAGE_BYTES`, `MCP_MAX_FILESYSTEM_RECURSIVE_ENTRIES`, `MCP_MAX_FILESYSTEM_RECURSIVE_DEPTH`, `MCP_MAX_FILESYSTEM_AGGREGATE_BYTES`, `MCP_MAX_FILESYSTEM_STAGING_BYTES`, `MCP_MAX_FILESYSTEM_PACKAGE_PREVIEWS`, `MCP_MAX_FILESYSTEM_PACKAGE_PREVIEW_BYTES`, and `MCP_FILESYSTEM_PACKAGE_PREVIEW_TTL_SECONDS`. Global `MCP_MAX_FILE_BYTES`, `MCP_MAX_FINGERPRINT_ENTRY_DETAILS`, and `MCP_MAX_OUTPUT_BYTES` also apply. Path strings have a fixed v1 structural bound of 32,768 UTF-8 bytes.

Expected complexity:

- non-recursive package preparation: `O(operations + bytes explicitly prepared)`;
- recursive operations: `O(entries + file bytes that must be fingerprinted/copied/backed up)`;
- retained memory: bounded by manifest, path/detail limits, staging buffers, and capability cache rather than total tree bytes;
- disk staging: bounded independently from in-memory limits.

Large trees should stream hashes/copies rather than materialize all file bytes in memory.

## Compatibility strategy

R24 deliberately replaces the four overlapping simple MCP mutation tools rather than exposing duplicate safety models. At R24 surface activation:

- remove public `create_directory`;
- remove public `copy_file`;
- remove public `move_file`;
- remove public `delete_file`;
- add read-only `filesystem_package`;
- add mutating `filesystem_package_apply` with a `previewId`-only schema.

The existing internal filesystem primitives behind the removed tools remain reusable implementation building blocks. No compatibility wrapper or alias keeps the old public names alive. This intentional breaking surface change ships in Scripthold `3.0.0` and is reflected in migration documentation, the authoritative tool catalog, runtime registration, README/TOOLS references, schema tests, and connector acceptance.

Text/encoding-aware tools such as `write_whole_file`, `edit_file`, `patch_package`, `convert_encoding`, and their R23 apply partners remain separate because they provide materially different contracts rather than duplicate namespace operations.

R24 uses the stable public `UNSUPPORTED` category for safe pre-mutation rejection such as cross-filesystem moves or platform namespace guarantees that cannot be implemented without weakening the contract. No compatibility path may reintroduce the mixed read-only/mutation annotation problem solved by R23.

## Implementation architecture

R24 must stay small enough to audit. It is a filesystem namespace workflow, not a generic transaction framework.

The preferred ownership split is:

- `filetoolsserver/handler`: strict MCP decoding, annotations, result/error mapping, capability entry points, and output limits only;
- `internal/security`: allowed-root authorization and missing-path evidence, including the nearest existing ancestor and enough retained path evidence to detect parent replacement or escape;
- `internal/filesystem`: reusable file/directory identity, same-volume detection, deterministic exact tree enumeration, durable staging/sync/no-replace namespace primitives, exact non-recursive removal, and platform-specific implementations;
- `internal/backupstore`: existing batch preflight/capture authority plus an R24 source-operation category when needed;
- one small transport-independent R24 planner/executor package: typed manifest validation, operation conflict analysis, retained plan state, apply revalidation, commit ordering, and post-failure classification. Do not create a general-purpose transaction DSL or duplicate filesystem/security primitives inside handlers.

Existing primitives must be reused where their guarantees match R24. New abstractions are justified only for gaps demonstrated by focused tests, especially real-directory identity, exact recursive scopes, same-volume checks, directory staging/publish, and recursive exact deletion.

### Retained recursive evidence

A recursive scope must retain enough deterministic evidence to prove that apply is acting on the prepared tree without retaining whole file contents in memory. At minimum each approved entry needs canonical relative path, entry kind, stable identity where the platform provides one, regular-file size and SHA-256 content evidence, and any parent/root identity required for revalidation. File bytes are streamed.

The complete approved recursive set is retained inside the capability. Preview output may use a bounded deterministic summary plus counts, bytes, aggregate fingerprint, and bounded entry details; the internal destructive scope itself may never be truncated. Hard links inside a recursive tree may appear as distinct namespace entries and may share object identity; they are not followed as links and directory copy does not promise to preserve hard-link topology. Duplicate top-level package operands that alias the same object remain conflicts.

### Metadata contract

R24 v1 exposes no caller-controlled permission, owner, ACL, xattr, alternate-stream, sparse-file, compression, timestamp, or hard-link options. Copy/move operations must preserve exact file bytes. Native move keeps the metadata semantics of the platform rename primitive. Copy may preserve additional metadata only where the implementation already has a safe cross-platform contract and tests prove it; public documentation must otherwise make no such promise. `mkdir` and `createFile` use the existing secure creation policy without adding a metadata mini-language.

## Required tests

R24 implementation must include focused and regression coverage for:

### Manifest and bounds

- unknown fields/types;
- empty/no-op packages;
- duplicate/aliased paths;
- oversized operation arrays, path lengths, contents, recursive trees, depths, retained details, and outputs;
- deterministic ordering and repeated preview results for unchanged state.

### Path security

- `..` and lexical aliases;
- symlinks, junctions, reparse points, hard links, Windows case/short-path aliases;
- source/destination overlap;
- missing ancestors;
- protected backup/task-store roots;
- special files and unsupported entry types.

### Recursive copy/delete

- empty trees;
- deep trees within limits;
- large bounded trees;
- mixed files/directories;
- escaping links inside trees;
- destination collisions;
- newly created/removed/replaced descendants after preview;
- cancellation at every traversal/copy/delete phase;
- injected read/write/sync/rename/remove failures.

### Moves and namespace changes

- file and directory rename;
- destination created concurrently;
- source replaced concurrently;
- same-source/destination aliases;
- cross-volume behavior;
- parent replacement/race cases;
- post-operation state classification.

### Backup integration

- required capture before every destructive delete of regular-file content;
- aggregate recursive quota preflight;
- capture failure prevents destructive mutation;
- backups survive later package failure;
- no backup on preview or no-op;
- backup IDs and source-operation metadata remain correct and bounded.

### Platform and regression

- Windows/Linux/macOS native namespace behavior;
- long paths where supported;
- directory sync/write-through guarantees;
- race detector where available;
- stdio/HTTP schema and result equivalence;
- connector smoke demonstrating ordinary filesystem refactors no longer require shell/script fallback;
- complete repository security/static/vulnerability/documentation/build gates applicable to the change.

## Devil's advocate findings

### Risk: recursive delete becomes a high-impact data-loss primitive

Mitigation: deletion scope is fully enumerated during read-only preview, bounded, identity-bound, revalidated, and cannot absorb new descendants. Required backup policy is captured before irreversible loss. Apply accepts only the prepared capability.

### Risk: a "filesystem package" becomes an arbitrary command language

Mitigation: the manifest contains a closed set of typed filesystem operations with unknown fields/types rejected. It accepts no executable, command line, shell expression, environment override, or hook.

### Risk: multi-operation packages imply false atomicity

Mitigation: all feasible preflight/staging occurs first, commit order is deterministic, partial states are inspected from actual filesystem evidence, and the API explicitly reports `PARTIAL_COMMIT`/equivalent rather than claiming rollback.

### Risk: recursive operations cross links or aliases after preview

Mitigation: secure traversal, stable identities, parent/path revalidation, no-follow behavior, and conflict on newly appearing or replaced entries. Platform-specific alias and reparse tests are release blockers.

### Risk: backing up an entire delete tree exhausts the store

Mitigation: conservative aggregate quota admission occurs during preview/apply before deletion. Required policy fails closed; no implicit GC or partial destructive phase is allowed merely to free space.

## Completion gate

R24 is complete only when:

1. `filesystem_package`, `filesystem_package_apply`, `filesystem-package-v1`, all seven strict operation schemas, hard limits, and `UNSUPPORTED` behavior are documented and test-defined;
2. typed package preview is physically read-only and apply accepts only `previewId`;
3. public `create_directory`, `copy_file`, `move_file`, and `delete_file` are removed with no duplicate compatibility wrappers;
4. `mkdir`, raw-byte `createFile`, `copyFile`, `copyDirectory`, same-volume `move`, `deleteFile`, and exact-scope `deleteDirectory` are implemented within the approved bounded contract;
5. recursive copy/delete are complete, deterministic, no-follow, limit-fail-closed, and reject unsupported link/special/nested-volume scope instead of silently skipping it;
6. no package path uses arbitrary shell/script execution internally and cross-filesystem moves are not emulated through shell or copy-plus-delete;
7. all allowed-root/link/alias/protected-root/missing-ancestor/parent-identity invariants are preserved;
8. every irreversible deletion of existing regular-file bytes completes required persistent backup capture and verification before the destructive phase;
9. durable staging/no-replace publication is verified for creation/copy paths and destination races cannot overwrite existing objects;
10. one-shot capabilities are bounded, replay-safe, restart-invalidated, and concurrency-tested;
11. deterministic partial-state evidence is implemented without false whole-package atomicity or rollback claims;
12. stdio/HTTP schemas and MCP annotations remain consistent and the tool catalog stays within its serialized budget;
13. connector-level use demonstrates ordinary filesystem namespace work without script fallback or duplicate simple tools;
14. focused TDD, failure-injection, concurrency/race, platform, static-analysis, vulnerability, documentation, catalog, and repository regression gates pass as applicable.

## Relationship to later milestones

R24 provides safe workspace mutation primitives and does not implement source-code understanding. R25/R27 later added read-only source intelligence; those capabilities do not weaken R24's path, backup, preview/apply, or partial-state contracts.
