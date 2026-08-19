# Logging and Diagnostics Lifecycle

## Status

**COMPLETE in source for R29.** This document is the authoritative contract for process-wide Scripthold diagnostics, HTTP/security access logging, and optional bounded diagnostic-file retention. R29 changes no public MCP tool or prompt schema and is not itself a release or deployment action.

Durable task stdout/stderr remains governed by [DURABLE_TASKS.md](DURABLE_TASKS.md) and the `task_logs` tool. HTTP admission/security logging remains additionally governed by [HTTP_SECURITY.md](HTTP_SECURITY.md).

## Goals and boundaries

R29 centralizes process-wide server diagnostics while preserving three distinct data classes:

1. **Server diagnostics** — internal lifecycle, tool-category, task-control, filesystem, source-intelligence, and other process diagnostics emitted through the `server` channel.
2. **HTTP/security access logs** — bounded request/admission evidence emitted through the `http_access` channel using the HTTP threat model's fixed-field policy.
3. **Durable task stdout/stderr** — task-owned execution output retained only by the task store and read through `task_logs`; it is never copied into the R29 diagnostic sink.

The MCP SDK logger is also distinct. The Scripthold command runtime does not route SDK logging into the R29 server or HTTP channels. Embedders may still supply the historical SDK `Logger`; `ToolLogger` is a separate server-construction option for category-only tool lifecycle diagnostics.

R29 does not add dynamic logging control, remote log upload, a logging MCP tool, task-output mirroring, persistent tracing, or a background maintenance service.

## Startup authority

Logging policy is sampled at process startup. Clients cannot change it through MCP requests, prompts, roots, HTTP headers, or task requests.

| Variable | Meaning | Default | Hard rule |
|---|---|---:|---|
| `MCP_LOG_LEVEL` | Diagnostic level: `debug`, `info`, `warn`/`warning`, or `error`. Unknown historical values retain the compatibility fallback to `info`. | `info` | Startup only |
| `MCP_LOG_DIR` | Existing absolute directory for optional diagnostic files. Unset means stderr only. | unset | Must already exist and must not be a symlink/reparse-point directory |
| `MCP_LOG_MAX_FILE_BYTES` | Maximum bytes in one active/plain diagnostic file before rotation. | `8388608` | `1..268435456` |
| `MCP_LOG_MAX_TOTAL_BYTES` | Aggregate diagnostic storage ceiling used to derive writer/archive budgets. | `134217728` | `1..8589934592` and at least four times the per-file limit |
| `MCP_LOG_RETENTION_DAYS` | Maximum archive age before cleanup. | `7` | `1..3650` |

`--version` remains a metadata-only command and does not initialize diagnostic file logging.

An invalid numeric policy or a structurally invalid configured log directory (missing, relative, not a directory, or a symlink/reparse-point directory) fails command startup with a generic path-free error. After that structural validation, operational maintenance or ownership failures may instead disable the file sink and continue on stderr as described below. R29 does not create the configured directory or broaden its permissions; directory placement and operating-system access remain operator authority.

## stderr and file sinks

stderr remains the always-available diagnostic destination. When `MCP_LOG_DIR` is unset, no R29 diagnostic files are created.

When file logging is configured successfully, the same redacted records are written to stderr and to an owned active file. File logging is bounded independently of product data:

- each process acquires one exclusive writer slot from a deterministic bounded slot set;
- each slot has one active file at a time;
- rotation occurs before a record would exceed the configured per-file bound;
- rotated files are published with no-replace rename semantics and compressed with gzip;
- retention removes expired archives and then removes oldest archives as needed to preserve the aggregate archive budget;
- the aggregate budget reserves enough headroom for all active writers and an in-progress bounded rotation rather than treating compressed size as guaranteed savings;
- no background goroutine performs periodic cleanup: maintenance occurs during startup, rotation, degradation recovery, and clean close.

Writer-slot and maintenance lock files are internal ownership evidence, not log records.

## Multi-process ownership and crash recovery

Multiple Scripthold processes may share one configured diagnostic directory without sharing an active file.

- A per-slot operating-system lock grants exclusive ownership of that active filename.
- A short global maintenance lock serializes archive rename/compression/cleanup work.
- Startup recovery may finalize a stale active file only after acquiring that slot's ownership lock, proving that no live writer owns it.
- A live writer never releases its slot merely because file logging degrades. It retains ownership until close, preventing another process from reusing a slot while a preserved active file still belongs to the degraded writer.
- Archive publication uses the repository's native no-replace filesystem primitive; an existing archive name is never overwritten.

A crashed process can therefore leave one bounded active file. A later process that proves ownership may rotate/compress it before reusing the slot.

## Failure semantics

Diagnostic persistence is subordinate to normal product correctness. A logging failure must not turn a successful file/tool/task operation into a product failure.

- Invalid startup policy or a structurally invalid explicitly configured directory fails startup before the server starts.
- If startup housekeeping or writer-slot ownership cannot safely enforce the file-storage policy, Scripthold continues on stderr and leaves existing diagnostic evidence intact.
- If an owned active file cannot be opened safely after ownership has been established, startup fails with a generic path-free error rather than weakening file-sink guarantees.
- If all writer slots are already owned, that process continues on stderr without stealing another process's file.
- A steady-state write, sync, maintenance-lock, rename, or cleanup failure disables that process's file sink and keeps stderr active.
- A rename failure preserves the already-written active file rather than truncating or overwriting it.
- A compression failure preserves the bounded uncompressed archive and still runs retention/aggregate cleanup.
- Temporary gzip publication uses create-exclusive/no-replace semantics; incomplete temporary output is removed when possible and never replaces an existing archive.
- Clean close makes a bounded best-effort attempt to finalize a degraded writer before releasing its ownership slot. If finalization still fails, the active file remains recoverable evidence for a future owner.

Warnings generated by the diagnostic lifecycle are fixed strings. They do not include configured directories, archive names, OS error strings, command text, source text, or credentials.

## Redaction contract

R29 applies redaction before either stderr or the optional file sink receives a record.

- Production diagnostic event messages are static string literals; a repository AST regression rejects dynamic `Debug`/`Info`/`Warn`/`Error` messages in the production logging surface.
- Path-like string attributes are replaced with short SHA-256 fingerprints for correlation rather than clear filesystem paths.
- Attributes whose keys indicate secrets, authorization material, command/body/content/diff/preview data, panic/stack data, human error messages, sessions, tokens, passwords, or generic values are replaced with `[redacted]`.
- Only an explicit category-oriented string allowlist is retained in clear text, such as channel, process role, tool name, stable error code, HTTP method/fixed route, transport, phase/state/kind, and encoding category.
- Unexpected string and object values are redacted by default; bounded numeric, boolean, duration, and time evidence may remain.
- Retained strings and event messages have fixed byte bounds.

Tool middleware therefore logs lifecycle category, tool name, and stable error code but not human-readable MCP failure text, raw Go errors, recovered panic values, or stacks.

The R29 logger is diagnostic evidence, not a secret store. Operators should still select an appropriately controlled logging directory and apply their normal host-level log-access policy.

## HTTP relationship

HTTP access logging continues to satisfy the stricter field policy in [HTTP_SECURITY.md](HTTP_SECURITY.md). R29 supplies that pipeline with the dedicated `http_access` channel; it does not merge access records with MCP SDK diagnostics.

Tool lifecycle logging may now remain enabled for HTTP because the runtime uses the category-only redacted `ToolLogger`. The SDK logger remains separate and is not connected to the command runtime's R29 channels.

## Durable task relationship

R29 does not change task output persistence. Executor stdout/stderr remains in the owner-only task store under its existing per-task, cursor, rotation, retention, and recovery rules. `task_logs` is the only public read surface for that output.

Task worker/supervisor/executor process diagnostics may use the R29 `server` channel with a fixed process `role`, but task command text and task stdout/stderr are not diagnostic attributes.

## Verification contract

R29 completion requires evidence for:

- startup defaults, invalid configuration, and path-free startup failures;
- redaction of paths, secrets, panic/error text, oversized strings, and unexpected structured values;
- static production log-message discipline;
- stderr-only behavior and explicit server/HTTP channel separation;
- SDK logger versus tool logger separation;
- per-file rotation, gzip compression, age retention, and aggregate-size bounding;
- concurrent writer-slot ownership and repeated concurrency regression;
- stale-active recovery and clean close;
- injected rename, compression, and cleanup failures without product-state corruption;
- Windows native behavior plus Linux/macOS compilation of the diagnostics and command integration;
- normal, race, vet, lint, Staticcheck, vulnerability, documentation-link, repository-identity, and final diff/status gates applicable to the milestone.

Release publication, deployment, and runtime activation remain separate maintainer actions.
