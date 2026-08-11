# Scripthold Project Direction and Upstream Relationship

**Scripthold** is the product identity of `zoster81/scripthold`, an independently versioned downstream fork of [`dimitar-grigorov/mcp-file-tools`](https://github.com/dimitar-grigorov/mcp-file-tools), created by **Dimitar Grigorov**. It preserves the original project's encoding-aware text-file purpose and GPL-3.0 lineage, while maintaining its own module path, MCP Registry identity, release pipeline, public API decisions, transport architecture, and deployment documentation.

This repository is not a compatibility branch intended for routine merging with upstream. Both projects may develop useful ideas independently, but changes must be reviewed and implemented against each project's current architecture rather than copied or synchronized mechanically.

## Product identity

**Code from the web. Work locally. Recover safely.**

Scripthold is a secure local workspace runtime that lets web-based and local AI agents work with real source code and text files inside explicitly authorized directories.

**Scripthold was built with Scripthold.** Its own development has used the same web-to-local workflow offered to users.

## Product scope

Scripthold is a secure, encoding-aware MCP filesystem service for local, tunneled, containerized, and explicitly secured network deployments. Its supported scope includes:

- the same authoritative 30-tool current source catalog over stdio and Streamable HTTP;
- process-wide allowed-directory policy with symlink, junction, reparse-point, and missing-ancestor validation;
- bounded-memory decoding, reading, grep, conversion, line-ending, and BOM operations across 168 registered encodings in the current R22 source tree, with automatic detection intentionally narrower than explicit codec support;
- durable staged mutations with practical concurrent-change detection, no-replace creation, backup rollback, and platform-specific synchronization;
- an optional dedicated persistent backup store with immutable content-addressed objects, checksummed manifests, bounded management/audit, approval-bound pre-state capture for prepared edits and patch packages, one-shot original-target restore, explicit generation-bound garbage collection, and a separate mutation-free offline diagnostic command for existing stores;
- transport-independent error categories and tool metadata;
- MCP `2026-07-28` support through official stable Go SDK `v1.7.0`, with modern stdio gating and stateless HTTP while preserving roots-dependent legacy stdio and stateful HTTP behind the same shared security boundary;
- optional durable `task_run` shell/script execution with idempotent admission, a persistent bounded queue, supervisor/worker/helper isolation, parallelism, logical locks, cursor logs, cancellation, recovery, and retention; both kinds are disabled by default and HTTP retains its additional execution gate;
- reproducible multi-platform releases, checksum-driven MCP Registry publication, and a non-root transport-neutral container.

Binary or media interpretation remains outside the server's scope. Per-agent filesystem ACLs are also outside the current model: every connection to one process shares its startup roots and policy. Deploy separate processes when technical isolation is required.

## Current development direction

R22 completed with Scripthold `2.2.0` on 2026-08-11. The release delivers the authoritative 168-codec capability registry, full UTF-32 LE/BE text support, the complete applicable repository-pinned `golang.org/x/text v0.40.0` surface, 88 deterministic pure-Go single-byte additions, 21 multibyte/stateful or residual exact additions derived and verified from pinned GNU libiconv, conservative detector trust gating, bounded partial-coverage reporting, the registry-driven public-operation matrix, and adversarial/resource verification. The final exact release commit passed all six cross-platform builds, complete race/static/vulnerability/fuzz gates, native Windows/macOS/Linux smokes, hardened Linux container stdio/direct-TLS HTTP checks, and deterministic GoReleaser verification before the annotated release tag. GitHub then published the normal release assets, generated the MCPB artifacts exclusively in the release workflow, and published `io.github.zoster81/scripthold` version `2.2.0` to the MCP Registry. No later release-scoped milestone is currently active. The deployed Windows amd64 runtime remains `2.1.1` until a separately authorized deployment and rollback cycle. The production runtime remains independent of libiconv/uchardet native libraries; those projects remain pinned sources for charset definitions, fixtures, detector nomenclature, and differential oracles. See [GLOBAL_ENCODING_COVERAGE.md](GLOBAL_ENCODING_COVERAGE.md) and [ROADMAP.md](ROADMAP.md).

The final 2.2.0 encoding count is intentionally not fixed during planning. Aliases do not count as separate encodings, explicit codec support may be broader than safe automatic detection, and ambiguous content must continue to fail explicitly rather than using a permissive fallback. UTF-64 is not introduced as a proprietary format; R22 covers standardized Unicode transformation formats through UTF-32 and excludes machine-dependent internal character representations.

## Supported transports

| Transport | Intended use | Authentication boundary | Directory policy |
|---|---|---|---|
| stdio | Client-managed local processes, desktop/CLI MCP clients, and secure tunnel bridges | Operating-system process and client configuration | Startup directories are authoritative; dynamic client roots are accepted only when no directories were configured at startup |
| Streamable HTTP | Persistent local services, containers, trusted reverse proxies, and explicitly secured remote deployments | Bearer token on every MCP request; loopback by default; TLS or a trusted proxy boundary for non-loopback listeners | Startup directories are immutable and shared by all HTTP requests; legacy sessions remain stateful while MCP `2026-07-28` requests are stateless |

Both transports construct the server through the same `BuildServer` path and expose the same tools, limits, execution policy, encoding behavior, and typed errors. The HTTP-specific threat model and deployment rules are defined in [HTTP_SECURITY.md](HTTP_SECURITY.md).

## Relationship to upstream

The original project created by Dimitar Grigorov remains the source of the encoding-aware file-tool implementation and continues to evolve as its own product. This fork retains attribution and tracks upstream developments for ideas, bug reports, and security lessons, but it does not promise source-level or schema compatibility with later upstream releases.

Potential upstream suggestions should be concept-level and narrowly scoped to the original project's product boundaries. Features that depend on this fork's native HTTP service, execution tools, process-wide multi-transport policy, fork-owned Registry identity, or deployment infrastructure should remain fork-specific unless upstream independently chooses those directions.

Conversely, upstream agent-experience improvements may be considered here only after they are adapted to this fork's bounded-memory pipeline, durable mutation layer, stable public schemas, transport equivalence requirements, and security model.

## Reciprocal feature exchange

R15 explicitly credits the original project as the source for optional read line numbers, richer grep modes and paging, `.gitignore`-aware traversal, bounded result sorting, batch encoding dry runs, encoding workflow prompts, unified-patch editing, and opt-in fuzzy matching. Both the user-facing concepts and the original implementation approaches informed this fork's evaluation of behavior, edge cases, and trade-offs. The resulting code was reworked specifically for the fork's secure walker, bounded-memory streaming, durable mutation layer, stable 23-tool catalog, process-wide roots, and stdio/Streamable HTTP equivalence rather than mechanically synchronized.

This attribution is intended as reciprocal engineering exchange rather than one-way ownership. Improvements developed in either project may inspire the other, and useful functionality, implementation techniques, tests, and security findings may flow in either direction through concept-level discussion or normal GPL-3.0-compatible contributions. Neither repository is expected to accept the other's code unchanged, and shared work does not erase their separate APIs, release histories, security models, or maintenance decisions.

## Maintenance policy

- Release and API decisions are made for this fork's users rather than to minimize upstream merge conflicts.
- Upstream changes are reviewed selectively; no automatic merge or rebasing policy exists.
- Public documentation must distinguish shared lineage from current fork behavior.
- Cross-project proposals should describe the user problem and desired behavior, credit the project where the idea was observed, and not assume that either repository can accept the other's implementation unchanged.
