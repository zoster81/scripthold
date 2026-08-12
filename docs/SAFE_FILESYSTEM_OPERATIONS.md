# Safe Filesystem Operations Design

## Status

**APPROVED — R24 PLANNED DESIGN BASELINE.** This document records the approved outcome and non-negotiable safety contract for R24. Implementation must not begin while another release-scoped milestone is active unless maintainers explicitly reprioritize the roadmap.

R24 exists to remove routine dependence on `task_run` shell/script fallback for filesystem changes that are currently outside Scripthold's dedicated safe operations. Exact final public tool names and versioned manifest names remain implementation-time compatibility decisions, but the capability model and safety requirements below are approved.

## Problem

The current dedicated filesystem surface deliberately supports a narrow set of safe operations:

- `create_directory` creates directories recursively;
- `copy_file` copies one regular file without replacement;
- `move_file` moves or renames one file or directory without replacement;
- `delete_file` deletes one file only;
- `write_whole_file`, `edit_file`, and `patch_package` mutate text under their own encoding-aware contracts;
- `patch_package` edits existing regular files but intentionally does not create, delete, move, or rename them.

This leaves ordinary repository-maintenance tasks such as recursive directory copy/delete, coordinated renames, file creation plus deletion, or a bounded refactor involving several namespace changes without a typed dedicated workflow. Falling back to an arbitrary shell command weakens reviewability and can bypass Scripthold's path confinement, deterministic preflight, conflict checks, durable mutation primitives, backup policy, and structured partial-state evidence.

R24 provides a typed filesystem-change package rather than turning shell execution into the normal mutation path.

## Goals

R24 will provide a bounded, declarative, preview/apply workflow that can safely coordinate common filesystem namespace changes, including:

- create directories;
- create files from explicitly supplied bounded content where the operation contract supports it;
- copy regular files;
- recursively copy real directories under explicit limits;
- move or rename files;
- move or rename directories;
- delete regular files;
- recursively delete real directories after enumerating the complete approved scope;
- coordinate several such operations in one ordered package;
- support overwrite/replace behavior only when the exact overwritten pre-state was inspected, approved, and protected according to the operation's backup policy;
- preserve R23's one-shot preparation capability: read-only preview returns `previewId`, apply accepts only that identifier;
- bind every source, destination, destructive target, and relevant parent namespace to the prepared state;
- expose deterministic planned effects and structured verification evidence;
- preserve allowed-root, symlink, junction, reparse-point, hard-link, missing-ancestor, and protected-root rules;
- integrate persistent backup before irreversible loss where the approved backup policy requires it;
- preserve stdio and Streamable HTTP equivalence.

## Non-goals

R24 will not:

- accept arbitrary shell commands, scripts, glob-expanded command strings, or executable names;
- become a generic build/deployment engine;
- modify files outside configured allowed roots;
- follow directory symlinks, junctions, or reparse points as recursive copy/delete shortcuts;
- claim whole-package atomicity when the operating system cannot provide it;
- claim automatic rollback unless a future separately reviewed transaction design can prove it;
- silently overwrite an uninspected destination;
- recursively delete a tree whose exact bounded scope was not prepared and approved;
- make Git history changes, commits, pushes, tags, branches, releases, or deployment actions part of a filesystem package;
- replace text-aware editing when encoding/BOM/line-ending preservation is required;
- expose the persistent backup store as an ordinary filesystem root.

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

The final public name is intentionally deferred, but the conceptual model is a versioned `filesystem-package` / `workspace-change` manifest with an ordered array of typed operations.

Each operation must have exactly one explicit type. The initial type family is expected to cover:

- `mkdir`;
- `createFile` or equivalent explicit file creation;
- `copyFile`;
- `copyDirectory`;
- `move`/`rename`;
- `deleteFile`;
- `deleteDirectory`;
- an explicitly reviewed replace/overwrite form where required.

Unknown operation types and unknown fields are rejected. The package must never infer an operation from ambiguous combinations of fields.

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

Traversal must be deterministic and cancellation-aware. Directory links and reparse points are not followed. Encountering an unsafe or unsupported special file must fail or be reported according to an explicitly reviewed policy; it must never be silently copied/deleted as though it were a regular file.

### Copy directory

A recursive copy must:

- require the destination tree state expected by the preview;
- prevent copying a directory into itself or its descendant;
- reject destination collisions unless an explicit replace policy covers the exact inspected entries;
- stage file bytes using the durable filesystem layer where practical;
- preserve only metadata explicitly promised by the API;
- create directories/files in deterministic order;
- verify copied file content and final tree evidence before success.

### Delete directory

A recursive delete is destructive and must be especially explicit.

Preview must return the bounded exact deletion set or a deterministic aggregate plus enough retained details for meaningful approval. Apply may delete only entries bound into that preview. New unexpected descendants, changed identities, replaced entries, or scope growth after preview cause `CONFLICT` rather than being swept into the deletion.

Directory deletion must proceed children-before-parent and return structured actual-state evidence after any failure.

## Move and rename semantics

Moves remain no-replace by default. Any overwrite variant is a distinct explicitly approved behavior, never an accidental consequence of a generic destination field.

Preview binds:

- source identity and type;
- source fingerprint/evidence where applicable;
- destination expected state;
- parent identities relevant to the namespace change;
- cross-device implications detected before apply where possible.

Cross-filesystem directory moves cannot be represented as falsely atomic renames. If supported, they must be modeled explicitly as a prepared copy-plus-verify-plus-delete workflow with partial-state semantics and required source protection. Otherwise they fail as unsupported before mutation.

## File content creation and replacement

R24 is primarily a namespace package, not a second text editor.

When a package creates a file from caller-supplied content, the design must define whether the content is raw bytes or encoded text. It must not silently reinterpret text encodings. For encoding-aware modifications of existing text, callers should use the R23 edit workflow or a package composition mechanism that delegates to the shared text preparation pipeline.

Overwrite of an existing file requires the exact existing state to be part of preview and subject to required backup policy before replacement.

## Backup integration

R24 must reuse the persistent backup authority rather than inventing an unrelated backup format.

The baseline rules are:

- preview performs only read-only backup/quota admission checks;
- apply performs required backup capture before the first irreversible loss of approved existing file bytes;
- delete/overwrite operations that would destroy existing regular-file content are eligible for required persistent capture;
- recursively deleting a directory requires capture of every in-scope regular file that the approved backup policy covers before deletion begins, subject to bounded aggregate quota admission;
- a failed required capture prevents the destructive phase;
- move/rename that preserves the same file bytes does not automatically imply duplicate backup, but any fallback operation that deletes the original after copying must define protection explicitly;
- no logical no-op creates a backup;
- backups improve recovery evidence but do not create a false whole-package rollback guarantee.

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
- backup policy and quota reservation evidence;
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
- target reinspection before destructive deletion/overwrite;
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

The implementation must publish hard bounds before activation.

Expected complexity:

- non-recursive package preparation: `O(operations + bytes explicitly prepared)`;
- recursive operations: `O(entries + file bytes that must be fingerprinted/copied/backed up)`;
- retained memory: bounded by manifest, path/detail limits, staging buffers, and capability cache rather than total tree bytes;
- disk staging: bounded independently from in-memory limits.

Large trees should stream hashes/copies rather than materialize all file bytes in memory.

## Compatibility strategy

R24 should complement existing simple tools rather than remove them automatically.

The activation design must decide:

- final public package tool names;
- whether simple `copy_file`, `move_file`, `delete_file`, and `create_directory` remain convenience tools;
- how R23 annotations apply to preview/apply surfaces;
- whether package format is exposed as one versioned manifest or several narrower tools;
- how backup defaults from R23 flow into package preparation;
- whether any public error additions are necessary.

No compatibility wrapper may reintroduce a mixed read-only/mutation annotation problem solved by R23.

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

- required capture before destructive delete/overwrite;
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

1. final public names, schemas, limits, and compatibility decisions are documented;
2. typed package preview is physically read-only and apply accepts only `previewId`;
3. create/copy/move/delete plus recursive directory copy/delete are implemented within the approved bounded contract;
4. no package path uses arbitrary shell/script execution internally;
5. all allowed-root/link/alias/protected-root invariants are preserved;
6. destructive operations integrate required persistent backup before irreversible loss;
7. deterministic partial-state evidence is implemented without false atomicity/rollback claims;
8. stdio/HTTP and MCP annotations remain consistent;
9. connector-level use demonstrates that the intended filesystem tasks can be completed through dedicated tools rather than script fallback;
10. focused, failure-injection, concurrency/race, platform, static-analysis, vulnerability, documentation, and repository regression gates pass as applicable.

## Relationship to later milestones

R24 provides safe workspace mutation primitives. It does not implement source-code understanding. R25/R27 may use R24 for prepared filesystem refactors, but source intelligence cannot weaken R24's path, backup, preview/apply, or partial-state contracts.
