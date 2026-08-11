# Offline Backup Store Diagnostics Design

## Status

**COMPLETE.** This document is the authoritative R19 diagnostic-only contract. Existing-only lock acquisition, bounded fail-soft scanning, the offline JSON CLI, and the cross-platform verification gate are implemented. No repair, quarantine, deletion, migration, salvage, clone, or reconstruction capability is authorized.

This document defines the security boundary, command contract, data flow, limits, output schema, failure semantics, and verification gate for diagnosing an existing persistent backup store when normal server startup cannot open it safely.

The completed R18 subsystem remains authoritative for the store format, locking, immutable objects and manifests, derived index, restore, garbage collection, and crash-recovery invariants. R19 adds no MCP action and authorizes no repair, quarantine, deletion, migration, or reconstruction of backup data.

## Problem statement

A configured backup store intentionally fails closed when its descriptor, layout, manifests, referenced objects, permissions, links, or other structural evidence is unsafe. Normal startup returns only sanitized failure information. The diagnostic command provides bounded offline evidence without using the normal `backupstore.Open` path, because normal open may create an empty store, rebuild a derived index, or finish recognized GC-trash cleanup.

The offline diagnostic path:

- opens only an already existing store;
- acquires the same exclusive process lock without creating a missing lock file;
- performs no filesystem mutation, including index rebuild, permission repair, cleanup, or timestamp update;
- reports enough stable evidence to identify the failure category;
- never exposes object bytes, target contents, store paths, temporary paths, credentials, or capability secrets;
- remains usable even when normal server startup rejects the store;
- preserves the completed R18 fail-closed behavior for normal stdio and Streamable HTTP startup.

## Goals

The diagnostic command:

- distinguish descriptor, layout, permission, link, manifest, object, index, staging, trash, and limit failures;
- support quick metadata diagnosis and optional bounded full object hashing;
- return deterministic machine-readable JSON suitable for support and automation;
- retain bounded issue evidence rather than stop at the first structural issue;
- operate under the existing owner-only and single-writer security boundary;
- classify whether the store is safe for normal startup without attempting to make it safe;
- preserve the source store byte-for-byte and namespace-for-namespace;
- remain cross-platform across Windows, Linux, and macOS.

## Non-goals

Diagnostics never:

- repair or rewrite `store.json`;
- create a missing store, lock file, directory, descriptor, index, manifest, object, staging entry, or trash entry;
- rebuild or replace the derived index;
- remove recognized or unknown GC trash;
- remove staging files or orphan objects;
- quarantine, rename, chmod, chown, relink, copy, migrate, or salvage any store entry;
- rewrite manifest checksums, store identifiers, backup identifiers, target paths, pin state, or timestamps;
- restore public target files;
- add an MCP tool or action;
- run against a store currently held by a server or another diagnostic process;
- weaken normal startup validation or convert structural errors into warnings;
- claim that a healthy diagnostic result protects against a malicious actor with the same operating-system identity.

Any future repair or salvage workflow requires a separate approved design with its own failure/data-loss model. It must not be inferred from diagnostic evidence alone.

## Command boundary

The implemented command namespace is:

```text
scripthold backup-store diagnose --store <absolute-path> [options]
```

The command is selected before transport parsing and never starts an MCP server. It does not read `MCP_BACKUP_STORE_DIR`; the store path must be supplied explicitly so ambient server configuration cannot select a diagnostic target accidentally.

Implemented options:

- `--store <absolute-path>`: required existing store root;
- `--mode quick|full`: defaults to `quick`;
- `--max-objects <positive-integer>`: optional diagnostic object/manifest bound;
- `--max-bytes <positive-integer>`: optional full-hash byte bound;
- `--pretty`: optional indented JSON; compact JSON remains the default.

Unknown options, duplicate singleton options, positional directories, transport options, empty values, negative values, integer overflow, and trailing arguments fail before filesystem access.

The command writes exactly one JSON document to stdout. Human-readable startup or usage errors go to stderr. Logging remains on stderr and must not contain the store path or entry names.

## Existing-store and lock semantics

Diagnostics must use a dedicated existing-store opener rather than `backupstore.Open`.

The opener must:

1. require a non-empty absolute path;
2. reject filesystem roots, NUL bytes, lexical escapes, symlinks, junctions, reparse points, aliases, and uncertain existing components;
3. require the root to exist as a real owner-only directory;
4. require `store.lock` to exist as a single-link owner-only regular file;
5. open and exclusively lock the existing lock file without create flags;
6. fail with `CONFLICT` when another process owns the lock;
7. retain and revalidate root identity for the diagnostic lifetime;
8. release the lock on success, error, cancellation, and panic recovery;
9. never call directory creation, descriptor creation, layout creation, index persistence, or GC recovery code.

The same-process identity remains trusted to the same extent as R18. Diagnostics are not a sandbox and do not defend against concurrent out-of-band filesystem modification by a privileged or same-identity attacker. Stable identity checks must detect practical changes during the scan.

## Diagnostic data flow

The diagnostic scan is intentionally separate from normal startup sequencing:

1. parse and validate command arguments without filesystem access;
2. validate the existing root and acquire the existing exclusive lock;
3. retain root identity and validate owner-only root metadata;
4. read `store.json` with the strict bounded descriptor decoder;
5. when the descriptor is valid, inspect the expected layout without creating missing entries;
6. scan manifests, objects, index, staging, and trash under explicit limits;
7. optionally hash referenced objects in `full` mode;
8. compare the persisted index with a derived in-memory projection without writing it;
9. revalidate root and lock identity;
10. emit one bounded path-free report;
11. release all handles and the lock.

Descriptor failure does not authorize guessing format values or parsing manifests as another store version. The report may continue with format-independent root/layout evidence, but descriptor-dependent checks must be marked not performed rather than fabricated.

## Output contract

The versioned output format is `backup-diagnostic-v1`.

Top-level fields include:

```json
{
  "formatVersion": "backup-diagnostic-v1",
  "mode": "quick",
  "diagnosable": true,
  "safeForNormalOpen": false,
  "descriptorValid": true,
  "layoutValid": true,
  "generation": "...",
  "manifestCount": 0,
  "objectCount": 0,
  "referencedBytes": 0,
  "orphanObjectCount": 0,
  "orphanObjectBytes": 0,
  "stagingEntryCount": 0,
  "stagingEntryBytes": 0,
  "trashEntryCount": 0,
  "trashEntryBytes": 0,
  "indexConsistent": false,
  "checks": [],
  "issues": []
}
```

`checks` records deterministic high-level check status without paths:

```json
{
  "name": "descriptor",
  "status": "passed"
}
```

Allowed check states are `passed`, `failed`, `skipped`, and `limited`.

`issues` contains bounded stable records:

```json
{
  "code": "INDEX_REBUILD_REQUIRED",
  "scope": "index",
  "message": "derived index is missing, corrupt, or stale",
  "identifier": ""
}
```

The optional `identifier` may contain only a validated backup ID or object digest when returning it is necessary to distinguish repeated issues. It must never contain target paths, store paths, filenames, labels, file contents, temporary names, or arbitrary input strings. Duplicate issues should be coalesced with a bounded count where practical.

`diagnosable` means the command obtained enough trusted format evidence to run descriptor-dependent checks. `safeForNormalOpen` is true only when the exact normal-open structural invariants pass and no issue was truncated or skipped by a requested limit. A stale or missing derived index keeps `safeForNormalOpen` true only if normal `Open` can deterministically rebuild it; the report must still mark `indexConsistent: false` and return `INDEX_REBUILD_REQUIRED` as a non-structural maintenance issue.

## Bounds

Diagnostics must remain bounded independently of hostile store contents.

- Descriptor, manifest, and index byte limits reuse the R18 format maxima.
- Default object and manifest counts use the configured hard-safe R18 defaults when no server configuration is available.
- User-requested bounds may lower but never exceed repository hard maxima.
- Full hashing stops before exceeding `maxBytes` and marks remaining checks `limited`.
- Directory enumeration reads at most the applicable limit plus one entry.
- Issue retention is capped; truncation adds exactly one `LIMIT` issue.
- JSON output must remain below `MCP_MAX_OUTPUT_BYTES` when that environment value is valid, otherwise the normal documented default applies.
- No object bytes, manifest raw JSON, labels, target paths, or unbounded error strings are retained.
- Cancellation is checked between entries and during object hashing.

Quick mode complexity is `O(manifests + objects + residual entries)` metadata work under configured limits. Full mode adds `O(total referenced object bytes hashed)`. Retained memory is bounded by descriptor data, one manifest, bounded aggregate projections, issue limits, and output limits; complete object contents are never materialized.

## Error and exit semantics

The JSON report is the authoritative diagnostic result when argument parsing, root access, and lock acquisition succeed.

Exit codes:

- `0`: report emitted and `safeForNormalOpen` is true;
- `2`: report emitted, diagnosis completed, and one or more issues make normal open unsafe or maintenance is required;
- `1`: no trustworthy report could be emitted because arguments, root validation, lock acquisition, permissions, cancellation, or output serialization failed.

A diagnosis with bounded `limited` checks exits `2`, never `0`.

Errors must reuse stable internal operation kinds where possible:

- malformed options or unsupported modes: `INVALID_INPUT`;
- invalid or non-existing root: `INVALID_PATH`;
- linked, aliased, or escaping root: `SYMLINK_ESCAPE` or `ACCESS_DENIED`;
- active store lock or identity change: `CONFLICT`;
- owner or permission mismatch: `PERMISSION`;
- requested or observed limits: `LIMIT`;
- cancellation: `CANCELLED`;
- filesystem or decoding failures: `IO_ERROR`.

CLI stderr messages remain sanitized and must not print the supplied store path.

## Compatibility

- Existing `--version`, stdio invocation, Streamable HTTP invocation, positional allowed directories, environment defaults, and MCP schemas remain unchanged.
- The `backup-store` token becomes a reserved first positional command only when followed by a recognized offline subcommand. A normal allowed directory literally named `backup-store` remains usable after `--` or as an absolute path.
- No environment variable enables diagnostics implicitly.
- Normal server startup continues to use `backupstore.Open` and retains automatic derived-index rebuild plus recognized GC-trash cleanup.
- Diagnostic code may share strict decoders, bounded scanners, permission checks, and identity primitives, but must not call mutating startup helpers.

## Test strategy

### Command parsing

- valid quick and full commands;
- missing subcommand, missing store, duplicate options, unknown fields, invalid mode, zero/negative/overflow limits;
- `--version` remains unchanged;
- stdio/HTTP argument compatibility remains unchanged;
- an allowed directory named `backup-store` remains addressable through unambiguous syntax;
- no filesystem call occurs for malformed input.

### Existing-store boundary

- missing root, missing lock file, relative path, filesystem root, regular-file root, symlink, junction, reparse point, path alias, permissive permissions, wrong owner, special file, and hard-linked lock;
- active server lock and concurrent diagnostic lock fail with `CONFLICT`;
- lock acquisition creates no file and changes no metadata intentionally;
- root or lock replacement during diagnosis fails closed;
- lock and handles are released after success, failure, cancellation, and panic recovery.

### Diagnostic behavior

- healthy empty and populated stores;
- missing, malformed, unknown-field, oversized, and unsupported descriptor;
- missing or unexpected layout entries;
- valid, malformed, checksum-tampered, duplicate, oversized, permission-unsafe, and hard-linked manifests;
- missing, corrupt, size-mismatched, permission-unsafe, linked, hard-linked, and orphan objects;
- missing, corrupt, stale, and valid derived index without persistence;
- recognized GC trash, malformed recognized trash, unknown trash, capture staging residue, and nested/special residual entries;
- quick mode does not hash objects;
- full mode hashes exact bytes and detects digest mismatch;
- cancellation and byte/object/output limits produce deterministic partial evidence;
- issue order and JSON bytes are deterministic for the same store state;
- output contains no target path, store path, label, object bytes, raw manifest, temporary name, or lock identifier.

### Mutation-negative tests

For every diagnosis class, snapshot the complete store namespace, file bytes, modes, modification times, identities, and link counts before and after. The diagnostic command must leave them unchanged. The tests must explicitly prove that it does not:

- create a missing lock or index;
- rebuild a stale index;
- clean recognized GC trash;
- remove staging or orphan objects;
- repair permissions;
- create missing directories or descriptors.

### Platform and repository gates

- Windows ACL, junction, reparse-point, long-path, drive-root, and case-folding coverage;
- Linux/macOS owner, mode, symlink, hard-link, and directory-sync assumptions;
- race detector for lock and cancellation behavior;
- fuzz command option parsing and diagnostic issue decoding where useful;
- six-target command/test compilation;
- native external CLI smoke on available platforms;
- complete tests, vet, Staticcheck, govulncheck, workflow checks, Gitleaks, documentation links, and diff review.

## Implementation boundary

`OpenExistingForDiagnosis` requires an existing owner-only root and existing single-link lock, acquires that lock without create flags, retains root/lock identity, and exposes no capture, restore, GC, index-persistence, or normal initialization methods. The scanner reuses only bounded read-only format primitives after descriptor/layout trust is established; the CLI uses strict explicit arguments and never consults `MCP_BACKUP_STORE_DIR`.

Current evidence does not justify recovery-plan, quarantine, clone, repair, salvage, or migration capabilities. Any future mutation capability requires concrete operator evidence and a separately approved failure/data-loss model.

## Devil's advocate findings

### Risk: diagnostics accidentally repair the store

Reusing `backupstore.Open` could create missing state, rebuild the index, and clean recognized GC trash. Diagnostics therefore use a separate existing-only opener plus mutation-negative namespace snapshots. Helpers capable of writing are excluded from the diagnostic dependency graph.

### Risk: the command races a running server

Reading without the writer lock could produce inconsistent or misleading evidence. Diagnostics require the same exclusive lock, opened without create semantics, and fail when a server or another diagnostic process is active.

### Risk: fail-soft scanning weakens normal startup

A diagnostic report may continue after an issue, but normal startup must remain fail closed. The scanner returns evidence only; `backupstore.Open` continues to reject the first structural issue and never consumes a diagnostic `safeForNormalOpen` assertion as authority.

### Risk: output becomes a path or content disclosure channel

The report uses fixed codes, fixed scopes, bounded messages, and optional validated backup IDs or digests only. Tests inject private-looking paths, labels, bytes, and temporary names and require their absence from stdout, stderr, and logs.

### Risk: a limited scan incorrectly declares health

Any skipped, truncated, saturated, or byte-limited check forces `safeForNormalOpen: false` and exit code `2`. Only a complete scan under applicable bounds can return exit code `0`.

### Risk: reserving a command name breaks existing positional directories

The parser recognizes `backup-store` only as an explicit first-token command with a known subcommand. `--` and absolute path syntax preserve access to an allowed directory whose basename is `backup-store`. Compatibility tests protect historical invocation.

## Completion gate

R19 is complete only when an existing store can be diagnosed offline on all supported platforms without any filesystem mutation, active stores are rejected by the same exclusive lock boundary, healthy and corrupt states produce deterministic bounded path-free JSON, limited scans never claim health, normal stdio/HTTP behavior remains unchanged, and the complete focused, regression, race, static, vulnerability, six-target, native smoke, documentation, and security verification matrix passes.

## Completion record

Completed on 2026-08-06. The existing-only opener validates owner-only root and lock state, compares pre-acquisition lock identity with the acquired handle, retains root/lock identity, and excludes every mutating store method. The scanner snapshots descriptor bytes and required layout identities, revalidates them after bounded scanning, and returns fixed path-free checks and issues. Quick and full reports cover healthy, rebuildable-index, descriptor, layout, manifest, missing/corrupt object, permission, link, residual-state, limit, cancellation, and concurrent-change cases without modifying the store. The CLI uses explicit strict arguments, never reads `MCP_BACKUP_STORE_DIR`, emits one bounded JSON document, and returns exit `0`, `2`, or `1` according to the documented contract.

The complete Go suite, complete race detector, vet, Staticcheck, govulncheck, six fuzz campaigns, Node release tests, manual MCP harness, GoReleaser/workflow checks, six Windows/Linux/macOS amd64/arm64 command and test compilations, native Windows external CLI smoke, text/link/catalog identity checks, and Gitleaks history/content scans passed. No repair or mutation capability was added.
