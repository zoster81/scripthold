# Changelog

This changelog records user-visible Scripthold changes. Detailed implementation history remains available in Git.

## Unreleased

### Added

- Added bounded redacted server diagnostics with optional file rotation/retention, while keeping HTTP access logs and durable-task output separate.
- Added protected persistent backups through `backupPolicy: "pinned"` and explicit exact-ID `backup_delete` removal.

### Changed

- Raised the Go baseline to 1.27.1 and refreshed the repository validation/release toolchain, including Node.js 26.8.2, golangci-lint 2.13.2, govulncheck 1.8.0, GoReleaser 2.18.1, Gitleaks 8.30.1, and CodeQL Action 4.38.0.
- Updated the stable MCP Go SDK to v1.8.0 and refreshed its resolved support dependencies.
- Improved connector reliability with a bounded synchronous call deadline, retained retrieval for oversized completed responses, and optional durable ownership for eligible long-running read-only calls. Modern MCP clients can observe the same durable work through native task APIs; legacy clients continue to use `deferred_operation`.
- Reduced Source Intelligence memory duplication without changing its public schemas or capability claims.
- Changed per-target backup history from a hard saturation barrier to synchronous oldest-eligible retention after a newer backup is durable; the default retained version target is now 64.

### Fixed

- Hardened Source Intelligence across multiple real-world language/provider cases, including Haskell, Julia, PHP/PHP-HTML, Blade, JavaScript/TypeScript, Lua/Luau, JSP, Erlang, F#, BASIC-family providers, Gleam, Jinja/Twig, Terraform/HCL, and malformed Zig declarations.
- Fixed deferred-operation state-observation races and made cancellation interrupt durable store-lock waits correctly.
- Fixed saturated backup targets becoming permanently unable to accept a newer backup.
- Fixed `backup_delete` output-budget handling so destructive authority is rejected before execution when a valid response cannot fit.
- Accepted `none` as a case-insensitive compatibility alias for BOM policy `never`; returned metadata remains canonical `never`.

## 3.1.6 - 2026-08-19

- Hardened integer-size handling reported by CodeQL, including native-width configuration parsing and bounded backup-list cursor assembly.

## 3.1.5 - 2026-08-19

- Redesigned repository verification around fail-closed evidence tiers, one shared risk-based fuzz manifest, exact-SHA release authority, and complementary local validation profiles.
- Fixed Unicode-sensitive byte-offset handling in Classic ASP, ASP.NET, and Razor composite scanning.

## 3.1.0 - 2026-08-18

- Completed engine-hygiene work without changing the public MCP surface: removed obsolete internal/test-only paths, consolidated compatible implementations, renamed responsibility-oriented source files, and retained non-equivalent compatibility behavior.

## 3.0.0 - 2026-08-17

### Added

- Added broad read-only Source Intelligence with 101 active providers, `source_symbols`, structural `source_query`, bounded project relations/context, fail-closed language ambiguity, and process-local incremental generations.
- Added offline evidence-preserving backup recovery into a separate destination.
- Added `filesystem_package` / `filesystem_package_apply` for bounded coordinated create/copy/move/delete operations.
- Added dedicated mutation apply tools so edit, patch, restore/GC, encoding, and BOM preparation stays read-only until a one-shot `previewId` is applied.
- Added `golangci-lint` and CodeQL gates to the release-quality pipeline.

### Changed

- Replaced the simple public create/copy/move/delete tools with the safer filesystem-package model.
- Split mixed read/write MCP surfaces into truthful preparation and mutation tools; apply requests accept only the prepared capability identifier.
- Strengthened persistent-backup integration for approval-bound mutations and destructive filesystem operations.

See [`docs/MIGRATION_3.0.md`](docs/MIGRATION_3.0.md) for caller migration details.

## 2.2.0 - 2026-08-11

- Expanded the encoding registry to 168 canonical read/write encodings.
- Added full UTF-32 LE/BE text-pipeline support and strict GB18030:2022 handling.
- Hardened automatic encoding detection to fail closed on ambiguous, malformed, binary-like, or weakly evidenced input.
- Made grep/batch partial coverage and encoding failures explicit and bounded.

## 2.1.1 - 2026-08-10

- Consolidated release validation around one exact-commit pre-tag gate.
- Renamed whole-document replacement from `write_file` to `write_whole_file` to make destructive replacement semantics explicit.
- Fixed a missing-path resolution race and durable-task allowed-root persistence behavior.
- Corrected MCP Registry publication to consume verified platform MCPB release assets.

## 2.1.0 - 2026-08-09

### Added

- Added durable asynchronous task execution with persistent queueing, idempotency, logical locks, bounded logs, cancellation, recovery, and independent supervisor/worker/executor ownership.
- Added deterministic path fingerprints, one-shot edit/patch approvals, structured verification, richer grep/search, batch conversion, and encoding workflow prompts.
- Added the optional persistent backup store with bounded review, capture, restore, audit, and explicit GC.
- Added MCP `2026-07-28` support while retaining compatible legacy stdio/HTTP behavior.

### Changed

- Replaced synchronous public `run_script` and `shell` with the durable task API.
- Expanded backup and mutation workflows with stronger conflict, capability, and partial-state evidence.

## 2.0.0 - 2026-08-02

### Added

- Added native authenticated Streamable HTTP beside stdio, including loopback defaults, bearer authentication, Host/Origin checks, bounded admission, TLS/proxy controls, health/readiness, and graceful shutdown.
- Added stable typed operation errors, configurable resource limits, a shared authoritative tool catalog, and multi-platform release verification.
- Added bounded streaming text/encoding operations and shared durable mutation primitives.
- Added conservative extension-independent BOMless UTF-16 detection and broad malformed/binary false-positive protection.

### Changed

- New files default to UTF-8; existing files continue to preserve confidently detected encodings unless explicitly converted.
- Dynamic MCP roots are limited to roots-capable stdio clients started without configured directories; HTTP roots remain startup policy.
- Release/container workflows were hardened around reproducible builds and non-root execution.

See [`docs/MIGRATION_2.0.md`](docs/MIGRATION_2.0.md) for the intentional 1.8-to-2.0 compatibility changes.

## 1.8.0 - 2026-07-25

- Established the fork-owned release/update pipeline and repository identity.
- Added shared encoding/BOM-aware file operations, typed errors, deterministic traversal, durable file mutation primitives, and the first optional execution tools.
- Added Windows reparse/junction path hardening and practical concurrent-change protection for file mutations.

## Earlier fork history

Earlier development details are preserved in Git history. The changelog intentionally does not duplicate commit-by-commit or phase-by-phase engineering records.
