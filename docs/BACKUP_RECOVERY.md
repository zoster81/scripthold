# Backup Store Recovery Contract

## Status

R26 is **COMPLETE**. This document defines the final offline evidence-preserving backup recovery contract completed in 2026-08.

## Scope

Normal backup-store startup remains fail closed on structural corruption and `backup-store diagnose` remains read-only. Recovery is a separate explicit offline `recover-plan` / `recover-apply` workflow: the source store is immutable evidence, the persisted plan is strictly decoded and recomputed under the existing-store lock, only fully trusted manifest/object records are reconstructed, and output is built in a separate owner-only destination that must pass a full audit before no-replace promotion.

Recovery never invents bytes or metadata, never silently rewrites the source, never auto-adopts the recovered store, and keeps uncertain/orphan/corrupt evidence explicit. Deployment or selection of a recovered destination remains a separate operator action.
## Recovery philosophy

The source store is evidence. R26 must favor **evidence preservation over availability**.

The baseline recovery sequence is:

```text
existing source store
    -> exclusive existing-store lock
    -> bounded immutable scan
    -> classify evidence
    -> deterministic recovery plan
    -> explicit recovery apply
    -> new/recovery destination state
    -> full integrity audit
    -> operator review
    -> separate explicit adoption/deployment decision
```

Recovery output is not automatically made active. Choosing a recovered store for a future server startup is a separate operator configuration/deployment action.

## Source-store mutation boundary

R26 planning must be strictly mutation-free with respect to the source store.

The approved baseline does **not** authorize generic in-place repair of authoritative source data. Any future in-place mode would require a separate explicit design/approval proving rollback and evidence-preservation semantics.

Recovery apply may read the source store and create a separate destination/recovery area under explicit operator control. It must not alter source manifests, source objects, source descriptor, source index, source staging/trash, permissions, link counts, or timestamps intentionally.

Tests must snapshot source namespace, bytes, metadata, and identities before/after both planning and recovery apply.

## Command boundary

R26 remains an offline CLI capability rather than an MCP action operating on an unhealthy store.

The public offline CLI names and required path arguments are frozen:

```text
scripthold backup-store recover-plan --store <source> --output <plan.json> [--max-manifests N] [--max-objects N] [--max-bytes N] [--pretty]
scripthold backup-store recover-apply --store <source> --plan <plan.json> --destination <new-store> --report <report.json> [--pretty]
```

All five path-bearing values are explicit absolute local paths. `recover-plan` and `recover-apply` never infer the source from `MCP_BACKUP_STORE_DIR`, never start an MCP transport, and never authorize a live-store mutation. Unknown, duplicate, empty, positional, or unsupported arguments fail closed.

The plan is a persisted strict `backup-recovery-plan-v1` JSON document so review may survive process restart. It is **not a bearer capability and is not trusted merely because it exists**: apply reopens the source under the exclusive existing-store lock, repeats the bounded evidence scan with the plan's frozen bounds, recomputes the deterministic plan identity, and requires semantic equality before creating or resuming destination state. No signing secret or hidden process-local capability is introduced. A source change therefore invalidates the plan without a TTL-dependent security assumption.

`--output` is created owner-only and no-replace after planning completes; its contents are path-free. A limited scan may produce review evidence but is marked non-applicable and cannot be consumed by `recover-apply`. `recover-plan` exits `0` only for a complete applicable plan, `2` for a complete command result that is non-applicable/limited and requires operator action, and `1` for usage, validation, I/O, locking, or other operational failure. `recover-apply` exits `0` only after a destination has been fully audited and the recovery report durably installed; a healthy recovered subset remains explicitly `recovered_with_omissions` in the report rather than being called a full recovery.

## Evidence classification

Every scanned item must be classified using deterministic evidence.

At minimum, recovery distinguishes:

- **trusted descriptor** — strict supported descriptor validated;
- **trusted manifest** — filename/schema/checksum/permissions/link state valid and referenced fields internally consistent;
- **trusted object** — expected regular-file type, permissions/link state, size, and full SHA-256 match;
- **trusted backup record** — trusted manifest referencing a trusted matching object;
- **derived-state-only issue** — source authoritative records are healthy but index or recognized disposable residue is stale/corrupt;
- **orphan trusted object** — valid hash-addressed object with no live trusted manifest reference;
- **untrusted manifest/object/residue** — evidence insufficient or contradictory;
- **missing evidence** — a manifest references an object that does not exist or cannot be verified.

A recovered active backup manifest can be created only from sufficient authoritative evidence. Unknown bytes never become trusted because of filename alone. A trusted supported descriptor is required for an applicable recovery plan; missing, malformed, or unsupported descriptor evidence may be diagnosed/reported but cannot be used to guess a store format.

## Salvage rules

### Healthy authoritative records

A manifest/object pair that passes the complete current validation contract may be reconstructed in the recovered store with its logical backup identity preserved. `BackupID`, `CreatedAt`, `TargetPath`, `SourceOperation`, object algorithm/digest/bytes, content fingerprint, original mode/modification time, label, pin state, and supported format fields remain exact. Only the fresh destination `StoreID` is substituted and `ManifestChecksum` is recomputed over that destination manifest. Any collision or field that cannot be preserved exactly rejects that record rather than inventing replacement metadata.

### Derived index corruption

The index is rebuildable derived state. Recovery regenerates it from recovered trusted manifests/objects rather than copying an untrusted source index.

### Missing/corrupt referenced objects

A trusted manifest whose object is missing or fails digest verification cannot become a valid live backup in the recovered store. The recovery report must retain bounded evidence that the backup record was omitted/unrecoverable.

The system must not rewrite the manifest to point at different bytes.

### Orphan objects

A cryptographically valid orphan object is not automatically a backup because target path, creation time, source operation, and other manifest authority are absent. It remains reported source evidence and is not promoted into live destination authority without a trustworthy manifest.

### Malformed manifests with apparently valid fields

Partial JSON or recoverable-looking fields are not sufficient to create authoritative records unless the final R26 design defines a narrowly provable reconstruction rule. The default is quarantine/report, not guessing.

### Permissions/link damage

Recovery may read unsafe source entries only when the existing-store security boundary can do so without following aliases or accepting link ambiguity. A destination copy must be created with the correct owner-only permissions and single-link invariants; changing source permissions is not part of recovery.

## Destination store

The recovered destination is a new explicit absolute path that must not exist when a new apply begins. It:

- does not overlap or alias the source store;
- does not overlap public allowed roots when later used as a server backup store;
- is created with owner-only permissions;
- receives a **fresh store ID** in a current validated descriptor;
- uses immutable no-replace object/manifest installation;
- is never partially presented at the requested destination path before a complete full audit.

Apply builds under a same-parent staging directory so final promotion is same-volume. The staging/state names are derived from a bounded SHA-256 key over the canonical destination identity plus `planId`; they do not embed the source path. A strict owner-only recovery-state record binds staging to the plan, destination identity, fresh destination store ID, and phase. Retry may resume only recognized state whose identities and already-written content revalidate exactly. Unknown/ambiguous residue is never deleted, adopted, or overwritten automatically.

After construction, apply removes/finishes temporary recovery state as required, rebuilds derived index state, performs a complete full audit, and promotes the audited staging directory to the still-absent destination with a native same-volume no-replace rename. A crash after promotion but before report installation is recoverable only through the same plan/state evidence and another full audit; an arbitrary pre-existing destination is always a conflict.

Recovery provenance does **not** add a new entry to `backup-store-v1`. `--report` names a mandatory owner-only, strict, no-replace `backup-recovery-report-v1` sidecar outside both source and destination stores. The plan plus report form the audit record: the plan carries the accepted/rejected evidence and exact actions; the report binds that plan to the fresh destination store ID, resulting generation/counts, omission status, and final full-audit result. Ordinary plan/report JSON is path-free.

## Plan/apply contract

### Recovery plan

Planning is read-only with respect to the source and emits a strict persisted plan containing:

- `formatVersion = backup-recovery-plan-v1`;
- deterministic lowercase SHA-256 `planId`;
- trusted source store ID/format and descriptor fingerprint;
- a deterministic recovery evidence digest over the ordered source classification, independent of the derived index;
- the exact scan bounds and whether coverage was complete;
- counts/bytes of trusted records and expected destination state;
- deterministic accepted actions identified by backup ID, manifest checksum, object digest, and object bytes without exposing target paths;
- rejected/unrecoverable backup IDs where trustworthy plus stable reason codes/counts for other rejected evidence;
- derived-state, orphan, unknown, and bounded warning summaries;
- `applicable` and omission status.

The JSON decoder rejects unknown fields, duplicate/trailing data, malformed identities, inconsistent counts, unsorted/duplicate actions, and values outside fixed bounds. `planId` is computed from a canonical semantic encoding that excludes formatting and wall-clock time. Source changes between plan and apply change the recomputed evidence/plan identity and fail as stale/conflict.

### Recovery apply

Apply must:

1. strictly decode the persisted plan before filesystem mutation;
2. re-acquire/retain the exclusive existing-source lock and revalidate source root/lock identity;
3. repeat the bounded source scan with the plan's exact bounds and require the same canonical plan/evidence identity;
4. validate canonical source/destination/plan/report non-overlap and derive the recognized sibling staging/state identity;
5. create or exactly resume only plan-bound owner-only staging state;
6. copy every accepted object from the source through a bounded stream and verify its complete SHA-256/size before no-replace installation;
7. reconstruct only accepted manifests, preserving authoritative fields, substituting only the fresh destination store ID and recomputing the checksum;
8. rebuild the derived index from recovered authoritative records rather than copying source derived state;
9. sync durable staging state and perform a complete full backup-store audit;
10. promote the audited staging directory to the absent destination through native same-volume no-replace rename;
11. write the strict no-replace `backup-recovery-report-v1` provenance sidecar and finish recognized recovery state;
12. return bounded path-free recovered/rejected counts and final audit evidence.

Any source mismatch before promotion fails as stale/conflict. Any interrupted destination-side state remains explicitly recovery staging or a plan-bound post-promotion state and must never be mistaken for an automatically adopted store.

## Frozen implementation decisions

The following R26 decisions are binding. An implementation chat must not silently reopen them; a maintainer must explicitly revise this document and the roadmap before code diverges.

1. **Offline CLI only.** R26 adds `recover-plan` and `recover-apply` under `scripthold backup-store`; it adds no MCP tool, prompt, transport behavior, startup repair, or automatic adoption.
2. **Existing source is evidence.** Source access extends/factors the R19 existing-only diagnostic authority. It must not call ordinary `backupstore.Open` in a way that initializes, repairs, cleans, rewrites, or adopts source state.
3. **Persisted plan, deterministic revalidation.** `backup-recovery-plan-v1` survives restart but grants no authority by itself. Apply recomputes the same bounded plan under lock and requires exact semantic identity. No process-local preview, TTL, signature secret, or ambient store selection is part of R26.
4. **Trusted descriptor required.** R26 may report a missing/malformed/unsupported descriptor, but an applicable plan requires a strictly supported trusted descriptor. R26 does not guess a format or migrate versions.
5. **Only fully trusted backup records are promoted.** A live recovered backup requires a trusted manifest plus the exact fully hashed referenced object. Orphans, partial JSON, digest mismatches, and missing objects remain evidence only.
6. **Logical backup identity is preserved.** Recovered manifests preserve every authoritative field exactly except destination `StoreID`; `ManifestChecksum` is recomputed because that field changes. `BackupID` is never regenerated to hide a collision.
7. **Derived state is rebuilt.** Source index/staging/trash is never copied as authoritative recovery state. Destination index/generation is rebuilt from recovered manifests/objects.
8. **New destination, staged promotion.** The requested destination is never an in-place source repair and is not exposed until a same-parent staged store passes a full audit. Promotion is native same-volume no-replace rename.
9. **Restart is explicit and fail-closed.** Only exact plan-bound owner-only recovery state may resume. Every retained file is revalidated before reuse. Unknown residue or an unrelated pre-existing destination is a conflict, never an excuse for cleanup or overwrite.
10. **Provenance stays outside `backup-store-v1`.** The mandatory `backup-recovery-report-v1` sidecar plus the reviewed plan record what was accepted/omitted and prove the final audit without adding unknown files to the recovered store.
11. **Bounds reuse existing policy where possible.** Manifest, object, and byte defaults/hard ceilings reuse the backup-store limits for equivalent dimensions. Recovery adds only the independent caps required for retained issues, plan/report bytes, staging state, and elapsed work; limits that prevent complete verification make a plan non-applicable.
12. **No deployment side effect.** A successful apply creates a verified offline destination/report only. Launcher, environment, runtime, connector, release, or backup-store adoption remains a separate operator action.

## Recovery provenance

R26 must preserve an audit trail sufficient to answer:

- which source store generation/descriptor evidence was examined;
- which backup IDs were accepted;
- which were rejected and why;
- which objects were copied/deduplicated;
- which orphan/unknown entries were not promoted;
- whether any derived state was rebuilt;
- whether the destination passed full integrity audit.

Provenance must remain bounded and must not expose source/destination local paths in ordinary machine-readable support output. A local operator mode may display explicit paths only if separately documented as CLI-local behavior and never through MCP/HTTP logs.

## Security boundary

Recovery operates on potentially attacker-controlled/corrupt filesystem state.

It must preserve or strengthen:

- root identity retention;
- exclusive lock semantics;
- no symlink/junction/reparse following;
- hard-link ambiguity rejection;
- special-file rejection;
- bounded strict JSON decoding;
- content-addressed digest verification;
- owner-only destination permissions;
- no-replace installation;
- path/log redaction;
- no execution of data found in the store.

A corrupt manifest field is data, not a path authorization instruction.

## Resource bounds

Recovery can scan/copy large stores and therefore requires independent bounds for:

- manifests scanned;
- objects scanned;
- total bytes hashed;
- total bytes copied;
- issues retained;
- orphan/unknown evidence retained;
- output bytes;
- elapsed time/cancellation;
- destination staging/trash state.

Full object verification is mandatory for objects promoted into recovered authoritative state. If configured bounds prevent complete verification, the run cannot claim a fully recovered healthy store.

## Failure semantics

Recovery results must distinguish:

- complete successful recovery;
- successful recovery with intentionally omitted unrecoverable records;
- limited/incomplete scan;
- stale source conflict;
- destination partial creation;
- source corruption that prevents trustworthy format interpretation;
- cancellation;
- permission/I/O/space/quota failures.

"Recovered" must never mean "all original backups restored" unless evidence proves that all authoritative records were recovered.

## Relationship to normal startup

Normal `backupstore.Open` remains fail closed.

R26 must not make startup consume recovery plans, auto-select alternative objects, ignore invalid manifests, or silently quarantine corruption. Operators run recovery explicitly, review results, then separately configure a verified destination store if desired.

R19 diagnosis remains valuable before and after R26:

- before recovery, to understand source health;
- after recovery, to independently inspect the completed destination offline;
- as mutation-negative regression evidence.

## Required tests

### Source immutability

For every success/failure/cancellation path, snapshot and compare source:

- namespace;
- bytes;
- permissions;
- modification times;
- identities;
- link counts.

Planning and apply must not intentionally change source state.

### Evidence classification

- healthy stores;
- stale/missing index;
- valid manifest/object pairs;
- missing objects;
- digest mismatches;
- malformed/checksum-invalid manifests;
- duplicate IDs;
- orphan objects;
- unexpected staging/trash;
- permission/link/special-file failures;
- unsupported descriptor/version;
- source changes during scan/apply.

### Destination reconstruction

- fresh destination creation;
- source/destination overlap rejection;
- no-replace collisions;
- owner-only permissions;
- object deduplication;
- manifest preservation/ordering;
- derived index rebuild;
- failure at every stage/write/sync/rename;
- restart/retry after incomplete destination;
- final full audit mandatory before success.

### Security and limits

- symlink/junction/reparse/hard-link attacks;
- path traversal inside corrupted JSON fields;
- oversized manifests/objects/issues;
- cancellation during hashing/copy;
- disk-full/permission failures;
- no source/object bytes or internal paths leaked to normal logs/output;
- fuzz strict decoders and recovery-plan parser;
- race tests for lock/cancellation/state where applicable.

### Platform/repository gates

- Windows/Linux/macOS behavior;
- long-path/case/permission semantics;
- six supported build targets when implementation changes command code;
- complete Go/race/vet/static/vulnerability checks;
- documentation/link/secret scans;
- native offline CLI smoke on available platforms.

## Devil's advocate findings

### Risk: "repair" destroys the best remaining evidence

Mitigation: the baseline does not authorize in-place mutation. Source remains immutable and recovery is reconstructed into a separate destination with explicit provenance.

### Risk: orphan bytes are promoted into fake backups

Mitigation: object digest proves bytes, not backup metadata. Without a trustworthy manifest, orphan objects cannot become live backup records automatically.

### Risk: a recovered store is incomplete but presented as healthy

Mitigation: final status reports accepted/rejected counts explicitly and requires a complete full destination audit. Missing unrecoverable records remain visible in recovery evidence.

### Risk: corrupt JSON injects paths or identifiers into recovery operations

Mitigation: strict bounded schemas, canonical validation, source entries treated as untrusted data, destination paths derived only from validated digest/backup-ID forms, and no arbitrary filesystem path from corrupt metadata is followed without authorization.

### Risk: recovery becomes an automatic startup bypass

Mitigation: R26 is offline and explicit. Normal startup remains fail closed and never invokes recovery automatically.

## Completion gate

R26 is complete only when:

1. exact offline commands/plan format and restart semantics are documented;
2. source-store planning and recovery apply are proven mutation-free against the source;
3. trustworthy records can be reconstructed into a separate destination without inventing metadata or bytes;
4. unrecoverable and orphan evidence is classified explicitly rather than silently discarded/promoted;
5. destination store creation is crash-aware, owner-only, no-replace, and non-overlapping;
6. every promoted object is fully hashed and every promoted manifest passes the authoritative contract;
7. successful completion requires a full destination audit;
8. normal startup and R19 diagnosis remain fail-closed/read-only respectively;
9. failure-injection, cancellation, security-negative, race/platform, static/vulnerability, and repository regression gates pass as applicable;
10. adoption/deployment of a recovered store remains a separate explicit operator action.

## Relationship to R23/R24

R26 reuses the conceptual plan/apply discipline established by R23, but an unhealthy backup store is a distinct internal security authority and is not manipulated through ordinary R24 workspace filesystem tools. Recovery retains its own offline lock, protected-root, and evidence-preservation boundary.
