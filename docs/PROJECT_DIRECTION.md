# Scripthold Project Direction

Scripthold is the independently versioned `zoster81/scripthold` fork of the original [`mcp-file-tools`](https://github.com/dimitar-grigorov/mcp-file-tools) project created by Dimitar Grigorov. It retains the GPL-3.0 lineage while maintaining its own module path, MCP identity, release pipeline, API, transport architecture, security model, and documentation.

Scripthold is not a compatibility branch that routinely mirrors upstream source. Ideas and fixes may be evaluated in either direction, but every change is reviewed against the receiving project's current architecture.

## Product identity

**Code from the web. Work locally. Recover safely.**

Scripthold is a secure local-workspace MCP runtime for web, desktop, and CLI agents that need controlled access to source code and text files inside explicitly authorized directories.

The product focuses on:

- safe reading and mutation of mixed/legacy text encodings;
- deterministic, approval-bound filesystem changes;
- optional persistent backup and recovery;
- bounded source navigation and structural code intelligence;
- optional durable local task execution;
- stdio and authenticated Streamable HTTP access to the same tool catalog;
- explicit resource limits and fail-closed path/security behavior.

Binary/media interpretation and per-agent filesystem ACLs are outside the current model. Every connection to one server process shares its startup authorization policy; use separate processes when technical isolation is required.

## Current product state

The current public release is Scripthold **3.2.1** with 38 tools, 3 guided prompts, 168 registered encodings, and 101 active source-intelligence providers.

Current behavior is documented in [`ARCHITECTURE.md`](ARCHITECTURE.md) and [`TOOLS.md`](../TOOLS.md). Current/future work is in [`ROADMAP.md`](ROADMAP.md), and release changes are in [`CHANGELOG.md`](../CHANGELOG.md).

## Supported transports

| Transport | Intended use | Authentication boundary | Directory policy |
|---|---|---|---|
| stdio | Client-managed local processes, desktop/CLI clients, secure bridge/tunnel topologies | Operating-system process and client configuration | Startup directories are authoritative; dynamic client roots are accepted only when no startup directories were configured |
| Streamable HTTP | Persistent local services, containers, trusted reverse proxies, explicitly secured remote deployments | Bearer token on every MCP request; loopback by default; TLS or a trusted proxy boundary for non-loopback listeners | Startup directories are immutable and shared by all requests; HTTP clients cannot mutate roots |

Both transports build the same server and expose the same tools, prompts, encoding behavior, limits, execution policy, and typed errors. HTTP deployment requirements are in [`HTTP_SECURITY.md`](HTTP_SECURITY.md).

## Distribution posture

The Official MCP Registry currently distributes Scripthold through six platform-specific MCPB packages whose package launch transport is `stdio`. That Registry metadata describes the published package launch contract; it does not limit the executable's native Streamable HTTP capability.

Streamable HTTP will be added to Registry package metadata only when a versioned distribution can preserve the complete non-loopback security model, including bearer-token secret handling, explicit workspace mounts, non-root/read-only container hardening, and direct TLS or a trusted proxy boundary. A localhost `remote` entry is not a substitute for a publicly reachable service.

Third-party indexes and trust directories are discovery surfaces rather than product authority. Public badges must be backed by a real workflow result or verified listing. Directory publication failures remain separate from GitHub Release and Official MCP Registry success.

Scripthold remains GPL-3.0. Distribution catalogs whose current local-server policy excludes GPL are not targets for automated submission; distribution policy does not justify relicensing the project.

## Relationship to upstream

The original project remains an independent product. Scripthold may review upstream ideas, bug reports, tests, and security lessons, but does not promise source-level, schema, release, or deployment compatibility with later upstream versions.

Attribution to the original project remains permanent. New Scripthold work should describe the user problem and the resulting local design rather than maintaining a running diary of cross-project implementation history.

## Maintenance policy

- Product and API decisions are made for Scripthold users.
- Public behavior is defined by implementation, tests, the tool catalog, and current source-of-truth documentation.
- Security and compatibility boundaries are changed deliberately, not incidentally.
- Public documentation must remain reproducible from a normal clone and must not contain private workstation/runtime state.
- Historical implementation detail belongs in Git; documentation should describe current behavior, migration actions, release changes, and planned work.
