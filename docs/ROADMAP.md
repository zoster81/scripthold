# Scripthold Development Roadmap

This document is the authoritative source for **current and future milestone state** in `zoster81/scripthold`. Completed engineering history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md), release-by-release changes in [CHANGELOG.md](../CHANGELOG.md), and subsystem contracts in their dedicated design documents.

## Current state

- Current public release: **Scripthold `2.2.0`**.
- Public surface: **30 tools**, **3 guided prompts**, and **168 registered text encodings** over stdio and Streamable HTTP.
- R21 and R22 are complete.
- **No release-scoped milestone is currently active.** A new milestone must be explicitly approved before planned work is described as active.
- Publication does not imply deployment. Operator-specific deployment, rollback, and runtime state are governed separately by [PUBLISHING.md](PUBLISHING.md) and private operational procedures.

## Operating rules

- At most one release-scoped milestone may be `ACTIVE` at a time.
- Define the user-visible outcome, compatibility boundary, security implications, and completion gate before implementation begins.
- Keep changes scoped to the active milestone unless maintainers explicitly reprioritize them.
- Use content bytes and decoded-text evidence for encoding detection; filenames and extensions never select an encoding.
- Preserve stdio behavior while transport work is in progress.
- Keep both `task_run` execution kinds disabled by default on every transport.
- Public releases require an exact clean commit, a dated changelog entry, the complete release-candidate gate, and the immutable publication procedure in [PUBLISHING.md](PUBLISHING.md).
- Every milestone uses the reusable engineering checks in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md).
- Completed implementation detail moves to the appropriate design/history document instead of accumulating indefinitely in this roadmap.

## Milestone overview

| Milestone | Status | Outcome |
|---|---|---|
| R1 | COMPLETE | Shared encoding/BOM-aware text-document core. |
| R2 | COMPLETE | Secure deterministic recursive traversal across Unix and Windows path semantics. |
| R3 | COMPLETE | Durable staged mutation layer with conflict detection and rollback-aware backup handling. |
| R4 | COMPLETE | Transport-independent typed operation errors and stable public error categories. |
| R5 | COMPLETE | Bounded ordered concurrency for batch and search operations. |
| R6 | COMPLETE | Shared execution preparation and authoritative tool metadata. |
| R7 | COMPLETE | Documentation, roadmap, and contributor-workflow reset with private/public state separation. |
| R8 | COMPLETE | Conservative content-based encoding detection independent of filenames. |
| R9 | COMPLETE | Bounded-memory streaming for large text operations and explicit full-document limits. |
| R10 | COMPLETE | 2.0 public API and compatibility cleanup. |
| R11 | COMPLETE | Transport-independent server construction and process-wide root policy. |
| R12 | COMPLETE | Approved Streamable HTTP threat model and fail-closed security contract. |
| R13 | COMPLETE | Native authenticated Streamable HTTP while preserving stdio. |
| R14 | COMPLETE | Hardening, publication, deployment verification, rollback, and restoration for `2.0.0`. |
| R15 | COMPLETE | Agent-oriented read/search/edit workflows and three guided encoding prompts. |
| R16 | COMPLETE | Deterministic fingerprints, one-shot edit approval, patch packages, and structured verification. |
| R17 | COMPLETE | Approved persistent-backup lifecycle and security design. |
| R18 | COMPLETE | Persistent backup capture, review, restore, audit, and explicit garbage collection. |
| R19 | COMPLETE | Mutation-free offline diagnostics for an existing backup store. |
| R20 | COMPLETE | MCP `2026-07-28` adoption with retained legacy behavior and shared security boundaries. |
| R21 | COMPLETE | Durable asynchronous task execution and exact-commit release gating in the `2.1.x` line. |
| R22 | COMPLETE | Global portable encoding coverage, full UTF-32 support, detector hardening, and published `2.2.0`. |

Detailed outcomes for R1–R22 are recorded in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md).

## Latest completed milestone — R22

R22 delivered the release-scoped encoding expansion documented in [GLOBAL_ENCODING_COVERAGE.md](GLOBAL_ENCODING_COVERAGE.md):

- 168 canonical read/write encodings under one authoritative capability registry;
- full UTF-32 LE/BE support across the text-operation pipeline;
- the complete applicable repository-pinned `golang.org/x/text` surface plus verified pure-Go mappings and stateful/multibyte codecs derived from pinned GNU libiconv evidence;
- conservative automatic detection with explicit ambiguity when evidence is insufficient;
- bounded visible partial-failure reporting for grep and batch operations;
- registry-driven public-operation coverage, adversarial/resource verification, deterministic release packaging, and the complete cross-platform release-candidate gate.

Scripthold `2.2.0` was published on 2026-08-11 through the exact-commit workflow in [PUBLISHING.md](PUBLISHING.md). Actual MCPB bundles, their checksum manifest, the final MCPB-backed Registry manifest, and Registry publication were produced only by GitHub after tagging.

## Starting the next milestone

Before adding a new `ACTIVE` milestone:

1. state the concrete user or maintainer problem;
2. define what is in scope and explicitly out of scope;
3. identify public API, compatibility, security, filesystem, encoding, concurrency, and platform consequences;
4. create a dedicated design document when the work introduces a new subsystem or trust boundary;
5. define focused tests and the completion gate;
6. add the milestone to the overview and mark exactly one milestone `ACTIVE`.

Until those decisions are made, ordinary maintenance and bug fixes may proceed under the existing contracts without inventing a placeholder roadmap milestone.
