# MCP 2026-07-28 Adoption Design

## Status

**R20 is complete in source. Stdio and same-endpoint Streamable HTTP support `2026-07-28` without depending on deprecated client roots or protocol sessions, supported legacy HTTP remains stateful, and legacy stdio roots still fall back deterministically to `2025-11-25`. Compatibility, conformance, fuzz, native, container, race, static-analysis, vulnerability, and six-target gates are complete. The R20 source remains unreleased and is not present in the deployed `2.0.0` binary.**

This document defines the compatibility boundary, transport architecture, security invariants, implementation phases, and verification gate for adopting Model Context Protocol version `2026-07-28` while retaining the existing `2025-11-25` behavior.

The R20 source baseline now uses official stable `github.com/modelcontextprotocol/go-sdk v1.7.0`, which supports final protocol version `2026-07-28`. Phase 2 qualified the release, Phase 3 implements stdio negotiation, and Phase 4 implements same-endpoint stateful/stateless HTTP routing. Publication and runtime adoption remain separate later decisions.

Authoritative external references:

- [MCP 2026-07-28 specification announcement](https://blog.modelcontextprotocol.io/posts/2026-07-28/)
- [MCP Go SDK releases](https://github.com/modelcontextprotocol/go-sdk/releases)
- [MCP Go SDK protocol guide](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md)
- [SEP-2577 roots, sampling, and logging deprecation](https://modelcontextprotocol.io/seps/2577-deprecate-roots-sampling-and-logging)

## Current baseline

The implementation intentionally provides one shared server through two transports and two HTTP protocol generations:

- stdio through one SDK transport and the shared `filetoolsserver.BuildServer` server;
- authenticated Streamable HTTP through one hardened outer handler that routes to either a legacy stateful SDK handler or a `2026-07-28` stateless SDK handler.

The HTTP generations preserve distinct protocol state while sharing process policy:

- legacy initialization creates an `Mcp-Session-Id`, reserves application session capacity, and retains authenticated `POST`, `GET`, `DELETE`, idle timeout, and SSE semantics;
- exact `2026-07-28` requests use authenticated stateless `POST`, emit no session identifier, reserve no legacy session capacity, and propagate disconnect cancellation through the SDK stateless option;
- malformed, duplicate, empty, comma-joined, and whitespace-variant protocol-version headers fail before SDK dispatch, while any other exact unsupported singleton is routed only to the stateless SDK error path so it can return the protocol-defined structured unsupported-version response without creating session state;
- `Last-Event-ID` replay remains disabled because no event store is configured.

The stdio profile may use legacy MCP client roots only when no startup allowed directories were supplied. Roots are informational compatibility input, not an access-control boundary. Process-wide normalized allowed directories remain authoritative.

## Protocol changes relevant to this server

MCP `2026-07-28` changes the transport assumptions that affect this repository:

- protocol-level initialization and sessions are removed;
- each request carries protocol, client, and capability metadata;
- `server/discover` provides optional capability discovery;
- Streamable HTTP requests use standardized `Mcp-Method` and `Mcp-Name` headers;
- server-initiated calls are replaced by Multi Round-Trip Requests;
- list responses may include cache hints;
- roots, sampling, and logging are deprecated;
- legacy protocol versions remain relevant during the compatibility window.

This server does not currently initiate sampling or elicitation calls, expose resources, or depend on protocol logging. The main migration risks are therefore HTTP session coexistence, stdio roots compatibility, version routing, cancellation, header/body consistency, and downgrade behavior.

## Goals

R20 must:

- support the final `2026-07-28` protocol through an official stable Go SDK;
- preserve supported legacy versions and the published `2025-11-25` stateful behavior;
- preserve the same tool catalog, tool schemas, prompts, limits, allowed-root policy, error categories, and execution gates;
- keep stdio and Streamable HTTP behavior equivalent at the tool layer;
- add stateless HTTP without weakening authentication, Host, Origin, proxy, rate, concurrency, body, timeout, logging, or shutdown controls;
- prevent the new protocol from depending on deprecated client roots;
- retain deterministic downgrade and rejection behavior for old, new, malformed, missing, and unsupported protocol-version inputs;
- avoid pre-release dependencies and avoid adopting unrelated protocol extensions.

## Non-goals

R20 does not add:

- MCP Apps;
- Tasks;
- application-managed OAuth or an authorization server;
- Enterprise Managed Authorization;
- Multi Round-Trip Requests for tool confirmations or missing input;
- an application-selected positive list-result cache lifetime or cache policy;
- distributed tracing export;
- durable subscriptions or event replay;
- server-side protocol session state for `2026-07-28`;
- per-client roots or per-client filesystem ACLs;
- new MCP tools, prompts, resources, or public file-operation schemas;
- removal of legacy roots support before the compatibility policy permits it.

Each of these requires separate evidence and design if later needed.

## Stable SDK adoption gate

No implementation phase may update `go.mod` until all of the following are true:

1. the official Go SDK publishes a stable release that explicitly supports final protocol version `2026-07-28`;
2. the release is not marked pre-release and does not require a pseudo-version;
3. release notes and protocol documentation identify supported legacy versions and stateless HTTP behavior;
4. the module checksum is fetched through the existing Go module trust path;
5. the public APIs needed for version filtering, stateless HTTP, discovery, request cancellation, and legacy fallback are available without an SDK fork;
6. known security advisories and compatibility flags for the candidate SDK are reviewed;
7. a temporary qualification run passes outside the committed dependency graph before `go.mod` changes.

If the stable SDK cannot satisfy the design through public APIs, R20 stops for a new design review. It must not vendor or fork the SDK opportunistically.

## Phase 2 qualification record

Completed on 2026-08-07 against official stable `github.com/modelcontextprotocol/go-sdk v1.7.0`.

Release and dependency evidence:

- GitHub marks `v1.7.0` as a final, non-draft, non-prerelease release published on 2026-07-28; the Go module resolves the version tag to source commit `bc72835f62eb94d0fb484439f886b6885b075f36`.
- The module was fetched through the normal Go checksum path with module sum `h1:yqjY2dsbKAC0LSuWZVBMrHgiG8ukXv6NRo0JiALay44=` and `go.mod` sum `h1:dL7u98E/zjJTGzEq+j30jQ8K2k1mb6LeAH4inEcSGts=`.
- The temporary module change updates only the SDK and adds `golang.org/x/sync v0.22.0` plus `golang.org/x/time v0.15.0` as indirect dependencies. No new direct dependency is introduced.
- `go mod verify` and `go mod tidy -diff` pass. The SDK license file records the MCP project's Apache-2.0 transition while retaining MIT terms for contributions not yet relicensed.

Public API and protocol findings:

- `StreamableHTTPOptions.Stateless`, `MaxRequestBodyBytes`, and `PropagateRequestCancellation` provide the required stateless transport, bounded SDK body read, and disconnect cancellation controls.
- `server/discover`, per-request metadata, standardized HTTP header validation, legacy fallback, and transport-level `ProtocolVersionSupporter` are public SDK behavior; no vendoring or SDK fork is required.
- Stateful Streamable HTTP continues to negotiate `2025-11-25`; stateless Streamable HTTP negotiates `2026-07-28`, emits no `Mcp-Session-Id`, and exposes the same 27 tools and three prompts.
- Stateless request cancellation reaches a blocked tool handler when `PropagateRequestCancellation` is enabled.
- The default SDK capability projection still includes protocol logging unless `ServerOptions.Capabilities` is set explicitly. Phase 3 must project only implemented capabilities and preserve logging solely where legacy compatibility requires it.

Stdio design correction:

- `ProtocolVersionSupporter` correctly removes `2026-07-28` from `server/discover`, but using that filter alone on one persistent stdio-like connection causes the client to attempt legacy `initialize` after discovery has already initialized SDK session state; the SDK then rejects the request as duplicate initialization.
- A receiving middleware gate that rejects `server/discover` before the SDK discovery handler runs produces a clean fallback to legacy `initialize` and preserves the 27-tool, three-prompt catalog.
- Phase 3 therefore must not rely on transport filtering alone when startup roots are absent. It must reject discovery before SDK state mutation and retain the existing legacy roots initialization path. When startup roots are present, normal stdio discovery may negotiate `2026-07-28`.

Verification evidence:

- focused new/legacy HTTP, stdio fallback, catalog, no-session-header, and cancellation qualification tests pass;
- every Go package passes regression tests in deterministic shards, and every package passes the race detector with the workspace GCC/CGO toolchain;
- `go vet` passes; Staticcheck reports only intentional `SA1019` uses of legacy roots/logging, and the inherited check set passes when only that code is excluded;
- govulncheck reports no vulnerabilities;
- the operational harness passes all 27 tools, including backup capture, restore, and GC paths;
- Windows, Linux, and macOS builds succeed for amd64 and arm64;
- current release/manifest Node tests pass.

The compatibility flags documented by `v1.7.0` were reviewed and none is enabled. Official protocol conformance, an independent client, fuzzing, native external smoke, and container validation were deferred to Phase 5 and are recorded below. No launcher, runtime, deployment, release, tag, commit, or push changed during qualification.

## Phase 3 implementation record

Completed in source on 2026-08-07.

Stdio negotiation behavior:

- the source dependency is updated to official stable Go SDK `v1.7.0` with only the two qualified indirect additions, `golang.org/x/sync v0.22.0` and `golang.org/x/time v0.15.0`;
- when startup allowed directories exist, normal SDK discovery negotiates `2026-07-28` and tool requests remain bound to those process-owned directories;
- when client roots are disabled, stdio may also negotiate `2026-07-28`; an empty process root set remains empty and no client metadata broadens it;
- when client roots are enabled and no startup directories exist, a receiving middleware rejects `server/discover` with JSON-RPC `MethodNotFound` before the SDK discovery handler can mutate session state; the official client then performs the normal legacy initialization fallback and negotiates `2025-11-25`;
- legacy initialization and `notifications/roots/list_changed` continue to populate and clear the process-wide dynamic roots according to the existing validation rules;
- modern discovery removes deprecated protocol logging from its capability projection, while legacy initialization retains logging compatibility for the existing update notification path;
- the shared server, 27-tool catalog, three prompts, backup-store authority, execution policy, limits, error behavior, and lifecycle remain single-instance and transport-independent.

Verification evidence:

- TDD reproduced the pre-upgrade modern-negotiation failure on `v1.6.1`, then reproduced the unsafe modern negotiation of dynamic roots on `v1.7.0` before the middleware gate;
- focused tests cover configured startup roots, disabled client roots, legacy dynamic roots, roots-change notifications, exact negotiated versions, logging capability projection, 27 tools, and three prompts;
- focused server, command, and HTTP regressions pass; the complete Go suite passes serially, and the one test that exceeded its deadline during a highly parallel monolithic run passed immediately in isolation and in the complete serial run;
- every package passes the race detector with CGO and GCC, `go vet`, Staticcheck with only documented local legacy suppressions, and govulncheck;
- `go mod verify`, `go mod tidy -diff`, the complete 27-tool stdio harness, current Node release tests, and Windows/Linux/macOS amd64/arm64 builds pass;
- temporary build outputs were removed, and no launcher, active runtime, deployment, release, tag, or push changed.

Official protocol conformance, an independent client, fuzzing, native external smoke, and container validation remain Phase 5 work.

## Phase 4 implementation record

Completed in source on 2026-08-07.

HTTP routing behavior:

- the existing `/mcp` endpoint retains one Host, Origin, bearer-authentication, trusted-proxy, rate, concurrency, body-budget, timeout, logging, execution, readiness, and shutdown pipeline;
- the outer handler accepts exactly the five protocol versions supported by SDK `v1.7.0`: `2026-07-28`, `2025-11-25`, `2025-06-18`, `2025-03-26`, and `2024-11-05`;
- an absent protocol-version header remains valid only as the legacy initialization route, the four exact legacy versions route to the stateful handler, and exact `2026-07-28` routes to the stateless handler;
- repeated, empty, comma-joined, and whitespace-variant version values fail before either SDK handler runs; an otherwise well-formed unsupported singleton is routed to the stateless SDK solely so it can return `UnsupportedProtocolVersionError` with HTTP `400` and the supported/requested version data;
- the stateless handler enables `Stateless`, the configured SDK body bound, and `PropagateRequestCancellation`; it accepts only authenticated `POST`, rejects any `Mcp-Session-Id`, and never creates, acquires, releases, or consumes capacity in the legacy session tracker;
- the legacy handler retains stateful initialization, session IDs, authenticated `POST`/`GET`/`DELETE`, SSE, idle expiry, explicit deletion, and the existing bounded session gate;
- `Mcp-Method` and `Mcp-Name` remain untrusted network metadata: the application does not authorize from them, and a deliberately contradictory `Mcp-Method` is rejected by SDK header/body validation;
- both generations use the same `*mcp.Server`, 27 tools, three prompts, process roots, backup store, execution policy, and tool limits.

Verification evidence:

- TDD first proved that the pre-Phase-4 HTTP path negotiated only `2025-11-25`; the new modern-routing test then passed only after the stateless handler was added;
- focused routing tests cover exact modern/legacy versions, absent legacy initialization, malformed/repeated/contradictory headers, modern GET/DELETE/session-header rejection, no stateless session admission, a full legacy session gate, body-limit enforcement before SDK dispatch, SDK method-header mismatch rejection, 27 tools, three prompts, and modern disconnect cancellation;
- the complete HTTP package plus focused server/CLI integration pass, followed by the complete serial Go suite;
- every Go package passes the race detector with CGO/GCC, `go vet`, Staticcheck, govulncheck, `go mod verify`, and clean `go mod tidy -diff`;
- the 27-tool stdio harness, current Node release tests, and Windows/Linux/macOS amd64/arm64 builds pass, with temporary build outputs removed;
- no launcher, deployed runtime, release, tag, or push changed.

Official protocol conformance, independent-client interoperability, bounded fuzz campaigns, native external smoke, and container validation were deferred to Phase 5 and are recorded below.

## Phase 5 compatibility and conformance record

Completed in source on 2026-08-08.

Compatibility findings and fixes:

- official conformance first exposed one applicable defect: the outer HTTP router converted an otherwise well-formed unknown protocol version into a plain HTTP `400` before the SDK could return the required structured `UnsupportedProtocolVersionError`;
- the router now distinguishes malformed header shape from an unsupported singleton. Malformed/repeated/empty/comma-joined/whitespace-variant values still fail at the outer boundary; an unsupported singleton enters only the stateless SDK error lane, never session admission, and returns JSON-RPC code `-32022` with HTTP `400` plus requested/supported version data;
- the same change preserves every shared authentication, Host, Origin, proxy, rate, concurrency, body-budget, timeout, logging, execution, readiness, and shutdown control;
- Go SDK `v1.7.0` discovery/list serialization was verified to emit default `ttlMs: 0` and `cacheScope: "public"`; Scripthold selects no positive cache lifetime;
- execution-test infrastructure also reproduced a generic process-lifecycle edge where a descendant retaining inherited output handles could prolong `Wait` after its direct child exited. `internal/execution` now uses a bounded `Cmd.WaitDelay`, and the Windows process-tree helper has its own bounded termination timeout.

Conformance and interoperability evidence:

- `@modelcontextprotocol/conformance@0.2.0-alpha.10` was used only as external test tooling and is not part of the Go dependency graph;
- the final `2026-07-28` `server-stateless` run reports 24/28 checks successful. The remaining four are explicitly `Not testable` because the product does not expose the conformance suite's artificial `test_missing_capability`, `test_streaming_elicitation`, or `test_logging_tool` diagnostic tools; the structured unsupported-version and HTTP-400 checks are successful;
- the independent `http-header-validation` scenario passes 13/13 checks with no failures or warnings, covering method/name mismatch, missing headers, OWS handling, case-insensitive header names, and case-sensitive method values;
- tools-list, prompts-list, DNS-rebinding, caching, and multi-stream scenarios produced successful applicable checks. The custom-header scenario is not applicable because Scripthold declares no `x-mcp-header` tool annotations;
- on Windows, some alpha conformance invocations terminate in the Node/libuv harness after printing complete successful scenario results. Those post-result harness aborts are not counted as successful process exits and do not alter the individual protocol-check evidence;
- independent TypeScript SDK `1.30.0` interoperability negotiates legacy `2025-11-25` with a real session and observes all 27 tools and three prompts. Modern `2026-07-28` behavior is independently exercised by the official conformance client.

Verification evidence:

- focused routing, security-failure, body/concurrency, cancellation, session-capacity, and structured-error tests pass;
- the complete serial Go suite and complete race coverage pass, followed by `go vet`, Staticcheck, govulncheck, `go mod verify`, and clean `go mod tidy -diff`;
- bounded fuzz campaigns pass for protocol classification, Host normalization, Origin normalization, trusted-proxy client-address parsing, and JSON-RPC round trips. The JSON-RPC harness excludes only the documented Go SDK `v1.7.0` empty-method decode/encode asymmetry from its round-trip invariant;
- the full 27-tool operational harness and current Node release tests pass;
- a native Windows binary built from the final worktree passes the external stdio MCP smoke;
- Windows, Linux, and macOS amd64/arm64 command builds and affected command/HTTP/execution test binaries compile successfully;
- a fresh Linux/amd64 container built from the final worktree passes UID `10001`, read-only-root, dropped-capability, `no-new-privileges`, bounded-tmpfs, stdio MCP, direct-TLS HTTP security responses, and clean shutdown checks. A local auxiliary cross-container DNS topology was unavailable in the OCI test backend, so the successful product checks use an in-container TLS client while the repository CI recipe remains the cross-container/external-network gate;
- no push, tag, release, deployment, launcher modification, or runtime restart is part of this completion record.

## Transport architecture

### Shared server

Both protocol generations use the same `*mcp.Server` built by `filetoolsserver.BuildServer`. Tool registration, middleware, roots policy, backup-store authority, limits, execution policy, and lifecycle context remain shared.

Protocol selection must never create a second tool catalog or handler implementation.

### Stdio

Stdio remains the default transport.

When startup allowed directories are configured:

- the server may advertise `2026-07-28` and supported legacy versions;
- no request depends on client roots;
- tool behavior is identical across negotiated protocol versions.

When no startup allowed directories are configured:

- the server must not negotiate `2026-07-28` while filesystem authority would depend on deprecated client roots;
- protocol negotiation is capped to the highest legacy version that supports the existing roots compatibility path;
- legacy initialization and roots notifications continue to populate the process-wide root set according to the existing rules;
- startup does not silently broaden access and does not substitute the current working directory.

Phase 3 implements the reviewed receiving middleware gate. By default it rejects `server/discover` before SDK dispatch only when client roots are enabled and startup directories are absent; configured-root and roots-disabled sessions use normal modern discovery. R21 adds the stdio-only `MCP_STDIO_LEGACY_HANDSHAKE=1` compatibility override for intermediaries that probe discovery but then send legacy `initialize` on the same persistent connection. The override rejects discovery before the SDK can populate initialization state, allowing deterministic legacy fallback without making duplicate initialization valid. The OpenAI tunnel example opts into this compatibility mode for its dedicated `MCP_COMMAND` child. Streamable HTTP ignores the setting. The implementation does not fork the SDK, duplicate the server, or broaden filesystem authority.

### Streamable HTTP

The existing endpoint path remains unchanged. R20 adds two internal SDK handlers behind the same hardened outer middleware:

- **legacy stateful handler:** preserves the current `2025-11-25` and older behavior, session gate, authenticated `POST`/`GET`/`DELETE`, session timeout, and SSE semantics;
- **new stateless handler:** accepts `2026-07-28` requests, creates no protocol session, emits no `Mcp-Session-Id`, and uses no session gate or event store.

The outer handler selects the protocol path only from the normalized `MCP-Protocol-Version` header:

- exact `2026-07-28` routes to the stateless handler;
- supported legacy versions and legacy initialization without a version header route to the stateful handler;
- malformed, repeated, empty, comma-joined, or whitespace-variant version headers fail before SDK dispatch; an exact unsupported singleton uses the stateless SDK error path only to produce the protocol-defined structured unsupported-version response;
- `Mcp-Method` and `Mcp-Name` are never trusted for authorization or filesystem policy;
- header/body method-name consistency remains an SDK protocol-validation responsibility and must be covered by negative tests.

No middleware may pre-read or duplicate the JSON body merely to choose the handler. Body limits and aggregate reservations continue to wrap the single body stream before SDK decoding.

### Stateless HTTP method policy

For `2026-07-28`:

- authenticated `POST` is the MCP request method;
- protocol-level `GET` and `DELETE` are rejected because there is no session stream or session termination operation;
- `Last-Event-ID` remains rejected;
- any received `Mcp-Session-Id` is rejected rather than ignored;
- no session capacity is reserved;
- the normal non-SSE concurrency semaphore applies;
- client disconnect cancellation propagates to the tool handler through the stable SDK option intended for stateless requests;
- request timeout, body limits, rate limiting, authentication, Host, Origin, proxy, logging, and shutdown behavior remain unchanged.

Health and readiness routes are protocol-independent.

## Request admission order

The common outer pipeline remains security-significant:

1. identify peer and trusted-proxy status;
2. enforce proxy-only plaintext boundaries;
3. validate Host;
4. validate Origin for every method;
5. apply peer rate limiting;
6. serve health or readiness where applicable;
7. reject shutdown or non-ready work;
8. validate path and empty query;
9. authenticate the bearer token;
10. validate transport method and protocol-version header shape;
11. apply concurrency and body-budget admission;
12. route to the stateless or stateful SDK handler;
13. update legacy session admission only for the stateful path;
14. emit one category-only redacted access log.

Protocol generation does not alter bearer authority, process roots, or execution policy.

## Header handling

The new standard headers are untrusted network data.

- Reject multiple values for singleton MCP headers.
- Bound header bytes through the existing HTTP server limit.
- Require the exact final protocol version string for stateless routing.
- Do not accept version aliases, whitespace variants, date normalization, or prefix matching.
- Do not use `Mcp-Method` or `Mcp-Name` to bypass JSON-RPC decoding, tool lookup, authentication, rate limiting, or authorization.
- Do not log arbitrary header values.
- Negative tests must cover absent, duplicate, malformed, unsupported, conflicting, and body-mismatched headers.

Reverse proxies may route on standardized headers, but the application continues to validate the complete request independently.

## Discovery and capability projection

`server/discover` is provided by the stable SDK, not hand-written locally.

The advertised capability projection must:

- expose only protocol versions actually accepted by the selected transport/configuration;
- preserve the same server identity, instructions, tools, and prompts as legacy initialization;
- omit capabilities the server does not implement;
- avoid advertising deprecated roots support on `2026-07-28`;
- preserve legacy roots capability when the stdio compatibility path requires it;
- remain deterministic for the same binary and startup configuration.

No client-supplied discovery metadata changes process roots or tool registration.

## Tool-list caching

The stable Go SDK `v1.7.0` structurally emits its default cache fields in discovery/list results: `ttlMs: 0` and `cacheScope: "public"`. Scripthold does not select a positive cache lifetime or otherwise opt into reusable list-result caching; a zero TTL means the result is immediately stale.

Although this server's catalog is static after startup, any future positive cache lifetime would affect dynamic roots compatibility, prompt availability, execution policy, and later protocol behavior. Choosing a positive lifetime or a different application cache policy therefore remains separate future work.

## Multi Round-Trip Requests

The current tool handlers complete within one request and do not make server-to-client requests. R20 therefore advertises no MRTR-dependent capability and introduces no `input_required` flow.

Approval-bound edit, patch-package, restore, and garbage-collection operations retain their existing explicit preview/capability/apply contracts. They are not migrated to MRTR implicitly.

## Roots deprecation strategy

Roots remain a legacy stdio compatibility feature only.

- Startup roots remain the preferred and authoritative configuration for every protocol version.
- HTTP never accepts client roots.
- `2026-07-28` never relies on roots.
- Legacy stdio roots remain supported while the protocol compatibility window requires them.
- No warning is written to stdout; any deprecation notice belongs only in bounded stderr developer logging or documentation.
- Removing roots support requires a later milestone with migration evidence and explicit compatibility impact.

## Error and downgrade behavior

- A `2026-07-28` request sent to a binary that has not completed R20 receives a deterministic unsupported-version error from the existing stack.
- After R20, exact new-version requests use stateless semantics.
- Legacy clients continue to initialize and use stateful sessions unchanged.
- A new SDK client may discover and negotiate the highest mutually supported version.
- If stateless discovery fails, client-side fallback behavior is owned by the client SDK; the server does not emulate legacy initialization inside the stateless path.
- Unsupported versions fail rather than silently downgrade an individual request.
- The selected protocol generation cannot change inside one legacy session.
- JSON-RPC errors remain path-free and stack-free.

## Configuration impact

R20 did not require a protocol-mode setting for normal SDK clients or HTTP dual routing. R21 stdio compatibility testing exposed a distinct intermediary case: an intermediary may probe `server/discover` and then still send legacy `initialize` on the same connection, which SDK `v1.7.0` correctly rejects after discovery has populated initialization state. `MCP_STDIO_LEGACY_HANDSHAKE=1` is therefore a narrow stdio-only compatibility override; it defaults off, does not change HTTP routing, and rejects discovery before SDK state mutation rather than accepting duplicate initialization. The default OpenAI tunnel example enables it only for the tunnel-owned stdio process; the independent local HTTP process has separate transport state and ignores it.

The preferred outcome remains automatic backward-compatible routing:

- existing operators retain stateful clients without changing configuration;
- new clients can use stateless `2026-07-28` on the same authenticated endpoint;
- session limits apply only to legacy sessions;
- all other resource limits remain shared.

Any required new setting must have a secure default, hard bounds where applicable, startup validation, documentation, and configuration tests before implementation.

## Test strategy

### Dependency qualification

- stable SDK tag and module version only;
- `go mod tidy -diff` and `go mod verify`;
- inspect release compatibility flags and public API changes;
- no unexpected new direct dependencies;
- vulnerability and license review;
- clean downgrade back to the previous module files during the qualification spike.

### Stdio compatibility

- startup roots with a new client negotiate `2026-07-28` by default;
- `MCP_STDIO_LEGACY_HANDSHAKE=1` forces a clean legacy fallback for stdio intermediaries that probe discovery but still issue legacy initialization;
- startup roots with legacy clients negotiate supported legacy versions;
- no startup roots cap negotiation to the legacy roots-compatible protocol;
- roots notifications remain stdio-only and legacy-only;
- all 27 tools and prompts remain identical;
- representative read, write, preview/apply, backup, error, cancellation, and output-limit behavior remains equivalent.

### HTTP protocol routing

- exact new-version header reaches the stateless handler;
- legacy initialize without a version header reaches the stateful handler;
- supported legacy version headers reach the stateful handler;
- malformed, duplicate, empty, and unsupported version headers fail deterministically;
- `Mcp-Method` and `Mcp-Name` mismatch the body and are rejected;
- routing never requires reading the body twice;
- stateless requests emit no session header and consume no session slot;
- stateful session admission, expiry, GET, DELETE, and shutdown remain unchanged;
- stateless GET, DELETE, `Last-Event-ID`, and session headers are rejected;
- concurrent new and legacy clients remain isolated at protocol state while sharing process policy.

### Security and limits

Repeat the complete HTTP negative matrix for both protocol generations:

- authentication;
- Host and Origin;
- trusted and untrusted proxies;
- body and aggregate-body limits;
- request concurrency and peer rate limits;
- timeout and cancellation;
- logging redaction;
- execution dual opt-in;
- shutdown and readiness;
- malformed JSON and content types;
- no CORS headers;
- no path, token, body, complete session identifier, or header-value leakage.

### Interoperability and conformance

- official Go SDK client against stdio and HTTP;
- at least one independent current MCP client when available;
- official protocol conformance tests supported by the stable SDK;
- stateful legacy and stateless new requests in the same process;
- native Windows external smoke;
- Linux container direct-TLS HTTP smoke;
- six supported OS/architecture command and test compilation targets.

## Devil's advocate findings

### Risk: pre-release SDK behavior becomes production authority

A pre-release dependency could change protocol semantics, public APIs, or security defaults before stabilization. R20 blocks all runtime implementation until an official stable SDK release exists and passes a temporary qualification gate.

### Risk: same-endpoint dual routing creates an authentication or body-limit bypass

Two SDK handlers behind separate middleware stacks could diverge. R20 requires one common outer security/admission pipeline and routing only after authentication and bounded request admission. Protocol routing cannot bypass shared controls or consume the body twice.

### Risk: stateless requests leak into the legacy session gate

Reserving legacy session capacity for stateless traffic would create artificial denial of service and incorrect cleanup. The stateless path never creates, acquires, or releases an application session record; tests assert session counts remain unchanged.

### Risk: deprecated roots remain a hidden authority in the new protocol

Advertising the new protocol while depending on client roots could start the server with an empty or unsafe authority model. R20 caps stdio negotiation to a legacy version when startup roots are absent and never enables roots over HTTP.

### Risk: protocol headers become trusted authorization metadata

Gateways can route on `Mcp-Method` and `Mcp-Name`, but clients control them. The application continues to authenticate first and relies on SDK header/body consistency checks; filesystem and execution authorization remain based on decoded tool calls and process policy.

### Risk: enabling the new version silently changes published HTTP behavior

Legacy stateful sessions remain accepted on the same endpoint. New stateless behavior is selected only by the exact new protocol version. The completion gate includes direct regression evidence for the existing `2025-11-25` profile.

## Implementation phases

1. **Design and readiness — complete.** The compatibility contract, protocol coupling, and stable-SDK gate are approved.
2. **Stable SDK qualification — complete.** Official stable `v1.7.0` passed reversible dependency, API, security, compatibility, race, vulnerability, catalog, and six-target qualification without changing the committed module graph.
3. **Stdio version gating — complete.** Modern discovery is enabled for configured-root and roots-disabled sessions; a pre-discovery middleware gate retains legacy initialization and dynamic roots when startup directories are absent.
4. **Dual-generation HTTP — complete.** Exact version routing now selects separate stateful legacy and stateless `2026-07-28` SDK handlers behind the shared hardened middleware, with strict header rejection, stateless cancellation, and no stateless session admission.
5. **Compatibility and conformance — complete.** Official conformance, independent legacy-client interoperability, security failure injection, bounded fuzz campaigns, native and hardened-container smoke, complete race coverage, static/vulnerability analysis, and six-target command/test compilation passed. The conformance run also exposed and drove the structured unsupported-version fix.
6. **Documentation and completion — complete.** Protocol/security references, public status, publishing notes, cache-hint reality, and the completion record are aligned without changing the published runtime.

## Completion gate

R20 is complete only when an official stable Go SDK supports final protocol `2026-07-28`; stdio and HTTP accept the new version without relying on deprecated roots or protocol sessions; supported legacy clients retain their current behavior; stateless and stateful HTTP share every security and resource-control boundary; tool catalogs and results remain equivalent; malformed and downgrade cases fail deterministically; and the complete focused, regression, race, static, vulnerability, conformance, six-target, native, container, documentation, and security verification matrix passes.

## Completion record

The gate completed in source on 2026-08-08. Stable Go SDK `v1.7.0` is the only production MCP SDK dependency; modern stdio and stateless HTTP are verified beside retained legacy behavior; official conformance drove and then verified the structured unsupported-version path; independent legacy interoperability, fuzzing, native and hardened-container smoke, complete Go/race/static/vulnerability checks, and all six supported build targets passed. The source catalog remains 27 tools with three prompts. The milestone remains unreleased and undeployed.

No push, tag, release, deployment, launcher change, or runtime restart is part of the milestone without separate explicit authorization.
