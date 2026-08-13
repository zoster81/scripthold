# Backup Store Recovery Design

## Status

**ACTIVE — R26 IMPLEMENTATION HANDOFF.** R26 was explicitly activated on 2026-08-13 after the completed R25 delivery reached `main` and its exact push-event CI/release-candidate gate passed. This document is now the binding sequential implementation handoff for recovery and salvage beyond the completed R19 diagnostic-only command. R27 remains `PLANNED` and must not be implemented incidentally.

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
11. **Bounds reuse existing policy where possible.** Manifest, object, and byte defaults/hard ceilings reuse the current backup-store limits for equivalent dimensions. R26 may add only the minimal independent caps required for retained issues, plan/report bytes, staging state, and elapsed work; limits that prevent complete verification make a plan non-applicable.
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

## Sequential TDD implementation plan

Every phase is mandatory and sequential. A new implementation chat must read the required project context, this document, `OFFLINE_BACKUP_DIAGNOSTICS.md`, `PERSISTENT_BACKUP_LIFECYCLE.md`, `SAFE_FILESYSTEM_OPERATIONS.md`, `ROADMAP.md`, `DEVELOPMENT_CHECKLIST.md`, applicable `AGENTS.md` files, and the private operator backlog; then inspect repository state and continue from the first incomplete phase. Do not start R27 or skip a failing earlier phase.

### Phase 0 — activation, baseline, and invariant map

- confirm R26 is the sole `ACTIVE` release-scoped milestone and R25 is complete;
- record branch, `HEAD`, `origin/main`, working-tree/unrelated changes and current backup-store format invariants;
- map R19 diagnostic opener/scanner, descriptor/manifest/index codecs, owner-only ACL/mode helpers, locking, durable filesystem primitives, and CLI dispatch;
- run focused backup-store/diagnostic baselines before code changes.

Exit criterion: module/security map is established and relevant baselines are green or pre-existing failures are isolated.

### Phase 1 — freeze CLI, schemas, bounds, and stable outcomes

Add RED tests for the exact commands/arguments above; strict `backup-recovery-plan-v1`, `backup-recovery-state-v1`, and `backup-recovery-report-v1` codecs; deterministic/path-free serialization; canonical `planId`; plan applicability/omission states; exit-code mapping; and reuse of existing backup-store limits. Fuzz every new strict decoder.

Exit criterion: contract tests fail only because R26 implementation is absent and no public MCP/catalog change is required.

### Phase 2 — mutation-free source evidence scanner

Factor/reuse the R19 existing-store security authority to enumerate descriptor, manifests, objects, derived state, recognized residue, and unknown entries without initialization or cleanup. Classify trusted/untrusted/missing/orphan evidence deterministically, retain bounded issues, fully hash only within explicit bounds, and compute the recovery evidence digest independent of source index state.

Exit criterion: healthy and corrupt fixtures classify deterministically while byte/namespace/metadata/identity snapshots prove zero source mutation on success, failure, limit, and cancellation.

### Phase 3 — deterministic recovery planner and persisted plan

Build ordered accepted/rejected actions from Phase 2 evidence, preserve no target paths in serialized actions, compute canonical `planId`, mark limited/incompatible scans non-applicable, and atomically/no-replace write owner-only plan files outside the source store. Re-read the written plan through the strict decoder and verify identity before returning success.

Exit criterion: repeated plans over identical source evidence are semantically identical; any source evidence change produces a different identity or conflict.

### Phase 4 — destination authorization, staging identity, and restart state

Implement canonical non-overlap/alias checks for source, destination, plan, report, staging and state; derive deterministic same-parent staging/state names; create fresh destination store identity only in staging; and implement strict resume rules. Test symlink/junction/reparse, hard-link, case/8.3/macOS alias, pre-existing destination, unknown residue, wrong-plan state, and crash checkpoints.

Exit criterion: destination work cannot target/alias the source, expose an unaudited requested destination, overwrite unknown state, or resume foreign staging.

### Phase 5 — verified object reconstruction

For each accepted action, stream source object bytes through complete SHA-256/size verification, enforce copy/space/limit/cancellation bounds, install immutable owner-only destination objects no-replace, and verify any exact already-staged object before reuse. Never copy orphan/untrusted bytes into authoritative destination object state merely because they are present.

Exit criterion: success/dedup/retry are byte-exact; corruption, replacement, partial write, short read, disk-full, sync and cancellation failures remain explicit and source-immutable.

### Phase 6 — manifest reconstruction with preserved logical identity

Re-read/revalidate each accepted source manifest and reconstruct it with every authoritative field preserved except fresh destination `StoreID` and recomputed `ManifestChecksum`. Reject collisions, mismatched plan evidence, malformed IDs, impossible metadata, or any record that cannot be represented exactly.

Exit criterion: destination manifests round-trip through the current strict decoder, preserve backup identity/metadata, and reference only verified destination objects.

### Phase 7 — derived index rebuild and full staged audit

Rebuild index/generation solely from destination authoritative records, persist/sync it through existing durable rules, and run the complete full audit over staging. A limited audit or any issue blocks promotion. Verify expected manifest/object/pin/byte counts against the plan.

Exit criterion: only a completely audited staged store is eligible for promotion; stale/corrupt source index state has no authority over the result.

### Phase 8 — promotion, report, and idempotent completion

Promote staging to the absent requested destination with native same-volume no-replace rename, handle plan-bound post-promotion retry, write the strict owner-only no-replace provenance report, independently reopen/full-audit the final destination, and finish recognized recovery state. Report `recovered` versus `recovered_with_omissions` truthfully.

Exit criterion: command success implies a durable report plus a fully audited destination; interruption at every boundary has a deterministic retry/conflict result and never rewrites source evidence.

### Phase 9 — stale-plan, concurrency, cancellation, and failure injection

Cover source changes before/during re-scan, active writer/lock contention, destination races, plan/report replacement, cancellation during hashing/copy/audit, permission/quota/disk-full/sync/rename failures, and every durable phase transition. No failure path may claim rollback that was not performed.

Exit criterion: stale/partial/cancelled states are classified exactly and race tests show no competing recovery state transitions.

### Phase 10 — offline CLI wiring and operator output

Wire the strict parsers into `runCommand` before transport startup. Keep normal output path-free/content-free, honor `--pretty`, never read `MCP_BACKUP_STORE_DIR`, never start MCP, and keep usage/validation errors distinct from completed recovery evidence. Add end-to-end command tests for plan review across process restart and apply through report creation.

Exit criterion: native CLI smoke proves the complete offline plan/restart/apply workflow without server startup or source mutation.

### Phase 11 — adversarial/platform conformance

Run Windows/Linux/macOS-focused coverage for owner-only permissions, long/case paths, 8.3/macOS aliases, symlink/junction/reparse/hard-link/special-file attacks, Unicode paths/metadata, malformed/oversized JSON and identifiers, unsupported formats, huge sparse/orphan namespaces, and concurrent filesystem replacement. Snapshot the source before/after every adversarial case.

Exit criterion: platform-specific path/permission behavior never weakens source immutability, destination separation, or fail-closed evidence classification.

### Phase 12 — full repository and release-candidate gate

Review the complete diff and confirm no unrelated changes. Run formatting, focused tests, `go mod verify`, full normal and race suites, `go vet`, Staticcheck, govulncheck, relevant fuzz targets, documentation/link/catalog/project-identity checks, Gitleaks, `git diff --check`, offline CLI/native smoke, and all supported Windows/Linux/macOS amd64/arm64 build gates required by changed code. Use GitHub native/container CI as the authoritative cross-platform runtime gate where local execution is unavailable.

Exit criterion: every applicable local and exact pushed-commit CI gate is green with no unresolved security/correctness failure.

### Phase 13 — completion documentation and R27 handoff

Update this document from implementation handoff to completed contract/verification record, move concise outcome to `ROADMAP_HISTORY.md`, update roadmap/README/changelog truthfully, confirm normal startup and R19 diagnostics remain unchanged, and perform an explicit R27 compatibility check without implementing R27. Do not build/activate a candidate, alter a launcher, tag, release, or deploy unless separately authorized.

Exit criterion: R26 completion evidence is reproducible from tracked docs/Git/CI, R27 remains the only next planned milestone, and the repository is clean after any authorized commit/push.

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
