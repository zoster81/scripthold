# Backup Store Recovery Design

## Status

**APPROVED — R26 PLANNED DESIGN BASELINE.** This document records the approved direction for recovery, salvage, and repair planning beyond the completed R19 diagnostic-only command. R26 is not active while an earlier release-scoped milestone is active unless maintainers explicitly reprioritize the roadmap.

The current production contract remains unchanged: normal backup-store startup fails closed on structural corruption, and `scripthold backup-store diagnose` is read-only and authorizes no repair. R26 must not weaken those guarantees merely to improve availability.

## Problem

R18 created a fail-closed persistent backup store with immutable content-addressed objects and manifests, a rebuildable derived index, explicit restore/GC, and crash-consistency invariants. R19 added offline diagnosis of an existing store without mutation.

When R19 reports damage that normal startup cannot safely accept, the operator currently receives evidence but no Scripthold-managed recovery path. Examples include:

- malformed or missing manifests;
- missing/corrupt referenced objects;
- inconsistent or unknown residual entries;
- permission/link-state damage;
- stale or corrupt derived metadata that cannot be trusted automatically;
- partially recoverable stores containing both valid and invalid backup records.

R26 provides a **separately reviewed offline recovery workflow** that can salvage trustworthy backup evidence without converting uncertain bytes into valid records silently.

## Goals

R26 will:

- operate offline under the same exclusive store-lock boundary as diagnostics;
- preserve the original source store as evidence during diagnosis and planning;
- distinguish repairable derived state from authoritative-data corruption;
- produce a deterministic, bounded recovery plan before any recovery output is created;
- prefer reconstruction/salvage into a separate destination store or quarantine/recovery area rather than silently rewriting the original store;
- verify every object and manifest admitted into recovered authoritative state;
- preserve provenance explaining why each record was accepted, rejected, quarantined, reconstructed, or omitted;
- never invent missing file bytes, checksums, backup IDs, source paths, timestamps, or pin state;
- retain immutable-object and immutable-manifest principles in recovered stores;
- preserve owner-only permissions, one-writer locking, protected-root separation, and path redaction;
- provide bounded operator-readable and machine-readable evidence;
- support cancellation and crash-safe restart/retry semantics;
- define explicit completion/partial-recovery states rather than a binary "fixed" claim.

## Non-goals

R26 will not:

- make normal server startup auto-repair structural corruption;
- turn R19 diagnostics into a mutating command;
- silently rewrite the original store in place;
- delete uncertain source evidence automatically;
- fabricate missing object bytes from manifests or metadata;
- accept a digest mismatch because the file "looks plausible";
- reconstruct target-file contents from diffs, logs, Git, or unrelated workspace files unless a separately approved future design explicitly introduces that evidence source;
- expose backup object bytes or internal paths through MCP;
- weaken object hashing, manifest checksums, hard-link checks, permissions, lock ownership, or protected-root rules;
- claim secure deletion of corrupt/quarantined material;
- run against a live server-owned backup store;
- automatically deploy or switch a server to the recovered store.

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

Exact final command names are deferred, but the conceptual flow should resemble:

```text
scripthold backup-store recover-plan --store <source> [bounded options]
scripthold backup-store recover-apply --store <source> --plan <capability-or-plan> --destination <new-store>
```

The implementation may choose a one-process capability or an explicitly persisted signed plan format. That decision must be made before implementation because offline recovery may need to survive process restart. In either case, apply must execute exactly the reviewed plan or fail stale/conflict validation.

The command must never infer the source store from ambient `MCP_BACKUP_STORE_DIR` unless an explicit reviewed compatibility decision says otherwise. Explicit paths reduce accidental targeting.

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

A recovered active backup manifest can be created only from sufficient authoritative evidence. Unknown bytes never become trusted because of filename alone.

## Salvage rules

### Healthy authoritative records

A manifest/object pair that passes the complete current validation contract may be copied into the recovered store with its logical backup identity preserved where doing so remains format-valid and collision-safe.

### Derived index corruption

The index is rebuildable derived state. A recovered store should regenerate it from recovered trusted manifests/objects rather than copy an untrusted index.

### Missing/corrupt referenced objects

A trusted manifest whose object is missing or fails digest verification cannot become a valid live backup in the recovered store. The recovery report must retain bounded evidence that the backup record was omitted/unrecoverable.

The system must not rewrite the manifest to point at different bytes.

### Orphan objects

A cryptographically valid orphan object is not automatically a backup because target path, creation time, source operation, and other manifest authority are absent. It may be copied into a quarantine/evidence area or reported as salvageable raw evidence according to the final design, but it must not be promoted to a live manifest without authoritative metadata.

### Malformed manifests with apparently valid fields

Partial JSON or recoverable-looking fields are not sufficient to create authoritative records unless the final R26 design defines a narrowly provable reconstruction rule. The default is quarantine/report, not guessing.

### Permissions/link damage

Recovery may read unsafe source entries only when the existing-store security boundary can do so without following aliases or accepting link ambiguity. A destination copy must be created with the correct owner-only permissions and single-link invariants; changing source permissions is not part of recovery.

## Destination store

The recovered destination must be a new explicit absolute path that:

- does not overlap the source store;
- does not overlap public allowed roots when later used as a server backup store;
- is created with owner-only permissions;
- has a fresh validated store descriptor unless format-preserving reconstruction requires a specifically reviewed identity rule;
- uses immutable no-replace object/manifest installation;
- is never partially presented as a healthy completed store without a final full audit.

The destination must record recovery provenance in a way that does not alter the immutable public manifest semantics or expose source paths. Exact provenance format is an R26 implementation design decision.

A failed recovery apply may leave a recognizable incomplete recovery destination, but it must never be mistaken for an adopted healthy store. Restart/retry semantics must be deterministic.

## Plan/apply contract

### Recovery plan

Planning is read-only and must return:

- source format/generation evidence where trusted;
- counts/bytes of trusted records;
- counts of rejected/unrecoverable records by stable reason;
- derived-state issues;
- orphan/unknown evidence counts;
- expected destination object/manifest counts and bytes;
- deterministic ordered recovery actions;
- bounded warnings;
- a plan identity bound to the exact source evidence.

The plan must be invalidated by source changes between plan and apply.

### Recovery apply

Apply must:

1. re-acquire/retain the exclusive existing-source lock;
2. revalidate source root/lock identity;
3. validate the exact reviewed plan and source evidence;
4. validate/create the explicit destination under strict non-overlap rules;
5. copy and fully verify trusted objects;
6. recreate/copy only trusted manifests under no-replace semantics;
7. build derived state from recovered authoritative records;
8. sync all durable destination state;
9. perform a complete full audit of the destination;
10. return final recovered/rejected counts and audit evidence.

Any source mismatch before destination commitment must fail as stale/conflict. Any destination partial state after a failure must be reported explicitly and remain distinguishable from successful completion.

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

R26 may use the same conceptual plan/apply discipline established by R23, but an unhealthy backup store is a distinct internal security authority and must not be manipulated through ordinary R24 workspace filesystem tools. Recovery retains its own offline lock, protected-root, and evidence-preservation boundary.
