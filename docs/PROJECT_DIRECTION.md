# Scripthold Project Direction and Upstream Relationship

**Scripthold** is the product identity of `zoster81/scripthold`, an independently versioned downstream fork of the [original `mcp-file-tools` project](https://github.com/dimitar-grigorov/mcp-file-tools), created by **Dimitar Grigorov**. It preserves the original project's GPL-3.0 lineage and encoding-aware text-file purpose while maintaining its own module path, MCP Registry identity, release pipeline, public API, transport architecture, security model, and deployment documentation.

Scripthold is not a compatibility branch intended for routine source synchronization with upstream. Ideas, tests, fixes, and implementation techniques may inform either project, but every change is reviewed against the receiving project's current architecture and product boundaries.

## Product identity

**Code from the web. Work locally. Recover safely.**

Scripthold is a secure local-workspace MCP runtime for web, desktop, and CLI agents that need controlled access to real source code and text files inside explicitly authorized directories.

**Scripthold was built with Scripthold.** Its development uses the same web-to-local workflow offered to users.

## Product scope

The maintained product includes:

- one authoritative tool catalog over stdio and Streamable HTTP: 36 tools in the current `3.1.0` release, plus 3 guided prompts;
- process-wide allowed-directory policy with symlink, junction, reparse-point, missing-ancestor, and Windows path-alias validation;
- 168 registered text encodings with content-based detection intentionally narrower than explicit codec support;
- bounded native read-only source intelligence through `source_symbols` and `source_query`, with 101 active approved R27 providers, evidence-qualified detection, capability-specific project relations, bounded task context, coherent process-local indexing, decoded-source coordinates, and no external parser/compiler/LSP runtime dependency;
- bounded-memory text reading, grep, conversion, BOM, and line-ending operations, with explicit limits for full-document edits;
- durable staged mutations with practical concurrent-change detection, no-replace creation, platform synchronization, and operation-specific rollback/recovery evidence;
- deterministic fingerprints, one-shot edit/package approval, strict patch packages, and typed structured verification;
- an optional dedicated persistent backup store with immutable content-addressed objects, bounded review/audit, approval-bound capture, original-target restore, and explicit garbage collection;
- mutation-free offline diagnosis of an existing backup store;
- optional durable `task_run` shell/script execution with idempotent admission, persistent bounded queueing, independent supervisor/worker/executor lifecycle, logical locks, bounded logs, recovery, retention, and cancellation;
- MCP `2026-07-28` support through the stable Go SDK, with modern stateless HTTP beside retained stateful legacy HTTP behavior under the same outer security pipeline;
- reproducible multi-platform releases and a non-root transport-neutral container.

Binary/media interpretation and per-agent filesystem ACLs are outside the current product model. Every connection to one server process shares the same startup roots and policy. Run separate processes when technical isolation is required.

## Current product state

Scripthold `3.1.0` is the current public release with 36 tools, 3 guided prompts, 168 registered text encodings, and 101 active approved source-intelligence providers across 103 registry rows. It preserves the R23-R27 public surface shipped in `3.0.0` and adds the completed R28 engine-hygiene work without changing the MCP tool surface. R27 completed on 2026-08-16 with capability-specific project relations, bounded structural search/context, and coherent process-local incremental indexing, while Dockerfile and Make remain auxiliary inactive metadata.

The current development surface remains native Go and fail-closed: source intelligence does not execute project code or depend on external parser/compiler/LSP runtimes, ambiguous language routing is reported instead of guessed, and persistent on-disk source indexing is not enabled. No milestone after R28 is active by default.

Current milestone state belongs in [ROADMAP.md](ROADMAP.md); completed history in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md); release changes in [CHANGELOG.md](../CHANGELOG.md); and subsystem contracts in [MCP_MUTATION_SURFACE.md](MCP_MUTATION_SURFACE.md), [SAFE_FILESYSTEM_OPERATIONS.md](SAFE_FILESYSTEM_OPERATIONS.md), [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md), [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md), [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md), and [ENGINE_HYGIENE.md](ENGINE_HYGIENE.md). Publication and deployment remain separate operations; public product documentation does not track private workstation runtime state.

## Supported transports

| Transport | Intended use | Authentication boundary | Directory policy |
|---|---|---|---|
| stdio | Client-managed local processes, desktop/CLI MCP clients, and secure tunnel bridges | Operating-system process and client configuration | Startup directories are authoritative; dynamic client roots are accepted only when no directories were configured at startup |
| Streamable HTTP | Persistent local services, containers, trusted reverse proxies, and explicitly secured remote deployments | Bearer token on every MCP request; loopback by default; TLS or a trusted proxy boundary for non-loopback listeners | Startup directories are immutable and shared by all HTTP requests; HTTP clients cannot mutate roots |

Both transports construct the same server through `BuildServer` and expose the same tools, prompts, encoding behavior, limits, execution policy, and typed errors. The HTTP threat model and deployment rules are defined in [HTTP_SECURITY.md](HTTP_SECURITY.md).

## Relationship to upstream

The [original `mcp-file-tools` project](https://github.com/dimitar-grigorov/mcp-file-tools) remains an independent product and the source of Scripthold's original encoding-aware file-tool lineage. Scripthold reviews upstream developments selectively for useful ideas, bug reports, tests, and security lessons; it does not promise source-level, schema, release, or deployment compatibility with later upstream versions.

R15 explicitly credited upstream concepts and implementation approaches that informed optional line numbers, richer grep, `.gitignore` traversal, bounded sorting, batch encoding dry runs, encoding workflow prompts, unified-patch editing, and opt-in fuzzy matching. The resulting Scripthold implementations were adapted to this fork's secure walker, bounded-memory pipeline, durable mutation layer, process-wide roots, stable schemas, and dual-transport security model rather than copied as a synchronization strategy.

## Maintenance policy

- Release and API decisions are made for Scripthold users, not to minimize merge conflicts with upstream.
- Public behavior is defined by this repository's implementation, tool catalog, tests, and source-of-truth documentation.
- Upstream ideas are reviewed selectively and adapted only when they fit Scripthold's security, compatibility, resource, and maintenance boundaries.
- Cross-project proposals should describe the user problem and credit the project where an idea was observed without assuming either repository can accept the other's implementation unchanged.
- Public documentation must keep reproducible product behavior separate from private operator state, local runtime state, credentials, and workstation-specific orchestration.
