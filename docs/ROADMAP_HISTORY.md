# Scripthold Engineering Milestone History

This document records concise public outcomes for completed milestones R1–R24. It is history, not the active planning surface: current and future milestone state belongs in [ROADMAP.md](ROADMAP.md), release-by-release user-visible changes in [CHANGELOG.md](../CHANGELOG.md), and detailed subsystem contracts in the linked design documents.

The record intentionally excludes private workstation paths, connector state, process identifiers, local binaries, and operator handoff details.

## R1 — Shared text-document core

Unified encoding, BOM, line-ending, decoding, encoding, and snapshot behavior behind a shared text-document pipeline. Read, write, edit, conversion, line-ending, and BOM operations gained consistent preservation semantics and multilingual regression coverage.

## R2 — Secure deterministic traversal

Introduced one deterministic recursive walker with resolved-root enforcement, missing-ancestor validation, cancellation, depth/exclusion controls, and Unix/Windows coverage for symlinks, junctions, reparse points, drive roots, UNC paths, and short-path aliases.

## R3 — Durable mutation layer

Replaced handler-specific writes with same-directory staging, synchronization, atomic/no-replace platform commits, optimistic conflict detection, and transactional `.bak` replacement with explicit rollback/recovery behavior. Failure injection covers staging, sync, commit, cleanup, permission, backup, rollback, and concurrent-change boundaries.

## R4 — Typed operation errors

Separated domain failures from MCP formatting and introduced stable transport-independent categories for invalid input, path/security failures, encoding, permissions, conflicts, cancellation, limits, I/O, and internal failures. Batch and single-tool mapping share the same vocabulary.

## R5 — Bounded ordered concurrency

Added a reusable worker coordinator that bounds in-flight work while preserving deterministic result order, cancellation semantics, partial results, and early stop. Batch reads and grep adopted it without unbounded result accumulation.

## R6 — Execution preparation and authoritative tool metadata

Centralized process preparation, bounded output, timeout, cancellation, environment filtering, and final script/work-directory verification while retaining distinct execution authorization. Added the embedded authoritative tool catalog consumed by runtime registration and release projection. Later R21 work replaced the synchronous public execution tools with the durable `task_run` family.

## R7 — Roadmap and documentation reset

Separated current planning, engineering history, contributor checks, scoped agent guidance, and private operator state. Public documentation became extension-independent and explicitly rejected the idea that domain-specific filenames influence encoding detection.

## R8 — Generic encoding detection

Completed conservative content-based detection with strict UTF-16 validation, decoded-text quality checks, binary rejection, round-trip evidence, deterministic confidence, explicit ambiguity, and consistent sample/chunked/full semantics. Identical bytes produce the same decision under unrelated filenames.

## R9 — Bounded-memory streaming pipeline

Moved read, batch, grep, conversion, line-ending, and BOM paths to bounded streaming or disk staging. Added decoded-line limits, aggregate output limits, early rejection for full-document operations, and benchmarks demonstrating source-size-independent working memory for representative streaming paths.

## R10 — Public API and compatibility cleanup

Used the 2.0 boundary to remove the deprecated `directory_tree` tool, normalize public field naming, stabilize error codes, make ambiguous non-empty text fail closed, separate resource limits, and document intentional 1.8-to-2.0 migration behavior in [MIGRATION_2.0.md](MIGRATION_2.0.md).

## R11 — Transport-independent server architecture

Separated configuration, CLI parsing, shared server construction, and transport startup. `BuildServer` became the single tool-registration and process-policy path; startup roots became process-wide authority, with dynamic client roots limited to the stdio compatibility case where no startup roots exist.

## R12 — Streamable HTTP security design

Approved the fail-closed native HTTP threat model in [HTTP_SECURITY.md](HTTP_SECURITY.md): loopback by default, bearer authentication on every MCP request, exact Host/Origin validation, no CORS, explicit TLS/proxy requirements for non-loopback exposure, bounded resources, redacted logging, and a second execution opt-in.

## R13 — Native Streamable HTTP

Implemented authenticated Streamable HTTP on the shared server path while preserving stdio. Added startup validation, Host/Origin/authentication middleware, TLS/trusted-proxy support, bounded request/session/rate state, health/readiness endpoints, graceful shutdown, and transport-equivalence/security tests.

## R14 — Hardening and Scripthold 2.0.0

Completed the first public 2.x release on 2026-08-02 after cross-platform, container, workflow, migration, documentation, packaging, and security gates. The published Windows runtime was exercised over stdio and authenticated HTTP; a controlled rollback to the retained pre-release baseline and restoration to `2.0.0` verified the rollback procedure.

## R15 — Agent ergonomics and project-aware workflows

Added optional line-numbered reads, richer paged grep, `.gitignore`-aware traversal, bounded sorting, batch conversion previews, strict unified patches, ambiguity-safe fuzzy edits, and three shared encoding workflow prompts. The work credited relevant concepts and implementation approaches from the original project while adapting them to Scripthold's architecture.

## R16 — Verified change workflows

Implemented deterministic `fingerprint_paths`, bounded one-shot `edit_file` preview/apply, strict `patch-package-v1` inspect/dry-run/apply/verify, and typed `verify_state`. The completed contract is maintained in [VERIFIED_CHANGE_WORKFLOWS.md](VERIFIED_CHANGE_WORKFLOWS.md).

## R17 — Persistent backup lifecycle design

Approved the dedicated non-overlapping store, one-writer model, immutable objects/manifests, derived index, bounded quotas, approval-bound capture, original-target restore with mandatory safety backup, explicit garbage collection, and no-automatic-rollback decisions documented in [PERSISTENT_BACKUP_LIFECYCLE.md](PERSISTENT_BACKUP_LIFECYCLE.md).

## R18 — Persistent backup implementation

Implemented the R17 contract: exact-byte capture, conservative reservations, bounded recovery/indexing, `backup_store` review/audit, approval-bound edit/package backups, one-shot restore, immutable pins, and explicit generation-bound garbage collection. The complete subsystem passed race, failure-injection, fuzz, cross-platform, native, and hardened-container verification.

## R19 — Offline backup-store diagnostics

Added the mutation-free `backup-store diagnose` path for an existing store. It acquires the existing exclusive lock without creating state, performs bounded deterministic quick/full diagnosis, reports path-free structured evidence, and never repairs, rebuilds, cleans, quarantines, migrates, or deletes store data. See [OFFLINE_BACKUP_DIAGNOSTICS.md](OFFLINE_BACKUP_DIAGNOSTICS.md).

## R20 — MCP 2026-07-28 adoption

Adopted MCP `2026-07-28` through the stable Go SDK while preserving supported legacy behavior. Modern stdio discovery and stateless same-endpoint HTTP coexist with retained roots-dependent legacy stdio and stateful legacy HTTP behind the same filesystem, authentication, resource, logging, and execution boundaries. See [MCP_2026_07_28_ADOPTION.md](MCP_2026_07_28_ADOPTION.md).

## R21 — Durable tasks and 2.1.x release consolidation

Replaced request-bound execution with the five-tool durable task family documented in [DURABLE_TASKS.md](DURABLE_TASKS.md). Added idempotent admission, a persistent bounded queue, independent supervisor/worker/executor topology, logical locks, bounded cursor logs, recovery, retention, and process-tree cancellation. Release validation was consolidated into one exact-pushed-commit `CI` release-candidate gate, and `2.1.1` was published, deployment-smoked, actively rolled back to `2.0.0`, and restored.

## R22 — Global encoding coverage and Scripthold 2.2.0

Expanded the authoritative registry to 168 canonical read/write encodings, promoted UTF-32 LE/BE to the full text pipeline, added verified pure-Go single-byte and multibyte/stateful codecs from pinned GNU libiconv evidence, hardened conservative detection, made grep/batch partial encoding failures explicit, and added a registry-driven public-operation matrix. Representative codec classes passed malformed-input, cancellation, limit, concurrency, fuzz, resource, race, static/vulnerability, native, container, and six-target release gates.

Scripthold `2.2.0` was published on 2026-08-11 from the exact commit that passed the complete push-event release-candidate gate. GoReleaser produced the normal binaries/archives/checksum manifest; MCPB bundles, their checksum manifest, the final MCPB-backed Registry manifest, and Registry publication were produced only by the GitHub release workflows. The detailed completed contract remains in [GLOBAL_ENCODING_COVERAGE.md](GLOBAL_ENCODING_COVERAGE.md).

## R23 — Truthful MCP mutation surface and backup UX

Separated the five historical mixed preparation/review tools from six dedicated mutating apply tools, with every apply schema accepting only a one-shot `previewId`. Added exact-result binding for BOM and encoding conversion, operator-default persistent backup policy, backup history/comparison, and backup-before-staging ordering for package/restore safety paths. The source-side gate included a registry-driven mutation-integrity matrix across all 168 encodings, full normal/race/static/vulnerability checks, deterministic encoding fuzzing, and six-target compilation. Connector acceptance on 2026-08-12 confirmed the separated schemas and operationally verified side-effect-free edit preview, exact apply, replay rejection, and rejection of the removed direct-edit form. The completed contract remains in [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md); the next-major caller migration is documented in [MIGRATION_3.0.md](MIGRATION_3.0.md).

## R24 — Safe filesystem operations

Replaced the four overlapping simple namespace mutation tools with read-only `filesystem_package` plus `previewId`-only `filesystem_package_apply`. The strict `filesystem-package-v1` surface coordinates no-replace directory/file creation, raw-byte file creation, file and exact recursive directory copy, native same-volume move, and backup-before-loss file/exact-directory deletion. Exact recursive scopes are deterministic and fail closed on links, special entries, nested volumes, limits, or post-preview drift; one-shot capabilities bind path/object/content/tree/parent evidence, and durable progress is reported through bounded `PARTIAL_COMMIT` evidence rather than false atomicity or automatic rollback claims.

R24 completed on 2026-08-13 after focused/adversarial and full local verification, activated-candidate connector acceptance of the 34-tool surface, native Windows/Linux/macOS race/regression and binary/server smoke, deterministic fuzzing, static/vulnerability checks, container smoke, six-target compilation, and the aggregate push-event `Release candidate` gate. The completed contract and verification record remain in [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md).

## Historical verification boundary

Milestone summaries intentionally omit exhaustive command logs and ephemeral CI/session detail. Git history, GitHub Actions, GitHub Releases, [CHANGELOG.md](../CHANGELOG.md), and the dedicated design documents preserve the reproducible evidence appropriate to those surfaces. New planning belongs only in [ROADMAP.md](ROADMAP.md).
