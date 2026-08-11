# R1-R6 Engineering History

This document summarizes the early public engineering outcomes that precede the current milestone record in [ROADMAP.md](ROADMAP.md). It intentionally excludes workstation paths, private connector state, process identifiers, local binaries, and operator handoff details.

For current work, use [ROADMAP.md](ROADMAP.md), [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md), and the repository's scoped [`AGENTS.md`](../AGENTS.md) files.

## R1 — Shared text-document core

### Objective

Unify encoding, BOM, line-ending, decoding, encoding, and file-snapshot behavior that had been duplicated across handlers.

### Outcomes

- Added a shared encoding/BOM-aware document pipeline used by read, write, edit, conversion, line-ending, and BOM operations.
- Made explicit encoding override auto-detection while retaining detected-encoding metadata.
- Added public BOM policies: `auto`, `always`, `never`, and `preserve`.
- Preserved UTF-8 and UTF-16 BOM behavior consistently and rejected BOM/encoding conflicts.
- Centralized line-ending restoration and byte-identical no-op handling.
- Added multilingual UTF-8, UTF-16, legacy-code-page, BOM, and LF/CRLF regression coverage.

### Remaining boundary

The shared document path still loads complete files into memory. Bounded-memory streaming is tracked in R9.

## R2 — Secure deterministic traversal

### Objective

Apply one path-resolution and traversal policy to recursive tools on Unix and Windows.

### Outcomes

- Added a shared deterministic lexical walker for tree, search, and grep operations.
- Resolved every visited entry before exposing it to callers.
- Prevented allowed-root escapes through symlinks, Windows junctions, and other reparse points.
- Validated missing paths through their nearest existing ancestor.
- Added cancellation, depth, exclusion, error-policy, and early-stop behavior.
- Added Unix symlink and Windows drive-root, junction, reparse-point, UNC, and short-path regression tests.

### Remaining boundary

Path validation reduces but cannot eliminate every path-based TOCTOU window without handle-relative operations.

## R3 — Durable mutation layer

### Objective

Replace handler-specific writes with shared durable and conflict-aware filesystem primitives.

### Outcomes

- Added same-directory staging, file sync, atomic replacement, and platform-specific namespace commits.
- Added optimistic snapshots for practical concurrent-modification detection.
- Prevented initially missing destinations from overwriting paths created concurrently.
- Added transactional backup replacement with rollback and explicit recovery artifacts on rollback failure.
- Migrated write, edit, encoding conversion, line-ending conversion, BOM changes, copy, move, and delete operations.
- Added failure-injection coverage for staging, sync, commit, cleanup, backup, rollback, permission, and conflict failures.

### Remaining boundary

Directory durability differs by platform; Unix directory sync and Windows write-through behavior remain explicit rather than presented as identical guarantees.

## R4 — Typed operation errors

### Objective

Separate domain failures from MCP response formatting and stabilize programmatic error categories.

### Outcomes

- Added transport-independent error kinds for invalid input, access control, missing paths, encoding, permissions, conflicts, cancellation, limits, and filesystem failures.
- Centralized MCP and batch error mapping.
- Preserved compatibility messages while adding stable per-file batch error codes.
- Added tests for wrapped and joined causes, security categories, encoding failures, mutation conflicts, cancellation, and filesystem errors.

## R5 — Bounded ordered concurrency

### Objective

Share deterministic concurrency mechanics across batch operations without unbounded result accumulation.

### Outcomes

- Added a generic ordered worker coordinator with bounded in-flight work.
- Preserved input and traversal order regardless of worker completion order.
- Added explicit cancellation modes, partial-result preservation, and commit-driven early stop.
- Migrated `read_multiple_files` and `grep_text_files` to the shared coordinator.
- Enforced `maxMatches` during scanning rather than after unbounded collection.
- Added saturation, reverse-completion, cancellation, early-stop, and race-oriented tests.

### Remaining boundary

Worker count is bounded, but complete decoded documents can still dominate aggregate memory until R9.

## R6 — Execution preparation and authoritative tool metadata

### Objective

Share safe process preparation without merging authorization policies, and remove metadata duplication across runtime and release tooling.

### Outcomes

- Consolidated common process validation, bounded output, timeout, cancellation, and environment preparation.
- Kept `run_script` and `shell` as separate capabilities with independent feature flags and authorization boundaries.
- Added final script and working-directory revalidation plus file metadata and digest checks before process start.
- Added an embedded authoritative tool catalog for names, titles, descriptions, and annotations.
- Made runtime registration and Registry manifest generation consume the same catalog.
- Added drift tests requiring README links and `TOOLS.md` sections for every catalog tool.
- Pinned validation tool versions used by CI and release workflows.

### Remaining boundary

At the R6 boundary, `run_script` still used a path-based process creation API, so the final validation-to-execution transition could not be fully atomic; `shell` remained intentionally unrestricted after working-directory validation and disabled by default. Later milestones replaced these synchronous public execution tools with the durable `task_run` family.

## Cross-cutting verification established by R1-R6

The completed foundations introduced or expanded coverage for:

- focused and full Go tests;
- Go module verification, vet, Staticcheck, and govulncheck;
- race-detector execution where a CGO compiler is available;
- Windows, Linux, and macOS compilation paths;
- manual server-operation tests;
- release-script and metadata drift tests;
- JSON, YAML, PowerShell, workflow, and Markdown validation;
- secret scanning and release configuration checks when available;
- deterministic traversal, durable mutation, typed error, and bounded-concurrency regressions.

Exact historical commits and release artifacts remain available through Git history and GitHub Releases. Current milestone status and future completion gates are defined only in [ROADMAP.md](ROADMAP.md).
